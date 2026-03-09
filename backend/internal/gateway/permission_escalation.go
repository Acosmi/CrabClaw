package gateway

// permission_escalation.go — P2 权限提升管理器
// 行业对照: Britive JIT Access / Zero Standing Privileges (ZSP)
//
// 管理智能体临时权限提升的完整生命周期：
//   - 请求提权 → 推送 WebSocket 事件
//   - 用户审批/拒绝 → 设置 TTL / 记审计
//   - TTL 到期 / 任务完成 → 自动降权
//
// 线程安全：所有状态操作通过 sync.Mutex 保护。

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// ---------- 类型定义 ----------

// MountRequest 临时路径放行请求（工作区默认路径不在此列）。
type MountRequest struct {
	HostPath  string `json:"hostPath"`  // 宿主机绝对路径
	MountMode string `json:"mountMode"` // "ro" 或 "rw"
}

func sanitizeMountRequests(reqs []MountRequest) []MountRequest {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]MountRequest, 0, len(reqs))
	seen := make(map[string]struct{}, len(reqs))
	for _, r := range reqs {
		hostPath := strings.TrimSpace(r.HostPath)
		if hostPath == "" || !filepath.IsAbs(hostPath) {
			continue
		}
		hostPath = filepath.Clean(hostPath)
		mode := strings.ToLower(strings.TrimSpace(r.MountMode))
		if mode != "rw" {
			mode = "ro"
		}
		key := hostPath + "|" + mode
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, MountRequest{HostPath: hostPath, MountMode: mode})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeMountRequests(base, extra []MountRequest) []MountRequest {
	combined := make([]MountRequest, 0, len(base)+len(extra))
	combined = append(combined, base...)
	combined = append(combined, extra...)
	return sanitizeMountRequests(combined)
}

// PendingEscalationRequest 等待审批的提权请求。
type PendingEscalationRequest struct {
	ID             string                  `json:"id"`
	RequestedLevel string                  `json:"requestedLevel"` // "allowlist" | "sandboxed" | "full"
	ApprovalType   string                  `json:"approvalType,omitempty"`
	Reason         string                  `json:"reason"`
	RunID          string                  `json:"runId,omitempty"`
	SessionID      string                  `json:"sessionId,omitempty"`
	RequestedAt    time.Time               `json:"requestedAt"`
	TTLMinutes     int                     `json:"ttlMinutes"`              // 建议的 TTL
	MountRequests  []MountRequest          `json:"mountRequests,omitempty"` // 临时路径放行
	Workflow       runner.ApprovalWorkflow `json:"workflow,omitempty"`
}

type EscalationRequestOptions struct {
	ID               string
	Level            string
	Reason           string
	RunID            string
	SessionID        string
	OriginatorChatID string
	OriginatorUserID string
	TTLMinutes       int
	MountRequests    []MountRequest
	Workflow         runner.ApprovalWorkflow
}

// ActiveEscalationGrant 当前活跃的临时提权。
type ActiveEscalationGrant struct {
	ID            string         `json:"id"`
	Level         string         `json:"level"` // 临时级别：allowlist | sandboxed | full
	GrantedAt     time.Time      `json:"grantedAt"`
	ExpiresAt     time.Time      `json:"expiresAt"`
	RunID         string         `json:"runId,omitempty"`
	SessionID     string         `json:"sessionId,omitempty"`
	MountRequests []MountRequest `json:"mountRequests,omitempty"` // 临时路径放行配置
}

// EscalationStatus 提权状态快照（供 API 返回）。
type EscalationStatus struct {
	HasPending  bool                      `json:"hasPending"`
	Pending     *PendingEscalationRequest `json:"pending,omitempty"`
	HasActive   bool                      `json:"hasActive"`
	Active      *ActiveEscalationGrant    `json:"active,omitempty"`
	BaseLevel   string                    `json:"baseLevel"`   // exec-approvals 持久化级别
	ActiveLevel string                    `json:"activeLevel"` // 有效级别（含临时提权）
}

func buildTypedMountAccessResult(req *PendingEscalationRequest, result ApprovalResultNotification) *TypedApprovalResultNotification {
	if req == nil || req.ApprovalType != ApprovalTypeMountAccess || len(req.MountRequests) != 1 {
		return nil
	}
	return &TypedApprovalResultNotification{
		Type:       ApprovalTypeMountAccess,
		ID:         req.ID,
		Approved:   result.Approved,
		Reason:     result.Reason,
		TTLMinutes: result.TTLMinutes,
		MountPath:  req.MountRequests[0].HostPath,
		MountMode:  req.MountRequests[0].MountMode,
		Workflow:   req.Workflow,
	}
}

