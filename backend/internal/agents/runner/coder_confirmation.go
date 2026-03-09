package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------- Coder 确认流 ----------

// CoderConfirmPreview 包含用于前端预览的工具调用详情。
type CoderConfirmPreview struct {
	FilePath  string `json:"filePath,omitempty"`
	OldString string `json:"oldString,omitempty"` // edit: 待替换文本片段
	NewString string `json:"newString,omitempty"` // edit: 新文本片段
	Content   string `json:"content,omitempty"`   // write: 内容预览 (截断 500 字符)
	Command   string `json:"command,omitempty"`   // bash: 命令文本
	LineCount int    `json:"lineCount,omitempty"` // write: 内容行数
}

// CoderConfirmationRequest 表示一个等待用户确认的 coder 操作。
type CoderConfirmationRequest struct {
	ID          string               `json:"id"`
	ToolName    string               `json:"toolName"` // "edit"|"write"|"bash"
	Args        json.RawMessage      `json:"args"`     // 原始参数
	Preview     *CoderConfirmPreview `json:"preview"`  // 预览数据
	CreatedAtMs int64                `json:"createdAtMs"`
	ExpiresAtMs int64                `json:"expiresAtMs"`
	Workflow    ApprovalWorkflow     `json:"workflow,omitempty"`
}

// CoderConfirmBroadcastFunc 广播回调（解耦 runner 与 gateway）。
type CoderConfirmBroadcastFunc func(event string, payload interface{})

// CoderConfirmRemoteNotifyFunc 远程通知回调（飞书/钉钉等非 Web 渠道）。
// sessionKey 用于确定目标渠道（如 "feishu:<chatID>"）。
type CoderConfirmRemoteNotifyFunc func(req CoderConfirmationRequest, sessionKey string)

// CoderConfirmationManager 管理 coder 工具调用确认流。
// 当 coder 触发可确认工具 (edit/write/bash/argus 高风险操作) 时：
//  1. 广播 "coder.confirm.requested" 给前端（WebSocket）
//  2. 推送远程通知到非 Web 渠道（飞书卡片等）
//  3. 阻塞等待用户决策（allow/deny）或超时
//  4. 前端通过 "coder.confirm.resolve" RPC 回调，或飞书卡片按钮回调
//
// 为 nil 时完全跳过确认（兼容现有行为）。
type CoderConfirmationManager struct {
	mu           sync.Mutex
	pending      map[string]*coderConfirmationEntry // id → request + decision channel
	broadcast    CoderConfirmBroadcastFunc
	remoteNotify CoderConfirmRemoteNotifyFunc
	timeout      time.Duration // 默认 5min

	// Phase 7: 合约感知审批路由
	approvalRouter *ApprovalRouter
	activeContract *DelegationContract
}

type coderConfirmationEntry struct {
	ch  chan string
	req CoderConfirmationRequest
}

// NewCoderConfirmationManager 创建确认管理器。
func NewCoderConfirmationManager(broadcastFn CoderConfirmBroadcastFunc, remoteNotifyFn CoderConfirmRemoteNotifyFunc, timeout time.Duration) *CoderConfirmationManager {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &CoderConfirmationManager{
		pending:      make(map[string]*coderConfirmationEntry),
		broadcast:    broadcastFn,
		remoteNotify: remoteNotifyFn,
		timeout:      timeout,
	}
}

// RequestConfirmation 请求用户确认一个 coder 操作。
// 阻塞直到用户决策、超时或 ctx 取消。
// sessionKey 标识发起会话（如 "feishu:<chatID>"），用于路由远程通知。
// 返回 true 表示用户批准，false 表示拒绝/超时/取消。
func (m *CoderConfirmationManager) RequestConfirmation(ctx context.Context, toolName string, args json.RawMessage, sessionKey string) (bool, error) {
	return m.RequestConfirmationWithMetadata(ctx, toolName, args, sessionKey, ApprovalWorkflow{})
}

