package channels

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// iMessage 引导适配器 — 继承自 src/channels/plugins/onboarding/imessage.ts (274L)

// IMessageDmPolicyInfo iMessage DM 策略元数据
var IMessageDmPolicyInfo = struct {
	Label        string
	PolicyKey    string
	AllowFromKey string
}{
	Label:        "iMessage",
	PolicyKey:    "channels.imessage.dmPolicy",
	AllowFromKey: "channels.imessage.allowFrom",
}

// DetectImsgBinary 检测 imsg CLI 是否可用
func DetectImsgBinary(cliPath string) bool {
	if cliPath == "" {
		cliPath = "imsg"
	}
	_, err := exec.LookPath(cliPath)
	return err == nil
}

// ParseIMessageAllowFromInput 解析 iMessage allowFrom 输入
func ParseIMessageAllowFromInput(raw string) []string {
	var result []string
	for _, part := range regexp.MustCompile(`[\n,;]+`).Split(raw, -1) {
		t := strings.TrimSpace(part)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// ValidateIMessageAllowFromEntry 验证单个 iMessage allowFrom 条目
func ValidateIMessageAllowFromEntry(entry string) string {
	if entry == "*" {
		return ""
	}
	lower := strings.ToLower(entry)
	if strings.HasPrefix(lower, "chat_id:") {
		id := strings.TrimSpace(entry[8:])
		if !regexp.MustCompile(`^\d+$`).MatchString(id) {
			return fmt.Sprintf("Invalid chat_id: %s", entry)
		}
		return ""
	}
	if strings.HasPrefix(lower, "chat_guid:") {
		if strings.TrimSpace(entry[10:]) == "" {
			return "Invalid chat_guid entry"
		}
		return ""
	}
	if strings.HasPrefix(lower, "chat_identifier:") {
		if strings.TrimSpace(entry[16:]) == "" {
			return "Invalid chat_identifier entry"
		}
		return ""
	}
	// E.164 phone or email
	if regexp.MustCompile(`^\+?\d{8,}$`).MatchString(strings.ReplaceAll(entry, " ", "")) {
		return ""
	}
	if regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(entry) {
		return ""
	}
	return fmt.Sprintf("Invalid handle: %s", entry)
}

// BuildIMessageOnboardingStatus 构建 iMessage 引导状态
func BuildIMessageOnboardingStatus(configured bool, cliDetected bool, cliPath string) OnboardingStatus {
	if cliPath == "" {
		cliPath = "imsg"
	}
	var statusParts []string
	if configured {
		statusParts = append(statusParts, "iMessage: configured")
	} else {
		statusParts = append(statusParts, "iMessage: needs setup")
	}
	cliStatus := "missing"
	if cliDetected {
		cliStatus = "found"
	}
	statusParts = append(statusParts, fmt.Sprintf("imsg: %s (%s)", cliStatus, cliPath))

	hint := "imsg missing"
	score := 0
	if cliDetected {
		hint = "imsg found"
		score = 1
	}
	if configured {
		score = 2
	}
	return OnboardingStatus{
		Channel:         ChannelIMessage,
		Configured:      configured,
		StatusLines:     statusParts,
		SelectionHint:   hint,
		QuickstartScore: &score,
	}
}

// ---------- 交互向导 ----------

// ConfigureIMessageParams iMessage 配置参数。
type ConfigureIMessageParams struct {
	Cfg       *types.OpenAcosmiConfig
	Prompter  Prompter
	AccountID string
}

// ConfigureIMessageResult iMessage 配置结果。
type ConfigureIMessageResult struct {
	Cfg       *types.OpenAcosmiConfig
	AccountID string
}

// ConfigureIMessage 交互式 iMessage 频道配置向导。
// 对应 TS imessageOnboardingAdapter.configure (imessage.ts L181-263)。
func ConfigureIMessage(params ConfigureIMessageParams) (*ConfigureIMessageResult, error) {
	cfg := params.Cfg
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	prompter := params.Prompter
	accountID := params.AccountID
	if accountID == "" {
		accountID = DefaultAccountID
	}

	// 确保 channels.imessage 结构存在
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.IMessage == nil {
		cfg.Channels.IMessage = &types.IMessageConfig{}
	}
	enabledTrue := true
	cfg.Channels.IMessage.Enabled = &enabledTrue

	// CLI 路径检测
	resolvedCliPath := cfg.Channels.IMessage.CliPath
	if resolvedCliPath == "" {
		resolvedCliPath = "imsg"
	}
	cliDetected := DetectImsgBinary(resolvedCliPath)

	if !cliDetected {
		t, err := prompter.TextInput(i18n.Tp("onboard.ch.imessage.cli_path"), "imsg", resolvedCliPath, func(v string) string {
			if strings.TrimSpace(v) == "" {
				return "Required"
			}
			return ""
		})
		if err != nil {
			return nil, err
		}
		resolvedCliPath = strings.TrimSpace(t)
		if resolvedCliPath == "" {
			prompter.Note(i18n.Tp("onboard.ch.imessage.cli_req"), i18n.Tp("onboard.ch.imessage.title"))
		}
	}

	// 写入 CLI 路径
	if resolvedCliPath != "" {
		if accountID == DefaultAccountID {
			cfg.Channels.IMessage.CliPath = resolvedCliPath
		} else {
			if cfg.Channels.IMessage.Accounts == nil {
				cfg.Channels.IMessage.Accounts = make(map[string]*types.IMessageAccountConfig)
			}
			acct := cfg.Channels.IMessage.Accounts[accountID]
			if acct == nil {
				acct = &types.IMessageAccountConfig{}
				e := true
				acct.Enabled = &e
			}
			acct.CliPath = resolvedCliPath
			cfg.Channels.IMessage.Accounts[accountID] = acct
		}
	}

	// 展示 next steps
	prompter.Note(strings.Join([]string{
		"This is still a work in progress.",
		"Ensure Crab Claw（蟹爪） has Full Disk Access to Messages DB.",
		"Grant Automation permission for Messages when prompted.",
		"List chats with: imsg chats --limit 20",
		"Docs: https://docs.openacosmi.dev/imessage",
	}, "\n"), "iMessage next steps")

	return &ConfigureIMessageResult{Cfg: cfg, AccountID: accountID}, nil
}

// ---------- 配置写入辅助 ----------

// SetIMessageDmPolicy 设置 iMessage DM 策略。
func SetIMessageDmPolicy(cfg *types.OpenAcosmiConfig, policy types.DmPolicy) *types.OpenAcosmiConfig {
	ensureIMessageConfig(cfg)
	cfg.Channels.IMessage.DmPolicy = policy
	if policy == "open" {
		cfg.Channels.IMessage.AllowFrom = addWildcardInterface(cfg.Channels.IMessage.AllowFrom)
	}
	return cfg
}

// SetIMessageAllowFrom 设置 iMessage DM allowFrom。
func SetIMessageAllowFrom(cfg *types.OpenAcosmiConfig, accountID string, allowFrom []string) *types.OpenAcosmiConfig {
	ensureIMessageConfig(cfg)
	ifaces := make([]interface{}, len(allowFrom))
	for i, v := range allowFrom {
		ifaces[i] = v
	}
	if accountID == DefaultAccountID {
		cfg.Channels.IMessage.AllowFrom = ifaces
	} else {
		if cfg.Channels.IMessage.Accounts == nil {
			cfg.Channels.IMessage.Accounts = make(map[string]*types.IMessageAccountConfig)
		}
		acct := cfg.Channels.IMessage.Accounts[accountID]
		if acct == nil {
			acct = &types.IMessageAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.AllowFrom = ifaces
		cfg.Channels.IMessage.Accounts[accountID] = acct
	}
	return cfg
}

// PromptIMessageAllowFrom 交互式 iMessage allowFrom 输入。
func PromptIMessageAllowFrom(cfg *types.OpenAcosmiConfig, prompter Prompter, accountID string) (*types.OpenAcosmiConfig, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	prompter.Note(strings.Join([]string{
		"Allowlist iMessage DMs by handle or chat target.",
		"Examples:",
		"- +15555550123",
		"- user@example.com",
		"- chat_id:123",
		"- chat_guid:... or chat_identifier:...",
		"Multiple entries: comma-separated.",
	}, "\n"), "iMessage allowlist")

	entry, err := prompter.TextInput(
		"iMessage allowFrom (handle or chat_id)",
		"+15555550123, user@example.com, chat_id:123",
		"",
		func(v string) string {
			raw := strings.TrimSpace(v)
			if raw == "" {
				return "Required"
			}
			parts := ParseIMessageAllowFromInput(raw)
			for _, part := range parts {
				if errMsg := ValidateIMessageAllowFromEntry(part); errMsg != "" {
					return errMsg
				}
			}
			return ""
		},
	)
	if err != nil {
		return cfg, err
	}
	parts := ParseIMessageAllowFromInput(entry)
	unique := UniqueStrings(parts)
	return SetIMessageAllowFrom(cfg, accountID, unique), nil
}

// DisableIMessage 禁用 iMessage 频道。
func DisableIMessage(cfg *types.OpenAcosmiConfig) *types.OpenAcosmiConfig {
	if cfg.Channels != nil && cfg.Channels.IMessage != nil {
		e := false
		cfg.Channels.IMessage.Enabled = &e
	}
	return cfg
}

func ensureIMessageConfig(cfg *types.OpenAcosmiConfig) {
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.IMessage == nil {
		cfg.Channels.IMessage = &types.IMessageConfig{}
	}
	e := true
	cfg.Channels.IMessage.Enabled = &e
}
