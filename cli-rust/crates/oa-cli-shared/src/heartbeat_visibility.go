package infra

// ---------- 心跳可见性配置 ----------

// ResolvedHeartbeatVisibility 解析后的心跳可见性设置。
type ResolvedHeartbeatVisibility struct {
	ShowOk       bool `json:"showOk"`
	ShowAlerts   bool `json:"showAlerts"`
	UseIndicator bool `json:"useIndicator"`
}

// ChannelHeartbeatVisibilityConfig 频道心跳可见性配置。
type ChannelHeartbeatVisibilityConfig struct {
	ShowOk       *bool `json:"showOk,omitempty"`
	ShowAlerts   *bool `json:"showAlerts,omitempty"`
	UseIndicator *bool `json:"useIndicator,omitempty"`
}

// 默认可见性设置。
var defaultHeartbeatVisibility = ResolvedHeartbeatVisibility{
	ShowOk:       false, // 默认静默
	ShowAlerts:   true,  // 显示内容消息
	UseIndicator: true,  // 发出指示器事件
}

// ---------- 可见性解析 ----------

// HeartbeatVisibilityConfig 心跳可见性解析所需的配置接口。
// 通过接口解耦与 OpenAcosmiConfig 的直接依赖。
type HeartbeatVisibilityConfig interface {
	// ChannelDefaultsHeartbeat 返回全局频道默认心跳可见性配置。
	ChannelDefaultsHeartbeat() *ChannelHeartbeatVisibilityConfig
	// PerChannelHeartbeat 返回特定频道的心跳可见性配置。
	PerChannelHeartbeat(channel string) *ChannelHeartbeatVisibilityConfig
	// PerAccountHeartbeat 返回特定频道+account 的心跳可见性配置。
	PerAccountHeartbeat(channel, accountID string) *ChannelHeartbeatVisibilityConfig
}

// ResolveHeartbeatVisibility 解析心跳可见性设置。
// 优先级: perAccount > perChannel > channelDefaults > globalDefaults
func ResolveHeartbeatVisibility(cfg HeartbeatVisibilityConfig, channel, accountID string) ResolvedHeartbeatVisibility {
	// webchat 仅用 channel defaults
	if channel == "webchat" {
		channelDefaults := cfg.ChannelDefaultsHeartbeat()
		return mergeVisibility(channelDefaults, nil, nil)
	}

	channelDefaults := cfg.ChannelDefaultsHeartbeat()
	perChannel := cfg.PerChannelHeartbeat(channel)
	var perAccount *ChannelHeartbeatVisibilityConfig
	if accountID != "" {
		perAccount = cfg.PerAccountHeartbeat(channel, accountID)
	}

	return mergeVisibility(channelDefaults, perChannel, perAccount)
}

// mergeVisibility 按优先级合并可见性设置。
// precedence: perAccount > perChannel > channelDefaults > default
func mergeVisibility(
	channelDefaults, perChannel, perAccount *ChannelHeartbeatVisibilityConfig,
) ResolvedHeartbeatVisibility {
	return ResolvedHeartbeatVisibility{
		ShowOk:       resolveBoolChain(defaultHeartbeatVisibility.ShowOk, channelDefaults, perChannel, perAccount, func(c *ChannelHeartbeatVisibilityConfig) *bool { return c.ShowOk }),
		ShowAlerts:   resolveBoolChain(defaultHeartbeatVisibility.ShowAlerts, channelDefaults, perChannel, perAccount, func(c *ChannelHeartbeatVisibilityConfig) *bool { return c.ShowAlerts }),
		UseIndicator: resolveBoolChain(defaultHeartbeatVisibility.UseIndicator, channelDefaults, perChannel, perAccount, func(c *ChannelHeartbeatVisibilityConfig) *bool { return c.UseIndicator }),
	}
}

// resolveBoolChain 沿优先级链解析布尔值。
// 从最高优先级 (perAccount) 到最低 (default) 依次查找非 nil 值。
func resolveBoolChain(
	defaultVal bool,
	channelDefaults, perChannel, perAccount *ChannelHeartbeatVisibilityConfig,
	getter func(*ChannelHeartbeatVisibilityConfig) *bool,
) bool {
	// 最高优先级: perAccount
	if perAccount != nil {
		if v := getter(perAccount); v != nil {
			return *v
		}
	}
	// perChannel
	if perChannel != nil {
		if v := getter(perChannel); v != nil {
			return *v
		}
	}
	// channelDefaults
	if channelDefaults != nil {
		if v := getter(channelDefaults); v != nil {
			return *v
		}
	}
	return defaultVal
}
