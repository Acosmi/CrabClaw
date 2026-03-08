package channels

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// Slack 引导适配器 — 继承自 src/channels/plugins/onboarding/slack.ts (545L)

// SlackRequiredBotScopes Slack Bot 所需的 OAuth scopes
var SlackRequiredBotScopes = []string{
	"app_mentions:read",
	"channels:history",
	"channels:read",
	"chat:write",
	"groups:history",
	"groups:read",
	"im:history",
	"im:read",
	"im:write",
	"mpim:history",
	"mpim:read",
	"reactions:read",
	"reactions:write",
	"users:read",
	"files:read",
	"files:write",
}

// SlackRequiredEvents Slack 所需的事件订阅
var SlackRequiredEvents = []string{
	"app_mention",
	"message.channels",
	"message.groups",
	"message.im",
	"message.mpim",
	"reaction_added",
}

// SlackDmPolicyInfo Slack DM 策略元数据
var SlackDmPolicyInfo = struct {
	Label        string
	PolicyKey    string
	AllowFromKey string
}{
	Label:        "Slack",
	PolicyKey:    "channels.slack.dm.policy",
	AllowFromKey: "channels.slack.dm.allowFrom",
}

// BuildSlackAppManifest 构建 Slack App Manifest JSON。
// 对应 TS buildSlackManifest (slack.ts L44-98)。
func BuildSlackAppManifest(botName string) string {
	if botName == "" {
		botName = "Crab Claw"
	}
	manifest := map[string]interface{}{
		"display_information": map[string]interface{}{
			"name":        botName,
			"description": "AI assistant powered by Crab Claw（蟹爪）",
		},
		"features": map[string]interface{}{
			"bot_user": map[string]interface{}{
				"display_name":           botName,
				"always_online":          true,
				"direct_messages_tab":    true,
				"messages_tab_read_only": false,
			},
		},
		"oauth_config": map[string]interface{}{
			"scopes": map[string]interface{}{
				"bot": SlackRequiredBotScopes,
			},
		},
		"settings": map[string]interface{}{
			"event_subscriptions": map[string]interface{}{
				"bot_events": SlackRequiredEvents,
			},
			"interactivity": map[string]interface{}{
				"is_enabled": false,
			},
			"org_deploy_enabled":     false,
			"socket_mode_enabled":    true,
			"token_rotation_enabled": false,
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	return string(data)
}

// BuildSlackOnboardingStatus 构建 Slack 引导状态
func BuildSlackOnboardingStatus(configured bool) OnboardingStatus {
	statusStr := "needs tokens"
	if configured {
		statusStr = "configured"
	}
	score := 1
	hint := "needs tokens"
	if configured {
		hint = "configured"
		score = 2
	}
	return OnboardingStatus{
		Channel:    ChannelSlack,
		Configured: configured,
		StatusLines: []string{
			fmt.Sprintf("Slack: %s", statusStr),
		},
		SelectionHint:   hint,
		QuickstartScore: &score,
	}
}

// ---------- 交互向导 ----------

// ConfigureSlackParams Slack 配置参数。
type ConfigureSlackParams struct {
	Cfg       *types.OpenAcosmiConfig
	Prompter  Prompter
	AccountID string
}

// ConfigureSlackResult Slack 配置结果。
type ConfigureSlackResult struct {
	Cfg       *types.OpenAcosmiConfig
	AccountID string
}

// NoteSlackTokenHelp 展示 Slack token 帮助文本。
func NoteSlackTokenHelp(prompter Prompter, botName string) {
	if botName == "" {
		botName = "Crab Claw"
	}
	prompter.Note(strings.Join([]string{
		"1) api.slack.com/apps → Create New App → From an app manifest",
		"2) Paste the manifest JSON shown above",
		"3) Install → copy Bot Token (xoxb-...)",
		"4) Basic Information → App-Level Tokens → Generate (scope: connections:write)",
		"5) Copy App Token (xapp-...)",
		"Docs: https://docs.openacosmi.dev/slack",
	}, "\n"), i18n.Tp("onboard.ch.slack.title"))
}

// ConfigureSlack 交互式 Slack 频道配置向导。
// 对应 TS slackOnboardingAdapter.configure (slack.ts L270-520)。
func ConfigureSlack(params ConfigureSlackParams) (*ConfigureSlackResult, error) {
	cfg := params.Cfg
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	prompter := params.Prompter
	accountID := params.AccountID
	if accountID == "" {
		accountID = DefaultAccountID
	}

	// 确保 channels.slack 结构存在
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.Slack == nil {
		cfg.Channels.Slack = &types.SlackConfig{}
	}

	enabledTrue := true
	cfg.Channels.Slack.Enabled = &enabledTrue

	// Bot name
	botName := cfg.Channels.Slack.Name
	if botName == "" {
		botName = "Crab Claw"
	}

	// 展示 manifest
	manifestJSON := BuildSlackAppManifest(botName)
	prompter.Note(manifestJSON, i18n.Tp("onboard.ch.slack.manifest"))

	// 检测已有 token
	hasConfigBotToken := cfg.Channels.Slack.BotToken != ""
	hasConfigAppToken := cfg.Channels.Slack.AppToken != ""
	envBotToken := strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN"))
	envAppToken := strings.TrimSpace(os.Getenv("SLACK_APP_TOKEN"))

	// Bot Token
	var botToken string
	if envBotToken != "" && !hasConfigBotToken && accountID == DefaultAccountID {
		keepEnv, err := prompter.Confirm(i18n.Tp("onboard.ch.slack.bot_env"), true)
		if err != nil {
			return nil, err
		}
		if !keepEnv {
			t, err := prompter.TextInput(i18n.Tp("onboard.ch.slack.bot_token"), "xoxb-...", "", func(v string) string {
				if strings.TrimSpace(v) == "" {
					return "Required"
				}
				return ""
			})
			if err != nil {
				return nil, err
			}
			botToken = strings.TrimSpace(t)
		}
	} else if hasConfigBotToken {
		keep, err := prompter.Confirm(i18n.Tp("onboard.ch.slack.bot_keep"), true)
		if err != nil {
			return nil, err
		}
		if !keep {
			t, err := prompter.TextInput(i18n.Tp("onboard.ch.slack.bot_token"), "xoxb-...", "", func(v string) string {
				if strings.TrimSpace(v) == "" {
					return "Required"
				}
				return ""
			})
			if err != nil {
				return nil, err
			}
			botToken = strings.TrimSpace(t)
		}
	} else {
		NoteSlackTokenHelp(prompter, botName)
		t, err := prompter.TextInput(i18n.Tp("onboard.ch.slack.bot_token"), "xoxb-...", "", func(v string) string {
			if strings.TrimSpace(v) == "" {
				return "Required"
			}
			return ""
		})
		if err != nil {
			return nil, err
		}
		botToken = strings.TrimSpace(t)
	}

	// App Token
	var appToken string
	if envAppToken != "" && !hasConfigAppToken && accountID == DefaultAccountID {
		keepEnv, err := prompter.Confirm(i18n.Tp("onboard.ch.slack.app_env"), true)
		if err != nil {
			return nil, err
		}
		if !keepEnv {
			t, err := prompter.TextInput(i18n.Tp("onboard.ch.slack.app_token"), "xapp-...", "", func(v string) string {
				if strings.TrimSpace(v) == "" {
					return "Required"
				}
				return ""
			})
			if err != nil {
				return nil, err
			}
			appToken = strings.TrimSpace(t)
		}
	} else if hasConfigAppToken {
		keep, err := prompter.Confirm(i18n.Tp("onboard.ch.slack.app_keep"), true)
		if err != nil {
			return nil, err
		}
		if !keep {
			t, err := prompter.TextInput(i18n.Tp("onboard.ch.slack.app_token"), "xapp-...", "", func(v string) string {
				if strings.TrimSpace(v) == "" {
					return "Required"
				}
				return ""
			})
			if err != nil {
				return nil, err
			}
			appToken = strings.TrimSpace(t)
		}
	} else {
		t, err := prompter.TextInput(i18n.Tp("onboard.ch.slack.app_token"), "xapp-...", "", func(v string) string {
			if strings.TrimSpace(v) == "" {
				return "Required"
			}
			return ""
		})
		if err != nil {
			return nil, err
		}
		appToken = strings.TrimSpace(t)
	}

	// 写入 tokens
	if accountID == DefaultAccountID {
		if botToken != "" {
			cfg.Channels.Slack.BotToken = botToken
		}
		if appToken != "" {
			cfg.Channels.Slack.AppToken = appToken
		}
	} else {
		if cfg.Channels.Slack.Accounts == nil {
			cfg.Channels.Slack.Accounts = make(map[string]*types.SlackAccountConfig)
		}
		acct := cfg.Channels.Slack.Accounts[accountID]
		if acct == nil {
			acct = &types.SlackAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		if botToken != "" {
			acct.BotToken = botToken
		}
		if appToken != "" {
			acct.AppToken = appToken
		}
		cfg.Channels.Slack.Accounts[accountID] = acct
	}

	// Channel access 配置
	hasChannels := cfg.Channels.Slack.Channels != nil && len(cfg.Channels.Slack.Channels) > 0
	currentPolicy := cfg.Channels.Slack.GroupPolicy
	if currentPolicy == "" {
		currentPolicy = types.GroupPolicy("allowlist")
	}
	accessConfig, err := PromptChannelAccessConfig(
		prompter, "Slack channels",
		ChannelAccessPolicy(currentPolicy), nil,
		"#general, #random, C0123456",
		hasChannels,
	)
	if err != nil {
		return nil, err
	}
	if accessConfig != nil {
		cfg = SetSlackGroupPolicy(cfg, accountID, string(accessConfig.Policy))
		if accessConfig.Policy == AccessPolicyAllowlist && len(accessConfig.Entries) > 0 {
			cfg = SetSlackChannelAllowlist(cfg, accountID, accessConfig.Entries)
		}
	}

	return &ConfigureSlackResult{Cfg: cfg, AccountID: accountID}, nil
}

// ---------- 配置写入辅助 ----------

// SetSlackDmPolicy 设置 Slack DM 策略。
func SetSlackDmPolicy(cfg *types.OpenAcosmiConfig, policy types.DmPolicy) *types.OpenAcosmiConfig {
	ensureSlackConfig(cfg)
	if cfg.Channels.Slack.DM == nil {
		cfg.Channels.Slack.DM = &types.SlackDmConfig{}
	}
	if cfg.Channels.Slack.DM.Enabled == nil {
		e := true
		cfg.Channels.Slack.DM.Enabled = &e
	}
	cfg.Channels.Slack.DM.Policy = policy
	if policy == "open" {
		cfg.Channels.Slack.DM.AllowFrom = addWildcardInterface(cfg.Channels.Slack.DM.AllowFrom)
	}
	return cfg
}

// SetSlackGroupPolicy 设置 Slack 群组策略。
func SetSlackGroupPolicy(cfg *types.OpenAcosmiConfig, accountID string, policy string) *types.OpenAcosmiConfig {
	ensureSlackConfig(cfg)
	if accountID == DefaultAccountID {
		cfg.Channels.Slack.GroupPolicy = types.GroupPolicy(policy)
	} else {
		if cfg.Channels.Slack.Accounts == nil {
			cfg.Channels.Slack.Accounts = make(map[string]*types.SlackAccountConfig)
		}
		acct := cfg.Channels.Slack.Accounts[accountID]
		if acct == nil {
			acct = &types.SlackAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.GroupPolicy = types.GroupPolicy(policy)
		cfg.Channels.Slack.Accounts[accountID] = acct
	}
	return cfg
}

// SetSlackChannelAllowlist 设置 Slack channel 允许列表。
func SetSlackChannelAllowlist(cfg *types.OpenAcosmiConfig, accountID string, channelKeys []string) *types.OpenAcosmiConfig {
	ensureSlackConfig(cfg)
	channels := make(map[string]*types.SlackChannelConfig)
	for _, key := range channelKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		allow := true
		channels[trimmed] = &types.SlackChannelConfig{Allow: &allow}
	}
	if accountID == DefaultAccountID {
		cfg.Channels.Slack.Channels = channels
	} else {
		if cfg.Channels.Slack.Accounts == nil {
			cfg.Channels.Slack.Accounts = make(map[string]*types.SlackAccountConfig)
		}
		acct := cfg.Channels.Slack.Accounts[accountID]
		if acct == nil {
			acct = &types.SlackAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.Channels = channels
		cfg.Channels.Slack.Accounts[accountID] = acct
	}
	return cfg
}

// SetSlackAllowFrom 设置 Slack DM allowFrom。
func SetSlackAllowFrom(cfg *types.OpenAcosmiConfig, allowFrom []string) *types.OpenAcosmiConfig {
	ensureSlackConfig(cfg)
	if cfg.Channels.Slack.DM == nil {
		cfg.Channels.Slack.DM = &types.SlackDmConfig{}
	}
	e := true
	cfg.Channels.Slack.DM.Enabled = &e
	ifaces := make([]interface{}, len(allowFrom))
	for i, v := range allowFrom {
		ifaces[i] = v
	}
	cfg.Channels.Slack.DM.AllowFrom = ifaces
	return cfg
}

// DisableSlack 禁用 Slack 频道。
func DisableSlack(cfg *types.OpenAcosmiConfig) *types.OpenAcosmiConfig {
	if cfg.Channels != nil && cfg.Channels.Slack != nil {
		e := false
		cfg.Channels.Slack.Enabled = &e
	}
	return cfg
}

func ensureSlackConfig(cfg *types.OpenAcosmiConfig) {
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.Slack == nil {
		cfg.Channels.Slack = &types.SlackConfig{}
	}
	e := true
	cfg.Channels.Slack.Enabled = &e
}
