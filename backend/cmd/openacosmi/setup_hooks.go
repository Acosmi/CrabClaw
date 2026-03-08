package main

// setup_hooks.go — Onboarding Hooks 设置向导
// TS 对照: src/commands/onboard-hooks.ts (86L)
//
// 提供 SetupInternalHooks — 发现可用 hooks 并引导用户启用。

import (
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/hooks"
	"github.com/Acosmi/ClawAcosmi/internal/tui"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// SetupInternalHooks 引导用户发现并启用内部 hooks。
// 对应 TS: setupInternalHooks (onboard-hooks.ts L8-85)。
//
// 流程:
//  1. 展示 hooks 说明
//  2. 发现 workspace 中可用的 hooks (eligible)
//  3. 多选启用
//  4. 写入 config
func SetupInternalHooks(
	cfg *types.OpenAcosmiConfig,
	workspaceDir string,
	prompter tui.WizardPrompter,
) (*types.OpenAcosmiConfig, error) {
	// 1. 展示 hooks 说明
	prompter.Note(i18n.Tp("onboard.hook.intro"), i18n.Tp("onboard.hook.title"))

	// 2. 发现可用 hooks
	var configMap map[string]interface{}
	var eligCtx *hooks.HookEligibilityContext
	var entries []hooks.HookEntry

	// 从 workspace 加载 hook entries
	entries = hooks.LoadHookEntriesFromDir(workspaceDir, hooks.HookSourceWorkspace, "")
	report := hooks.BuildWorkspaceHookStatus(workspaceDir, configMap, eligCtx, entries)

	// 筛选 eligible hooks
	var eligible []hooks.HookStatusEntry
	for _, h := range report.Hooks {
		if h.Eligible {
			eligible = append(eligible, h)
		}
	}

	if len(eligible) == 0 {
		prompter.Note(
			i18n.Tp("onboard.hook.none"),
			i18n.Tp("onboard.hook.title"),
		)
		return cfg, nil
	}

	// 3. 多选启用 — 构建选项列表
	options := []tui.PromptOption{
		{Value: "__skip__", Label: "Skip for now"},
	}
	for _, h := range eligible {
		emoji := "🔗"
		if h.Emoji != "" {
			emoji = h.Emoji
		}
		options = append(options, tui.PromptOption{
			Value: h.Name,
			Label: fmt.Sprintf("%s %s", emoji, h.Name),
			Hint:  h.Description,
		})
	}

	selected, err := prompter.MultiSelect("Enable hooks?", options, nil)
	if err != nil {
		return cfg, fmt.Errorf("hook selection: %w", err)
	}

	// 过滤 __skip__
	var toEnable []string
	for _, name := range selected {
		if name != "__skip__" {
			toEnable = append(toEnable, name)
		}
	}

	if len(toEnable) == 0 {
		return cfg, nil
	}

	// 4. 写入 config 的 hooks.internal.entries
	next := shallowCopyConfig(cfg)
	if next.Hooks == nil {
		next.Hooks = &types.HooksConfig{}
	}
	if next.Hooks.Internal == nil {
		next.Hooks.Internal = &types.InternalHooksConfig{}
	}
	trueVal := true
	next.Hooks.Internal.Enabled = &trueVal

	if next.Hooks.Internal.Entries == nil {
		next.Hooks.Internal.Entries = make(map[string]*types.HookConfig)
	}
	for _, name := range toEnable {
		existing := next.Hooks.Internal.Entries[name]
		if existing == nil {
			existing = &types.HookConfig{}
		}
		enabled := true
		existing.Enabled = &enabled
		next.Hooks.Internal.Entries[name] = existing
	}

	// 5. 展示配置结果
	suffix := ""
	if len(toEnable) > 1 {
		suffix = "s"
	}
	prompter.Note(strings.Join([]string{
		fmt.Sprintf("Enabled %d hook%s: %s", len(toEnable), suffix, strings.Join(toEnable, ", ")),
		"",
		"You can manage hooks later with:",
		"  crabclaw hooks list",
		"  crabclaw hooks enable <name>",
		"  crabclaw hooks disable <name>",
	}, "\n"), i18n.Tp("onboard.hook.summary"))

	return next, nil
}
