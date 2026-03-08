package main

import "github.com/spf13/cobra"

// 对应 TS src/cli/plugins-cli.ts

func newPluginsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Plugin management",
		Long:  "Install, update, remove, and list Crab Claw（蟹爪） plugins.",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List installed plugins",
			RunE: func(cmd *cobra.Command, args []string) error {
				cmd.Println("📋 Plugins list not yet implemented")
				return nil
			},
		},
		&cobra.Command{
			Use:   "install [name]",
			Short: "Install a plugin",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cmd.Printf("📦 Installing plugin: %s (not yet implemented)\n", args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:   "update",
			Short: "Update all plugins",
			RunE: func(cmd *cobra.Command, args []string) error {
				cmd.Println("🔄 Plugins update not yet implemented")
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove [name]",
			Short: "Remove a plugin",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cmd.Printf("🗑️ Removing plugin: %s (not yet implemented)\n", args[0])
				return nil
			},
		},
	)

	return cmd
}
