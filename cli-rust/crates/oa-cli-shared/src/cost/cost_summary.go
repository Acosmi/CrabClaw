// cost_summary.go — 跨会话成本聚合。
//
// TS 对照: infra/session-cost-usage.ts (lines 370-459)
//
// 多会话每日成本/使用量聚合。
package cost

import (
	"sort"
)

// LoadCostUsageSummary 加载跨会话汇总。
// TS 对照: session-cost-usage.ts loadCostUsageSummary()
func LoadCostUsageSummary(params CostSummaryParams) (*CostUsageSummary, error) {
	sessions, err := DiscoverAllSessions(params.StateDir, params.AgentID)
	if err != nil {
		return nil, err
	}

	// 限制会话数量
	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(sessions) > limit {
		sessions = sessions[len(sessions)-limit:]
	}

	totals := CostUsageTotals{}
	dailyMap := make(map[string]*CostUsageDailyEntry)

	for _, sess := range sessions {
		summary, err := LoadSessionCostSummary(SessionCostParams{
			SessionID:      sess.SessionID,
			TranscriptPath: sess.TranscriptPath,
			UsagePath:      sess.UsagePath,
		})
		if err != nil {
			continue // 跳过损坏的会话
		}

		// 累加总计
		totals.TotalInputTokens += summary.TotalCost.InputTokens
		totals.TotalOutputTokens += summary.TotalCost.OutputTokens
		totals.TotalCacheRead += summary.TotalCost.CacheRead
		totals.TotalCacheWrite += summary.TotalCost.CacheWrite
		totals.TotalCostUSD += summary.TotalCost.TotalCostUSD
		totals.TotalMessages += summary.MessageTotal.User + summary.MessageTotal.Assistant +
			summary.MessageTotal.System + summary.MessageTotal.Tool
		totals.TotalToolCalls += summary.ToolCallTotal

		// 聚合每日数据
		for _, daily := range summary.DailyUsage {
			if _, ok := dailyMap[daily.Date]; !ok {
				dailyMap[daily.Date] = &CostUsageDailyEntry{Date: daily.Date}
			}
			d := dailyMap[daily.Date]
			d.InputTokens += daily.Cost.InputTokens
			d.OutputTokens += daily.Cost.OutputTokens
			d.CacheRead += daily.Cost.CacheRead
			d.CacheWrite += daily.Cost.CacheWrite
			d.TotalCostUSD += daily.Cost.TotalCostUSD
			d.Messages += daily.Messages.User + daily.Messages.Assistant +
				daily.Messages.System + daily.Messages.Tool
			d.ToolCalls += daily.ToolCalls
		}
	}

	// 排序每日数据
	var days []string
	for d := range dailyMap {
		days = append(days, d)
	}
	sort.Strings(days)
	var dailyUsage []CostUsageDailyEntry
	for _, d := range days {
		dailyUsage = append(dailyUsage, *dailyMap[d])
	}

	return &CostUsageSummary{
		Totals:     totals,
		DailyUsage: dailyUsage,
	}, nil
}
