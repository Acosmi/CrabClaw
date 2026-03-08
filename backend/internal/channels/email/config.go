package email

// config.go — Email Channel 配置解析 + 校验 + Provider 预置

import (
	"fmt"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ProviderPreset 邮箱服务商预置配置
type ProviderPreset struct {
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
	Security types.EmailSecurityMode
	AuthMode types.EmailAuthMode
}

// providerPresets 内置服务商预置表
// 用户可在账号配置中覆盖任意字段
var providerPresets = map[types.EmailProviderID]ProviderPreset{
	types.EmailProviderAliyun: {
		IMAPHost: "imap.qiye.aliyun.com",
		IMAPPort: 993,
		SMTPHost: "smtp.qiye.aliyun.com",
		SMTPPort: 465,
		Security: types.EmailSecurityTLS,
		AuthMode: types.EmailAuthAppPassword,
	},
	types.EmailProviderQQ: {
		IMAPHost: "imap.qq.com",
		IMAPPort: 993,
		SMTPHost: "smtp.qq.com",
		SMTPPort: 465,
		Security: types.EmailSecurityTLS,
		AuthMode: types.EmailAuthAppPassword,
	},
	types.EmailProviderTencentExmail: {
		IMAPHost: "imap.exmail.qq.com",
		IMAPPort: 993,
		SMTPHost: "smtp.exmail.qq.com",
		SMTPPort: 465,
		Security: types.EmailSecurityTLS,
		AuthMode: types.EmailAuthAppPassword,
	},
	types.EmailProviderNetease163: {
		IMAPHost: "imap.163.com",
		IMAPPort: 993,
		SMTPHost: "smtp.163.com",
		SMTPPort: 465,
		Security: types.EmailSecurityTLS,
		AuthMode: types.EmailAuthAppPassword,
	},
}

// GetProviderPreset 获取服务商预置配置（不存在则返回 nil）
func GetProviderPreset(id types.EmailProviderID) *ProviderPreset {
	if p, ok := providerPresets[id]; ok {
		return &p
	}
	return nil
}

// ResolvedEmailAccount 已解析的邮箱账号信息
type ResolvedEmailAccount struct {
	AccountID string
	Config    *types.EmailAccountConfig
}

// ResolveEmailAccount 从配置中解析目标邮箱账号。
// accountID 为 accounts map 中的 key。
func ResolveEmailAccount(cfg *types.OpenAcosmiConfig, accountID string) *ResolvedEmailAccount {
	if cfg == nil || cfg.Channels == nil || cfg.Channels.Email == nil {
		return nil
	}
	emailCfg := cfg.Channels.Email
	if emailCfg.Accounts == nil {
		return nil
	}
	acct, ok := emailCfg.Accounts[accountID]
	if !ok || acct == nil {
		return nil
	}
	return &ResolvedEmailAccount{
		AccountID: accountID,
		Config:    acct,
	}
}

// ApplyProviderDefaults 将 provider 预置值合并到账号配置（仅填充零值字段）。
// 不修改用户已显式设置的字段。
func ApplyProviderDefaults(acct *types.EmailAccountConfig) {
	if acct == nil {
		return
	}
	preset := GetProviderPreset(acct.Provider)

	// IMAP 默认值
	if acct.IMAP == nil {
		acct.IMAP = &types.EmailIMAPConfig{}
	}
	if preset != nil {
		if acct.IMAP.Host == "" {
			acct.IMAP.Host = preset.IMAPHost
		}
		if acct.IMAP.Port == 0 {
			acct.IMAP.Port = preset.IMAPPort
		}
		if acct.IMAP.Security == "" {
			acct.IMAP.Security = preset.Security
		}
	}
	if acct.IMAP.Mode == "" {
		acct.IMAP.Mode = types.EmailIMAPModeAuto
	}
	if len(acct.IMAP.Mailboxes) == 0 {
		acct.IMAP.Mailboxes = []string{"INBOX"}
	}
	if acct.IMAP.PollIntervalSeconds == 0 {
		acct.IMAP.PollIntervalSeconds = 60
	}
	if acct.IMAP.IdleRestartMinutes == 0 {
		acct.IMAP.IdleRestartMinutes = 25 // RFC 2177: IDLE 应在 29 分钟内重启
	}
	if acct.IMAP.FetchBatchSize == 0 {
		acct.IMAP.FetchBatchSize = 20
	}
	if acct.IMAP.MaxFetchBytes == 0 {
		acct.IMAP.MaxFetchBytes = 5 * 1024 * 1024 // 5MB
	}

	// SMTP 默认值
	if acct.SMTP == nil {
		acct.SMTP = &types.EmailSMTPConfig{}
	}
	if preset != nil {
		if acct.SMTP.Host == "" {
			acct.SMTP.Host = preset.SMTPHost
		}
		if acct.SMTP.Port == 0 {
			acct.SMTP.Port = preset.SMTPPort
		}
		if acct.SMTP.Security == "" {
			acct.SMTP.Security = preset.Security
		}
	}

	// Auth 默认值
	if acct.Auth.Mode == "" && preset != nil {
		acct.Auth.Mode = preset.AuthMode
	}

	// Login 默认与 Address 相同
	if acct.Login == "" {
		acct.Login = acct.Address
	}

	// Routing 默认值
	if acct.Routing == nil {
		acct.Routing = &types.EmailRoutingConfig{
			SessionBy:           "references_or_message_id",
			IgnoreAutoSubmitted: true,
			IgnoreMailingList:   true,
			IgnoreNoReply:       true,
			IgnoreSelfSent:      true,
		}
	}

	// Limits 默认值
	if acct.Limits == nil {
		acct.Limits = &types.EmailLimitsConfig{
			MaxMessageBytes:             5 * 1024 * 1024,
			MaxAttachmentBytes:          10 * 1024 * 1024,
			MaxAttachments:              5,
			AllowAttachmentMimePrefixes: []string{"image/", "text/", "application/pdf"},
		}
	}

	// Behavior 默认值
	if acct.Behavior == nil {
		acct.Behavior = &types.EmailBehaviorConfig{
			DownloadAttachments: true,
			HTMLMode:            types.EmailHTMLSafeText,
			SendReplyAsText:     true,
		}
	}
}

// CloneEmailAccountConfig 深拷贝邮箱账号配置。
// 用于在 ApplyProviderDefaults 前保护原始 config 不被修改（修 M1）。
func CloneEmailAccountConfig(src *types.EmailAccountConfig) *types.EmailAccountConfig {
	if src == nil {
		return nil
	}
	dst := *src // 浅拷贝值类型字段（Address, Provider, Login, Auth 等）

	// 深拷贝指针类型字段
	if src.Enabled != nil {
		v := *src.Enabled
		dst.Enabled = &v
	}
	if src.IMAP != nil {
		imapCopy := *src.IMAP
		if src.IMAP.Mailboxes != nil {
			imapCopy.Mailboxes = make([]string, len(src.IMAP.Mailboxes))
			copy(imapCopy.Mailboxes, src.IMAP.Mailboxes)
		}
		dst.IMAP = &imapCopy
	}
	if src.SMTP != nil {
		smtpCopy := *src.SMTP
		dst.SMTP = &smtpCopy
	}
	if src.Routing != nil {
		routingCopy := *src.Routing
		dst.Routing = &routingCopy
	}
	if src.Limits != nil {
		limitsCopy := *src.Limits
		if src.Limits.AllowAttachmentMimePrefixes != nil {
			limitsCopy.AllowAttachmentMimePrefixes = make([]string, len(src.Limits.AllowAttachmentMimePrefixes))
			copy(limitsCopy.AllowAttachmentMimePrefixes, src.Limits.AllowAttachmentMimePrefixes)
		}
		dst.Limits = &limitsCopy
	}
	if src.Behavior != nil {
		behaviorCopy := *src.Behavior
		dst.Behavior = &behaviorCopy
	}
	return &dst
}

// ValidateEmailAccount 校验邮箱账号配置必填字段。
func ValidateEmailAccount(acct *types.EmailAccountConfig) error {
	if acct == nil {
		return fmt.Errorf("email account config is nil")
	}
	if acct.Address == "" {
		return fmt.Errorf("email address is required")
	}
	if acct.Auth.Password == "" {
		return fmt.Errorf("email auth.password is required")
	}
	// Sentinel 检测：keyring 恢复失败时凭证可能残留占位符
	if acct.Auth.Password == config.KeyringSentinel {
		return fmt.Errorf("email auth.password contains keyring sentinel (keyring restore failed); please reconfigure")
	}
	if acct.IMAP != nil && acct.IMAP.Host == "" && acct.Provider == "" {
		return fmt.Errorf("email imap.host is required when provider is not set")
	}
	if acct.SMTP != nil && acct.SMTP.Host == "" && acct.Provider == "" {
		return fmt.Errorf("email smtp.host is required when provider is not set")
	}
	return nil
}
