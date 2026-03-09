package runner

// P3-7, P3-8: Contract tests verifying prompt ⊆ executor tool set invariant.
// The core bug (5.1) was that the prompt listed ALL tools while the executor only
// allowed the intent-filtered subset. After P3-1~P3-3, the prompt is built from
// the filtered tool set, so prompt tools must be a subset of filtered tools.

import (
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/prompt"
)

// allTiers returns the six intent tiers for iteration.
func allTiers() []intentTier {
	return []intentTier{
		intentGreeting,
		intentQuestion,
		intentTaskLight,
		intentTaskWrite,
		intentTaskDelete,
		intentTaskMultimodal,
	}
}

// buildMockTools creates a representative tool set similar to what buildToolDefinitions returns.
func buildMockTools() []llmclient.ToolDef {
	tree := capabilities.DefaultTree()
	summaries := tree.ToolSummaries()
	var tools []llmclient.ToolDef
	for name := range summaries {
		tools = append(tools, llmclient.ToolDef{Name: name})
	}
	return tools
}

// extractPromptToolNames parses tool names from a prompt's ## Tooling section.
// Each tool line has format "- tool_name: description" or "- tool_name".
func extractPromptToolNames(systemPrompt string) []string {
	var names []string
	inTooling := false
	for _, line := range strings.Split(systemPrompt, "\n") {
		if strings.HasPrefix(line, "## Tooling") {
			inTooling = true
			continue
		}
		if inTooling && strings.HasPrefix(line, "## ") {
			break // next section
		}
		if inTooling && strings.HasPrefix(line, "- ") {
			// Extract tool name: "- tool_name: description" or "- tool_name"
			rest := strings.TrimPrefix(line, "- ")
			name := rest
			if idx := strings.Index(rest, ":"); idx > 0 {
				name = rest[:idx]
			}
			name = strings.TrimSpace(name)
			if name != "" && !strings.Contains(name, " ") {
				names = append(names, name)
			}
		}
	}
	return names
}

// TestPromptToolNames_SubsetOfFilteredTools verifies that for every intent tier,
// the tool names appearing in the prompt's ## Tooling section are a subset of
// the tools allowed by filterToolsByIntent.
// P3-7: contract test for the 5.1 bug fix.
func TestPromptToolNames_SubsetOfFilteredTools(t *testing.T) {
	allTools := buildMockTools()
	allSummaries := capabilities.TreeToolSummaries()

	for _, tier := range allTiers() {
		t.Run(string(tier), func(t *testing.T) {
			// Step 1: filter tools by intent (same as RunAttempt post-P3-1)
			filtered := filterToolsByIntent(allTools, tier)

			// Step 2: extract tool names from filtered set
			filteredNames := make(map[string]bool, len(filtered))
			toolNameList := make([]string, len(filtered))
			for i, tool := range filtered {
				filteredNames[tool.Name] = true
				toolNameList[i] = tool.Name
			}

			// Step 3: build filtered summaries (same as RunAttempt post-P3-3)
			filteredSummaries := make(map[string]string, len(toolNameList))
			for _, name := range toolNameList {
				if s, ok := allSummaries[name]; ok {
					filteredSummaries[name] = s
				}
			}

			// Step 4: build prompt with filtered tool names
			bp := prompt.BuildParams{
				Mode:          prompt.PromptModeFull,
				ToolNames:     toolNameList,
				ToolSummaries: filteredSummaries,
			}
			systemPrompt := prompt.BuildAgentSystemPrompt(bp)

			// Step 5: extract tool names from the prompt's ## Tooling section
			promptTools := extractPromptToolNames(systemPrompt)

			// Step 6: verify prompt tools ⊆ filtered tools
			for _, pt := range promptTools {
				if !filteredNames[pt] {
					t.Errorf("tier=%s: prompt contains tool %q which is NOT in the filtered tool set", tier, pt)
				}
			}
		})
	}
}

// TestGreetingTier_NoToolsInPrompt verifies that the greeting tier's prompt
// contains zero tool names in the ## Tooling section.
// P3-8: greeting tier should have no tools.
func TestGreetingTier_NoToolsInPrompt(t *testing.T) {
	allTools := buildMockTools()

	// greeting tier returns nil tools
	filtered := filterToolsByIntent(allTools, intentGreeting)
	if len(filtered) != 0 {
		t.Fatalf("greeting tier should have 0 filtered tools, got %d", len(filtered))
	}

	// Build prompt with empty tool set
	bp := prompt.BuildParams{
		Mode:          prompt.PromptModeFull,
		ToolNames:     nil,
		ToolSummaries: nil,
	}
	systemPrompt := prompt.BuildAgentSystemPrompt(bp)

	// Verify no tool names in prompt
	promptTools := extractPromptToolNames(systemPrompt)
	if len(promptTools) > 0 {
		t.Errorf("greeting tier prompt should have 0 tools, got %d: %v", len(promptTools), promptTools)
	}
}
