package reply

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// TS 对照: auto-reply/reply/reply-elevated.ts (234L)

// ElevatedPermissionResult 提权权限解析结果。
type ElevatedPermissionResult struct {
	Enabled  bool
	Allowed  bool
	Failures []ElevatedFailure
}

// ChannelDockElevatedFallbackProvider 频道 dock 提权 fallback 回调（DI 注入）。
// 签名: (provider string, accountId string, cfgRaw interface{}) -> fallback 允许列表
// 由 gateway 启动时设置，避免 reply → channels 导入环。
var ChannelDockElevatedFallbackProvider func(provider, accountId string, cfgRaw interface{}) []interface{}

// ChannelIDNormalizerProvider 频道 ID 规范化（DI 注入）。
// 签名: (raw string) -> normalized string
// 由 gateway 启动时设置，避免 reply → channels 导入环。
var ChannelIDNormalizerProvider func(raw string) string

// ---------- 内部辅助函数 ----------

// normalizeAllowToken 规范化 allow-list token。
// TS 对照: reply-elevated.ts L10-15
func normalizeAllowToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// slugAllowToken 生成 allow-list token 的 slug 形式。
// TS 对照: reply-elevated.ts L17-29
func slugAllowToken(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return ""
	}
	// 移除前导 @#
	text = slugLeadingSymbolsRe.ReplaceAllString(text, "")
	// 空格/下划线 → -
	text = slugWhitespaceRe.ReplaceAllString(text, "-")
	// 非字母数字 → -
	text = slugNonAlnumRe.ReplaceAllString(text, "-")
	// 合并连续 -
	text = slugMultiDashRe.ReplaceAllString(text, "-")
	// 去首尾 -
	text = strings.Trim(text, "-")
	return text
}

var (
	slugLeadingSymbolsRe = regexp.MustCompile(`^[@#]+`)
	slugWhitespaceRe     = regexp.MustCompile(`[\s_]+`)
	slugNonAlnumRe       = regexp.MustCompile(`[^a-z0-9-]+`)
	slugMultiDashRe      = regexp.MustCompile(`-{2,}`)
)

// senderPrefixRe 移除常见的 sender 前缀。
// TS 对照: reply-elevated.ts L31-46
// 注意：TS 动态引入 CHAT_CHANNEL_ORDER + INTERNAL_MESSAGE_CHANNEL，
// 此处硬编码已知前缀以避免运行时 DI 复杂性。
var senderPrefixRe = regexp.MustCompile(
	`(?i)^(telegram|whatsapp|discord|slack|signal|imessage|google-chat|line|msteams|web|internal|user|group|channel):`,
)

// stripSenderPrefix 移除 sender 前缀（如 "telegram:", "user:"）。
func stripSenderPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return senderPrefixRe.ReplaceAllString(trimmed, "")
}

// resolveElevatedAllowList 从 allowFrom 配置中按 provider key 查询允许列表。
// TS 对照: reply-elevated.ts L48-58
func resolveElevatedAllowList(
	allowFrom types.AgentElevatedAllowFromConfig,
	provider string,
	fallback []interface{},
) []interface{} {
	if allowFrom == nil {
		return fallback
	}
	value, ok := allowFrom[provider]
	if !ok || len(value) == 0 {
		return fallback
	}
	return value
}

// isApprovedElevatedSender 检查当前发送者是否在提权允许列表中。
// 使用多 token 匹配：SenderName/SenderUsername/SenderE164/From/To + normalize/slug/stripPrefix 变体。
// TS 对照: reply-elevated.ts L60-132
func isApprovedElevatedSender(
	provider string,
	ctx *autoreply.MsgContext,
	allowFrom types.AgentElevatedAllowFromConfig,
	fallbackAllowFrom []interface{},
) bool {
	rawAllow := resolveElevatedAllowList(allowFrom, provider, fallbackAllowFrom)
	if len(rawAllow) == 0 {
		return false
	}

	// 构建 allowTokens
	var allowTokens []string
	for _, entry := range rawAllow {
		s := strings.TrimSpace(fmt.Sprintf("%v", entry))
		if s != "" {
			allowTokens = append(allowTokens, s)
		}
	}
	if len(allowTokens) == 0 {
		return false
	}

	// 通配符检查
	for _, t := range allowTokens {
		if t == "*" {
			return true
		}
	}

	// 构建 sender token 集合
	tokens := make(map[string]struct{})
	addToken := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		tokens[trimmed] = struct{}{}
		normalized := normalizeAllowToken(trimmed)
		if normalized != "" {
			tokens[normalized] = struct{}{}
		}
		slugged := slugAllowToken(trimmed)
		if slugged != "" {
			tokens[slugged] = struct{}{}
		}
	}

	addToken(ctx.SenderName)
	addToken(ctx.SenderUsername)
	// SenderTag — Go MsgContext 暂无此字段，跳过
	addToken(ctx.SenderE164)
	addToken(ctx.From)
	addToken(stripSenderPrefix(ctx.From))
	addToken(ctx.To)
	addToken(stripSenderPrefix(ctx.To))

	// 匹配
	for _, rawEntry := range allowTokens {
		entry := strings.TrimSpace(rawEntry)
		if entry == "" {
			continue
		}
		stripped := stripSenderPrefix(entry)
		if _, ok := tokens[entry]; ok {
			return true
		}
		if _, ok := tokens[stripped]; ok {
			return true
		}
		normalized := normalizeAllowToken(stripped)
		if normalized != "" {
			if _, ok := tokens[normalized]; ok {
				return true
			}
		}
		slugged := slugAllowToken(stripped)
		if slugged != "" {
			if _, ok := tokens[slugged]; ok {
				return true
			}
		}
	}

	return false
}

