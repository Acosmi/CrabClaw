package slack

// Slack 斜杠命令处理 — 继承自 src/slack/monitor/slash.ts (629L)
// 完整实现：DM 策略 + 频道访问控制 + 安全上下文构建 + 交互式菜单 +
// Native 命令注册 + 回复分发高级选项。

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply/reply"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/security"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- 常量 ----------
// TS 对照: slash.ts L44-45

const (
	slackCommandArgActionID    = "openacosmi_cmdarg"
	slackCommandArgValuePrefix = "cmdarg"
)

// ---------- 交互式参数菜单编解码 ----------
// TS 对照: slash.ts L58-110

// encodeSlackCommandArgValue 编码交互式菜单参数值。
// TS 对照: slash.ts L58-71
func encodeSlackCommandArgValue(command, arg, value, userID string) string {
	return strings.Join([]string{
		slackCommandArgValuePrefix,
		url.QueryEscape(command),
		url.QueryEscape(arg),
		url.QueryEscape(value),
		url.QueryEscape(userID),
	}, "|")
}

// SlackCommandArgParsed 解析后的参数菜单值。
type SlackCommandArgParsed struct {
	Command string
	Arg     string
	Value   string
	UserID  string
}

// parseSlackCommandArgValue 解析交互式菜单参数值。
// TS 对照: slash.ts L73-110
func parseSlackCommandArgValue(raw string) *SlackCommandArgParsed {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) != 5 || parts[0] != slackCommandArgValuePrefix {
		return nil
	}
	decode := func(s string) string {
		v, err := url.QueryUnescape(s)
		if err != nil {
			return ""
		}
		return v
	}
	command := decode(parts[1])
	arg := decode(parts[2])
	value := decode(parts[3])
	userID := decode(parts[4])
	if command == "" || arg == "" || value == "" || userID == "" {
		return nil
	}
	return &SlackCommandArgParsed{
		Command: command,
		Arg:     arg,
		Value:   value,
		UserID:  userID,
	}
}

// ---------- Block Kit 菜单构建 ----------
// TS 对照: slash.ts L47-140

// chunkItems 分块。
// TS 对照: slash.ts L47-56
func chunkItems[T any](items []T, size int) [][]T {
	if size <= 0 {
		return [][]T{items}
	}
	var rows [][]T
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		rows = append(rows, items[i:end])
	}
	return rows
}

// SlackArgChoice 参数菜单选项。
type SlackArgChoice struct {
	Value string
	Label string
}

// buildSlackCommandArgMenuBlocks 构建参数菜单 Block Kit 块。
// TS 对照: slash.ts L112-140
func buildSlackCommandArgMenuBlocks(title, command, arg string, choices []SlackArgChoice, userID string) []map[string]interface{} {
	var blocks []map[string]interface{}
	blocks = append(blocks, map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": title,
		},
	})

	rows := chunkItems(choices, 5)
	for _, row := range rows {
		var elements []map[string]interface{}
		for _, choice := range row {
			elements = append(elements, map[string]interface{}{
				"type":      "button",
				"action_id": slackCommandArgActionID,
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": choice.Label,
				},
				"value": encodeSlackCommandArgValue(command, arg, choice.Value, userID),
			})
		}
		blocks = append(blocks, map[string]interface{}{
			"type":     "actions",
			"elements": elements,
		})
	}
	return blocks
}

// ---------- Native 命令注册 (DY-031) ----------
// TS 对照: slash.ts L495-552

// SlackNativeCommandEntry 解析后的 native 命令条目。
type SlackNativeCommandEntry struct {
	Spec autoreply.NativeCommandSpec
}

// ResolveSlackNativeCommands 根据配置解析 Slack native 命令列表。
// TS 对照: slash.ts L495-509
// 返回 nil 表示 native 命令未启用或无可用命令。
func ResolveSlackNativeCommands(cfg *types.OpenAcosmiConfig, accountConfig *types.SlackAccountConfig) []autoreply.NativeCommandSpec {
	// 解析 provider 级和全局级 NativeCommandsSetting
	var providerNative config.NativeCommandsSetting
	var globalNative config.NativeCommandsSetting
	if accountConfig != nil && accountConfig.Commands != nil {
		providerNative = toNativeCommandsSetting(accountConfig.Commands.Native)
	}
	if cfg != nil && cfg.Commands != nil {
		globalNative = toNativeCommandsSetting(cfg.Commands.Native)
	}

	nativeEnabled := config.ResolveNativeCommandsEnabled("slack", providerNative, globalNative)
	if !nativeEnabled {
		return nil
	}

	// 构建 CommandsEnabledConfig（从全局 cfg.commands 提取）
	var cmdsCfg *autoreply.CommandsEnabledConfig
	if cfg != nil && cfg.Commands != nil {
		cmdsCfg = &autoreply.CommandsEnabledConfig{
			Config: cfg.Commands.Config,
			Debug:  cfg.Commands.Debug,
			Bash:   cfg.Commands.Bash,
		}
	}

	return autoreply.ListNativeCommandSpecsForConfig(cmdsCfg, "slack")
}

