package signal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply/reply"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/Acosmi/ClawAcosmi/pkg/utils"
)

// Signal 入站事件处理 — 继承自 src/signal/monitor/event-handler.ts (582L)

// SignalEventHandlerDeps 事件处理器依赖
type SignalEventHandlerDeps struct {
	Ctx                   context.Context
	CFG                   *types.OpenAcosmiConfig
	BaseURL               string
	Account               string
	AccountID             string
	Deps                  *SignalMonitorDeps
	GroupHistories        *reply.HistoryMap
	HistoryLimit          int
	ReadReceiptsViaDaemon bool // autoStart && sendReadReceipts → daemon 已处理已读回执
	LogInfo               func(string)
	LogError              func(string)
	OnError               func(error)
}

// SignalInboundEntry 解析后的入站消息条目
type SignalInboundEntry struct {
	SenderName        string
	SenderDisplay     string
	SenderRecipient   string
	SenderPeerId      string
	GroupID           string
	GroupName         string
	IsGroup           bool
	BodyText          string
	Timestamp         int64
	MessageID         string
	MediaPath         string
	MediaType         string
	CommandAuthorized bool
}

// CreateSignalEventHandler 创建 Signal 事件处理函数
// 返回的函数处理每一个 SSE 事件
func CreateSignalEventHandler(deps SignalEventHandlerDeps) func(event SignalSSEvent) {
	accountConfig := ResolveSignalAccount(deps.CFG, deps.AccountID)
	dmPolicy := string(accountConfig.Config.DmPolicy)
	if dmPolicy == "" {
		dmPolicy = "allowlist"
	}
	groupPolicy := string(accountConfig.Config.GroupPolicy)
	if groupPolicy == "" {
		groupPolicy = "disabled"
	}

	allowFrom := channels.DeduplicateAllowlist(interfaceSliceToStringSlice(accountConfig.Config.AllowFrom))
	groupAllowFrom := channels.DeduplicateAllowlist(interfaceSliceToStringSlice(accountConfig.Config.GroupAllowFrom))
	reactionMode := string(accountConfig.Config.ReactionNotifications)
	reactionAllowlist := channels.DeduplicateAllowlist(interfaceSliceToStringSlice(accountConfig.Config.ReactionAllowlist))
	ignoreAttachments := false
	if accountConfig.Config.IgnoreAttachments != nil {
		ignoreAttachments = *accountConfig.Config.IgnoreAttachments
	}
	sendReadReceipts := true
	if accountConfig.Config.SendReadReceipts != nil {
		sendReadReceipts = *accountConfig.Config.SendReadReceipts
	}

	// 对齐 TS: 入站防抖器（per-key，EH-H1）
	debounceMs := autoreply.ResolveInboundDebounceMs(0, autoreply.DefaultDebounceMs)
	var debouncers sync.Map // map[string]*signalDebouncePending

	// flushDebounce 刷新指定 key 的防抖缓冲
	flushDebounce := func(key string) {
		val, ok := debouncers.LoadAndDelete(key)
		if !ok {
			return
		}
		pending := val.(*signalDebouncePending)
		pending.mu.Lock()
		if pending.timer != nil {
			pending.timer.Stop()
		}
		entries := pending.entries
		pending.mu.Unlock()

		if len(entries) == 0 {
			return
		}

		last := entries[len(entries)-1]
		if len(entries) == 1 {
			dispatchSignalInbound(deps, last)
			return
		}
		// 合并多条消息文本
		var texts []string
		for _, e := range entries {
			if e.BodyText != "" {
				texts = append(texts, e.BodyText)
			}
		}
		combinedText := strings.Join(texts, "\n")
		if strings.TrimSpace(combinedText) == "" {
			return
		}
		combined := last
		combined.BodyText = combinedText
		combined.MediaPath = ""
		combined.MediaType = ""
		dispatchSignalInbound(deps, combined)
	}

	return func(event SignalSSEvent) {
		if event.Event != "receive" || event.Data == "" {
			return
		}

		var payload SignalReceivePayload
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			deps.LogError(fmt.Sprintf("failed to parse event: %s", err))
			return
		}
		if payload.Exception != nil && payload.Exception.Message != "" {
			deps.LogError(fmt.Sprintf("receive exception: %s", payload.Exception.Message))
		}

		envelope := payload.Envelope
		if envelope == nil {
			return
		}
		// 忽略同步消息
		if envelope.SyncMessage != nil {
			return
		}

		sender := ResolveSignalSender(ptrStr(envelope.SourceNumber), ptrStr(envelope.SourceUuid))
		if sender == nil {
			return
		}

		// 排除自身消息
		if deps.Account != "" && sender.Kind == SignalSenderPhone {
			if sender.E164 == utils.NormalizeE164(deps.Account) {
				return
			}
		}

		// 解析 dataMessage（含 editMessage 回退）
		dataMessage := envelope.DataMessage
		if dataMessage == nil && envelope.EditMessage != nil {
			dataMessage = envelope.EditMessage.DataMessage
		}

		// 反应消息处理
		reaction := resolveReaction(envelope.ReactionMessage, dataMessage)
		messageText := ""
		if dataMessage != nil && dataMessage.Message != nil {
			messageText = strings.TrimSpace(*dataMessage.Message)
		}
		quoteText := ""
		if dataMessage != nil && dataMessage.Quote != nil && dataMessage.Quote.Text != nil {
			quoteText = strings.TrimSpace(*dataMessage.Quote.Text)
		}
		hasBodyContent := messageText != "" || quoteText != "" || (reaction == nil && dataMessage != nil && len(dataMessage.Attachments) > 0)

		// 反应消息处理分支
		if reaction != nil && !hasBodyContent {
			if ptrBool(reaction.IsRemove) {
				return // 忽略反应移除
			}
			emojiLabel := "emoji"
			if reaction.Emoji != nil && strings.TrimSpace(*reaction.Emoji) != "" {
				emojiLabel = strings.TrimSpace(*reaction.Emoji)
			}
			senderDisplay := FormatSignalSenderDisplay(sender)
			senderName := ptrStr(envelope.SourceName)
			if senderName == "" {
				senderName = senderDisplay
			}
			deps.LogInfo(fmt.Sprintf("signal reaction: %s from %s", emojiLabel, senderName))

			targets := ResolveReactionTargets(ptrStr(reaction.TargetAuthor), ptrStr(reaction.TargetAuthorUuid))
			shouldNotify := ShouldEmitSignalReactionNotification(
				reactionMode, deps.Account, targets, sender, reactionAllowlist,
			)
			if !shouldNotify {
				return
			}

			groupID := ""
			groupName := ""
			if reaction.GroupInfo != nil {
				groupID = ptrStr(reaction.GroupInfo.GroupID)
				groupName = ptrStr(reaction.GroupInfo.GroupName)
			}
			isGroup := groupID != ""

			messageID := "unknown"
			if reaction.TargetSentTimestamp != nil {
				messageID = fmt.Sprintf("%d", *reaction.TargetSentTimestamp)
			}

			var groupLabel string
			if groupID != "" {
				if groupName != "" {
					groupLabel = fmt.Sprintf("%s id:%s", groupName, groupID)
				} else {
					groupLabel = fmt.Sprintf("Signal Group id:%s", groupID)
				}
			}

			var targetLabel string
			if len(targets) > 0 {
				targetLabel = targets[0].Display
			}

			text := BuildSignalReactionSystemEventText(emojiLabel, senderName, messageID, targetLabel, groupLabel)
			deps.LogInfo(fmt.Sprintf("signal reaction event: %s", text))

			// SIG-A: 系统事件分发 — 反应通知
			if deps.Deps != nil && deps.Deps.EnqueueSystemEvent != nil {
				// 对齐 TS: contextKey = signal:reaction:added:messageId:senderId:emojiLabel:groupId
				senderId := FormatSignalSenderId(sender)
				contextKeyParts := []string{"signal", "reaction", "added", messageID, senderId, emojiLabel}
				if groupID != "" {
					contextKeyParts = append(contextKeyParts, groupID)
				}
				contextKey := strings.Join(contextKeyParts, ":")

				// 对齐 TS: 使用 agent route sessionKey
				senderPeerId := ResolveSignalPeerId(sender)
				peerKind := "direct"
				peerID := senderPeerId
				if isGroup {
					peerKind = "group"
					peerID = groupID
				}
				sessionKey := "signal:" + ResolveSignalRecipient(sender)
				if deps.Deps.ResolveAgentRoute != nil {
					if route, err := deps.Deps.ResolveAgentRoute(AgentRouteParams{
						Channel: "signal", AccountID: deps.AccountID,
						PeerKind: peerKind, PeerID: peerID,
					}); err == nil {
						sessionKey = route.SessionKey
					}
				}
				if err := deps.Deps.EnqueueSystemEvent(text, sessionKey, contextKey); err != nil {
					deps.LogError(fmt.Sprintf("signal: enqueue reaction event failed: %s", err))
				}
			}
			return
		}

		if dataMessage == nil {
			return
		}

		// 解析发送者信息
		senderDisplay := FormatSignalSenderDisplay(sender)
		senderRecipient := ResolveSignalRecipient(sender)
		senderPeerId := ResolveSignalPeerId(sender)
		senderAllowId := FormatSignalSenderId(sender)
		if senderRecipient == "" {
			return
		}

		groupID := ""
		groupName := ""
		if dataMessage.GroupInfo != nil {
			groupID = ptrStr(dataMessage.GroupInfo.GroupID)
			groupName = ptrStr(dataMessage.GroupInfo.GroupName)
		}
		isGroup := groupID != ""

		// 对齐 TS: 从 pairing store 读取动态白名单并合并
		var storeAllowFrom []string
		if deps.Deps != nil && deps.Deps.ReadAllowFromStore != nil {
			if sa, err := deps.Deps.ReadAllowFromStore("signal"); err == nil {
				storeAllowFrom = sa
			}
		}
		effectiveDmAllow := channels.DeduplicateAllowlist(append(allowFrom, storeAllowFrom...))
		effectiveGroupAllow := channels.DeduplicateAllowlist(append(groupAllowFrom, storeAllowFrom...))

		// DM 策略检查
		dmAllowed := dmPolicy == "open" || IsSignalSenderAllowed(sender, effectiveDmAllow)
		if !isGroup {
			if dmPolicy == "disabled" {
				return
			}
			if !dmAllowed {
				if dmPolicy == "pairing" {
					// SIG-B: 配对请求管理
					handleSignalPairing(deps, FormatSignalSenderId(sender), senderAllowId, senderRecipient)
				} else {
					deps.LogInfo(fmt.Sprintf("Blocked signal sender %s (dmPolicy=%s)", senderDisplay, dmPolicy))
				}
				return
			}
		}

		// 群组策略检查
		if isGroup && groupPolicy == "disabled" {
			deps.LogInfo("Blocked signal group message (groupPolicy: disabled)")
			return
		}
		if isGroup && groupPolicy == "allowlist" {
			if len(effectiveGroupAllow) == 0 {
				deps.LogInfo("Blocked signal group message (groupPolicy: allowlist, no groupAllowFrom)")
				return
			}
			if !IsSignalSenderAllowed(sender, effectiveGroupAllow) {
				deps.LogInfo(fmt.Sprintf("Blocked signal group sender %s (not in groupAllowFrom)", senderDisplay))
				return
			}
		}

		// 对齐 TS: 控制命令门控（EH-H3）
		useAccessGroups := true
		if deps.CFG != nil && deps.CFG.Commands != nil && deps.CFG.Commands.UseAccessGroups != nil && !*deps.CFG.Commands.UseAccessGroups {
			useAccessGroups = false
		}
		hasControlCmd := autoreply.HasControlCommand(messageText)
		commandGate := channels.ResolveControlCommandGate(channels.ControlCommandGateParams{
			UseAccessGroups: useAccessGroups,
			Authorizers: []channels.ControlCommandAuthorizer{
				{Configured: len(effectiveDmAllow) > 0, Allowed: IsSignalSenderAllowed(sender, effectiveDmAllow)},
				{Configured: len(effectiveGroupAllow) > 0, Allowed: IsSignalSenderAllowed(sender, effectiveGroupAllow)},
			},
			AllowTextCommands: true,
			HasControlCommand: hasControlCmd,
		})
		commandAuthorized := dmAllowed
		if isGroup {
			commandAuthorized = commandGate.CommandAuthorized
		}
		if isGroup && commandGate.ShouldBlock {
			deps.LogInfo(fmt.Sprintf("Blocked signal group command from %s (unauthorized)", senderDisplay))
			return
		}

		// 附件处理（对齐 TS: fetchAttachment — 含 size check + base64 decode + media save）
		var mediaPath, mediaType string
		mediaMaxBytes := ResolveMaxBytes(deps.CFG, 0, deps.AccountID)
		if !ignoreAttachments && len(dataMessage.Attachments) > 0 {
			first := dataMessage.Attachments[0]
			if first.ID != nil && *first.ID != "" {
				deps.LogInfo(fmt.Sprintf("signal: fetching attachment id=%s", *first.ID))
				// 对齐 TS: 先检查 attachment.size 是否超限
				if first.Size != nil && *first.Size > int64(mediaMaxBytes) {
					deps.LogError(fmt.Sprintf("signal: attachment %s exceeds %dMB limit",
						*first.ID, mediaMaxBytes/(1024*1024)))
				} else {
					path, ct, err := fetchSignalAttachment(deps.Ctx, deps.BaseURL, deps.Account,
						&first, senderRecipient, groupID, mediaMaxBytes)
					if err != nil {
						deps.LogError(fmt.Sprintf("signal: fetch attachment failed: %s", err))
					} else if path != "" {
						mediaPath = path
						mediaType = ct
					}
				}
			}
		}

		// 构建消息体
		bodyText := messageText
		if bodyText == "" {
			bodyText = quoteText
		}
		if bodyText == "" {
			return
		}

		// 对齐 TS: 仅在非 daemon 模式下手动发送已读回执
		if sendReadReceipts && !deps.ReadReceiptsViaDaemon && !isGroup {
			receiptTimestamp := ptrInt64(envelope.Timestamp)
			if receiptTimestamp == 0 && dataMessage.Timestamp != nil {
				receiptTimestamp = *dataMessage.Timestamp
			}
			if receiptTimestamp > 0 {
				// SIG-C: 发送已读回执
				if err := SendReadReceiptSignal(deps.Ctx, senderRecipient, receiptTimestamp, SignalSendOpts{
					BaseURL:   deps.BaseURL,
					Account:   deps.Account,
					AccountID: deps.AccountID,
				}); err != nil {
					deps.LogError(fmt.Sprintf("signal: send read receipt failed: %s", err))
				}
			}
		}

		senderName := ptrStr(envelope.SourceName)
		if senderName == "" {
			senderName = senderDisplay
		}
		messageID := ""
		if envelope.Timestamp != nil {
			messageID = fmt.Sprintf("%d", *envelope.Timestamp)
		}

		entry := SignalInboundEntry{
			SenderName:        senderName,
			SenderDisplay:     senderDisplay,
			SenderRecipient:   senderRecipient,
			SenderPeerId:      senderPeerId,
			GroupID:           groupID,
			GroupName:         groupName,
			IsGroup:           isGroup,
			BodyText:          bodyText,
			Timestamp:         ptrInt64(envelope.Timestamp),
			MessageID:         messageID,
			MediaPath:         mediaPath,
			MediaType:         mediaType,
			CommandAuthorized: commandAuthorized,
		}

		deps.LogInfo(fmt.Sprintf("signal inbound: from=%s len=%d group=%v",
			entry.SenderDisplay, len(entry.BodyText), entry.IsGroup))

		// 对齐 TS: 防抖判断（有控制命令或有媒体则直接分发，否则防抖）
		shouldDebounce := entry.BodyText != "" &&
			entry.MediaPath == "" && entry.MediaType == "" &&
			!autoreply.HasControlCommand(entry.BodyText)

		if !shouldDebounce || debounceMs <= 0 {
			dispatchSignalInbound(deps, entry)
			return
		}

		// 构建防抖 key（对齐 TS: signal:accountId:conversationId:senderPeerId）
		conversationID := entry.SenderPeerId
		if entry.IsGroup {
			conversationID = entry.GroupID
			if conversationID == "" {
				conversationID = "unknown"
			}
		}
		debounceKey := fmt.Sprintf("signal:%s:%s:%s", deps.AccountID, conversationID, entry.SenderPeerId)

		val, loaded := debouncers.LoadOrStore(debounceKey, &signalDebouncePending{
			entries: []SignalInboundEntry{entry},
		})
		pending := val.(*signalDebouncePending)

		if loaded {
			pending.mu.Lock()
			pending.entries = append(pending.entries, entry)
			if pending.timer != nil {
				pending.timer.Stop()
			}
			key := debounceKey
			pending.timer = time.AfterFunc(time.Duration(debounceMs)*time.Millisecond, func() {
				flushDebounce(key)
			})
			pending.mu.Unlock()
			return
		}

		pending.mu.Lock()
		key := debounceKey
		pending.timer = time.AfterFunc(time.Duration(debounceMs)*time.Millisecond, func() {
			flushDebounce(key)
		})
		pending.mu.Unlock()
	}
}

