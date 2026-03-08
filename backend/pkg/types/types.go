// Package types 定义 Crab Claw（蟹爪）后端的共享类型。
//
// 对应原版 src/config/types.*.ts 中的核心类型定义。
// 此文件定义基础类型，各子域类型将在后续 Phase 中逐步添加。
package types

import "time"

// ============================================================
// 频道相关类型 — 继承自 src/channels/chat-type.ts
// ============================================================

// ChatType 聊天类型
// 原版: export type ChatType = "direct" | "group" | "channel"
type ChatType string

const (
	ChatDirect  ChatType = "direct"
	ChatGroup   ChatType = "group"
	ChatChannel ChatType = "channel"
)

// NormalizeChatType 规范化聊天类型字符串
// 继承自原版 normalizeChatType() — 将 "dm" 映射为 "direct"
func NormalizeChatType(raw string) (ChatType, bool) {
	switch raw {
	case "direct", "dm":
		return ChatDirect, true
	case "group":
		return ChatGroup, true
	case "channel":
		return ChatChannel, true
	default:
		return "", false
	}
}

// ChannelType 频道类型标识
type ChannelType string

const (
	ChannelDiscord  ChannelType = "discord"
	ChannelSlack    ChannelType = "slack"
	ChannelTelegram ChannelType = "telegram"
	ChannelWhatsApp ChannelType = "whatsapp"
	ChannelSignal   ChannelType = "signal"
	ChannelIMessage ChannelType = "imessage"
	ChannelLine     ChannelType = "line"
	ChannelWeb      ChannelType = "web"
	ChannelFeishu   ChannelType = "feishu"
	ChannelDingTalk ChannelType = "dingtalk"
	ChannelWeCom    ChannelType = "wecom"
)

// ============================================================
// 基础配置类型 — 继承自 src/config/types.base.ts
// ============================================================

// ReplyMode 回复模式
type ReplyMode string

const (
	ReplyText    ReplyMode = "text"
	ReplyCommand ReplyMode = "command"
)

// TypingMode 打字指示器模式
type TypingMode string

const (
	TypingNever    TypingMode = "never"
	TypingInstant  TypingMode = "instant"
	TypingThinking TypingMode = "thinking"
	TypingMessage  TypingMode = "message"
)

// SessionScope 会话范围
type SessionScope string

const (
	SessionScopePerSender SessionScope = "per-sender"
	SessionScopeGlobal    SessionScope = "global"
)

// DmScope DM 会话范围
type DmScope string

const (
	DmScopeMain               DmScope = "main"
	DmScopePerPeer            DmScope = "per-peer"
	DmScopePerChannelPeer     DmScope = "per-channel-peer"
	DmScopePerAccountChanPeer DmScope = "per-account-channel-peer"
)

// ReplyToMode 回复引用模式
type ReplyToMode string

const (
	ReplyToOff   ReplyToMode = "off"
	ReplyToFirst ReplyToMode = "first"
	ReplyToAll   ReplyToMode = "all"
)

// GroupPolicy 群聊策略
type GroupPolicy string

const (
	GroupOpen        GroupPolicy = "open"
	GroupDisabled    GroupPolicy = "disabled"
	GroupAllowlist   GroupPolicy = "allowlist"
	GroupAlways      GroupPolicy = "always"
	GroupSilentToken GroupPolicy = "silent_token"
	GroupMentionOnly GroupPolicy = "mention_only"
)

// DmPolicy 私聊策略
type DmPolicy string

const (
	DmPairing   DmPolicy = "pairing"
	DmAllowlist DmPolicy = "allowlist"
	DmOpen      DmPolicy = "open"
	DmDisabled  DmPolicy = "disabled"
)

// OutboundRetryConfig 出站请求重试配置
// 原版: export type OutboundRetryConfig
type OutboundRetryConfig struct {
	Attempts   int     `json:"attempts,omitempty"`   // 最大重试次数（默认3）
	MinDelayMs int     `json:"minDelayMs,omitempty"` // 最小延迟(ms)
	MaxDelayMs int     `json:"maxDelayMs,omitempty"` // 最大延迟上限(ms)
	Jitter     float64 `json:"jitter,omitempty"`     // 抖动因子(0-1)
}

// BlockStreamingCoalesceConfig 流式合并配置
type BlockStreamingCoalesceConfig struct {
	MinChars int `json:"minChars,omitempty"`
	MaxChars int `json:"maxChars,omitempty"`
	IdleMs   int `json:"idleMs,omitempty"`
}

// BlockStreamingChunkConfig 流式分块配置
type BlockStreamingChunkConfig struct {
	MinChars        int    `json:"minChars,omitempty"`
	MaxChars        int    `json:"maxChars,omitempty"`
	BreakPreference string `json:"breakPreference,omitempty"` // "paragraph"|"newline"|"sentence"
}

