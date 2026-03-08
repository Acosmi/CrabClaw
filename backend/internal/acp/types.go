package acp

import (
	"context"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- ACP 协议版本 ----------

// ACPProtocolVersion ACP 协议版本号（对齐 @agentclientprotocol/sdk PROTOCOL_VERSION）。
const ACPProtocolVersion = "2025-03-26"

// ---------- ACP Agent 信息 ----------

// ACPAgentInfo ACP Agent 元信息（对应 TS ACP_AGENT_INFO）。
var ACPAgentInfo = AgentInfo{
	Name:    "openacosmi-acp",
	Title:   "Crab Claw ACP Gateway",
	Version: config.BuildVersion,
}

// ---------- ACP 会话类型 ----------

// AcpSession ACP 会话实例（对应 TS AcpSession）。
type AcpSession struct {
	SessionID   string
	SessionKey  string
	Cwd         string
	CreatedAt   int64
	ActiveRunID string
	// CancelFunc 用于取消活跃运行（替代 TS AbortController）。
	CancelFunc context.CancelFunc
}

// AcpServerOptions ACP 服务端选项（对应 TS AcpServerOptions）。
type AcpServerOptions struct {
	GatewayURL             string
	GatewayToken           string
	GatewayPassword        string
	DefaultSessionKey      string
	DefaultSessionLabel    string
	RequireExistingSession bool
	ResetSession           bool
	PrefixCwd              *bool // nil = 未设置（默认 true）
	Verbose                bool
}

// PrefixCwdEnabled 返回 PrefixCwd 的有效值，默认 true。
func (o *AcpServerOptions) PrefixCwdEnabled() bool {
	if o.PrefixCwd == nil {
		return true
	}
	return *o.PrefixCwd
}

// ---------- SDK 等价类型：Content Blocks ----------

// ContentBlock ACP 内容块（对应 @agentclientprotocol/sdk ContentBlock）。
type ContentBlock struct {
	Type string `json:"type"` // "text" | "image" | "resource" | "resource_link"

	// text 块字段
	Text string `json:"text,omitempty"`

	// image 块字段
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`

	// resource 块字段
	Resource *ResourceContent `json:"resource,omitempty"`

	// resource_link 块字段
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

// ResourceContent ACP 资源内容。
type ResourceContent struct {
	Text string `json:"text,omitempty"`
}

// ---------- SDK 等价类型：Tool ----------

// ToolKind 工具类型分类（对应 @agentclientprotocol/sdk ToolKind）。
type ToolKind = string

const (
	ToolKindRead    ToolKind = "read"
	ToolKindEdit    ToolKind = "edit"
	ToolKindDelete  ToolKind = "delete"
	ToolKindMove    ToolKind = "move"
	ToolKindSearch  ToolKind = "search"
	ToolKindExecute ToolKind = "execute"
	ToolKindFetch   ToolKind = "fetch"
	ToolKindOther   ToolKind = "other"
)

// ---------- SDK 等价类型：Stop Reason ----------

// StopReason 停止原因（对应 @agentclientprotocol/sdk StopReason）。
type StopReason = string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonCancelled StopReason = "cancelled"
	StopReasonRefusal   StopReason = "refusal"
)

// ---------- SDK 等价类型：Protocol Messages ----------

// AgentInfo Agent 信息（对应 SDK AgentInfo）。
type AgentInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// AgentCapabilities Agent 能力声明（对应 SDK AgentCapabilities）。
type AgentCapabilities struct {
	LoadSession         bool                 `json:"loadSession,omitempty"`
	PromptCapabilities  *PromptCapabilities  `json:"promptCapabilities,omitempty"`
	MCPCapabilities     *MCPCapabilities     `json:"mcpCapabilities,omitempty"`
	SessionCapabilities *SessionCapabilities `json:"sessionCapabilities,omitempty"`
}

// PromptCapabilities 提示词能力。
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// MCPCapabilities MCP 能力。
type MCPCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// SessionCapabilities 会话能力。
type SessionCapabilities struct {
	List interface{} `json:"list,omitempty"` // {} 表示支持
}

// ClientCapabilities 客户端能力（对应 SDK ClientCapabilities）。
type ClientCapabilities struct {
	FS       *FSCapabilities `json:"fs,omitempty"`
	Terminal bool            `json:"terminal,omitempty"`
}

// FSCapabilities 文件系统能力。
type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

// ClientInfo 客户端信息。
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ---------- SDK 等价类型：Request/Response ----------

// InitializeRequest 初始化请求。
type InitializeRequest struct {
	ProtocolVersion    string              `json:"protocolVersion"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitempty"`
	ClientInfo         *ClientInfo         `json:"clientInfo,omitempty"`
}

// InitializeResponse 初始化响应。
type InitializeResponse struct {
	ProtocolVersion   string             `json:"protocolVersion"`
	AgentCapabilities *AgentCapabilities `json:"agentCapabilities,omitempty"`
	AgentInfo         *AgentInfo         `json:"agentInfo,omitempty"`
	AuthMethods       []interface{}      `json:"authMethods"`
}

// MCPServerConfig MCP 服务器配置。
type MCPServerConfig struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	URL       string `json:"url,omitempty"`
}

