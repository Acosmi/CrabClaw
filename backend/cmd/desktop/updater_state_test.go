package main

import (
	"os"
	"path/filepath"
	"testing"

	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestWriteReadDeleteDesktopUpdateStateAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop-update-state.json")
	progress := 0.5
	downloaded := int64(512)
	total := int64(1024)

	state := DesktopUpdateState{
		CurrentVersion:   "1.0.0",
		CandidateVersion: "1.1.0",
		Channel:          "beta",
		InstallKind:      installKindWindowsNSIS,
		State:            desktopUpdateStageDownloading,
		Progress:         &progress,
		DownloadedBytes:  &downloaded,
		TotalBytes:       &total,
		ReadyToInstall:   false,
		LastCheckedAt:    "2026-03-09T12:00:00Z",
	}

	if _, err := writeDesktopUpdateStateAt(path, state); err != nil {
		t.Fatalf("writeDesktopUpdateStateAt: %v", err)
	}

	got, err := readDesktopUpdateStateAt(path)
	if err != nil {
		t.Fatalf("readDesktopUpdateStateAt: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil update state")
	}
	if got.CandidateVersion != "1.1.0" {
		t.Fatalf("candidateVersion: got %q", got.CandidateVersion)
	}
	if got.InstallKind != installKindWindowsNSIS {
		t.Fatalf("installKind: got %q", got.InstallKind)
	}
	if got.State != desktopUpdateStageDownloading {
		t.Fatalf("state: got %q", got.State)
	}
	if got.UpdatedAt == "" {
		t.Fatal("expected UpdatedAt to be auto-populated")
	}

	if _, err := readDesktopUpdateStateAt(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatalf("expected missing state file to return nil, got error: %v", err)
	}
}

func TestSyncDesktopUpdateStateFromConfig(t *testing.T) {
	tmpHome := t.TempDir()
	oldHome := os.Getenv("OPENACOSMI_HOME")
	oldProjectRoot := os.Getenv("CRABCLAW_PROJECT_ROOT")
	t.Cleanup(func() {
		_ = os.Setenv("OPENACOSMI_HOME", oldHome)
		_ = os.Setenv("CRABCLAW_PROJECT_ROOT", oldProjectRoot)
	})
	_ = os.Setenv("OPENACOSMI_HOME", tmpHome)
	_ = os.Setenv("CRABCLAW_PROJECT_ROOT", "/Users/dev/Desktop/CrabClaw")

	cfg := &types.OpenAcosmiConfig{
		Update: &types.OpenAcosmiUpdateConfig{
			Channel:       "beta",
			LastCheckedAt: "2026-03-09T15:00:00Z",
		},
	}

	if err := syncDesktopUpdateStateFromConfig(cfg); err != nil {
		t.Fatalf("syncDesktopUpdateStateFromConfig: %v", err)
	}

	got, err := readDesktopUpdateState()
	if err != nil {
		t.Fatalf("readDesktopUpdateState: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted desktop update state")
	}
	if got.Channel != "beta" {
		t.Fatalf("channel: got %q", got.Channel)
	}
	if got.InstallKind != installKindSource {
		t.Fatalf("installKind: got %q", got.InstallKind)
	}
	if got.UpdateManager != "source" {
		t.Fatalf("updateManager: got %q", got.UpdateManager)
	}
	if got.State != desktopUpdateStageIdle {
		t.Fatalf("state: got %q", got.State)
	}
	if got.CurrentVersion == "" {
		t.Fatal("expected currentVersion to be set")
	}
}

