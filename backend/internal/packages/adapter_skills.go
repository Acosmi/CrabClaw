package packages

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// AdaptRemoteSkill 将远程技能商店条目映射为统一 PackageCatalogItem。
func AdaptRemoteSkill(item skills.RemoteSkillItem, installed bool) types.PackageCatalogItem {
	var tags []string
	if item.Tags != "" {
		for _, t := range strings.Split(item.Tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	return types.PackageCatalogItem{
		ID:            item.ID,
		Kind:          types.PackageKindSkill,
		Key:           item.Key,
		Name:          item.Name,
		Description:   item.Description,
		Icon:          item.Icon,
		Version:       item.Version,
		Author:        item.Author,
		Tags:          tags,
		SecurityLevel: item.SecurityLevel,
		SecurityScore: item.SecurityScore,
		DownloadCount: item.DownloadCount,
		Source:        "remote",
		IsInstalled:   installed,
	}
}

// AdaptLocalSkill 将本地技能条目映射为统一 PackageCatalogItem。
func AdaptLocalSkill(entry skills.SkillEntry) types.PackageCatalogItem {
	item := types.PackageCatalogItem{
		ID:          entry.Skill.Name,
		Kind:        types.PackageKindSkill,
		Key:         entry.Skill.Name,
		Name:        entry.Skill.Name,
		Description: entry.Skill.Description,
		Source:      "local",
		IsInstalled: true,
	}

	// [FIX P0-L01: 从 Metadata.Tools 填充 CapabilityTags]
	if m := entry.Metadata; m != nil {
		if m.SkillKey != "" {
			item.Key = m.SkillKey
			item.ID = m.SkillKey
		}
		if m.Emoji != "" {
			item.Icon = m.Emoji
		}
		if len(m.Tools) > 0 {
			item.CapabilityTags = m.Tools
		}
	}

	return item
}