// signalDebouncePending 入站防抖缓冲（per-key）
type signalDebouncePending struct {
	mu      sync.Mutex
	entries []SignalInboundEntry
	timer   *time.Timer
}

// resolveReaction 解析有效的反应消息
func resolveReaction(envelopeReaction *SignalReactionMsg, dataMessage *SignalDataMessage) *SignalReactionMsg {
	if isValidReaction(envelopeReaction) {
		return envelopeReaction
	}
	if dataMessage != nil && isValidReaction(dataMessage.Reaction) {
		return dataMessage.Reaction
	}
	return nil
}

// isValidReaction 判断是否为有效的反应消息
func isValidReaction(r *SignalReactionMsg) bool {
	if r == nil {
		return false
	}
	return r.Emoji != nil && strings.TrimSpace(*r.Emoji) != "" && r.TargetSentTimestamp != nil
}

// interfaceSliceToStringSlice 将 []interface{} 转换为 []string
func interfaceSliceToStringSlice(items []interface{}) []string {
	var result []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// handleSignalPairing SIG-B: 配对请求管理
func handleSignalPairing(deps SignalEventHandlerDeps, sender, senderAllowId, senderRecipient string) {
	deps.LogInfo(fmt.Sprintf("signal pairing request sender=%s", senderAllowId))

	if deps.Deps == nil || deps.Deps.UpsertPairingRequest == nil {
		deps.LogInfo("signal: pairing needed but UpsertPairingRequest not available")
		return
	}

	meta := map[string]string{"sender": senderRecipient}
	result, err := deps.Deps.UpsertPairingRequest(PairingRequestParams{
		Channel: "signal",
		ID:      senderRecipient,
		Meta:    meta,
	})
	if err != nil {
		deps.LogInfo(fmt.Sprintf("signal: pairing upsert failed: %s", err))
		return
	}
	if result.Created {
		replyText := BuildPairingReply("signal",
			fmt.Sprintf("Your Signal sender id: %s", senderRecipient),
			result.Code)
		if _, err := SendMessageSignal(deps.Ctx, sender, replyText, SignalSendOpts{
			BaseURL:        deps.BaseURL,
			Account:        deps.Account,
			AccountID:      deps.AccountID,
			SkipFormatting: true,
		}); err != nil {
			deps.LogInfo(fmt.Sprintf("signal: pairing reply failed for %s: %s", senderRecipient, err))
		}
	}
}

// BuildPairingReply 构建配对回复消息
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

// dispatchSignalInbound SIG-A: 入站消息分发管线
// resolveAgentRoute → 构建 MsgContext → recordInboundSession → dispatchInboundMessage
func dispatchSignalInbound(deps SignalEventHandlerDeps, entry SignalInboundEntry) {
	if deps.Deps == nil || deps.Deps.ResolveAgentRoute == nil {
		deps.LogInfo("signal: inbound received but ResolveAgentRoute not available (DI stub)")
		return
	}

	// Agent 路由
	peerKind := "direct"
	peerID := entry.SenderRecipient
	if entry.IsGroup {
		peerKind = "group"
		peerID = entry.GroupID
		if peerID == "" {
			peerID = "unknown"
		}
	}

	route, err := deps.Deps.ResolveAgentRoute(AgentRouteParams{
		Channel:   "signal",
		AccountID: deps.AccountID,
		PeerKind:  peerKind,
		PeerID:    peerID,
	})
	if err != nil {
		deps.LogError(fmt.Sprintf("signal: resolve agent route failed: %s", err))
		return
	}

	// 构建 MsgContext
	signalTo := "signal:" + entry.SenderRecipient
	fromField := "signal:" + entry.SenderRecipient
	if entry.IsGroup {
		signalTo = "signal:group:" + entry.GroupID
		fromField = signalTo
	}

	fromLabel := entry.SenderDisplay
	if entry.IsGroup && entry.GroupName != "" {
		fromLabel = fmt.Sprintf("%s (group:%s)", entry.GroupName, entry.GroupID)
	}

	chatType := "direct"
	if entry.IsGroup {
		chatType = "group"
	}

	// 信封格式化
	body := formatSignalEnvelope("Signal", fromLabel, entry.Timestamp, entry.BodyText, chatType)

	// 对齐 TS: 群组历史上下文（EH-M2 / DY-S01）
	combinedBody := body
	historyKey := ""
	if entry.IsGroup {
		historyKey = entry.GroupID
		if historyKey == "" {
			historyKey = "unknown"
		}
	}
	if entry.IsGroup && historyKey != "" && deps.GroupHistories != nil && deps.HistoryLimit > 0 {
		historyEntry := &reply.HistoryEntry{
			Sender:    entry.SenderName,
			Body:      entry.BodyText,
			Timestamp: entry.Timestamp,
			MessageID: entry.MessageID,
		}
		combinedBody = reply.BuildHistoryContextFromMap(
			deps.GroupHistories, historyKey, deps.HistoryLimit,
			historyEntry, body,
			func(he reply.HistoryEntry) string {
				bodyWithID := he.Body
				if he.MessageID != "" {
					bodyWithID = fmt.Sprintf("%s [id:%s]", he.Body, he.MessageID)
				}
				return formatSignalEnvelope("Signal", fromLabel, he.Timestamp, bodyWithID, "group")
			},
			"\n", true,
		)
	}

	msgCtx := &autoreply.MsgContext{
		Body:               combinedBody,
		RawBody:            entry.BodyText,
		CommandBody:        entry.BodyText,
		From:               fromField,
		To:                 signalTo,
		SessionKey:         route.SessionKey,
		AccountID:          route.AccountID,
		ChatType:           chatType,
		ConversationLabel:  fromLabel,
		SenderName:         entry.SenderName,
		SenderID:           entry.SenderPeerId,
		Provider:           "signal",
		Surface:            "signal",
		IsGroup:            entry.IsGroup,
		OriginatingChannel: "signal",
		OriginatingTo:      signalTo,
		MessageSid:         entry.MessageID,
		CommandAuthorized:  entry.CommandAuthorized,
	}

	if entry.IsGroup && entry.GroupName != "" {
		msgCtx.GroupSubject = entry.GroupName
	}

	reply.FinalizeInboundContext(msgCtx, nil)

	// 会话记录
	if deps.Deps.RecordInboundSession != nil {
		var lastRoute *LastRouteUpdate
		if !entry.IsGroup {
			lastRoute = &LastRouteUpdate{
				SessionKey: route.MainSessionKey,
				Channel:    "signal",
				To:         entry.SenderRecipient,
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
			deps.LogInfo(fmt.Sprintf("signal: failed updating session meta: %s", err))
		}
	}

	// 对齐 TS: 发送打字状态（EH-M1）
	if msgCtx.To != "" {
		if err := SendTypingSignal(deps.Ctx, msgCtx.To, SignalSendOpts{
			BaseURL:   deps.BaseURL,
			Account:   deps.Account,
			AccountID: deps.AccountID,
		}); err != nil {
			deps.LogInfo(fmt.Sprintf("signal: typing indicator failed for %s: %s", msgCtx.To, err))
		}
	}

	// 分发到 auto-reply
	if deps.Deps.DispatchInboundMessage == nil {
		deps.LogInfo("signal: inbound processed but DispatchInboundMessage not available (DI stub)")
		return
	}

	result, err := deps.Deps.DispatchInboundMessage(deps.Ctx, DispatchParams{
		Ctx:        msgCtx,
		Dispatcher: nil,
	})
	if err != nil {
		deps.LogError(fmt.Sprintf("signal: dispatch inbound failed: %s", err))
	}

	// 对齐 TS: dispatch 成功后清除群组历史
	if entry.IsGroup && historyKey != "" && deps.GroupHistories != nil && deps.HistoryLimit > 0 {
		if result != nil && result.QueuedFinal {
			deps.GroupHistories.ClearEntries(historyKey)
		}
	}
}

// formatSignalEnvelope 格式化 Signal 入站消息信封
// 与 iMessage FormatInboundEnvelope 逻辑一致，避免跨频道包依赖
func formatSignalEnvelope(channel, from string, timestampMs int64, body, chatType string) string {
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

// fetchSignalAttachment 下载 Signal 附件（对齐 TS monitor.ts fetchAttachment）
// 通过 signal-cli RPC getAttachment 获取 base64 数据，解码后存储到本地媒体目录
func fetchSignalAttachment(ctx context.Context, baseURL, account string,
	att *SignalAttachment, sender, groupID string, maxBytes int) (string, string, error) {

	if att == nil || att.ID == nil || *att.ID == "" {
		return "", "", nil
	}

	// 对齐 TS: 构建 RPC 参数（含 sender/groupId 用于寻址）
	params := map[string]interface{}{
		"id": *att.ID,
	}
	if account != "" {
		params["account"] = account
	}
	if groupID != "" {
		params["groupId"] = groupID
	} else if sender != "" {
		params["recipient"] = sender
	} else {
		return "", "", nil
	}

	raw, err := SignalRpcRequest(ctx, baseURL, "getAttachment", params, "")
	if err != nil {
		return "", "", fmt.Errorf("getAttachment RPC: %w", err)
	}
	if raw == nil {
		return "", "", nil
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", "", fmt.Errorf("unmarshal attachment: %w", err)
	}
	if result.Data == "" {
		return "", "", nil
	}

	// base64 解码
	buffer, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return "", "", fmt.Errorf("decode base64 attachment: %w", err)
	}

	// 保存到媒体目录
	contentType := ptrStr(att.ContentType)
	saved, err := media.SaveMediaBuffer(buffer, contentType, "inbound", int64(maxBytes), "")
	if err != nil {
		return "", "", fmt.Errorf("save attachment: %w", err)
	}
	return saved.Path, saved.ContentType, nil
}
