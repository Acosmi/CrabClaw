// provider_fetch_copilot.go — GitHub Copilot 使用量。
//
// TS 对照: provider-usage.fetch.copilot.ts (67L)
package cost

import (
	"context"
	"encoding/json"
)

func fetchCopilotUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	hdrs := map[string]string{
		"Authorization":        "token " + auth.Token,
		"Editor-Version":       "vscode/1.96.2",
		"User-Agent":           "GitHubCopilotChat/0.26.7",
		"X-Github-Api-Version": "2025-04-01",
	}
	resp, err := fetchJSON(ctx, "GET", "https://api.github.com/copilot_internal/user", hdrs, "")
	if err != nil {
		return errSnapshot(ProviderCopilot, err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errSnapshot(ProviderCopilot, httpErr(resp.StatusCode)), nil
	}
	var data struct {
		QuotaSnapshots *struct {
			PremiumInteractions *struct {
				PercentRemaining *float64 `json:"percent_remaining"`
			} `json:"premium_interactions"`
			Chat *struct {
				PercentRemaining *float64 `json:"percent_remaining"`
			} `json:"chat"`
		} `json:"quota_snapshots"`
		CopilotPlan string `json:"copilot_plan"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return errSnapshot(ProviderCopilot, "parse error"), nil
	}
	var ws []ProviderUsageWindow
	if data.QuotaSnapshots != nil {
		if pi := data.QuotaSnapshots.PremiumInteractions; pi != nil {
			rem := float64(0)
			if pi.PercentRemaining != nil {
				rem = *pi.PercentRemaining
			}
			ws = append(ws, ProviderUsageWindow{Label: "Premium", UsedPercent: ClampPercent(100 - rem)})
		}
		if ch := data.QuotaSnapshots.Chat; ch != nil {
			rem := float64(0)
			if ch.PercentRemaining != nil {
				rem = *ch.PercentRemaining
			}
			ws = append(ws, ProviderUsageWindow{Label: "Chat", UsedPercent: ClampPercent(100 - rem)})
		}
	}
	return okSnapshot(ProviderCopilot, ws, data.CopilotPlan), nil
}
