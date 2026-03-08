package channels

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// Signal 引导适配器 — 继承自 src/channels/plugins/onboarding/signal.ts (322L)

// SignalDmPolicyInfo Signal DM 策略元数据
var SignalDmPolicyInfo = struct {
	Label        string
	PolicyKey    string
	AllowFromKey string
}{
	Label:        "Signal",
	PolicyKey:    "channels.signal.dmPolicy",
	AllowFromKey: "channels.signal.allowFrom",
}

// DetectSignalCli 检测 signal-cli 是否可用
func DetectSignalCli(cliPath string) bool {
	if cliPath == "" {
		cliPath = "signal-cli"
	}
	_, err := exec.LookPath(cliPath)
	return err == nil
}

// ParseSignalAllowFromInput 解析 Signal allowFrom 输入
func ParseSignalAllowFromInput(raw string) []string {
	var result []string
	for _, part := range regexp.MustCompile(`[\n,;]+`).Split(raw, -1) {
		t := strings.TrimSpace(part)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// IsUUIDLike 检测字符串是否为 UUID 格式
func IsUUIDLike(value string) bool {
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(strings.ToLower(value))
}

// NormalizeE164 简单 E.164 规范化
func NormalizeE164(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	cleaned := regexp.MustCompile(`[^\d+]`).ReplaceAllString(trimmed, "")
	if cleaned == "" {
		return ""
	}
	if !strings.HasPrefix(cleaned, "+") {
		cleaned = "+" + cleaned
	}
	// 基本 E.164 校验: 至少 8 位数字
	digits := regexp.MustCompile(`\d`).FindAllString(cleaned, -1)
	if len(digits) < 8 {
		return ""
	}
	return cleaned
}

// BuildSignalOnboardingStatus 构建 Signal 引导状态
func BuildSignalOnboardingStatus(configured bool, cliDetected bool, cliPath string) OnboardingStatus {
	if cliPath == "" {
		cliPath = "signal-cli"
	}
	var statusParts []string
	if configured {
		statusParts = append(statusParts, "Signal: configured")
	} else {
		statusParts = append(statusParts, "Signal: needs setup")
	}
	cliStatus := "missing"
	if cliDetected {
		cliStatus = "found"
	}
	statusParts = append(statusParts, fmt.Sprintf("signal-cli: %s (%s)", cliStatus, cliPath))

	hint := "signal-cli missing"
	score := 0
	if cliDetected {
		hint = "signal-cli found"
		score = 1
	}
	if configured {
		score = 2
	}
	return OnboardingStatus{
		Channel:         ChannelSignal,
		Configured:      configured,
		StatusLines:     statusParts,
		SelectionHint:   hint,
		QuickstartScore: &score,
	}
}

// ---------- 交互向导 ----------

// ConfigureSignalParams Signal 配置参数。
type ConfigureSignalParams struct {
	Cfg       *types.OpenAcosmiConfig
	Prompter  Prompter
	AccountID string
}

// ConfigureSignalResult Signal 配置结果。
type ConfigureSignalResult struct {
	Cfg       *types.OpenAcosmiConfig
	AccountID string
}

// ConfigureSignal 交互式 Signal 频道配置向导。
// 对应 TS signalOnboardingAdapter.configure (signal.ts L182-311)。
func ConfigureSignal(params ConfigureSignalParams) (*ConfigureSignalResult, error) {
	cfg := params.Cfg
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	prompter := params.Prompter
	accountID := params.AccountID
	if accountID == "" {
		accountID = DefaultAccountID
	}

	// 确保 channels.signal 结构存在
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.Signal == nil {
		cfg.Channels.Signal = &types.SignalConfig{}
	}
	enabledTrue := true
	cfg.Channels.Signal.Enabled = &enabledTrue

	// signal-cli 检测
	resolvedCliPath := cfg.Channels.Signal.CliPath
	if resolvedCliPath == "" {
		resolvedCliPath = "signal-cli"
	}
	cliDetected := DetectSignalCli(resolvedCliPath)

	if !cliDetected {
		prompter.Note(
			"signal-cli not found. Install it, then rerun this step or set channels.signal.cliPath.",
			"Signal",
		)
	}

	// Account number
	account := cfg.Channels.Signal.Account
	if account != "" {
		keep, err := prompter.Confirm(i18n.Tf("onboard.ch.signal.keep", account), true)
		if err != nil {
			return nil, err
		}
		if !keep {
			account = ""
		}
	}
	if account == "" {
		t, err := prompter.TextInput(i18n.Tp("onboard.ch.signal.number"), "+15555550123", "", func(v string) string {
			if strings.TrimSpace(v) == "" {
				return "Required"
			}
			return ""
		})
		if err != nil {
			return nil, err
		}
		account = strings.TrimSpace(t)
	}

	// 写入配置
	if account != "" {
		if accountID == DefaultAccountID {
			cfg.Channels.Signal.Account = account
			cfg.Channels.Signal.CliPath = resolvedCliPath
		} else {
			if cfg.Channels.Signal.Accounts == nil {
				cfg.Channels.Signal.Accounts = make(map[string]*types.SignalAccountConfig)
			}
			acct := cfg.Channels.Signal.Accounts[accountID]
			if acct == nil {
				acct = &types.SignalAccountConfig{}
				e := true
				acct.Enabled = &e
			}
			acct.Account = account
			acct.CliPath = resolvedCliPath
			cfg.Channels.Signal.Accounts[accountID] = acct
		}
	}

	// Next steps note
	prompter.Note(strings.Join([]string{
		`Link device with: signal-cli link -n "Crab Claw"`,
		"Scan QR in Signal → Linked Devices",
		"Then run: crabclaw gateway call channels.status --params '{\"probe\":true}'",
		"Docs: https://docs.openacosmi.dev/signal",
	}, "\n"), "Signal next steps")

	return &ConfigureSignalResult{Cfg: cfg, AccountID: accountID}, nil
}

// ---------- 配置写入辅助 ----------

// SetSignalDmPolicy 设置 Signal DM 策略。
func SetSignalDmPolicy(cfg *types.OpenAcosmiConfig, policy types.DmPolicy) *types.OpenAcosmiConfig {
	ensureSignalConfig(cfg)
	cfg.Channels.Signal.DmPolicy = policy
	if policy == "open" {
		cfg.Channels.Signal.AllowFrom = addWildcardInterface(cfg.Channels.Signal.AllowFrom)
	}
	return cfg
}

// SetSignalAllowFrom 设置 Signal DM allowFrom。
func SetSignalAllowFrom(cfg *types.OpenAcosmiConfig, accountID string, allowFrom []string) *types.OpenAcosmiConfig {
	ensureSignalConfig(cfg)
	ifaces := make([]interface{}, len(allowFrom))
	for i, v := range allowFrom {
		ifaces[i] = v
	}
	if accountID == DefaultAccountID {
		cfg.Channels.Signal.AllowFrom = ifaces
	} else {
		if cfg.Channels.Signal.Accounts == nil {
			cfg.Channels.Signal.Accounts = make(map[string]*types.SignalAccountConfig)
		}
		acct := cfg.Channels.Signal.Accounts[accountID]
		if acct == nil {
			acct = &types.SignalAccountConfig{}
			e := true
			acct.Enabled = &e
		}
		acct.AllowFrom = ifaces
		cfg.Channels.Signal.Accounts[accountID] = acct
	}
	return cfg
}

// PromptSignalAllowFrom 交互式 Signal allowFrom 输入。
func PromptSignalAllowFrom(cfg *types.OpenAcosmiConfig, prompter Prompter, accountID string) (*types.OpenAcosmiConfig, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	prompter.Note(strings.Join([]string{
		"Allowlist Signal DMs by sender id.",
		"Examples:",
		"- +15555550123",
		"- uuid:123e4567-e89b-12d3-a456-426614174000",
		"Multiple entries: comma-separated.",
	}, "\n"), "Signal allowlist")

	entry, err := prompter.TextInput(
		"Signal allowFrom (E.164 or uuid)",
		"+15555550123, uuid:123e4567-e89b-12d3-a456-426614174000",
		"",
		func(v string) string {
			raw := strings.TrimSpace(v)
			if raw == "" {
				return "Required"
			}
			parts := ParseSignalAllowFromInput(raw)
			for _, part := range parts {
				if part == "*" {
					continue
				}
				if strings.HasPrefix(strings.ToLower(part), "uuid:") {
					if strings.TrimSpace(part[5:]) == "" {
						return "Invalid uuid entry"
					}
					continue
				}
				if IsUUIDLike(part) {
					continue
				}
				if NormalizeE164(part) == "" {
					return fmt.Sprintf("Invalid entry: %s", part)
				}
			}
			return ""
		},
	)
	if err != nil {
		return cfg, err
	}
	parts := ParseSignalAllowFromInput(entry)
	var normalized []string
	for _, part := range parts {
		if part == "*" {
			normalized = append(normalized, "*")
			continue
		}
		if strings.HasPrefix(strings.ToLower(part), "uuid:") {
			normalized = append(normalized, fmt.Sprintf("uuid:%s", strings.TrimSpace(part[5:])))
			continue
		}
		if IsUUIDLike(part) {
			normalized = append(normalized, fmt.Sprintf("uuid:%s", part))
			continue
		}
		e := NormalizeE164(part)
		if e != "" {
			normalized = append(normalized, e)
		}
	}
	unique := UniqueStrings(normalized)
	return SetSignalAllowFrom(cfg, accountID, unique), nil
}

// DisableSignal 禁用 Signal 频道。
func DisableSignal(cfg *types.OpenAcosmiConfig) *types.OpenAcosmiConfig {
	if cfg.Channels != nil && cfg.Channels.Signal != nil {
		e := false
		cfg.Channels.Signal.Enabled = &e
	}
	return cfg
}

func ensureSignalConfig(cfg *types.OpenAcosmiConfig) {
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}
	if cfg.Channels.Signal == nil {
		cfg.Channels.Signal = &types.SignalConfig{}
	}
	e := true
	cfg.Channels.Signal.Enabled = &e
}
