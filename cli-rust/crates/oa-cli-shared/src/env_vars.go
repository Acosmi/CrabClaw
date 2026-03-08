package infra

// env_vars.go — 环境变量注册中心
// 对应 TS: 多文件分散引用的 process.env.OPENACOSMI_* 变量
//
// 集中注册 HIDDEN-1（10 个缺失环境变量）和 HIDDEN-2（4 个第三方 API Key）。
// 提供统一的辅助函数读取和解析环境变量。

import (
	"os"
	"strconv"
	"strings"
)

// ---------- HIDDEN-1: 10 个缺失环境变量 ----------

// AllowMultiGateway 检查是否允许多网关实例并发运行。
// 对应 TS: gateway-lock.ts:182 — env.OPENACOSMI_ALLOW_MULTI_GATEWAY === "1"
func AllowMultiGateway() bool {
	return isTruthyEnvValue(preferredEnvValue("CRABCLAW_ALLOW_MULTI_GATEWAY", "OPENACOSMI_ALLOW_MULTI_GATEWAY"))
}

// ConfigCacheMs 返回配置缓存 TTL 毫秒数。
// 对应 TS: config/io.ts:559-572 — resolveConfigCacheMs
// 返回 0 表示禁用缓存；-1 表示使用默认值。
func ConfigCacheMs() int {
	raw := preferredEnvValue("CRABCLAW_CONFIG_CACHE_MS", "OPENACOSMI_CONFIG_CACHE_MS")
	if raw == "" {
		return -1 // 使用默认值
	}
	if raw == "0" {
		return 0 // 禁用缓存
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	if parsed < 0 {
		return 0
	}
	return parsed
}

// SessionManagerCacheTTLMs 返回 session manager 缓存 TTL 毫秒数。
// 对应 TS: session-manager-cache.ts:14-18
// 返回 -1 表示使用默认值（45000ms）。
func SessionManagerCacheTTLMs() int {
	raw := preferredEnvValue("CRABCLAW_SESSION_MANAGER_CACHE_TTL_MS", "OPENACOSMI_SESSION_MANAGER_CACHE_TTL_MS")
	if raw == "" {
		return -1
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	if parsed <= 0 {
		return 0
	}
	return parsed
}

// UpdateInProgress 检查是否正在进行更新。
// 对应 TS: doctor-update.ts:34 — isTruthyEnvValue(process.env.OPENACOSMI_UPDATE_IN_PROGRESS)
func UpdateInProgress() bool {
	return isTruthyEnvValue(preferredEnvValue("CRABCLAW_UPDATE_IN_PROGRESS", "OPENACOSMI_UPDATE_IN_PROGRESS"))
}

// TestHandshakeTimeoutMs 返回测试用握手超时毫秒数。
// 对应 TS: server-constants.ts:22-30 — 仅在测试环境使用。
// 返回 0 表示不覆盖默认值。
func TestHandshakeTimeoutMs() int {
	raw := preferredEnvValue("CRABCLAW_TEST_HANDSHAKE_TIMEOUT_MS", "OPENACOSMI_TEST_HANDSHAKE_TIMEOUT_MS")
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

// NoRespawn 检查是否禁止进程重生。
// 对应 TS: entry.ts:35 — isTruthyEnvValue(process.env.OPENACOSMI_NO_RESPAWN)
// Go 进程不需要 Node.js 风格的 respawn，但保留此变量作为标志位。
func NoRespawn() bool {
	return isTruthyEnvValue(preferredEnvValue("CRABCLAW_NO_RESPAWN", "OPENACOSMI_NO_RESPAWN"))
}

// NodeExecHostMode 返回节点执行主机模式。
// 对应 TS: node-host/runner.ts:161
// 返回 "app" 表示强制使用 app 执行主机，空字符串表示未设置。
func NodeExecHostMode() string {
	return strings.ToLower(preferredEnvValue("CRABCLAW_NODE_EXEC_HOST", "OPENACOSMI_NODE_EXEC_HOST"))
}

// NodeExecFallbackAllowed 检查节点执行 fallback 是否被允许。
// 对应 TS: node-host/runner.ts:162-163
// 默认 true，仅当变量显式为 "0" 时返回 false。
func NodeExecFallbackAllowed() bool {
	raw := strings.ToLower(preferredEnvValue("CRABCLAW_NODE_EXEC_FALLBACK", "OPENACOSMI_NODE_EXEC_FALLBACK"))
	return raw != "0"
}

// BrowserEnabled 检查浏览器功能是否启用。
// 对应 TS: browser/config.ts:154 — cfg?.enabled ?? DEFAULT_OPENACOSMI_BROWSER_ENABLED
// 此处仅检查环境变量层级的覆盖；最终结果还需结合 config 判断。
// 返回: true=显式开启, false=显式关闭, 从 env 未设置时返回 true（默认启用）。
func BrowserEnabled() bool {
	raw := preferredEnvValue("CRABCLAW_BROWSER_ENABLED", "OPENACOSMI_BROWSER_ENABLED")
	if raw == "" {
		return true // 默认启用
	}
	return isTruthyEnvValue(raw)
}

// ---------- HIDDEN-2: 第三方 API Key 获取变量 ----------

// ZaiAPIKey 返回 z.ai API Key（优先 ZAI_API_KEY，兼容旧版 Z_AI_API_KEY）。
// 对应 TS: model-auth.ts:260 — pick("ZAI_API_KEY") ?? pick("Z_AI_API_KEY")
func ZaiAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("ZAI_API_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv("Z_AI_API_KEY"))
}

// ChutesClientID 返回 Chutes OAuth Client ID。
// 对应 TS: chutes-oauth.ts:161 — process.env.CHUTES_CLIENT_ID
func ChutesClientID() string {
	return strings.TrimSpace(os.Getenv("CHUTES_CLIENT_ID"))
}

// ChutesClientSecret 返回 Chutes OAuth Client Secret。
// 对应 TS: chutes-oauth.ts:165 — process.env.CHUTES_CLIENT_SECRET
func ChutesClientSecret() string {
	return strings.TrimSpace(os.Getenv("CHUTES_CLIENT_SECRET"))
}

// SherpaOnnxModelDir 返回 Sherpa-ONNX 模型目录路径。
// 对应 TS: media-understanding/runner.ts:331 — process.env.SHERPA_ONNX_MODEL_DIR
func SherpaOnnxModelDir() string {
	return strings.TrimSpace(os.Getenv("SHERPA_ONNX_MODEL_DIR"))
}

// WhisperCppModel 返回 Whisper.cpp 模型路径。
// 对应 TS: media-understanding/runner.ts:293 — process.env.WHISPER_CPP_MODEL
func WhisperCppModel() string {
	return strings.TrimSpace(os.Getenv("WHISPER_CPP_MODEL"))
}

// ---------- 辅助函数 ----------

// isTruthyEnvValue 判断环境变量值是否为真值。
// 对应 TS: infra/env.ts — isTruthyEnvValue
func isTruthyEnvValue(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "1" || v == "true" || v == "yes"
}
