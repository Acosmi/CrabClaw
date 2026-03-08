package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply/reply"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// WhatsApp 入站消息处理管线 — WA-A + WA-B
// 继承自 src/web/auto-reply/monitor/ + src/web/inbound/monitor.ts
// 与 iMessage (A2) / Signal (A3) 管线结构一致。

// WhatsAppEventHandlerDeps 事件处理器依赖
type WhatsAppEventHandlerDeps struct {
	Ctx       context.Context
	CFG       *types.OpenAcosmiConfig
	AccountID string
	Deps      *WhatsAppMonitorDeps
	LogInfo   func(string)
	LogError  func(string)
}

// HandleInboundMessageFull WA-A 主入口 — 完整的入站消息处理管线
// 从 WebInboundMessage 入口，执行：
// 1. 去重检查
// 2. DM/群组策略门控
// 3. 配对请求管理（WA-B）
// 4. 附件解析
// 5. 路由 → 会话记录 → 分发
func HandleInboundMessageFull(deps WhatsAppEventHandlerDeps, msg *WebInboundMessage) {
	if msg == nil {
		return
	}

	account := ResolveWhatsAppAccount(deps.CFG, deps.AccountID)
	dmPolicy := string(account.DmPolicy)
	if dmPolicy == "" {
		dmPolicy = "allowlist"
	}
	groupPolicy := string(account.GroupPolicy)
	if groupPolicy == "" {
		groupPolicy = "disabled"
	}

	// 去重
	dedupeKey := fmt.Sprintf("wa:%s:%s:%s", account.AccountID, msg.From, msg.ID)
	if msg.ID == "" {
		dedupeKey = fmt.Sprintf("wa:%s:%s:%d:%s",
			account.AccountID, msg.From, msg.Timestamp,
			truncateForDedupe(msg.Body, 80))
	}
	if IsRecentInboundMessage(dedupeKey) {
		return
	}

	isGroup := msg.ChatType == "group" || IsWhatsAppGroupJid(msg.ChatID)
	senderDisplay := resolveSenderDisplay(msg)
	senderAllowId := msg.From

	// DM 策略
	if !isGroup {
		if dmPolicy == "disabled" {
			return
		}
		dmAllowed := dmPolicy == "open" || isSenderAllowed(senderAllowId, account.AllowFrom)
		if !dmAllowed {
			// 检查 pairing store 动态允许
			if deps.Deps != nil && deps.Deps.ReadAllowFromStore != nil {
				dynamic, err := deps.Deps.ReadAllowFromStore("whatsapp")
				if err == nil && isSenderAllowed(senderAllowId, dynamic) {
					dmAllowed = true
				}
			}
		}
		if !dmAllowed {
			if dmPolicy == "pairing" {
				handleWhatsAppPairing(deps, account, msg, senderAllowId)
			} else {
				deps.LogInfo(fmt.Sprintf("Blocked whatsapp sender %s (dmPolicy=%s)", senderDisplay, dmPolicy))
			}
			return
		}
	}

	// 群组策略
	if isGroup && groupPolicy == "disabled" {
		deps.LogInfo("Blocked whatsapp group message (groupPolicy: disabled)")
		return
	}
	if isGroup && groupPolicy == "allowlist" {
		if len(account.GroupAllowFrom) == 0 {
			deps.LogInfo("Blocked whatsapp group message (groupPolicy: allowlist, no groupAllowFrom)")
			return
		}
		if !isSenderAllowed(senderAllowId, account.GroupAllowFrom) {
			deps.LogInfo(fmt.Sprintf("Blocked whatsapp group sender %s (not in groupAllowFrom)", senderDisplay))
			return
		}
	}
	if isGroup && groupPolicy == "mention_only" {
		if !msg.WasMentioned {
			return // 群组中仅处理被 @ 的消息
		}
	}
	// always 模式：任何群组消息都触发回复（无需 mention）
	if isGroup && groupPolicy == "always" {
		// 无需额外检查，允许所有群组消息通过
		deps.LogInfo(fmt.Sprintf("whatsapp group message allowed (groupPolicy: always) from=%s", senderDisplay))
	}
	// silent_token 模式：消息包含特定静默 token 时触发回复
	if isGroup && groupPolicy == "silent_token" {
		silentToken := ResolveSilentToken(account)
		if silentToken == "" || !strings.Contains(msg.Body, silentToken) {
			deps.LogInfo(fmt.Sprintf("whatsapp group message skipped (groupPolicy: silent_token, token not found) from=%s", senderDisplay))
			return
		}
		deps.LogInfo(fmt.Sprintf("whatsapp group message allowed (groupPolicy: silent_token, token matched) from=%s", senderDisplay))
	}

	// 消息体
	bodyText := strings.TrimSpace(msg.Body)
	if bodyText == "" {
		bodyText = strings.TrimSpace(FormatLocationText(msg.Location))
	}
	if bodyText == "" {
		return
	}

	// 附件处理
	var mediaPath, mediaType string
	if msg.MediaPath != "" {
		mediaPath = msg.MediaPath
		mediaType = msg.MediaType
	} else if msg.MediaURL != "" && deps.Deps != nil && deps.Deps.ResolveMedia != nil {
		maxBytes := MaxMediaSizeBytes
		if account.MediaMaxMB != nil {
			maxBytes = *account.MediaMaxMB * 1024 * 1024
		}
		path, ct, err := deps.Deps.ResolveMedia(msg.MediaURL, maxBytes)
		if err != nil {
			deps.LogError(fmt.Sprintf("whatsapp: resolve media failed: %s", err))
		} else {
			mediaPath = path
			mediaType = ct
		}
	}
	_ = mediaPath // 可选，传递给 MsgContext
	_ = mediaType

	deps.LogInfo(fmt.Sprintf("whatsapp inbound: from=%s len=%d group=%v",
		senderDisplay, len(bodyText), isGroup))

	// 分发到 agent 路由
	dispatchWhatsAppInbound(deps, account, msg, bodyText, isGroup, senderDisplay)
}

