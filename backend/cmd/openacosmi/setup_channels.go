package main

// setup_channels.go — 交互式频道设置向导
// TS 对照: src/commands/onboard-channels.ts (675L)
//
// 频道状态收集 → 状态展示 → 交互选择 → 每频道配置 → DM 策略

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/tui"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- 频道设置入口 ----------

// SetupChannels 交互式频道设置向导主函数。
// 对应 TS setupChannels (onboard-channels.ts)。
func SetupChannels(
	cfg *types.OpenAcosmiConfig,
	prompter tui.WizardPrompter,
	opts *channels.SetupChannelsOptions,
) (*types.OpenAcosmiConfig, error) {
	if opts == nil {
		opts = &channels.SetupChannelsOptions{}
	}

	// 1. 收集频道状态
	statuses := CollectChannelStatus(cfg, opts)

	// 2. 显示状态摘要
	if !opts.SkipStatusNote {
		NoteChannelStatus(prompter, statuses)
	}

	// 3. 构建选择选项
	options := BuildSelectionOptions(statuses, opts)
	if len(options) == 0 {
		prompter.Note(i18n.Tp("onboard.ch.all_set"), i18n.Tp("onboard.ch.title"))
		return cfg, nil
	}

	// 4. 构建默认选择（quickstart）
	defaults := ResolveQuickstartDefault(statuses, opts)

	// 5. 交互选择
	promptOptions := make([]tui.PromptOption, len(options))
	for i, opt := range options {
		promptOptions[i] = tui.PromptOption{
			Value: string(opt.Channel),
			Label: opt.Label,
			Hint:  opt.Hint,
		}
	}
	defaultValues := make([]string, len(defaults))
	for i, d := range defaults {
		defaultValues[i] = string(d)
	}

	selected, err := prompter.MultiSelect(i18n.Tp("onboard.ch.title"), promptOptions, defaultValues)
	if err != nil {
		return cfg, fmt.Errorf("channel selection: %w", err)
	}

	if len(selected) == 0 {
		prompter.Note(i18n.Tp("onboard.ch.none"), i18n.Tp("onboard.ch.title"))
		return cfg, nil
	}

	// 6. 逐频道配置
	nextConfig := cfg
	for _, chRaw := range selected {
		ch := channels.ChannelID(chRaw)
		result, err := HandleChannelChoice(nextConfig, prompter, ch, opts)
		if err != nil {
			slog.Warn("channel setup error", "channel", ch, "error", err)
			continue
		}
		nextConfig = result
	}

	// 7. DM 策略
	if !opts.SkipDmPolicyPrompt {
		nextConfig, err = MaybeConfigureDmPolicies(nextConfig, prompter, selected)
		if err != nil {
			return nextConfig, fmt.Errorf("dm policy: %w", err)
		}
	}

	return nextConfig, nil
}

// ---------- 频道状态收集 ----------

// ChannelStatusEntry 频道状态条目。
type ChannelStatusEntry struct {
	Channel    channels.ChannelID
	Label      string
	Configured bool
	StatusLine string
	Score      int // quickstart 得分
}

// CollectChannelStatus 收集所有已知频道的状态。
// 对应 TS collectChannelStatus (onboard-channels.ts)。
func CollectChannelStatus(cfg *types.OpenAcosmiConfig, opts *channels.SetupChannelsOptions) []ChannelStatusEntry {
	knownChannels := []struct {
		id    channels.ChannelID
		label string
	}{
		{channels.ChannelDiscord, "Discord"},
		{channels.ChannelTelegram, "Telegram"},
		{channels.ChannelSlack, "Slack"},
		{channels.ChannelWhatsApp, "WhatsApp"},
		{channels.ChannelSignal, "Signal"},
		{channels.ChannelWeb, "Web"},
		{channels.ChannelIMessage, "iMessage"},
		{channels.ChannelGoogleChat, "Google Chat"},
		{channels.ChannelMSTeams, "Microsoft Teams"},
	}

	var statuses []ChannelStatusEntry
	for _, ch := range knownChannels {
		configured := isChannelConfigured(cfg, ch.id)
		statusLine := "not configured"
		if configured {
			statusLine = "configured ✓"
		}
		score := channelQuickstartScore(ch.id)
		statuses = append(statuses, ChannelStatusEntry{
			Channel:    ch.id,
			Label:      ch.label,
			Configured: configured,
			StatusLine: statusLine,
			Score:      score,
		})
	}
	return statuses
}

