package packages

import (
	"context"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/plugins"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestCatalogBrowse_AggregatesSkillAndPlugin(t *testing.T) {
	skillLoader := func() []skills.SkillEntry {
		return []skills.SkillEntry{
			{Skill: skills.Skill{Name: "local-skill-1", Description: "A local skill"}, Enabled: true},
		}
	}
	pluginLoader := func() []plugins.PluginCandidate {
		return []plugins.PluginCandidate{
			{IDHint: "plugin-1", PackageName: "My Plugin", PackageDescription: "A plugin", Origin: plugins.PluginOriginWorkspace},
		}
	}

	catalog := NewPackageCatalog(nil, skillLoader, pluginLoader, nil)
	items, total, err := catalog.Browse(context.Background(), "", "", "", 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// 第一个是 skill
	if items[0].Kind != types.PackageKindSkill {
		t.Errorf("expected skill, got %s", items[0].Kind)
	}
	if items[0].Name != "local-skill-1" {
		t.Errorf("expected name=local-skill-1, got %s", items[0].Name)
	}

	// 第二个是 plugin
	if items[1].Kind != types.PackageKindPlugin {
		t.Errorf("expected plugin, got %s", items[1].Kind)
	}
	if items[1].Name != "My Plugin" {
		t.Errorf("expected name=My Plugin, got %s", items[1].Name)
	}
}

func TestCatalogBrowse_KindFilter(t *testing.T) {
	skillLoader := func() []skills.SkillEntry {
		return []skills.SkillEntry{
			{Skill: skills.Skill{Name: "s1"}, Enabled: true},
		}
	}
	pluginLoader := func() []plugins.PluginCandidate {
		return []plugins.PluginCandidate{
			{IDHint: "p1", PackageName: "P1"},
		}
	}

	catalog := NewPackageCatalog(nil, skillLoader, pluginLoader, nil)

	// 过滤只要 skill
	items, total, err := catalog.Browse(context.Background(), "skill", "", "", 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if items[0].Kind != types.PackageKindSkill {
		t.Errorf("expected skill kind")
	}

	// 过滤只要 plugin
	items, total, err = catalog.Browse(context.Background(), "plugin", "", "", 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if items[0].Kind != types.PackageKindPlugin {
		t.Errorf("expected plugin kind")
	}
}

func TestCatalogBrowse_KeywordFilter(t *testing.T) {
	skillLoader := func() []skills.SkillEntry {
		return []skills.SkillEntry{
			{Skill: skills.Skill{Name: "docker-deploy", Description: "Deploy via Docker"}},
			{Skill: skills.Skill{Name: "git-commit", Description: "Git operations"}},
		}
	}

	catalog := NewPackageCatalog(nil, skillLoader, nil, nil)

	items, total, err := catalog.Browse(context.Background(), "", "docker", "", 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if items[0].Name != "docker-deploy" {
		t.Errorf("expected docker-deploy, got %s", items[0].Name)
	}
}

func TestCatalogBrowse_Pagination(t *testing.T) {
	skillLoader := func() []skills.SkillEntry {
		entries := make([]skills.SkillEntry, 5)
		for i := range entries {
			entries[i] = skills.SkillEntry{Skill: skills.Skill{Name: "skill-" + string(rune('a'+i))}}
		}
		return entries
	}

	catalog := NewPackageCatalog(nil, skillLoader, nil, nil)

	// 第一页 2 条
	items, total, err := catalog.Browse(context.Background(), "", "", "", 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected total=5, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// 第三页应该只有 1 条
	items, _, err = catalog.Browse(context.Background(), "", "", "", 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item on page 3, got %d", len(items))
	}

	// 超出范围返回空
	items, _, err = catalog.Browse(context.Background(), "", "", "", 10, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items beyond range, got %d", len(items))
	}
}

func TestCatalogDetail_NotFound(t *testing.T) {
	catalog := NewPackageCatalog(nil, nil, nil, nil)
	_, err := catalog.Detail(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
}
