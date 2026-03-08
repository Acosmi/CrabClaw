package packages

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestAdaptRemoteSkill(t *testing.T) {
	remote := skills.RemoteSkillItem{
		ID:            "abc-123",
		Key:           "my-skill",
		Name:          "My Skill",
		Description:   "Does stuff",
		Version:       "1.0.0",
		Author:        "Alice",
		Icon:          "star",
		SecurityLevel: "standard",
		SecurityScore: 85,
		DownloadCount: 1234,
		Tags:          "ai, automation, test",
	}

	item := AdaptRemoteSkill(remote, true)

	if item.ID != "abc-123" {
		t.Errorf("expected ID=abc-123, got %s", item.ID)
	}
	if item.Kind != types.PackageKindSkill {
		t.Errorf("expected kind=skill, got %s", item.Kind)
	}
	if item.Key != "my-skill" {
		t.Errorf("expected key=my-skill, got %s", item.Key)
	}
	if item.Source != "remote" {
		t.Errorf("expected source=remote, got %s", item.Source)
	}
	if !item.IsInstalled {
		t.Error("expected isInstalled=true")
	}
	if item.Author != "Alice" {
		t.Errorf("expected author=Alice, got %s", item.Author)
	}
	if item.SecurityScore != 85 {
		t.Errorf("expected securityScore=85, got %d", item.SecurityScore)
	}
	if item.DownloadCount != 1234 {
		t.Errorf("expected downloadCount=1234, got %d", item.DownloadCount)
	}
	if len(item.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(item.Tags))
	}
	if item.Tags[0] != "ai" || item.Tags[1] != "automation" || item.Tags[2] != "test" {
		t.Errorf("unexpected tags: %v", item.Tags)
	}
}

func TestAdaptRemoteSkill_NotInstalled(t *testing.T) {
	remote := skills.RemoteSkillItem{
		ID:   "xyz",
		Key:  "other",
		Name: "Other",
	}
	item := AdaptRemoteSkill(remote, false)
	if item.IsInstalled {
		t.Error("expected isInstalled=false")
	}
}

func TestAdaptLocalSkill(t *testing.T) {
	entry := skills.SkillEntry{
		Skill: skills.Skill{
			Name:        "my-local-skill",
			Description: "A local skill",
		},
		Enabled: true,
		Metadata: &skills.OpenAcosmiSkillMetadata{
			SkillKey: "local-key",
			Emoji:    "🔧",
		},
	}

	item := AdaptLocalSkill(entry)

	if item.Kind != types.PackageKindSkill {
		t.Errorf("expected kind=skill, got %s", item.Kind)
	}
	if item.Key != "local-key" {
		t.Errorf("expected key=local-key (from metadata), got %s", item.Key)
	}
	if item.Source != "local" {
		t.Errorf("expected source=local, got %s", item.Source)
	}
	if !item.IsInstalled {
		t.Error("expected isInstalled=true for local skill")
	}
	if item.Icon != "🔧" {
		t.Errorf("expected icon=🔧, got %s", item.Icon)
	}
	if item.Description != "A local skill" {
		t.Errorf("expected description from Skill, got %s", item.Description)
	}
}

func TestAdaptLocalSkill_NoMetadata(t *testing.T) {
	entry := skills.SkillEntry{
		Skill: skills.Skill{Name: "bare-skill"},
	}

	item := AdaptLocalSkill(entry)

	if item.Key != "bare-skill" {
		t.Errorf("expected key=bare-skill (fallback to name), got %s", item.Key)
	}
	if item.Icon != "" {
		t.Errorf("expected empty icon, got %s", item.Icon)
	}
}