// isChannelConfigured 检测频道是否在配置中存在。
func isChannelConfigured(cfg *types.OpenAcosmiConfig, ch channels.ChannelID) bool {
	if cfg == nil || cfg.Channels == nil {
		return false
	}
	cc := cfg.Channels
	switch ch {
	case channels.ChannelDiscord:
		return cc.Discord != nil
	case channels.ChannelTelegram:
		return cc.Telegram != nil
	case channels.ChannelSlack:
		return cc.Slack != nil
	case channels.ChannelWhatsApp:
		return cc.WhatsApp != nil
	case channels.ChannelSignal:
		return cc.Signal != nil
	case channels.ChannelIMessage:
		return cc.IMessage != nil
	case channels.ChannelGoogleChat:
		return cc.GoogleChat != nil
	case channels.ChannelMSTeams:
		return cc.MSTeams != nil
	case channels.ChannelWeb, channels.ChannelWebchat:
		// Web 频道检查 Extra
		if cc.Extra != nil {
			_, ok := cc.Extra[string(ch)]
			return ok
		}
		return false
	default:
		if cc.Extra != nil {
			_, ok := cc.Extra[string(ch)]
			return ok
		}
		return false
	}
}

// channelQuickstartScore 返回 quickstart 优先级分数（越高越推荐）。
func channelQuickstartScore(ch channels.ChannelID) int {
	scores := map[channels.ChannelID]int{
		channels.ChannelDiscord:  90,
		channels.ChannelSlack:    85,
		channels.ChannelTelegram: 80,
		channels.ChannelWeb:      70,
		channels.ChannelWhatsApp: 60,
		channels.ChannelSignal:   50,
	}
	if s, ok := scores[ch]; ok {
		return s
	}
	return 10
}

// ---------- 状态展示 ----------

// NoteChannelStatus 展示频道状态摘要。
// 对应 TS noteChannelStatus (onboard-channels.ts)。
func NoteChannelStatus(prompter tui.WizardPrompter, statuses []ChannelStatusEntry) {
	var lines []string
	for _, s := range statuses {
		icon := "○"
		if s.Configured {
			icon = "●"
		}
		lines = append(lines, fmt.Sprintf("%s %s — %s", icon, s.Label, s.StatusLine))
	}
	prompter.Note(strings.Join(lines, "\n"), i18n.Tp("onboard.ch.status_title"))
}

// ---------- 选项构建 ----------

// ChannelOption 频道选择选项。
type ChannelOption struct {
	Channel channels.ChannelID
	Label   string
	Hint    string
}

// BuildSelectionOptions 构建频道选择列表。
// 对应 TS buildSelectionOptions (onboard-channels.ts)。
func BuildSelectionOptions(statuses []ChannelStatusEntry, opts *channels.SetupChannelsOptions) []ChannelOption {
	var options []ChannelOption
	for _, s := range statuses {
		hint := ""
		if s.Configured {
			hint = "already configured"
		}
		options = append(options, ChannelOption{
			Channel: s.Channel,
			Label:   s.Label,
			Hint:    hint,
		})
	}
	return options
}

// ResolveQuickstartDefault 按 quickstartScore 排序，返回推荐默认选择。
// 对应 TS resolveQuickstartDefault (onboard-channels.ts)。
func ResolveQuickstartDefault(statuses []ChannelStatusEntry, opts *channels.SetupChannelsOptions) []channels.ChannelID {
	if len(opts.InitialSelection) > 0 {
		return opts.InitialSelection
	}

	if !opts.QuickstartDefaults {
		return nil
	}

	// 按分数排序
	sorted := make([]ChannelStatusEntry, len(statuses))
	copy(sorted, statuses)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// 取前 2 个未配置的
	var defaults []channels.ChannelID
	for _, s := range sorted {
		if !s.Configured && len(defaults) < 2 {
			defaults = append(defaults, s.Channel)
		}
	}
	return defaults
}

// ---------- 频道配置 ----------

// ConfiguredChannelAction 已配置频道操作类型。
// 对应 TS ConfiguredChannelAction (onboard-channels.ts L35)。
type ConfiguredChannelAction string

