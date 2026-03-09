package runner

import (
	"context"
	"testing"
	"time"
)

// ---------- Approve 流程 ----------

func TestResultApproval_Approve(t *testing.T) {
	var broadcastCalls []string
	broadcast := func(event string, payload interface{}) {
		broadcastCalls = append(broadcastCalls, event)
	}

	mgr := NewResultApprovalManager(broadcast, nil, 5*time.Second)
	defer mgr.Close()

	var decision ResultApprovalDecision
	var reqErr error
	done := make(chan struct{})

	go func() {
		decision, reqErr = mgr.RequestResultApproval(context.Background(), ResultApprovalRequest{
			OriginalTask: "build a REST API",
			ContractID:   "test-001",
			Result:       "code written",
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if mgr.PendingCount() != 1 {
		t.Fatalf("PendingCount() = %d, want 1", mgr.PendingCount())
	}

	// 找到 pending ID 并 approve
	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	if err := mgr.ResolveResultApproval(pendingID, ResultApprovalDecision{Action: "approve"}); err != nil {
		t.Fatalf("ResolveResultApproval error: %v", err)
	}

	<-done

	if reqErr != nil {
		t.Errorf("RequestResultApproval error: %v", reqErr)
	}
	if decision.Action != "approve" {
		t.Errorf("decision.Action = %q, want %q", decision.Action, "approve")
	}
	if mgr.PendingCount() != 0 {
		t.Errorf("PendingCount after resolve = %d, want 0", mgr.PendingCount())
	}

	// 验证广播事件
	if len(broadcastCalls) < 2 {
		t.Fatalf("broadcast called %d times, want >= 2", len(broadcastCalls))
	}
	if broadcastCalls[0] != "result.approve.requested" {
		t.Errorf("first broadcast = %q, want %q", broadcastCalls[0], "result.approve.requested")
	}
	if broadcastCalls[1] != "result.approve.resolved" {
		t.Errorf("second broadcast = %q, want %q", broadcastCalls[1], "result.approve.resolved")
	}
}

// ---------- Reject 流程 ----------

func TestResultApproval_Reject(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	var decision ResultApprovalDecision
	done := make(chan struct{})

	go func() {
		decision, _ = mgr.RequestResultApproval(context.Background(), ResultApprovalRequest{
			OriginalTask: "refactor login",
			ContractID:   "test-002",
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	_ = mgr.ResolveResultApproval(pendingID, ResultApprovalDecision{
		Action:   "reject",
		Feedback: "missing tests",
	})

	<-done

	if decision.Action != "reject" {
		t.Errorf("decision.Action = %q, want %q", decision.Action, "reject")
	}
	if decision.Feedback != "missing tests" {
		t.Errorf("decision.Feedback = %q, want %q", decision.Feedback, "missing tests")
	}
}

// ---------- 超时自动批准（与 Phase 1 超时拒绝不同） ----------

func TestResultApproval_TimeoutAutoApproves(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 200*time.Millisecond)
	defer mgr.Close()

	decision, err := mgr.RequestResultApproval(context.Background(), ResultApprovalRequest{
		OriginalTask: "slow review",
		ContractID:   "test-003",
	})

	if err != nil {
		t.Fatalf("RequestResultApproval error: %v", err)
	}
	if decision.Action != "approve" {
		t.Errorf("timeout decision.Action = %q, want %q", decision.Action, "approve")
	}
	if decision.Feedback != "timeout_auto_approved" {
		t.Errorf("timeout decision.Feedback = %q, want %q", decision.Feedback, "timeout_auto_approved")
	}
}

// ---------- Context 取消自动批准 ----------

func TestResultApproval_ContextCancel(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan ResultApprovalDecision)
	go func() {
		d, _ := mgr.RequestResultApproval(ctx, ResultApprovalRequest{
			OriginalTask: "cancelled review",
			ContractID:   "test-004",
		})
		done <- d
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	decision := <-done
	if decision.Action != "approve" {
		t.Errorf("cancel decision.Action = %q, want %q", decision.Action, "approve")
	}
}

// ---------- Resolve 错误路径 ----------

func TestResultApproval_ResolveUnknownID(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	err := mgr.ResolveResultApproval("nonexistent-id", ResultApprovalDecision{Action: "approve"})
	if err == nil {
		t.Error("ResolveResultApproval with unknown ID should return error")
	}
}

func TestResultApproval_ResolveInvalidAction(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	err := mgr.ResolveResultApproval("any-id", ResultApprovalDecision{Action: "edit"})
	if err == nil {
		t.Error("ResolveResultApproval with invalid action should return error")
	}
}

// ---------- Default Timeout ----------

func TestResultApproval_DefaultTimeout(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 0)
	defer mgr.Close()

	if mgr.Timeout() != 3*time.Minute {
		t.Errorf("default Timeout() = %v, want %v", mgr.Timeout(), 3*time.Minute)
	}
}

// ---------- Close 幂等性 ----------

func TestResultApproval_CloseIdempotent(t *testing.T) {
	mgr := NewResultApprovalManager(nil, nil, 5*time.Second)
	mgr.Close()
	mgr.Close() // 不应 panic
}

func TestResultApproval_BroadcastsWorkflowUpdates(t *testing.T) {
	type workflowEvent struct {
		source   string
		workflow ApprovalWorkflow
	}

	var events []workflowEvent
	broadcast := func(event string, payload interface{}) {
		if event != "approval.workflow.updated" {
			return
		}
		record, ok := payload.(map[string]interface{})
		if !ok {
			t.Fatalf("payload type = %T, want map[string]interface{}", payload)
		}
		source, _ := record["source"].(string)
		workflow, _ := record["workflow"].(ApprovalWorkflow)
		events = append(events, workflowEvent{source: source, workflow: workflow})
	}

	mgr := NewResultApprovalManager(broadcast, nil, 5*time.Second)
	defer mgr.Close()

	done := make(chan struct{})
	go func() {
		_, _ = mgr.RequestResultApproval(context.Background(), ResultApprovalRequest{
			OriginalTask: "review child result",
			ContractID:   "test-005",
			Result:       "all checks passed",
			Workflow: NewSingleStageApprovalWorkflow(
				"review child result",
				ApprovalTypeResultReview,
				"result_review（交付前最终签收）",
			),
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	if err := mgr.ResolveResultApproval(pendingID, ResultApprovalDecision{Action: "approve"}); err != nil {
		t.Fatalf("ResolveResultApproval error: %v", err)
	}

	<-done

	if len(events) != 2 {
		t.Fatalf("workflow events = %d, want 2", len(events))
	}
	if events[0].workflow.Status != ApprovalStagePending {
		t.Fatalf("first workflow status = %q, want pending", events[0].workflow.Status)
	}
	if events[1].workflow.Status != ApprovalStageApproved {
		t.Fatalf("final workflow status = %q, want approved", events[1].workflow.Status)
	}
}
