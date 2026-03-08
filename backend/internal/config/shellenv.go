package config

// Shell 环境变量回退 — 对应 src/infra/shell-env.ts (173 行)
//
// 当进程不是从终端启动时（如 macOS GUI 启动），用户的 .bashrc/.zshrc 中设置的
// 环境变量可能不存在。此模块通过执行 `$SHELL -l -c "env -0"` 从登录 shell
// 中读取环境变量并应用缺失的 key。
//
// 依赖:
//   - os/exec (替代 Node.js child_process.execFileSync)
//   - pkg/utils.IsTruthy (替代 isTruthyEnvValue)
//
// 注意: 此功能在 Windows 上不可用。

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/utils"
)

const (
	defaultShellEnvTimeoutMs  = 15000
	defaultShellMaxBufferSize = 2 * 1024 * 1024 // 2MB
)

var (
	shellEnvAppliedKeys []string
	shellEnvMu          sync.Mutex
	cachedShellPath     *string // nil = not cached, non-nil = cached (empty string = not found)
	shellPathMu         sync.Mutex
)

// ShellEnvFallbackResult 回退结果
type ShellEnvFallbackResult struct {
	OK            bool
	Applied       []string
	SkippedReason string // "already-has-keys" | "disabled" | ""
	Error         string
}

// ShellEnvFallbackOptions 回退选项
type ShellEnvFallbackOptions struct {
	Enabled      bool
	ExpectedKeys []string
	TimeoutMs    int
	Logger       interface {
		Warn(msg string, args ...interface{})
	}
}

// LoadShellEnvFallback 从登录 shell 加载环境变量
// 对应 TS: loadShellEnvFallback(opts)
func LoadShellEnvFallback(opts ShellEnvFallbackOptions) ShellEnvFallbackResult {
	shellEnvMu.Lock()
	defer shellEnvMu.Unlock()

	if !opts.Enabled {
		shellEnvAppliedKeys = nil
		return ShellEnvFallbackResult{OK: true, SkippedReason: "disabled"}
	}

	// 检查是否已有预期的 key
	hasAnyKey := false
	for _, key := range opts.ExpectedKeys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			hasAnyKey = true
			break
		}
	}
	if hasAnyKey {
		shellEnvAppliedKeys = nil
		return ShellEnvFallbackResult{OK: true, SkippedReason: "already-has-keys"}
	}

	timeout := opts.TimeoutMs
	if timeout <= 0 {
		timeout = defaultShellEnvTimeoutMs
	}

	shell := resolveShell()

	shellEnv, err := execShellEnv(shell, time.Duration(timeout)*time.Millisecond)
	if err != nil {
		msg := err.Error()
		if opts.Logger != nil {
			opts.Logger.Warn("[openacosmi] shell env fallback failed", "error", msg)
		}
		shellEnvAppliedKeys = nil
		return ShellEnvFallbackResult{OK: false, Error: msg}
	}

	var applied []string
	for _, key := range opts.ExpectedKeys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			continue
		}
		if val, ok := shellEnv[key]; ok && strings.TrimSpace(val) != "" {
			os.Setenv(key, val)
			applied = append(applied, key)
		}
	}

	shellEnvAppliedKeys = applied
	return ShellEnvFallbackResult{OK: true, Applied: applied}
}

// ShouldEnableShellEnvFallback 检查是否应启用 shell 环境回退
func ShouldEnableShellEnvFallback() bool {
	return utils.IsTruthy(compatEnvValue("CRABCLAW_LOAD_SHELL_ENV", "OPENACOSMI_LOAD_SHELL_ENV"))
}

// ShouldDeferShellEnvFallback 检查是否应延迟 shell 环境回退
func ShouldDeferShellEnvFallback() bool {
	return utils.IsTruthy(compatEnvValue("CRABCLAW_DEFER_SHELL_ENV_FALLBACK", "OPENACOSMI_DEFER_SHELL_ENV_FALLBACK"))
}

// ResolveShellEnvFallbackTimeoutMs 解析 shell 环境回退的超时时间
func ResolveShellEnvFallbackTimeoutMs() int {
	raw := compatEnvValue("CRABCLAW_SHELL_ENV_TIMEOUT_MS", "OPENACOSMI_SHELL_ENV_TIMEOUT_MS")
	if raw == "" {
		return defaultShellEnvTimeoutMs
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultShellEnvTimeoutMs
	}
	if parsed < 0 {
		return 0
	}
	return parsed
}

// GetShellPathFromLoginShell 从登录 shell 中获取 PATH(带缓存)
func GetShellPathFromLoginShell(timeoutMs int) string {
	shellPathMu.Lock()
	defer shellPathMu.Unlock()

	if cachedShellPath != nil {
		return *cachedShellPath
	}

	if runtime.GOOS == "windows" {
		empty := ""
		cachedShellPath = &empty
		return ""
	}

	if timeoutMs <= 0 {
		timeoutMs = defaultShellEnvTimeoutMs
	}

	shell := resolveShell()
	shellEnv, err := execShellEnv(shell, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		empty := ""
		cachedShellPath = &empty
		return ""
	}

	shellPath := strings.TrimSpace(shellEnv["PATH"])
	cachedShellPath = &shellPath
	return shellPath
}

// GetShellEnvAppliedKeys 返回上次应用的 key 列表
func GetShellEnvAppliedKeys() []string {
	shellEnvMu.Lock()
	defer shellEnvMu.Unlock()
	result := make([]string, len(shellEnvAppliedKeys))
	copy(result, shellEnvAppliedKeys)
	return result
}

// ResetShellPathCacheForTests 测试用: 重置 PATH 缓存
func ResetShellPathCacheForTests() {
	shellPathMu.Lock()
	defer shellPathMu.Unlock()
	cachedShellPath = nil
}

// ----- 内部函数 -----

// resolveShell 获取当前用户的 shell 路径
func resolveShell() string {
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func compatEnvValue(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// execShellEnv 执行登录 shell 获取环境变量
func execShellEnv(shell string, timeout time.Duration) (map[string]string, error) {
	cmd := exec.Command(shell, "-l", "-c", "env -0")
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	cmd.Stdin = nil

	// 超时控制
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return nil, exec.ErrNotFound
	}

	return parseShellEnvOutput(stdout.Bytes()), nil
}

// parseShellEnv 解析 "env -0" 输出 (NUL 分隔的 KEY=VALUE)
// 对应 TS: parseShellEnv(stdout)
func parseShellEnvOutput(data []byte) map[string]string {
	result := make(map[string]string)
	parts := bytes.Split(data, []byte{0})
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		eq := bytes.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		key := string(part[:eq])
		value := string(part[eq+1:])
		if key != "" {
			result[key] = value
		}
	}
	return result
}
