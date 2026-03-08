package packages

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/plugins"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestAdaptLocalPlugin(t *testing.T) {
	candidate := plugins.PluginCandidate{
		IDHint:             "my-plugin",
		PackageName:        "My Plugin",
		PackageDescription: "Does plugin things",
		PackageVersion:     "2.1.0",
		Origin:             plugins.PluginOriginWorkspace,
	}

	item := AdaptLocalPlugin(candidate)

	if item.ID != "my-plugin" {
		t.Errorf("expected ID=my-plugin, got %s", item.ID)
	}
	if item.Kind != types.PackageKindPlugin {
		t.Errorf("expected kind=plugin, got %s", item.Kind)
	}
	if item.Name != "My Plugin" {
		t.Errorf("expected name=My Plugin, got %s", item.Name)
	}
	if item.Version != "2.1.0" {
		t.Errorf("expected version=2.1.0, got %s", item.Version)
	}
	if item.Source != "local" {
		t.Errorf("expected source=local, got %s", item.Source)
	}
	if !item.IsInstalled {
		t.Error("expected isInstalled=true")
	}
}

func TestAdaptLocalPlugin_Bundled(t *testing.T) {
	candidate := plugins.PluginCandidate{
		IDHint: "bundled-plugin",
		Origin: plugins.PluginOriginBundled,
	}

	item := AdaptLocalPlugin(candidate)
	if item.Source != "builtin" {
		t.Errorf("expected source=builtin for bundled, got %s", item.Source)
	}
}

func TestAdaptLocalPlugin_FallbackName(t *testing.T) {
	candidate := plugins.PluginCandidate{
		IDHint:      "fallback-id",
		PackageName: "", // empty
	}

	item := AdaptLocalPlugin(candidate)
	if item.Name != "fallback-id" {
		t.Errorf("expected name to fallback to IDHint, got %s", item.Name)
	}
}