const (
	ConfiguredActionUpdate  ConfiguredChannelAction = "update"
	ConfiguredActionDisable ConfiguredChannelAction = "disable"
	ConfiguredActionDelete  ConfiguredChannelAction = "delete"
	ConfiguredActionSkip    ConfiguredChannelAction = "skip"
)

// HandleChannelChoice 处理单个频道配置选择。
// 如果频道已配置，委托给 HandleConfiguredChannel。
// 对应 TS handleChannelChoice (onboard-channels.ts L570-608)。
func HandleChannelChoice(
	cfg *types.OpenAcosmiConfig,
	prompter tui.WizardPrompter,
	ch channels.ChannelID,
	opts *channels.SetupChannelsOptions,
) (*types.OpenAcosmiConfig, error) {
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}

	// 如果频道已配置 → 委托给 HandleConfiguredChannel
	if isChannelConfigured(cfg, ch) {
		label := channelLabel(ch)
		return HandleConfiguredChannel(cfg, prompter, ch, label, opts)
	}

	return configureNewChannel(cfg, prompter, ch, opts)
}

// HandleConfiguredChannel 处理已配置频道的操作选择。
// 对应 TS handleConfiguredChannel (onboard-channels.ts L496-568)。
func HandleConfiguredChannel(
	cfg *types.OpenAcosmiConfig,
	prompter tui.WizardPrompter,
	ch channels.ChannelID,
	label string,
	opts *channels.SetupChannelsOptions,
) (*types.OpenAcosmiConfig, error) {
	// 构建操作选项
	actionOptions := []tui.PromptOption{
		{Value: string(ConfiguredActionUpdate), Label: "Modify settings"},
	}

	allowDisable := opts != nil && opts.AllowDisable
	if allowDisable {
		actionOptions = append(actionOptions,
			tui.PromptOption{Value: string(ConfiguredActionDisable), Label: "Disable (keeps config)"},
			tui.PromptOption{Value: string(ConfiguredActionDelete), Label: "Delete config"},
		)
	}
	actionOptions = append(actionOptions,
		tui.PromptOption{Value: string(ConfiguredActionSkip), Label: "Skip (leave as-is)"},
	)

	action, err := prompter.Select(
		i18n.Tp("onboard.ch.action"),
		actionOptions,
		string(ConfiguredActionUpdate),
	)
	if err != nil {
		return cfg, fmt.Errorf("configured channel action: %w", err)
	}

	switch ConfiguredChannelAction(action) {
	case ConfiguredActionSkip:
		return cfg, nil

	case ConfiguredActionUpdate:
		return configureNewChannel(cfg, prompter, ch, opts)

	case ConfiguredActionDisable:
		if !allowDisable {
			return cfg, nil
		}
		return disableChannel(cfg, ch)

	case ConfiguredActionDelete:
		if !allowDisable {
			return cfg, nil
		}
		confirmed, err := prompter.Confirm(
			i18n.Tf("onboard.ch.disable_confirm", label),
			false,
		)
		if err != nil || !confirmed {
			return cfg, err
		}
		return deleteChannelConfig(cfg, ch)

	default:
		return cfg, nil
	}
}