func buildTypedExecEscalationResult(req *PendingEscalationRequest, result ApprovalResultNotification) *TypedApprovalResultNotification {
	if req == nil || req.ApprovalType != ApprovalTypeExecEscalation {
		return nil
	}
	return &TypedApprovalResultNotification{
		Type:           ApprovalTypeExecEscalation,
		ID:             req.ID,
		Approved:       result.Approved,
		Reason:         result.Reason,
		TTLMinutes:     result.TTLMinutes,
		RequestedLevel: result.RequestedLevel,
		Workflow:       req.Workflow,
	}
}

func notifyEscalationResult(remote *RemoteApprovalNotifier, req *PendingEscalationRequest, result ApprovalResultNotification) {
	if remote == nil {
		return
	}
	if typed := buildTypedMountAccessResult(req, result); typed != nil {
		remote.NotifyTypedOrApprovalResult(typed, result)
		return
	}
	if typed := buildTypedExecEscalationResult(req, result); typed != nil {
		remote.NotifyTypedOrApprovalResult(typed, result)
		return
	}
	remote.NotifyResult(result)
}

// ---------- 管理器 ----------

// EscalationManager 管理临时权限提升的生命周期。
type EscalationManager struct {
	mu              sync.Mutex
	pending         *PendingEscalationRequest
	active          *ActiveEscalationGrant
	broadcaster     *Broadcaster
	auditLogger     *EscalationAuditLogger
	deescalateTimer *time.Timer
	approvalTimeout *time.Timer             // Phase 8: 审批超时定时器
	remoteNotifier  *RemoteApprovalNotifier // P4: 远程审批通知
	maxAllowedLevel string                  // 默认 "sandboxed"，需显式配置才可设为 "full"
	log             *slog.Logger
}

// NewEscalationManager 创建提权管理器。
func NewEscalationManager(broadcaster *Broadcaster, auditLogger *EscalationAuditLogger, remoteNotifier *RemoteApprovalNotifier) *EscalationManager {
	return &EscalationManager{
		broadcaster:     broadcaster,
		auditLogger:     auditLogger,
		remoteNotifier:  remoteNotifier,
		maxAllowedLevel: string(infra.ExecSecuritySandboxed), // 默认上限 L2，L3 需显式启用
		log:             slog.Default().With("subsystem", "escalation-mgr"),
	}
}

// SetMaxAllowedLevel 设置权限上限（由配置注入）。
func (m *EscalationManager) SetMaxAllowedLevel(level string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxAllowedLevel = level
}

// ---------- 请求提权 ----------

// RequestEscalation 智能体请求临时提权。
// 如果已有 pending 请求，返回错误。
// originatorChatID: 触发权限请求的群聊 ID（如飞书 chat_id），用于审批卡片群发。
// originatorUserID: 触发权限请求的远程用户 ID（如飞书 open_id），用于审批卡片私聊。
// mountRequests: 临时路径放行审批请求（可选）。
func (m *EscalationManager) RequestEscalation(id, level, reason, runID, sessionID, originatorChatID, originatorUserID string, ttlMinutes int, mountRequests ...MountRequest) error {
	return m.RequestEscalationWithMetadata(EscalationRequestOptions{
		ID:               id,
		Level:            level,
		Reason:           reason,
		RunID:            runID,
		SessionID:        sessionID,
		OriginatorChatID: originatorChatID,
		OriginatorUserID: originatorUserID,
		TTLMinutes:       ttlMinutes,
		MountRequests:    mountRequests,
	})
}

