//go:build linux

// platform_wsl.go — WSL 检测 (S2-7: HIDDEN-8)

package infra

import (
	"os"
	"strings"
	"sync"
)

// isWSLEnv 通过环境变量快速检测 WSL（无 IO）。
func isWSLEnv() bool {
	return os.Getenv("WSL_INTEROP") != "" ||
		os.Getenv("WSL_DISTRO_NAME") != "" ||
		os.Getenv("WSLENV") != ""
}

var (
	wslOnce   sync.Once
	wslCached bool
)

// IsWSL 检测当前环境是否为 Windows Subsystem for Linux。
// 先通过环境变量快速路径，回退读取 /proc/sys/kernel/osrelease。
// 结果通过 sync.Once 缓存。
func IsWSL() bool {
	wslOnce.Do(func() {
		if isWSLEnv() {
			wslCached = true
			return
		}
		data, err := os.ReadFile("/proc/sys/kernel/osrelease")
		if err != nil {
			return
		}
		content := strings.ToLower(string(data))
		wslCached = strings.Contains(content, "microsoft") || strings.Contains(content, "wsl")
	})
	return wslCached
}

// IsWSL2 检测是否为 WSL 2。
// 先检查 WSL_INTEROP 环境变量（WSL2 特有），回退读取 /proc/version。
func IsWSL2() bool {
	if os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "wsl2")
}
