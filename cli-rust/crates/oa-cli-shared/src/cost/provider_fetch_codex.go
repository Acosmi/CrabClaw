// provider_fetch_codex.go — OpenAI Codex 使用量。
//
// TS 对照: provider-usage.fetch.codex.ts (101L)
package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

func fetchCodexUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	hdrs := map[string]string{
		"Authorization": "Bearer " + auth.Token,
		"User-Agent":    "CodexBar",
		"Accept":        "application/json",
	}
	if auth.AccountID != "" {
		hdrs["ChatGPT-Account-Id"] = auth.AccountID
	}
	resp, err := fetchJSON(ctx, "GET", "https://chatgpt.com/backend-api/wham/usage", hdrs, "")
	if err != nil {
		return errSnapshot(ProviderOpenAICodex, err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errSnapshot(ProviderOpenAICodex, "Token expired"), nil
	}
	if resp.StatusCode != 200 {
		return errSnapshot(ProviderOpenAICodex, httpErr(resp.StatusCode)), nil
	}
	var data struct {
		RateLimit *struct {
			PrimaryWindow   *codexRateWindow `json:"primary_window"`
			SecondaryWindow *codexRateWindow `json:"secondary_window"`
		} `json:"rate_limit"`
		PlanType string `json:"plan_type"`
		Credits  *struct {
			Balance interface{} `json:"balance"`
		} `json:"credits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return errSnapshot(ProviderOpenAICodex, "parse error"), nil
	}
	var ws []ProviderUsageWindow
	if data.RateLimit != nil {
		if pw := data.RateLimit.PrimaryWindow; pw != nil {
			h := int(math.Round(float64(pw.LimitWindowSec) / 3600))
			if h == 0 {
				h = 3
			}
			ws = append(ws, ProviderUsageWindow{
				Label: fmt.Sprintf("%dh", h), UsedPercent: ClampPercent(pw.UsedPercent),
				ResetAt: pw.epochMs(),
			})
		}
		if sw := data.RateLimit.SecondaryWindow; sw != nil {
			h := int(math.Round(float64(sw.LimitWindowSec) / 3600))
			label := fmt.Sprintf("%dh", h)
			if h >= 24 {
				label = "Day"
			}
			ws = append(ws, ProviderUsageWindow{
				Label: label, UsedPercent: ClampPercent(sw.UsedPercent),
				ResetAt: sw.epochMs(),
			})
		}
	}
	plan := data.PlanType
	if data.Credits != nil && data.Credits.Balance != nil {
		bal := parseNumeric(data.Credits.Balance)
		suffix := fmt.Sprintf("$%.2f", bal)
		if plan != "" {
			plan = fmt.Sprintf("%s (%s)", plan, suffix)
		} else {
			plan = suffix
		}
	}
	return okSnapshot(ProviderOpenAICodex, ws, plan), nil
}

type codexRateWindow struct {
	LimitWindowSec int     `json:"limit_window_seconds"`
	UsedPercent    float64 `json:"used_percent"`
	ResetAt        *int64  `json:"reset_at"`
}

func (w *codexRateWindow) epochMs() *int64 {
	if w.ResetAt == nil {
		return nil
	}
	v := *w.ResetAt * 1000
	return &v
}

func parseNumeric(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	}
	return 0
}
