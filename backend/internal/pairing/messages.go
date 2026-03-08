package pairing

// 配对消息构建 — 对齐 src/pairing/pairing-messages.ts (21L) + src/channels/plugins/pairing-message.ts (3L)

import (
	"os"
	"strings"
)

// PairingApprovedMessage 配对批准确认消息。
// 对齐 TS PAIRING_APPROVED_MESSAGE。
const PairingApprovedMessage = "✅ Crab Claw（蟹爪） access approved. Send a message to start chatting."

// BuildPairingReply 构建配对回复文本。
// 对齐 TS buildPairingReply({ channel, idLine, code })。
func BuildPairingReply(channel, idLine, code string) string {
	return strings.Join([]string{
		"Crab Claw（蟹爪）: access not configured.",
		"",
		idLine,
		"",
		"Pairing code: " + code,
		"",
		"Ask the bot owner to approve with:",
		formatCliCommand("crabclaw pairing approve " + channel + " <code>"),
	}, "\n")
}

// formatCliCommand 格式化 CLI 命令（考虑 OPENACOSMI_PROFILE）。
// 对齐 TS formatCliCommand()：如果设置了 profile，在 CLI 名称后插入 --profile 参数。
func formatCliCommand(cmd string) string {
	profile := strings.TrimSpace(os.Getenv("OPENACOSMI_PROFILE"))
	if profile == "" {
		return cmd
	}
	const prefix = "crabclaw "
	if strings.HasPrefix(cmd, prefix) {
		return "crabclaw --profile " + profile + " " + cmd[len(prefix):]
	}
	return cmd
}
