package main

// cmd_sandbox.go — 沙箱管理 CLI 子命令。
//
// TS 对照: cli/sandbox-cli.ts (175L) + commands/sandbox.ts (201L) +
//          commands/sandbox-explain.ts (338L) + commands/sandbox-display.ts (137L)
//
// 提供 3 个子命令: list, recreate, explain

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/agents/sandbox"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/sessions"
)

// ---------- sandbox 顶层命令 ----------

func newSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Manage sandbox containers (Docker-based agent isolation)",
		Long: `Manage Docker-based sandbox containers used for agent isolation.

Subcommands allow you to list, recreate, and inspect sandbox containers.`,
	}

	cmd.AddCommand(
		newSandboxListCmd(),
		newSandboxRecreateCmd(),
		newSandboxExplainCmd(),
	)

	return cmd
}

// ---------- sandbox list ----------

func newSandboxListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sandbox containers and their status",
		Long: `List all sandbox containers (or browser containers with --browser) and their status.

Shows container name, running status, Docker image, age, idle time, and session key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")
			browserFlag, _ := cmd.Flags().GetBool("browser")

			stateDir := config.ResolveStateDir()
			registryPath := filepath.Join(stateDir, "sandbox", sandbox.RegistryFilename)
			browserRegistryPath := filepath.Join(stateDir, "sandbox", sandbox.BrowserRegistryFilename)

			if browserFlag {
				return sandboxListBrowsers(cmd, browserRegistryPath, jsonFlag)
			}
			return sandboxListContainers(cmd, registryPath, browserRegistryPath, jsonFlag)
		},
	}

	cmd.Flags().Bool("json", false, "Output result as JSON")
	cmd.Flags().Bool("browser", false, "List browser containers only")

	return cmd
}

func sandboxListContainers(cmd *cobra.Command, registryPath, browserRegistryPath string, jsonOut bool) error {
	containers, err := sandbox.ListSandboxContainers(registryPath, sandbox.DefaultImage)
	if err != nil {
		containers = nil // 容错
	}

	browsers, err := sandbox.ListSandboxBrowserContainers(browserRegistryPath, sandbox.DefaultBrowserImage)
	if err != nil {
		browsers = nil
	}

	if jsonOut {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"containers": containers,
			"browsers":   browsers,
		}, "", "  ")
		cmd.Println(string(data))
		return nil
	}

	// 展示容器
	if len(containers) == 0 {
		cmd.Println("No sandbox containers found.")
	} else {
		cmd.Print("\n📦 Sandbox Containers:\n\n")
		for _, c := range containers {
			printContainerInfo(cmd, c)
		}
	}

	// 摘要
	totalCount := len(containers) + len(browsers)
	runningCount := countRunningContainers(containers) + countRunningBrowsers(browsers)
	mismatchCount := countMismatchContainers(containers) + countMismatchBrowsers(browsers)

	cmd.Printf("Total: %d (%d running)\n", totalCount, runningCount)
	if mismatchCount > 0 {
		cmd.Printf("\n⚠️  %d container(s) with image mismatch detected.\n", mismatchCount)
		cmd.Println("   Run 'crabclaw sandbox recreate --all' to update all containers.")
	}

	return nil
}

func sandboxListBrowsers(cmd *cobra.Command, browserRegistryPath string, jsonOut bool) error {
	browsers, err := sandbox.ListSandboxBrowserContainers(browserRegistryPath, sandbox.DefaultBrowserImage)
	if err != nil {
		browsers = nil
	}

	if jsonOut {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"containers": []interface{}{},
			"browsers":   browsers,
		}, "", "  ")
		cmd.Println(string(data))
		return nil
	}

	if len(browsers) == 0 {
		cmd.Println("No sandbox browser containers found.")
	} else {
		cmd.Print("\n🌐 Sandbox Browser Containers:\n\n")
		for _, b := range browsers {
			printBrowserInfo(cmd, b)
		}
	}

	runningCount := countRunningBrowsers(browsers)
	mismatchCount := countMismatchBrowsers(browsers)

	cmd.Printf("Total: %d (%d running)\n", len(browsers), runningCount)
	if mismatchCount > 0 {
		cmd.Printf("\n⚠️  %d container(s) with image mismatch detected.\n", mismatchCount)
		cmd.Println("   Run 'crabclaw sandbox recreate --all' to update all containers.")
	}

	return nil
}

// ---------- sandbox recreate ----------

func newSandboxRecreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recreate",
		Short: "Remove containers to force recreation with updated config",
		Long: `Remove sandbox containers so they will be automatically recreated