// ResolveElevatedPermissions 解析提权权限（双层检查：global → agent 级别）。
// TS 对照: reply-elevated.ts L134-203
func ResolveElevatedPermissions(
	cfg *types.OpenAcosmiConfig,
	agentId string,
	ctx *autoreply.MsgContext,
	provider string,
) ElevatedPermissionResult {
	// 全局配置
	var globalConfig *types.ToolsElevatedConfig
	if cfg.Tools != nil {
		globalConfig = cfg.Tools.Elevated
	}

	// Agent 配置
	var agentElevatedConfig *types.AgentToolsElevatedConfig
	ac := scope.ResolveAgentConfig(cfg, agentId)
	if ac != nil && ac.Tools != nil {
		agentElevatedConfig = ac.Tools.Elevated
	}

	// 启用检查
	globalEnabled := globalConfig == nil || globalConfig.Enabled == nil || *globalConfig.Enabled
	agentEnabled := agentElevatedConfig == nil || agentElevatedConfig.Enabled == nil || *agentElevatedConfig.Enabled
	enabled := globalEnabled && agentEnabled

	var failures []ElevatedFailure
	if !globalEnabled {
		failures = append(failures, ElevatedFailure{Gate: "enabled", Key: "tools.elevated.enabled"})
	}
	if !agentEnabled {
		failures = append(failures, ElevatedFailure{Gate: "enabled", Key: "agents.list[].tools.elevated.enabled"})
	}
	if !enabled {
		return ElevatedPermissionResult{Enabled: enabled, Allowed: false, Failures: failures}
	}

	// provider 检查
	if provider == "" {
		failures = append(failures, ElevatedFailure{Gate: "provider", Key: "ctx.Provider"})
		return ElevatedPermissionResult{Enabled: enabled, Allowed: false, Failures: failures}
	}

	// dock fallback 查询（通过 DI 避免导入环）
	var fallbackAllowFrom []interface{}
	if ChannelDockElevatedFallbackProvider != nil && ChannelIDNormalizerProvider != nil {
		normalizedProvider := ChannelIDNormalizerProvider(provider)
		if normalizedProvider != "" {
			fallbackAllowFrom = ChannelDockElevatedFallbackProvider(normalizedProvider, ctx.AccountID, cfg)
		}
	}

	// 全局 allowFrom 检查
	var globalAllowFrom types.AgentElevatedAllowFromConfig
	if globalConfig != nil {
		globalAllowFrom = globalConfig.AllowFrom
	}
	globalAllowed := isApprovedElevatedSender(provider, ctx, globalAllowFrom, fallbackAllowFrom)
	if !globalAllowed {
		failures = append(failures, ElevatedFailure{
			Gate: "allowFrom",
			Key:  fmt.Sprintf("tools.elevated.allowFrom.%s", provider),
		})
		return ElevatedPermissionResult{Enabled: enabled, Allowed: false, Failures: failures}
	}

	// Agent allowFrom 检查
	agentAllowed := true
	if agentElevatedConfig != nil && agentElevatedConfig.AllowFrom != nil {
		agentAllowed = isApprovedElevatedSender(provider, ctx, agentElevatedConfig.AllowFrom, fallbackAllowFrom)
	}
	if !agentAllowed {
		failures = append(failures, ElevatedFailure{
			Gate: "allowFrom",
			Key:  fmt.Sprintf("agents.list[].tools.elevated.allowFrom.%s", provider),
		})
	}

	return ElevatedPermissionResult{
		Enabled:  enabled,
		Allowed:  globalAllowed && agentAllowed,
		Failures: failures,
	}
}

// FormatElevatedUnavailableMessage 格式化提权不可用消息。
// TS 对照: reply-elevated.ts L206-233
func FormatElevatedUnavailableMessage(runtimeSandboxed bool, failures []ElevatedFailure, sessionKey string) string {
	var lines []string

	runtime := "direct"
	if runtimeSandboxed {
		runtime = "sandboxed"
	}
	lines = append(lines, fmt.Sprintf("elevated is not available right now (runtime=%s).", runtime))

	if len(failures) > 0 {
		var parts []string
		for _, f := range failures {
			parts = append(parts, fmt.Sprintf("%s (%s)", f.Gate, f.Key))
		}
		lines = append(lines, "Failing gates: "+strings.Join(parts, ", "))
	} else {
		lines = append(lines, "Failing gates: enabled (tools.elevated.enabled / agents.list[].tools.elevated.enabled), allowFrom (tools.elevated.allowFrom.<provider>).")
	}

	lines = append(lines, "Fix-it keys:")
	lines = append(lines, "- tools.elevated.enabled")
	lines = append(lines, "- tools.elevated.allowFrom.<provider>")
	lines = append(lines, "- agents.list[].tools.elevated.enabled")
	lines = append(lines, "- agents.list[].tools.elevated.allowFrom.<provider>")

	if sessionKey != "" {
		lines = append(lines, fmt.Sprintf("See: `crabclaw sandbox explain --session %s`", sessionKey))
	}

	return strings.Join(lines, "\n")
}
