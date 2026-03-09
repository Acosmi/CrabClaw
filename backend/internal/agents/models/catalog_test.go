package models

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestModelCatalog(t *testing.T) {
	registry := NewModelRegistry()
	registry.mu.Lock()
	registry.providers = map[string]ProviderConfig{
		"anthropic": {
			Models: []ModelDefinitionConfig{
				{ID: "claude-3", Name: "Claude 3"},
				{ID: "claude-3.5", Name: "Claude 3.5"},
			},
		},
		"openai": {
			Models: []ModelDefinitionConfig{
				{ID: "gpt-4", Name: "GPT-4", Input: []string{"text", "image"}},
			},
		},
	}
	registry.mu.Unlock()

	catalog := NewModelCatalog()
	catalog.BuildFromRegistry(registry)

	all := catalog.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}

	// 按 provider ASC 排序: anthropic 在前
	if all[0].Provider != "anthropic" {
		t.Errorf("first entry should be anthropic: %+v", all[0])
	}
}

func TestFindModel(t *testing.T) {
	registry := NewModelRegistry()
	registry.mu.Lock()
	registry.providers = map[string]ProviderConfig{
		"anthropic": {
			Models: []ModelDefinitionConfig{
				{ID: "claude-3", Name: "Claude 3"},
			},
		},
	}
	registry.mu.Unlock()

	catalog := NewModelCatalog()
	catalog.BuildFromRegistry(registry)

	// 大小写不敏感
	found := catalog.FindModel("Anthropic", "Claude-3")
	if found == nil {
		t.Fatal("FindModel should find claude-3")
	}
	if found.Name != "Claude 3" {
		t.Errorf("Name = %q", found.Name)
	}

	missing := catalog.FindModel("openai", "nonexistent")
	if missing != nil {
		t.Error("FindModel should return nil for missing")
	}
}

func TestModelSupportsVision(t *testing.T) {
	// nil entry
	if ModelSupportsVision(nil) {
		t.Error("nil entry should return false")
	}

	// No input
	noInput := &ModelCatalogEntry{ID: "test"}
	if ModelSupportsVision(noInput) {
		t.Error("no input should return false")
	}

	// text only
	textOnly := &ModelCatalogEntry{ID: "test", Input: []string{"text"}}
	if ModelSupportsVision(textOnly) {
		t.Error("text-only should return false")
	}

	// With image
	withImage := &ModelCatalogEntry{ID: "test", Input: []string{"text", "image"}}
	if !ModelSupportsVision(withImage) {
		t.Error("image input should return true")
	}
}

func TestModelCatalogBuildFromConfig(t *testing.T) {
	catalog := NewModelCatalog()
	catalog.BuildFromConfig(&types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"openai": {
					Models: []types.ModelDefinitionConfig{
						{
							ID:            "gpt-4o",
							Name:          "GPT-4o",
							ContextWindow: 128000,
							Reasoning:     true,
							Input:         []types.ModelInputType{types.ModelInputText, types.ModelInputImage},
						},
					},
				},
				"anthropic": {
					Models: []types.ModelDefinitionConfig{
						{ID: "claude-sonnet-4", Name: "Claude Sonnet 4"},
					},
				},
			},
		},
	})

	all := catalog.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
	if all[0].Provider != "anthropic" {
		t.Fatalf("expected anthropic to sort first, got %+v", all[0])
	}
	if all[1].Provider != "openai" || all[1].Name != "GPT-4o" {
		t.Fatalf("unexpected openai entry: %+v", all[1])
	}
	if all[1].ContextWindow == nil || *all[1].ContextWindow != 128000 {
		t.Fatalf("expected context window to be carried over, got %+v", all[1].ContextWindow)
	}
	if !ModelSupportsVision(&all[1]) {
		t.Fatal("expected GPT-4o entry to support vision")
	}
}
