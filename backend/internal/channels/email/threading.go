package email

// threading.go — Phase 5: Inbound Bridge
// 线程归并 + session key 生成 + 去重 + 入站消息映射
// 将 ParsedEmail 转为系统可消费的入站消息并通过 DispatchMultimodalFunc 分发

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

// InboundEmailMessage 入站邮件内部结构（Phase 5 核心类型）
// 由 ParsedEmail + RawEmailMessage 合并而成，供 session key 生成 + MsgContext 映射
type InboundEmailMessage struct {
	AccountID    string
	Provider     string
	Mailbox      string
	UID          uint32
	UIDValidity  uint32
	MessageID    string
	InReplyTo    string
	References   []string
	From         string
	To           []string
	Cc           []string
	Subject      string
	TextBody     string
	HTMLBody     string // 内部保留，不暴露给 agent
	Attachments  []EmailAttachment
	InlineImages []EmailAttachment
	ReceivedAt   time.Time
	RawSize      uint32
	HasHTML      bool
}

// EmailMetadata 结构化元数据（供回复 builder / transcript / 调试）
type EmailMetadata struct {
	MessageID   string   `json:"messageId"`
	InReplyTo   string   `json:"inReplyTo,omitempty"`
	References  []string `json:"references,omitempty"`
	Subject     string   `json:"subject"`
	From        string   `json:"from"`
	To          []string `json:"to"`
	Cc          []string `json:"cc,omitempty"`
	ReceivedAt  string   `json:"receivedAt"`
	HasHTML     bool     `json:"hasHTML"`
	Attachments int      `json:"attachments"`
	SessionKey  string   `json:"sessionKey"`
}

// ThreadResolver 线程归并器
type ThreadResolver struct {
	accountID string
	dedup     *DedupCache
}

// NewThreadResolver 创建线程归并器
func NewThreadResolver(accountID string, dedup *DedupCache) *ThreadResolver {
	return &ThreadResolver{
		accountID: accountID,
		dedup:     dedup,
	}
}

// --- Session Key 生成 ---

// ResolveSessionKey 根据线程头生成 session key
// 优先级: In-Reply-To → References[0] → subject+participants 退化
func ResolveSessionKey(accountID string, msg *InboundEmailMessage) string {
	// 优先: In-Reply-To 或 References 的 root message-id
	rootMsgID := resolveRootMessageID(msg)
	if rootMsgID != "" {
		hash := shortHash(rootMsgID)
		return fmt.Sprintf("email:%s:thread:%s", accountID, hash)
	}

	// 退化: subject + peer hash
	subject := normalizeSubject(msg.Subject)
	peer := extractSenderAddress(msg.From)

	subjectHash := shortHash(subject)
	peerHash := shortHash(peer)
	return fmt.Sprintf("email:%s:subject:%s:peer:%s", accountID, subjectHash, peerHash)
}

// resolveRootMessageID 查找线程根 Message-ID
func resolveRootMessageID(msg *InboundEmailMessage) string {
	// References 的第一个是线程根
	if len(msg.References) > 0 {
		return msg.References[0]
	}
	// In-Reply-To 作为次选
	if msg.InReplyTo != "" {
		return msg.InReplyTo
	}
	return ""
}

// normalizeSubject 规范化邮件主题（去除 Re:/Fwd:/回复: 等前缀）
func normalizeSubject(subject string) string {
	s := strings.TrimSpace(subject)
	for {
		lower := strings.ToLower(s)
		trimmed := false
		for _, prefix := range []string{"re:", "re：", "fwd:", "fw:", "回复:", "转发:"} {
			if strings.HasPrefix(lower, prefix) {
				s = strings.TrimSpace(s[len(prefix):])
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return s
}

// extractSenderAddress 从 From 字段提取纯邮箱地址
func extractSenderAddress(from string) string {
	// "Name <addr@host.com>" → "addr@host.com"
	if idx := strings.LastIndex(from, "<"); idx >= 0 {
		end := strings.Index(from[idx:], ">")
		if end > 0 {
			return strings.ToLower(from[idx+1 : idx+end])
		}
	}
	return strings.ToLower(strings.TrimSpace(from))
}

// shortHash 生成 SHA-256 短哈希（前 12 位 hex）
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:6]) // 12 hex chars
}

// --- 去重 ---

// GenerateDedupKey 生成去重键
// 优先级: Message-ID → (UIDVALIDITY, UID) → (Date, From, Subject, size hash)
func GenerateDedupKey(msg *InboundEmailMessage) string {
	// Level 1: Message-ID
	if msg.MessageID != "" {
		return "msgid:" + msg.MessageID
	}
	// Level 2: UIDVALIDITY + UID
	if msg.UIDValidity > 0 && msg.UID > 0 {
		return fmt.Sprintf("uid:%d:%d", msg.UIDValidity, msg.UID)
	}
	// Level 3: Date+From+Subject+Size hash
	composite := fmt.Sprintf("%s|%s|%s|%d",
		msg.ReceivedAt.Format(time.RFC3339),
		msg.From,
		msg.Subject,
		msg.RawSize,
	)
	return "hash:" + shortHash(composite)
}

// --- InboundEmailMessage 构造 ---