with current configuration when next needed.

Must specify exactly one of: --all, --session <key>, or --agent <id>.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			allFlag, _ := cmd.Flags().GetBool("all")
			sessionFlag, _ := cmd.Flags().GetString("session")
			agentFlag, _ := cmd.Flags().GetString("agent")
			browserFlag, _ := cmd.Flags().GetBool("browser")
			forceFlag, _ := cmd.Flags().GetBool("force")

			// 互斥校验
			if !allFlag && sessionFlag == "" && agentFlag == "" {
				return fmt.Errorf("please specify --all, --session <key>, or --agent <id>")
			}
			exclusiveCount := 0
			if allFlag {
				exclusiveCount++
			}
			if sessionFlag != "" {
				exclusiveCount++
			}
			if agentFlag != "" {
				exclusiveCount++
			}
			if exclusiveCount > 1 {
				return fmt.Errorf("please specify only one of: --all, --session, --agent")
			}

			stateDir := config.ResolveStateDir()
			registryPath := filepath.Join(stateDir, "sandbox", sandbox.RegistryFilename)
			browserRegistryPath := filepath.Join(stateDir, "sandbox", sandbox.BrowserRegistryFilename)

			// 获取并过滤容器列表
			var containers []sandbox.ContainerInfo
			var browsers []sandbox.BrowserContainerInfo

			if !browserFlag {
				all, _ := sandbox.ListSandboxContainers(registryPath, sandbox.DefaultImage)
				containers = filterContainers(all, allFlag, sessionFlag, agentFlag)
			}
			// --session/--agent 时同时过滤 browser 容器（对齐 TS fetchAndFilterContainers）
			// --browser 显式指定时仅处理 browser；--all 时不含 browser（需 --browser 显式启用）
			if browserFlag || sessionFlag != "" || agentFlag != "" {
				all, _ := sandbox.ListSandboxBrowserContainers(browserRegistryPath, sandbox.DefaultBrowserImage)
				browsers = filterBrowserContainers(all, allFlag || browserFlag, sessionFlag, agentFlag)
			}

			totalCount := len(containers) + len(browsers)
			if totalCount == 0 {
				cmd.Println("No containers found matching the criteria.")
				return nil
			}

			// 预览
			cmd.Print("\nContainers to be recreated:\n\n")
			if len(containers) > 0 {
				cmd.Println("📦 Sandbox Containers:")
				for _, c := range containers {
					status := "stopped"
					if c.Running {
						status = "running"
					}
					cmd.Printf("  - %s (%s)\n", c.ContainerName, status)
				}
			}
			if len(browsers) > 0 {
				cmd.Println("\n🌐 Browser Containers:")
				for _, b := range browsers {
					status := "stopped"
					if b.Running {
						status = "running"
					}
					cmd.Printf("  - %s (%s)\n", b.ContainerName, status)
				}
			}
			cmd.Printf("\nTotal: %d container(s)\n", totalCount)

			// 确认
			if !forceFlag {
				cmd.Print("\nThis will stop and remove these containers. Continue? [y/N] ")
				var answer string
				fmt.Scanln(&answer)
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y") {
					cmd.Println("Cancelled.")
					return nil
				}
			}

			// 执行删除
			cmd.Print("\nRemoving containers...\n\n")
			successCount := 0
			failCount := 0

			for _, c := range containers {
				if err := sandbox.RemoveSandboxContainer(registryPath, c.ContainerName); err != nil {
					cmd.PrintErrf("✗ Failed to remove %s: %v\n", c.ContainerName, err)
					failCount++
				} else {
					cmd.Printf("✓ Removed %s\n", c.ContainerName)
					successCount++
				}
			}

			for _, b := range browsers {
				if err := sandbox.RemoveSandboxBrowser(browserRegistryPath, b.ContainerName, b.SessionKey); err != nil {
					cmd.PrintErrf("✗ Failed to remove %s: %v\n", b.ContainerName, err)
					failCount++
				} else {
					cmd.Printf("✓ Removed %s\n", b.ContainerName)
					successCount++
				}
			}

			cmd.Printf("\nDone: %d removed, %d failed\n", successCount, failCount)
			if successCount > 0 {
				cmd.Println("\nContainers will be automatically recreated when the agent is next used.")
			}

			if failCount > 0 {
				return fmt.Errorf("%d container(s) failed to remove", failCount)
			}
			return nil
		},
	}

	cmd.Flags().Bool("all", false, "Recreate all sandbox containers")
	cmd.Flags().String("session", "", "Recreate container for specific session key")
	cmd.Flags().String("agent", "", "Recreate containers for specific agent")
	cmd.Flags().Bool("browser", false, "Only recreate browser containers")
	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}

