package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/cli"
)

// 对应 TS src/cli/models-cli.ts + src/commands/models/

func newModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Model configuration",
		Long:  "List, set, and manage AI model configurations.",
	}

	cmd.AddCommand(
		newModelsListCmd(),
		newModelsSetCmd(),
		newModelsGetCmd(),
	)

	return cmd
}

func newModelsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available models",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")

			opts := cli.GatewayRPCOpts{JSON: jsonFlag, TimeoutMs: 10000}
			result, err := cli.CallGatewayFromCLI("models.list", opts, nil)
			if err != nil {
				cmd.PrintErrln("Failed to reach gateway:", err)
				return nil
			}

			if jsonFlag {
				data, _ := json.MarshalIndent(result, "", "  ")
				cmd.Println(string(data))
				return nil
			}

			// Parse response and display with source tags
			m, ok := result.(map[string]interface{})
			if !ok {
				cmd.Println("Unexpected response format")
				return nil
			}
			modelsRaw, _ := m["models"].([]interface{})
			if len(modelsRaw) == 0 {
				cmd.Println("No models configured.")
				return nil
			}

			for _, entry := range modelsRaw {
				em, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := em["id"].(string)
				name, _ := em["name"].(string)
				provider, _ := em["provider"].(string)
				source, _ := em["source"].(string)
				if source == "" {
					source = "custom"
				}
				display := name
				if display == "" {
					display = id
				}
				cmd.Printf("  %s (%s/%s) [%s]\n", display, provider, id, source)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newModelsSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <provider/model>",
		Short: "Set the default model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model := args[0]
			opts := cli.GatewayRPCOpts{TimeoutMs: 10000}
			params := map[string]interface{}{"model": model}
			_, err := cli.CallGatewayFromCLI("models.default.set", opts, params)
			if err != nil {
				return fmt.Errorf("failed to set model: %w", err)
			}
			cmd.Printf("Default model set to: %s\n", model)
			return nil
		},
	}
	return cmd
}

func newModelsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show current default model",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag, _ := cmd.Flags().GetBool("json")
			opts := cli.GatewayRPCOpts{JSON: jsonFlag, TimeoutMs: 10000}
			result, err := cli.CallGatewayFromCLI("models.default.get", opts, nil)
			if err != nil {
				cmd.PrintErrln("Failed to reach gateway:", err)
				return nil
			}

			if jsonFlag {
				data, _ := json.MarshalIndent(result, "", "  ")
				cmd.Println(string(data))
				return nil
			}

			m, ok := result.(map[string]interface{})
			if !ok {
				cmd.Println("Unexpected response format")
				return nil
			}
			model, _ := m["model"].(string)
			if model == "" {
				cmd.Println("No default model configured (using built-in default).")
			} else {
				cmd.Printf("Default model: %s\n", model)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}
