package packages

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/plugins"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// PackageCatalog 统一目录聚合器。
type PackageCatalog struct {
	skillClient  *skills.SkillStoreClient
	skillLoader  func() []skills.SkillEntry
	pluginLoader func() []plugins.PluginCandidate
	ledger       *PackageLedger
}

// NewPackageCatalog 创建统一目录实例。
func NewPackageCatalog(
	skillClient *skills.SkillStoreClient,
	skillLoader func() []skills.SkillEntry,
	pluginLoader func() []plugins.PluginCandidate,
	ledger *PackageLedger,
) *PackageCatalog {
	return &PackageCatalog{
		skillClient:  skillClient,
		skillLoader:  skillLoader,
		pluginLoader: pluginLoader,
		ledger:       ledger,
	}
}

// Browse 浏览统一目录。
// [FIX P3-M03: 使用 context 控制远程调用超时]
func (c *PackageCatalog) Browse(ctx context.Context, kind, keyword, category string, page, pageSize int) ([]types.PackageCatalogItem, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var items []types.PackageCatalogItem

	// 1. 收集本地技能
	localSkillKeys := make(map[string]bool)
	if c.skillLoader != nil && (kind == "" || kind == string(types.PackageKindSkill)) {
		entries := c.skillLoader()
		for _, entry := range entries {
			item := AdaptLocalSkill(entry)
			items = append(items, item)
			localSkillKeys[item.Key] = true
		}
	}

	// 2. 收集远程技能（去重: 与本地同 key 的合并）
	// [FIX P3-M03: 远程调用带 10s 超时，防止挂起]
	if c.skillClient != nil && c.skillClient.Available() && (kind == "" || kind == string(types.PackageKindSkill)) {
		type browseResult struct {
			items []skills.RemoteSkillItem
			err   error
		}
		browseCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ch := make(chan browseResult, 1)
		go func() {
			items, err := c.skillClient.Browse(category, keyword)
			ch <- browseResult{items, err}
		}()

		var remoteItems []skills.RemoteSkillItem
		var err error
		select {
		case res := <-ch:
			remoteItems, err = res.items, res.err
		case <-browseCtx.Done():
			err = fmt.Errorf("remote browse timeout: %w", browseCtx.Err())
		}
		if err != nil {
			slog.Warn("packages: remote skill browse failed (degraded)", "error", err)
		} else {
			for _, r := range remoteItems {
				if localSkillKeys[r.Key] {
					for i := range items {
						if items[i].Key == r.Key && items[i].Kind == types.PackageKindSkill {
							items[i].Source = "remote"
							items[i].IsInstalled = true
							if items[i].Description == "" {
								items[i].Description = r.Description
							}
							if items[i].Version == "" {
								items[i].Version = r.Version
							}
							if items[i].DownloadCount == 0 {
								items[i].DownloadCount = r.DownloadCount
							}
							break
						}
					}
				} else {
					items = append(items, AdaptRemoteSkill(r, false))
				}
			}
		}
	}

	// 3. 收集本地插件
	if c.pluginLoader != nil && (kind == "" || kind == string(types.PackageKindPlugin)) {
		candidates := c.pluginLoader()
		for _, candidate := range candidates {
			items = append(items, AdaptLocalPlugin(candidate))
		}
	}

	// 4. 关键字过滤
	if keyword != "" {
		kw := strings.ToLower(keyword)
		var filtered []types.PackageCatalogItem
		for _, item := range items {
			if matchesKeyword(item, kw) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// 5. 分页
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return nil, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return items[start:end], total, nil
}

// Detail 获取单个包详情。
func (c *PackageCatalog) Detail(_ context.Context, id string) (*types.PackageCatalogItem, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	if c.skillLoader != nil {
		entries := c.skillLoader()
		for _, entry := range entries {
			item := AdaptLocalSkill(entry)
			if item.ID == id || item.Key == id {
				return &item, nil
			}
		}
	}

	if c.skillClient != nil && c.skillClient.Available() {
		remote, err := c.skillClient.Detail(id)
		if err == nil && remote != nil {
			installed := false
			if c.ledger != nil {
				installed = c.ledger.Has(remote.Key)
			}
			item := AdaptRemoteSkill(*remote, installed)
			return &item, nil
		}
	}

	if c.pluginLoader != nil {
		candidates := c.pluginLoader()
		for _, candidate := range candidates {
			if candidate.IDHint == id {
				item := AdaptLocalPlugin(candidate)
				return &item, nil
			}
		}
	}

	return nil, fmt.Errorf("package not found: %s", id)
}

func matchesKeyword(item types.PackageCatalogItem, keyword string) bool {
	if strings.Contains(strings.ToLower(item.Name), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Description), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Key), keyword) {
		return true
	}
	for _, tag := range item.Tags {
		if strings.Contains(strings.ToLower(tag), keyword) {
			return true
		}
	}
	return false
}