// configureNewChannel 配置新频道（或重新配置现有频道）。
// 当 prompter 非 nil 时，调用各频道的交互向导。
func configureNewChannel(cfg *types.OpenAcosmiConfig, prompter tui.WizardPrompter, ch channels.ChannelID, opts *channels.SetupChannelsOptions) (*types.OpenAcosmiConfig, error) {
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}

	accountID := channels.DefaultAccountID
	if opts != nil && opts.AccountIDs != nil {
		if id, ok := opts.AccountIDs[ch]; ok && id != "" {
			accountID = channels.NormalizeAccountID(id)
		}
	}

	// 如果有 prompter，使用交互向导
	if prompter != nil {
		var adapter channels.Prompter = newPrompterAdapter(prompter)
		switch ch {
		case channels.ChannelDiscord:
			result, err := channels.ConfigureDiscord(channels.ConfigureDiscordParams{
				Cfg: cfg, Prompter: adapter, AccountID: accountID,
			})
			if err != nil {
				return cfg, err
			}
			slog.Info("channel configured", "channel", ch, "accountId", result.AccountID)
			return result.Cfg, nil
		case channels.ChannelSlack:
			result, err := channels.ConfigureSlack(channels.ConfigureSlackParams{
				Cfg: cfg, Prompter: adapter, AccountID: accountID,
			})
			if err != nil {
				return cfg, err
			}
			slog.Info("channel configured", "channel", ch, "accountId", result.AccountID)
			return result.Cfg, nil
		case channels.ChannelTelegram:
			result, err := channels.ConfigureTelegram(channels.ConfigureTelegramParams{
				Cfg: cfg, Prompter: adapter, AccountID: accountID,
			})
			if err != nil {
				return cfg, err
			}
			slog.Info("channel configured", "channel", ch, "accountId", result.AccountID)
			return result.Cfg, nil
		case channels.ChannelWhatsApp:
			result, err := channels.ConfigureWhatsApp(channels.ConfigureWhatsAppParams{
				Cfg: cfg, Prompter: adapter, AccountID: accountID,
			})
			if err != nil {
				return cfg, err
			}
			slog.Info("channel configured", "channel", ch, "accountId", result.AccountID)
			return result.Cfg, nil
		case channels.ChannelSignal:
			result, err := channels.ConfigureSignal(channels.ConfigureSignalParams{
				Cfg: cfg, Prompter: adapter, AccountID: accountID,
			})
			if err != nil {
				return cfg, err
			}
			slog.Info("channel configured", "channel", ch, "accountId", result.AccountID)
			return result.Cfg, nil
		case channels.ChannelIMessage:
			result, err := channels.ConfigureIMessage(channels.ConfigureIMessageParams{
				Cfg: cfg, Prompter: adapter, AccountID: accountID,
			})
			if err != nil {
				return cfg, err
			}
			slog.Info("channel configured", "channel", ch, "accountId", result.AccountID)
			return result.Cfg, nil
		}
	}

	// 无 prompter 时（测试/非交互模式），仅启用频道
	enabledTrue := true

	switch ch {
	case channels.ChannelDiscord:
		if cfg.Channels.Discord == nil {
			cfg.Channels.Discord = &types.DiscordConfig{}
		}
		cfg.Channels.Discord.Enabled = &enabledTrue
	case channels.ChannelTelegram:
		if cfg.Channels.Telegram == nil {
			cfg.Channels.Telegram = &types.TelegramConfig{}
		}
		cfg.Channels.Telegram.Enabled = &enabledTrue
	case channels.ChannelSlack:
		if cfg.Channels.Slack == nil {
			cfg.Channels.Slack = &types.SlackConfig{}
		}
		cfg.Channels.Slack.Enabled = &enabledTrue
	case channels.ChannelWhatsApp:
		if cfg.Channels.WhatsApp == nil {
			cfg.Channels.WhatsApp = &types.WhatsAppConfig{}
		}
		cfg.Channels.WhatsApp.Enabled = &enabledTrue
	case channels.ChannelSignal:
		if cfg.Channels.Signal == nil {
			cfg.Channels.Signal = &types.SignalConfig{}
		}
		cfg.Channels.Signal.Enabled = &enabledTrue
	case channels.ChannelIMessage:
		if cfg.Channels.IMessage == nil {
			cfg.Channels.IMessage = &types.IMessageConfig{}
		}
		cfg.Channels.IMessage.Enabled = &enabledTrue
	case channels.ChannelGoogleChat:
		if cfg.Channels.GoogleChat == nil {
			cfg.Channels.GoogleChat = &types.GoogleChatConfig{}
		}
		cfg.Channels.GoogleChat.Enabled = &enabledTrue
	case channels.ChannelMSTeams:
		if cfg.Channels.MSTeams == nil {
			cfg.Channels.MSTeams = &types.MSTeamsConfig{}
		}
		cfg.Channels.MSTeams.Enabled = &enabledTrue
	default:
		if cfg.Channels.Extra == nil {
			cfg.Channels.Extra = make(map[string]interface{})
		}
		cfg.Channels.Extra[string(ch)] = map[string]interface{}{
			"enabled":   true,
			"accountId": accountID,
		}
	}

	slog.Info("channel configured", "channel", ch, "accountId", accountID)
	return cfg, nil
}

// ---------- Prompter 适配器 ----------

// prompterAdapter 将 tui.WizardPrompter 适配为 channels.Prompter。
// 桥接 tui.PromptOption ↔ channels.PromptOption 类型差异。
type prompterAdapter struct {
	inner tui.WizardPrompter
}

