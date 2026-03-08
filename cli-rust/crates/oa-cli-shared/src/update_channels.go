package infra

// update_channels.go — 更新频道定义
// 对应 TS: src/infra/update-channels.ts (83L)
//
// 定义 stable/beta/dev 更新频道及其解析逻辑。

import (
	"fmt"
	"strings"
)

// UpdateChannel 更新频道类型。
type UpdateChannel string

const (
	ChannelStable UpdateChannel = "stable"
	ChannelBeta   UpdateChannel = "beta"
	ChannelDev    UpdateChannel = "dev"
)

// UpdateChannelSource 频道来源。
type UpdateChannelSource string

const (
	ChannelSourceConfig    UpdateChannelSource = "config"
	ChannelSourceGitTag    UpdateChannelSource = "git-tag"
	ChannelSourceGitBranch UpdateChannelSource = "git-branch"
	ChannelSourceDefault   UpdateChannelSource = "default"
)

const (
	// DefaultPackageChannel 包安装默认频道。
	DefaultPackageChannel = ChannelStable
	// DefaultGitChannel git 安装默认频道。
	DefaultGitChannel = ChannelDev
	// DevBranch 开发分支名。
	DevBranch = "main"
)

// NormalizeUpdateChannel 规范化更新频道名称。
// 对应 TS: normalizeUpdateChannel(value)
func NormalizeUpdateChannel(value string) (UpdateChannel, bool) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "stable":
		return ChannelStable, true
	case "beta":
		return ChannelBeta, true
	case "dev":
		return ChannelDev, true
	}
	return "", false
}

// ChannelToTag 频道→标签映射。
// 对应 TS: channelToNpmTag(channel)
func ChannelToTag(channel UpdateChannel) string {
	switch channel {
	case ChannelBeta:
		return "beta"
	case ChannelDev:
		return "dev"
	default:
		return "latest"
	}
}

// IsBetaTag 检查标签是否为 beta 版本。
func IsBetaTag(tag string) bool {
	return strings.Contains(strings.ToLower(tag), "-beta")
}

// IsStableTag 检查标签是否为稳定版本。
func IsStableTag(tag string) bool {
	return !IsBetaTag(tag)
}

// ResolvedChannel 解析后的频道信息。
type ResolvedChannel struct {
	Channel UpdateChannel
	Source  UpdateChannelSource
}

// ResolveEffectiveUpdateChannel 根据配置和安装方式解析实际更新频道。
// 对应 TS: resolveEffectiveUpdateChannel(params)
func ResolveEffectiveUpdateChannel(configChannel UpdateChannel, installKind string, gitTag, gitBranch string) ResolvedChannel {
	if configChannel != "" {
		return ResolvedChannel{Channel: configChannel, Source: ChannelSourceConfig}
	}

	if installKind == "git" {
		if gitTag != "" {
			ch := ChannelStable
			if IsBetaTag(gitTag) {
				ch = ChannelBeta
			}
			return ResolvedChannel{Channel: ch, Source: ChannelSourceGitTag}
		}
		if gitBranch != "" && gitBranch != "HEAD" {
			return ResolvedChannel{Channel: ChannelDev, Source: ChannelSourceGitBranch}
		}
		return ResolvedChannel{Channel: DefaultGitChannel, Source: ChannelSourceDefault}
	}

	return ResolvedChannel{Channel: DefaultPackageChannel, Source: ChannelSourceDefault}
}

// FormatUpdateChannelLabel 格式化频道标签。
// 对应 TS: formatUpdateChannelLabel(params)
func FormatUpdateChannelLabel(channel UpdateChannel, source UpdateChannelSource, gitTag, gitBranch string) string {
	switch source {
	case ChannelSourceConfig:
		return fmt.Sprintf("%s (config)", channel)
	case ChannelSourceGitTag:
		if gitTag != "" {
			return fmt.Sprintf("%s (%s)", channel, gitTag)
		}
		return fmt.Sprintf("%s (tag)", channel)
	case ChannelSourceGitBranch:
		if gitBranch != "" {
			return fmt.Sprintf("%s (%s)", channel, gitBranch)
		}
		return fmt.Sprintf("%s (branch)", channel)
	default:
		return fmt.Sprintf("%s (default)", channel)
	}
}