// dispatchWhatsAppInbound WA-A: 入站消息分发管线
// resolveAgentRoute → 构建 MsgContext → recordInboundSession → dispatchInboundMessage
func dispatchWhatsAppInbound(
	deps WhatsAppEventHandlerDeps,
	account ResolvedWhatsAppAccount,
	msg *WebInboundMessage,
	bodyText string,
	isGroup bool,
	senderDisplay string,
) {
	if deps.Deps == nil || deps.Deps.ResolveAgentRoute == nil {
		deps.LogInfo("whatsapp: inbound received but ResolveAgentRoute not available (DI stub)")
		return
	}

	peerKind := "direct"
	peerID := msg.From
	if isGroup {
		peerKind = "group"
		peerID = msg.ChatID
		if peerID == "" {
			peerID = "unknown"
		}
	}

	route, err := deps.Deps.ResolveAgentRoute(AgentRouteParams{
		Channel:   "whatsapp",
		AccountID: account.AccountID,
		PeerKind:  peerKind,
		PeerID:    peerID,
	})
	if err != nil {
		deps.LogError(fmt.Sprintf("whatsapp: resolve agent route failed: %s", err))
		return
	}

	// 构建地址标识
	waTo := "whatsapp:" + msg.From
	fromField := "whatsapp:" + msg.From
	if isGroup {
		waTo = "whatsapp:group:" + msg.ChatID
		fromField = waTo
	}

	fromLabel := senderDisplay
	if isGroup && msg.GroupSubject != "" {
		fromLabel = fmt.Sprintf("%s (group:%s)", msg.GroupSubject, msg.ChatID)
	}

	chatType := "direct"
	if isGroup {
		chatType = "group"
	}

	// 信封格式化
	body := formatWhatsAppEnvelope("WhatsApp", fromLabel, msg.Timestamp, bodyText, chatType)

	msgCtx := &autoreply.MsgContext{
		Body:               body,
		RawBody:            bodyText,
		CommandBody:        bodyText,
		From:               fromField,
		To:                 waTo,
		SessionKey:         route.SessionKey,
		AccountID:          route.AccountID,
		ChatType:           chatType,
		ConversationLabel:  fromLabel,
		SenderName:         msg.PushName,
		SenderID:           msg.SenderJid,
		Provider:           "whatsapp",
		Surface:            "whatsapp",
		IsGroup:            isGroup,
		OriginatingChannel: "whatsapp",
		OriginatingTo:      waTo,
		MessageSid:         msg.ID,
		CommandAuthorized:  true,
	}

	if isGroup && msg.GroupSubject != "" {
		msgCtx.GroupSubject = msg.GroupSubject
	}

	reply.FinalizeInboundContext(msgCtx, nil)

	// 会话记录
	if deps.Deps.RecordInboundSession != nil {
		var lastRoute *LastRouteUpdate
		if !isGroup {
			lastRoute = &LastRouteUpdate{
				SessionKey: route.MainSessionKey,
				Channel:    "whatsapp",
				To:         msg.From,
				AccountID:  route.AccountID,
			}
		}
		storePath := ""
		if deps.Deps.ResolveStorePath != nil {
			storePath = deps.Deps.ResolveStorePath(route.AgentID)
		}
		if err := deps.Deps.RecordInboundSession(RecordSessionParams{
			StorePath:       storePath,
			SessionKey:      msgCtx.SessionKey,
			Ctx:             msgCtx,
			UpdateLastRoute: lastRoute,
		}); err != nil {
			deps.LogInfo(fmt.Sprintf("whatsapp: failed updating session meta: %s", err))
		}
	}

	// 分发到 auto-reply
	if deps.Deps.DispatchInboundMessage == nil {
		deps.LogInfo("whatsapp: inbound processed but DispatchInboundMessage not available (DI stub)")
		return
	}

	_, err = deps.Deps.DispatchInboundMessage(deps.Ctx, DispatchParams{
		Ctx:        msgCtx,
		Dispatcher: nil,
	})
	if err != nil {
		deps.LogError(fmt.Sprintf("whatsapp: dispatch inbound failed: %s", err))
	}
}

