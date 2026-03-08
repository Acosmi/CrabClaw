package packages

import (
	"github.com/Acosmi/ClawAcosmi/internal/plugins"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// AdaptLocalPlugin 将插件发现候选项映射为统一 PackageCatalogItem。
func AdaptLocalPlugin(candidate plugins.PluginCandidate) types.PackageCatalogItem {
	name := candidate.PackageName
	if name == "" {
		name = candidate.IDHint
	}

	source := "local"
	if candidate.Origin == plugins.PluginOriginBundled {
		source = "builtin"
	}

	return types.PackageCatalogItem{
		ID:          candidate.IDHint,
		Kind:        types.PackageKindPlugin,
		Key:         candidate.IDHint,
		Name:        name,
		Description: candidate.PackageDescription,
		Version:     candidate.PackageVersion,
		Source:      source,
		IsInstalled: true,
	}
}
