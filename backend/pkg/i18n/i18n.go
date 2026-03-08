// Package i18n 提供真正的国际化支持（非翻译工具）。
//
// 设计理念：
//   - 默认语言：中文 (zh-CN)
//   - 通过语言包切换，而非在线翻译
//   - 代码标识符保持英文，用户可见文本使用对应语言
//   - 支持带参数的消息模板
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Lang 语言标识
type Lang string

const (
	// LangZhCN 中文（默认）
	LangZhCN Lang = "zh-CN"
	// LangEnUS 英文
	LangEnUS Lang = "en-US"
)

// 全局语言管理器
var (
	current Lang
	mu      sync.RWMutex
	bundles = map[Lang]map[string]string{}
)

func init() {
	// 注册中文语言包
	bundles[LangZhCN] = map[string]string{
		"app.starting":          "Crab Claw（蟹爪） v{version} 正在启动...",
		"app.shutdown":          "Crab Claw（蟹爪） 已关闭",
		"app.gateway.listening": "网关服务器已启动，监听端口 {port}",
		"app.gateway.stopping":  "正在停止网关服务器...",
		"app.config.loaded":     "配置已加载: {path}",
		"app.config.error":      "配置加载失败: {error}",
		"error.not_found":       "未找到: {item}",
		"error.unauthorized":    "认证失败: {reason}",
		"error.internal":        "内部错误: {detail}",
		"agent.session.created": "会话已创建: {id}",
		"agent.session.ended":   "会话已结束: {id}",
		"agent.tool.executing":  "正在执行工具: {name}",
		"agent.tool.completed":  "工具执行完成: {name}",
		"channel.connected":     "频道已连接: {name}",
		"channel.disconnected":  "频道已断开: {name}",
		"channel.message.recv":  "收到消息: {channel} <- {sender}",
		"channel.message.sent":  "发送消息: {channel} -> {recipient}",
	}

	// 注册英文语言包
	bundles[LangEnUS] = map[string]string{
		"app.starting":          "Crab Claw（蟹爪） v{version} starting...",
		"app.shutdown":          "Crab Claw（蟹爪） has been shut down",
		"app.gateway.listening": "Gateway server started, listening on port {port}",
		"app.gateway.stopping":  "Stopping gateway server...",
		"app.config.loaded":     "Configuration loaded: {path}",
		"app.config.error":      "Configuration load failed: {error}",
		"error.not_found":       "Not found: {item}",
		"error.unauthorized":    "Authentication failed: {reason}",
		"error.internal":        "Internal error: {detail}",
		"agent.session.created": "Session created: {id}",
		"agent.session.ended":   "Session ended: {id}",
		"agent.tool.executing":  "Executing tool: {name}",
		"agent.tool.completed":  "Tool execution completed: {name}",
		"channel.connected":     "Channel connected: {name}",
		"channel.disconnected":  "Channel disconnected: {name}",
		"channel.message.recv":  "Message received: {channel} <- {sender}",
		"channel.message.sent":  "Message sent: {channel} -> {recipient}",
	}
}

// Init 初始化语言设置
func Init(lang Lang) {
	mu.Lock()
	defer mu.Unlock()
	current = lang
}

// InitFromEnv 从环境变量自动检测并设置语言。
// 优先级: CRABCLAW_LANG > OPENACOSMI_LANG > LC_ALL > LANG > 默认 zh-CN。
func InitFromEnv() {
	for _, env := range []string{"CRABCLAW_LANG", "OPENACOSMI_LANG", "LC_ALL", "LANG"} {
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		vLower := strings.ToLower(v)
		if strings.Contains(vLower, "en") {
			Init(LangEnUS)
			return
		}
		if strings.Contains(vLower, "zh") {
			Init(LangZhCN)
			return
		}
	}
	Init(LangZhCN) // 默认中文
}

// SetLang 切换当前语言
func SetLang(lang Lang) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := bundles[lang]; !ok {
		return // 不支持的语言，忽略
	}
	current = lang
}

// GetLang 获取当前语言
func GetLang() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T 翻译消息键为当前语言的文本。
// 支持 {key} 格式的参数替换。
//
// 用法：
//
//	i18n.T("app.starting", map[string]string{"version": "1.0.0"})
//	// 中文: "Crab Claw（蟹爪） v1.0.0 正在启动..."
//	// 英文: "Crab Claw（蟹爪） v1.0.0 starting..."
func T(key string, params map[string]string) string {
	mu.RLock()
	lang := current
	mu.RUnlock()

	bundle, ok := bundles[lang]
	if !ok {
		bundle = bundles[LangZhCN] // 回退到中文
	}

	msg, ok := bundle[key]
	if !ok {
		return fmt.Sprintf("[missing: %s]", key)
	}

	// 参数替换
	for k, v := range params {
		msg = strings.ReplaceAll(msg, "{"+k+"}", v)
	}

	return msg
}

// Tp 无参数翻译快捷函数。
func Tp(key string) string {
	return T(key, nil)
}

// Tf 带 fmt 格式化参数的翻译快捷函数。
// 用法: i18n.Tf("onboard.found", count) → "发现了 3 个项目"
// 语言包中使用 %d %s 等 fmt 占位符。
func Tf(key string, args ...interface{}) string {
	template := Tp(key)
	if len(args) == 0 {
		return template
	}
	return fmt.Sprintf(template, args...)
}

// RegisterBundle 注册自定义语言包（允许扩展或覆盖）
func RegisterBundle(lang Lang, messages map[string]string) {
	mu.Lock()
	defer mu.Unlock()

	existing, ok := bundles[lang]
	if !ok {
		bundles[lang] = messages
		return
	}

	// 合并到现有语言包
	for k, v := range messages {
		existing[k] = v
	}
}