func (m *EscalationManager) RequestEscalationWithMetadata(opts EscalationRequestOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sanitizedMounts := sanitizeMountRequests(opts.MountRequests)

	if m.pending != nil {
		return fmt.Errorf("already have a pending escalation request (id=%s)", m.pending.ID)
	}

	allowMountExtensionOnActive := m.active != nil &&
		m.active.Level == opts.Level &&
		len(sanitizedMounts) > 0
	if m.active != nil && !allowMountExtensionOnActive {
		return fmt.Errorf("already have an active escalation grant (level=%s, expires=%s)", m.active.Level, m.active.ExpiresAt.Format(time.RFC3339))
	}

	// 验证 level（支持 L1/L2/L3 提权）
	if opts.Level != string(infra.ExecSecurityAllowlist) &&
		opts.Level != string(infra.ExecSecuritySandboxed) &&
		opts.Level != string(infra.ExecSecurityFull) {
		return fmt.Errorf("invalid escalation level %q, must be \"allowlist\", \"sandboxed\", or \"full\"", opts.Level)
	}

	// Design Fix 3: base level 已满足请求级别时不创建 pending。
	// 例外: 同级路径放行扩展（mountRequests 非空）允许继续走审批。
	baseLevel := readBaseSecurityLevel()
	baseSatisfied := infra.LevelOrder(infra.ExecSecurity(baseLevel)) >= infra.LevelOrder(infra.ExecSecurity(opts.Level))
	if baseSatisfied && len(sanitizedMounts) == 0 {
		return fmt.Errorf("base level %q already satisfies requested level %q", baseLevel, opts.Level)
	}

	// 权限边界检查：requestedLevel 不得超过 maxAllowedLevel
	if m.maxAllowedLevel != "" && infra.LevelOrder(infra.ExecSecurity(opts.Level)) > infra.LevelOrder(infra.ExecSecurity(m.maxAllowedLevel)) {
		return fmt.Errorf("requested level %q exceeds max allowed level %q", opts.Level, m.maxAllowedLevel)
	}

	if opts.TTLMinutes <= 0 && !isPermanentEscalationLevel(opts.Level) {
		opts.TTLMinutes = 30 // 默认 30 分钟
	}

	m.pending = &PendingEscalationRequest{
		ID:             opts.ID,
		RequestedLevel: opts.Level,
		ApprovalType:   ApprovalTypeExecEscalation,
		Reason:         opts.Reason,
		RunID:          opts.RunID,
		SessionID:      opts.SessionID,
		RequestedAt:    time.Now(),
		TTLMinutes:     opts.TTLMinutes,
		MountRequests:  sanitizedMounts,
		Workflow:       opts.Workflow,
	}
	if len(sanitizedMounts) == 1 {
		m.pending.ApprovalType = ApprovalTypeMountAccess
		if m.pending.Workflow.ID != "" {
			m.pending.Workflow = m.pending.Workflow.MarkStagePending(ApprovalTypeMountAccess, opts.ID)
		}
	}

	m.log.Info("escalation requested",
		"id", opts.ID,
		"level", opts.Level,
		"reason", opts.Reason,
		"runId", opts.RunID,
		"ttlMinutes", opts.TTLMinutes,
	)

	// 审计日志
	if m.auditLogger != nil {
		m.auditLogger.Log(EscalationAuditEntry{
			Timestamp:      time.Now(),
			Event:          AuditEventRequest,
			RequestID:      opts.ID,
			RequestedLevel: opts.Level,
			Reason:         opts.Reason,
			RunID:          opts.RunID,
			SessionID:      opts.SessionID,
			TTLMinutes:     opts.TTLMinutes,
		})
	}

	// 广播给前端
	if m.broadcaster != nil {
		m.broadcaster.Broadcast("exec.approval.requested", map[string]interface{}{
			"id":             opts.ID,
			"requestedLevel": opts.Level,
			"reason":         opts.Reason,
			"runId":          opts.RunID,
			"sessionId":      opts.SessionID,
			"requestedAt":    m.pending.RequestedAt.UnixMilli(),
			"ttlMinutes":     opts.TTLMinutes,
			"mountRequests":  sanitizedMounts,
			"workflow":       m.pending.Workflow,
		}, nil)
	}
	runnerBroadcastWorkflow := m.pending.Workflow
	if m.broadcaster != nil && runnerBroadcastWorkflow.ID != "" {
		m.broadcaster.Broadcast("approval.workflow.updated", map[string]interface{}{
			"source":    "exec.approval.requested",
			"requestId": opts.ID,
			"workflow":  runnerBroadcastWorkflow,
			"ts":        time.Now().UnixMilli(),
		}, nil)
	}

	// P4: 同时推送远程审批通知（异步，不阻塞）。
	// 单一 mount request 优先走 typed mount_access 卡片；其余 provider 回退到 legacy 审批卡片。
	if m.remoteNotifier != nil {
		approvalReq := ApprovalCardRequest{
			EscalationID:     opts.ID,
			RequestedLevel:   opts.Level,
			Reason:           opts.Reason,
			RunID:            opts.RunID,
			SessionID:        opts.SessionID,
			TTLMinutes:       opts.TTLMinutes,
			RequestedAt:      m.pending.RequestedAt,
			OriginatorChatID: opts.OriginatorChatID,
			OriginatorUserID: opts.OriginatorUserID,
			Workflow:         m.pending.Workflow,
		}
		var typedReq *TypedApprovalRequest
		if len(sanitizedMounts) == 1 {
			typedReq = &TypedApprovalRequest{
				Type:             ApprovalTypeMountAccess,
				ID:               opts.ID,
				Reason:           opts.Reason,
				TTLMinutes:       opts.TTLMinutes,
				RequestedAt:      m.pending.RequestedAt,
				OriginatorChatID: opts.OriginatorChatID,
				OriginatorUserID: opts.OriginatorUserID,
				MountPath:        sanitizedMounts[0].HostPath,
				MountMode:        sanitizedMounts[0].MountMode,
				Workflow:         m.pending.Workflow,
			}
		}
		m.remoteNotifier.NotifyEscalation(approvalReq, typedReq)
	}

	// Phase 8: 启动审批超时定时器（默认 10 分钟，与 TTL 解耦）
	m.startApprovalTimeoutLocked(10 * time.Minute)

	// Phase 4.1: 持久化到磁盘（best-effort，错误仅 warn）
	m.persistPendingLocked()

	return nil
}