// ---------- sandbox explain ----------

func newSandboxExplainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain",
		Short: "Explain effective sandbox/tool policy for a session/agent",
		Long: `Show the effective sandbox configuration and tool policy for a given
session or agent. Useful for debugging why a tool is blocked or why
a session is/isn't sandboxed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")
			agentFlag, _ := cmd.Flags().GetString("agent")
			sessionFlag, _ := cmd.Flags().GetString("session")

			// 加载配置
			cfgLoader := config.NewConfigLoader()
			cfg, err := cfgLoader.LoadConfig()
			if err != nil {
				return fmt.Errorf("配置加载失败: %w", err)
			}

			// 解析目标 agent
			agentID := "main"
			if agentFlag != "" {
				agentID = agentFlag
			}

			// 从配置中提取全局沙箱配置
			globalSandboxCfg := sandbox.SandboxConfig{}
			if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Sandbox != nil {
				sb := cfg.Agents.Defaults.Sandbox
				if sb.Mode != "" {
					globalSandboxCfg.Enabled = sb.Mode != "off"
				}
				if sb.Scope != "" {
					globalSandboxCfg.Scope = sandbox.SandboxScope(sb.Scope)
				}
				if sb.WorkspaceAccess != "" {
					globalSandboxCfg.Workspace = sandbox.SandboxWorkspaceAccess(sb.WorkspaceAccess)
				}
			}

			// 解析沙箱模式和工具策略
			mode := sandbox.ResolveSandboxMode(globalSandboxCfg)
			toolPolicy := sandbox.ResolveToolPolicyForAgent(globalSandboxCfg, nil)
			runtimeStatus := sandbox.ResolveSandboxRuntimeStatus(globalSandboxCfg, agentID, nil)

			// 构建输出
			payload := map[string]interface{}{
				"agentId": agentID,
				"sandbox": map[string]interface{}{
					"mode":            mode,
					"scope":           globalSandboxCfg.Scope,
					"workspaceAccess": globalSandboxCfg.Workspace,
					"enabled":         globalSandboxCfg.Enabled,
					"isSandboxed":     runtimeStatus.IsSandboxed,
				},
				"toolPolicy": map[string]interface{}{
					"allow": toolPolicy.Allow,
					"deny":  toolPolicy.Deny,
				},
				"docker": map[string]interface{}{
					"image":   orDefault(globalSandboxCfg.Docker.Image, sandbox.DefaultImage),
					"workdir": orDefault(globalSandboxCfg.Docker.Workdir, sandbox.DefaultWorkdir),
					"network": orDefault(globalSandboxCfg.Docker.Network, "none"),
				},
				"browser": map[string]interface{}{
					"enabled": globalSandboxCfg.Browser.Enabled,
					"image":   orDefault(globalSandboxCfg.Browser.Image, sandbox.DefaultBrowserImage),
				},
				"docsUrl": "docs/skills/tools/sandbox/SKILL.md",
			}

			// S2-4: 如果指定了 --session，从 session store 查询上下文
			var sessionCtx map[string]interface{}
			if sessionFlag != "" {
				stateDir := config.ResolveStateDir()
				storePath := filepath.Join(stateDir, "sessions", "sessions.json")
				store := sessions.NewSessionStore(storePath)
				if entry, err := store.Get(sessionFlag); err == nil && entry != nil {
					sessionCtx = map[string]interface{}{
						"sessionKey":   sessionFlag,
						"channel":      entry.Channel,
						"provider":     entry.ProviderOverride,
						"model":        entry.ModelOverride,
						"chatType":     entry.ChatType,
						"execSecurity": entry.ExecSecurity,
						"execHost":     entry.ExecHost,
					}
					payload["session"] = sessionCtx
				}
			}

			if jsonFlag {
				data, _ := json.MarshalIndent(payload, "", "  ")
				cmd.Printf("%s\n", string(data))
				return nil
			}

			// 人类可读输出
			cmd.Println("Effective sandbox:")
			cmd.Printf("  agentId:         %s\n", agentID)
			cmd.Printf("  mode:            %s\n", mode)
			cmd.Printf("  scope:           %s\n", globalSandboxCfg.Scope)
			cmd.Printf("  enabled:         %v\n", globalSandboxCfg.Enabled)
			cmd.Printf("  isSandboxed:     %v\n", runtimeStatus.IsSandboxed)
			cmd.Printf("  workspaceAccess: %s\n", globalSandboxCfg.Workspace)
			cmd.Println()

			cmd.Println("Docker:")
			cmd.Printf("  image:   %s\n", orDefault(globalSandboxCfg.Docker.Image, sandbox.DefaultImage))
			cmd.Printf("  workdir: %s\n", orDefault(globalSandboxCfg.Docker.Workdir, sandbox.DefaultWorkdir))
			cmd.Printf("  network: %s\n", orDefault(globalSandboxCfg.Docker.Network, "none"))
			cmd.Println()

			cmd.Println("Sandbox tool policy:")
			cmd.Printf("  allow: %s\n", formatStringSlice(toolPolicy.Allow))
			cmd.Printf("  deny:  %s\n", formatStringSlice(toolPolicy.Deny))
			cmd.Println()

			cmd.Println("Browser:")
			cmd.Printf("  enabled: %v\n", globalSandboxCfg.Browser.Enabled)
			cmd.Printf("  image:   %s\n", orDefault(globalSandboxCfg.Browser.Image, sandbox.DefaultBrowserImage))
			cmd.Println()

			cmd.Println("Docs: docs/skills/tools/sandbox/SKILL.md")

			// S2-4: session context
			if sessionCtx != nil {
				cmd.Println()
				cmd.Println("Session context:")
				cmd.Printf("  sessionKey:   %s\n", sessionFlag)
				for _, key := range []string{"channel", "provider", "model", "chatType", "execSecurity", "execHost"} {
					if v, ok := sessionCtx[key].(string); ok && v != "" {
						cmd.Printf("  %-13s %s\n", key+":", v)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().String("session", "", "Session key to inspect (defaults to agent main)")
	cmd.Flags().String("agent", "", "Agent id to inspect (defaults to main)")
	cmd.Flags().Bool("json", false, "Output result as JSON")

	return cmd
}

// ---------- 辅助函数 ----------

func printContainerInfo(cmd *cobra.Command, c sandbox.ContainerInfo) {
	status := "⚫ stopped"
	if c.Running {
		status = "🟢 running"
	}
	imageMatch := "✓"
	if !c.ImageMatch {
		imageMatch = "⚠️  mismatch"
	}

	cmd.Printf("  %s\n", c.ContainerName)
	cmd.Printf("    Status:  %s\n", status)
	cmd.Printf("    Image:   %s %s\n", c.Image, imageMatch)
	cmd.Printf("    Age:     %s\n", formatDurationCompact(time.Since(time.UnixMilli(c.CreatedAtMs))))
	cmd.Printf("    Idle:    %s\n", formatDurationCompact(time.Since(time.UnixMilli(c.LastUsedAtMs))))
	cmd.Printf("    Session: %s\n", c.SessionKey)
	cmd.Println()
}

func printBrowserInfo(cmd *cobra.Command, b sandbox.BrowserContainerInfo) {
	status := "⚫ stopped"
	if b.Running {
		status = "🟢 running"
	}
	imageMatch := "✓"
	if !b.ImageMatch {
		imageMatch = "⚠️  mismatch"
	}

	cmd.Printf("  %s\n", b.ContainerName)
	cmd.Printf("    Status:  %s\n", status)
	cmd.Printf("    Image:   %s %s\n", b.Image, imageMatch)
	cmd.Printf("    CDP:     %d\n", b.CDPPort)
	if b.NoVncPort > 0 {
		cmd.Printf("    noVNC:   %d\n", b.NoVncPort)
	}
	cmd.Printf("    Age:     %s\n", formatDurationCompact(time.Since(time.UnixMilli(b.CreatedAtMs))))
	cmd.Printf("    Idle:    %s\n", formatDurationCompact(time.Since(time.UnixMilli(b.LastUsedAtMs))))
	cmd.Printf("    Session: %s\n", b.SessionKey)
	cmd.Println()
}

func filterContainers(all []sandbox.ContainerInfo, allFlag bool, session, agent string) []sandbox.ContainerInfo {
	if allFlag {
		return all
	}
	var filtered []sandbox.ContainerInfo
	for _, c := range all {
		if session != "" && c.SessionKey == session {
			filtered = append(filtered, c)
		} else if agent != "" {
			prefix := "agent:" + agent
			if c.SessionKey == prefix || strings.HasPrefix(c.SessionKey, prefix+":") {
				filtered = append(filtered, c)
			}
		}
	}
	return filtered
}

func filterBrowserContainers(all []sandbox.BrowserContainerInfo, allFlag bool, session, agent string) []sandbox.BrowserContainerInfo {
	if allFlag {
		return all
	}
	var filtered []sandbox.BrowserContainerInfo
	for _, b := range all {
		if session != "" && b.SessionKey == session {
			filtered = append(filtered, b)
		} else if agent != "" {
			prefix := "agent:" + agent
			if b.SessionKey == prefix || strings.HasPrefix(b.SessionKey, prefix+":") {
				filtered = append(filtered, b)
			}
		}
	}
	return filtered
}

func countRunningContainers(items []sandbox.ContainerInfo) int {
	count := 0
	for _, c := range items {
		if c.Running {
			count++
		}
	}
	return count
}

func countRunningBrowsers(items []sandbox.BrowserContainerInfo) int {
	count := 0
	for _, b := range items {
		if b.Running {
			count++
		}
	}
	return count
}

func countMismatchContainers(items []sandbox.ContainerInfo) int {
	count := 0
	for _, c := range items {
		if !c.ImageMatch {
			count++
		}
	}
	return count
}

func countMismatchBrowsers(items []sandbox.BrowserContainerInfo) int {
	count := 0
	for _, b := range items {
		if !b.ImageMatch {
			count++
		}
	}
	return count
}

func formatDurationCompact(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func formatStringSlice(items []string) string {
	if len(items) == 0 {
		return "(empty)"
	}
	return strings.Join(items, ", ")
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
