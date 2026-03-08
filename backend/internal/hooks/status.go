package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// 钩子状态报告
// 对应 TS: hooks-status.ts
// ============================================================================

// HookStatusConfigCheck 配置检查结果
type HookStatusConfigCheck struct {
	Path      string      `json:"path"`
	Value     interface{} `json:"value"`
	Satisfied bool        `json:"satisfied"`
}

// HookInstallOption 安装选项
type HookInstallOption struct {
	ID    string          `json:"id"`
	Kind  HookInstallKind `json:"kind"`
	Label string          `json:"label"`
	Bins  []string        `json:"bins,omitempty"`
}

// HookStatusEntry 单个钩子的状态
type HookStatusEntry struct {
	Name            string                  `json:"name"`
	Description     string                  `json:"description"`
	Source          string                  `json:"source"`
	PluginID        string                  `json:"pluginId,omitempty"`
	FilePath        string                  `json:"filePath"`
	BaseDir         string                  `json:"baseDir"`
	HandlerPath     string                  `json:"handlerPath"`
	HookKey         string                  `json:"hookKey"`
	Emoji           string                  `json:"emoji,omitempty"`
	Homepage        string                  `json:"homepage,omitempty"`
	Events          []string                `json:"events"`
	Always          bool                    `json:"always"`
	Disabled        bool                    `json:"disabled"`
	Eligible        bool                    `json:"eligible"`
	ManagedByPlugin bool                    `json:"managedByPlugin"`
	Requirements    HookStatusRequirements  `json:"requirements"`
	Missing         HookStatusRequirements  `json:"missing"`
	ConfigChecks    []HookStatusConfigCheck `json:"configChecks"`
	Install         []HookInstallOption     `json:"install"`
}

// HookStatusRequirements 需求/缺失集合
type HookStatusRequirements struct {
	Bins    []string `json:"bins"`
	AnyBins []string `json:"anyBins"`
	Env     []string `json:"env"`
	Config  []string `json:"config"`
	OS      []string `json:"os"`
}

// HookStatusReport 钩子状态报告
type HookStatusReport struct {
	WorkspaceDir    string            `json:"workspaceDir"`
	ManagedHooksDir string            `json:"managedHooksDir"`
	Hooks           []HookStatusEntry `json:"hooks"`
}

// BuildWorkspaceHookStatus 构建钩子状态报告
// 对应 TS: hooks-status.ts buildWorkspaceHookStatus
func BuildWorkspaceHookStatus(workspaceDir string, config map[string]interface{}, eligibility *HookEligibilityContext, entries []HookEntry) HookStatusReport {
	managedHooksDir := filepath.Join(ResolveConfigDir(), "hooks")
	if entries == nil {
		entries = LoadWorkspaceHookEntries(workspaceDir, config)
	}

	hookStatuses := make([]HookStatusEntry, 0, len(entries))
	for _, entry := range entries {
		hookStatuses = append(hookStatuses, buildHookStatus(&entry, config, eligibility))
	}

	return HookStatusReport{
		WorkspaceDir:    workspaceDir,
		ManagedHooksDir: managedHooksDir,
		Hooks:           hookStatuses,
	}
}

