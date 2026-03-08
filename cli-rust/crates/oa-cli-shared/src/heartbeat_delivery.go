package infra

// heartbeat_delivery.go — Heartbeat 单次执行逻辑（全量）
// 对应 TS: heartbeat-runner.ts runHeartbeatOnce (L490-860)
//
// FIX-6: 扩展 deps + channel adapter + session resolve + reply parsing

import (
	"strings"
	"time"
)

// ---------- 类型定义 ----------

// HeartbeatDeliveryResult 单次 heartbeat 执行结果。
type HeartbeatDeliveryResult struct {
	Status    HeartbeatStatus `json:"status"`
	Channel   string          `json:"channel,omitempty"`
	To        string          `json:"to,omitempty"`
	Preview   string          `json:"preview,omitempty"`
	ErrorMsg  string          `json:"error,omitempty"`
	UpdatedAt int64           `json:"updatedAt,omitempty"`
}

// HeartbeatDeliveryDeps 投递依赖（接口注入避免循环引用）。
type HeartbeatDeliveryDeps struct {
	SendMessage      func(channel, to, text string) error
	GetSession       func(sessionKey string) (string, error)
	FormatForChannel func(channel, text string) string
	RestoreUpdatedAt func(sessionKey string, ts int64)
	NowMs            func() int64
}

// HeartbeatAgentConfig 每个 agent 的心跳配置。
type HeartbeatAgentConfig struct {
	Enabled    bool   `json:"enabled"`
	IntervalMs int64  `json:"intervalMs,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Channel    string `json:"channel,omitempty"`
	To         string `json:"to,omitempty"`
	SessionKey string `json:"sessionKey,omitempty"`
	AccountID  string `json:"accountId,omitempty"`
}

// HeartbeatReplyPayload 心跳回复内容。
type HeartbeatReplyPayload struct {
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ---------- 配置解析 ----------

// DefaultHeartbeatIntervalMs 默认心跳间隔。
const DefaultHeartbeatIntervalMs = 3_600_000 // 1 小时

// ResolveHeartbeatIntervalMs 解析心跳间隔。
func ResolveHeartbeatIntervalMs(cfg *HeartbeatAgentConfig) int64 {
	if cfg != nil && cfg.IntervalMs > 0 {
		return cfg.IntervalMs
	}
	return DefaultHeartbeatIntervalMs
}

// IsHeartbeatEnabledForAgent 判断 agent 是否启用心跳。
func IsHeartbeatEnabledForAgent(cfg *HeartbeatAgentConfig) bool {
	return cfg != nil && cfg.Enabled
}

// ResolveHeartbeatPrompt 解析心跳 prompt。
func ResolveHeartbeatPrompt(cfg *HeartbeatAgentConfig) string {
	if cfg != nil && cfg.Prompt != "" {
		return cfg.Prompt
	}
	return "heartbeat check"
}

// ResolveHeartbeatSummaryForAgent 解析心跳摘要配置（TS L250-300）。
func ResolveHeartbeatSummaryForAgent(cfg *HeartbeatAgentConfig) map[string]string {
	if cfg == nil {
		return nil
	}
	summary := make(map[string]string)
	if cfg.Channel != "" {
		summary["channel"] = cfg.Channel
	}
	if cfg.To != "" {
		summary["to"] = cfg.To
	}
	if cfg.SessionKey != "" {
		summary["sessionKey"] = cfg.SessionKey
	}
	summary["intervalMs"] = intToStr(ResolveHeartbeatIntervalMs(cfg))
	return summary
}

// ---------- Reply 处理 ----------

// NormalizeHeartbeatReply 标准化心跳回复文本（TS L830-860）。
func NormalizeHeartbeatReply(text string) string {
	text = strings.TrimSpace(text)
	// 移除 thinking tokens
	text = stripThinkingTokens(text)
	text = strings.TrimSpace(text)
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return text
}

// stripThinkingTokens 移除 <thinking>...</thinking> 标签。
func stripThinkingTokens(text string) string {
	for {
		startIdx := strings.Index(text, "<thinking>")
		if startIdx == -1 {
			break
		}
		endIdx := strings.Index(text[startIdx:], "</thinking>")
		if endIdx == -1 {
			text = text[:startIdx]
			break
		}
		text = text[:startIdx] + text[startIdx+endIdx+len("</thinking>"):]
	}
	return text
}

// ResolveHeartbeatReplyPayload 从回复数组提取文本（TS L800-830）。
func ResolveHeartbeatReplyPayload(replies []string) HeartbeatReplyPayload {
	if len(replies) == 0 {
		return HeartbeatReplyPayload{}
	}
	combined := strings.Join(replies, "\n")
	text := NormalizeHeartbeatReply(combined)
	truncated := len(combined) > 200
	return HeartbeatReplyPayload{
		Text:      text,
		Truncated: truncated,
	}
}

// defaultNowMs 默认时间函数。
func defaultNowMs() int64 {
	return time.Now().UnixMilli()
}
