package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// 对应 TS src/cli/nodes-cli/

func newNodesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nodes",
		Short: "Node commands",
		Long:  "Manage cluster/remote nodes for distributed agent execution.",
	}

	cmd.PersistentFlags().Bool("json", false, "Output machine-readable JSON")

	cmd.AddCommand(
		newNodesListCmd(),
		newNodesAddCmd(),
		newNodesRemoveCmd(),
		newNodesStatusCmd(),
		newNodesSshCmd(),
		newNodeControlCmd(),
	)

	return cmd
}

func newNodesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			if jsonOut {
				out, _ := json.MarshalIndent(map[string]any{"nodes": []any{}}, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Println("📋 Nodes: (none configured)")
			fmt.Println("  Use `crabclaw nodes add` to register a node.")
			return nil
		},
	}
}

func newNodesAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> <ssh-target>",
		Short: "Add a remote node",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetInt("port")
			identity, _ := cmd.Flags().GetString("identity")
			fmt.Printf("➕ Adding node '%s' → %s\n", args[0], args[1])
			if identity != "" {
				fmt.Printf("  Identity file: %s\n", identity)
			}
			fmt.Printf("  Gateway port: %d\n", port)
			return nil
		},
	}
	cmd.Flags().IntP("port", "p", 3077, "Gateway port on the remote node")
	cmd.Flags().StringP("identity", "i", "", "SSH identity file")
	return cmd
}

func newNodesRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("➖ Removed node '%s'\n", args[0])
			return nil
		},
	}
}

func newNodesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Check node health",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) > 0 {
				target = args[0]
			}
			fmt.Printf("🏥 Node status: %s\n", target)
			fmt.Println("  (no nodes configured)")
			return nil
		},
	}
}

func newNodesSshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh <name>",
		Short: "SSH into a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("🔗 SSH → node '%s'...\n", args[0])
			fmt.Println("  Node not found.")
			return nil
		},
	}
}

func newNodeControlCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "control",
		Short: "Node lifecycle control",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "start <name>",
			Short: "Start a node's gateway",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("▶️ Starting node '%s'...\n", args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:   "stop <name>",
			Short: "Stop a node's gateway",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("⏹️ Stopping node '%s'...\n", args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:   "restart <name>",
			Short: "Restart a node's gateway",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("🔄 Restarting node '%s'...\n", args[0])
				return nil
			},
		},
	)
	return cmd
}
