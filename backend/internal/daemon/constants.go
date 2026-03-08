package daemon

import (
	"fmt"
	"strings"
)

// 服务标签常量
// 对应 TS: constants.ts
const (
	// Gateway 服务标签
	GatewayLaunchAgentLabel     = "ai.openacosmi.gateway"
	GatewayLaunchAgentLabelV2   = "ai.crabclaw.gateway"
	GatewaySystemdServiceName   = "openacosmi-gateway"
	GatewaySystemdServiceNameV2 = "crabclaw-gateway"
	GatewayWindowsTaskName      = "OpenAcosmi Gateway"
	GatewayWindowsTaskNameV2    = "Crab Claw Gateway"
	GatewayDisplayName          = "Crab Claw Gateway"
	GatewayServiceMarker        = "openacosmi"
	GatewayServiceKind          = "gateway"

	// Node 服务标签
	NodeLaunchAgentLabel      = "ai.openacosmi.node"
	NodeLaunchAgentLabelV2    = "ai.crabclaw.node"
	NodeSystemdServiceName    = "openacosmi-node"
	NodeSystemdServiceNameV2  = "crabclaw-node"
	NodeWindowsTaskName       = "OpenAcosmi Node"
	NodeWindowsTaskNameV2     = "Crab Claw Node"
	NodeDisplayName           = "Crab Claw Node Host"
	NodeServiceMarker         = "openacosmi"
	NodeServiceKind           = "node"
	NodeWindowsTaskScriptName = "node.cmd"
)

// 审计问题码常量
// 对应 TS: service-audit.ts SERVICE_AUDIT_CODES
const (
	AuditCodeGatewayCommandMissing            = "gateway-command-missing"
	AuditCodeGatewayEntrypointMismatch        = "gateway-entrypoint-mismatch"
	AuditCodeGatewayPathMissing               = "gateway-path-missing"
	AuditCodeGatewayPathMissingDirs           = "gateway-path-missing-dirs"
	AuditCodeGatewayPathNonMinimal            = "gateway-path-nonminimal"
	AuditCodeGatewayRuntimeBun                = "gateway-runtime-bun"
	AuditCodeGatewayRuntimeNodeVersionManager = "gateway-runtime-node-version-manager"
	AuditCodeGatewayRuntimeNodeSystemMissing  = "gateway-runtime-node-system-missing"
	AuditCodeLaunchdKeepAlive                 = "launchd-keep-alive"
	AuditCodeLaunchdRunAtLoad                 = "launchd-run-at-load"
	AuditCodeSystemdAfterNetworkOnline        = "systemd-after-network-online"
	AuditCodeSystemdRestartSec                = "systemd-restart-sec"
	AuditCodeSystemdWantsNetworkOnline        = "systemd-wants-network-online"
)

// NormalizeGatewayProfile 规范化 profile 名称，返回空字符串表示默认 profile
// 对应 TS: constants.ts normalizeGatewayProfile
func NormalizeGatewayProfile(profile string) string {
	trimmed := strings.TrimSpace(profile)
	if trimmed == "" || strings.EqualFold(trimmed, "default") {
		return ""
	}
	return trimmed
}

// ResolveGatewayProfileSuffix 返回 profile 后缀（如 "-myprofile"）
// 对应 TS: constants.ts resolveGatewayProfileSuffix
func ResolveGatewayProfileSuffix(profile string) string {
	normalized := NormalizeGatewayProfile(profile)
	if normalized == "" {
		return ""
	}
	return "-" + normalized
}

// ResolveGatewayLaunchAgentLabel 解析 launchd agent 标签
// 对应 TS: constants.ts resolveGatewayLaunchAgentLabel
func ResolveGatewayLaunchAgentLabel(profile string) string {
	normalized := NormalizeGatewayProfile(profile)
	if normalized == "" {
		return GatewayLaunchAgentLabel
	}
	return fmt.Sprintf("ai.openacosmi.%s", normalized)
}

// ResolveGatewaySystemdServiceName 解析 systemd 服务名
// 对应 TS: constants.ts resolveGatewaySystemdServiceName
func ResolveGatewaySystemdServiceName(profile string) string {
	suffix := ResolveGatewayProfileSuffix(profile)
	if suffix == "" {
		return GatewaySystemdServiceName
	}
	return "openacosmi-gateway" + suffix
}