func newPrompterAdapter(p tui.WizardPrompter) *prompterAdapter {
	return &prompterAdapter{inner: p}
}

func (a *prompterAdapter) Intro(title string)         { a.inner.Intro(title) }
func (a *prompterAdapter) Outro(message string)       { a.inner.Outro(message) }
func (a *prompterAdapter) Note(message, title string) { a.inner.Note(message, title) }

func (a *prompterAdapter) Select(message string, options []channels.PromptOption, initialValue string) (string, error) {
	tuiOpts := make([]tui.PromptOption, len(options))
	for i, o := range options {
		tuiOpts[i] = tui.PromptOption{Value: o.Value, Label: o.Label, Hint: o.Hint}
	}
	return a.inner.Select(message, tuiOpts, initialValue)
}

func (a *prompterAdapter) MultiSelect(message string, options []channels.PromptOption, initialValues []string) ([]string, error) {
	tuiOpts := make([]tui.PromptOption, len(options))
	for i, o := range options {
		tuiOpts[i] = tui.PromptOption{Value: o.Value, Label: o.Label, Hint: o.Hint}
	}
	return a.inner.MultiSelect(message, tuiOpts, initialValues)
}

func (a *prompterAdapter) TextInput(message, placeholder, initial string, validate func(string) string) (string, error) {
	return a.inner.TextInput(message, placeholder, initial, validate)
}

func (a *prompterAdapter) Confirm(message string, initial bool) (bool, error) {
	return a.inner.Confirm(message, initial)
}

// disableChannel 禁用频道（保留配置）。
// 对应 TS handleConfiguredChannel "disable" 分支。
func disableChannel(cfg *types.OpenAcosmiConfig, ch channels.ChannelID) (*types.OpenAcosmiConfig, error) {
	if cfg.Channels == nil {
		return cfg, nil
	}
	enabledFalse := false

	switch ch {
	case channels.ChannelDiscord:
		if cfg.Channels.Discord != nil {
			cfg.Channels.Discord.Enabled = &enabledFalse
		}
	case channels.ChannelTelegram:
		if cfg.Channels.Telegram != nil {
			cfg.Channels.Telegram.Enabled = &enabledFalse
		}
	case channels.ChannelSlack:
		if cfg.Channels.Slack != nil {
			cfg.Channels.Slack.Enabled = &enabledFalse
		}
	case channels.ChannelWhatsApp:
		if cfg.Channels.WhatsApp != nil {
			cfg.Channels.WhatsApp.Enabled = &enabledFalse
		}
	case channels.ChannelSignal:
		if cfg.Channels.Signal != nil {
			cfg.Channels.Signal.Enabled = &enabledFalse
		}
	case channels.ChannelIMessage:
		if cfg.Channels.IMessage != nil {
			cfg.Channels.IMessage.Enabled = &enabledFalse
		}
	case channels.ChannelGoogleChat:
		if cfg.Channels.GoogleChat != nil {
			cfg.Channels.GoogleChat.Enabled = &enabledFalse
		}
	case channels.ChannelMSTeams:
		if cfg.Channels.MSTeams != nil {
			cfg.Channels.MSTeams.Enabled = &enabledFalse
		}
	}
	slog.Info("channel disabled", "channel", ch)
	return cfg, nil
}

// deleteChannelConfig 删除频道配置。
// 对应 TS handleConfiguredChannel "delete" 分支。
func deleteChannelConfig(cfg *types.OpenAcosmiConfig, ch channels.ChannelID) (*types.OpenAcosmiConfig, error) {
	if cfg.Channels == nil {
		return cfg, nil
	}
	switch ch {
	case channels.ChannelDiscord:
		cfg.Channels.Discord = nil
	case channels.ChannelTelegram:
		cfg.Channels.Telegram = nil
	case channels.ChannelSlack:
		cfg.Channels.Slack = nil
	case channels.ChannelWhatsApp:
		cfg.Channels.WhatsApp = nil
	case channels.ChannelSignal:
		cfg.Channels.Signal = nil
	case channels.ChannelIMessage:
		cfg.Channels.IMessage = nil
	case channels.ChannelGoogleChat:
		cfg.Channels.GoogleChat = nil
	case channels.ChannelMSTeams:
		cfg.Channels.MSTeams = nil
	default:
		if cfg.Channels.Extra != nil {
			delete(cfg.Channels.Extra, string(ch))
		}
	}
	slog.Info("channel config deleted", "channel", ch)
	return cfg, nil
}

