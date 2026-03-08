// provider_format.go — 供应商使用量格式化输出。
//
// TS 对照: provider-usage.format.ts (129L)
package cost

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// FormatResetRemaining 格式化剩余重置时间。
func FormatResetRemaining(targetMs *int64, now int64) string {
	if targetMs == nil {
		return ""
	}
	if now == 0 {
		now = time.Now().UnixMilli()
	}
	diff := *targetMs - now
	if diff <= 0 {
		return "now"
	}
	mins := int(diff / 60000)
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	hours := mins / 60
	m := mins % 60
	if hours < 24 {
		if m > 0 {
			return fmt.Sprintf("%dh %dm", hours, m)
		}
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	if days < 7 {
		return fmt.Sprintf("%dd %dh", days, hours%24)
	}
	t := time.UnixMilli(*targetMs)
	return t.Format("Jan 2")
}

// FormatUsageReportLines 格式化使用量报告行。
func FormatUsageReportLines(summary *ProviderUsageSummary, now int64) []string {
	if len(summary.Providers) == 0 {
		return []string{"Usage: no provider usage available."}
	}
	lines := []string{"Usage:"}
	for _, e := range summary.Providers {
		plan := ""
		if e.Plan != "" {
			plan = fmt.Sprintf(" (%s)", e.Plan)
		}
		if e.Error != "" {
			lines = append(lines, fmt.Sprintf("  %s%s: %s", e.DisplayName, plan, e.Error))
			continue
		}
		if len(e.Windows) == 0 {
			lines = append(lines, fmt.Sprintf("  %s%s: no data", e.DisplayName, plan))
			continue
		}
		lines = append(lines, fmt.Sprintf("  %s%s", e.DisplayName, plan))
		for _, w := range e.Windows {
			rem := ClampPercent(100 - w.UsedPercent)
			reset := FormatResetRemaining(w.ResetAt, now)
			suffix := ""
			if reset != "" {
				suffix = " · resets " + reset
			}
			lines = append(lines, fmt.Sprintf("    %s: %.0f%% left%s", w.Label, rem, suffix))
		}
	}
	return lines
}

// FormatUsageSummaryLine 单行摘要。
func FormatUsageSummaryLine(summary *ProviderUsageSummary, now int64) string {
	var parts []string
	for _, e := range summary.Providers {
		if len(e.Windows) == 0 || e.Error != "" {
			continue
		}
		best := pickPrimaryWindow(e.Windows)
		if best == nil {
			continue
		}
		rem := ClampPercent(100 - best.UsedPercent)
		parts = append(parts, fmt.Sprintf("%s %.0f%% left (%s)", e.DisplayName, rem, best.Label))
	}
	if len(parts) == 0 {
		return ""
	}
	return "📊 Usage: " + strings.Join(parts, " · ")
}

func pickPrimaryWindow(ws []ProviderUsageWindow) *ProviderUsageWindow {
	if len(ws) == 0 {
		return nil
	}
	best := &ws[0]
	for i := 1; i < len(ws); i++ {
		if ws[i].UsedPercent > best.UsedPercent {
			best = &ws[i]
		}
	}
	return best
}

// math import used by ClampPercent in shared
var _ = math.Max