func buildHookStatus(entry *HookEntry, config map[string]interface{}, eligibility *HookEligibilityContext) HookStatusEntry {
	hookKey := ResolveHookKey(entry.Hook.Name, entry)
	hookConfig := ResolveHookConfigEntry(config, hookKey)
	managedByPlugin := entry.Hook.Source == HookSourcePlugin

	disabled := false
	if !managedByPlugin && hookConfig != nil && hookConfig.Enabled != nil && !*hookConfig.Enabled {
		disabled = true
	}

	always := entry.Metadata != nil && entry.Metadata.Always != nil && *entry.Metadata.Always
	emoji := ""
	if entry.Metadata != nil && entry.Metadata.Emoji != "" {
		emoji = entry.Metadata.Emoji
	} else if e, ok := entry.Frontmatter["emoji"]; ok {
		emoji = e
	}

	homepage := ""
	for _, key := range []string{"homepage", "website", "url"} {
		if entry.Metadata != nil && entry.Metadata.Homepage != "" {
			homepage = entry.Metadata.Homepage
			break
		}
		if v, ok := entry.Frontmatter[key]; ok && strings.TrimSpace(v) != "" {
			homepage = strings.TrimSpace(v)
			break
		}
	}

	events := entry.Metadata.eventsOrEmpty()
	requiredBins := entry.Metadata.binsOrEmpty()
	requiredAnyBins := entry.Metadata.anyBinsOrEmpty()
	requiredEnv := entry.Metadata.envOrEmpty()
	requiredConfig := entry.Metadata.configOrEmpty()
	requiredOS := entry.Metadata.osOrEmpty()

	// Calculate missing
	var missingBins []string
	for _, bin := range requiredBins {
		if HasBinary(bin) {
			continue
		}
		if eligibility != nil && eligibility.Remote != nil && eligibility.Remote.HasBin != nil && eligibility.Remote.HasBin(bin) {
			continue
		}
		missingBins = append(missingBins, bin)
	}

	var missingAnyBins []string
	if len(requiredAnyBins) > 0 {
		found := false
		for _, bin := range requiredAnyBins {
			if HasBinary(bin) {
				found = true
				break
			}
		}
		if !found && eligibility != nil && eligibility.Remote != nil && eligibility.Remote.HasAnyBin != nil {
			found = eligibility.Remote.HasAnyBin(requiredAnyBins)
		}
		if !found {
			missingAnyBins = requiredAnyBins
		}
	}

	currentOS := ResolveRuntimePlatform()
	var missingOS []string
	if len(requiredOS) > 0 {
		osMatch := containsStr(requiredOS, currentOS)
		if !osMatch && eligibility != nil && eligibility.Remote != nil {
			for _, p := range eligibility.Remote.Platforms {
				if containsStr(requiredOS, p) {
					osMatch = true
					break
				}
			}
		}
		if !osMatch {
			missingOS = requiredOS
		}
	}

	var missingEnv []string
	for _, envName := range requiredEnv {
		if os.Getenv(envName) != "" {
			continue
		}
		if hookConfig != nil && hookConfig.Env != nil && hookConfig.Env[envName] != "" {
			continue
		}
		missingEnv = append(missingEnv, envName)
	}

	configChecks := make([]HookStatusConfigCheck, 0, len(requiredConfig))
	var missingConfig []string
	for _, pathStr := range requiredConfig {
		value := ResolveConfigPath(config, pathStr)
		satisfied := IsConfigPathTruthy(config, pathStr)
		configChecks = append(configChecks, HookStatusConfigCheck{
			Path: pathStr, Value: value, Satisfied: satisfied,
		})
		if !satisfied {
			missingConfig = append(missingConfig, pathStr)
		}
	}

	missing := HookStatusRequirements{}
	if !always {
		missing = HookStatusRequirements{
			Bins: ensureSlice(missingBins), AnyBins: ensureSlice(missingAnyBins),
			Env: ensureSlice(missingEnv), Config: ensureSlice(missingConfig),
			OS: ensureSlice(missingOS),
		}
	}

	eligible := !disabled && (always ||
		(len(missing.Bins) == 0 && len(missing.AnyBins) == 0 &&
			len(missing.Env) == 0 && len(missing.Config) == 0 && len(missing.OS) == 0))

	return HookStatusEntry{
		Name: entry.Hook.Name, Description: entry.Hook.Description,
		Source: string(entry.Hook.Source), PluginID: entry.Hook.PluginID,
		FilePath: entry.Hook.FilePath, BaseDir: entry.Hook.BaseDir,
		HandlerPath: entry.Hook.HandlerPath, HookKey: hookKey,
		Emoji: emoji, Homepage: homepage, Events: ensureSlice(events),
		Always: always, Disabled: disabled, Eligible: eligible, ManagedByPlugin: managedByPlugin,
		Requirements: HookStatusRequirements{
			Bins: ensureSlice(requiredBins), AnyBins: ensureSlice(requiredAnyBins),
			Env: ensureSlice(requiredEnv), Config: ensureSlice(requiredConfig),
			OS: ensureSlice(requiredOS),
		},
		Missing: missing, ConfigChecks: configChecks,
		Install: normalizeInstallOptions(entry),
	}
}

func normalizeInstallOptions(entry *HookEntry) []HookInstallOption {
	if entry.Metadata == nil || len(entry.Metadata.Install) == 0 {
		return nil
	}
	opts := make([]HookInstallOption, 0, len(entry.Metadata.Install))
	for i, spec := range entry.Metadata.Install {
		id := strings.TrimSpace(spec.ID)
		if id == "" {
			id = fmt.Sprintf("%s-%d", spec.Kind, i)
		}
		label := strings.TrimSpace(spec.Label)
		if label == "" {
			switch spec.Kind {
			case HookInstallBundled:
				label = "Bundled with Crab Claw（蟹爪）"
			case HookInstallNPM:
				if spec.Package != "" {
					label = fmt.Sprintf("Install %s (npm)", spec.Package)
				} else {
					label = "Run installer"
				}
			case HookInstallGit:
				if spec.Repository != "" {
					label = fmt.Sprintf("Install from %s", spec.Repository)
				} else {
					label = "Run installer"
				}
			default:
				label = "Run installer"
			}
		}
		opts = append(opts, HookInstallOption{
			ID: id, Kind: spec.Kind, Label: label, Bins: spec.Bins,
		})
	}
	return opts
}

func ensureSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
