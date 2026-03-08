// provider_fetch_claude.go — Claude/Anthropic 使用量获取。
//
// TS 对照: provider-usage.fetch.claude.ts (197L)
package cost

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

type claudeUsageResp struct {
	FiveHour       *claudeWindow `json:"five_hour"`
	SevenDay       *claudeWindow `json:"seven_day"`
	SevenDaySonnet *claudeWindow `json:"seven_day_sonnet"`
	SevenDayOpus   *claudeWindow `json:"seven_day_opus"`
}

type claudeWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

func resolveClaudeWebSessionKey() string {
	for _, k := range []string{"CLAUDE_AI_SESSION_KEY", "CLAUDE_WEB_SESSION_KEY"} {
		if v := strings.TrimSpace(os.Getenv(k)); strings.HasPrefix(v, "sk-ant-") {
			return v
		}
	}
	cookie := strings.TrimSpace(os.Getenv("CLAUDE_WEB_COOKIE"))
	if cookie == "" {
		return ""
	}
	stripped := regexp.MustCompile(`(?i)^cookie:\s*`).ReplaceAllString(cookie, "")
	m := regexp.MustCompile(`(?i)(?:^|;\s*)sessionKey=([^;\s]+)`).FindStringSubmatch(stripped)
	if len(m) > 1 && strings.HasPrefix(strings.TrimSpace(m[1]), "sk-ant-") {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseClaudeWindows(data *claudeUsageResp) []ProviderUsageWindow {
	var ws []ProviderUsageWindow
	if data.FiveHour != nil && data.FiveHour.Utilization != nil {
		w := ProviderUsageWindow{Label: "5h", UsedPercent: ClampPercent(*data.FiveHour.Utilization)}
		if data.FiveHour.ResetsAt != "" {
			if t := parseISO(data.FiveHour.ResetsAt); t > 0 {
				w.ResetAt = &t
			}
		}
		ws = append(ws, w)
	}
	if data.SevenDay != nil && data.SevenDay.Utilization != nil {
		w := ProviderUsageWindow{Label: "Week", UsedPercent: ClampPercent(*data.SevenDay.Utilization)}
		if data.SevenDay.ResetsAt != "" {
			if t := parseISO(data.SevenDay.ResetsAt); t > 0 {
				w.ResetAt = &t
			}
		}
		ws = append(ws, w)
	}
	mw := data.SevenDaySonnet
	label := "Sonnet"
	if mw == nil {
		mw = data.SevenDayOpus
		label = "Opus"
	}
	if mw != nil && mw.Utilization != nil {
		ws = append(ws, ProviderUsageWindow{Label: label, UsedPercent: ClampPercent(*mw.Utilization)})
	}
	return ws
}

func claudeUsageHeaders(auth ProviderAuth) map[string]string {
	return map[string]string{
		"Authorization":     "Bearer " + auth.Token,
		"User-Agent":        "CrabClaw",
		"Accept":            "application/json",
		"anthropic-version": "2023-06-01",
		"anthropic-beta":    "oauth-2025-04-20",
	}
}

func fetchClaudeUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	hdrs := claudeUsageHeaders(auth)
	resp, err := fetchJSON(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", hdrs, "")
	if err != nil {
		return errSnapshot(ProviderAnthropic, err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errSnapshot(ProviderAnthropic, httpErr(resp.StatusCode)), nil
	}
	var data claudeUsageResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return errSnapshot(ProviderAnthropic, "parse error"), nil
	}
	ws := parseClaudeWindows(&data)
	return okSnapshot(ProviderAnthropic, ws, ""), nil
}
