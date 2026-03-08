package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// 对应 TS src/cli/browser-cli*.ts (8 子模块)

func newBrowserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser",
		Short: "Browser automation tools",
		Long:  "Manage Crab Claw（蟹爪）'s dedicated browser (Chrome/Chromium).",
	}

	cmd.PersistentFlags().String("browser-profile", "", "Browser profile name (default from config)")
	cmd.PersistentFlags().Bool("json", false, "Output machine-readable JSON")

	cmd.AddCommand(
		newBrowserStatusCmd(),
		newBrowserOpenCmd(),
		newBrowserSessionsCmd(),
		newBrowserCloseCmd(),
		newBrowserExtensionCmd(),
		newBrowserInspectCmd(),
		newBrowserActionCmd(),
		newBrowserDebugCmd(),
		newBrowserStateCmd(),
	)

	return cmd
}

func newBrowserStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show browser status",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("🌐 Browser status:")
			fmt.Println("  State: not running")
			fmt.Println("  Profile: default")
			fmt.Println("Hint: use `crabclaw browser open` to launch")
			return nil
		},
	}
}

func newBrowserOpenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open [url]",
		Short: "Open a URL in the managed browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			headless, _ := cmd.Flags().GetBool("headless")
			url := "about:blank"
			if len(args) > 0 {
				url = args[0]
			}
			mode := "headed"
			if headless {
				mode = "headless"
			}
			fmt.Printf("🌐 Opening browser (%s): %s\n", mode, url)
			// TODO: 通过 Gateway RPC 启动浏览器
			return nil
		},
	}
	cmd.Flags().Bool("headless", false, "Run in headless mode")
	return cmd
}

func newBrowserSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sessions",
		Short: "List active browser sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("📋 Active browser sessions: (none)")
			return nil
		},
	}
}

func newBrowserCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close",
		Short: "Close the managed browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("🔒 Browser closed.")
			return nil
		},
	}
}

func newBrowserExtensionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extension",
		Short: "Manage browser extensions",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List installed extensions",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("📦 Extensions: (none installed)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "install <path>",
			Short: "Install a browser extension",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("📦 Installing extension from: %s\n", args[0])
				return nil
			},
		},
	)
	return cmd
}

func newBrowserInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect a page element",
		RunE: func(cmd *cobra.Command, args []string) error {
			selector, _ := cmd.Flags().GetString("selector")
			if selector == "" {
				selector = "body"
			}
			fmt.Printf("🔍 Inspecting: %s\n", selector)
			return nil
		},
	}
	cmd.Flags().StringP("selector", "s", "", "CSS selector to inspect")
	return cmd
}

func newBrowserActionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "action",
		Short: "Browser action commands",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "click <selector>",
			Short: "Click an element",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("🖱️ Click: %s\n", args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:   "type <selector> <text>",
			Short: "Type text into an element",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("⌨️ Type into %s: %s\n", args[0], args[1])
				return nil
			},
		},
		&cobra.Command{
			Use:   "screenshot [file]",
			Short: "Take a screenshot",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				file := "screenshot.png"
				if len(args) > 0 {
					file = args[0]
				}
				fmt.Printf("📸 Screenshot saved to: %s\n", file)
				return nil
			},
		},
	)
	return cmd
}

func newBrowserDebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Browser debug utilities",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "console",
			Short: "Show browser console output",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("🖥️ Console output: (empty)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "network",
			Short: "Show network requests",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("🌐 Network log: (empty)")
				return nil
			},
		},
	)
	return cmd
}

func newBrowserStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Browser state management",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "cookies",
			Short: "Manage cookies",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("🍪 Cookies: (empty)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "clear",
			Short: "Clear browser state",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("🧹 Browser state cleared.")
				return nil
			},
		},
	)
	return cmd
}