// BuildInboundMessage 从 RawEmailMessage + ParsedEmail 构造 InboundEmailMessage
func BuildInboundMessage(accountID, provider, mailbox string, raw RawEmailMessage, parsed *ParsedEmail, uidValidity uint32) *InboundEmailMessage {
	return &InboundEmailMessage{
		AccountID:    accountID,
		Provider:     provider,
		Mailbox:      mailbox,
		UID:          raw.UID,
		UIDValidity:  uidValidity,
		MessageID:    parsed.MessageID,
		InReplyTo:    parsed.InReplyTo,
		References:   parsed.References,
		From:         parsed.From,
		To:           parsed.To,
		Cc:           parsed.Cc,
		Subject:      parsed.Subject,
		TextBody:     parsed.TextBody,
		HTMLBody:     parsed.HTMLBody,
		Attachments:  parsed.Attachments,
		InlineImages: parsed.InlineImages,
		ReceivedAt:   parsed.Date,
		RawSize:      raw.Size,
		HasHTML:      parsed.HasHTML,
	}
}

// --- ChannelMessage 映射 ---

// ToChannelMessage 将 InboundEmailMessage 转为 ChannelMessage（供 DispatchMultimodalFunc 消费）
func ToChannelMessage(msg *InboundEmailMessage) *channels.ChannelMessage {
	cm := &channels.ChannelMessage{
		Text:        msg.TextBody,
		MessageID:   msg.MessageID,
		MessageType: "email",
	}

	// 附件 → ChannelAttachment
	for _, att := range msg.Attachments {
		cm.Attachments = append(cm.Attachments, toChannelAttachment(att))
	}
	// 内嵌图片也作为附件传递
	for _, img := range msg.InlineImages {
		cm.Attachments = append(cm.Attachments, toChannelAttachment(img))
	}

	return cm
}

// toChannelAttachment 转换 EmailAttachment → ChannelAttachment
func toChannelAttachment(att EmailAttachment) channels.ChannelAttachment {
	category := categorizeAttachment(att.ContentType)
	return channels.ChannelAttachment{
		Category: category,
		FileName: att.Filename,
		FileSize: int64(att.Size),
		MimeType: att.ContentType,
		Data:     att.Data,
	}
}

// categorizeAttachment 根据 MIME 类型归类附件
func categorizeAttachment(mimeType string) string {
	lower := strings.ToLower(mimeType)
	switch {
	case strings.HasPrefix(lower, "image/"):
		return "image"
	case strings.HasPrefix(lower, "audio/"):
		return "audio"
	case strings.HasPrefix(lower, "video/"):
		return "video"
	default:
		return "document"
	}
}

// --- 元数据 ---

// BuildMetadata 构造结构化元数据
func BuildMetadata(msg *InboundEmailMessage, sessionKey string) EmailMetadata {
	return EmailMetadata{
		MessageID:   msg.MessageID,
		InReplyTo:   msg.InReplyTo,
		References:  msg.References,
		Subject:     msg.Subject,
		From:        msg.From,
		To:          msg.To,
		Cc:          msg.Cc,
		ReceivedAt:  msg.ReceivedAt.Format(time.RFC3339),
		HasHTML:     msg.HasHTML,
		Attachments: len(msg.Attachments) + len(msg.InlineImages),
		SessionKey:  sessionKey,
	}
}

// --- Inbound Bridge 入口 ---

// ProcessInbound 处理入站邮件完整流程:
// 1. 解析 MIME
// 2. 构造 InboundEmailMessage
// 3. 去重检查
// 4. 生成 session key
// 5. 映射为 ChannelMessage
// 6. 调用 DispatchMultimodalFunc
func ProcessInbound(
	accountID string,
	provider string,
	mailbox string,
	uidValidity uint32,
	rawMsgs []RawEmailMessage,
	limits ParseLimits,
	dedup *DedupCache,
	dispatch channels.DispatchMultimodalFunc,
) {
	if dispatch == nil {
		slog.Warn("email: DispatchMultimodalFunc not set, dropping inbound messages",
			"account", accountID, "count", len(rawMsgs))
		return
	}

	for _, raw := range rawMsgs {
		// 1. 解析 MIME
		parsed, err := ParseEmail(raw.Body, limits)
		if err != nil {
			slog.Warn("email: MIME parse failed, skipping message",
				"account", accountID, "uid", raw.UID, "error", err)
			continue
		}

		// 2. 构造 InboundEmailMessage
		msg := BuildInboundMessage(accountID, provider, mailbox, raw, parsed, uidValidity)

		// 3. 去重
		dedupKey := GenerateDedupKey(msg)
		if dedup != nil && dedup.HasSeen(dedupKey) {
			slog.Debug("email: duplicate message skipped",
				"account", accountID, "uid", raw.UID, "dedupKey", dedupKey)
			continue
		}

		// 4. 生成 session key
		sessionKey := ResolveSessionKey(accountID, msg)

		// 5. 记录去重
		if dedup != nil {
			dedup.MarkSeen(dedupKey)
		}

		// 6. 映射 ChannelMessage
		cm := ToChannelMessage(msg)

		// 7. 构造路由参数并分发
		senderAddr := extractSenderAddress(msg.From)
		slog.Info("email: dispatching inbound message",
			"account", accountID,
			"from", senderAddr,
			"subject", msg.Subject,
			"sessionKey", sessionKey,
			"uid", raw.UID,
			"attachments", len(cm.Attachments),
		)

		// chatID = sessionKey（email 用 sessionKey 作为 chatID 路由到同一会话）
		// userID = sender address
		reply := dispatch("email", accountID, sessionKey, senderAddr, cm)
		if reply != nil && reply.Text != "" {
			slog.Debug("email: dispatch returned reply",
				"account", accountID, "sessionKey", sessionKey, "replyLen", len(reply.Text))
		}
	}
}
