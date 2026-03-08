package cli

import (
	"fmt"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// 对应 TS src/cli/program/config-guard.ts — 配置校验守卫
// 审计修复项: H2-3 (didRunDoctorConfigFlow) + H7-1 (config allowlist)

var (
	configGuardOnce   sync.Once
	configGuardResult error
)

// allowedInvalidCommands 配置无效时仍允许执行的命令。
// 对应 TS ALLOWED_INVALID_COMMANDS。
var allowedInvalidCommands = map[string]bool{
	"doctor": true,
	"logs":   true,
	"health": true,
	"help":   true,
	"status": true,
}

// allowedGatewaySubcommands gateway 子命令中配置无效时仍允许执行的。
// 对应 TS ALLOWED_INVALID_GATEWAY_SUBCOMMANDS。
var allowedGatewaySubcommands = map[string]bool{
	"status":    true,
	"probe":     true,
	"health":    true,
	"discover":  true,
	"call":      true,
	"install":   true,
	"uninstall": true,
	"start":     true,
	"stop":      true,
	"restart":   true,
}

// IsCommandAllowedWithInvalidConfig 判断命令是否在配置无效时仍可执行。
func IsCommandAllowedWithInvalidConfig(commandPath []string) bool {
	if len(commandPath) == 0 {
		return false
	}
	if allowedInvalidCommands[commandPath[0]] {
		return true
	}
	if commandPath[0] == "gateway" && len(commandPath) > 1 {
		return allowedGatewaySubcommands[commandPath[1]]
	}
	return false
}

// EnsureConfigReady 确保配置有效，否则根据 allowlist 决定是否阻断。
// 对应 TS ensureConfigReady()。
func EnsureConfigReady(commandPath []string) error {
	// allowlist 命令不需要有效配置
	if IsCommandAllowedWithInvalidConfig(commandPath) {
		return nil
	}

	configGuardOnce.Do(func() {
		loader := config.NewConfigLoader()
		snapshot, err := loader.ReadConfigFileSnapshot()
		if err != nil {
			configGuardResult = fmt.Errorf("无法读取配置: %w — 运行 'crabclaw doctor' 进行诊断", err)
			return
		}
		if !snapshot.Valid {
			msg := "配置文件无效"
			if len(snapshot.Issues) > 0 {
				msg = fmt.Sprintf("配置文件无效: %s", snapshot.Issues[0].Message)
			}
			configGuardResult = fmt.Errorf("%s — 运行 'crabclaw doctor' 进行修复", msg)
		}
	})

	return configGuardResult
}
