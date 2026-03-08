package infra

// update_startup.go — 启动时更新检查
// 对应 TS: src/infra/update-startup.ts (123L)
//
// Gateway 启动时自动检查更新，发现新版本时输出日志通知。
// 非阻塞：在后台 goroutine 中执行检查。

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// UpdateStartupConfig 启动更新检查配置。
type UpdateStartupConfig struct {
	// Root 项目根目录。
	Root string
	// StateDir 状态文件目录。
	StateDir string
	// TimeoutMs 超时时间（毫秒）。
	TimeoutMs int
	// FetchGit 是否执行 git fetch。
	FetchGit bool
	// Channel 配置的更新频道。
	Channel UpdateChannel
	// Logger 日志回调。
	Logger LogFunc
	// Disabled 禁用检查。
	Disabled bool
}

// UpdateStartupResult 启动更新检查结果。
type UpdateStartupResult struct {
	Available    bool          `json:"available"`
	CurrentTag   string        `json:"currentTag,omitempty"`
	Behind       int           `json:"behind,omitempty"`
	Channel      UpdateChannel `json:"channel,omitempty"`
	ChannelLabel string        `json:"channelLabel,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// startupUpdateState 全局启动更新状态（最多执行一次）。
var startupUpdateState struct {
	once   sync.Once
	result *UpdateStartupResult
}

// CheckUpdateOnStartup 在启动时异步检查更新。
// 对应 TS: checkUpdateOnStartup(config)
//
// 在后台 goroutine 中执行。调用 GetStartupUpdateResult() 获取结果。
func CheckUpdateOnStartup(config UpdateStartupConfig) {
	if config.Disabled {
		return
	}
	startupUpdateState.once.Do(func() {
		go runStartupUpdateCheck(config)
	})
}

// GetStartupUpdateResult 获取启动更新检查结果（非阻塞）。
// 如果检查尚未完成返回 nil。
func GetStartupUpdateResult() *UpdateStartupResult {
	return startupUpdateState.result
}

// ResetStartupUpdateForTest 重置启动更新状态（仅测试）。
func ResetStartupUpdateForTest() {
	startupUpdateState.once = sync.Once{}
	startupUpdateState.result = nil
}

func runStartupUpdateCheck(config UpdateStartupConfig) {
	timeoutMs := config.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 15_000
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	_ = ctx // 用于后续扩展

	result := &UpdateStartupResult{}

	check := CheckUpdateStatus(config.Root, timeoutMs, config.FetchGit)
	if check.Git == nil {
		result.Error = "not a git repository"
		startupUpdateState.result = result
		return
	}

	result.CurrentTag = check.Git.Tag

	// 解析有效频道
	resolved := ResolveEffectiveUpdateChannel(config.Channel, string(check.InstallKind), check.Git.Tag, check.Git.Branch)
	result.Channel = resolved.Channel
	result.ChannelLabel = FormatUpdateChannelLabel(resolved.Channel, resolved.Source, check.Git.Tag, check.Git.Branch)

	// 检查是否有更新
	if check.Git.Behind != nil && *check.Git.Behind > 0 {
		result.Available = true
		result.Behind = *check.Git.Behind

		if config.Logger != nil {
			config.Logger("info", fmt.Sprintf(
				"Update available: %d commit(s) behind upstream (channel: %s, tag: %s)",
				result.Behind, result.ChannelLabel, result.CurrentTag,
			))
		}
	}

	startupUpdateState.result = result
}
