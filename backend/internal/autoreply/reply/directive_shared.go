package reply

import (
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
)

// TS 对照: auto-reply/reply/directive-handling.shared.ts (67L)

const SystemMark = "⚙️"

// FormatDirectiveAck 格式化指令确认消息。
// TS 对照: directive-handling.shared.ts L6-14
func FormatDirectiveAck(text string) string {
	if text == "" {
		return text
	}
	if strings.HasPrefix(text, SystemMark) {
		return text
	}
	return SystemMark + " " + text
}

// FormatOptionsLine 格式化选项行。
func FormatOptionsLine(options string) string {
	return "Options: " + options + "."
}

// WithOptions 将选项行追加到文本后。
func WithOptions(line, options string) string {
	return line + "\n" + FormatOptionsLine(options)
}

// FormatElevatedRuntimeHint 格式化直连运行时提示。
func FormatElevatedRuntimeHint() string {
	return SystemMark + " Runtime is direct; sandboxing does not apply."
}

// FormatElevatedEvent 格式化 elevated 事件文本。
func FormatElevatedEvent(level autoreply.ElevatedLevel) string {
	switch level {
	case autoreply.ElevatedFull:
		return "Elevated FULL — exec runs on host with auto-approval."
	case autoreply.ElevatedAsk, autoreply.ElevatedOn:
		return "Elevated ASK — exec runs on host; approvals may still apply."
	default:
		return "Elevated OFF — exec stays in sandbox."
	}
}

// FormatReasoningEvent 格式化 reasoning 事件文本。
func FormatReasoningEvent(level autoreply.ReasoningLevel) string {
	switch level {
	case autoreply.ReasoningStream:
		return "Reasoning STREAM — emit live <think>."
	case autoreply.ReasoningOn:
		return "Reasoning ON — include <think>."
	default:
		return "Reasoning OFF — hide <think>."
	}
}

// FormatElevatedUnavailableText 格式化 elevated 不可用文本。
// TS 对照: directive-handling.shared.ts L43-66
func FormatElevatedUnavailableText(runtimeSandboxed bool, failures []ElevatedGateFailure, sessionKey string) string {
	runtimeLabel := "direct"
	if runtimeSandboxed {
		runtimeLabel = "sandboxed"
	}
	lines := []string{
		fmt.Sprintf("elevated is not available right now (runtime=%s).", runtimeLabel),
	}
	if len(failures) > 0 {
		parts := make([]string, 0, len(failures))
		for _, f := range failures {
			parts = append(parts, fmt.Sprintf("%s (%s)", f.Gate, f.Key))
		}
		lines = append(lines, "Failing gates: "+strings.Join(parts, ", "))
	} else {
		lines = append(lines, "Fix-it keys: tools.elevated.enabled, tools.elevated.allowFrom.<provider>, agents.list[].tools.elevated.*")
	}
	if sessionKey != "" {
		lines = append(lines, fmt.Sprintf("See: `crabclaw sandbox explain --session %s`", sessionKey))
	}
	return strings.Join(lines, "\n")
}

// ElevatedGateFailure 提权门控失败条目。
type ElevatedGateFailure struct {
	Gate string
	Key  string
}