// MarkdownTableMode Markdown 表格渲染模式
type MarkdownTableMode string

const (
	MarkdownTableOff     MarkdownTableMode = "off"
	MarkdownTableBullets MarkdownTableMode = "bullets"
	MarkdownTableCode    MarkdownTableMode = "code"
)

// MarkdownConfig Markdown 配置
type MarkdownConfig struct {
	Tables MarkdownTableMode `json:"tables,omitempty"`
}

// HumanDelayConfig 仿人延迟配置
type HumanDelayConfig struct {
	Mode  string `json:"mode,omitempty"`  // "off"|"natural"|"custom"
	MinMs int    `json:"minMs,omitempty"` // 最小延迟(ms)，默认800
	MaxMs int    `json:"maxMs,omitempty"` // 最大延迟(ms)，默认2500
}

// SessionSendPolicyAction 会话发送策略动作
type SessionSendPolicyAction string

const (
	SendPolicyAllow SessionSendPolicyAction = "allow"
	SendPolicyDeny  SessionSendPolicyAction = "deny"
)

// SessionSendPolicyMatch 会话发送策略匹配条件
type SessionSendPolicyMatch struct {
	Channel   string   `json:"channel,omitempty"`
	ChatType  ChatType `json:"chatType,omitempty"`
	KeyPrefix string   `json:"keyPrefix,omitempty"`
}

// SessionSendPolicyRule 会话发送策略规则
type SessionSendPolicyRule struct {
	Action SessionSendPolicyAction `json:"action"`
	Match  *SessionSendPolicyMatch `json:"match,omitempty"`
}

// SessionSendPolicyConfig 会话发送策略配置
type SessionSendPolicyConfig struct {
	Default SessionSendPolicyAction `json:"default,omitempty"`
	Rules   []SessionSendPolicyRule `json:"rules,omitempty"`
}

// SessionResetMode 会话重置模式
type SessionResetMode string

const (
	SessionResetDaily SessionResetMode = "daily"
	SessionResetIdle  SessionResetMode = "idle"
)

// SessionResetConfig 会话重置配置
type SessionResetConfig struct {
	Mode        SessionResetMode `json:"mode,omitempty"`
	AtHour      *int             `json:"atHour,omitempty"`      // 每日重置的本地小时(0-23)
	IdleMinutes *int             `json:"idleMinutes,omitempty"` // 空闲窗口(分钟)
}

// SessionResetByTypeConfig 按聊天类型的会话重置配置
type SessionResetByTypeConfig struct {
	Direct *SessionResetConfig `json:"direct,omitempty"`
	DM     *SessionResetConfig `json:"dm,omitempty"` // @deprecated 使用 Direct 代替
	Group  *SessionResetConfig `json:"group,omitempty"`
	Thread *SessionResetConfig `json:"thread,omitempty"`
}

// SessionConfig 会话总配置
// 原版: export type SessionConfig
type SessionConfig struct {
	Scope                 SessionScope                   `json:"scope,omitempty"`
	DmScope               DmScope                        `json:"dmScope,omitempty"`
	IdentityLinks         map[string][]string            `json:"identityLinks,omitempty"`
	ResetTriggers         []string                       `json:"resetTriggers,omitempty"`
	IdleMinutes           *int                           `json:"idleMinutes,omitempty"`
	Reset                 *SessionResetConfig            `json:"reset,omitempty"`
	ResetByType           *SessionResetByTypeConfig      `json:"resetByType,omitempty"`
	ResetByChannel        map[string]*SessionResetConfig `json:"resetByChannel,omitempty"`
	Store                 string                         `json:"store,omitempty"`
	TypingIntervalSeconds *int                           `json:"typingIntervalSeconds,omitempty"`
	TypingMode            TypingMode                     `json:"typingMode,omitempty"`
	MainKey               string                         `json:"mainKey,omitempty"`
	SendPolicy            *SessionSendPolicyConfig       `json:"sendPolicy,omitempty"`
	AgentToAgent          *AgentToAgentConfig            `json:"agentToAgent,omitempty"`
}

// AgentToAgentConfig Agent 间通信配置
type AgentToAgentConfig struct {
	MaxPingPongTurns *int `json:"maxPingPongTurns,omitempty"` // 最大乒乓轮次(0-5)，默认5
}

// LogLevel 日志级别
type LogLevel string

const (
	LogSilent LogLevel = "silent"
	LogFatal  LogLevel = "fatal"
	LogError  LogLevel = "error"
	LogWarn   LogLevel = "warn"
	LogInfo   LogLevel = "info"
	LogDebug  LogLevel = "debug"
	LogTrace  LogLevel = "trace"
)

// ConsoleStyle 控制台输出风格
type ConsoleStyle string

const (
	ConsolePretty  ConsoleStyle = "pretty"
	ConsoleCompact ConsoleStyle = "compact"
	ConsoleJSON    ConsoleStyle = "json"
)

