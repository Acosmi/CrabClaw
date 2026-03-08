// audit.go — 安全审计完整实现。
//
// TS 对照: security/audit.ts (~993L), audit-extra.ts (~1306L)
//
// 执行全面安全审计：文件系统权限、网关配置、浏览器控制、
// 日志配置、特权模式、钩子加固、密钥泄露、模型卫生、
// 云同步目录检测。
package security

import (
	"fmt"
	"strings"
	"time"
)

// SecurityAuditSeverity 审计发现严重程度。
// TS 对照: audit.ts SecurityAuditSeverity
type SecurityAuditSeverity string

const (
	SeverityCritical SecurityAuditSeverity = "critical"
	SeverityWarn     SecurityAuditSeverity = "warn"
	SeverityInfo     SecurityAuditSeverity = "info"
)

// SecurityAuditFinding 单条审计发现。
// TS 对照: audit.ts SecurityAuditFinding
type SecurityAuditFinding struct {
	CheckID     string                `json:"checkId"`
	Severity    SecurityAuditSeverity `json:"severity"`
	Title       string                `json:"title"`
	Detail      string                `json:"detail"`
	Remediation string                `json:"remediation,omitempty"`
}

// SecurityAuditSummary 审计摘要。
// TS 对照: audit.ts SecurityAuditSummary
type SecurityAuditSummary struct {
	Critical int `json:"critical"`
	Warn     int `json:"warn"`
	Info     int `json:"info"`
}

// SecurityAuditDeepResult 深度审计结果（Gateway 探针/WebSocket 测试）。
// TS 对照: audit.ts SecurityAuditReport.deep
type SecurityAuditDeepResult struct {
	Gateway struct {
		Attempted bool    `json:"attempted"`
		URL       *string `json:"url"`
		OK        bool    `json:"ok"`
		Error     *string `json:"error"`
	} `json:"gateway,omitempty"`
	WebSocket struct {
		Attempted bool    `json:"attempted"`
		URL       *string `json:"url"`
		OK        bool    `json:"ok"`
		Error     *string `json:"error"`
	} `json:"websocket,omitempty"`
}

// SecurityAuditReport 完整审计报告。
// TS 对照: audit.ts SecurityAuditReport
type SecurityAuditReport struct {
	Timestamp int64                    `json:"ts"`
	Summary   SecurityAuditSummary     `json:"summary"`
	Findings  []SecurityAuditFinding   `json:"findings"`
	Deep      *SecurityAuditDeepResult `json:"deep,omitempty"`
}

// SecurityAuditOptions 审计选项。
// TS 对照: audit.ts SecurityAuditOptions
type SecurityAuditOptions struct {
	ConfigPath             string
	StateDir               string
	Deep                   bool
	IncludeFilesystem      bool
	IncludeChannelSecurity bool
	DeepTimeoutMs          int

	// --- 依赖注入：使审计函数可脱离外部模块独立测试 ---

	// GatewayConfig 网关配置快照（bind/auth/tailscale 等）。
	GatewayConfig *GatewayConfigSnapshot

	// BrowserConfig 浏览器控制配置快照。
	BrowserConfig *BrowserConfigSnapshot

	// LoggingConfig 日志配置快照。
	LoggingConfig *LoggingConfigSnapshot

	// ElevatedConfig 特权工具配置。
	ElevatedConfig *ElevatedConfigSnapshot

	// HooksConfig 钩子配置。
	HooksConfig *HooksConfigSnapshot

	// ModelRefs 已配置的模型引用列表。
	ModelRefs []ModelRef

	// --- 新增字段（CW1 补充）---

	// SandboxEnabled 是否启用沙箱。
	SandboxEnabled bool

	// PluginsAllow plugins.allow 配置列表。
	PluginsAllow []string

	// IncludePaths 配置 include 文件路径列表。
	IncludePaths []string

	// AgentIDs 已配置的 agent ID 列表（用于深度 FS 检查）。
	AgentIDs []string

	// LogFilePath 日志文件路径。
	LogFilePath string

	// Channels 通道配置（动态结构，用于暴露矩阵）。
	Channels map[string]interface{}

	// InstalledSkills 已安装技能列表（用于代码安全扫描）。
	InstalledSkills []InstalledSkillEntry
}

// ---------- 配置快照类型（DI — 解耦外部依赖）----------

