package packages

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// PackageInstaller 统一安装编排器。
type PackageInstaller struct {
	skillClient   *skills.SkillStoreClient
	docsSkillsDir string
	ledger        *PackageLedger
	catalogDetail func(id string) (*types.PackageCatalogItem, error)
}

// NewPackageInstaller 创建安装编排器。
func NewPackageInstaller(
	skillClient *skills.SkillStoreClient,
	docsSkillsDir string,
	ledger *PackageLedger,
	catalogDetail func(id string) (*types.PackageCatalogItem, error),
) *PackageInstaller {
	return &PackageInstaller{
		skillClient:   skillClient,
		docsSkillsDir: docsSkillsDir,
		ledger:        ledger,
		catalogDetail: catalogDetail,
	}
}

// Install 安装指定包。
func (inst *PackageInstaller) Install(kind types.PackageKind, id string) (*types.PackageInstallRecord, error) {
	// [FIX P3-M02: Package ID 格式校验，防止路径穿越]
	if id == "" || strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return nil, fmt.Errorf("invalid package id: %q", id)
	}

	switch kind {
	case types.PackageKindSkill:
		return inst.installSkill(id)
	case types.PackageKindPlugin:
		return inst.installPlugin(id)
	case types.PackageKindBundle:
		return inst.installBundle(id)
	default:
		return nil, fmt.Errorf("unsupported package kind: %s", kind)
	}
}

// Remove 移除指定包。
func (inst *PackageInstaller) Remove(kind types.PackageKind, id string) error {
	if inst.ledger == nil {
		return fmt.Errorf("ledger not available")
	}
	return inst.ledger.Remove(id)
}

func (inst *PackageInstaller) installSkill(id string) (*types.PackageInstallRecord, error) {
	if inst.skillClient == nil || !inst.skillClient.Available() {
		return nil, fmt.Errorf("skill store client not available")
	}
	if inst.docsSkillsDir == "" {
		return nil, fmt.Errorf("docs skills directory not configured")
	}

	result, err := skills.PullSkillToLocal(inst.skillClient, id, inst.docsSkillsDir)
	if err != nil {
		return nil, fmt.Errorf("install skill %s: %w", id, err)
	}

	record := types.PackageInstallRecord{
		ID:          id,
		Kind:        types.PackageKindSkill,
		Key:         result.SkillName,
		Source:      "remote",
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}

	if inst.ledger != nil {
		if err := inst.ledger.Add(record); err != nil {
			slog.Warn("packages: ledger add failed", "error", err, "id", id)
		}
	}

	return &record, nil
}

func (inst *PackageInstaller) installPlugin(id string) (*types.PackageInstallRecord, error) {
	if inst.catalogDetail == nil {
		return nil, fmt.Errorf("catalog detail not available")
	}
	item, err := inst.catalogDetail(id)
	if err != nil {
		return nil, fmt.Errorf("plugin detail: %w", err)
	}
	if item == nil {
		return nil, fmt.Errorf("plugin not found: %s", id)
	}

	mode := item.ExecutionMode
	if mode != "" && mode != "builtin" && mode != "bridge" {
		return nil, fmt.Errorf("plugin install rejected: executionMode=%q not allowed (only builtin|bridge)", mode)
	}

	record := types.PackageInstallRecord{
		ID:          id,
		Kind:        types.PackageKindPlugin,
		Key:         item.Key,
		Version:     item.Version,
		Source:      item.Source,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}

	if inst.ledger != nil {
		if err := inst.ledger.Add(record); err != nil {
			slog.Warn("packages: ledger add failed", "error", err, "id", id)
		}
	}

	return &record, nil
}

func (inst *PackageInstaller) installBundle(id string) (*types.PackageInstallRecord, error) {
	if inst.catalogDetail == nil {
		return nil, fmt.Errorf("catalog detail not available")
	}
	item, err := inst.catalogDetail(id)
	if err != nil {
		return nil, fmt.Errorf("bundle detail: %w", err)
	}
	if item == nil || item.Kind != types.PackageKindBundle {
		return nil, fmt.Errorf("bundle not found: %s", id)
	}
	if len(item.BundleItems) == 0 {
		return nil, fmt.Errorf("bundle %s has no items", id)
	}

	// 事务化安装
	var installed []types.PackageInstallRecord
	for _, ref := range item.BundleItems {
		subRecord, err := inst.Install(ref.Kind, ref.ID)
		if err != nil {
			// [FIX P3-L01: 回滚错误统计]
			slog.Warn("packages: bundle item failed, rolling back",
				"bundleId", id, "failedItem", ref.ID, "error", err,
				"rollbackCount", len(installed))
			var rollbackErrors int
			for _, r := range installed {
				if removeErr := inst.Remove(r.Kind, r.ID); removeErr != nil {
					rollbackErrors++
					slog.Error("packages: rollback failed", "id", r.ID, "error", removeErr)
				}
			}
			if rollbackErrors > 0 {
				return nil, fmt.Errorf("bundle install failed at item %s (rollback: %d/%d errors): %w",
					ref.ID, rollbackErrors, len(installed), err)
			}
			return nil, fmt.Errorf("bundle install failed at item %s: %w", ref.ID, err)
		}
		installed = append(installed, *subRecord)
	}

	bundleRecord := types.PackageInstallRecord{
		ID:          id,
		Kind:        types.PackageKindBundle,
		Key:         item.Key,
		Version:     item.Version,
		Source:      item.Source,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if inst.ledger != nil {
		if err := inst.ledger.Add(bundleRecord); err != nil {
			slog.Warn("packages: ledger add bundle failed", "error", err, "id", id)
		}
	}

	return &bundleRecord, nil
}
