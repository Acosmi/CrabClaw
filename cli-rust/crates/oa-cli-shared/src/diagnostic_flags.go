package infra

// diagnostic_flags.go — 诊断标志管理
// 对应 TS: src/infra/diagnostic-flags.ts (92L)
//
// 运行时 feature flags + 环境变量覆盖。
// 供调试和运维模式切换使用。

import (
	"os"
	"strings"
	"sync"
)

// DiagnosticFlag 诊断标志名称。
type DiagnosticFlag string

const (
	// FlagVerboseLogging 详细日志输出。
	FlagVerboseLogging DiagnosticFlag = "verbose_logging"
	// FlagDiagnosticsEnabled 诊断事件系统开关。
	FlagDiagnosticsEnabled DiagnosticFlag = "diagnostics_enabled"
	// FlagSlowQueryLog 慢查询日志。
	FlagSlowQueryLog DiagnosticFlag = "slow_query_log"
	// FlagDebugWebSocket WebSocket 调试日志。
	FlagDebugWebSocket DiagnosticFlag = "debug_websocket"
	// FlagDebugLLM LLM 调用调试日志。
	FlagDebugLLM DiagnosticFlag = "debug_llm"
	// FlagDebugHeartbeat 心跳调试日志。
	FlagDebugHeartbeat DiagnosticFlag = "debug_heartbeat"
	// FlagTraceRequests 请求跟踪。
	FlagTraceRequests DiagnosticFlag = "trace_requests"
	// FlagDryRun 测试模式（不实际执行副作用）。
	FlagDryRun DiagnosticFlag = "dry_run"
)

// diagFlags 全局诊断标志存储。
var diagFlags = &diagnosticFlagStore{
	flags: make(map[DiagnosticFlag]bool),
}

type diagnosticFlagStore struct {
	mu    sync.RWMutex
	flags map[DiagnosticFlag]bool
}

// SetDiagnosticFlag 设置诊断标志。
func SetDiagnosticFlag(flag DiagnosticFlag, value bool) {
	diagFlags.mu.Lock()
	defer diagFlags.mu.Unlock()
	diagFlags.flags[flag] = value
}

// GetDiagnosticFlag 获取诊断标志值。
// 优先检查环境变量 OPENACOSMI_DIAG_{FLAG}（大写）。
// 对应 TS: getDiagnosticFlag(flag)
func GetDiagnosticFlag(flag DiagnosticFlag) bool {
	// 1. 环境变量覆盖（优先）
	envKey := "OPENACOSMI_DIAG_" + strings.ToUpper(string(flag))
	if v := os.Getenv(envKey); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}

	// 2. 运行时标志
	diagFlags.mu.RLock()
	defer diagFlags.mu.RUnlock()
	return diagFlags.flags[flag]
}

// GetAllDiagnosticFlags 获取所有诊断标志的当前值。
func GetAllDiagnosticFlags() map[DiagnosticFlag]bool {
	diagFlags.mu.RLock()
	defer diagFlags.mu.RUnlock()

	allFlags := []DiagnosticFlag{
		FlagVerboseLogging, FlagDiagnosticsEnabled, FlagSlowQueryLog,
		FlagDebugWebSocket, FlagDebugLLM, FlagDebugHeartbeat,
		FlagTraceRequests, FlagDryRun,
	}

	result := make(map[DiagnosticFlag]bool, len(allFlags))
	for _, flag := range allFlags {
		// 解锁再获取（GetDiagnosticFlag 也会加锁）
		result[flag] = diagFlags.flags[flag]
	}

	// 环境变量覆盖
	for _, flag := range allFlags {
		envKey := "OPENACOSMI_DIAG_" + strings.ToUpper(string(flag))
		if v := os.Getenv(envKey); v != "" {
			result[flag] = v == "1" || strings.EqualFold(v, "true")
		}
	}

	return result
}

// InitDiagnosticFlagsFromConfig 从配置初始化诊断标志。
func InitDiagnosticFlagsFromConfig(diagnosticsEnabled bool) {
	SetDiagnosticFlag(FlagDiagnosticsEnabled, diagnosticsEnabled)
	if diagnosticsEnabled {
		SetDiagnosticsEnabled(true)
	}
}

// ResetDiagnosticFlagsForTest 重置所有标志（仅测试）。
func ResetDiagnosticFlagsForTest() {
	diagFlags.mu.Lock()
	defer diagFlags.mu.Unlock()
	diagFlags.flags = make(map[DiagnosticFlag]bool)
}
