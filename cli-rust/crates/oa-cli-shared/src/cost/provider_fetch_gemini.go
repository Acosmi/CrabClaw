// provider_fetch_gemini.go — Gemini 使用量。
//
// TS 对照: provider-usage.fetch.gemini.ts (90L)
package cost

import (
	"context"
	"encoding/json"
	"strings"
)

func fetchGeminiUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	pid := auth.Provider
	hdrs := map[string]string{
		"Authorization": "Bearer " + auth.Token,
		"Content-Type":  "application/json",
	}
	resp, err := fetchJSON(ctx, "POST", "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota", hdrs, "{}")
	if err != nil {
		return errSnapshot(pid, err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errSnapshot(pid, httpErr(resp.StatusCode)), nil
	}
	var data struct {
		Buckets []struct {
			ModelID           string   `json:"modelId"`
			RemainingFraction *float64 `json:"remainingFraction"`
		} `json:"buckets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return errSnapshot(pid, "parse error"), nil
	}
	quotas := map[string]float64{}
	for _, b := range data.Buckets {
		model := b.ModelID
		if model == "" {
			model = "unknown"
		}
		frac := 1.0
		if b.RemainingFraction != nil {
			frac = *b.RemainingFraction
		}
		if v, ok := quotas[model]; !ok || frac < v {
			quotas[model] = frac
		}
	}
	var proMin, flashMin float64 = 1, 1
	var hasPro, hasFlash bool
	for model, frac := range quotas {
		lower := strings.ToLower(model)
		if strings.Contains(lower, "pro") {
			hasPro = true
			if frac < proMin {
				proMin = frac
			}
		}
		if strings.Contains(lower, "flash") {
			hasFlash = true
			if frac < flashMin {
				flashMin = frac
			}
		}
	}
	var ws []ProviderUsageWindow
	if hasPro {
		ws = append(ws, ProviderUsageWindow{Label: "Pro", UsedPercent: ClampPercent((1 - proMin) * 100)})
	}
	if hasFlash {
		ws = append(ws, ProviderUsageWindow{Label: "Flash", UsedPercent: ClampPercent((1 - flashMin) * 100)})
	}
	return okSnapshot(pid, ws, ""), nil
}
