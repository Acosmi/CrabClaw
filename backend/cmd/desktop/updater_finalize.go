package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const desktopPendingUpdateFilename = "desktop-update-pending.json"

type desktopPendingUpdate struct {
	InstallKind    string `json:"installKind,omitempty"`
	CurrentPath    string `json:"currentPath,omitempty"`
	BackupPath     string `json:"backupPath,omitempty"`
	DownloadedFile string `json:"downloadedFile,omitempty"`
	FromVersion    string `json:"fromVersion,omitempty"`
	ToVersion      string `json:"toVersion,omitempty"`
	AppliedAt      string `json:"appliedAt,omitempty"`
}

func finalizeDesktopPendingUpdate() error {
	pending, err := readDesktopPendingUpdate()
	if err != nil || pending == nil {
		return err
	}
	if strings.TrimSpace(pending.InstallKind) != string(installKindLinuxAppImage) {
		return nil
	}

	state, err := readDesktopUpdateState()
	if err != nil {
		return err
	}
	if state == nil {
		state = &DesktopUpdateState{}
	}

	currentVersion := resolveDesktopCurrentVersion()
	state.CurrentVersion = currentVersion
	state.InstallKind = detectDesktopInstallKind()
	state.UpdateManager = resolveDesktopUpdateManager(string(state.InstallKind))
	state.ManagedBySystem = state.InstallKind == installKindWindowsMSIX
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	switch currentVersion {
	case strings.TrimSpace(pending.ToVersion):
		clearDesktopPendingCandidate(state)
		state.RollbackAvailable = strings.TrimSpace(pending.BackupPath) != ""
		state.RollbackVersion = strings.TrimSpace(pending.FromVersion)
		state.RollbackBackupPath = strings.TrimSpace(pending.BackupPath)
		state.State = desktopUpdateStageIdle
		state.LastError = ""
	case strings.TrimSpace(pending.FromVersion):
		clearDesktopPendingCandidate(state)
		clearDesktopRollback(state)
		state.State = desktopUpdateStageFailed
		state.LastError = fmt.Sprintf("AppImage update to %s was not confirmed", strings.TrimSpace(pending.ToVersion))
	default:
		return nil
	}

	if _, err := writeDesktopUpdateState(*state); err != nil {
		return err
	}
	return deleteDesktopPendingUpdate()
}

func resolveDesktopPendingUpdatePath() string {
	return filepath.Join(filepath.Dir(resolveDesktopUpdateStatePath()), desktopPendingUpdateFilename)
}

func readDesktopPendingUpdate() (*desktopPendingUpdate, error) {
	raw, err := os.ReadFile(resolveDesktopPendingUpdatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pending desktopPendingUpdate
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("decode pending desktop update: %w", err)
	}
	return &pending, nil
}

func writeDesktopPendingUpdate(pending desktopPendingUpdate) error {
	raw, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pending desktop update: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(resolveDesktopPendingUpdatePath()), 0o755); err != nil {
		return fmt.Errorf("create pending desktop update dir: %w", err)
	}
	if err := os.WriteFile(resolveDesktopPendingUpdatePath(), raw, 0o644); err != nil {
		return fmt.Errorf("write pending desktop update: %w", err)
	}
	return nil
}

func deleteDesktopPendingUpdate() error {
	err := os.Remove(resolveDesktopPendingUpdatePath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func clearDesktopPendingCandidate(state *DesktopUpdateState) {
	state.CandidateVersion = ""
	state.ManifestURL = ""
	state.AssetURL = ""
	state.AssetName = ""
	state.AssetSHA256 = ""
	state.DownloadedFile = ""
	state.PublishedAt = ""
	state.ReadyToInstall = false
	state.Progress = nil
	state.DownloadedBytes = nil
	state.TotalBytes = nil
}
