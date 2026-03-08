package infra

// approval_forwarder.go — Exec Approval 转发器
// 对应 TS: src/infra/exec-approval-forwarder.ts (352L)
//
// 管理执行审批请求的转发和超时处理。

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---------- 类型定义 ----------

// ApprovalForwardMode 转发模式。
type ApprovalForwardMode string

const (
	ApprovalForwardModeSession  ApprovalForwardMode = "session"
	ApprovalForwardModeTargets  ApprovalForwardMode = "targets"
	ApprovalForwardModeBoth     ApprovalForwardMode = "both"
	ApprovalForwardModeExplicit ApprovalForwardMode = "explicit"
	ApprovalForwardModeOff      ApprovalForwardMode = "off"
)

// ExecApprovalRequestDetail 审批请求详情。
type ExecApprovalRequestDetail struct {
	Command      string `json:"command"`
	Cwd          string `json:"cwd,omitempty"`
	Host         string `json:"host,omitempty"`
	Security     string `json:"security,omitempty"`
	Ask          string `json:"ask,omitempty"`
	AgentID      string `json:"agentId,omitempty"`
	ResolvedPath string `json:"resolvedPath,omitempty"`
	SessionKey   string `json:"sessionKey,omitempty"`
}

// ExecApprovalRequest 执行审批请求（完整字段）。
type ExecApprovalRequest struct {
	ID          string                    `json:"id"`
	Request     ExecApprovalRequestDetail `json:"request"`
	CreatedAtMs int64                     `json:"createdAtMs"`
	ExpiresAtMs int64                     `json:"expiresAtMs"`
	// 兼容旧字段
	Command    string `json:"command,omitempty"`
	AgentID    string `json:"agentId,omitempty"`
	SessionKey string `json:"sessionKey,omitempty"`
	Host       string `json:"host,omitempty"`
	Cwd        string `json:"cwd,omitempty"`
	Reason     string `json:"reason,omitempty"`
	CreatedAt  int64  `json:"createdAt,omitempty"`
}

// ExecApprovalResolved 执行审批结果。
type ExecApprovalResolved struct {
	ID         string `json:"id"`
	Decision   string `json:"decision"` // "allow-once", "allow-always", "deny"
	ResolvedBy string `json:"resolvedBy,omitempty"`
	Ts         int64  `json:"ts,omitempty"`
	Command    string `json:"command,omitempty"`
}

// ExecApprovalForwardTarget 转发目标。
type ExecApprovalForwardTarget struct {
	Channel   string `json:"channel"`
	To        string `json:"to"`
	AccountID string `json:"accountId,omitempty"`
	ThreadID  string `json:"threadId,omitempty"`
	Source    string `json:"source,omitempty"` // "session" | "target"
}

// ApprovalForwarderConfig 转发器配置。
type ApprovalForwarderConfig struct {
	Mode          ApprovalForwardMode
	TimeoutMs     int64
	Enabled       bool
	AgentFilter   []string
	SessionFilter []string
	Targets       []ExecApprovalForwardTarget
	OnRequest     func(req ExecApprovalRequest)
	OnResolved    func(res ExecApprovalResolved)
	DeliverFunc   func(target ExecApprovalForwardTarget, text string) error
	ResolveTarget func(req ExecApprovalRequest) *ExecApprovalForwardTarget
	NowMs         func() int64
}

// pendingApproval 待处理的审批。
type pendingApproval struct {
	request   ExecApprovalRequest
	targets   []ExecApprovalForwardTarget
	timer     *time.Timer
	createdAt time.Time
}

// ---------- ApprovalForwarder ----------

// ApprovalForwarder 管理审批请求转发。
type ApprovalForwarder struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval
	cfg     ApprovalForwarderConfig
	stopped bool
}

const defaultApprovalForwarderTimeout = 120_000 // 2 分钟

