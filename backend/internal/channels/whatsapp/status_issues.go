package whatsapp

import (
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// WhatsApp 状态问题收集 — 继承自 src/channels/plugins/status-issues/whatsapp.ts (74L)

// StatusIssueSeverity 问题严重度
type StatusIssueSeverity string

const (
	SeverityError   StatusIssueSeverity = "error"
	SeverityWarning StatusIssueSeverity = "warning"
	SeverityInfo    StatusIssueSeverity = "info"
)

// ChannelStatusIssue 频道状态问题
type ChannelStatusIssue struct {
	Channel   string              `json:"channel"`
	AccountID string              `json:"accountId,omitempty"`
	Severity  StatusIssueSeverity `json:"severity"`
	Title     string              `json:"title"`
	Detail    string              `json:"detail,omitempty"`
	FixHint   string              `json:"fixHint,omitempty"`
}

// ChannelAccountSnapshot 频道账户快照
type ChannelAccountSnapshot struct {
	AccountID         string `json:"accountId"`
	Linked            bool   `json:"linked"`
	Running           bool   `json:"running"`
	Connected         bool   `json:"connected"`
	ReconnectAttempts int    `json:"reconnectAttempts"`
	LastError         string `json:"lastError,omitempty"`
}

// CollectWhatsAppStatusIssues 收集 WhatsApp 账户状态问题
func CollectWhatsAppStatusIssues(cfg *types.OpenAcosmiConfig, snapshot *ChannelAccountSnapshot) []ChannelStatusIssue {
	if snapshot == nil {
		return nil
	}

	var issues []ChannelStatusIssue
	accountLabel := snapshot.AccountID
	if accountLabel == "" || accountLabel == "default" {
		accountLabel = "WhatsApp"
	} else {
		accountLabel = fmt.Sprintf("WhatsApp (%s)", accountLabel)
	}

	// 未链接
	if !snapshot.Linked {
		issues = append(issues, ChannelStatusIssue{
			Channel:   "whatsapp",
			AccountID: snapshot.AccountID,
			Severity:  SeverityError,
			Title:     accountLabel + " not linked",
			Detail:    "No WhatsApp Web session found.",
			FixHint:   "Run `crabclaw channels login --channel whatsapp` to link.",
		})
		return issues
	}

	// 未运行
	if !snapshot.Running {
		issues = append(issues, ChannelStatusIssue{
			Channel:   "whatsapp",
			AccountID: snapshot.AccountID,
			Severity:  SeverityWarning,
			Title:     accountLabel + " not running",
			Detail:    "WhatsApp is linked but the gateway is not running.",
			FixHint:   "Start the gateway to connect WhatsApp.",
		})
		return issues
	}

	// 已运行但未连接
	if !snapshot.Connected {
		detail := "WhatsApp is connecting..."
		severity := SeverityWarning
		if snapshot.ReconnectAttempts > 3 {
			severity = SeverityError
			detail = fmt.Sprintf("WhatsApp has been trying to reconnect for %d attempts.", snapshot.ReconnectAttempts)
		}
		if snapshot.LastError != "" {
			detail += " Last error: " + snapshot.LastError
		}
		issues = append(issues, ChannelStatusIssue{
			Channel:   "whatsapp",
			AccountID: snapshot.AccountID,
			Severity:  severity,
			Title:     accountLabel + " disconnected",
			Detail:    detail,
		})
	}

	// 有错误
	if snapshot.LastError != "" && snapshot.Connected {
		issues = append(issues, ChannelStatusIssue{
			Channel:   "whatsapp",
			AccountID: snapshot.AccountID,
			Severity:  SeverityInfo,
			Title:     accountLabel + " recovered with previous error",
			Detail:    snapshot.LastError,
		})
	}

	return issues
}

// ── 辅助工具 ──

// LooksLikeWhatsAppTargetID 判断是否看起来像 WhatsApp 目标 ID
func LooksLikeWhatsAppTargetID(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	candidate := stripWhatsAppTargetPrefixes(trimmed)
	// 群组 JID
	if IsWhatsAppGroupJid(candidate) {
		return true
	}
	// 用户 JID
	if IsWhatsAppUserTarget(candidate) {
		return true
	}
	// 纯电话号码（3+ 位数字，可带 + 号）
	digits := 0
	for _, ch := range candidate {
		if ch >= '0' && ch <= '9' {
			digits++
		} else if ch == '+' && digits == 0 {
			continue
		} else if ch == ' ' || ch == '-' || ch == '(' || ch == ')' {
			continue
		} else {
			return false
		}
	}
	return digits >= 3
}
