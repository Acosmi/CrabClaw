// provider_shared.go — 供应商使用量共享常量和工具。
//
// TS 对照: infra/provider-usage.shared.ts (64L) — 全量对齐
package cost

import (
	"context"
	"math"
	"time"
)

// DefaultProviderTimeoutMs 默认超时（毫秒）。
const DefaultProviderTimeoutMs = 5000

// ProviderLabels 供应商显示名。
// TS 对照: provider-usage.shared.ts PROVIDER_LABELS
var ProviderLabels = map[UsageProviderId]string{
	ProviderAnthropic:   "Claude",
	ProviderCopilot:     "Copilot",
	ProviderGeminiCLI:   "Gemini",
	ProviderAntigravity: "Antigravity",
	ProviderMinimax:     "MiniMax",
	ProviderOpenAICodex: "Codex",
	ProviderXiaomi:      "Xiaomi",
	ProviderZai:         "z.ai",
}

// UsageProviders 所有支持的供应商列表。
var UsageProviders = []UsageProviderId{
	ProviderAnthropic,
	ProviderCopilot,
	ProviderGeminiCLI,
	ProviderAntigravity,
	ProviderMinimax,
	ProviderOpenAICodex,
	ProviderXiaomi,
	ProviderZai,
}

// IgnoredErrors 可忽略的认证错误。
var IgnoredErrors = map[string]bool{
	"No credentials": true,
	"No token":       true,
	"No API key":     true,
	"Not logged in":  true,
	"No auth":        true,
}

// ClampPercent 将百分比限制在 [0, 100]。
func ClampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

// WithTimeout 带超时执行。
func WithTimeout[T any](ctx context.Context, fn func(context.Context) (T, error), ms int) (T, error) {
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
	defer cancel()
	return fn(ctx2)
}

// parseISO 将 ISO 8601 字符串转为 epoch 毫秒。
func parseISO(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}
