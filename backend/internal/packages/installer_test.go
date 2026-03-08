package packages

import (
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestInstaller_InstallPlugin_RejectsWasmMode(t *testing.T) {
	dir := t.TempDir()
	ledger := NewPackageLedger(filepath.Join(dir, "installs.json"))

	catalogDetail := func(id string) (*types.PackageCatalogItem, error) {
		return &types.PackageCatalogItem{
			ID:            id,
			Kind:          types.PackageKindPlugin,
			Key:           "wasm-plugin",
			ExecutionMode: "wasm",
		}, nil
	}

	installer := NewPackageInstaller(nil, "", ledger, catalogDetail)
	_, err := installer.Install(types.PackageKindPlugin, "wasm-plugin")
	if err == nil {
		t.Fatal("expected error for wasm execution mode")
	}
	if got := err.Error(); !contains(got, "not allowed") {
		t.Errorf("expected 'not allowed' error, got: %s", got)
	}
}

func TestInstaller_InstallPlugin_AllowsBuiltin(t *testing.T) {
	dir := t.TempDir()
	ledger := NewPackageLedger(filepath.Join(dir, "installs.json"))

	catalogDetail := func(id string) (*types.PackageCatalogItem, error) {
		return &types.PackageCatalogItem{
			ID:            id,
			Kind:          types.PackageKindPlugin,
			Key:           "builtin-plugin",
			ExecutionMode: "builtin",
			Source:        "local",
		}, nil
	}

	installer := NewPackageInstaller(nil, "", ledger, catalogDetail)
	record, err := installer.Install(types.PackageKindPlugin, "builtin-plugin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Kind != types.PackageKindPlugin {
		t.Errorf("expected kind=plugin, got %s", record.Kind)
	}
	if record.Key != "builtin-plugin" {
		t.Errorf("expected key=builtin-plugin, got %s", record.Key)
	}

	// 验证写入账本
	if !ledger.Has("builtin-plugin") {
		t.Error("expected ledger to have builtin-plugin")
	}
}

func TestInstaller_InstallBundle_TransactionRollback(t *testing.T) {
	dir := t.TempDir()
	ledger := NewPackageLedger(filepath.Join(dir, "installs.json"))

	callCount := 0
	catalogDetail := func(id string) (*types.PackageCatalogItem, error) {
		if id == "bundle-1" {
			return &types.PackageCatalogItem{
				ID:   "bundle-1",
				Kind: types.PackageKindBundle,
				Key:  "test-bundle",
				BundleItems: []types.BundleRef{
					{Kind: types.PackageKindPlugin, ID: "ok-plugin", Key: "ok-plugin"},
					{Kind: types.PackageKindPlugin, ID: "fail-plugin", Key: "fail-plugin"},
				},
			}, nil
		}
		if id == "ok-plugin" {
			callCount++
			return &types.PackageCatalogItem{
				ID:            "ok-plugin",
				Kind:          types.PackageKindPlugin,
				Key:           "ok-plugin",
				ExecutionMode: "builtin",
			}, nil
		}
		if id == "fail-plugin" {
			callCount++
			// 返回 wasm 模式使安装失败
			return &types.PackageCatalogItem{
				ID:            "fail-plugin",
				Kind:          types.PackageKindPlugin,
				Key:           "fail-plugin",
				ExecutionMode: "wasm",
			}, nil
		}
		return nil, nil
	}

	installer := NewPackageInstaller(nil, "", ledger, catalogDetail)
	_, err := installer.Install(types.PackageKindBundle, "bundle-1")
	if err == nil {
		t.Fatal("expected bundle install to fail")
	}

	// 回滚: ok-plugin 应该从账本移除
	records := ledger.List("")
	if len(records) != 0 {
		t.Errorf("expected 0 records after rollback, got %d", len(records))
	}
}

func TestInstaller_Remove(t *testing.T) {
	dir := t.TempDir()
	ledger := NewPackageLedger(filepath.Join(dir, "installs.json"))
	ledger.Add(types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "k1"})

	installer := NewPackageInstaller(nil, "", ledger, nil)
	if err := installer.Remove(types.PackageKindSkill, "s1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if ledger.Has("k1") {
		t.Error("expected ledger to not have k1 after remove")
	}
}

func TestInstaller_UnsupportedKind(t *testing.T) {
	installer := NewPackageInstaller(nil, "", nil, nil)
	_, err := installer.Install(types.PackageKind("unknown"), "some-id")
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
