package types

import "encoding/json"

// Channels 聚合配置类型 — 继承自 src/config/types.channels.ts (38 行)

// ChannelHeartbeatVisibilityConfig 频道心跳可见性配置
type ChannelHeartbeatVisibilityConfig struct {
	ShowOk       *bool `json:"showOk,omitempty"`       // 默认 false
	ShowAlerts   *bool `json:"showAlerts,omitempty"`   // 默认 true
	UseIndicator *bool `json:"useIndicator,omitempty"` // 默认 true
}

// ChannelDefaultsConfig 频道默认配置
type ChannelDefaultsConfig struct {
	GroupPolicy GroupPolicy                       `json:"groupPolicy,omitempty"`
	Heartbeat   *ChannelHeartbeatVisibilityConfig `json:"heartbeat,omitempty"`
}

// ChannelsConfig 全部频道聚合配置
// B3: 实现自定义 UnmarshalJSON/MarshalJSON 以支持 TS 的 [key: string]: unknown 索引签名
type ChannelsConfig struct {
	Defaults    *ChannelDefaultsConfig `json:"defaults,omitempty"`
	WhatsApp    *WhatsAppConfig        `json:"whatsapp,omitempty"`
	Telegram    *TelegramConfig        `json:"telegram,omitempty"`
	Discord     *DiscordConfig         `json:"discord,omitempty"`
	GoogleChat  *GoogleChatConfig      `json:"googlechat,omitempty"`
	Slack       *SlackConfig           `json:"slack,omitempty"`
	Signal      *SignalConfig          `json:"signal,omitempty"`
	IMessage    *IMessageConfig        `json:"imessage,omitempty"`
	MSTeams     *MSTeamsConfig         `json:"msteams,omitempty"`
	Feishu      *FeishuConfig          `json:"feishu,omitempty"`
	DingTalk    *DingTalkConfig        `json:"dingtalk,omitempty"`
	WeCom       *WeComConfig           `json:"wecom,omitempty"`
	WeChatMP    *WeChatMPConfig        `json:"wechat_mp,omitempty"`
	Xiaohongshu *XiaohongshuConfig     `json:"xiaohongshu,omitempty"`
	Website     *WebsiteConfig         `json:"website,omitempty"`
	Email       *EmailConfig           `json:"email,omitempty"`
	// Extra 存储未知频道的配置 (前向兼容插件频道)
	Extra map[string]interface{} `json:"-"`
}

// channelsConfigKnownKeys 已知频道配置键 — 与 TS types.channels.ts 对齐
var channelsConfigKnownKeys = map[string]bool{
	"defaults":    true,
	"whatsapp":    true,
	"telegram":    true,
	"discord":     true,
	"googlechat":  true,
	"slack":       true,
	"signal":      true,
	"imessage":    true,
	"msteams":     true,
	"feishu":      true,
	"dingtalk":    true,
	"wecom":       true,
	"wechat_mp":   true,
	"xiaohongshu": true,
	"website":     true,
	"email":       true,
}

// UnmarshalJSON 自定义反序列化：已知键解析到对应字段，未知键收集到 Extra
func (c *ChannelsConfig) UnmarshalJSON(data []byte) error {
	// 第一步：用别名类型解析已知字段，避免无限递归
	type Alias ChannelsConfig
	aux := &struct {
		*Alias
	}{Alias: (*Alias)(c)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// 第二步：解析全量键，过滤出未知键存入 Extra
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, val := range raw {
		if channelsConfigKnownKeys[key] {
			continue
		}
		if c.Extra == nil {
			c.Extra = make(map[string]interface{})
		}
		var parsed interface{}
		if err := json.Unmarshal(val, &parsed); err != nil {
			c.Extra[key] = string(val) // 解析失败则存为原始字符串
		} else {
			c.Extra[key] = parsed
		}
	}
	return nil
}

// MarshalJSON 自定义序列化：已知字段 + Extra 合并输出
func (c ChannelsConfig) MarshalJSON() ([]byte, error) {
	// 先序列化已知字段
	type Alias ChannelsConfig
	known, err := json.Marshal((*Alias)(&c))
	if err != nil {
		return known, err
	}
	if len(c.Extra) == 0 {
		return known, nil
	}
	// 合并 Extra 键到结果
	var base map[string]json.RawMessage
	if err := json.Unmarshal(known, &base); err != nil {
		return known, err
	}
	for key, val := range c.Extra {
		raw, err := json.Marshal(val)
		if err != nil {
			continue
		}
		base[key] = raw
	}
	return json.Marshal(base)
}
