package cli

import (
	"sync"
)

// 对应 TS src/cli/route.ts — 快速路由机制
// 审计修复项: CLI-P3-1
// 注：Go 端 Cobra 启动速度足够快，此模块保持接口对等但不做性能优化。

// RoutedCommandHandler 快速路由命令处理器。
type RoutedCommandHandler struct {
	// Run 执行路由命令，返回是否成功。
	Run func(argv []string) (bool, error)
	// LoadPlugins 是否在执行前加载插件注册表。
	LoadPlugins bool
}

var (
	routedMu       sync.RWMutex
	routedCommands = make(map[string]RoutedCommandHandler) // key: "cmd" 或 "cmd/sub"
)

// RegisterRoutedCommand 注册快速路由命令。
// path 为命令路径，最多 2 层深度（如 ["message", "send"]）。
// 对应 TS command-registry.ts registerRoutedCommand。
func RegisterRoutedCommand(path []string, handler RoutedCommandHandler) {
	key := buildRouteKey(path)
	routedMu.Lock()
	routedCommands[key] = handler
	routedMu.Unlock()
}

// TryRouteCli 尝试快速路由 CLI 命令，绕过 Cobra 完整注册。
// 返回 (true, nil) 表示命令已被路由处理，调用方应直接退出。
// 返回 (false, nil) 表示无匹配路由，调用方应走正常 Cobra 流程。
// 对应 TS route.ts tryRouteCli()。
func TryRouteCli(argv []string) (bool, error) {
	// 环境变量禁用快速路由
	if IsTruthyAnyEnv("CRABCLAW_DISABLE_ROUTE_FIRST", "OPENACOSMI_DISABLE_ROUTE_FIRST") {
		return false, nil
	}
	// --help/--version 不走快速路由
	if HasHelpOrVersion(argv) {
		return false, nil
	}

	path := GetCommandPath(argv, 2)
	if len(path) == 0 {
		return false, nil
	}

	handler, ok := findRoutedCommand(path)
	if !ok {
		return false, nil
	}

	// 加载插件（如需要）
	if handler.LoadPlugins {
		EnsurePluginRegistryLoaded()
	}

	return handler.Run(argv)
}

// findRoutedCommand 查找匹配的路由命令。
// 先尝试 2 层路径匹配，再尝试 1 层。
func findRoutedCommand(path []string) (RoutedCommandHandler, bool) {
	routedMu.RLock()
	defer routedMu.RUnlock()

	// 先尝试精确 2 层匹配
	if len(path) >= 2 {
		key := buildRouteKey(path[:2])
		if h, ok := routedCommands[key]; ok {
			return h, true
		}
	}
	// 再尝试 1 层匹配
	key := buildRouteKey(path[:1])
	if h, ok := routedCommands[key]; ok {
		return h, true
	}
	return RoutedCommandHandler{}, false
}

// buildRouteKey 构建路由键。
func buildRouteKey(path []string) string {
	if len(path) == 0 {
		return ""
	}
	if len(path) == 1 {
		return path[0]
	}
	return path[0] + "/" + path[1]
}

// ClearRoutedCommandsForTest 清空路由注册（仅用于测试）。
func ClearRoutedCommandsForTest() {
	routedMu.Lock()
	routedCommands = make(map[string]RoutedCommandHandler)
	routedMu.Unlock()
}