// ---------- 审批/拒绝 ----------

// ResolveEscalation 用户审批或拒绝提权请求。
// approve=true → 创建 activeGrant + 启动 TTL 定时器。
// approve=false → 清除 pending + 广播拒绝事件。
func (m *EscalationManager) ResolveEscalation(approve bool, ttlMinutes int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pending == nil {
		return fmt.Errorf("no pending escalation request to resolve")
	}

	req := m.pending
	existingActive := m.active
	m.pending = nil
	stageType := ApprovalTypeExecEscalation
	if req.ApprovalType == ApprovalTypeMountAccess {
		stageType = ApprovalTypeMountAccess
	}

	// Phase 4.1: 清除磁盘持久化（best-effort）
	m.clearPersistedPending()

	// Phase 8: 清除审批超时定时器
	m.stopApprovalTimeoutLocked()

	if !approve {
		resolvedWorkflow := req.Workflow
		if resolvedWorkflow.ID != "" {
			resolvedWorkflow = resolvedWorkflow.MarkStageResolved(stageType, req.ID, "deny")
		}
		m.log.Info("escalation denied",
			"id", req.ID,
			"level", req.RequestedLevel,
		)

		if m.auditLogger != nil {
			m.auditLogger.Log(EscalationAuditEntry{
				Timestamp:      time.Now(),
				Event:          AuditEventDeny,
				RequestID:      req.ID,
				RequestedLevel: req.RequestedLevel,
				RunID:          req.RunID,
				SessionID:      req.SessionID,
			})
		}

		if m.broadcaster != nil {
			m.broadcaster.Broadcast("exec.approval.resolved", map[string]interface{}{
				"id":       req.ID,
				"approved": false,
				"level":    string(infra.ExecSecurityDeny),
				"workflow": resolvedWorkflow,
			}, nil)
			m.broadcaster.Broadcast("approval.workflow.updated", map[string]interface{}{
				"source":    "exec.approval.resolved",
				"requestId": req.ID,
				"workflow":  resolvedWorkflow,
				"ts":        time.Now().UnixMilli(),
			}, nil)
		}

		// Phase 8: 推送拒绝结果卡片
		notifyEscalationResult(m.remoteNotifier, req, ApprovalResultNotification{
			EscalationID:   req.ID,
			Approved:       false,
			Reason:         "审批请求被拒绝 / Approval request denied",
			RequestedLevel: req.RequestedLevel,
		})
		return nil
	}

	baseLevel := readBaseSecurityLevel()
	if infra.LevelOrder(infra.ExecSecurity(baseLevel)) >= infra.LevelOrder(infra.ExecSecurity(req.RequestedLevel)) {
		resolvedWorkflow := req.Workflow
		if resolvedWorkflow.ID != "" {
			resolvedWorkflow = resolvedWorkflow.MarkStageResolved(stageType, req.ID, "approve")
		}
		m.log.Info("escalation satisfied by base level",
			"id", req.ID,
			"requestedLevel", req.RequestedLevel,
			"baseLevel", baseLevel,
		)

		if m.auditLogger != nil {
			m.auditLogger.Log(EscalationAuditEntry{
				Timestamp:      time.Now(),
				Event:          AuditEventApprove,
				RequestID:      req.ID,
				RequestedLevel: baseLevel,
				RunID:          req.RunID,
				SessionID:      req.SessionID,
			})
		}

		if m.broadcaster != nil {
			payload := map[string]interface{}{
				"id":       req.ID,
				"approved": true,
				"level":    baseLevel,
			}
			if isPermanentEscalationLevel(baseLevel) {
				payload["permanent"] = true
			}
			payload["workflow"] = resolvedWorkflow
			m.broadcaster.Broadcast("exec.approval.resolved", payload, nil)
			m.broadcaster.Broadcast("approval.workflow.updated", map[string]interface{}{
				"source":    "exec.approval.resolved",
				"requestId": req.ID,
				"workflow":  resolvedWorkflow,
				"ts":        time.Now().UnixMilli(),
			}, nil)
		}

		notifyEscalationResult(m.remoteNotifier, req, ApprovalResultNotification{
			EscalationID:   req.ID,
			Approved:       true,
			RequestedLevel: baseLevel,
		})
		return nil
	}

	if isPermanentEscalationLevel(req.RequestedLevel) {
		resolvedWorkflow := req.Workflow
		if resolvedWorkflow.ID != "" {
			resolvedWorkflow = resolvedWorkflow.MarkStageResolved(stageType, req.ID, "approve")
		}
		if err := persistBaseSecurityLevel(infra.ExecSecurityFull); err != nil {
			return fmt.Errorf("persist permanent full access: %w", err)
		}
		if existingActive != nil && isPermanentEscalationLevel(existingActive.Level) {
			m.active = nil
			if m.deescalateTimer != nil {
				m.deescalateTimer.Stop()
				m.deescalateTimer = nil
			}
		}

		m.log.Info("permanent escalation approved",
			"id", req.ID,
			"level", req.RequestedLevel,
		)

		if m.auditLogger != nil {
			m.auditLogger.Log(EscalationAuditEntry{
				Timestamp:      time.Now(),
				Event:          AuditEventApprove,
				RequestID:      req.ID,
				RequestedLevel: req.RequestedLevel,
				RunID:          req.RunID,
				SessionID:      req.SessionID,
			})
		}

		if m.broadcaster != nil {
			m.broadcaster.Broadcast("exec.approval.resolved", map[string]interface{}{
				"id":        req.ID,
				"approved":  true,
				"level":     req.RequestedLevel,
				"permanent": true,
				"workflow":  resolvedWorkflow,
			}, nil)
			m.broadcaster.Broadcast("approval.workflow.updated", map[string]interface{}{
				"source":    "exec.approval.resolved",
				"requestId": req.ID,
				"workflow":  resolvedWorkflow,
				"ts":        time.Now().UnixMilli(),
			}, nil)
		}

		notifyEscalationResult(m.remoteNotifier, req, ApprovalResultNotification{
			EscalationID:   req.ID,
			Approved:       true,
			RequestedLevel: req.RequestedLevel,
		})
		return nil
	}

	// 审批通过
	resolvedWorkflow := req.Workflow
	if resolvedWorkflow.ID != "" {
		resolvedWorkflow = resolvedWorkflow.MarkStageResolved(stageType, req.ID, "approve")
	}
	if ttlMinutes <= 0 {
		ttlMinutes = req.TTLMinutes
	}
	if ttlMinutes <= 0 {
		ttlMinutes = 30
	}

	// 分级 TTL 硬上限（临时授权路径）
	// L2(sandboxed): 4 小时（有沙箱保护但有网络）
	// L1(allowlist): 8 小时（受限操作，风险较低）
	switch req.RequestedLevel {
	case string(infra.ExecSecuritySandboxed):
		if ttlMinutes > 240 {
			ttlMinutes = 240
		}
	case string(infra.ExecSecurityAllowlist):
		if ttlMinutes > 480 {
			ttlMinutes = 480
		}
	}

	now := time.Now()
	mergedMounts := req.MountRequests
	runID := req.RunID
	sessionID := req.SessionID
	if existingActive != nil && existingActive.Level == req.RequestedLevel {
		mergedMounts = mergeMountRequests(existingActive.MountRequests, req.MountRequests)
		if runID == "" {
			runID = existingActive.RunID
		}
		if sessionID == "" {
			sessionID = existingActive.SessionID
		}
	}
	m.active = &ActiveEscalationGrant{
		ID:            req.ID,
		Level:         req.RequestedLevel,
		GrantedAt:     now,
		ExpiresAt:     now.Add(time.Duration(ttlMinutes) * time.Minute),
		RunID:         runID,
		SessionID:     sessionID,
		MountRequests: mergedMounts,
	}

	m.log.Info("escalation approved",
		"id", req.ID,
		"level", req.RequestedLevel,
		"ttlMinutes", ttlMinutes,
		"expiresAt", m.active.ExpiresAt.Format(time.RFC3339),
	)

	if m.auditLogger != nil {
		m.auditLogger.Log(EscalationAuditEntry{
			Timestamp:      time.Now(),
			Event:          AuditEventApprove,
			RequestID:      req.ID,
			RequestedLevel: req.RequestedLevel,
			RunID:          req.RunID,
			SessionID:      req.SessionID,
			TTLMinutes:     ttlMinutes,
		})
	}

	if m.broadcaster != nil {
		m.broadcaster.Broadcast("exec.approval.resolved", map[string]interface{}{
			"id":        req.ID,
			"approved":  true,
			"level":     req.RequestedLevel,
			"expiresAt": m.active.ExpiresAt.UnixMilli(),
			"workflow":  resolvedWorkflow,
		}, nil)
		m.broadcaster.Broadcast("approval.workflow.updated", map[string]interface{}{
			"source":    "exec.approval.resolved",
			"requestId": req.ID,
			"workflow":  resolvedWorkflow,
			"ts":        time.Now().UnixMilli(),
		}, nil)
	}

	// 启动自动降权定时器
	m.startDeescalateTimerLocked(time.Duration(ttlMinutes) * time.Minute)

	// Phase 8: 推送批准结果卡片
	notifyEscalationResult(m.remoteNotifier, req, ApprovalResultNotification{
		EscalationID:   req.ID,
		Approved:       true,
		RequestedLevel: req.RequestedLevel,
		TTLMinutes:     ttlMinutes,
	})

	return nil
}

