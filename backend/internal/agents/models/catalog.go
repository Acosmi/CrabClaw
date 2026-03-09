package models

import (
	"sort"
	"strings"
	"sync"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- 模型目录 ----------

// ModelCatalogEntry 模型目录条目。
// TS 参考: model-catalog.ts → ModelCatalogEntry
type ModelCatalogEntry struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	ContextWindow *int     `json:"contextWindow,omitempty"`
	Reasoning     *bool    `json:"reasoning,omitempty"`
	Input         []string `json:"input,omitempty"` // ["text", "image"]
}

// ModelCatalog 从 ModelRegistry 构造的全量模型目录。
// 按 provider → name 排序。
type ModelCatalog struct {
	mu      sync.RWMutex
	entries []ModelCatalogEntry
}

// NewModelCatalog 创建空的模型目录。
func NewModelCatalog() *ModelCatalog {
	return &ModelCatalog{}
}

// BuildFromRegistry 从注册表构建完整的模型目录。
// TS 参考: model-catalog.ts → loadModelCatalog()
func (c *ModelCatalog) BuildFromRegistry(registry *ModelRegistry) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	var entries []ModelCatalogEntry
	for providerName, provider := range registry.providers {
		for _, m := range provider.Models {
			id := strings.TrimSpace(m.ID)
			if id == "" {
				continue
			}
			prov := strings.TrimSpace(providerName)
			if prov == "" {
				continue
			}
			name := strings.TrimSpace(m.Name)
			if name == "" {
				name = id
			}
			entry := ModelCatalogEntry{
				ID:            id,
				Name:          name,
				Provider:      prov,
				ContextWindow: m.ContextWindow,
				Reasoning:     m.Reasoning,
				Input:         m.Input,
			}
			entries = append(entries, entry)
		}
	}

	// 排序: provider ASC, name ASC
	sort.Slice(entries, func(i, j int) bool {
		pi := entries[i].Provider
		pj := entries[j].Provider
		if pi != pj {
			return pi < pj
		}
		return entries[i].Name < entries[j].Name
	})

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = entries
}

// BuildFromConfig 从运行时配置构建完整模型目录。
func (c *ModelCatalog) BuildFromConfig(cfg *types.OpenAcosmiConfig) {
	var entries []ModelCatalogEntry
	if cfg != nil && cfg.Models != nil {
		for providerName, provider := range cfg.Models.Providers {
			if provider == nil {
				continue
			}
			prov := strings.TrimSpace(providerName)
			if prov == "" {
				continue
			}
			for _, m := range provider.Models {
				id := strings.TrimSpace(m.ID)
				if id == "" {
					continue
				}
				name := strings.TrimSpace(m.Name)
				if name == "" {
					name = id
				}
				entry := ModelCatalogEntry{
					ID:       id,
					Name:     name,
					Provider: prov,
				}
				if m.ContextWindow > 0 {
					contextWindow := m.ContextWindow
					entry.ContextWindow = &contextWindow
				}
				if m.Reasoning {
					reasoning := true
					entry.Reasoning = &reasoning
				}
				if len(m.Input) > 0 {
					entry.Input = make([]string, 0, len(m.Input))
					for _, input := range m.Input {
						entry.Input = append(entry.Input, string(input))
					}
				}
				entries = append(entries, entry)
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		pi := entries[i].Provider
		pj := entries[j].Provider
		if pi != pj {
			return pi < pj
		}
		return entries[i].Name < entries[j].Name
	})

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = entries
}

// All 返回全部目录条目（副本）。
func (c *ModelCatalog) All() []ModelCatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]ModelCatalogEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

// FindModel 按 provider 和 modelID 查找, 大小写不敏感。
// TS 参考: model-catalog.ts → findModelInCatalog()
func (c *ModelCatalog) FindModel(provider, modelID string) *ModelCatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	np := strings.ToLower(strings.TrimSpace(provider))
	nm := strings.ToLower(strings.TrimSpace(modelID))
	for i := range c.entries {
		if strings.ToLower(c.entries[i].Provider) == np &&
			strings.ToLower(c.entries[i].ID) == nm {
			entry := c.entries[i]
			return &entry
		}
	}
	return nil
}

// ModelSupportsVision 检查模型是否支持图片输入。
// TS 参考: model-catalog.ts → modelSupportsVision()
func ModelSupportsVision(entry *ModelCatalogEntry) bool {
	if entry == nil {
		return false
	}
	for _, inp := range entry.Input {
		if inp == "image" {
			return true
		}
	}
	return false
}