// GatewayConfigSnapshot 网关配置快照。
type GatewayConfigSnapshot struct {
	Bind                         string // "loopback" | "0.0.0.0" 等
	AuthMode                     string // "token" | "password" | "none"
	AuthToken                    string
	AuthPassword                 string
	TailscaleMode                string // "off" | "serve" | "funnel"
	ControlUIEnabled             bool
	AllowInsecureAuth            bool
	DangerouslyDisableDeviceAuth bool
	TrustedProxies               []string
}

// BrowserConfigSnapshot 浏览器控制配置快照。
type BrowserConfigSnapshot struct {
	Enabled  bool
	Profiles []BrowserProfileSnapshot
}

// BrowserProfileSnapshot 单个浏览器配置 profile。
type BrowserProfileSnapshot struct {
	Name       string
	CDPUrl     string
	IsLoopback bool
}

// LoggingConfigSnapshot 日志配置快照。
type LoggingConfigSnapshot struct {
	RedactSensitive string // "off" | "tools" | ...
}

// ElevatedConfigSnapshot 特权工具配置快照。
type ElevatedConfigSnapshot struct {
	Enabled   bool
	AllowFrom map[string][]string // provider → allow list
}

// HooksConfigSnapshot 钩子系统配置快照。
type HooksConfigSnapshot struct {
	Enabled      bool
	Token        string
	Path         string
	GatewayToken string // 用于检测 token 复用
}

// ModelRef 配置的模型引用。
type ModelRef struct {
	ID     string
	Source string
}

// ---------- countBySeverity ----------

// CountBySeverity 按严重程度统计发现数。
// TS 对照: audit.ts countBySeverity()
func CountBySeverity(findings []SecurityAuditFinding) SecurityAuditSummary {
	var s SecurityAuditSummary
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			s.Critical++
		case SeverityWarn:
			s.Warn++
		case SeverityInfo:
			s.Info++
		}
	}
	return s
}

// ---------- 辅助函数 ----------

// normalizeAllowFromList 规范化 allowFrom 列表。
// TS 对照: audit.ts normalizeAllowFromList()
func normalizeAllowFromList(list []string) []string {
	var out []string
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// classifyChannelWarningSeverity 根据警告消息内容分类严重程度。
// TS 对照: audit.ts classifyChannelWarningSeverity()
func classifyChannelWarningSeverity(message string) SecurityAuditSeverity {
	s := strings.ToLower(message)
	if strings.Contains(s, "dms: open") ||
		strings.Contains(s, `grouppolicy="open"`) ||
		strings.Contains(s, `dmpolicy="open"`) {
		return SeverityCritical
	}
	if strings.Contains(s, "allows any") ||
		strings.Contains(s, "anyone can dm") ||
		strings.Contains(s, "public") {
		return SeverityCritical
	}
	if strings.Contains(s, "locked") || strings.Contains(s, "disabled") {
		return SeverityInfo
	}
	return SeverityWarn
}

// ---------- collectFilesystemFindings ----------

// CollectFilesystemFindings 检查 stateDir 和 configPath 的文件系统权限。
// TS 对照: audit.ts collectFilesystemFindings()
func CollectFilesystemFindings(stateDir, configPath string) []SecurityAuditFinding {
	var findings []SecurityAuditFinding

	// 检查 stateDir 权限
	stateDirPerms := InspectPathPermissions(stateDir)
	if stateDirPerms.OK {
		if stateDirPerms.IsSymlink {
			findings = append(findings, SecurityAuditFinding{
				CheckID:  "fs.state_dir.symlink",
				Severity: SeverityWarn,
				Title:    "State dir is a symlink",
				Detail:   stateDir + " is a symlink; treat this as an extra trust boundary.",
			})
		}
		if stateDirPerms.WorldWritable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.state_dir.perms_world_writable",
				Severity:    SeverityCritical,
				Title:       "State dir is world-writable",
				Detail:      FormatPermissionDetail(stateDir, stateDirPerms) + "; other users can write into your Crab Claw（蟹爪） state.",
				Remediation: FormatPermissionRemediation(stateDir, stateDirPerms, true, 0o700),
			})
		} else if stateDirPerms.GroupWritable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.state_dir.perms_group_writable",
				Severity:    SeverityWarn,
				Title:       "State dir is group-writable",
				Detail:      FormatPermissionDetail(stateDir, stateDirPerms) + "; group users can write into your Crab Claw（蟹爪） state.",
				Remediation: FormatPermissionRemediation(stateDir, stateDirPerms, true, 0o700),
			})
		} else if stateDirPerms.GroupReadable || stateDirPerms.WorldReadable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.state_dir.perms_readable",
				Severity:    SeverityWarn,
				Title:       "State dir is readable by others",
				Detail:      FormatPermissionDetail(stateDir, stateDirPerms) + "; consider restricting to 700.",
				Remediation: FormatPermissionRemediation(stateDir, stateDirPerms, true, 0o700),
			})
		}
	}

	// 检查 configPath 权限
	configPerms := InspectPathPermissions(configPath)
	if configPerms.OK {
		if configPerms.IsSymlink {
			findings = append(findings, SecurityAuditFinding{
				CheckID:  "fs.config.symlink",
				Severity: SeverityWarn,
				Title:    "Config file is a symlink",
				Detail:   configPath + " is a symlink; make sure you trust its target.",
			})
		}
		if configPerms.WorldWritable || configPerms.GroupWritable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.config.perms_writable",
				Severity:    SeverityCritical,
				Title:       "Config file is writable by others",
				Detail:      FormatPermissionDetail(configPath, configPerms) + "; another user could change gateway/auth/tool policies.",
				Remediation: FormatPermissionRemediation(configPath, configPerms, false, 0o600),
			})
		} else if configPerms.WorldReadable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.config.perms_world_readable",
				Severity:    SeverityCritical,
				Title:       "Config file is world-readable",
				Detail:      FormatPermissionDetail(configPath, configPerms) + "; config can contain tokens and private settings.",
				Remediation: FormatPermissionRemediation(configPath, configPerms, false, 0o600),
			})
		} else if configPerms.GroupReadable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.config.perms_group_readable",
				Severity:    SeverityWarn,
				Title:       "Config file is group-readable",
				Detail:      FormatPermissionDetail(configPath, configPerms) + "; config can contain tokens and private settings.",
				Remediation: FormatPermissionRemediation(configPath, configPerms, false, 0o600),
			})
		}
	}

	return findings
}

