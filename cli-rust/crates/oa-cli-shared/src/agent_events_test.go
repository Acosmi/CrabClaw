package infra

import (
	"testing"
)

func TestRegisterAndGetAgentRunContext(t *testing.T) {
	ResetAgentRunContextForTest()

	// 空 runID → 不注册
	RegisterAgentRunContext("", AgentRunContext{SessionKey: "s1"})
	if got := GetAgentRunContext(""); got != nil {
		t.Error("expected nil for empty runID")
	}

	// 正常注册
	RegisterAgentRunContext("run-1", AgentRunContext{
		SessionKey:   "sk-1",
		VerboseLevel: "high",
		IsHeartbeat:  false,
	})
	ctx := GetAgentRunContext("run-1")
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.SessionKey != "sk-1" || ctx.VerboseLevel != "high" {
		t.Errorf("unexpected context: %+v", ctx)
	}

	// 更新合并
	RegisterAgentRunContext("run-1", AgentRunContext{
		VerboseLevel: "low",
		IsHeartbeat:  true,
	})
	ctx = GetAgentRunContext("run-1")
	if ctx.SessionKey != "sk-1" {
		t.Error("sessionKey should be preserved")
	}
	if ctx.VerboseLevel != "low" {
		t.Errorf("verboseLevel not updated: %s", ctx.VerboseLevel)
	}
	if !ctx.IsHeartbeat {
		t.Error("isHeartbeat should be true")
	}

	// 清除
	ClearAgentRunContext("run-1")
	if got := GetAgentRunContext("run-1"); got != nil {
		t.Error("expected nil after clear")
	}
}

func TestEmitAgentEvent(t *testing.T) {
	ResetAgentRunContextForTest()

	var received []AgentEventPayload
	cancel := OnAgentEvent(func(evt AgentEventPayload) {
		received = append(received, evt)
	})
	defer cancel()

	RegisterAgentRunContext("r1", AgentRunContext{SessionKey: "sk"})
	EmitAgentEvent("r1", StreamTool, map[string]interface{}{"key": "val"}, "")

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	evt := received[0]
	if evt.Seq != 1 {
		t.Errorf("expected seq 1, got %d", evt.Seq)
	}
	if evt.SessionKey != "sk" {
		t.Errorf("expected sessionKey from context, got %q", evt.SessionKey)
	}

	// 第二次发射 → seq 递增
	EmitAgentEvent("r1", StreamAssistant, nil, "override-sk")
	if received[1].Seq != 2 {
		t.Errorf("expected seq 2, got %d", received[1].Seq)
	}
	if received[1].SessionKey != "override-sk" {
		t.Error("explicit sessionKey should override")
	}
}

func TestGetAgentRunContext_ReturnsCopy(t *testing.T) {
	ResetAgentRunContextForTest()
	RegisterAgentRunContext("r2", AgentRunContext{SessionKey: "orig"})
	ctx := GetAgentRunContext("r2")
	ctx.SessionKey = "mutated"
	ctx2 := GetAgentRunContext("r2")
	if ctx2.SessionKey != "orig" {
		t.Error("GetAgentRunContext should return a copy")
	}
}