// ---------- Channel Primer ----------

// NoteChannelPrimer 展示频道入门说明（DM 安全、配对机制）。
// 对应 TS noteChannelPrimer (onboard-channels.ts L179-204)。
func NoteChannelPrimer(prompter tui.WizardPrompter, statuses []ChannelStatusEntry) {
	var channelLines []string
	for _, s := range statuses {
		channelLines = append(channelLines, fmt.Sprintf("  • %s", s.Label))
	}

	lines := []string{
		"DM security: default is pairing; unknown DMs get a pairing code.",
		"Approve with: crabclaw pairing approve <channel> <code>",
		`Public DMs require dmPolicy="open" + allowFrom=["*"].`,
		`Multi-user DMs: set session.dmScope="per-channel-peer" to isolate sessions.`,
		"Docs: https://docs.openacosmi.dev/start/pairing",
		"",
	}
	lines = append(lines, channelLines...)

	prompter.Note(strings.Join(lines, "\n"), i18n.Tp("onboard.ch.howto_title"))
}

// ---------- DM 策略 ----------

// MaybeConfigureDmPolicies DM 策略配置交互。
// 对应 TS maybeConfigureDmPolicies (onboard-channels.ts)。
func MaybeConfigureDmPolicies(
	cfg *types.OpenAcosmiConfig,
	prompter tui.WizardPrompter,
	selectedChannels []string,
) (*types.OpenAcosmiConfig, error) {
	if len(selectedChannels) == 0 {
		return cfg, nil
	}

	// DM 策略选项
	policyOptions := []tui.PromptOption{
		{Value: string(channels.AccessPolicyAllowlist), Label: "Allowlist", Hint: "Only specified users can DM"},
		{Value: string(channels.AccessPolicyOpen), Label: "Open", Hint: "Anyone can DM the bot"},
		{Value: string(channels.AccessPolicyDisabled), Label: "Disabled", Hint: "No DMs allowed"},
	}

	selected, err := prompter.Select(i18n.Tp("onboard.ch.dm_policy"), policyOptions, string(channels.AccessPolicyAllowlist))
	if err != nil {
		return cfg, fmt.Errorf("dm policy selection: %w", err)
	}

	policy := channels.ChannelAccessPolicy(selected)

	// 如果 allowlist 策略，提示输入允许的用户
	var allowlistEntries []string
	if policy == channels.AccessPolicyAllowlist {
		input, err := prompter.TextInput(
			i18n.Tp("onboard.ch.dm_input"),
			"user1, user2",
			"",
			nil,
		)
		if err != nil {
			return cfg, fmt.Errorf("allowlist input: %w", err)
		}
		if strings.TrimSpace(input) != "" {
			allowlistEntries = channels.ParseAllowlistEntries(input)
		}
	}

	// 应用策略到 channels defaults
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.Defaults == nil {
		cfg.Channels.Defaults = &types.ChannelDefaultsConfig{}
	}

	// DM 策略设置到 defaults.groupPolicy
	cfg.Channels.Defaults.GroupPolicy = types.GroupPolicy(string(policy))

	// 记录 allowlist（如有）
	if len(allowlistEntries) > 0 {
		slog.Info("dm allowlist set", "entries", allowlistEntries)
	}

	return cfg, nil
}

// ---------- 内部辅助 ----------

func containsChannelID(ids []channels.ChannelID, target channels.ChannelID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// channelLabel 返回频道的显示标签。
func channelLabel(ch channels.ChannelID) string {
	labels := map[channels.ChannelID]string{
		channels.ChannelDiscord:    "Discord",
		channels.ChannelTelegram:   "Telegram",
		channels.ChannelSlack:      "Slack",
		channels.ChannelWhatsApp:   "WhatsApp",
		channels.ChannelSignal:     "Signal",
		channels.ChannelWeb:        "Web",
		channels.ChannelIMessage:   "iMessage",
		channels.ChannelGoogleChat: "Google Chat",
		channels.ChannelMSTeams:    "Microsoft Teams",
	}
	if l, ok := labels[ch]; ok {
		return l
	}
	return string(ch)
}
