package types

// Email Channel 配置类型 — Phase 1
// 挂载位置: channels.email
// 文档: docs/codex/2026-03-08-邮箱系统详细落地计划.md

// EmailProviderID 邮箱服务商标识
type EmailProviderID string

const (
	EmailProviderAliyun        EmailProviderID = "aliyun"         // 阿里企业邮箱
	EmailProviderQQ            EmailProviderID = "qq"             // QQ 邮箱
	EmailProviderTencentExmail EmailProviderID = "tencent_exmail" // 腾讯企业邮箱（企业微信邮箱）
	EmailProviderNetease163    EmailProviderID = "netease163"     // 网易 163 邮箱
)

// EmailAuthMode 认证模式
type EmailAuthMode string

const (
	EmailAuthAppPassword EmailAuthMode = "app_password" // 授权码 / 客户端专用密码
)

// EmailSecurityMode TLS 安全模式
type EmailSecurityMode string

const (
	EmailSecurityTLS      EmailSecurityMode = "tls"      // 直连 TLS (端口 993/465)
	EmailSecuritySTARTTLS EmailSecurityMode = "starttls" // STARTTLS 升级
	EmailSecurityNone     EmailSecurityMode = "none"     // 明文（不推荐）
)

// EmailIMAPMode 收件运行模式
type EmailIMAPMode string

const (
	EmailIMAPModeAuto EmailIMAPMode = "auto" // 优先 IDLE，失败降级 Poll
	EmailIMAPModePoll EmailIMAPMode = "poll" // 强制 Poll
	EmailIMAPModeIdle EmailIMAPMode = "idle" // 强制 IDLE
)

// EmailHTMLMode HTML 处理模式
type EmailHTMLMode string

const (
	EmailHTMLSafeText EmailHTMLMode = "safe_text" // HTML → 安全纯文本（默认）
	EmailHTMLStrip    EmailHTMLMode = "strip"     // 仅去除标签
)

// EmailAuthConfig 认证配置
// 字段名 "password" 命中 sensitiveKeyPatterns `(?i)password`，自动进入 OS Keyring
type EmailAuthConfig struct {
	Mode     EmailAuthMode `json:"mode,omitempty"`     // 认证模式，默认 app_password
	Password string        `json:"password,omitempty"` // 授权码（Keyring 自动脱敏）
}

// EmailIMAPConfig IMAP 收件配置
type EmailIMAPConfig struct {
	Host                string            `json:"host,omitempty"`                // IMAP 服务器地址
	Port                int               `json:"port,omitempty"`                // IMAP 端口，默认 993
	Security            EmailSecurityMode `json:"security,omitempty"`            // TLS 模式，默认 tls
	Mode                EmailIMAPMode     `json:"mode,omitempty"`                // 运行模式，默认 auto
	Mailboxes           []string          `json:"mailboxes,omitempty"`           // 监听邮箱文件夹，默认 ["INBOX"]
	PollIntervalSeconds int               `json:"pollIntervalSeconds,omitempty"` // Poll 间隔秒数，默认 60
	IdleRestartMinutes  int               `json:"idleRestartMinutes,omitempty"`  // IDLE 定时重启分钟数，默认 25（RFC 2177 建议 29 分钟内）
	FetchBatchSize      int               `json:"fetchBatchSize,omitempty"`      // 单次拉取邮件数量上限，默认 20
	MaxFetchBytes       int64             `json:"maxFetchBytes,omitempty"`       // 单封邮件最大字节数，默认 5MB
}

// EmailSMTPConfig SMTP 发件配置
type EmailSMTPConfig struct {
	Host     string            `json:"host,omitempty"`     // SMTP 服务器地址
	Port     int               `json:"port,omitempty"`     // SMTP 端口，默认 465
	Security EmailSecurityMode `json:"security,omitempty"` // TLS 模式，默认 tls
	FromName string            `json:"fromName,omitempty"` // 发件人显示名
}