// ---------- collectGatewayConfigFindings ----------

// CollectGatewayConfigFindings 审计网关配置安全性。
// TS 对照: audit.ts collectGatewayConfigFindings()
func CollectGatewayConfigFindings(gw *GatewayConfigSnapshot) []SecurityAuditFinding {
	if gw == nil {
		return nil
	}
	var findings []SecurityAuditFinding

	bind := gw.Bind
	if bind == "" {
		bind = "loopback"
	}
	hasToken := strings.TrimSpace(gw.AuthToken) != ""
	hasPassword := strings.TrimSpace(gw.AuthPassword) != ""
	hasSharedSecret := (gw.AuthMode == "token" && hasToken) || (gw.AuthMode == "password" && hasPassword)
	hasTailscaleAuth := gw.TailscaleMode == "serve"
	hasGatewayAuth := hasSharedSecret || hasTailscaleAuth

	// 绑定到非 loopback 但无 auth
	if bind != "loopback" && !hasSharedSecret {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "gateway.bind_no_auth",
			Severity:    SeverityCritical,
			Title:       "Gateway binds beyond loopback without auth",
			Detail:      fmt.Sprintf(`gateway.bind="%s" but no gateway.auth token/password is configured.`, bind),
			Remediation: "Set gateway.auth (token recommended) or bind to loopback.",
		})
	}

	// loopback + controlUI + 无 trustedProxies
	if bind == "loopback" && gw.ControlUIEnabled && len(gw.TrustedProxies) == 0 {
		findings = append(findings, SecurityAuditFinding{
			CheckID:  "gateway.trusted_proxies_missing",
			Severity: SeverityWarn,
			Title:    "Reverse proxy headers are not trusted",
			Detail: "gateway.bind is loopback and gateway.trustedProxies is empty. " +
				"If you expose the Control UI through a reverse proxy, configure trusted proxies " +
				"so local-client checks cannot be spoofed.",
			Remediation: "Set gateway.trustedProxies to your proxy IPs or keep the Control UI local-only.",
		})
	}

	// loopback + controlUI + 无任何 auth
	if bind == "loopback" && gw.ControlUIEnabled && !hasGatewayAuth {
		findings = append(findings, SecurityAuditFinding{
			CheckID:  "gateway.loopback_no_auth",
			Severity: SeverityCritical,
			Title:    "Gateway auth missing on loopback",
			Detail: "gateway.bind is loopback but no gateway auth secret is configured. " +
				"If the Control UI is exposed through a reverse proxy, unauthenticated access is possible.",
			Remediation: "Set gateway.auth (token recommended) or keep the Control UI local-only.",
		})
	}

	// Tailscale Funnel
	if gw.TailscaleMode == "funnel" {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "gateway.tailscale_funnel",
			Severity:    SeverityCritical,
			Title:       "Tailscale Funnel exposure enabled",
			Detail:      `gateway.tailscale.mode="funnel" exposes the Gateway publicly; keep auth strict and treat it as internet-facing.`,
			Remediation: `Prefer tailscale.mode="serve" (tailnet-only) or set tailscale.mode="off".`,
		})
	} else if gw.TailscaleMode == "serve" {
		findings = append(findings, SecurityAuditFinding{
			CheckID:  "gateway.tailscale_serve",
			Severity: SeverityInfo,
			Title:    "Tailscale Serve exposure enabled",
			Detail:   `gateway.tailscale.mode="serve" exposes the Gateway to your tailnet (loopback behind Tailscale).`,
		})
	}

	// ControlUI 不安全 auth
	if gw.AllowInsecureAuth {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "gateway.control_ui.insecure_auth",
			Severity:    SeverityCritical,
			Title:       "Control UI allows insecure HTTP auth",
			Detail:      "gateway.controlUi.allowInsecureAuth=true allows token-only auth over HTTP and skips device identity.",
			Remediation: "Disable it or switch to HTTPS (Tailscale Serve) or localhost.",
		})
	}

	// ControlUI 禁用设备 auth
	if gw.DangerouslyDisableDeviceAuth {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "gateway.control_ui.device_auth_disabled",
			Severity:    SeverityCritical,
			Title:       "DANGEROUS: Control UI device auth disabled",
			Detail:      "gateway.controlUi.dangerouslyDisableDeviceAuth=true disables device identity checks for the Control UI.",
			Remediation: "Disable it unless you are in a short-lived break-glass scenario.",
		})
	}

	// Token 太短
	token := strings.TrimSpace(gw.AuthToken)
	if gw.AuthMode == "token" && token != "" && len(token) < 24 {
		findings = append(findings, SecurityAuditFinding{
			CheckID:  "gateway.token_too_short",
			Severity: SeverityWarn,
			Title:    "Gateway token looks short",
			Detail:   fmt.Sprintf("gateway auth token is %d chars; prefer a long random token.", len(token)),
		})
	}

	return findings
}

