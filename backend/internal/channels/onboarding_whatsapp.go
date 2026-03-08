package channels

import (
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/channels/whatsapp"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// WhatsApp 引导适配器 — 继承自 src/channels/plugins/onboarding/whatsapp.ts (359L)

// WhatsAppDmPolicyInfo WhatsApp DM 策略元数据
var WhatsAppDmPolicyInfo = struct {
	Label        string
	PolicyKey    string
	AllowFromKey string
}{
	Label:        "WhatsApp",
	PolicyKey:    "channels.whatsapp.dmPolicy",
	AllowFromKey: "channels.whatsapp.allowFrom",
}

// DetectWhatsAppLinked 检测 WhatsApp 是否已关联。
// 通过检查 authDir 目录下的 creds.json 判断。
func DetectWhatsAppLinked(cfg *types.OpenAcosmiConfig, accountID string) bool {
	if cfg.Channels == nil || cfg.Channels.WhatsApp == nil {
		return false
	}
	if accountID == "" || accountID == DefaultAccountID {
		return cfg.Channels.WhatsApp.AuthDir != ""
	}
	if cfg.Channels.WhatsApp.Accounts != nil {
		if acct, ok := cfg.Channels.WhatsApp.Accounts[accountID]; ok {
			return acct.AuthDir != ""
		}
	}
	return false
}

// NormalizeWhatsAppPhone 规范化 WhatsApp 手机号码
func NormalizeWhatsAppPhone(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// 移除非数字和+以外的字符
	cleaned := regexp.MustCompile(`[^\d+]`).ReplaceAllString(trimmed, "")
	if cleaned == "" {
		return ""
	}
	// 确保以+开头
	if !strings.HasPrefix(cleaned, "+") {
		cleaned = "+" + cleaned
	}
	return cleaned
}

// ParseWhatsAppAllowFromInput 解析 WhatsApp allowFrom 输入
func ParseWhatsAppAllowFromInput(raw string) []string {
	var result []string
	for _, part := range regexp.MustCompile(`[\n,;]+`).Split(raw, -1) {
		t := strings.TrimSpace(part)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// BuildWhatsAppOnboardingStatus 构建 WhatsApp 引导状态
func BuildWhatsAppOnboardingStatus(configured bool, linked bool) OnboardingStatus {
	var statusParts []string
	if configured {
		statusParts = append(statusParts, "WhatsApp: configured")
	} else {
		statusParts = append(statusParts, "WhatsApp: needs setup")
	}
	if linked {
		statusParts = append(statusParts, "linked: yes")
	} else {
		statusParts = append(statusParts, "linked: no")
	}
	score := 0
	hint := "needs linking"
	if linked {
		hint = "linked"
		score = 1
	}
	if configured {
		score = 2
	}
	return OnboardingStatus{
		Channel:         ChannelWhatsApp,
		Configured:      configured,
		StatusLines:     statusParts,
		SelectionHint:   hint,
		QuickstartScore: &score,
	}
}

// ---------- 交互向导 ----------

// ConfigureWhatsAppParams WhatsApp 配置参数。
type ConfigureWhatsAppParams struct {
	Cfg       *types.OpenAcosmiConfig
	Prompter  Prompter
	AccountID string
}

// ConfigureWhatsAppResult WhatsApp 配置结果。
type ConfigureWhatsAppResult struct {
	Cfg       *types.OpenAcosmiConfig
	AccountID string
}

// ConfigureWhatsApp 交互式 WhatsApp 频道配置向导。
// 对应 TS whatsappOnboardingAdapter.configure (whatsapp.ts L175-340)。
func ConfigureWhatsApp(params ConfigureWhatsAppParams) (*ConfigureWhatsAppResult, error) {
	cfg := params.Cfg
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	prompter := params.Prompter
	accountID := params.AccountID
	if accountID == "" {
		accountID = DefaultAccountID
	}

	// 确保 channels.whatsapp 结构存在
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.WhatsApp == nil {
		cfg.Channels.WhatsApp = &types.WhatsAppConfig{}
	}
	enabledTrue := true
	cfg.Channels.WhatsApp.Enabled = &enabledTrue

	// 检测是否已链接
	linked := DetectWhatsAppLinked(cfg, accountID)

	if !linked {
		prompter.Note(strings.Join([]string{
			"WhatsApp requires linking via QR code.",
			"After completing setup, run: crabclaw gateway start",
			"Then scan the QR code with your phone.",
			"Docs: https://docs.openacosmi.dev/whatsapp",
		}, "\n"), "WhatsApp linking")

		// 尝试 QR 登录
		authDir := ResolveWhatsAppAuthDir(cfg, accountID)
		loginResult, loginErr := whatsapp.LoginWeb(whatsapp.LoginWebOptions{
			Verbose:   false,
			AuthDir:   authDir,
			AccountID: accountID,
		})

		if loginErr != nil {
			prompter.Note(
				"WhatsApp linking failed: "+loginErr.Error()+"\n"+
					"You can link later: crabclaw channels login --channel whatsapp",
				"WhatsApp login",
			)
		} else if loginResult != nil && loginResult.Connected {
			prompter.Note("✅ "+loginResult.Message, i18n.Tp("onboard.ch.whatsapp.linked"))
			linked = true
		} else if loginResult != nil {
			prompter.Note(
				loginResult.Message+"\n"+
					"Link later: crabclaw channels login --channel whatsapp --verbose",
				"WhatsApp login",
			)
		}
	}

	// Self-chat mode
	selfChatMode := false
	if cfg.Channels.WhatsApp.SelfChatMode != nil {
		selfChatMode = *cfg.Channels.WhatsApp.SelfChatMode
	}
	wantSelfChat, err := prompter.Confirm(i18n.Tp("onboard.ch.whatsapp.selfchat"), selfChatMode)
	if err != nil {
		return nil, err
	}
	if accountID == DefaultAccountID {
		cfg.Channels.WhatsApp.SelfChatMode = &wantSelfChat
	} else {
		if cfg.Channels.WhatsApp.Accounts == nil {
			cfg.Channels.WhatsApp.Accounts = make(map[string]*types.WhatsAppAccountConfig)
		}
		acct := cfg.Channels.WhatsApp.Accounts[accountID]
		if acct == nil {
			acct = &types.WhatsAppAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.SelfChatMode = &wantSelfChat
		cfg.Channels.WhatsApp.Accounts[accountID] = acct
	}

	// Group access 配置
	hasGroups := cfg.Channels.WhatsApp.Groups != nil && len(cfg.Channels.WhatsApp.Groups) > 0
	currentPolicy := cfg.Channels.WhatsApp.GroupPolicy
	if currentPolicy == "" {
		currentPolicy = types.GroupPolicy("allowlist")
	}
	accessConfig, err := PromptChannelAccessConfig(
		prompter, "WhatsApp groups",
		ChannelAccessPolicy(currentPolicy), nil,
		"Group Name, group-jid@g.us",
		hasGroups,
	)
	if err != nil {
		return nil, err
	}
	if accessConfig != nil {
		cfg = SetWhatsAppGroupPolicy(cfg, accountID, string(accessConfig.Policy))
	}

	return &ConfigureWhatsAppResult{Cfg: cfg, AccountID: accountID}, nil
}

// ---------- 配置写入辅助 ----------

// SetWhatsAppDmPolicy 设置 WhatsApp DM 策略。
func SetWhatsAppDmPolicy(cfg *types.OpenAcosmiConfig, policy types.DmPolicy) *types.OpenAcosmiConfig {
	ensureWhatsAppConfig(cfg)
	cfg.Channels.WhatsApp.DmPolicy = policy
	if policy == "open" {
		cfg.Channels.WhatsApp.AllowFrom = AddWildcardAllowFrom(cfg.Channels.WhatsApp.AllowFrom)
	}
	return cfg
}

// SetWhatsAppGroupPolicy 设置 WhatsApp 群组策略。
func SetWhatsAppGroupPolicy(cfg *types.OpenAcosmiConfig, accountID string, policy string) *types.OpenAcosmiConfig {
	ensureWhatsAppConfig(cfg)
	if accountID == DefaultAccountID {
		cfg.Channels.WhatsApp.GroupPolicy = types.GroupPolicy(policy)
	} else {
		if cfg.Channels.WhatsApp.Accounts == nil {
			cfg.Channels.WhatsApp.Accounts = make(map[string]*types.WhatsAppAccountConfig)
		}
		acct := cfg.Channels.WhatsApp.Accounts[accountID]
		if acct == nil {
			acct = &types.WhatsAppAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.GroupPolicy = types.GroupPolicy(policy)
		cfg.Channels.WhatsApp.Accounts[accountID] = acct
	}
	return cfg
}

// SetWhatsAppAllowFrom 设置 WhatsApp DM allowFrom。
func SetWhatsAppAllowFrom(cfg *types.OpenAcosmiConfig, accountID string, allowFrom []string) *types.OpenAcosmiConfig {
	ensureWhatsAppConfig(cfg)
	if accountID == DefaultAccountID {
		cfg.Channels.WhatsApp.AllowFrom = allowFrom
	} else {
		if cfg.Channels.WhatsApp.Accounts == nil {
			cfg.Channels.WhatsApp.Accounts = make(map[string]*types.WhatsAppAccountConfig)
		}
		acct := cfg.Channels.WhatsApp.Accounts[accountID]
		if acct == nil {
			acct = &types.WhatsAppAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.AllowFrom = allowFrom
		cfg.Channels.WhatsApp.Accounts[accountID] = acct
	}
	return cfg
}

// SetWhatsAppSelfChatMode 设置 WhatsApp self-chat 模式。
func SetWhatsAppSelfChatMode(cfg *types.OpenAcosmiConfig, selfChat bool) *types.OpenAcosmiConfig {
	ensureWhatsAppConfig(cfg)
	cfg.Channels.WhatsApp.SelfChatMode = &selfChat
	return cfg
}

// PromptWhatsAppAllowFrom 交互式 WhatsApp allowFrom 输入。
func PromptWhatsAppAllowFrom(cfg *types.OpenAcosmiConfig, prompter Prompter, accountID string) (*types.OpenAcosmiConfig, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	prompter.Note(strings.Join([]string{
		"Allowlist WhatsApp DMs by phone number.",
		"Examples: +15555550123, +4915123456789",
		"Multiple entries: comma-separated, or * for all.",
	}, "\n"), "WhatsApp allowlist")

	entry, err := prompter.TextInput(
		"WhatsApp allowFrom (E.164 phone number)",
		"+15555550123",
		"",
		func(v string) string {
			if strings.TrimSpace(v) == "" {
				return "Required"
			}
			return ""
		},
	)
	if err != nil {
		return cfg, err
	}
	parts := ParseWhatsAppAllowFromInput(entry)
	// 规范化手机号
	var normalized []string
	for _, p := range parts {
		if p == "*" {
			normalized = append(normalized, p)
			continue
		}
		n := NormalizeWhatsAppPhone(p)
		if n != "" {
			normalized = append(normalized, n)
		}
	}
	unique := UniqueStrings(normalized)
	return SetWhatsAppAllowFrom(cfg, accountID, unique), nil
}

// DisableWhatsApp 禁用 WhatsApp 频道。
func DisableWhatsApp(cfg *types.OpenAcosmiConfig) *types.OpenAcosmiConfig {
	if cfg.Channels != nil && cfg.Channels.WhatsApp != nil {
		e := false
		cfg.Channels.WhatsApp.Enabled = &e
	}
	return cfg
}

func ensureWhatsAppConfig(cfg *types.OpenAcosmiConfig) {
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.WhatsApp == nil {
		cfg.Channels.WhatsApp = &types.WhatsAppConfig{}
	}
	e := true
	cfg.Channels.WhatsApp.Enabled = &e
}

// ResolveWhatsAppAuthDir 解析 WhatsApp 认证目录
func ResolveWhatsAppAuthDir(cfg *types.OpenAcosmiConfig, accountID string) string {
	if cfg.Channels == nil || cfg.Channels.WhatsApp == nil {
		return ""
	}
	if accountID == "" || accountID == DefaultAccountID {
		return cfg.Channels.WhatsApp.AuthDir
	}
	if cfg.Channels.WhatsApp.Accounts != nil {
		if acct, ok := cfg.Channels.WhatsApp.Accounts[accountID]; ok {
			return acct.AuthDir
		}
	}
	return ""
}