func (m *CoderConfirmationManager) RequestConfirmationWithMetadata(
	ctx context.Context,
	toolName string,
	args json.RawMessage,
	sessionKey string,
	workflow ApprovalWorkflow,
) (bool, error) {
	// Phase 7: 合约上下文风险评估——低风险自动放行，高风险路由到用户
	// 快照：在锁下拷贝指针，避免与 SetActiveContract/ClearActiveContract 竞争
	m.mu.Lock()
	router := m.approvalRouter
	contract := m.activeContract
	m.mu.Unlock()
	if router != nil && contract != nil {
		decision, reason := router.RouteApproval(contract, toolName, args)
		router.recordAudit(contract.ContractID, toolName, decision, reason)
		switch decision {
		case ApprovalAutoApproved:
			slog.Debug("coder confirmation auto-approved by contract",
				"tool", toolName, "reason", reason, "contractID", contract.ContractID)
			return true, nil
		case ApprovalAutoDenied:
			slog.Info("coder confirmation auto-denied by contract",
				"tool", toolName, "reason", reason, "contractID", contract.ContractID)
			return false, nil
		}
		// ApprovalAskUser: 继续现有流程
	}

	now := time.Now()
	req := CoderConfirmationRequest{
		ID:          uuid.New().String(),
		ToolName:    toolName,
		Args:        args,
		Preview:     extractCoderPreview(toolName, args),
		CreatedAtMs: now.UnixMilli(),
		ExpiresAtMs: now.Add(m.timeout).UnixMilli(),
	}
	if stageType := workflowStageTypeForCoderTool(toolName); stageType != "" && workflow.ID != "" {
		req.Workflow = workflow.MarkStagePending(stageType, req.ID)
	} else {
		req.Workflow = workflow
	}

	ch := make(chan string, 1)

	m.mu.Lock()
	m.pending[req.ID] = &coderConfirmationEntry{ch: ch, req: req}
	m.mu.Unlock()

	// 广播确认请求到前端（WebSocket）
	if m.broadcast != nil {
		m.broadcast("coder.confirm.requested", req)
	}
	broadcastApprovalWorkflow(m.broadcast, req.Workflow, "coder.confirm.requested", req.ID)

	// 推送远程通知到非 Web 渠道（飞书卡片等）
	if m.remoteNotify != nil && sessionKey != "" {
		m.remoteNotify(req, sessionKey)
	}

	slog.Debug("coder confirmation requested",
		"id", req.ID,
		"tool", toolName,
		"sessionKey", sessionKey,
	)

	// 等待用户决策、超时或 ctx 取消
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()

	var decision string
	select {
	case decision = <-ch:
		// 用户已决策
	case <-timer.C:
		decision = "deny"
		slog.Info("coder confirmation timed out, auto-denying",
			"id", req.ID,
			"tool", toolName,
		)
	case <-ctx.Done():
		decision = "deny"
		slog.Debug("coder confirmation cancelled by context",
			"id", req.ID,
		)
	}

	// 清理 pending
	m.mu.Lock()
	delete(m.pending, req.ID)
	m.mu.Unlock()

	resolvedWorkflow := req.Workflow
	if stageType := workflowStageTypeForCoderTool(toolName); stageType != "" && resolvedWorkflow.ID != "" {
		if decision == "allow" {
			resolvedWorkflow = resolvedWorkflow.MarkStageResolved(stageType, req.ID, "approve")
		} else {
			resolvedWorkflow = resolvedWorkflow.MarkStageResolved(stageType, req.ID, "deny")
		}
	}

	// 广播决策结果
	if m.broadcast != nil {
		m.broadcast("coder.confirm.resolved", map[string]interface{}{
			"id":       req.ID,
			"decision": decision,
			"ts":       time.Now().UnixMilli(),
			"workflow": resolvedWorkflow,
		})
	}
	broadcastApprovalWorkflow(m.broadcast, resolvedWorkflow, "coder.confirm.resolved", req.ID)

	return decision == "allow", nil
}

// ResolveConfirmation 处理前端的确认决策回调。
// 由 WebSocket RPC "coder.confirm.resolve" 调用。
// 幂等性设计：双渠道（Web + 飞书）可能同时 resolve 同一 ID，
// 第二个调用静默成功（非 error），确保调用方无需处理竞态。
func (m *CoderConfirmationManager) ResolveConfirmation(id, decision string) error {
	if decision != "allow" && decision != "deny" {
		return fmt.Errorf("invalid decision: %q (expected allow/deny)", decision)
	}

	m.mu.Lock()
	entry, ok := m.pending[id]
	if ok {
		delete(m.pending, id) // 先删除，确保第二个 resolve 看到 not found
	}
	m.mu.Unlock()

	if !ok {
		// 幂等性：已被另一个渠道 resolve，属于正常竞态
		slog.Debug("coder confirmation already resolved (idempotent)",
			"id", id,
			"decision", decision,
		)
		return nil
	}

	// 非阻塞写入（channel 有 1 缓冲）
	select {
	case entry.ch <- decision:
		slog.Debug("coder confirmation resolved",
			"id", id,
			"decision", decision,
		)
	default:
		// channel 已被写入（超时先触发），忽略
		slog.Debug("coder confirmation channel already written", "id", id)
	}

	return nil
}

