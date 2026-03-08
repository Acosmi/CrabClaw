// provider_types.go — 供应商使用量类型定义。
//
// TS 对照: infra/provider-usage.types.ts (29L) — 全量对齐
//
// 供应商使用量快照、使用量窗口、汇总。
package cost

// ---------- 供应商 ID ----------

// UsageProviderId 供应商标识。
// TS 对照: provider-usage.types.ts UsageProviderId
type UsageProviderId string

const (
	ProviderAnthropic   UsageProviderId = "anthropic"
	ProviderCopilot     UsageProviderId = "github-copilot"
	ProviderGeminiCLI   UsageProviderId = "google-gemini-cli"
	ProviderAntigravity UsageProviderId = "google-antigravity"
	ProviderMinimax     UsageProviderId = "minimax"
	ProviderOpenAICodex UsageProviderId = "openai-codex"
	ProviderXiaomi      UsageProviderId = "xiaomi"
	ProviderZai         UsageProviderId = "zai"
)

// ---------- 使用量窗口 ----------

// ProviderUsageWindow 使用量窗口（配额百分比）。
// TS 对照: provider-usage.types.ts UsageWindow
type ProviderUsageWindow struct {
	Label       string  `json:"label"`
	UsedPercent float64 `json:"usedPercent"`
	ResetAt     *int64  `json:"resetAt,omitempty"`
}

// ---------- 使用量快照 ----------

// ProviderUsageSnapshot 单个供应商使用量快照。
// TS 对照: provider-usage.types.ts ProviderUsageSnapshot
type ProviderUsageSnapshot struct {
	Provider    UsageProviderId       `json:"provider"`
	DisplayName string                `json:"displayName"`
	Windows     []ProviderUsageWindow `json:"windows"`
	Plan        string                `json:"plan,omitempty"`
	Error       string                `json:"error,omitempty"`
}

// ---------- 汇总 ----------

// ProviderUsageSummary 供应商使用量汇总。
// TS 对照: provider-usage.types.ts UsageSummary
type ProviderUsageSummary struct {
	UpdatedAt int64                   `json:"updatedAt"`
	Providers []ProviderUsageSnapshot `json:"providers"`
}
