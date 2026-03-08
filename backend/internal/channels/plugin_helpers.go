package channels

import "fmt"

// 插件辅助函数 — 继承自 src/channels/plugins/helpers.ts (21L)

// ResolveChannelDefaultAccountID 解析频道默认账户 ID
func ResolveChannelDefaultAccountID(accountIDs []string) string {
	if len(accountIDs) > 0 {
		return accountIDs[0]
	}
	return DefaultAccountID
}

// FormatPairingApproveHint 格式化配对审批提示
func FormatPairingApproveHint(channelID string) string {
	return fmt.Sprintf("Approve via: `crabclaw pairing list %s` / `crabclaw pairing approve %s <code>`", channelID, channelID)
}