// EmailRoutingConfig 路由与过滤配置
type EmailRoutingConfig struct {
	SessionBy           string `json:"sessionBy,omitempty"`           // 线程归并策略，默认 references_or_message_id
	SubjectTag          string `json:"subjectTag,omitempty"`          // 主题标签，如 "[OA]"
	IgnoreAutoSubmitted bool   `json:"ignoreAutoSubmitted,omitempty"` // 忽略自动提交邮件
	IgnoreMailingList   bool   `json:"ignoreMailingList,omitempty"`   // 忽略列表邮件
	IgnoreNoReply       bool   `json:"ignoreNoReply,omitempty"`       // 忽略 noreply 地址
	IgnoreSelfSent      bool   `json:"ignoreSelfSent,omitempty"`      // 忽略自发自收
}

// EmailLimitsConfig 邮件限制配置
type EmailLimitsConfig struct {
	MaxMessageBytes             int64    `json:"maxMessageBytes,omitempty"`             // 单封邮件最大字节，默认 5MB
	MaxAttachmentBytes          int64    `json:"maxAttachmentBytes,omitempty"`          // 单附件最大字节，默认 10MB
	MaxAttachments              int      `json:"maxAttachments,omitempty"`              // 单封邮件最大附件数，默认 5
	AllowAttachmentMimePrefixes []string `json:"allowAttachmentMimePrefixes,omitempty"` // 附件白名单 MIME 前缀
	MaxSendPerHour              int      `json:"maxSendPerHour,omitempty"`              // 每小时发送上限（F-05）
	MaxSendPerDay               int      `json:"maxSendPerDay,omitempty"`               // 每日发送上限（F-05）
}

// EmailBehaviorConfig 邮件行为配置
type EmailBehaviorConfig struct {
	DownloadAttachments bool          `json:"downloadAttachments,omitempty"` // 是否下载附件
	HTMLMode            EmailHTMLMode `json:"htmlMode,omitempty"`            // HTML 处理模式，默认 safe_text
	SendReplyAsText     bool          `json:"sendReplyAsText,omitempty"`     // 回复以纯文本发送
	AutoReplyEnabled    bool          `json:"autoReplyEnabled,omitempty"`    // 自动回复开关
}

// EmailAccountConfig 单个邮箱账号配置
type EmailAccountConfig struct {
	Enabled  *bool           `json:"enabled,omitempty"`  // 启用开关，默认 true
	Provider EmailProviderID `json:"provider,omitempty"` // 邮箱服务商
	Name     string          `json:"name,omitempty"`     // 账号显示名
	Address  string          `json:"address,omitempty"`  // 邮箱地址（From 地址）
	Login    string          `json:"login,omitempty"`    // 登录用户名（通常与 address 相同）
	Auth     EmailAuthConfig `json:"auth,omitempty"`     // 认证配置

	IMAP     *EmailIMAPConfig     `json:"imap,omitempty"`     // IMAP 配置
	SMTP     *EmailSMTPConfig     `json:"smtp,omitempty"`     // SMTP 配置
	Routing  *EmailRoutingConfig  `json:"routing,omitempty"`  // 路由配置
	Limits   *EmailLimitsConfig   `json:"limits,omitempty"`   // 限制配置
	Behavior *EmailBehaviorConfig `json:"behavior,omitempty"` // 行为配置

	// 通用频道字段
	GroupPolicy    GroupPolicy                       `json:"groupPolicy,omitempty"`
	Heartbeat      *ChannelHeartbeatVisibilityConfig `json:"heartbeat,omitempty"`
	ResponsePrefix string                            `json:"responsePrefix,omitempty"`
}

// EmailConfig Email Channel 顶层配置
// 挂载于 channels.email
type EmailConfig struct {
	Enabled        *bool                          `json:"enabled,omitempty"`        // 全局启用开关（F-07）
	DefaultAccount string                         `json:"defaultAccount,omitempty"` // 默认账号 ID
	Accounts       map[string]*EmailAccountConfig `json:"accounts,omitempty"`       // 账号 map（非数组，避免 Keyring 路径漂移）
}
