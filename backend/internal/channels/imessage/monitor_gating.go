//go:build darwin

package imessage

import (
	"fmt"
	"strings"
)

// 控制命令门控 + 配对回复 — 对标 TS channels/command-gating.ts + pairing/pairing-messages.ts

// ControlCommandGateResult 控制命令门控结果
type ControlCommandGateResult struct {
	CommandAuthorized bool
	ShouldBlock       bool
}

// ControlCommandAuthorizer 单个授权者
type ControlCommandAuthorizer struct {
	Configured bool
	Allowed    bool
}

// ResolveControlCommandGate 解析控制命令门控。
// TS 对照: channels/command-gating.ts resolveControlCommandGate()
func ResolveControlCommandGate(params ControlCommandGateParams) ControlCommandGateResult {
	if !params.HasControlCommand {
		return ControlCommandGateResult{
			CommandAuthorized: false,
			ShouldBlock:       false,
		}
	}

	// 如果不启用访问组，所有命令均授权
	if !params.UseAccessGroups {
		return ControlCommandGateResult{
			CommandAuthorized: true,
			ShouldBlock:       false,
		}
	}

	// 检查所有授权者：至少一个已配置且允许 → 授权
	authorized := false
	anyConfigured := false
	for _, auth := range params.Authorizers {
		if auth.Configured {
			anyConfigured = true
			if auth.Allowed {
				authorized = true
				break
			}
		}
	}

	// 无任何授权者配置 → 放行
	if !anyConfigured {
		authorized = true
	}

	shouldBlock := false
	if params.AllowTextCommands && params.HasControlCommand && !authorized {
		shouldBlock = true
	}

	return ControlCommandGateResult{
		CommandAuthorized: authorized,
		ShouldBlock:       shouldBlock,
	}
}

// ControlCommandGateParams 门控参数
type ControlCommandGateParams struct {
	UseAccessGroups   bool
	Authorizers       []ControlCommandAuthorizer
	AllowTextCommands bool
	HasControlCommand bool
}

// BuildPairingReply 构建配对回复消息。
// TS 对照: pairing/pairing-messages.ts buildPairingReply()
func BuildPairingReply(channel, idLine, code string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("👋 Hi! This %s account is paired with Crab Claw（蟹爪）.",
		strings.ToUpper(channel[:1])+channel[1:]))
	lines = append(lines, "")
	if idLine != "" {
		lines = append(lines, idLine)
	}
	lines = append(lines, fmt.Sprintf("Your pairing code: %s", code))
	lines = append(lines, "")
	lines = append(lines, "To approve, run: /pair approve <code>")
	return strings.Join(lines, "\n")
}

// LogInboundDrop 记录入站消息被丢弃的日志。
// TS 对照: channels/logging.ts logInboundDrop()
func LogInboundDrop(logFn func(string), channel, reason, target string) {
	if logFn == nil {
		return
	}
	msg := fmt.Sprintf("%s: dropped inbound from %s (%s)", channel, target, reason)
	logFn(msg)
}
