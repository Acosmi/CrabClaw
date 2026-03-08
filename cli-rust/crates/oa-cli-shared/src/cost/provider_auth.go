// provider_auth.go — 供应商认证解析。
//
// TS 对照: infra/provider-usage.auth.ts (289L) — 全量对齐
//
// BW1-D3: 提取 AuthProfileReader 接口，支持将来替换为加密存储或 OAuth 刷新模块。
package cost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AuthProfileReader auth-profiles 读取接口。
// 默认实现为 JSON 文件解析，后续可替换为加密存储/OAuth 刷新实现。
type AuthProfileReader interface {
	ReadToken(agentDir, provider string) string
}

// DefaultAuthProfileReader 可替换的全局读取器实例。
var DefaultAuthProfileReader AuthProfileReader = defaultAuthProfileReader{}

// defaultAuthProfileReader JSON 文件解析实现。
type defaultAuthProfileReader struct{}

// ProviderAuth 供应商认证信息。
type ProviderAuth struct {
	Provider  UsageProviderId `json:"provider"`
	Token     string          `json:"token,omitempty"`
	AccountID string          `json:"accountId,omitempty"`
}

// ResolveProviderAuths 批量解析供应商认证。
// TS 对照: provider-usage.auth.ts resolveProviderAuths()
func ResolveProviderAuths(providers []UsageProviderId, agentDir string) []ProviderAuth {
	var auths []ProviderAuth
	for _, p := range providers {
		auth := resolveProviderAuth(p, agentDir)
		if auth != nil {
			auths = append(auths, *auth)
		}
	}
	return auths
}

func resolveProviderAuth(provider UsageProviderId, agentDir string) *ProviderAuth {
	switch provider {
	case ProviderAnthropic:
		return resolveAnthropicAuth(agentDir)
	case ProviderCopilot:
		return resolveCopilotAuth()
	case ProviderGeminiCLI:
		return resolveGeminiAuth(provider)
	case ProviderAntigravity:
		return resolveGeminiAuth(provider)
	case ProviderOpenAICodex:
		return resolveCodexAuth()
	case ProviderMinimax:
		return resolveMinimaxAuth(agentDir)
	case ProviderXiaomi:
		return resolveXiaomiAuth(agentDir)
	case ProviderZai:
		return resolveZaiAuth(agentDir)
	default:
		return nil
	}
}

func resolveAnthropicAuth(agentDir string) *ProviderAuth {
	// 1. 环境变量
	if key := envTrim("ANTHROPIC_API_KEY"); key != "" {
		return &ProviderAuth{Provider: ProviderAnthropic, Token: key}
	}
	// 2. auth-profiles.json
	if token := readAuthProfile(agentDir, "anthropic"); token != "" {
		return &ProviderAuth{Provider: ProviderAnthropic, Token: token}
	}
	return nil
}

func resolveCopilotAuth() *ProviderAuth {
	if token := envTrim("GITHUB_TOKEN"); token != "" {
		return &ProviderAuth{Provider: ProviderCopilot, Token: token}
	}
	return nil
}

func resolveGeminiAuth(provider UsageProviderId) *ProviderAuth {
	for _, k := range []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"} {
		if v := envTrim(k); v != "" {
			return &ProviderAuth{Provider: provider, Token: v}
		}
	}
	return nil
}

func resolveCodexAuth() *ProviderAuth {
	if token := envTrim("OPENAI_API_KEY"); token != "" {
		accountID := envTrim("OPENAI_ACCOUNT_ID")
		return &ProviderAuth{Provider: ProviderOpenAICodex, Token: token, AccountID: accountID}
	}
	return nil
}

func resolveMinimaxAuth(agentDir string) *ProviderAuth {
	if key := envTrim("MINIMAX_API_KEY"); key != "" {
		return &ProviderAuth{Provider: ProviderMinimax, Token: key}
	}
	if token := readAuthProfile(agentDir, "minimax"); token != "" {
		return &ProviderAuth{Provider: ProviderMinimax, Token: token}
	}
	return nil
}

func resolveXiaomiAuth(agentDir string) *ProviderAuth {
	if key := envTrim("XIAOMI_API_KEY"); key != "" {
		return &ProviderAuth{Provider: ProviderXiaomi, Token: key}
	}
	if token := readAuthProfile(agentDir, "xiaomi"); token != "" {
		return &ProviderAuth{Provider: ProviderXiaomi, Token: token}
	}
	return nil
}

func resolveZaiAuth(agentDir string) *ProviderAuth {
	if key := envTrim("ZAI_API_KEY"); key != "" {
		return &ProviderAuth{Provider: ProviderZai, Token: key}
	}
	if token := readAuthProfile(agentDir, "zai"); token != "" {
		return &ProviderAuth{Provider: ProviderZai, Token: token}
	}
	return nil
}

// ---------- 辅助 ----------

func envTrim(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// readAuthProfile 通过 DefaultAuthProfileReader 读取 auth profile token。
func readAuthProfile(agentDir, provider string) string {
	return DefaultAuthProfileReader.ReadToken(agentDir, provider)
}

// ReadToken 从 auth-profiles.json 解析 token。
func (d defaultAuthProfileReader) ReadToken(agentDir, provider string) string {
	if agentDir == "" {
		return ""
	}
	path := filepath.Join(agentDir, "auth-profiles.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var profiles map[string]interface{}
	if err := json.Unmarshal(data, &profiles); err != nil {
		return ""
	}
	entry, ok := profiles[provider].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, key := range []string{"token", "apiKey", "api_key"} {
		if v, ok := entry[key].(string); ok && v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
