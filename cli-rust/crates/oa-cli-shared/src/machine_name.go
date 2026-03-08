package infra

// machine_name.go — 机器显示名获取
// 对应 TS: src/infra/machine-name.ts (52L)
//
// macOS: scutil --get ComputerName → LocalHostName → os.Hostname()
// Linux/Windows: os.Hostname() 直接使用
// 结果以 sync.Once 缓存。

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	machineNameOnce   sync.Once
	machineNameCached string
)

// GetMachineDisplayName 获取机器的人类可读显示名称。
// 对应 TS: getMachineDisplayName()
//
// macOS 优先使用 scutil --get ComputerName（Finder 中显示的名称），
// 回退到 LocalHostName，最终回退到 os.Hostname()。
// 结果缓存，仅首次调用时执行外部命令。
func GetMachineDisplayName() string {
	machineNameOnce.Do(func() {
		machineNameCached = resolveMachineName()
	})
	return machineNameCached
}

func resolveMachineName() string {
	if os.Getenv("VITEST") != "" || os.Getenv("GO_TEST") != "" {
		return fallbackHostName()
	}

	if runtime.GOOS == "darwin" {
		if name := tryScutil("ComputerName"); name != "" {
			return name
		}
		if name := tryScutil("LocalHostName"); name != "" {
			return name
		}
	}

	return fallbackHostName()
}

// tryScutil 尝试通过 macOS scutil 命令获取系统名称。
func tryScutil(key string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--get", key).Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return ""
	}
	return value
}

func fallbackHostName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "openacosmi"
	}
	// 移除 .local 后缀（macOS 常见）
	hostname = strings.TrimSuffix(hostname, ".local")
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return "openacosmi"
	}
	return hostname
}

// ResetMachineNameForTest 重置缓存（仅测试使用）。
func ResetMachineNameForTest() {
	machineNameOnce = sync.Once{}
	machineNameCached = ""
}