// NewApprovalForwarder 创建审批转发器。
func NewApprovalForwarder(cfg ApprovalForwarderConfig) *ApprovalForwarder {
	if cfg.TimeoutMs <= 0 {
		cfg.TimeoutMs = defaultApprovalForwarderTimeout
	}
	if cfg.NowMs == nil {
		cfg.NowMs = func() int64 { return time.Now().UnixMilli() }
	}
	return &ApprovalForwarder{
		pending: make(map[string]*pendingApproval),
		cfg:     cfg,
	}
}

// ---------- 消息构建 (TS L118-178) ----------

// FormatApprovalCommand 格式化命令显示。
func FormatApprovalCommand(command string) string {
	if !strings.Contains(command, "\n") && !strings.Contains(command, "`") {
		return "`" + command + "`"
	}
	fence := "```"
	for strings.Contains(command, fence) {
		fence += "`"
	}
	return fence + "\n" + command + "\n" + fence
}

// BuildRequestMessage 构建审批请求消息。
func BuildRequestMessage(req ExecApprovalRequest, nowMs int64) string {
	lines := []string{"🔒 Exec approval required", "ID: " + req.ID}
	cmd := FormatApprovalCommand(req.Request.Command)
	if !strings.Contains(cmd, "\n") {
		lines = append(lines, "Command: "+cmd)
	} else {
		lines = append(lines, "Command:")
		lines = append(lines, cmd)
	}
	if req.Request.Cwd != "" {
		lines = append(lines, "CWD: "+req.Request.Cwd)
	}
	if req.Request.Host != "" {
		lines = append(lines, "Host: "+req.Request.Host)
	}
	if req.Request.AgentID != "" {
		lines = append(lines, "Agent: "+req.Request.AgentID)
	}
	if req.Request.Security != "" {
		lines = append(lines, "Security: "+req.Request.Security)
	}
	if req.Request.Ask != "" {
		lines = append(lines, "Ask: "+req.Request.Ask)
	}
	expiresIn := (req.ExpiresAtMs - nowMs) / 1000
	if expiresIn < 0 {
		expiresIn = 0
	}
	lines = append(lines, "Expires in: "+intToStr(expiresIn)+"s")
	lines = append(lines, "Reply with: /approve <id> allow-once|allow-always|deny")
	return strings.Join(lines, "\n")
}

// intToStr 简单 int64 → string
func intToStr(v int64) string {
	if v == 0 {
		return "0"
	}
	s := ""
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// DecisionLabel 人读决策标签。
func DecisionLabel(decision string) string {
	switch decision {
	case "allow-once":
		return "allowed once"
	case "allow-always":
		return "allowed always"
	default:
		return "denied"
	}
}

// BuildResolvedMessage 构建审批结果消息。
func BuildResolvedMessage(res ExecApprovalResolved) string {
	base := "✅ Exec approval " + DecisionLabel(res.Decision) + "."
	if res.ResolvedBy != "" {
		base += " Resolved by " + res.ResolvedBy + "."
	}
	return base + " ID: " + res.ID
}

// BuildExpiredMessage 构建审批超时消息。
func BuildExpiredMessage(req ExecApprovalRequest) string {
	return "⏱️ Exec approval expired. ID: " + req.ID
}

// ---------- 过滤逻辑 (TS L70-109) ----------

// MatchSessionFilter 检查 sessionKey 是否匹配过滤模式。
func MatchSessionFilter(sessionKey string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(sessionKey, pattern) {
			return true
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(sessionKey) {
			return true
		}
	}
	return false
}

// ShouldForwardApproval 判断是否应转发审批。
func ShouldForwardApproval(cfg *ApprovalForwarderConfig, req ExecApprovalRequest) bool {
	if cfg == nil || !cfg.Enabled {
		return false
	}
	if len(cfg.AgentFilter) > 0 {
		agentID := req.Request.AgentID
		if agentID == "" {
			agentID = req.AgentID
		}
		if agentID == "" {
			return false
		}
		found := false
		for _, f := range cfg.AgentFilter {
			if f == agentID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(cfg.SessionFilter) > 0 {
		sk := req.Request.SessionKey
		if sk == "" {
			sk = req.SessionKey
		}
		if sk == "" {
			return false
		}
		if !MatchSessionFilter(sk, cfg.SessionFilter) {
			return false
		}
	}
	return true
}