// toNativeCommandsSetting 将 types.NativeCommandsSetting (interface{}) 转换为 config.NativeCommandsSetting (*bool)。
// 复用 Discord 同名辅助函数的逻辑。
func toNativeCommandsSetting(v types.NativeCommandsSetting) config.NativeCommandsSetting {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case bool:
		return &val
	case *bool:
		return val
	default:
		return nil
	}
}

// resolveGlobalNativeSkillsSetting 从全局配置中获取 commands.nativeSkills 设置。
func resolveGlobalNativeSkillsSetting(cfg *types.OpenAcosmiConfig) config.NativeCommandsSetting {
	if cfg == nil || cfg.Commands == nil {
		return nil
	}
	return toNativeCommandsSetting(cfg.Commands.NativeSkills)
}

// ---------- Reply Delivery (DY-032) ----------
// TS 对照: replies.ts L121-166 deliverSlackSlashReplies

// SlackSlashReplyParams 斜杠命令回复投递参数。
type SlackSlashReplyParams struct {
	Replies   []autoreply.ReplyPayload
	Respond   func(text, responseType string) error // 封装 response_url 或 postEphemeral
	Ephemeral bool
	TextLimit int
	ChunkMode autoreply.ChunkMode
	TableMode types.MarkdownTableMode
}