func (m *CoderConfirmationManager) PendingRequest(id string) (CoderConfirmationRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.pending[id]
	if !ok || entry == nil {
		return CoderConfirmationRequest{}, false
	}
	return entry.req, true
}

func workflowStageTypeForCoderTool(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "send_media":
		return ApprovalTypeDataExportRunner
	case "bash", "bash (write detected)":
		return ApprovalTypeExecEscalationRunner
	default:
		return ""
	}
}

// (Phase 2A: isCoderConfirmable 已删除 — 审批逻辑移至 Ask 规则和 Argus 审批)

// extractCoderPreview 从工具参数中提取预览数据。
func extractCoderPreview(toolName string, args json.RawMessage) *CoderConfirmPreview {
	var parsed map[string]interface{}
	if err := json.Unmarshal(args, &parsed); err != nil {
		return nil
	}

	preview := &CoderConfirmPreview{}

	switch toolName {
	case "edit":
		if v, ok := parsed["filePath"].(string); ok {
			preview.FilePath = v
		}
		if v, ok := parsed["oldString"].(string); ok {
			preview.OldString = truncatePreview(v, 500)
		}
		if v, ok := parsed["newString"].(string); ok {
			preview.NewString = truncatePreview(v, 500)
		}
	case "write":
		if v, ok := parsed["filePath"].(string); ok {
			preview.FilePath = v
		}
		if v, ok := parsed["content"].(string); ok {
			preview.Content = truncatePreview(v, 500)
			preview.LineCount = countLines(v)
		}
	case "bash", "bash (write detected)":
		if v, ok := parsed["command"].(string); ok {
			preview.Command = v
		}
	case "send_media":
		if v, ok := parsed["file_path"].(string); ok {
			preview.FilePath = v
		}
		target := "(current channel)"
		if v, ok := parsed["target"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		}
		fileLabel := strings.TrimSpace(preview.FilePath)
		if fileLabel == "" {
			if v, ok := parsed["file_name"].(string); ok && strings.TrimSpace(v) != "" {
				fileLabel = strings.TrimSpace(v)
			} else {
				fileLabel = "(inline media)"
			}
		}
		preview.Command = fmt.Sprintf("send %s to %s", fileLabel, target)
		if v, ok := parsed["message"].(string); ok && strings.TrimSpace(v) != "" {
			preview.Content = truncatePreview(v, 200)
		}
	default:
		// Argus 工具（argus_open_url, argus_run_shell 等）：
		// 提取 target/command 字段作为预览
		if v, ok := parsed["target"].(string); ok {
			preview.Command = v
		} else if v, ok := parsed["command"].(string); ok {
			preview.Command = v
		}
	}

	return preview
}

// truncatePreview 截断预览文本到指定长度。
func truncatePreview(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// countLines 统计文本行数。
func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}

// ---------- Phase 7: 集中审批路由 ----------

// ApprovalDecision 审批决策类型。
type ApprovalDecision string

const (
	ApprovalAutoApproved ApprovalDecision = "auto_approved"
	ApprovalAskUser      ApprovalDecision = "ask_user"
	ApprovalAutoDenied   ApprovalDecision = "auto_denied"
)

// ApprovalAuditEntry 审批审计条目。
type ApprovalAuditEntry struct {
	Timestamp  time.Time        `json:"timestamp"`
	ContractID string           `json:"contract_id,omitempty"`
	ToolName   string           `json:"tool_name"`
	Decision   ApprovalDecision `json:"decision"`
	Reason     string           `json:"reason"`
}

// ApprovalRouter 合约上下文风险评估路由器。
// 基于规则评估（非 LLM 调用），零延迟判定。
type ApprovalRouter struct {
	mu       sync.Mutex
	auditLog []ApprovalAuditEntry
}

// NewApprovalRouter 创建审批路由器。
func NewApprovalRouter() *ApprovalRouter {
	return &ApprovalRouter{}
}

// RouteApproval 根据合约上下文评估工具调用风险，返回审批决策和原因。
func (r *ApprovalRouter) RouteApproval(contract *DelegationContract, toolName string, args json.RawMessage) (ApprovalDecision, string) {
	if contract == nil {
		return ApprovalAskUser, "no active contract"
	}

	// 解析工具参数
	var parsed map[string]interface{}
	if err := json.Unmarshal(args, &parsed); err != nil {
		return ApprovalAskUser, "failed to parse args"
	}

	return assessToolRisk(contract, toolName, parsed)
}

