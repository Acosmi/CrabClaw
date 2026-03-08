package gateway

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// ---------- Hooks 配置 ----------

const (
	// DefaultHooksPath 默认 hooks 路径前缀。
	DefaultHooksPath = "/hooks"
	// DefaultHooksMaxBodyBytes 默认 hooks 请求体大小上限 (256KB)。
	DefaultHooksMaxBodyBytes int64 = 256 * 1024
)

// HooksConfig 解析后的 Hooks 配置。
type HooksConfig struct {
	BasePath     string
	Token        string
	MaxBodyBytes int64
	Mappings     []HookMappingResolved
}

// HookMessageChannel 频道标识。
type HookMessageChannel string

// HookWakePayload wake hook 载荷。
type HookWakePayload struct {
	Text string `json:"text"`
	Mode string `json:"mode"` // "now" | "next-heartbeat"
}

// HookAgentPayload agent hook 载荷。
type HookAgentPayload struct {
	Message                    string             `json:"message"`
	Name                       string             `json:"name"`
	WakeMode                   string             `json:"wakeMode"` // "now" | "next-heartbeat"
	SessionKey                 string             `json:"sessionKey"`
	Deliver                    bool               `json:"deliver"`
	Channel                    HookMessageChannel `json:"channel"`
	To                         string             `json:"to,omitempty"`
	Model                      string             `json:"model,omitempty"`
	Thinking                   string             `json:"thinking,omitempty"`
	TimeoutSeconds             int                `json:"timeoutSeconds,omitempty"`
	AllowUnsafeExternalContent bool               `json:"allowUnsafeExternalContent,omitempty"` // H-R3
}

// ---------- Hooks 配置解析 ----------

// HooksRawConfig 原始 hooks 配置 (从 JSON/YAML 解析)。
type HooksRawConfig struct {
	Enabled       *bool               `json:"enabled,omitempty"`
	Token         string              `json:"token,omitempty"`
	Path          string              `json:"path,omitempty"`
	MaxBodyBytes  int64               `json:"maxBodyBytes,omitempty"`
	Presets       []string            `json:"presets,omitempty"`
	Mappings      []HookMappingConfig `json:"mappings,omitempty"`
	TransformsDir string              `json:"transformsDir,omitempty"`
	Gmail         *HooksGmailConfig   `json:"gmail,omitempty"`
}

// HooksGmailConfig Gmail hooks 配置。
type HooksGmailConfig struct {
	AllowUnsafeExternalContent *bool `json:"allowUnsafeExternalContent,omitempty"`
}

// ResolveHooksConfig 从配置解析 Hooks 设置。
// 返回 nil 表示 hooks 未启用。
func ResolveHooksConfig(raw *HooksRawConfig) (*HooksConfig, error) {
	if raw == nil || raw.Enabled == nil || !*raw.Enabled {
		return nil, nil
	}
	token := strings.TrimSpace(raw.Token)
	if token == "" {
		return nil, fmt.Errorf("hooks.enabled requires hooks.token")
	}

	basePath := strings.TrimSpace(raw.Path)
	if basePath == "" {
		basePath = DefaultHooksPath
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	// 去尾斜线 (除非就是 "/")
	if len(basePath) > 1 {
		basePath = strings.TrimRight(basePath, "/")
	}
	if basePath == "/" {
		return nil, fmt.Errorf("hooks.path may not be '/'")
	}

	maxBodyBytes := raw.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = DefaultHooksMaxBodyBytes
	}

	mappings := ResolveHookMappings(raw)
	return &HooksConfig{
		BasePath:     basePath,
		Token:        token,
		MaxBodyBytes: maxBodyBytes,
		Mappings:     mappings,
	}, nil
}

// ---------- Token 提取 ----------

// ExtractHookToken 从请求中提取 Hook token。
// 优先从 Authorization: Bearer 头提取，其次从 X-CrabClaw-Token，再回退 X-OpenAcosmi-Token。
func ExtractHookToken(r *http.Request) string {
	return GetGatewayToken(r)
}

// ---------- Header 标准化 ----------

