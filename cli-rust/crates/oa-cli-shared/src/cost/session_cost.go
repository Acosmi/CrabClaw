// session_cost.go — 会话成本与使用量加载。
//
// TS 对照: infra/session-cost-usage.ts (lines 149-1092)
//
// JSONL 文件扫描、成本分解计算、每日使用汇总、
// 延迟统计、模型使用跟踪、时间序列生成。
package cost

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------- JSONL 扫描 ----------

// transcriptEntry 转录文件条目（通用字段）。
type transcriptEntry struct {
	TimestampMs int64                  `json:"timestampMs"`
	Type        string                 `json:"type"`
	Role        string                 `json:"role,omitempty"`
	ToolName    string                 `json:"toolName,omitempty"`
	ToolCalls   []json.RawMessage      `json:"toolCalls,omitempty"`
	Usage       *transcriptUsage       `json:"usage,omitempty"`
	Model       string                 `json:"model,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
	LatencyMs   float64                `json:"latencyMs,omitempty"`
	Level       string                 `json:"level,omitempty"`
	Message     string                 `json:"message,omitempty"`
	Source      string                 `json:"source,omitempty"`
	Extra       map[string]interface{} `json:"-"`
}

type transcriptUsage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CacheRead    int     `json:"cacheRead"`
	CacheWrite   int     `json:"cacheWrite"`
	TotalCostUSD float64 `json:"totalCostUsd"`
}

// scanTranscriptFile 逐行扫描 JSONL 转录文件。
// TS 对照: session-cost-usage.ts scanTranscriptFile()
func scanTranscriptFile(filePath string, onEntry func(entry transcriptEntry)) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening transcript: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry transcriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // 跳过损坏行
		}
		onEntry(entry)
	}
	return scanner.Err()
}

// ---------- LoadSessionCostSummary ----------

// LoadSessionCostSummary 加载单个会话的成本摘要。
// TS 对照: session-cost-usage.ts loadSessionCostSummary()
func LoadSessionCostSummary(params SessionCostParams) (*SessionCostSummary, error) {
	summary := &SessionCostSummary{
		SessionID: params.SessionID,
	}

	dailyMap := make(map[string]*SessionDailyUsage)
	modelMap := make(map[string]*SessionModelUsage)
	toolMap := make(map[string]int)
	var latencies []float64
	var firstTs, lastTs int64

	err := scanTranscriptFile(params.TranscriptPath, func(entry transcriptEntry) {
		ts := entry.TimestampMs

		// 跟踪时间范围
		if firstTs == 0 || ts < firstTs {
			firstTs = ts
		}
		if ts > lastTs {
			lastTs = ts
		}

		// 每日日期
		date := msToDateString(ts)

		// 确保 daily entry 存在
		if _, ok := dailyMap[date]; !ok {
			dailyMap[date] = &SessionDailyUsage{Date: date}
		}
		daily := dailyMap[date]

		// 消息计数
		switch entry.Role {
		case "user":
			summary.MessageTotal.User++
			daily.Messages.User++
		case "assistant":
			summary.MessageTotal.Assistant++
			daily.Messages.Assistant++
		case "system":
			summary.MessageTotal.System++
			daily.Messages.System++
		case "tool":
			summary.MessageTotal.Tool++
			daily.Messages.Tool++
		}

		// 工具调用
		if len(entry.ToolCalls) > 0 {
			for _, tc := range entry.ToolCalls {
				var toolCall struct {
					Name string `json:"name"`
				}
				_ = json.Unmarshal(tc, &toolCall)
				if toolCall.Name != "" {
					toolMap[toolCall.Name]++
					summary.ToolCallTotal++
					daily.ToolCalls++
				}
			}
		}
		if entry.ToolName != "" {
			toolMap[entry.ToolName]++
		}

		// 使用量和成本
		if entry.Usage != nil {
			u := entry.Usage
			summary.TotalCost.InputTokens += u.InputTokens
			summary.TotalCost.OutputTokens += u.OutputTokens
			summary.TotalCost.CacheRead += u.CacheRead
			summary.TotalCost.CacheWrite += u.CacheWrite
			summary.TotalCost.TotalCostUSD += u.TotalCostUSD

			daily.Cost.InputTokens += u.InputTokens
			daily.Cost.OutputTokens += u.OutputTokens
			daily.Cost.CacheRead += u.CacheRead
			daily.Cost.CacheWrite += u.CacheWrite
			daily.Cost.TotalCostUSD += u.TotalCostUSD

			// 模型使用
			modelKey := entry.Model
			if modelKey == "" {
				modelKey = "unknown"
			}
			if _, ok := modelMap[modelKey]; !ok {
				modelMap[modelKey] = &SessionModelUsage{
					ModelID:  modelKey,
					Provider: entry.Provider,
				}
			}
			mu := modelMap[modelKey]
			mu.Cost.InputTokens += u.InputTokens
			mu.Cost.OutputTokens += u.OutputTokens
			mu.Cost.CacheRead += u.CacheRead
			mu.Cost.CacheWrite += u.CacheWrite
			mu.Cost.TotalCostUSD += u.TotalCostUSD
			mu.RequestCount++
		}

		// 延迟
		if entry.LatencyMs > 0 {
			latencies = append(latencies, entry.LatencyMs)
		}
	})

	if err != nil {
		return nil, err
	}

	// 排序每日使用
	var days []string
	for d := range dailyMap {
		days = append(days, d)
	}
	sort.Strings(days)
	for _, d := range days {
		summary.DailyUsage = append(summary.DailyUsage, *dailyMap[d])
	}

	// 延迟统计
	if len(latencies) > 0 {
		summary.Latency = computeLatencyStats(latencies)
	}

	// Top tools
	type toolCount struct {
		name  string
		count int
	}
	var tools []toolCount
	for name, count := range toolMap {
		tools = append(tools, toolCount{name, count})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].count > tools[j].count
	})
	limit := 20
	if len(tools) < limit {
		limit = len(tools)
	}
	for _, t := range tools[:limit] {
		summary.TopTools = append(summary.TopTools, SessionToolUsage{
			Name:  t.name,
			Count: t.count,
		})
	}

	// 模型使用
	for _, mu := range modelMap {
		summary.ModelUsage = append(summary.ModelUsage, *mu)
	}
	sort.Slice(summary.ModelUsage, func(i, j int) bool {
		return summary.ModelUsage[i].Cost.TotalCostUSD > summary.ModelUsage[j].Cost.TotalCostUSD
	})

	summary.FirstSeenMs = firstTs
	summary.LastSeenMs = lastTs

	return summary, nil
}

// ---------- LoadSessionUsageTimeSeries ----------

// LoadSessionUsageTimeSeries 加载会话使用量时间序列。
// TS 对照: session-cost-usage.ts loadSessionUsageTimeSeries()
func LoadSessionUsageTimeSeries(params TimeSeriesParams) (*SessionUsageTimeSeries, error) {
	series := &SessionUsageTimeSeries{
		SessionID: params.SessionID,
	}

	var cumCost float64
	var cumInput, cumOutput int

	err := scanTranscriptFile(params.TranscriptPath, func(entry transcriptEntry) {
		if entry.Usage == nil {
			return
		}
		u := entry.Usage
		cumCost += u.TotalCostUSD
		cumInput += u.InputTokens
		cumOutput += u.OutputTokens

		series.Points = append(series.Points, SessionUsageTimePoint{
			TimestampMs:      entry.TimestampMs,
			CumulativeCost:   cumCost,
			CumulativeInput:  cumInput,
			CumulativeOutput: cumOutput,
			EventType:        entry.Type,
		})
	})

	if err != nil {
		return nil, err
	}

	series.TotalCost = cumCost

	// 下采样
	maxPoints := params.MaxPoints
	if maxPoints <= 0 {
		maxPoints = 500
	}
	if len(series.Points) > maxPoints {
		series.Points = downsamplePoints(series.Points, maxPoints)
	}

	return series, nil
}

// downsamplePoints 下采样时间序列数据点。
// TS 对照: session-cost-usage.ts downsamplePoints()
func downsamplePoints(points []SessionUsageTimePoint, maxPoints int) []SessionUsageTimePoint {
	if len(points) <= maxPoints {
		return points
	}

	result := make([]SessionUsageTimePoint, 0, maxPoints)
	// 始终保留第一个和最后一个
	result = append(result, points[0])

	step := float64(len(points)-1) / float64(maxPoints-1)
	for i := 1; i < maxPoints-1; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx >= len(points) {
			idx = len(points) - 1
		}
		result = append(result, points[idx])
	}

	result = append(result, points[len(points)-1])
	return result
}

// ---------- LoadSessionLogs ----------

// LoadSessionLogs 加载会话日志条目。
// TS 对照: session-cost-usage.ts loadSessionLogs()
func LoadSessionLogs(params SessionLogsParams) ([]SessionLogEntry, error) {
	maxEntries := params.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 100
	}

	var entries []SessionLogEntry

	err := scanTranscriptFile(params.TranscriptPath, func(entry transcriptEntry) {
		if entry.Level == "" && entry.Message == "" {
			return
		}
		entries = append(entries, SessionLogEntry{
			TimestampMs: entry.TimestampMs,
			Level:       entry.Level,
			Message:     entry.Message,
			Source:      entry.Source,
		})
	})
	if err != nil {
		return nil, err
	}

	// 保留最近 N 条
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}

	return entries, nil
}

// ---------- DiscoverAllSessions ----------

// DiscoverAllSessions 发现所有会话。
// TS 对照: session-cost-usage.ts discoverAllSessions()
func DiscoverAllSessions(stateDir, agentID string) ([]DiscoveredSession, error) {
	sessionsDir := filepath.Join(stateDir, "agents", agentID, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}

	var sessions []DiscoveredSession
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sessionID := e.Name()
		transcriptPath := filepath.Join(sessionsDir, sessionID, "transcript.jsonl")
		usagePath := filepath.Join(sessionsDir, sessionID, "usage.jsonl")

		// 检查是否存在转录文件
		if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
			continue
		}

		info, _ := e.Info()
		var createdAt int64
		if info != nil {
			createdAt = info.ModTime().UnixMilli()
		}

		ds := DiscoveredSession{
			SessionID:      sessionID,
			AgentID:        agentID,
			TranscriptPath: transcriptPath,
			CreatedAtMs:    createdAt,
		}

		if _, err := os.Stat(usagePath); err == nil {
			ds.UsagePath = usagePath
		}

		sessions = append(sessions, ds)
	}

	return sessions, nil
}

// ---------- 辅助函数 ----------

// msToDateString 将毫秒时间戳转为 "YYYY-MM-DD" 格式。
func msToDateString(ms int64) string {
	t := time.UnixMilli(ms)
	return t.Format("2006-01-02")
}

// computeLatencyStats 计算延迟统计。
// TS 对照: session-cost-usage.ts computeLatencyStats()
func computeLatencyStats(latencies []float64) *SessionLatencyStats {
	if len(latencies) == 0 {
		return nil
	}

	sort.Float64s(latencies)
	n := len(latencies)

	var sum float64
	for _, v := range latencies {
		sum += v
	}

	return &SessionLatencyStats{
		Min:    latencies[0],
		Max:    latencies[n-1],
		Avg:    sum / float64(n),
		Median: percentile(latencies, 50),
		P95:    percentile(latencies, 95),
		P99:    percentile(latencies, 99),
		Count:  n,
	}
}

// percentile 计算已排序切片的百分位数。
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if upper >= len(sorted) {
		upper = len(sorted) - 1
	}
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