// ResolveGatewayWindowsTaskName 解析 Windows 计划任务名
// 对应 TS: constants.ts resolveGatewayWindowsTaskName
func ResolveGatewayWindowsTaskName(profile string) string {
	normalized := NormalizeGatewayProfile(profile)
	if normalized == "" {
		return GatewayWindowsTaskName
	}
	return fmt.Sprintf("OpenAcosmi Gateway (%s)", normalized)
}

func resolveGatewayWindowsTaskNameV2(profile string) string {
	normalized := NormalizeGatewayProfile(profile)
	if normalized == "" {
		return GatewayWindowsTaskNameV2
	}
	return fmt.Sprintf("%s (%s)", GatewayWindowsTaskNameV2, normalized)
}

func ResolveCompatibleGatewayLaunchAgentLabels(profile string) []string {
	normalized := NormalizeGatewayProfile(profile)
	if normalized == "" {
		return []string{GatewayLaunchAgentLabel, GatewayLaunchAgentLabelV2}
	}
	return []string{
		fmt.Sprintf("ai.openacosmi.%s", normalized),
		fmt.Sprintf("ai.crabclaw.%s", normalized),
	}
}

func ResolveCompatibleGatewaySystemdServiceNames(profile string) []string {
	suffix := ResolveGatewayProfileSuffix(profile)
	if suffix == "" {
		return []string{GatewaySystemdServiceName, GatewaySystemdServiceNameV2}
	}
	return []string{
		GatewaySystemdServiceName + suffix,
		GatewaySystemdServiceNameV2 + suffix,
	}
}

func ResolveCompatibleGatewayWindowsTaskNames(profile string) []string {
	return []string{
		ResolveGatewayWindowsTaskName(profile),
		resolveGatewayWindowsTaskNameV2(profile),
	}
}

// FormatGatewayServiceDescription 格式化服务描述
// 对应 TS: constants.ts formatGatewayServiceDescription
func FormatGatewayServiceDescription(profile, version string) string {
	normalizedProfile := NormalizeGatewayProfile(profile)
	trimmedVersion := strings.TrimSpace(version)

	var parts []string
	if normalizedProfile != "" {
		parts = append(parts, "profile: "+normalizedProfile)
	}
	if trimmedVersion != "" {
		parts = append(parts, "v"+trimmedVersion)
	}
	if len(parts) == 0 {
		return GatewayDisplayName
	}
	return fmt.Sprintf("%s (%s)", GatewayDisplayName, strings.Join(parts, ", "))
}

// ResolveNodeLaunchAgentLabel 返回 Node 服务的 launchd 标签
func ResolveNodeLaunchAgentLabel() string {
	return NodeLaunchAgentLabel
}

// ResolveNodeSystemdServiceName 返回 Node 服务的 systemd 服务名
func ResolveNodeSystemdServiceName() string {
	return NodeSystemdServiceName
}

// ResolveNodeWindowsTaskName 返回 Node 服务的 Windows 任务名
func ResolveNodeWindowsTaskName() string {
	return NodeWindowsTaskName
}

func ResolveCompatibleNodeLaunchAgentLabels() []string {
	return []string{NodeLaunchAgentLabel, NodeLaunchAgentLabelV2}
}

func ResolveCompatibleNodeSystemdServiceNames() []string {
	return []string{NodeSystemdServiceName, NodeSystemdServiceNameV2}
}

func ResolveCompatibleNodeWindowsTaskNames() []string {
	return []string{NodeWindowsTaskName, NodeWindowsTaskNameV2}
}

// FormatNodeServiceDescription 格式化 Node 服务描述
// 对应 TS: constants.ts formatNodeServiceDescription
func FormatNodeServiceDescription(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return NodeDisplayName
	}
	return fmt.Sprintf("%s (v%s)", NodeDisplayName, trimmed)
}

// NeedsNodeRuntimeMigration 检查审计问题中是否需要 Node 运行时迁移
// 对应 TS: service-audit.ts needsNodeRuntimeMigration
func NeedsNodeRuntimeMigration(issues []ServiceConfigIssue) bool {
	for _, issue := range issues {
		if issue.Code == AuditCodeGatewayRuntimeBun ||
			issue.Code == AuditCodeGatewayRuntimeNodeVersionManager {
			return true
		}
	}
	return false
}
