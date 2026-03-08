package channels

import (
	"fmt"
	"strings"
)

// 频道状态问题收集 — 继承自 src/channels/plugins/status-issues/ (5 文件)
// 检测各频道配置和运行时问题

// ChannelStatusIssue 频道状态问题
type ChannelStatusIssue struct {
	Channel   string `json:"channel"`
	AccountID string `json:"accountId"`
	Kind      string `json:"kind"` // "intent" | "config" | "permissions" | "runtime" | "auth"
	Message   string `json:"message"`
	Fix       string `json:"fix"`
}

// ── 辅助 (shared.ts) ──

func asString(v interface{}) string {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimSpace(s)
}

func isRecordMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func formatMatchMetadata(matchKey, matchSource interface{}) string {
	mk := asString(matchKey)
	ms := asString(matchSource)
	var parts []string
	if mk != "" {
		parts = append(parts, "matchKey="+mk)
	}
	if ms != "" {
		parts = append(parts, "matchSource="+ms)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return ""
}

func appendMatchMetadata(message string, matchKey, matchSource interface{}) string {
	meta := formatMatchMetadata(matchKey, matchSource)
	if meta != "" {
		return fmt.Sprintf("%s (%s)", message, meta)
	}
	return message
}

// ── Discord 状态问题 ──

// CollectDiscordStatusIssues 收集 Discord 状态问题
func CollectDiscordStatusIssues(accounts []map[string]interface{}) []ChannelStatusIssue {
	var issues []ChannelStatusIssue
	for _, entry := range accounts {
		accountID := asString(entry["accountId"])
		if accountID == "" {
			accountID = "default"
		}
		if entry["enabled"] == false || entry["configured"] != true {
			continue
		}
		// Intent 检查
		if app, ok := isRecordMap(entry["application"]); ok {
			if intents, ok := isRecordMap(app["intents"]); ok {
				mc := asString(intents["messageContent"])
				if mc == "disabled" {
					issues = append(issues, ChannelStatusIssue{
						Channel:   "discord",
						AccountID: accountID,
						Kind:      "intent",
						Message:   "Message Content Intent is disabled. Bot may not see normal channel messages.",
						Fix:       "Enable Message Content Intent in Discord Dev Portal → Bot → Privileged Gateway Intents.",
					})
				}
			}
		}
		// 权限审计
		if audit, ok := isRecordMap(entry["audit"]); ok {
			if uc, ok := audit["unresolvedChannels"].(float64); ok && uc > 0 {
				issues = append(issues, ChannelStatusIssue{
					Channel:   "discord",
					AccountID: accountID,
					Kind:      "config",
					Message:   fmt.Sprintf("Some configured guild channels are not numeric IDs (unresolvedChannels=%.0f).", uc),
					Fix:       "Use numeric channel IDs as keys in channels.discord.guilds.*.channels.",
				})
			}
			if channels, ok := audit["channels"].([]interface{}); ok {
				for _, ch := range channels {
					chMap, ok := isRecordMap(ch)
					if !ok {
						continue
					}
					if chMap["ok"] == true {
						continue
					}
					chID := asString(chMap["channelId"])
					if chID == "" {
						continue
					}
					var missing []string
					if arr, ok := chMap["missing"].([]interface{}); ok {
						for _, v := range arr {
							if s := asString(v); s != "" {
								missing = append(missing, s)
							}
						}
					}
					errStr := asString(chMap["error"])
					msg := fmt.Sprintf("Channel %s permission check failed.", chID)
					if len(missing) > 0 {
						msg += " missing " + strings.Join(missing, ", ")
					}
					if errStr != "" {
						msg += ": " + errStr
					}
					issues = append(issues, ChannelStatusIssue{
						Channel:   "discord",
						AccountID: accountID,
						Kind:      "permissions",
						Message:   appendMatchMetadata(msg, chMap["matchKey"], chMap["matchSource"]),
						Fix:       "Ensure the bot role can view + send in this channel.",
					})
				}
			}
		}
	}
	return issues
}

// ── Telegram 状态问题 ──

// CollectTelegramStatusIssues 收集 Telegram 状态问题
func CollectTelegramStatusIssues(accounts []map[string]interface{}) []ChannelStatusIssue {
	var issues []ChannelStatusIssue
	for _, entry := range accounts {
		accountID := asString(entry["accountId"])
		if accountID == "" {
			accountID = "default"
		}
		if entry["enabled"] == false || entry["configured"] != true {
			continue
		}
		// Privacy mode 检查
		if entry["allowUnmentionedGroups"] == true {
			issues = append(issues, ChannelStatusIssue{
				Channel:   "telegram",
				AccountID: accountID,
				Kind:      "config",
				Message:   "Config allows unmentioned group messages (requireMention=false). Telegram Bot API privacy mode will block most group messages unless disabled.",
				Fix:       "In BotFather run /setprivacy → Disable for this bot.",
			})
		}
		// 群组审计
		if audit, ok := isRecordMap(entry["audit"]); ok {
			if wc, ok := audit["hasWildcardUnmentionedGroups"].(bool); ok && wc {
				issues = append(issues, ChannelStatusIssue{
					Channel:   "telegram",
					AccountID: accountID,
					Kind:      "config",
					Message:   `Telegram groups config uses "*" with requireMention=false; membership probing is not possible without explicit group IDs.`,
					Fix:       "Add explicit numeric group ids under channels.telegram.groups.",
				})
			}
			if ug, ok := audit["unresolvedGroups"].(float64); ok && ug > 0 {
				issues = append(issues, ChannelStatusIssue{
					Channel:   "telegram",
					AccountID: accountID,
					Kind:      "config",
					Message:   fmt.Sprintf("Some configured Telegram groups are not numeric IDs (unresolvedGroups=%.0f).", ug),
					Fix:       "Use numeric chat IDs (e.g. -100...) as keys in channels.telegram.groups.",
				})
			}
			if groups, ok := audit["groups"].([]interface{}); ok {
				for _, g := range groups {
					gMap, ok := isRecordMap(g)
					if !ok || gMap["ok"] == true {
						continue
					}
					chatID := asString(gMap["chatId"])
					if chatID == "" {
						continue
					}
					status := asString(gMap["status"])
					errStr := asString(gMap["error"])
					msg := fmt.Sprintf("Group %s not reachable by bot.", chatID)
					if status != "" {
						msg += " status=" + status
					}
					if errStr != "" {
						msg += ": " + errStr
					}
					issues = append(issues, ChannelStatusIssue{
						Channel:   "telegram",
						AccountID: accountID,
						Kind:      "runtime",
						Message:   appendMatchMetadata(msg, gMap["matchKey"], gMap["matchSource"]),
						Fix:       "Invite the bot to the group, then DM the bot once (/start) and restart the gateway.",
					})
				}
			}
		}
	}
	return issues
}

// ── WhatsApp 状态问题 ──

// CollectWhatsAppStatusIssues 收集 WhatsApp 状态问题
func CollectWhatsAppStatusIssues(accounts []map[string]interface{}) []ChannelStatusIssue {
	var issues []ChannelStatusIssue
	for _, entry := range accounts {
		accountID := asString(entry["accountId"])
		if accountID == "" {
			accountID = "default"
		}
		if entry["enabled"] == false {
			continue
		}
		linked := entry["linked"] == true
		running := entry["running"] == true
		connected := entry["connected"] == true

		if !linked {
			issues = append(issues, ChannelStatusIssue{
				Channel:   "whatsapp",
				AccountID: accountID,
				Kind:      "auth",
				Message:   "Not linked (no WhatsApp Web session).",
				Fix:       "Run: `crabclaw channels login` (scan QR on the gateway host).",
			})
			continue
		}
		if running && !connected {
			msg := "Linked but disconnected"
			if ra, ok := entry["reconnectAttempts"].(float64); ok {
				msg += fmt.Sprintf(" (reconnectAttempts=%.0f)", ra)
			}
			if lastErr := asString(entry["lastError"]); lastErr != "" {
				msg += ": " + lastErr
			} else {
				msg += "."
			}
			issues = append(issues, ChannelStatusIssue{
				Channel:   "whatsapp",
				AccountID: accountID,
				Kind:      "runtime",
				Message:   msg,
				Fix:       "Run: `crabclaw doctor` (or restart the gateway).",
			})
		}
	}
	return issues
}

// CollectAllStatusIssues 收集所有频道状态问题
func CollectAllStatusIssues(channelSnapshots map[ChannelID][]map[string]interface{}) []ChannelStatusIssue {
	var issues []ChannelStatusIssue
	if accounts, ok := channelSnapshots[ChannelDiscord]; ok {
		issues = append(issues, CollectDiscordStatusIssues(accounts)...)
	}
	if accounts, ok := channelSnapshots[ChannelTelegram]; ok {
		issues = append(issues, CollectTelegramStatusIssues(accounts)...)
	}
	if accounts, ok := channelSnapshots[ChannelWhatsApp]; ok {
		issues = append(issues, CollectWhatsAppStatusIssues(accounts)...)
	}
	return issues
}