// ---------- collectBrowserControlFindings ----------

// CollectBrowserControlFindings 审计浏览器控制配置安全性。
// TS 对照: audit.ts collectBrowserControlFindings()
func CollectBrowserControlFindings(bc *BrowserConfigSnapshot) []SecurityAuditFinding {
	if bc == nil || !bc.Enabled {
		return nil
	}
	var findings []SecurityAuditFinding

	for _, profile := range bc.Profiles {
		if profile.IsLoopback {
			continue
		}
		if strings.HasPrefix(profile.CDPUrl, "http:") {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "browser.remote_cdp_http",
				Severity:    SeverityWarn,
				Title:       "Remote CDP uses HTTP",
				Detail:      fmt.Sprintf("browser profile %q uses http CDP (%s); this is OK only if it's tailnet-only or behind an encrypted tunnel.", profile.Name, profile.CDPUrl),
				Remediation: "Prefer HTTPS/TLS or a tailnet-only endpoint for remote CDP.",
			})
		}
	}

	return findings
}

// ---------- collectLoggingFindings ----------

// CollectLoggingFindings 审计日志配置安全性。
// TS 对照: audit.ts collectLoggingFindings()
func CollectLoggingFindings(lc *LoggingConfigSnapshot) []SecurityAuditFinding {
	if lc == nil || lc.RedactSensitive != "off" {
		return nil
	}
	return []SecurityAuditFinding{
		{
			CheckID:     "logging.redact_off",
			Severity:    SeverityWarn,
			Title:       "Tool summary redaction is disabled",
			Detail:      `logging.redactSensitive="off" can leak secrets into logs and status output.`,
			Remediation: `Set logging.redactSensitive="tools".`,
		},
	}
}

// ---------- collectElevatedFindings ----------

