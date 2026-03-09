// gen-frontend generates frontend TypeScript/JSON artifacts from the capability tree.
//
// Usage:
//
//	go run ./cmd/gen-frontend
//
// This regenerates:
//   - TOOL_GROUPS + TOOL_PROFILES constants (stdout preview, manual paste into tool-policy.ts)
//   - tool-display.json overlay from tree Display fields
//
// The generated output should be committed alongside the tree changes.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	caps "github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
)

func main() {
	tree := caps.GenerateTreeFromRegistry()

	// 1. Generate TS constants (print to stdout for review)
	fmt.Println("// === Generated TOOL_GROUPS + TOOL_PROFILES ===")
	fmt.Println(caps.GenerateToolPolicyTS(tree))

	// 2. Generate tool-display.json
	uiRoot := filepath.Join("..", "ui", "src", "ui")
	displayPath := filepath.Join(uiRoot, "tool-display.json")

	existing, err := os.ReadFile(displayPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", displayPath, err)
		existing = nil
	}

	generated, err := caps.GenerateToolDisplayJSON(tree, existing)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating display JSON: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(displayPath, generated, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", displayPath, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", displayPath, len(generated))

	// 3. Print tool sections summary
	fmt.Println("\n// === Generated Tool Sections (for agents.ts) ===")
	sections := caps.GenerateToolSections(tree)
	for _, s := range sections {
		fmt.Printf("//   %s: %v\n", s.ID, s.Tools)
	}

	// 4. Print wizard skill groups summary
	fmt.Println("\n// === Generated Wizard Skill Groups (for wizard-v2.ts) ===")
	wizGroups := caps.GenerateWizardSkillGroups(tree)
	for _, g := range wizGroups {
		fmt.Printf("//   %s (default=%v): %v\n", g.Key, g.DefaultOn, g.Tools)
	}

	fmt.Fprintln(os.Stderr, "frontend generation complete")
}