// NormalizeHookHeaders 将请求 header 统一为小写 key、单值 map。
func NormalizeHookHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(r.Header))
	for key, values := range r.Header {
		lk := strings.ToLower(key)
		if len(values) == 1 {
			headers[lk] = values[0]
		} else if len(values) > 1 {
			headers[lk] = strings.Join(values, ", ")
		}
	}
	return headers
}

// ---------- Payload 验证 ----------

// NormalizeWakePayload 验证并标准化 wake hook 载荷。
func NormalizeWakePayload(payload map[string]interface{}) (*HookWakePayload, error) {
	textRaw, _ := payload["text"].(string)
	text := strings.TrimSpace(textRaw)
	if text == "" {
		return nil, fmt.Errorf("text required")
	}
	mode := "now"
	if modeRaw, _ := payload["mode"].(string); modeRaw == "next-heartbeat" {
		mode = "next-heartbeat"
	}
	return &HookWakePayload{Text: text, Mode: mode}, nil
}

// NormalizeAgentPayload 验证并标准化 agent hook 载荷。
func NormalizeAgentPayload(payload map[string]interface{}) (*HookAgentPayload, error) {
	messageRaw, _ := payload["message"].(string)
	message := strings.TrimSpace(messageRaw)
	if message == "" {
		return nil, fmt.Errorf("message required")
	}

	nameRaw, _ := payload["name"].(string)
	name := strings.TrimSpace(nameRaw)
	if name == "" {
		name = "Hook"
	}

	wakeMode := "now"
	if wm, _ := payload["wakeMode"].(string); wm == "next-heartbeat" {
		wakeMode = "next-heartbeat"
	}

	sessionKeyRaw, _ := payload["sessionKey"].(string)
	sessionKey := strings.TrimSpace(sessionKeyRaw)
	if sessionKey == "" {
		sessionKey = "hook:" + uuid.New().String()
	}

	// channel (H-1: 验证 channel 值)
	channel := HookMessageChannel("last")
	if chRaw, ok := payload["channel"].(string); ok {
		ch := strings.TrimSpace(chRaw)
		if ch != "" {
			if err := validateHookChannel(ch); err != nil {
				return nil, err
			}
			channel = HookMessageChannel(ch)
		}
	}

	deliver := true
	if d, ok := payload["deliver"].(bool); ok {
		deliver = d
	}

	toRaw, _ := payload["to"].(string)
	to := strings.TrimSpace(toRaw)

	modelRaw, _ := payload["model"].(string)
	model := strings.TrimSpace(modelRaw)
	if payload["model"] != nil && model == "" {
		return nil, fmt.Errorf("model required")
	}

	thinkingRaw, _ := payload["thinking"].(string)
	thinking := strings.TrimSpace(thinkingRaw)

	timeoutSeconds := 0
	if ts, ok := payload["timeoutSeconds"].(float64); ok && ts > 0 {
		timeoutSeconds = int(ts)
	}

	allowUnsafe := false
	if v, ok := payload["allowUnsafeExternalContent"].(bool); ok {
		allowUnsafe = v
	}

	return &HookAgentPayload{
		Message:                    message,
		Name:                       name,
		WakeMode:                   wakeMode,
		SessionKey:                 sessionKey,
		Deliver:                    deliver,
		Channel:                    channel,
		To:                         to,
		Model:                      model,
		Thinking:                   thinking,
		TimeoutSeconds:             timeoutSeconds,
		AllowUnsafeExternalContent: allowUnsafe,
	}, nil
}

// validHookChannels 允许的 channel 值 (H-1)。
// TS 参考: types.hooks.ts L23-32
var validHookChannels = map[string]bool{
	"last":       true,
	"new":        true,
	"background": true,
	// TS 定义的 channel 名（与网关通道插件对齐）
	"whatsapp":   true,
	"telegram":   true,
	"discord":    true,
	"googlechat": true,
	"slack":      true,
	"signal":     true,
	"imessage":   true,
	"msteams":    true,
}

// RegisterHookChannel 动态注册 hook channel（供通道插件调用）。
func RegisterHookChannel(name string) {
	validHookChannels[name] = true
}

// validateHookChannel 验证 hook channel 值是否合法。
func validateHookChannel(ch string) error {
	if !validHookChannels[ch] {
		return fmt.Errorf("invalid channel %q", ch)
	}
	return nil
}
