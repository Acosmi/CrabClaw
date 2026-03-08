// provider_fetch.go — 供应商使用量 API 路由和 HTTP 客户端。
//
// TS 对照: provider-usage.fetch.ts + provider-usage.load.ts
package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/security"
)

var httpClient = security.CreatePinnedHTTPClient(time.Duration(DefaultProviderTimeoutMs) * time.Millisecond)

// FetchProviderUsage 路由到对应供应商获取使用量。
func FetchProviderUsage(ctx context.Context, auth ProviderAuth) (*ProviderUsageSnapshot, error) {
	switch auth.Provider {
	case ProviderAnthropic:
		return fetchClaudeUsage(ctx, auth)
	case ProviderCopilot:
		return fetchCopilotUsage(ctx, auth)
	case ProviderGeminiCLI, ProviderAntigravity:
		return fetchGeminiUsage(ctx, auth)
	case ProviderOpenAICodex:
		return fetchCodexUsage(ctx, auth)
	case ProviderMinimax:
		return fetchMinimaxUsage(ctx, auth)
	case ProviderZai:
		return fetchZaiUsage(ctx, auth)
	case ProviderXiaomi:
		return &ProviderUsageSnapshot{
			Provider:    ProviderXiaomi,
			DisplayName: ProviderLabels[ProviderXiaomi],
		}, nil
	default:
		return &ProviderUsageSnapshot{
			Provider:    auth.Provider,
			DisplayName: ProviderLabels[auth.Provider],
			Error:       "Unsupported provider",
		}, nil
	}
}

// LoadProviderUsageSummary 并发加载所有供应商使用量。
func LoadProviderUsageSummary(ctx context.Context, auths []ProviderAuth) *ProviderUsageSummary {
	summary := &ProviderUsageSummary{UpdatedAt: time.Now().UnixMilli()}
	if len(auths) == 0 {
		return summary
	}
	ch := make(chan ProviderUsageSnapshot, len(auths))
	for _, a := range auths {
		go func(auth ProviderAuth) {
			snap, err := WithTimeout(ctx, func(c context.Context) (*ProviderUsageSnapshot, error) {
				return FetchProviderUsage(c, auth)
			}, DefaultProviderTimeoutMs+1000)
			if err != nil {
				ch <- ProviderUsageSnapshot{
					Provider:    auth.Provider,
					DisplayName: ProviderLabels[auth.Provider],
					Error:       "Timeout",
				}
				return
			}
			if snap != nil {
				ch <- *snap
			}
		}(a)
	}
	for range auths {
		s := <-ch
		if len(s.Windows) > 0 || !IgnoredErrors[s.Error] {
			summary.Providers = append(summary.Providers, s)
		}
	}
	return summary
}

// fetchJSON 共享 HTTP JSON 请求。
func fetchJSON(ctx context.Context, method, url string, headers map[string]string, body string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return httpClient.Do(req)
}

// readJSONBody 读取并解析 JSON 响应体。
func readJSONBody(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// errSnapshot 创建错误快照。
func errSnapshot(provider UsageProviderId, msg string) *ProviderUsageSnapshot {
	return &ProviderUsageSnapshot{
		Provider:    provider,
		DisplayName: ProviderLabels[provider],
		Error:       msg,
	}
}

// okSnapshot 创建成功快照。
func okSnapshot(provider UsageProviderId, windows []ProviderUsageWindow, plan string) *ProviderUsageSnapshot {
	return &ProviderUsageSnapshot{
		Provider:    provider,
		DisplayName: ProviderLabels[provider],
		Windows:     windows,
		Plan:        plan,
	}
}

// httpErr 格式化 HTTP 错误。
func httpErr(status int) string {
	return fmt.Sprintf("HTTP %d", status)
}