// NewSessionRequest 创建会话请求。
type NewSessionRequest struct {
	Cwd        string                 `json:"cwd"`
	MCPServers []MCPServerConfig      `json:"mcpServers"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

// NewSessionResponse 创建会话响应。
type NewSessionResponse struct {
	SessionID string `json:"sessionId"`
}

// LoadSessionRequest 加载会话请求。
type LoadSessionRequest struct {
	SessionID  string                 `json:"sessionId"`
	Cwd        string                 `json:"cwd"`
	MCPServers []MCPServerConfig      `json:"mcpServers"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

// LoadSessionResponse 加载会话响应（空）。
type LoadSessionResponse struct{}

// ListSessionsRequest 列出会话请求。
type ListSessionsRequest struct {
	Cwd  string                 `json:"cwd,omitempty"`
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

// ListSessionEntry 会话列表条目。
type ListSessionEntry struct {
	SessionID string                 `json:"sessionId"`
	Cwd       string                 `json:"cwd"`
	Title     string                 `json:"title,omitempty"`
	UpdatedAt string                 `json:"updatedAt,omitempty"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

// ListSessionsResponse 列出会话响应。
type ListSessionsResponse struct {
	Sessions   []ListSessionEntry `json:"sessions"`
	NextCursor interface{}        `json:"nextCursor"` // null
}

// PromptRequest 提示词请求。
type PromptRequest struct {
	SessionID string                 `json:"sessionId"`
	Prompt    []ContentBlock         `json:"prompt"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

// PromptResponse 提示词响应。
type PromptResponse struct {
	StopReason StopReason `json:"stopReason"`
}

// AuthenticateRequest 认证请求（预留）。
type AuthenticateRequest struct{}

// AuthenticateResponse 认证响应（预留）。
type AuthenticateResponse struct{}

// SetSessionModeRequest 设置会话模式请求。
type SetSessionModeRequest struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId,omitempty"`
}

// SetSessionModeResponse 设置会话模式响应（空）。
type SetSessionModeResponse struct{}

// CancelNotification 取消通知。
type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// ---------- SDK 等价类型：Session Updates ----------

// SessionUpdate 会话更新事件。
type SessionUpdate struct {
	SessionUpdate string `json:"sessionUpdate"` // "agent_message_chunk" | "tool_call" | "tool_call_update" | "available_commands_update"

	// agent_message_chunk 字段
	Content *ContentBlock `json:"content,omitempty"`

	// tool_call 字段
	ToolCallID string                 `json:"toolCallId,omitempty"`
	ToolTitle  string                 `json:"title,omitempty"`
	Status     string                 `json:"status,omitempty"`
	RawInput   map[string]interface{} `json:"rawInput,omitempty"`
	Kind       ToolKind               `json:"kind,omitempty"`
	RawOutput  interface{}            `json:"rawOutput,omitempty"`

	// available_commands_update 字段
	AvailableCommands []AvailableCommand `json:"availableCommands,omitempty"`
}

// SessionNotification 会话通知。
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// AvailableCommand 可用命令（对应 SDK AvailableCommand）。
type AvailableCommand struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Input       *CommandInput `json:"input,omitempty"`
}

// CommandInput 命令输入提示。
type CommandInput struct {
	Hint string `json:"hint,omitempty"`
}

// ---------- SDK 等价类型：Permission ----------

// PermissionOption 权限选项。
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"` // "allow_once" | "deny" | ...
}

// RequestPermissionRequest 权限请求。
type RequestPermissionRequest struct {
	ToolCall *ToolCallInfo      `json:"toolCall,omitempty"`
	Options  []PermissionOption `json:"options,omitempty"`
}

// ToolCallInfo 工具调用信息。
type ToolCallInfo struct {
	Title string `json:"title,omitempty"`
}

// RequestPermissionResponse 权限响应。
type RequestPermissionResponse struct {
	Outcome *PermissionOutcome `json:"outcome,omitempty"`
}

// PermissionOutcome 权限决策结果。
type PermissionOutcome struct {
	Outcome  string `json:"outcome"` // "selected"
	OptionID string `json:"optionId"`
}

// ---------- ndJSON RPC 帧格式 ----------

// NDJSONMessage ndJSON 协议消息。
type NDJSONMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`     // 请求/响应 ID
	Method  string      `json:"method,omitempty"` // 方法名
	Params  interface{} `json:"params,omitempty"` // 参数
	Result  interface{} `json:"result,omitempty"` // 成功结果
	Error   *RPCError   `json:"error,omitempty"`  // 错误
}

// RPCError JSON-RPC 错误。
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ---------- Gateway 附件类型 ----------

// GatewayAttachment Gateway 附件（用于 chat.send 的图片附件）。
type GatewayAttachment struct {
	Type     string `json:"type"`
	MimeType string `json:"mimeType"`
	Content  string `json:"content"`
}

// ---------- Gateway 客户端常量（对应 TS GATEWAY_CLIENT_NAMES / MODES）----------

const (
	GatewayClientNameCLI = "cli"
	GatewayClientModeCLI = "cli"
)
