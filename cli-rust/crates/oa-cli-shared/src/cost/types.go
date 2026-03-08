// types.go — 会话成本与使用量类型定义。
//
// TS 对照: infra/session-cost-usage.ts (lines 15-147)
//
// 定义成本分解、使用量汇总、每日使用、延迟统计、
// 模型使用、会话发现等所有相关类型。
package cost

// ---------- 成本分解 ----------

// CostBreakdown 成本分解。
// TS 对照: session-cost-usage.ts CostBreakdown
type CostBreakdown struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CacheRead    int     `json:"cacheRead"`
	CacheWrite   int     `json:"cacheWrite"`
	TotalCostUSD float64 `json:"totalCostUsd"`
}

// ---------- 使用量汇总 ----------

// CostUsageTotals 使用量总计。
// TS 对照: session-cost-usage.ts CostUsageTotals
type CostUsageTotals struct {
	TotalInputTokens  int     `json:"totalInputTokens"`
	TotalOutputTokens int     `json:"totalOutputTokens"`
	TotalCacheRead    int     `json:"totalCacheRead"`
	TotalCacheWrite   int     `json:"totalCacheWrite"`
	TotalCostUSD      float64 `json:"totalCostUsd"`
	TotalMessages     int     `json:"totalMessages"`
	TotalToolCalls    int     `json:"totalToolCalls"`
}

// CostUsageDailyEntry 每日使用条目。
// TS 对照: session-cost-usage.ts CostUsageDailyEntry
type CostUsageDailyEntry struct {
	Date         string  `json:"date"` // "YYYY-MM-DD"
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CacheRead    int     `json:"cacheRead"`
	CacheWrite   int     `json:"cacheWrite"`
	TotalCostUSD float64 `json:"totalCostUsd"`
	Messages     int     `json:"messages"`
	ToolCalls    int     `json:"toolCalls"`
}

// CostUsageSummary 跨会话汇总。
// TS 对照: session-cost-usage.ts CostUsageSummary
type CostUsageSummary struct {
	Totals     CostUsageTotals       `json:"totals"`
	DailyUsage []CostUsageDailyEntry `json:"dailyUsage"`
}

// ---------- 会话使用量 ----------

// SessionDailyUsage 会话每日使用。
// TS 对照: session-cost-usage.ts SessionDailyUsage
type SessionDailyUsage struct {
	Date      string               `json:"date"`
	Cost      CostBreakdown        `json:"cost"`
	Messages  SessionMessageCounts `json:"messages"`
	ToolCalls int                  `json:"toolCalls"`
}

// SessionMessageCounts 消息计数。
// TS 对照: session-cost-usage.ts SessionMessageCounts
type SessionMessageCounts struct {
	User      int `json:"user"`
	Assistant int `json:"assistant"`
	System    int `json:"system"`
	Tool      int `json:"tool"`
}

// SessionLatencyStats 延迟统计。
// TS 对照: session-cost-usage.ts SessionLatencyStats
type SessionLatencyStats struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Avg    float64 `json:"avg"`
	Median float64 `json:"median"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	Count  int     `json:"count"`
}

// SessionToolUsage 工具使用量。
// TS 对照: session-cost-usage.ts SessionToolUsage
type SessionToolUsage struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	AvgMs float64 `json:"avgMs,omitempty"`
}

// SessionModelUsage 模型使用量。
// TS 对照: session-cost-usage.ts SessionModelUsage
type SessionModelUsage struct {
	ModelID      string        `json:"modelId"`
	Provider     string        `json:"provider"`
	Cost         CostBreakdown `json:"cost"`
	RequestCount int           `json:"requestCount"`
}

// SessionCostSummary 完整会话成本摘要。
// TS 对照: session-cost-usage.ts SessionCostSummary
type SessionCostSummary struct {
	SessionID     string               `json:"sessionId"`
	TotalCost     CostBreakdown        `json:"totalCost"`
	DailyUsage    []SessionDailyUsage  `json:"dailyUsage"`
	Latency       *SessionLatencyStats `json:"latency,omitempty"`
	TopTools      []SessionToolUsage   `json:"topTools"`
	ModelUsage    []SessionModelUsage  `json:"modelUsage"`
	MessageTotal  SessionMessageCounts `json:"messageTotal"`
	ToolCallTotal int                  `json:"toolCallTotal"`
	FirstSeenMs   int64                `json:"firstSeenMs"`
	LastSeenMs    int64                `json:"lastSeenMs"`
}

// ---------- 时间序列 ----------

// SessionUsageTimePoint 使用量时间点。
// TS 对照: session-cost-usage.ts SessionUsageTimePoint
type SessionUsageTimePoint struct {
	TimestampMs      int64   `json:"timestampMs"`
	CumulativeCost   float64 `json:"cumulativeCost"`
	CumulativeInput  int     `json:"cumulativeInput"`
	CumulativeOutput int     `json:"cumulativeOutput"`
	EventType        string  `json:"eventType,omitempty"`
}

// SessionUsageTimeSeries 使用量时间序列。
// TS 对照: session-cost-usage.ts SessionUsageTimeSeries
type SessionUsageTimeSeries struct {
	SessionID string                  `json:"sessionId"`
	Points    []SessionUsageTimePoint `json:"points"`
	TotalCost float64                 `json:"totalCost"`
}

// ---------- 会话日志 ----------

// SessionLogEntry 会话日志条目。
// TS 对照: session-cost-usage.ts SessionLogEntry
type SessionLogEntry struct {
	TimestampMs int64  `json:"timestampMs"`
	Level       string `json:"level"`
	Message     string `json:"message"`
	Source      string `json:"source,omitempty"`
}

// ---------- 会话发现 ----------

// DiscoveredSession 已发现的会话。
// TS 对照: session-cost-usage.ts DiscoveredSession
type DiscoveredSession struct {
	SessionID      string `json:"sessionId"`
	AgentID        string `json:"agentId"`
	TranscriptPath string `json:"transcriptPath"`
	UsagePath      string `json:"usagePath,omitempty"`
	CreatedAtMs    int64  `json:"createdAtMs"`
}

// ---------- 加载参数 ----------

// SessionCostParams 会话成本加载参数。
type SessionCostParams struct {
	SessionID      string
	TranscriptPath string
	UsagePath      string
}

// CostSummaryParams 汇总加载参数。
type CostSummaryParams struct {
	StateDir string
	AgentID  string
	Limit    int
}

// TimeSeriesParams 时间序列加载参数。
type TimeSeriesParams struct {
	SessionID      string
	TranscriptPath string
	MaxPoints      int
}

// SessionLogsParams 会话日志加载参数。
type SessionLogsParams struct {
	SessionID      string
	TranscriptPath string
	MaxEntries     int
}