// LoggingConfig 日志配置
// 原版: export type LoggingConfig
type LoggingConfig struct {
	Level           LogLevel     `json:"level,omitempty"`
	File            string       `json:"file,omitempty"`
	ConsoleLevel    LogLevel     `json:"consoleLevel,omitempty"`
	ConsoleStyle    ConsoleStyle `json:"consoleStyle,omitempty"`
	RedactSensitive string       `json:"redactSensitive,omitempty"` // "off"|"tools"
	RedactPatterns  []string     `json:"redactPatterns,omitempty"`
}

// DiagnosticsOtelConfig OpenTelemetry 诊断配置
type DiagnosticsOtelConfig struct {
	Enabled         *bool             `json:"enabled,omitempty"`
	Endpoint        string            `json:"endpoint,omitempty"`
	Protocol        string            `json:"protocol,omitempty"` // "http/protobuf"|"grpc"
	Headers         map[string]string `json:"headers,omitempty"`
	ServiceName     string            `json:"serviceName,omitempty"`
	Traces          *bool             `json:"traces,omitempty"`
	Metrics         *bool             `json:"metrics,omitempty"`
	Logs            *bool             `json:"logs,omitempty"`
	SampleRate      *float64          `json:"sampleRate,omitempty"`      // 采样率(0.0-1.0)
	FlushIntervalMs *int              `json:"flushIntervalMs,omitempty"` // 指标导出间隔(ms)
}

// DiagnosticsCacheTraceConfig 缓存追踪配置
type DiagnosticsCacheTraceConfig struct {
	Enabled         *bool  `json:"enabled,omitempty"`
	FilePath        string `json:"filePath,omitempty"`
	IncludeMessages *bool  `json:"includeMessages,omitempty"`
	IncludePrompt   *bool  `json:"includePrompt,omitempty"`
	IncludeSystem   *bool  `json:"includeSystem,omitempty"`
}

// DiagnosticsConfig 诊断总配置
type DiagnosticsConfig struct {
	Enabled    *bool                        `json:"enabled,omitempty"`
	Flags      []string                     `json:"flags,omitempty"`
	Otel       *DiagnosticsOtelConfig       `json:"otel,omitempty"`
	CacheTrace *DiagnosticsCacheTraceConfig `json:"cacheTrace,omitempty"`
}

// WebReconnectConfig WhatsApp Web 重连配置
type WebReconnectConfig struct {
	InitialMs   *int     `json:"initialMs,omitempty"`
	MaxMs       *int     `json:"maxMs,omitempty"`
	Factor      *float64 `json:"factor,omitempty"`
	Jitter      *float64 `json:"jitter,omitempty"`
	MaxAttempts *int     `json:"maxAttempts,omitempty"` // 0=无限制
}

// WebConfig WhatsApp Web 配置
type WebConfig struct {
	Enabled          *bool               `json:"enabled,omitempty"`
	HeartbeatSeconds *int                `json:"heartbeatSeconds,omitempty"`
	Reconnect        *WebReconnectConfig `json:"reconnect,omitempty"`
}

// AgentElevatedAllowFromConfig Agent 提权允许来源配置
// map[providerId][]string_or_number
type AgentElevatedAllowFromConfig map[string][]interface{}

// IdentityConfig 身份配置
type IdentityConfig struct {
	Name   string `json:"name,omitempty"`
	Theme  string `json:"theme,omitempty"`
	Emoji  string `json:"emoji,omitempty"`
	Avatar string `json:"avatar,omitempty"` // 工作区相对路径、HTTP(S) URL 或 data URI
}

// ============================================================
// 消息相关类型
// ============================================================

// MessageRole 消息角色
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// Message 通用消息结构
type Message struct {
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Name      string      `json:"name,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ============================================================
// 会话标识类型
// ============================================================

// SessionKey 会话标识
type SessionKey struct {
	AgentID   string `json:"agentId"`
	ChannelID string `json:"channelId"`
	ThreadID  string `json:"threadId,omitempty"`
	UserID    string `json:"userId,omitempty"`
}

// SessionState 会话状态
type SessionState string

const (
	SessionIdle    SessionState = "idle"
	SessionActive  SessionState = "active"
	SessionPending SessionState = "pending"
)

// AgentID Agent 标识
type AgentID = string

// ============================================================
// 网关相关类型
// ============================================================

// GatewayState 网关运行状态
type GatewayState string

const (
	GatewayStarting GatewayState = "starting"
	GatewayReady    GatewayState = "ready"
	GatewayStopping GatewayState = "stopping"
	GatewayStopped  GatewayState = "stopped"
)

// HealthStatus 健康检查状态
type HealthStatus struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Uptime    float64   `json:"uptime"`
	Timestamp time.Time `json:"timestamp"`
}