func TestFinalizeDesktopPendingUpdate_SuccessfulAppImageStartClearsCandidate(t *testing.T) {
	stateDir := t.TempDir()
	oldStateDir := os.Getenv("CRABCLAW_STATE_DIR")
	oldVersion := version
	oldAppImage := os.Getenv("APPIMAGE")
	t.Cleanup(func() {
		_ = os.Setenv("CRABCLAW_STATE_DIR", oldStateDir)
		_ = os.Setenv("APPIMAGE", oldAppImage)
		version = oldVersion
	})
	_ = os.Setenv("CRABCLAW_STATE_DIR", stateDir)
	_ = os.Setenv("APPIMAGE", filepath.Join(stateDir, "CrabClaw.AppImage"))
	version = "5.1.0"

	if _, err := writeDesktopUpdateState(DesktopUpdateState{
		CurrentVersion:   "5.0.0",
		CandidateVersion: "5.1.0",
		InstallKind:      installKindLinuxAppImage,
		UpdateManager:    "appimage",
		State:            desktopUpdateStageApplying,
		DownloadedFile:   filepath.Join(stateDir, "updates", "5.1.0", "CrabClaw.AppImage"),
		ReadyToInstall:   true,
	}); err != nil {
		t.Fatalf("writeDesktopUpdateState: %v", err)
	}
	if err := writeDesktopPendingUpdate(desktopPendingUpdate{
		InstallKind:    string(installKindLinuxAppImage),
		CurrentPath:    filepath.Join(stateDir, "CrabClaw.AppImage"),
		BackupPath:     filepath.Join(stateDir, "updates", "backups", "old.AppImage"),
		DownloadedFile: filepath.Join(stateDir, "updates", "5.1.0", "CrabClaw.AppImage"),
		FromVersion:    "5.0.0",
		ToVersion:      "5.1.0",
		AppliedAt:      "2026-03-09T12:00:00Z",
	}); err != nil {
		t.Fatalf("writeDesktopPendingUpdate: %v", err)
	}

	if err := finalizeDesktopPendingUpdate(); err != nil {
		t.Fatalf("finalizeDesktopPendingUpdate: %v", err)
	}

	got, err := readDesktopUpdateState()
	if err != nil {
		t.Fatalf("readDesktopUpdateState: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted desktop update state")
	}
	if got.CurrentVersion != "5.1.0" {
		t.Fatalf("currentVersion: got %q", got.CurrentVersion)
	}
	if got.CandidateVersion != "" {
		t.Fatalf("candidateVersion should be cleared, got %q", got.CandidateVersion)
	}
	if !got.RollbackAvailable {
		t.Fatal("expected rollback to remain available after successful update")
	}
	if got.RollbackVersion != "5.0.0" {
		t.Fatalf("rollbackVersion: got %q", got.RollbackVersion)
	}
	if got.RollbackBackupPath == "" {
		t.Fatal("expected rollback backup path to be preserved")
	}
	if got.State != desktopUpdateStageIdle {
		t.Fatalf("state: got %q", got.State)
	}
	if pending, err := readDesktopPendingUpdate(); err != nil {
		t.Fatalf("readDesktopPendingUpdate: %v", err)
	} else if pending != nil {
		t.Fatal("expected pending desktop update marker to be deleted")
	}
}

func TestFinalizeDesktopPendingUpdate_PreviousVersionMarksFailure(t *testing.T) {
	stateDir := t.TempDir()
	oldStateDir := os.Getenv("CRABCLAW_STATE_DIR")
	oldVersion := version
	oldAppImage := os.Getenv("APPIMAGE")
	t.Cleanup(func() {
		_ = os.Setenv("CRABCLAW_STATE_DIR", oldStateDir)
		_ = os.Setenv("APPIMAGE", oldAppImage)
		version = oldVersion
	})
	_ = os.Setenv("CRABCLAW_STATE_DIR", stateDir)
	_ = os.Setenv("APPIMAGE", filepath.Join(stateDir, "CrabClaw.AppImage"))
	version = "6.0.0"

	if _, err := writeDesktopUpdateState(DesktopUpdateState{
		CurrentVersion:     "6.0.0",
		CandidateVersion:   "6.1.0",
		InstallKind:        installKindLinuxAppImage,
		UpdateManager:      "appimage",
		State:              desktopUpdateStageApplying,
		DownloadedFile:     filepath.Join(stateDir, "updates", "6.1.0", "CrabClaw.AppImage"),
		ReadyToInstall:     true,
		RollbackAvailable:  true,
		RollbackVersion:    "6.0.0",
		RollbackBackupPath: filepath.Join(stateDir, "updates", "backups", "6.0.0.AppImage"),
	}); err != nil {
		t.Fatalf("writeDesktopUpdateState: %v", err)
	}
	if err := writeDesktopPendingUpdate(desktopPendingUpdate{
		InstallKind: string(installKindLinuxAppImage),
		FromVersion: "6.0.0",
		ToVersion:   "6.1.0",
	}); err != nil {
		t.Fatalf("writeDesktopPendingUpdate: %v", err)
	}

	if err := finalizeDesktopPendingUpdate(); err != nil {
		t.Fatalf("finalizeDesktopPendingUpdate: %v", err)
	}

	got, err := readDesktopUpdateState()
	if err != nil {
		t.Fatalf("readDesktopUpdateState: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted desktop update state")
	}
	if got.State != desktopUpdateStageFailed {
		t.Fatalf("state: got %q", got.State)
	}
	if got.CandidateVersion != "" {
		t.Fatalf("candidateVersion should be cleared, got %q", got.CandidateVersion)
	}
	if got.DownloadedFile != "" {
		t.Fatalf("downloadedFile should be cleared, got %q", got.DownloadedFile)
	}
	if got.RollbackAvailable {
		t.Fatal("expected rollback metadata to be cleared")
	}
	if got.LastError == "" {
		t.Fatal("expected LastError to be populated")
	}
	if pending, err := readDesktopPendingUpdate(); err != nil {
		t.Fatalf("readDesktopPendingUpdate: %v", err)
	} else if pending != nil {
		t.Fatal("expected pending desktop update marker to be deleted")
	}
}

