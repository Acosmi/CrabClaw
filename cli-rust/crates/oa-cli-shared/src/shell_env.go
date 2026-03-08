package infra

// shell_env.go — Shell 环境检测
// 对应 TS: src/infra/shell-env.ts (172L)

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// GetDefaultShell 获取用户默认 Shell。
// 对应 TS: getDefaultShell()
func GetDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	if runtime.GOOS == "windows" {
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec
		}
		return "cmd.exe"
	}
	return "/bin/sh"
}

// ShellEnvResult Shell 环境变量快照。
type ShellEnvResult struct {
	Env   map[string]string `json:"env"`
	Shell string            `json:"shell"`
}

// GetShellEnv 通过启动登录 Shell 获取用户完整环境变量。
// 对应 TS: getShellEnv()
func GetShellEnv() (*ShellEnvResult, error) {
	shell := GetDefaultShell()
	if runtime.GOOS == "windows" {
		return &ShellEnvResult{
			Env:   envSliceToMap(os.Environ()),
			Shell: shell,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 启动登录 Shell 导出环境变量
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", "env")
	out, err := cmd.Output()
	if err != nil {
		// 回退到当前进程环境
		return &ShellEnvResult{
			Env:   envSliceToMap(os.Environ()),
			Shell: shell,
		}, nil
	}

	env := parseEnvOutput(string(out))
	return &ShellEnvResult{Env: env, Shell: shell}, nil
}

// IsInteractiveShell 检查当前是否在交互式 Shell 中。
func IsInteractiveShell() bool {
	return os.Getenv("PS1") != "" || os.Getenv("TERM") != ""
}

// ─── 辅助函数 ───

func envSliceToMap(envSlice []string) map[string]string {
	result := make(map[string]string, len(envSlice))
	for _, entry := range envSlice {
		idx := strings.IndexByte(entry, '=')
		if idx < 0 {
			continue
		}
		result[entry[:idx]] = entry[idx+1:]
	}
	return result
}

func parseEnvOutput(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := line[:idx]
		value := line[idx+1:]
		if key != "" {
			result[key] = value
		}
	}
	return result
}