// CollectElevatedFindings 审计特权工具配置安全性。
// TS 对照: audit.ts collectElevatedFindings()
func CollectElevatedFindings(ec *ElevatedConfigSnapshot) []SecurityAuditFinding {
	if ec == nil || !ec.Enabled {
		return nil
	}
	if len(ec.AllowFrom) == 0 {
		return nil
	}

	var findings []SecurityAuditFinding
	for provider, list := range ec.AllowFrom {
		normalized := normalizeAllowFromList(list)
		for _, v := range normalized {
			if v == "*" {
				findings = append(findings, SecurityAuditFinding{
					CheckID:  fmt.Sprintf("tools.elevated.allowFrom.%s.wildcard", provider),
					Severity: SeverityCritical,
					Title:    "Elevated exec allowlist contains wildcard",
					Detail:   fmt.Sprintf("tools.elevated.allowFrom.%s includes \"*\" which effectively approves everyone on that channel for elevated mode.", provider),
				})
				break
			}
		}
		if len(normalized) > 25 {
			findings = append(findings, SecurityAuditFinding{
				CheckID:  fmt.Sprintf("tools.elevated.allowFrom.%s.large", provider),
				Severity: SeverityWarn,
				Title:    "Elevated exec allowlist is large",
				Detail:   fmt.Sprintf("tools.elevated.allowFrom.%s has %d entries; consider tightening elevated access.", provider, len(normalized)),
			})
		}
	}

	return findings
}

// ---------- RunSecurityAudit ----------

// RunSecurityAudit 执行安全审计。
// TS 对照: audit.ts runSecurityAudit()
func RunSecurityAudit(opts SecurityAuditOptions) (*SecurityAuditReport, error) {
	var findings []SecurityAuditFinding

	// 攻击面汇总
	findings = append(findings, CollectAttackSurfaceSummaryFindings(opts)...)

	// 云同步目录检测
	findings = append(findings, CollectSyncedFolderFindings(opts.StateDir, opts.ConfigPath)...)

	// 网关配置
	findings = append(findings, CollectGatewayConfigFindings(opts.GatewayConfig)...)

	// 浏览器控制
	findings = append(findings, CollectBrowserControlFindings(opts.BrowserConfig)...)

	// 日志配置
	findings = append(findings, CollectLoggingFindings(opts.LoggingConfig)...)

	// 特权工具
	findings = append(findings, CollectElevatedFindings(opts.ElevatedConfig)...)

	// 钩子加固
	findings = append(findings, CollectHooksHardeningFindings(opts.HooksConfig)...)

	// 密钥泄露
	findings = append(findings, CollectSecretsInConfigFindings(opts.GatewayConfig, opts.HooksConfig)...)

	// 模型卫生
	findings = append(findings, CollectModelHygieneFindings(opts.ModelRefs)...)

	// 小模型风险
	findings = append(findings, CollectSmallModelRiskFindings(SmallModelRiskParams{
		Models:         opts.ModelRefs,
		SandboxEnabled: opts.SandboxEnabled,
	})...)

	// 插件信任
	findings = append(findings, CollectPluginsTrustFindings(PluginsTrustParams{
		PluginsAllow: opts.PluginsAllow,
		StateDir:     opts.StateDir,
	})...)

	// Include 文件权限
	findings = append(findings, CollectIncludeFilePermFindings(IncludeFilePermParams{
		IncludePaths: opts.IncludePaths,
	})...)

	// 暴露矩阵
	elevatedEnabled := opts.ElevatedConfig != nil && opts.ElevatedConfig.Enabled
	findings = append(findings, CollectExposureMatrixFindings(ExposureMatrixParams{
		Channels:        opts.Channels,
		ElevatedEnabled: elevatedEnabled,
	})...)

	// 文件系统权限
	if opts.IncludeFilesystem {
		findings = append(findings, CollectFilesystemFindings(opts.StateDir, opts.ConfigPath)...)
	}

	// 深度检查
	if opts.Deep {
		findings = append(findings, CollectStateDeepFilesystemFindings(StateDeepFSParams{
			StateDir:    opts.StateDir,
			AgentIDs:    opts.AgentIDs,
			LogFilePath: opts.LogFilePath,
		})...)

		findings = append(findings, CollectPluginsCodeSafetyFindings(opts.StateDir)...)

		findings = append(findings, CollectInstalledSkillsCodeSafetyFindings(
			opts.InstalledSkills, opts.StateDir,
		)...)
	}

	summary := CountBySeverity(findings)
	return &SecurityAuditReport{
		Timestamp: time.Now().UnixMilli(),
		Summary:   summary,
		Findings:  findings,
	}, nil
}
