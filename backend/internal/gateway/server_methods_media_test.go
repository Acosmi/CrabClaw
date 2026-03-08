package gateway

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveMediaSourceState_DefaultSourcesAreNotMarkedConfigured(t *testing.T) {
	sources, enabled, explicitlyConfigured := resolveMediaSourceState(&types.OpenAcosmiConfig{}, []string{"weibo", "baidu", "zhihu"})
	if explicitlyConfigured {
		t.Fatal("expected default sources to be treated as not explicitly configured")
	}
	if len(enabled) != 3 {
		t.Fatalf("enabled sources = %d, want 3", len(enabled))
	}

	byName := make(map[string]map[string]interface{}, len(sources))
	for _, source := range sources {
		name, _ := source["name"].(string)
		byName[name] = source
	}
	for _, name := range []string{"weibo", "baidu", "zhihu"} {
		entry := byName[name]
		if entry == nil {
			t.Fatalf("missing source %q", name)
		}
		if entry["status"] != "default_enabled" {
			t.Fatalf("source %q status = %v, want default_enabled", name, entry["status"])
		}
		if configured, _ := entry["configured"].(bool); configured {
			t.Fatalf("source %q configured = true, want false", name)
		}
		if enabled, _ := entry["enabled"].(bool); !enabled {
			t.Fatalf("source %q enabled = false, want true", name)
		}
	}
}

func TestMediaToolState_DoesNotTreatBuiltinToolsAsConfigured(t *testing.T) {
	status, configured := mediaToolState(media.ToolTrendingTopics, true, &types.OpenAcosmiConfig{})
	if status != "builtin" {
		t.Fatalf("status = %q, want builtin", status)
	}
	if configured {
		t.Fatal("expected builtin trending tool to be treated as not explicitly configured")
	}

	status, configured = mediaToolState(media.ToolContentCompose, true, &types.OpenAcosmiConfig{})
	if status != "builtin" {
		t.Fatalf("status = %q, want builtin", status)
	}
	if configured {
		t.Fatal("expected builtin compose tool to be treated as not explicitly configured")
	}
}

func TestMediaToolState_PublishRequiresChannelConfiguration(t *testing.T) {
	status, configured := mediaToolState(media.ToolMediaPublish, true, &types.OpenAcosmiConfig{})
	if status != "needs_configuration" {
		t.Fatalf("status = %q, want needs_configuration", status)
	}
	if configured {
		t.Fatal("expected publish tool to be unconfigured without publisher credentials")
	}

	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Website: &types.WebsiteConfig{
				Enabled:   true,
				APIURL:    "https://example.com/api/posts",
				AuthType:  "bearer",
				AuthToken: "token",
			},
		},
	}
	status, configured = mediaToolState(media.ToolMediaPublish, true, cfg)
	if status != "configured" {
		t.Fatalf("status = %q, want configured", status)
	}
	if !configured {
		t.Fatal("expected publish tool to be configured when website publishing is configured")
	}
}

func TestMediaToolState_InteractRequiresXiaohongshuConfiguration(t *testing.T) {
	status, configured := mediaToolState(media.ToolSocialInteract, true, &types.OpenAcosmiConfig{})
	if status != "needs_configuration" {
		t.Fatalf("status = %q, want needs_configuration", status)
	}
	if configured {
		t.Fatal("expected social interact tool to be unconfigured without xiaohongshu credentials")
	}

	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Xiaohongshu: &types.XiaohongshuConfig{
				Enabled:    true,
				CookiePath: "/tmp/xhs-cookie.json",
			},
		},
	}
	status, configured = mediaToolState(media.ToolSocialInteract, true, cfg)
	if status != "configured" {
		t.Fatalf("status = %q, want configured", status)
	}
	if !configured {
		t.Fatal("expected social interact tool to be configured when xiaohongshu is configured")
	}
}