func TestFinalizeDesktopInstallerHandoff_NewVersionClearsCandidate(t *testing.T) {
	stateDir := t.TempDir()
	oldStateDir := os.Getenv("CRABCLAW_STATE_DIR")
	oldVersion := version
	t.Cleanup(func() {
		_ = os.Setenv("CRABCLAW_STATE_DIR", oldStateDir)
		version = oldVersion
	})
	_ = os.Setenv("CRABCLAW_STATE_DIR", stateDir)
	version = "10.1.0"

	if _, err := writeDesktopUpdateState(DesktopUpdateState{
		CurrentVersion:   "10.0.0",
		CandidateVersion: "10.1.0",
		InstallKind:      installKindMacOSWails,
		UpdateManager:    "host",
		State:            desktopUpdateStageApplying,
		DownloadedFile:   filepath.Join(stateDir, "updates", "10.1.0", "CrabClaw.dmg"),
		ReadyToInstall:   true,
	}); err != nil {
		t.Fatalf("writeDesktopUpdateState: %v", err)
	}
	if err := writeDesktopInstallerHandoff(desktopInstallerHandoff{
		InstallKind:  string(installKindMacOSWails),
		ArtifactPath: filepath.Join(stateDir, "updates", "10.1.0", "CrabClaw.dmg"),
		ArtifactName: "CrabClaw.dmg",
		FromVersion:  "10.0.0",
		ToVersion:    "10.1.0",
		LaunchedAt:   "2026-03-09T12:00:00Z",
	}); err != nil {
		t.Fatalf("writeDesktopInstallerHandoff: %v", err)
	}

	if err := finalizeDesktopInstallerHandoff(); err != nil {
		t.Fatalf("finalizeDesktopInstallerHandoff: %v", err)
	}

	got, err := readDesktopUpdateState()
	if err != nil {
		t.Fatalf("readDesktopUpdateState: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted desktop update state")
	}
	if got.State != desktopUpdateStageIdle {
		t.Fatalf("state: got %q", got.State)
	}
	if got.CandidateVersion != "" {
		t.Fatalf("candidateVersion should be cleared, got %q", got.CandidateVersion)
	}
	if got.DownloadedFile != "" {
		t.Fatalf("downloadedFile should be cleared, got %q", got.DownloadedFile)
	}
	if got.LastError != "" {
		t.Fatalf("expected no LastError, got %q", got.LastError)
	}
	if handoff, err := readDesktopInstallerHandoff(); err != nil {
		t.Fatalf("readDesktopInstallerHandoff: %v", err)
	} else if handoff != nil {
		t.Fatal("expected installer handoff marker to be deleted")
	}
}

func TestFinalizeDesktopInstallerHandoff_SameVersionMarksFailure(t *testing.T) {
	stateDir := t.TempDir()
	oldStateDir := os.Getenv("CRABCLAW_STATE_DIR")
	oldVersion := version
	t.Cleanup(func() {
		_ = os.Setenv("CRABCLAW_STATE_DIR", oldStateDir)
		version = oldVersion
	})
	_ = os.Setenv("CRABCLAW_STATE_DIR", stateDir)
	version = "11.0.0"

	if _, err := writeDesktopUpdateState(DesktopUpdateState{
		CurrentVersion:   "11.0.0",
		CandidateVersion: "11.1.0",
		InstallKind:      installKindWindowsNSIS,
		UpdateManager:    "installer",
		State:            desktopUpdateStageApplying,
		DownloadedFile:   filepath.Join(stateDir, "updates", "11.1.0", "CrabClaw-setup.exe"),
		ReadyToInstall:   true,
	}); err != nil {
		t.Fatalf("writeDesktopUpdateState: %v", err)
	}
	if err := writeDesktopInstallerHandoff(desktopInstallerHandoff{
		InstallKind:  string(installKindWindowsNSIS),
		ArtifactPath: filepath.Join(stateDir, "updates", "11.1.0", "CrabClaw-setup.exe"),
		ArtifactName: "CrabClaw-setup.exe",
		FromVersion:  "11.0.0",
		ToVersion:    "11.1.0",
		LaunchedAt:   "2026-03-09T12:00:00Z",
	}); err != nil {
		t.Fatalf("writeDesktopInstallerHandoff: %v", err)
	}

	if err := finalizeDesktopInstallerHandoff(); err != nil {
		t.Fatalf("finalizeDesktopInstallerHandoff: %v", err)
	}

	got, err := readDesktopUpdateState()
	if err != nil {
		t.Fatalf("readDesktopUpdateState: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted desktop update state")
	}
	if got.State != desktopUpdateStageFailed {
		t.Fatalf("state: got %q", got.State)
	}
	if got.CandidateVersion != "" {
		t.Fatalf("candidateVersion should be cleared, got %q", got.CandidateVersion)
	}
	if got.DownloadedFile != "" {
		t.Fatalf("downloadedFile should be cleared, got %q", got.DownloadedFile)
	}
	if got.LastError == "" {
		t.Fatal("expected LastError to be populated")
	}
	if handoff, err := readDesktopInstallerHandoff(); err != nil {
		t.Fatalf("readDesktopInstallerHandoff: %v", err)
	} else if handoff != nil {
		t.Fatal("expected installer handoff marker to be deleted")
	}
}
