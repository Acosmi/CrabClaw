// provider_fetch_zai.go — z.ai 使用量。
//
// TS 对照: provider-usage.fetch.zai.ts (97L)
package cost

import (
	"context"
	"encoding/json"
	"fmt"
)

func fetchZaiUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	hdrs := map[string]string{
		"Authorization": "Bearer " + auth.Token,
		"Accept":        "application/json",
	}
	resp, err := fetchJSON(ctx, "GET", "https://api.z.ai/api/monitor/usage/quota/limit", hdrs, "")
	if err != nil {
		return errSnapshot(ProviderZai, err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errSnapshot(ProviderZai, httpErr(resp.StatusCode)), nil
	}
	var data struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Data    *struct {
			PlanName string `json:"planName"`
			Plan     string `json:"plan"`
			Limits   []struct {
				Type          string  `json:"type"`
				Percentage    float64 `json:"percentage"`
				Unit          int     `json:"unit"`
				Number        int     `json:"number"`
				NextResetTime string  `json:"nextResetTime"`
			} `json:"limits"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return errSnapshot(ProviderZai, "parse error"), nil
	}
	if !data.Success || data.Code != 200 {
		msg := data.Msg
		if msg == "" {
			msg = "API error"
		}
		return errSnapshot(ProviderZai, msg), nil
	}
	var ws []ProviderUsageWindow
	if data.Data != nil {
		for _, lim := range data.Data.Limits {
			pct := ClampPercent(lim.Percentage)
			var resetAt *int64
			if lim.NextResetTime != "" {
				if t := parseISO(lim.NextResetTime); t > 0 {
					resetAt = &t
				}
			}
			wl := "Limit"
			switch lim.Unit {
			case 1:
				wl = fmt.Sprintf("%dd", lim.Number)
			case 3:
				wl = fmt.Sprintf("%dh", lim.Number)
			case 5:
				wl = fmt.Sprintf("%dm", lim.Number)
			}
			switch lim.Type {
			case "TOKENS_LIMIT":
				ws = append(ws, ProviderUsageWindow{Label: fmt.Sprintf("Tokens (%s)", wl), UsedPercent: pct, ResetAt: resetAt})
			case "TIME_LIMIT":
				ws = append(ws, ProviderUsageWindow{Label: "Monthly", UsedPercent: pct, ResetAt: resetAt})
			}
		}
	}
	plan := ""
	if data.Data != nil {
		plan = data.Data.PlanName
		if plan == "" {
			plan = data.Data.Plan
		}
	}
	return okSnapshot(ProviderZai, ws, plan), nil
}