// ---------- 自动降权 ----------

func (m *EscalationManager) startDeescalateTimerLocked(ttl time.Duration) {
	// 清除旧定时器
	if m.deescalateTimer != nil {
		m.deescalateTimer.Stop()
	}
	m.deescalateTimer = time.AfterFunc(ttl, func() {
		m.autoDeescalate("ttl_expired")
	})
}

// autoDeescalate TTL 到期时自动降权（从 timer callback 调用，需加锁）。
func (m *EscalationManager) autoDeescalate(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return
	}

	m.deescalateLocked(reason)
}

// deescalateLocked 执行降权操作（必须在持有 m.mu 时调用）。
// Fix 4: 提取为共享方法，供 TaskComplete、autoDeescalate、ManualRevoke 共用。
func (m *EscalationManager) deescalateLocked(reason string) {
	grant := m.active
	m.active = nil
	effectiveLevel := m.applyDeescalationFallbackLocked(grant.Level)

	if m.deescalateTimer != nil {
		m.deescalateTimer.Stop()
		m.deescalateTimer = nil
	}

	m.log.Info("escalation deescalated",
		"id", grant.ID,
		"level", grant.Level,
		"reason", reason,
	)

	if m.auditLogger != nil {
		eventType := AuditEventExpire
		if reason == "task_complete" {
			eventType = AuditEventTaskComplete
		} else if reason == "manual_revoke" {
			eventType = AuditEventManualRevoke
		}
		m.auditLogger.Log(EscalationAuditEntry{
			Timestamp:      time.Now(),
			Event:          eventType,
			RequestID:      grant.ID,
			RequestedLevel: grant.Level,
			RunID:          grant.RunID,
			SessionID:      grant.SessionID,
		})
	}

	if m.broadcaster != nil {
		m.broadcaster.Broadcast("exec.approval.resolved", map[string]interface{}{
			"id":       grant.ID,
			"approved": false,
			"level":    effectiveLevel,
			"reason":   reason,
		}, nil)
	}
}

