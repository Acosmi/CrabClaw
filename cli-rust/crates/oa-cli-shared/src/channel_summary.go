package infra

// channel_summary.go — 通道配置摘要
// 对应 TS: src/infra/channel-summary.ts
//
// 从结构化配置中提取已激活通道的摘要信息，
// 供 doctor 命令、status 查询和监控仪表板使用。

// ChannelSummary 单个通道的简要信息。
type ChannelSummary struct {
	// Type 通道类型（discord/telegram/line/slack/imessage/whatsapp/signal/web）。
	Type string `json:"type"`
	// Label 人类可读标签（如 Bot 用户名或渠道名称）。
	Label string `json:"label,omitempty"`
	// Enabled 通道是否已配置且启用。
	Enabled bool `json:"enabled"`
	// Connected 通道当前是否已连接（仅运行时可知，CLI 检查时为 false）。
	Connected bool `json:"connected,omitempty"`
}

// ChannelRawConfig 从 openacosmi.json 解析出的通道原始配置（最小化结构）。
// 使用 any 字段以兼容不同版本的通道配置格式。
type ChannelRawConfig struct {
	Discord  *ChannelEntry `json:"discord,omitempty"`
	Telegram *ChannelEntry `json:"telegram,omitempty"`
	LINE     *ChannelEntry `json:"line,omitempty"`
	Slack    *ChannelEntry `json:"slack,omitempty"`
	IMessage *ChannelEntry `json:"imessage,omitempty"`
	WhatsApp *ChannelEntry `json:"whatsapp,omitempty"`
	Signal   *ChannelEntry `json:"signal,omitempty"`
	Web      *ChannelEntry `json:"web,omitempty"`
}

// ChannelEntry 通道配置条目（公共字段）。
type ChannelEntry struct {
	// Enabled 显式禁用字段（缺省视为已启用）。
	Enabled *bool `json:"enabled,omitempty"`
	// Token/BotToken/APIKey 等凭证字段（用于判断是否已配置）。
	Token    string `json:"token,omitempty"`
	BotToken string `json:"botToken,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
	// AccountPhone 用于 WhatsApp/Signal。
	AccountPhone string `json:"accountPhone,omitempty"`
	// Handle 用于 iMessage。
	Handle string `json:"handle,omitempty"`
}

// isEntryEnabled 判断通道条目是否启用。
func isEntryEnabled(entry *ChannelEntry) bool {
	if entry == nil {
		return false
	}
	if entry.Enabled != nil && !*entry.Enabled {
		return false
	}
	// 有任意凭证字段即视为配置过
	return entry.Token != "" ||
		entry.BotToken != "" ||
		entry.APIKey != "" ||
		entry.AccountPhone != "" ||
		entry.Handle != ""
}

// SummarizeChannels 将通道原始配置转换为摘要列表。
// 对应 TS: summarizeChannels(cfg)
func SummarizeChannels(raw *ChannelRawConfig) []ChannelSummary {
	if raw == nil {
		return nil
	}

	var summaries []ChannelSummary

	add := func(channelType string, entry *ChannelEntry, labelFn func(*ChannelEntry) string) {
		if entry == nil {
			return
		}
		label := ""
		if labelFn != nil {
			label = labelFn(entry)
		}
		summaries = append(summaries, ChannelSummary{
			Type:    channelType,
			Label:   label,
			Enabled: isEntryEnabled(entry),
		})
	}

	add("discord", raw.Discord, func(e *ChannelEntry) string { return e.Token })
	add("telegram", raw.Telegram, func(e *ChannelEntry) string { return e.BotToken })
	add("line", raw.LINE, func(e *ChannelEntry) string { return "" })
	add("slack", raw.Slack, func(e *ChannelEntry) string { return "" })
	add("imessage", raw.IMessage, func(e *ChannelEntry) string { return e.Handle })
	add("whatsapp", raw.WhatsApp, func(e *ChannelEntry) string { return e.AccountPhone })
	add("signal", raw.Signal, func(e *ChannelEntry) string { return e.AccountPhone })
	add("web", raw.Web, func(e *ChannelEntry) string { return "" })

	return summaries
}

// EnabledChannelTypes 返回已启用通道的类型列表。
// 对应 TS: getEnabledChannelTypes(cfg)
func EnabledChannelTypes(raw *ChannelRawConfig) []string {
	summaries := SummarizeChannels(raw)
	var types []string
	for _, s := range summaries {
		if s.Enabled {
			types = append(types, s.Type)
		}
	}
	return types
}

// HasAnyChannel 判断是否至少配置了一个通道。
func HasAnyChannel(raw *ChannelRawConfig) bool {
	return len(EnabledChannelTypes(raw)) > 0
}
