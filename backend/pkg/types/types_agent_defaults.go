package types

// Agent 默认配置类型 — 继承自 src/config/types.agent-defaults.ts (269 行)

// AgentModelEntryConfig 模型目录中的单个模型入口配置
type AgentModelEntryConfig struct {
	Alias     string                 `json:"alias,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`    // 供应商特定 API 参数
	Streaming *bool                  `json:"streaming,omitempty"` // 默认 true，Ollama 设为 false
}

// AgentModelListConfig 模型选择配置（主模型 + 备选）
type AgentModelListConfig struct {
	Primary   string    `json:"primary,omitempty"`
	Fallbacks *[]string `json:"fallbacks,omitempty"`
}

// ============================================================
// 上下文裁剪 (Context Pruning) 配置
// ============================================================

// AgentContextPruningToolsConfig 裁剪工具过滤
type AgentContextPruningToolsConfig struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// AgentContextPruningSoftTrimConfig 软裁剪配置
type AgentContextPruningSoftTrimConfig struct {
	MaxChars  *int `json:"maxChars,omitempty"`
	HeadChars *int `json:"headChars,omitempty"`
	TailChars *int `json:"tailChars,omitempty"`
}

// AgentContextPruningHardClearConfig 硬清理配置
type AgentContextPruningHardClearConfig struct {
	Enabled     *bool  `json:"enabled,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// AgentContextPruningConfig 上下文裁剪配置
// 原版: export type AgentContextPruningConfig
type AgentContextPruningConfig struct {
	Mode                 string                              `json:"mode,omitempty"` // "off"|"cache-ttl"
	TTL                  string                              `json:"ttl,omitempty"`  // 过期时间字符串
	KeepLastAssistants   *int                                `json:"keepLastAssistants,omitempty"`
	SoftTrimRatio        *float64                            `json:"softTrimRatio,omitempty"`
	HardClearRatio       *float64                            `json:"hardClearRatio,omitempty"`
	MinPrunableToolChars *int                                `json:"minPrunableToolChars,omitempty"`
	Tools                *AgentContextPruningToolsConfig     `json:"tools,omitempty"`
	SoftTrim             *AgentContextPruningSoftTrimConfig  `json:"softTrim,omitempty"`
	HardClear            *AgentContextPruningHardClearConfig `json:"hardClear,omitempty"`
}

// ============================================================
// CLI 后端配置
// ============================================================

// CliBackendConfig CLI 后端配置
// 原版: export type CliBackendConfig
type CliBackendConfig struct {
	Command           string            `json:"command"`
	Args              []string          `json:"args,omitempty"`
	Output            string            `json:"output,omitempty"`       // "json"|"text"|"jsonl"
	ResumeOutput      string            `json:"resumeOutput,omitempty"` // "json"|"text"|"jsonl"
	Input             string            `json:"input,omitempty"`        // "arg"|"stdin"
	MaxPromptArgChars *int              `json:"maxPromptArgChars,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	ClearEnv          []string          `json:"clearEnv,omitempty"`
	ModelArg          string            `json:"modelArg,omitempty"`
	ModelAliases      map[string]string `json:"modelAliases,omitempty"`
	SessionArg        string            `json:"sessionArg,omitempty"`
	SessionArgs       []string          `json:"sessionArgs,omitempty"`
	ResumeArgs        []string          `json:"resumeArgs,omitempty"`
	SessionMode       string            `json:"sessionMode,omitempty"` // "always"|"existing"|"none"
	SessionIDFields   []string          `json:"sessionIdFields,omitempty"`
	SystemPromptArg   string            `json:"systemPromptArg,omitempty"`
	SystemPromptMode  string            `json:"systemPromptMode,omitempty"` // "append"|"replace"
	SystemPromptWhen  string            `json:"systemPromptWhen,omitempty"` // "first"|"always"|"never"
	ImageArg          string            `json:"imageArg,omitempty"`
	ImageMode         string            `json:"imageMode,omitempty"` // "repeat"|"list"
	Serialize         *bool             `json:"serialize,omitempty"`
}

// ============================================================
// 压缩 (Compaction) 配置
// ============================================================

// AgentCompactionMode 上下文压缩模式
type AgentCompactionMode string

const (
	CompactionDefault   AgentCompactionMode = "default"
	CompactionSafeguard AgentCompactionMode = "safeguard"
)

// AgentCompactionMemoryFlushConfig 压缩前记忆刷新配置
type AgentCompactionMemoryFlushConfig struct {
	Enabled             *bool  `json:"enabled,omitempty"`
	SoftThresholdTokens *int   `json:"softThresholdTokens,omitempty"`
	Prompt              string `json:"prompt,omitempty"`
	SystemPrompt        string `json:"systemPrompt,omitempty"`
}

// AgentCompactionConfig 压缩配置
type AgentCompactionConfig struct {
	Mode               AgentCompactionMode               `json:"mode,omitempty"`
	ReserveTokensFloor *int                              `json:"reserveTokensFloor,omitempty"`
	MaxHistoryShare    *float64                          `json:"maxHistoryShare,omitempty"` // 0.1-0.9，默认 0.5
	MemoryFlush        *AgentCompactionMemoryFlushConfig `json:"memoryFlush,omitempty"`
}

// ============================================================
// 心跳 (Heartbeat) 配置
// ============================================================