// TaskComplete 任务完成时立即降权（如果 runID 匹配）。
// Fix 4: 避免 TOCTOU 竞态——在同一把锁内完成检查+降权。
func (m *EscalationManager) TaskComplete(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil || (runID != "" && m.active.RunID != runID) {
		return
	}

	m.deescalateLocked("task_complete")
}

// ManualRevoke 用户手动撤销活跃提权。
func (m *EscalationManager) ManualRevoke() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return
	}
	m.deescalateLocked("manual_revoke")
}

// ---------- 状态查询 ----------

// GetStatus 返回当前提权状态快照。
// Fix 5+18: 过期 grant 惰性清理，通过 deescalateLocked 确保广播+审计。
func (m *EscalationManager) GetStatus() EscalationStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	baseLevel := readBaseSecurityLevel()

	// 惰性清理过期 grant（通过 deescalateLocked 确保广播事件 + 审计日志）
	if m.active != nil && !time.Now().Before(m.active.ExpiresAt) {
		m.deescalateLocked("lazy_ttl_expired")
	}

	status := EscalationStatus{
		BaseLevel: baseLevel,
	}

	if m.pending != nil {
		status.HasPending = true
		status.Pending = m.pending
	}

	if m.active != nil {
		status.HasActive = true
		status.Active = m.active
		status.ActiveLevel = m.active.Level
	} else {
		status.ActiveLevel = baseLevel
	}

	return status
}

// GetEffectiveLevel 返回当前有效安全级别（活跃临时提权 > 持久化配置）。
// Fix 5+18: 过期 grant 惰性清理，通过 deescalateLocked 确保广播+审计。
func (m *EscalationManager) GetEffectiveLevel() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil {
		if time.Now().Before(m.active.ExpiresAt) {
			return m.active.Level
		}
		// 过期，惰性清理（通过 deescalateLocked 确保广播事件 + 审计日志）
		m.deescalateLocked("lazy_ttl_expired")
	}

	return readBaseSecurityLevel()
}

// GetActiveMountRequests 返回活跃 grant 的 MountRequests（Phase 3.4）。
// 已过期返回 nil（惰性清理）。
func (m *EscalationManager) GetActiveMountRequests() []MountRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return nil
	}
	// 惰性清理过期 grant
	if !time.Now().Before(m.active.ExpiresAt) {
		m.deescalateLocked("lazy_ttl_expired")
		return nil
	}
	return m.active.MountRequests
}