// assessToolRisk 评估工具调用的风险等级。
// 规则评估，非 LLM 调用（LLM 每次 2-5s 延迟不可接受）。
func assessToolRisk(contract *DelegationContract, toolName string, parsed map[string]interface{}) (ApprovalDecision, string) {
	switch toolName {
	// 只读工具: 低风险，自动放行
	case "read", "read_file", "grep", "glob", "find", "ls", "list_dir", "search":
		return ApprovalAutoApproved, "read-only tool"

	// edit: 检查文件路径是否在 scope 内
	case "edit":
		filePath, _ := parsed["filePath"].(string)
		if filePath == "" {
			filePath, _ = parsed["file_path"].(string)
		}
		if filePath != "" && isPathInContractScope(contract, filePath) {
			return ApprovalAutoApproved, "edit within contract scope"
		}
		return ApprovalAskUser, "edit outside contract scope"

	// write: 即使在 scope 内也需要确认（创建新文件风险更高）
	case "write", "write_file":
		filePath, _ := parsed["filePath"].(string)
		if filePath == "" {
			filePath, _ = parsed["file_path"].(string)
		}
		if filePath != "" && isPathInContractScope(contract, filePath) {
			return ApprovalAskUser, "write within contract scope (requires confirmation)"
		}
		return ApprovalAskUser, "write outside contract scope"

	// bash: 检查命令是否在白名单内
	case "bash", "bash (write detected)":
		command, _ := parsed["command"].(string)
		if command == "" {
			return ApprovalAskUser, "empty command"
		}
		if isCommandInAllowedList(contract, command) {
			return ApprovalAutoApproved, "bash command in allowed list"
		}
		return ApprovalAskUser, "bash command not in allowed list"

	// argus_* 工具: 有独立审批系统（actionRiskMap），跳过
	default:
		if len(toolName) > 6 && toolName[:6] == "argus_" {
			return ApprovalAskUser, "argus tool (uses independent approval)"
		}
		return ApprovalAskUser, "unknown tool"
	}
}

// isPathInContractScope 检查文件路径是否在合约 scope 路径内。
func isPathInContractScope(contract *DelegationContract, filePath string) bool {
	if len(contract.Scope) == 0 {
		return false
	}
	scopePaths := make([]string, 0, len(contract.Scope))
	for _, s := range contract.Scope {
		scopePaths = append(scopePaths, s.Path)
	}
	return isPathUnderAny(filePath, scopePaths)
}

// isCommandInAllowedList 检查命令是否在合约白名单中。
// 要求精确的词边界匹配：允许 "cargo test" 不能匹配 "cargo test && rm -rf /"。
// 允许命令后跟空格+参数（如 "cargo test --release"），但不允许 shell 链接操作符（&&, ||, ;, |）。
func isCommandInAllowedList(contract *DelegationContract, command string) bool {
	if len(contract.Constraints.AllowedCommands) == 0 {
		return false
	}
	trimmed := strings.TrimSpace(command)
	// 拒绝包含 shell 链接操作符的命令（防止 "allowed && malicious"）
	for _, op := range []string{"&&", "||", ";", "|", "`", "$(", "\n"} {
		if strings.Contains(trimmed, op) {
			return false
		}
	}
	for _, allowed := range contract.Constraints.AllowedCommands {
		if trimmed == allowed {
			return true
		}
		// 允许命令后跟空格+参数（如 allowed="cargo test", command="cargo test --release"）
		if strings.HasPrefix(trimmed, allowed+" ") {
			return true
		}
	}
	return false
}

// recordAudit 记录审批审计条目。
func (r *ApprovalRouter) recordAudit(contractID, toolName string, decision ApprovalDecision, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditLog = append(r.auditLog, ApprovalAuditEntry{
		Timestamp:  time.Now(),
		ContractID: contractID,
		ToolName:   toolName,
		Decision:   decision,
		Reason:     reason,
	})
}

// FlushAudit 获取并清空审计日志。
func (r *ApprovalRouter) FlushAudit() []ApprovalAuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := r.auditLog
	r.auditLog = nil
	return entries
}

// SetActiveContract 设置当前活动合约（子 Agent spawn 前调用）。
func (m *CoderConfirmationManager) SetActiveContract(c *DelegationContract) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeContract = c
	// 按需创建路由器
	if m.approvalRouter == nil {
		m.approvalRouter = NewApprovalRouter()
	}
}

// ClearActiveContract 清除当前活动合约（子 Agent 完成后调用）。
func (m *CoderConfirmationManager) ClearActiveContract() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeContract = nil
}

// ApprovalRouter 返回审批路由器（可能为 nil）。
func (m *CoderConfirmationManager) ApprovalRouterRef() *ApprovalRouter {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.approvalRouter
}
