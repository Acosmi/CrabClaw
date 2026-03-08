package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestLedger_AddAndList(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")
	ledger := NewPackageLedger(fp)

	record := types.PackageInstallRecord{
		ID:          "skill-1",
		Kind:        types.PackageKindSkill,
		Key:         "my-skill",
		Version:     "1.0.0",
		Source:      "remote",
		InstalledAt: "2026-01-01T00:00:00Z",
	}

	if err := ledger.Add(record); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	records := ledger.List("")
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Key != "my-skill" {
		t.Errorf("expected key=my-skill, got %s", records[0].Key)
	}
}

func TestLedger_AddDuplicate(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")
	ledger := NewPackageLedger(fp)

	r1 := types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "k1", Version: "1.0"}
	r2 := types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "k1", Version: "2.0"}

	if err := ledger.Add(r1); err != nil {
		t.Fatalf("Add r1 failed: %v", err)
	}
	if err := ledger.Add(r2); err != nil {
		t.Fatalf("Add r2 failed: %v", err)
	}

	records := ledger.List("")
	if len(records) != 1 {
		t.Fatalf("expected 1 record (dedup), got %d", len(records))
	}
	if records[0].Version != "2.0" {
		t.Errorf("expected updated version=2.0, got %s", records[0].Version)
	}
	if records[0].UpdatedAt == "" {
		t.Error("expected updatedAt to be set on update")
	}
}

func TestLedger_Remove(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")
	ledger := NewPackageLedger(fp)

	ledger.Add(types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "k1"})
	ledger.Add(types.PackageInstallRecord{ID: "s2", Kind: types.PackageKindPlugin, Key: "k2"})

	if err := ledger.Remove("s1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	records := ledger.List("")
	if len(records) != 1 {
		t.Fatalf("expected 1 record after remove, got %d", len(records))
	}
	if records[0].ID != "s2" {
		t.Errorf("expected remaining record ID=s2, got %s", records[0].ID)
	}
}

func TestLedger_Has(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")
	ledger := NewPackageLedger(fp)

	ledger.Add(types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "my-key"})

	if !ledger.Has("my-key") {
		t.Error("expected Has(my-key)=true")
	}
	if ledger.Has("nonexistent") {
		t.Error("expected Has(nonexistent)=false")
	}
}

func TestLedger_ListByKind(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")
	ledger := NewPackageLedger(fp)

	ledger.Add(types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "k1"})
	ledger.Add(types.PackageInstallRecord{ID: "p1", Kind: types.PackageKindPlugin, Key: "k2"})
	ledger.Add(types.PackageInstallRecord{ID: "s2", Kind: types.PackageKindSkill, Key: "k3"})

	skills := ledger.List(types.PackageKindSkill)
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	plugins := ledger.List(types.PackageKindPlugin)
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
}

func TestLedger_Persistence(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")

	// 写入
	ledger1 := NewPackageLedger(fp)
	ledger1.Add(types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "persist-key", Version: "3.0"})

	// 重新加载
	ledger2 := NewPackageLedger(fp)
	records := ledger2.List("")
	if len(records) != 1 {
		t.Fatalf("expected 1 record after reload, got %d", len(records))
	}
	if records[0].Key != "persist-key" {
		t.Errorf("expected key=persist-key after reload, got %s", records[0].Key)
	}
	if records[0].Version != "3.0" {
		t.Errorf("expected version=3.0 after reload, got %s", records[0].Version)
	}
}

func TestLedger_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "installs.json")
	ledger := NewPackageLedger(fp)

	ledger.Add(types.PackageInstallRecord{ID: "s1", Kind: types.PackageKindSkill, Key: "k1"})

	// .tmp 文件不应存在
	tmpFile := fp + ".tmp"
	if _, err := os.Stat(tmpFile); err == nil {
		t.Error("tmp file should not exist after successful write")
	}

	// 主文件应存在
	if _, err := os.Stat(fp); err != nil {
		t.Errorf("main file should exist: %v", err)
	}
}