// GetPendingID 返回当前 pending 请求的 ID（用于 callback 验证）。
// Fix 9: 允许远程审批回调验证 escalation ID 是否匹配。
func (m *EscalationManager) GetPendingID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending != nil {
		return m.pending.ID
	}
	return ""
}

// Reset 清除所有内存状态（pending + active），停止定时器。用于运行时重置。
func (m *EscalationManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = nil
	m.active = nil
	// Phase 4.1: 清除磁盘持久化
	m.clearPersistedPending()
	if m.deescalateTimer != nil {
		m.deescalateTimer.Stop()
		m.deescalateTimer = nil
	}
	if m.approvalTimeout != nil {
		m.approvalTimeout.Stop()
		m.approvalTimeout = nil
	}
	m.log.Info("escalation manager reset")
}

// Close 关闭管理器，停止所有定时器。
func (m *EscalationManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deescalateTimer != nil {
		m.deescalateTimer.Stop()
		m.deescalateTimer = nil
	}
	if m.approvalTimeout != nil {
		m.approvalTimeout.Stop()
		m.approvalTimeout = nil
	}
}

// ---------- Phase 8: 审批超时 ----------

// startApprovalTimeoutLocked 启动审批超时定时器，到期自动拒绝。
func (m *EscalationManager) startApprovalTimeoutLocked(timeout time.Duration) {
	if m.approvalTimeout != nil {
		m.approvalTimeout.Stop()
	}
	m.approvalTimeout = time.AfterFunc(timeout, func() {
		m.log.Warn("审批超时，自动拒绝 / approval timed out, auto-denying",
			"timeout", timeout.String(),
		)
		if err := m.ResolveEscalation(false, 0); err != nil {
			// Fix 8: pending 可能已被用户手动处理，降级为 Debug 避免混淆
			m.log.Debug("审批超时自动拒绝已跳过（可能已被手动处理）", "error", err)
		}
	})
}

// stopApprovalTimeoutLocked 停止审批超时定时器（已持有锁时调用）。
func (m *EscalationManager) stopApprovalTimeoutLocked() {
	if m.approvalTimeout != nil {
		m.approvalTimeout.Stop()
		m.approvalTimeout = nil
	}
}

// ---------- Phase 4.1: 磁盘持久化 ----------

// persistPendingLocked 将当前 pending 请求持久化到磁盘。
// 必须在持有 m.mu 时调用。错误仅 warn 日志，不阻塞业务流程。
func (m *EscalationManager) persistPendingLocked() {
	if m.pending == nil {
		return
	}
	req := &infra.PersistedEscalationRequest{
		ID:             m.pending.ID,
		RequestedLevel: m.pending.RequestedLevel,
		Reason:         m.pending.Reason,
		RunID:          m.pending.RunID,
		SessionID:      m.pending.SessionID,
		RequestedAtMs:  m.pending.RequestedAt.UnixMilli(),
		TTLMinutes:     m.pending.TTLMinutes,
		MountRequests:  toPersistedMountRequests(m.pending.MountRequests),
	}
	if err := infra.SaveEscalationPending(req); err != nil {
		m.log.Warn("failed to persist escalation request to disk", "id", m.pending.ID, "error", err)
	}
}

// clearPersistedPending 从磁盘移除持久化的 pending 请求（best-effort）。
func (m *EscalationManager) clearPersistedPending() {
	if err := infra.ClearEscalationPending(); err != nil {
		m.log.Warn("failed to clear persisted escalation from disk", "error", err)
	}
}

