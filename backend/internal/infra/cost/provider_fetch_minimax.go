// provider_fetch_minimax.go — MiniMax 使用量。
//
// TS 对照: provider-usage.fetch.minimax.ts (401L)
// 使用启发式解析任意 API 响应形状。
package cost

import (
	"context"
	"encoding/json"
	"math"
)

func minimaxUsageHeaders(auth ProviderAuth) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + auth.Token,
		"Content-Type":  "application/json",
		"MM-API-Source": "OpenAcosmi",
	}
}

func fetchMinimaxUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	hdrs := minimaxUsageHeaders(auth)
	resp, err := fetchJSON(ctx, "GET", "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains", hdrs, "")
	if err != nil {
		return errSnapshot(ProviderMinimax, err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errSnapshot(ProviderMinimax, httpErr(resp.StatusCode)), nil
	}
	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return errSnapshot(ProviderMinimax, "Invalid JSON"), nil
	}
	if br, ok := raw["base_resp"].(map[string]interface{}); ok {
		if code, _ := br["status_code"].(float64); code != 0 {
			msg, _ := br["status_msg"].(string)
			if msg == "" {
				msg = "API error"
			}
			return errSnapshot(ProviderMinimax, msg), nil
		}
	}
	payload := raw
	if d, ok := raw["data"].(map[string]interface{}); ok {
		payload = d
	}
	pct := deriveUsedPercent(payload)
	if pct < 0 {
		return errSnapshot(ProviderMinimax, "Unsupported response shape"), nil
	}
	ws := []ProviderUsageWindow{{Label: "5h", UsedPercent: pct}}
	plan := pickStr(payload, "plan", "plan_name", "planName", "product", "tier")
	return okSnapshot(ProviderMinimax, ws, plan), nil
}

func deriveUsedPercent(m map[string]interface{}) float64 {
	pctKeys := []string{"used_percent", "usedPercent", "usage_percent", "usage_rate", "used_rate"}
	for _, k := range pctKeys {
		if v, ok := numVal(m[k]); ok {
			if v <= 1 {
				return ClampPercent(v * 100)
			}
			return ClampPercent(v)
		}
	}
	totalKeys := []string{"total", "total_amount", "totalAmount", "limit", "quota", "max"}
	usedKeys := []string{"used", "usage", "used_amount", "consumed"}
	total := pickNum(m, totalKeys)
	used := pickNum(m, usedKeys)
	if total > 0 && !math.IsNaN(used) {
		return ClampPercent((used / total) * 100)
	}
	return -1
}

func pickNum(m map[string]interface{}, keys []string) float64 {
	for _, k := range keys {
		if v, ok := numVal(m[k]); ok {
			return v
		}
	}
	return math.NaN()
}

func numVal(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case int:
		return float64(n), true
	}
	return 0, false
}

func pickStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