// HeartbeatActiveHoursConfig 心跳活动时段
type HeartbeatActiveHoursConfig struct {
	Start    string `json:"start,omitempty"`    // 24h 格式 HH:MM
	End      string `json:"end,omitempty"`      // 24h 格式 HH:MM，"24:00" 表示日末
	Timezone string `json:"timezone,omitempty"` // "user"|"local"|IANA TZ
}

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
	Every            string                      `json:"every,omitempty"` // 间隔字符串，默认 30m
	ActiveHours      *HeartbeatActiveHoursConfig `json:"activeHours,omitempty"`
	Model            string                      `json:"model,omitempty"`
	Session          string                      `json:"session,omitempty"` // "main" 或显式 key
	Target           string                      `json:"target,omitempty"`  // "last"|"none"|channelId
	To               string                      `json:"to,omitempty"`      // E.164 号码或 chat ID
	AccountID        string                      `json:"accountId,omitempty"`
	Prompt           string                      `json:"prompt,omitempty"`
	AckMaxChars      *int                        `json:"ackMaxChars,omitempty"`
	IncludeReasoning *bool                       `json:"includeReasoning,omitempty"`
}

// ============================================================
// 子代理 (Subagent) 默认配置
// ============================================================

// SubagentDefaultsConfig 子代理默认配置
type SubagentDefaultsConfig struct {
	MaxConcurrent       *int        `json:"maxConcurrent,omitempty"`
	ArchiveAfterMinutes *int        `json:"archiveAfterMinutes,omitempty"`
	Model               interface{} `json:"model,omitempty"` // string | {primary, fallbacks}
	Thinking            string      `json:"thinking,omitempty"`
}

// ============================================================
// 沙箱默认配置
// ============================================================

// SandboxDefaultsConfig 沙箱默认配置（Agent 级别）
type SandboxDefaultsConfig struct {
	Mode                   string                  `json:"mode,omitempty"`                   // "off"|"non-main"|"all"
	WorkspaceAccess        string                  `json:"workspaceAccess,omitempty"`        // "none"|"ro"|"rw"
	SessionToolsVisibility string                  `json:"sessionToolsVisibility,omitempty"` // "spawned"|"all"
	Scope                  string                  `json:"scope,omitempty"`                  // "session"|"agent"|"shared"
	PerSession             *bool                   `json:"perSession,omitempty"`             // @deprecated
	WorkspaceRoot          string                  `json:"workspaceRoot,omitempty"`
	Docker                 *SandboxDockerSettings  `json:"docker,omitempty"`
	Browser                *SandboxBrowserSettings `json:"browser,omitempty"`
	Prune                  *SandboxPruneSettings   `json:"prune,omitempty"`
}

// ============================================================
// Agent 默认总配置
// ============================================================

// AgentDefaultsConfig Agent 默认配置
// 原版: export type AgentDefaultsConfig (269 行)
type AgentDefaultsConfig struct {
	Model                  *AgentModelListConfig             `json:"model,omitempty"`
	ImageModel             *AgentModelListConfig             `json:"imageModel,omitempty"`
	Models                 map[string]*AgentModelEntryConfig `json:"models,omitempty"`
	Workspace              string                            `json:"workspace,omitempty"`
	RepoRoot               string                            `json:"repoRoot,omitempty"`
	SkipBootstrap          *bool                             `json:"skipBootstrap,omitempty"`
	BootstrapMaxChars      *int                              `json:"bootstrapMaxChars,omitempty"`
	UserTimezone           string                            `json:"userTimezone,omitempty"`
	TimeFormat             string                            `json:"timeFormat,omitempty"` // "auto"|"12"|"24"
	EnvelopeTimezone       string                            `json:"envelopeTimezone,omitempty"`
	EnvelopeTimestamp      string                            `json:"envelopeTimestamp,omitempty"` // "on"|"off"
	EnvelopeElapsed        string                            `json:"envelopeElapsed,omitempty"`   // "on"|"off"
	ContextTokens          *int                              `json:"contextTokens,omitempty"`
	CliBackends            map[string]*CliBackendConfig      `json:"cliBackends,omitempty"`
	ContextPruning         *AgentContextPruningConfig        `json:"contextPruning,omitempty"`
	Compaction             *AgentCompactionConfig            `json:"compaction,omitempty"`
	MemorySearch           *MemorySearchConfig               `json:"memorySearch,omitempty"`
	ThinkingDefault        string                            `json:"thinkingDefault,omitempty"`
	VerboseDefault         string                            `json:"verboseDefault,omitempty"`
	ElevatedDefault        string                            `json:"elevatedDefault,omitempty"`
	BlockStreamingDefault  string                            `json:"blockStreamingDefault,omitempty"`
	BlockStreamingBreak    string                            `json:"blockStreamingBreak,omitempty"`
	BlockStreamingChunk    *BlockStreamingChunkConfig        `json:"blockStreamingChunk,omitempty"`
	BlockStreamingCoalesce *BlockStreamingCoalesceConfig     `json:"blockStreamingCoalesce,omitempty"`
	HumanDelay             *HumanDelayConfig                 `json:"humanDelay,omitempty"`
	TimeoutSeconds         *int                              `json:"timeoutSeconds,omitempty"`
	MediaMaxMB             *int                              `json:"mediaMaxMb,omitempty"`
	TypingIntervalSeconds  *int                              `json:"typingIntervalSeconds,omitempty"`
	TypingMode             TypingMode                        `json:"typingMode,omitempty"`
	Heartbeat              *HeartbeatConfig                  `json:"heartbeat,omitempty"`
	MaxConcurrent          *int                              `json:"maxConcurrent,omitempty"`
	Subagents              *SubagentDefaultsConfig           `json:"subagents,omitempty"`
	Sandbox                *SandboxDefaultsConfig            `json:"sandbox,omitempty"`
	// (Phase 2A: Coder *CoderDefaultsConfig 已删除 — oa-coder 升级为 spawn_coder_agent)
}