// RestoreFromDisk 在 gateway 启动时从磁盘恢复未过期的 pending 审批请求。
// TTL 过期的请求不恢复（直接从磁盘清除）。
// 文件读写错误不阻塞启动（warn 日志即可）。
func (m *EscalationManager) RestoreFromDisk() {
	persisted := infra.ReadEscalationPending()
	if persisted == nil {
		return
	}

	requestedAt := time.UnixMilli(persisted.RequestedAtMs)
	// 使用审批超时（10 分钟）判断过期，而非 grant TTL。
	// TTLMinutes 是建议的授权时长（如 30 分钟），不是审批等待超时。
	const maxApprovalWait = 10 * time.Minute
	approvalDeadline := requestedAt.Add(maxApprovalWait)

	// 审批等待超时 → 丢弃并清理磁盘
	if time.Now().After(approvalDeadline) {
		m.log.Info("discarding expired persisted escalation (approval timeout)",
			"id", persisted.ID,
			"requestedAt", requestedAt.Format(time.RFC3339),
			"approvalDeadline", approvalDeadline.Format(time.RFC3339),
		)
		m.clearPersistedPending()
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 不覆盖已有内存状态
	if m.pending != nil || m.active != nil {
		return
	}

	m.pending = &PendingEscalationRequest{
		ID:             persisted.ID,
		RequestedLevel: persisted.RequestedLevel,
		Reason:         persisted.Reason,
		RunID:          persisted.RunID,
		SessionID:      persisted.SessionID,
		RequestedAt:    requestedAt,
		TTLMinutes:     persisted.TTLMinutes,
		MountRequests:  fromPersistedMountRequests(persisted.MountRequests),
	}

	// 用剩余审批时间重启定时器
	remaining := time.Until(approvalDeadline)
	if remaining <= 0 {
		remaining = time.Second // 极端边界: 刚好到审批截止时刻
	}
	m.startApprovalTimeoutLocked(remaining)

	m.log.Info("restored pending escalation from disk",
		"id", persisted.ID,
		"level", persisted.RequestedLevel,
		"remaining", remaining.String(),
	)

	// 广播给前端（如果有已连接的客户端）
	if m.broadcaster != nil {
		m.broadcaster.Broadcast("exec.approval.requested", map[string]interface{}{
			"id":             persisted.ID,
			"requestedLevel": persisted.RequestedLevel,
			"reason":         persisted.Reason,
			"runId":          persisted.RunID,
			"sessionId":      persisted.SessionID,
			"requestedAt":    persisted.RequestedAtMs,
			"ttlMinutes":     persisted.TTLMinutes,
			"mountRequests":  persisted.MountRequests,
			"restored":       true,
		}, nil)
	}
}

func toPersistedMountRequests(reqs []MountRequest) []infra.PersistedMountRequest {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]infra.PersistedMountRequest, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, infra.PersistedMountRequest{
			HostPath:  r.HostPath,
			MountMode: r.MountMode,
		})
	}
	return out
}

func fromPersistedMountRequests(reqs []infra.PersistedMountRequest) []MountRequest {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]MountRequest, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, MountRequest{
			HostPath:  r.HostPath,
			MountMode: r.MountMode,
		})
	}
	return sanitizeMountRequests(out)
}

// ---------- 内部辅助 ----------

// readBaseSecurityLevel 从 exec-approvals.json 读取持久化安全级别。
func readBaseSecurityLevel() string {
	snapshot := infra.ReadExecApprovalsSnapshot()
	if snapshot.File != nil && snapshot.File.Defaults != nil && snapshot.File.Defaults.Security != "" {
		return string(snapshot.File.Defaults.Security)
	}
	return string(infra.ExecSecurityDeny)
}

func persistBaseSecurityLevel(level infra.ExecSecurity) error {
	snapshot := infra.ReadExecApprovalsSnapshot()
	file := snapshot.File
	if file == nil {
		file = &infra.ExecApprovalsFile{Version: 1}
	}
	if file.Defaults == nil {
		file.Defaults = &infra.ExecApprovalsDefaults{}
	}
	file.Defaults.Security = level
	return infra.SaveExecApprovals(file)
}

func isPermanentEscalationLevel(level string) bool {
	return strings.EqualFold(strings.TrimSpace(level), string(infra.ExecSecurityFull))
}

// applyDeescalationFallbackLocked 按制度策略应用降权回落。
// 仅当从 L3(full) 降权且 escalationFallback=sandboxed 时，保证 base 至少为 L2。
func (m *EscalationManager) applyDeescalationFallbackLocked(grantLevel string) string {
	snapshot := infra.ReadExecApprovalsSnapshot()
	file := snapshot.File
	baseLevel := infra.ExecSecurityDeny
	if file != nil && file.Defaults != nil && file.Defaults.Security != "" {
		baseLevel = file.Defaults.Security
	}
	if grantLevel != string(infra.ExecSecurityFull) {
		return string(baseLevel)
	}

	fallback := infra.ExecEscalationFallbackBase
	if file != nil && file.Defaults != nil && file.Defaults.EscalationFallback != "" {
		fallback = file.Defaults.EscalationFallback
	}
	if fallback != infra.ExecEscalationFallbackSandboxed {
		return string(baseLevel)
	}
	if infra.LevelOrder(baseLevel) >= infra.LevelOrder(infra.ExecSecuritySandboxed) {
		return string(baseLevel)
	}

	// 需要将 base 提升到 L2，确保 L3 到期后固定回落到 sandboxed。
	if file == nil {
		file = &infra.ExecApprovalsFile{Version: 1}
	}
	if file.Defaults == nil {
		file.Defaults = &infra.ExecApprovalsDefaults{}
	}
	file.Defaults.Security = infra.ExecSecuritySandboxed
	if err := infra.SaveExecApprovals(file); err != nil {
		m.log.Warn("failed to persist sandboxed fallback during deescalation", "error", err)
		return string(baseLevel)
	}
	return string(infra.ExecSecuritySandboxed)
}
