package main

import (
	"github.com/Acosmi/ClawAcosmi/internal/gateway"
	"github.com/spf13/cobra"
)

// 对应 TS src/cli/gateway-cli/ — Gateway 控制命令组

func newGatewayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Gateway service control",
		Long:  "Manage the Crab Claw（蟹爪） WebSocket Gateway service — start, stop, restart, and status.",
	}

	cmd.AddCommand(
		newGatewayStartCmd(),
		newGatewayStopCmd(),
		newGatewayRestartCmd(),
		newGatewayStatusCmd(),
	)

	return cmd
}

func newGatewayStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Gateway service",
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetInt("port")
			controlUI, _ := cmd.Flags().GetString("control-ui-dir")

			opts := gateway.GatewayServerOptions{
				ControlUIDir: controlUI,
			}
			return gateway.RunGatewayBlocking(port, opts)
		},
	}
	cmd.Flags().IntP("port", "p", 19001, "Gateway port")
	cmd.Flags().Bool("force", false, "Kill existing process on port before starting")
	cmd.Flags().String("control-ui-dir", "", "Path to control UI static files")
	return cmd
}

func newGatewayStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the Gateway service",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Stopping Gateway...")
			return nil
		},
	}
}

func newGatewayRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the Gateway service",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Restarting Gateway...")
			return nil
		},
	}
}

func newGatewayStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Gateway service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Gateway status: unknown")
			return nil
		},
	}
}