// deliverSlackSlashReplies 投递斜杠命令回复（分块 + 表格模式 + 静默检测）。
// TS 对照: replies.ts L121-166
func deliverSlackSlashReplies(params SlackSlashReplyParams) error {
	var messages []string
	chunkLimit := params.TextLimit
	if chunkLimit <= 0 || chunkLimit > 4000 {
		chunkLimit = 4000
	}

	for _, payload := range params.Replies {
		textRaw := strings.TrimSpace(payload.Text)
		text := ""
		if textRaw != "" && !autoreply.IsSilentReplyText(textRaw) {
			text = textRaw
		}

		// 合并媒体 URL
		var mediaList []string
		if len(payload.MediaURLs) > 0 {
			mediaList = payload.MediaURLs
		} else if payload.MediaURL != "" {
			mediaList = []string{payload.MediaURL}
		}

		parts := []string{}
		if text != "" {
			parts = append(parts, text)
		}
		for _, u := range mediaList {
			trimmed := strings.TrimSpace(u)
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		combined := strings.Join(parts, "\n")
		if combined == "" {
			continue
		}

		// 分块处理
		// TS: chunkMode === "newline" ? chunkMarkdownTextWithMode(...) : [combined]
		var markdownChunks []string
		if params.ChunkMode == autoreply.ChunkModeNewline {
			markdownChunks = autoreply.ChunkMarkdownTextWithMode(combined, chunkLimit, autoreply.ChunkModeNewline)
		} else {
			markdownChunks = []string{combined}
		}

		// Markdown → Slack mrkdwn 转换 + 再分块
		for _, md := range markdownChunks {
			slackChunks := MarkdownToSlackMrkdwnChunks(md, chunkLimit)
			if len(slackChunks) == 0 && md != "" {
				slackChunks = []string{md}
			}
			messages = append(messages, slackChunks...)
		}
	}

	if len(messages) == 0 {
		return nil
	}

	responseType := "in_channel"
	if params.Ephemeral {
		responseType = "ephemeral"
	}

	for _, text := range messages {
		if err := params.Respond(text, responseType); err != nil {
			log.Printf("[slack] slash reply delivery failed: %v", err)
		}
	}
	return nil
}

// ResolveSlackMarkdownTableMode 解析 Slack 频道的 Markdown 表格渲染模式。
// TS 对照: config/markdown-tables.ts resolveMarkdownTableMode({cfg, channel: "slack", accountId})
// Slack 默认 "code"（不在 signal/whatsapp 特殊列表中）。
func ResolveSlackMarkdownTableMode(cfg *types.OpenAcosmiConfig, accountID string) types.MarkdownTableMode {
	defaultMode := types.MarkdownTableCode

	if cfg == nil {
		return defaultMode
	}

	// 从 cfg.channels.slack（或 cfg.slack）中按 accountId 级联查找
	slackCfg := resolveSlackChannelConfigSection(cfg)
	if slackCfg == nil {
		return defaultMode
	}

	// 账号级覆盖
	if accountID != "" && slackCfg.Accounts != nil {
		normalizedID := strings.ToLower(accountID)
		for key, acct := range slackCfg.Accounts {
			if strings.ToLower(key) == normalizedID && acct.Markdown != nil && acct.Markdown.Tables != "" {
				return acct.Markdown.Tables
			}
		}
	}

	// 频道级覆盖
	if slackCfg.Markdown != nil && slackCfg.Markdown.Tables != "" {
		return slackCfg.Markdown.Tables
	}

	return defaultMode
}

// slackMarkdownSection 用于解析 cfg 中 Slack 频道的 markdown 配置。
type slackMarkdownSection struct {
	Markdown *types.MarkdownConfig
	Accounts map[string]*types.SlackAccountConfig
}

// resolveSlackChannelConfigSection 从 cfg 中提取 Slack 频道配置段。
func resolveSlackChannelConfigSection(cfg *types.OpenAcosmiConfig) *slackMarkdownSection {
	if cfg == nil || cfg.Channels == nil || cfg.Channels.Slack == nil {
		return nil
	}
	slackCfg := cfg.Channels.Slack
	return &slackMarkdownSection{
		Markdown: slackCfg.Markdown,
		Accounts: slackCfg.Accounts,
	}
}

// ---------- HandleSlackSlashCommand 主函数 ----------

// HandleSlackSlashCommand 处理 Slack 斜杠命令。
// 完整实现 TS slash.ts L157-493 handleSlashCommand 的所有逻辑分支。
// 新增：commandDefinition + commandArgs 参数支持 native 命令解析（DY-031/033）。
func HandleSlackSlashCommand(ctx context.Context, monCtx *SlackMonitorContext, payload map[string]string) error {
	return handleSlackSlashCommandWithDef(ctx, monCtx, payload, nil, nil)
}

// HandleSlackSlashCommandWithNative 带命令定义的斜杠命令处理（native 命令专用入口）。
// TS 对照: slash.ts L157-493 handleSlashCommand with commandDefinition/commandArgs
func HandleSlackSlashCommandWithNative(
	ctx context.Context,
	monCtx *SlackMonitorContext,
	payload map[string]string,
	commandDef *autoreply.ChatCommandDefinition,
	commandArgs *autoreply.CommandArgs,
) error {
	return handleSlackSlashCommandWithDef(ctx, monCtx, payload, commandDef, commandArgs)
}

// handleSlackSlashCommandWithDef 内部实现。
func handleSlackSlashCommandWithDef(
	ctx context.Context,
	monCtx *SlackMonitorContext,
	payload map[string]string,
	commandDef *autoreply.ChatCommandDefinition,
	commandArgs *autoreply.CommandArgs,
) error {
	command := strings.TrimSpace(payload["command"])
	text := strings.TrimSpace(payload["text"])
	userID := payload["user_id"]
	userName := payload["user_name"]
	channelID := payload["channel_id"]
	channelName := payload["channel_name"]
	triggerID := payload["trigger_id"]
	responseURL := payload["response_url"]

	if command == "" {
		return nil
	}

	// TS: if (!prompt.trim()) ack({text: "Message required."})
	prompt := text
	if prompt == "" {
		prompt = command
	}

	log.Printf("[slack:%s] slash command: %s %s (user=%s channel=%s)",
		monCtx.AccountID, command, text, userID, channelID)

	// TS: if (ctx.botUserId && command.user_id === ctx.botUserId) return
	if monCtx.BotUserID != "" && userID == monCtx.BotUserID {
		return nil
	}

	// ---------- 频道类型推断 ----------
	// TS: slash.ts L180-187
	channelInfo := monCtx.resolveChannelInfo(channelID)
	var rawChannelType SlackChannelType
	if channelInfo != nil {
		rawChannelType = channelInfo.Type
	} else if channelName == "directmessage" {
		rawChannelType = SlackChannelTypeIM
	}
	channelType := NormalizeSlackChannelType(rawChannelType, channelID)
	isDirectMessage := channelType == SlackChannelTypeIM
	isGroupDm := channelType == SlackChannelTypeMPIM
	isRoom := channelType == SlackChannelTypeChannel || channelType == SlackChannelTypeGroup
	isRoomish := isRoom || isGroupDm

	// ---------- 频道允许检查 ----------
	// TS: slash.ts L189-201
	if !monCtx.IsChannelAllowed(channelID, channelType) {
		sendEphemeral(ctx, monCtx, channelID, userID, "This channel is not allowed.")
		return nil
	}

	// ---------- 动态 AllowFrom 合并 ----------
	// TS: slash.ts L203-205 storeAllowFrom + effectiveAllowFrom
	var storeAllowFrom []string
	if monCtx.Deps != nil && monCtx.Deps.ReadAllowFromStore != nil {
		if loaded, err := monCtx.Deps.ReadAllowFromStore("slack"); err == nil {
			storeAllowFrom = loaded
		}
	}
	mergedAllowFrom := append([]string{}, monCtx.AllowFrom...)
	mergedAllowFrom = append(mergedAllowFrom, storeAllowFrom...)
	_ = NormalizeAllowList(toInterfaceSlice(mergedAllowFrom))
	effectiveAllowFromLower := NormalizeAllowListLower(toInterfaceSlice(mergedAllowFrom))

	commandAuthorized := true
	var channelConfig *SlackChannelConfigResolved

	// ---------- DM 策略控制 ----------
	// TS: slash.ts L209-261
	if isDirectMessage {
		if !monCtx.DMEnabled || monCtx.DMPolicy == "disabled" {
			sendEphemeral(ctx, monCtx, channelID, userID, "Slack DMs are disabled.")
			return nil
		}
		if monCtx.DMPolicy != "open" {
			senderName := monCtx.ResolveUserName(userID)
			allowMatch := ResolveSlackAllowListMatch(effectiveAllowFromLower, userID, senderName)
			if !allowMatch.Allowed {
				if monCtx.DMPolicy == "pairing" {
					// TS: upsertChannelPairingRequest + buildPairingReply
					if monCtx.Deps != nil && monCtx.Deps.UpsertPairingRequest != nil {
						result, err := monCtx.Deps.UpsertPairingRequest(SlackPairingRequestParams{
							Channel: "slack",
							ID:      userID,
							Meta:    map[string]string{"name": senderName},
						})
						if err == nil && result.Created {
							log.Printf("[slack:%s] pairing request sender=%s name=%s",
								monCtx.AccountID, userID, senderName)
							pairingReply := buildSlackPairingReply(userID, result.Code)
							sendEphemeral(ctx, monCtx, channelID, userID, pairingReply)
						}
					}
				} else {
					log.Printf("[slack:%s] blocked slash sender %s (dmPolicy=%s)",
						monCtx.AccountID, userID, monCtx.DMPolicy)
					sendEphemeral(ctx, monCtx, channelID, userID, "You are not authorized to use this command.")
				}
				return nil
			}
			commandAuthorized = true
		}
	}

	// ---------- Room/Channel 访问控制 ----------
	// TS: slash.ts L263-340
	if isRoom {
		resolvedChannelName := ""
		if channelInfo != nil {
			resolvedChannelName = channelInfo.Name
		}
		channelConfig = ResolveSlackChannelConfig(
			channelID, resolvedChannelName,
			monCtx.ChannelConfigs, monCtx.RequireMention,
		)
		if monCtx.UseAccessGroups {
			channelAllowlistConfigured := len(monCtx.ChannelConfigs) > 0
			channelAllowed := channelConfig != nil && channelConfig.Allowed
			if !IsSlackChannelAllowedByPolicy(monCtx.GroupPolicy, channelAllowlistConfigured, channelAllowed) {
				sendEphemeral(ctx, monCtx, channelID, userID, "This channel is not allowed.")
				return nil
			}
			hasExplicitConfig := channelConfig != nil && channelConfig.MatchSource != ""
			if !channelAllowed && (monCtx.GroupPolicy != "open" || hasExplicitConfig) {
				sendEphemeral(ctx, monCtx, channelID, userID, "This channel is not allowed.")
				return nil
			}
		}
	}

	// ---------- 频道级用户白名单检查 ----------
	// TS: slash.ts L301-318
	senderName := monCtx.ResolveUserName(userID)
	if senderName == "" {
		senderName = userName
	}
	if senderName == "" {
		senderName = userID
	}

	channelUsersAllowlistConfigured := isRoom && channelConfig != nil &&
		len(channelConfig.Users) > 0
	channelUserAllowed := false
	if channelUsersAllowlistConfigured {
		channelUserAllowed = ResolveSlackUserAllowed(channelConfig.Users, userID, senderName)
	}
	if channelUsersAllowlistConfigured && !channelUserAllowed {
		sendEphemeral(ctx, monCtx, channelID, userID, "You are not authorized to use this command here.")
		return nil
	}

	// ---------- 多层授权合并 ----------
	// TS: slash.ts L320-340 resolveCommandAuthorizedFromAuthorizers
	ownerAllowed := ResolveSlackAllowListMatch(effectiveAllowFromLower, userID, senderName).Allowed
	if isRoomish {
		commandAuthorized = resolveSlackCommandAuthorized(
			monCtx.UseAccessGroups,
			len(effectiveAllowFromLower) > 0, ownerAllowed,
			channelUsersAllowlistConfigured, channelUserAllowed,
		)
		if monCtx.UseAccessGroups && !commandAuthorized {
			sendEphemeral(ctx, monCtx, channelID, userID, "You are not authorized to use this command.")
			return nil
		}
	}

	// ---------- DY-033: 交互式参数菜单呈现流 ----------
	// TS: slash.ts L342-366
	if commandDef != nil {
		menu := autoreply.ResolveCommandArgMenu(commandDef, commandArgs)
		if menu != nil {
			commandLabel := commandDef.NativeName
			if commandLabel == "" {
				commandLabel = commandDef.Key
			}
			title := menu.Title
			if title == "" {
				argDesc := menu.Arg.Description
				if argDesc == "" {
					argDesc = menu.Arg.Name
				}
				title = fmt.Sprintf("Choose %s for /%s.", argDesc, commandLabel)
			}
			choices := make([]SlackArgChoice, len(menu.Choices))
			for i, c := range menu.Choices {
				choices[i] = SlackArgChoice{Value: c.Value, Label: c.Label}
			}
			blocks := buildSlackCommandArgMenuBlocks(title, commandLabel, menu.Arg.Name, choices, userID)
			sendEphemeralBlocks(ctx, monCtx, channelID, userID, title, blocks)
			return nil
		}
	}

	// ---------- Agent 路由 ----------
	if monCtx.Deps == nil || monCtx.Deps.ResolveAgentRoute == nil {
		sendEphemeral(ctx, monCtx, channelID, userID, "Command not available (agent route not configured)")
		return nil
	}

	peerKind := "channel"
	peerID := channelID
	if isDirectMessage {
		peerKind = "direct"
		peerID = userID
	} else if isGroupDm {
		peerKind = "group"
	}

	route, err := monCtx.Deps.ResolveAgentRoute(SlackAgentRouteParams{
		Channel:   "slack",
		AccountID: monCtx.AccountID,
		PeerKind:  peerKind,
		PeerID:    peerID,
	})
	if err != nil {
		sendEphemeral(ctx, monCtx, channelID, userID, fmt.Sprintf("Route error: %v", err))
		return err
	}

	// ---------- 安全上下文构建 ----------
	// TS: slash.ts L381-435 — 完整字段对齐

	// From 地址 — 按 channelType 分 3 种格式
	// TS: slash.ts L399-403
	var from string
	switch {
	case isDirectMessage:
		from = "slack:" + userID
	case isRoom:
		from = "slack:channel:" + channelID
	default:
		from = "slack:group:" + channelID
	}

	// To 地址 — slash 命令固定为 "slash:{userId}"
	// TS: slash.ts L404
	to := "slash:" + userID

	// ChatType
	chatType := "channel"
	if isDirectMessage {
		chatType = "direct"
	}

	// Room label
	resolvedChName := ""
	if channelInfo != nil {
		resolvedChName = channelInfo.Name
	}
	roomLabel := "#" + channelID
	if resolvedChName != "" {
		roomLabel = "#" + resolvedChName
	}

	// UntrustedContext
	// TS: slash.ts L381-387 buildUntrustedChannelMetadata
	var untrustedContext []string
	if isRoomish && channelInfo != nil {
		metadata := security.BuildUntrustedChannelMetadata(
			"slack", "Slack channel description",
			[]string{channelInfo.Topic, channelInfo.Purpose}, nil,
		)
		if metadata != "" {
			untrustedContext = []string{metadata}
		}
	}

	// GroupSystemPrompt
	// TS: slash.ts L388-392
	var groupSystemPrompt string
	if isRoomish && channelConfig != nil {
		sp := strings.TrimSpace(channelConfig.SystemPrompt)
		if sp != "" {
			groupSystemPrompt = sp
		}
	}

	// SessionKey — TS: slash.ts L428
	// TS 格式: `agent:${route.agentId}:${slashCommand.sessionPrefix}:${command.user_id}`.toLowerCase()
	sessionPrefix := "slash"
	if monCtx.SlashCommand != nil && monCtx.SlashCommand.SessionPrefix != "" {
		sessionPrefix = monCtx.SlashCommand.SessionPrefix
	}
	sessionKey := strings.ToLower(fmt.Sprintf("agent:%s:%s:%s", route.AgentID, sessionPrefix, userID))

	// ConversationLabel
	// TS: slash.ts L406-416 resolveConversationLabel(...)
	var conversationLabel string
	if isDirectMessage {
		conversationLabel = senderName
	} else {
		conversationLabel = roomLabel
	}

	// WasMentioned — slash 命令始终为 true
	// TS: slash.ts L424
	wasMentioned := "true"

	// GroupSubject
	var groupSubject string
	if isRoomish {
		groupSubject = roomLabel
	}

	msgCtx := &autoreply.MsgContext{
		Body:                    prompt,
		RawBody:                 prompt,
		CommandBody:             prompt,
		From:                    from,
		To:                      to,
		SessionKey:              sessionKey,
		AccountID:               route.AccountID,
		ChatType:                chatType,
		ConversationLabel:       conversationLabel,
		SenderName:              senderName,
		SenderID:                userID,
		Provider:                "slack",
		Surface:                 "slack",
		IsGroup:                 !isDirectMessage,
		OriginatingChannel:      "slack",
		OriginatingTo:           "user:" + userID,
		CommandAuthorized:       commandAuthorized,
		CommandSource:           "native",
		CommandTargetSessionKey: route.SessionKey,
		WasMentioned:            wasMentioned,
		MessageSid:              triggerID,
		Timestamp:               time.Now().UnixMilli(),
		GroupSubject:            groupSubject,
		GroupSystemPrompt:       groupSystemPrompt,
		UntrustedContext:        untrustedContext,
	}

	// TS: CommandArgs 在 TS 中传入 finalizeInboundContext，Go 中 MsgContext 无此字段。
	// commandArgs 已在 prompt 构建阶段通过 BuildCommandTextFromArgs 处理。
	_ = commandArgs

	reply.FinalizeInboundContext(msgCtx, nil)

	// ---------- DY-032: Reply Delivery 高级选项 ----------
	// TS: slash.ts L444-485 dispatchReplyWithDispatcher + deliverSlackSlashReplies

	if monCtx.Deps.DispatchInboundMessage == nil {
		sendEphemeral(ctx, monCtx, channelID, userID, "Command received but dispatch not available")
		return nil
	}

	// 构建 respond 函数（封装 response_url 或 ephemeral fallback）
	respondFn := buildSlashRespondFunc(ctx, monCtx, channelID, userID, responseURL)

	// 解析 ephemeral 设置
	ephemeral := true
	if monCtx.SlashCommand != nil && monCtx.SlashCommand.Ephemeral != nil {
		ephemeral = *monCtx.SlashCommand.Ephemeral
	}

	// 解析 chunkMode 和 tableMode
	// TS: resolveChunkMode(cfg, "slack", route.accountId)
	chunkMode := autoreply.ResolveChunkMode(
		buildSlackProviderChunkConfig(monCtx.CFG),
		route.AccountID,
	)
	// TS: resolveMarkdownTableMode({cfg, channel: "slack", accountId: route.accountId})
	tableMode := ResolveSlackMarkdownTableMode(monCtx.CFG, route.AccountID)
	textLimit := monCtx.TextLimit
	if textLimit <= 0 {
		textLimit = 4000
	}

	// 构建 ReplyDispatcher，deliver 回调使用 deliverSlackSlashReplies
	dispatcher := reply.CreateReplyDispatcher(reply.ReplyDispatcherOptions{
		Deliver: func(replyPayload autoreply.ReplyPayload, kind reply.ReplyDispatchKind) error {
			return deliverSlackSlashReplies(SlackSlashReplyParams{
				Replies:   []autoreply.ReplyPayload{replyPayload},
				Respond:   respondFn,
				Ephemeral: ephemeral,
				TextLimit: textLimit,
				ChunkMode: chunkMode,
				TableMode: tableMode,
			})
		},
		OnError: func(err error, kind reply.ReplyDispatchKind) {
			log.Printf("[slack:%s] slash %s reply failed: %v", monCtx.AccountID, kind, err)
		},
	})
	defer dispatcher.Close()

	dispatchResult, dispatchErr := monCtx.Deps.DispatchInboundMessage(ctx, SlackDispatchParams{
		Ctx:        msgCtx,
		Dispatcher: dispatcher,
	})
	if dispatchErr != nil {
		_ = respondFn(fmt.Sprintf("Sorry, something went wrong handling that command: %v", dispatchErr), "ephemeral")
		return dispatchErr
	}

	// DY-032: 空回复检测
	// TS: if (counts.final + counts.tool + counts.block === 0)
	dispatcher.WaitForIdle()
	counts := dispatcher.GetQueuedCounts()
	totalCounts := counts[reply.DispatchFinal] + counts[reply.DispatchTool] + counts[reply.DispatchBlock]
	if totalCounts == 0 {
		// 发送空回复（TS: deliverSlackSlashReplies({ replies: [] })）
		// 如果 dispatch 也没 queuedFinal，发送一条提示
		if dispatchResult == nil || !dispatchResult.QueuedFinal {
			_ = respondFn("(No response)", "ephemeral")
		}
	}

	return nil
}

// buildSlashRespondFunc 构建斜杠命令 respond 函数。
// 优先使用 response_url（Slack 原生延迟回复），回退到 chat.postEphemeral。
func buildSlashRespondFunc(ctx context.Context, monCtx *SlackMonitorContext, channelID, userID, responseURL string) func(text, responseType string) error {
	if responseURL != "" {
		return func(text, responseType string) error {
			return postToResponseURL(ctx, responseURL, text, responseType)
		}
	}
	return func(text, responseType string) error {
		sendEphemeral(ctx, monCtx, channelID, userID, text)
		return nil
	}
}

// postToResponseURL 通过 Slack response_url 发送延迟回复。
// TS: respond({ text, response_type }) — Slack Bolt 封装
func postToResponseURL(ctx context.Context, responseURL, text, responseType string) error {
	if responseURL == "" || text == "" {
		return nil
	}
	payload := fmt.Sprintf(`{"text":%q,"response_type":%q}`, text, responseType)
	req, err := newJSONPostRequest(ctx, responseURL, payload)
	if err != nil {
		return err
	}
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("slack response_url post failed: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// buildSlackProviderChunkConfig 从全局 cfg 构建 Slack 的 ProviderChunkConfig。
// 用于 autoreply.ResolveChunkMode(cfg, "slack", accountId)。
func buildSlackProviderChunkConfig(cfg *types.OpenAcosmiConfig) *autoreply.ProviderChunkConfig {
	if cfg == nil || cfg.Channels == nil || cfg.Channels.Slack == nil {
		return nil
	}
	slackCfg := cfg.Channels.Slack
	result := &autoreply.ProviderChunkConfig{}
	if slackCfg.ChunkMode != "" {
		result.ChunkMode = autoreply.ChunkMode(slackCfg.ChunkMode)
	}
	if slackCfg.TextChunkLimit != nil && *slackCfg.TextChunkLimit > 0 {
		result.TextChunkLimit = *slackCfg.TextChunkLimit
	}
	if slackCfg.Accounts != nil {
		accts := make(map[string]autoreply.AccountChunkConfig)
		for id, acct := range slackCfg.Accounts {
			entry := autoreply.AccountChunkConfig{}
			if acct.TextChunkLimit != nil && *acct.TextChunkLimit > 0 {
				entry.TextChunkLimit = *acct.TextChunkLimit
			}
			accts[id] = entry
		}
		if len(accts) > 0 {
			result.Accounts = accts
		}
	}
	return result
}

// ---------- DY-031: Native 命令入口路由 ----------
// TS 对照: slash.ts L510-537 — for (const command of nativeCommands) { ... }

// HandleSlackNativeSlashCommand 处理 native 命令入口（命令名 → 定义 → 参数解析 → prompt 构建）。
// TS 对照: slash.ts L511-536
func HandleSlackNativeSlashCommand(
	ctx context.Context,
	monCtx *SlackMonitorContext,
	payload map[string]string,
	nativeCommandName string,
) error {
	commandDef := autoreply.FindCommandByNativeName(nativeCommandName, "slack")
	rawText := strings.TrimSpace(payload["text"])

	// 参数解析
	// TS: commandDefinition ? parseCommandArgs(commandDefinition, rawText) : rawText ? {raw: rawText} : undefined
	var cmdArgs *autoreply.CommandArgs
	if commandDef != nil {
		parsed := autoreply.ParseCommandArgs(commandDef, rawText)
		cmdArgs = &parsed
	} else if rawText != "" {
		cmdArgs = &autoreply.CommandArgs{Raw: rawText}
	}

	// prompt 构建
	// TS: commandDefinition ? buildCommandTextFromArgs(commandDefinition, commandArgs) : rawText ? `/${name} ${rawText}` : `/${name}`
	var prompt string
	if commandDef != nil {
		prompt = autoreply.BuildCommandTextFromArgs(commandDef, cmdArgs)
	} else if rawText != "" {
		prompt = "/" + nativeCommandName + " " + rawText
	} else {
		prompt = "/" + nativeCommandName
	}

	// 覆盖 payload 中的 text（使用解析后的 prompt 作为实际消息体）
	enrichedPayload := make(map[string]string, len(payload)+1)
	for k, v := range payload {
		enrichedPayload[k] = v
	}
	enrichedPayload["text"] = prompt

	return HandleSlackSlashCommandWithNative(ctx, monCtx, enrichedPayload, commandDef, cmdArgs)
}

// ---------- 辅助函数 ----------

// sendEphemeral 发送临时消息。
// TS: 内联 respond({ response_type: "ephemeral" })
func sendEphemeral(ctx context.Context, monCtx *SlackMonitorContext, channelID, userID, text string) {
	_, err := monCtx.Client.APICall(ctx, "chat.postEphemeral", map[string]interface{}{
		"channel": channelID,
		"user":    userID,
		"text":    text,
	})
	if err != nil {
		log.Printf("[slack:%s] ephemeral failed: %v", monCtx.AccountID, err)
	}
}

// sendEphemeralBlocks 发送带 Block Kit 的临时消息。
// TS: slash.ts L359-363 respond({ text, blocks, response_type: "ephemeral" })
func sendEphemeralBlocks(ctx context.Context, monCtx *SlackMonitorContext, channelID, userID, text string, blocks []map[string]interface{}) {
	payload := map[string]interface{}{
		"channel": channelID,
		"user":    userID,
		"text":    text,
	}
	if len(blocks) > 0 {
		payload["blocks"] = blocks
	}
	_, err := monCtx.Client.APICall(ctx, "chat.postEphemeral", payload)
	if err != nil {
		log.Printf("[slack:%s] ephemeral blocks failed: %v", monCtx.AccountID, err)
	}
}

// buildSlackPairingReply 构建配对回复消息。
// TS 对照: pairing/pairing-messages.ts buildPairingReply
func buildSlackPairingReply(userID, code string) string {
	lines := []string{
		"👋 Hi! This Slack account is paired with Crab Claw（蟹爪）.",
		"",
		fmt.Sprintf("Your Slack user id: %s", userID),
		fmt.Sprintf("Your pairing code: %s", code),
		"",
		"To approve, run: /pair approve <code>",
	}
	return strings.Join(lines, "\n")
}

// resolveSlackCommandAuthorized 多层授权合并。
// TS 对照: channels/command-gating.ts resolveCommandAuthorizedFromAuthorizers
// 复用 Discord 已实现的 commandAuthorizer 模式。
func resolveSlackCommandAuthorized(useAccessGroups bool, ownerConfigured, ownerAllowed, channelUsersConfigured, channelUserAllowed bool) bool {
	if !useAccessGroups {
		// TS: modeWhenAccessGroupsOff default "allow"
		return true
	}
	// useAccessGroups == true: any configured+allowed → authorized
	authorizers := []struct {
		configured bool
		allowed    bool
	}{
		{ownerConfigured, ownerAllowed},
		{channelUsersConfigured, channelUserAllowed},
	}
	for _, a := range authorizers {
		if a.configured && a.allowed {
			return true
		}
	}
	return false
}

// toInterfaceSlice 将 []string 转为 []interface{}。
func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

// HandleSlackCommandArgAction 处理交互式参数菜单的 action 回调。
// TS 对照: slash.ts L558-628 registerArgAction
// DY-033: 使用 HandleSlackSlashCommandWithNative 传入解析后的 commandDef + commandArgs。
func HandleSlackCommandArgAction(ctx context.Context, monCtx *SlackMonitorContext, actionValue string, actionUserID string, channelID string) error {
	parsed := parseSlackCommandArgValue(actionValue)
	if parsed == nil {
		sendEphemeral(ctx, monCtx, channelID, actionUserID, "Sorry, that button is no longer valid.")
		return nil
	}
	// TS: if (body.user?.id && parsed.userId !== body.user.id) — 用户校验
	if actionUserID != "" && parsed.UserID != actionUserID {
		sendEphemeral(ctx, monCtx, channelID, actionUserID, "That menu is for another user.")
		return nil
	}

	// TS: slash.ts L596-602 — 使用 findCommandByNativeName 查找定义，构建 commandArgs
	commandDef := autoreply.FindCommandByNativeName(parsed.Command, "slack")
	cmdArgs := &autoreply.CommandArgs{
		Values: autoreply.CommandArgValues{parsed.Arg: parsed.Value},
	}

	// 构建 prompt
	var prompt string
	if commandDef != nil {
		prompt = autoreply.BuildCommandTextFromArgs(commandDef, cmdArgs)
	} else {
		prompt = "/" + parsed.Command + " " + parsed.Value
	}

	return HandleSlackSlashCommandWithNative(ctx, monCtx, map[string]string{
		"command":      "/" + parsed.Command,
		"text":         prompt,
		"user_id":      parsed.UserID,
		"channel_id":   channelID,
		"trigger_id":   fmt.Sprintf("cmdarg-%d", time.Now().UnixMilli()),
		"response_url": "", // action 回调无 response_url
	}, commandDef, cmdArgs)
}

// ---------- HTTP 辅助 ----------

// newJSONPostRequest 创建 JSON POST 请求。
func newJSONPostRequest(ctx context.Context, rawURL, body string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// defaultHTTPClient 返回默认 HTTP 客户端。
func defaultHTTPClient() *http.Client {
	return http.DefaultClient
}
