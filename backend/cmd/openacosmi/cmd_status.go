package main

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/cli"
)

// 对应 TS src/commands/status.command.ts + status-all.ts

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show system status",
		Long:  "Display the status of all Crab Claw（蟹爪） services, agents, and channels.",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")
			timeout, _ := cmd.Flags().GetInt("timeout")

			opts := cli.GatewayRPCOpts{
				JSON:      jsonFlag,
				TimeoutMs: timeout,
			}

			result, err := cli.CallGatewayFromCLI("status", opts, nil)
			if err != nil {
				if jsonFlag {
					status := map[string]interface{}{
						"gateway": "offline",
						"error":   err.Error(),
					}
					data, _ := json.MarshalIndent(status, "", "  ")
					cmd.Println(string(data))
				} else {
					cmd.Println("📊 Crab Claw（蟹爪）状态")
					cmd.Println()
					cmd.Println("  ❌ Gateway: 未运行")
					cmd.Printf("     (%v)\n", err)
				}
				return nil // 不返回 error，非致命
			}

			if jsonFlag {
				data, _ := json.MarshalIndent(result, "", "  ")
				cmd.Println(string(data))
			} else {
				cmd.Println("📊 Crab Claw（蟹爪）状态")
				cmd.Println()

				// 解析 status 结果
				if m, ok := result.(map[string]interface{}); ok {
					phase, _ := m["phase"].(string)
					version, _ := m["version"].(string)
					clients, _ := m["clients"].(float64)

					phaseIcon := "🟢"
					if phase != "ready" {
						phaseIcon = "🟡"
					}

					cmd.Printf("  %s Gateway: %s\n", phaseIcon, phase)
					if version != "" {
						cmd.Printf("  📦 版本: %s\n", version)
					}
					cmd.Printf("  👥 连接客户端: %d\n", int(clients))
				} else {
					cmd.Printf("  ✅ Gateway: 在线 (%v)\n", result)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("deep", false, "Run deep health checks")
	cmd.Flags().Bool("all", false, "Show all details")
	cmd.Flags().Bool("usage", false, "Show resource usage")
	cmd.Flags().Int("timeout", 10000, "Timeout in milliseconds")
	return cmd
}