// handleWhatsAppPairing WA-B: 配对请求管理
func handleWhatsAppPairing(
	deps WhatsAppEventHandlerDeps,
	account ResolvedWhatsAppAccount,
	msg *WebInboundMessage,
	senderAllowId string,
) {
	deps.LogInfo(fmt.Sprintf("whatsapp pairing request sender=%s", senderAllowId))

	if deps.Deps == nil || deps.Deps.UpsertPairingRequest == nil {
		deps.LogInfo("whatsapp: pairing needed but UpsertPairingRequest not available")
		return
	}

	meta := map[string]string{
		"sender":   msg.From,
		"pushName": msg.PushName,
	}
	result, err := deps.Deps.UpsertPairingRequest(PairingRequestParams{
		Channel: "whatsapp",
		ID:      senderAllowId,
		Meta:    meta,
	})
	if err != nil {
		deps.LogInfo(fmt.Sprintf("whatsapp: pairing upsert failed: %s", err))
		return
	}
	if result.Created {
		replyText := BuildWhatsAppPairingReply(senderAllowId, result.Code)
		_, sendErr := SendMessageWhatsApp(msg.From, replyText, SendMessageOptions{
			AccountID: account.AccountID,
		})
		if sendErr != nil {
			slog.Error("whatsapp: pairing reply failed",
				slog.String("sender", senderAllowId),
				slog.String("error", sendErr.Error()),
			)
		}
	}
}

// BuildWhatsAppPairingReply 构建 WhatsApp 配对回复消息
// TS 对照: pairing/pairing-messages.ts buildPairingReply()
func BuildWhatsAppPairingReply(senderID, code string) string {
	var lines []string
	lines = append(lines, "👋 Hi! This WhatsApp account is paired with Crab Claw（蟹爪）.")
	lines = append(lines, "")
	if senderID != "" {
		lines = append(lines, fmt.Sprintf("Your WhatsApp sender id: %s", senderID))
	}
	lines = append(lines, fmt.Sprintf("Your pairing code: %s", code))
	lines = append(lines, "")
	lines = append(lines, "To approve, run: /pair approve <code>")
	return strings.Join(lines, "\n")
}

// formatWhatsAppEnvelope 格式化 WhatsApp 入站消息信封
// 与 iMessage/Signal FormatInboundEnvelope 逻辑一致，避免跨频道包依赖
func formatWhatsAppEnvelope(channel, from string, timestampMs int64, body, chatType string) string {
	var parts []string
	if channel != "" {
		parts = append(parts, fmt.Sprintf("[%s]", channel))
	}
	if from != "" {
		parts = append(parts, from)
	}
	if timestampMs > 0 {
		ts := time.UnixMilli(timestampMs)
		parts = append(parts, ts.Format("15:04"))
	}
	if chatType != "" {
		parts = append(parts, fmt.Sprintf("(%s)", chatType))
	}
	header := strings.Join(parts, " ")
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return header
	}
	return header + ": " + trimmed
}

// ── 辅助函数 ──

// resolveSenderDisplay 解析发送者显示名
func resolveSenderDisplay(msg *WebInboundMessage) string {
	if msg.PushName != "" {
		return msg.PushName
	}
	if msg.SenderName != "" {
		return msg.SenderName
	}
	if msg.SenderE164 != "" {
		return msg.SenderE164
	}
	return msg.From
}

// isSenderAllowed 检查发送者是否在允许列表中
func isSenderAllowed(senderID string, allowFrom []string) bool {
	if len(allowFrom) == 0 {
		return false
	}
	normalized := NormalizeWhatsAppTarget(senderID)
	for _, entry := range allowFrom {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "*" {
			return true
		}
		if trimmed == "" {
			continue
		}
		entryNormalized := NormalizeWhatsAppTarget(trimmed)
		if entryNormalized != "" && entryNormalized == normalized {
			return true
		}
		// 直接字符串匹配回退
		if strings.EqualFold(trimmed, senderID) {
			return true
		}
	}
	return false
}

// truncateForDedupe 截断字符串用于去重键（避免过长）
func truncateForDedupe(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
