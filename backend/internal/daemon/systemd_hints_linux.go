//go:build linux

package daemon

import (
	"strings"
)

// TS 对照: daemon/systemd-hints.ts (30L)

// IsSystemdUnavailableDetail 检查错误详情字符串是否包含 systemd 不可用的标志性文案。
// 对应 TS: systemd-hints.ts isSystemdUnavailableDetail
func IsSystemdUnavailableDetail(detail string) bool {
	if detail == "" {
		return false
	}
	normalized := strings.ToLower(detail)
	patterns := []string{
		"systemctl --user unavailable",
		"systemctl not available",
		"not been booted with systemd",
		"failed to connect to bus",
		"systemd user services are required",
	}
	for _, pat := range patterns {
		if strings.Contains(normalized, pat) {
			return true
		}
	}
	return false
}

// RenderSystemdUnavailableHints 生成 systemd 不可用时的用户友好提示。
// 对应 TS: systemd-hints.ts renderSystemdUnavailableHints
func RenderSystemdUnavailableHints(wsl bool) []string {
	if wsl {
		return []string{
			"WSL2 needs systemd enabled: edit /etc/wsl.conf with [boot]\\nsystemd=true",
			"Then run: wsl --shutdown (from PowerShell) and reopen your distro.",
			"Verify: systemctl --user status",
		}
	}
	return []string{
		"systemd user services are unavailable; install/enable systemd or run the gateway under your supervisor.",
		"If you're in a container, run the gateway in the foreground instead of `crabclaw gateway`.",
	}
}
