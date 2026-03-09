package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const desktopInstallerHandoffFilename = "desktop-update-handoff.json"

type desktopInstallerHandoff struct {
	InstallKind  string `json:"installKind,omitempty"`
	ArtifactPath string `json:"artifactPath,omitempty"`
	ArtifactName string `json:"artifactName,omitempty"`
	FromVersion  string `json:"fromVersion,omitempty"`
	ToVersion    string `json:"toVersion,omitempty"`
	LaunchedAt   string `json:"launchedAt,omitempty"`
}

func finalizeDesktopInstallerHandoff() error {
	handoff, err := readDesktopInstallerHandoff()
	if err != nil || handoff == nil {
		return err
	}

	installKind := strings.TrimSpace(handoff.InstallKind)
	if installKind != string(installKindWindowsNSIS) && installKind != string(installKindMacOSWails) {
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

	clearDesktopPendingCandidate(state)
	clearDesktopRollback(state)

	if currentVersion == strings.TrimSpace(handoff.FromVersion) {
		state.State = desktopUpdateStageFailed
		state.LastError = fmt.Sprintf("%s installer handoff to %s was not confirmed", installKind, strings.TrimSpace(handoff.ToVersion))
	} else {
		state.State = desktopUpdateStageIdle
		state.LastError = ""
	}

	if _, err := writeDesktopUpdateState(*state); err != nil {
		return err
	}
	return deleteDesktopInstallerHandoff()
}

func resolveDesktopInstallerHandoffPath() string {
	return filepath.Join(filepath.Dir(resolveDesktopUpdateStatePath()), desktopInstallerHandoffFilename)
}

func readDesktopInstallerHandoff() (*desktopInstallerHandoff, error) {
	raw, err := os.ReadFile(resolveDesktopInstallerHandoffPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var handoff desktopInstallerHandoff
	if err := json.Unmarshal(raw, &handoff); err != nil {
		return nil, fmt.Errorf("decode desktop installer handoff: %w", err)
	}
	return &handoff, nil
}

func writeDesktopInstallerHandoff(handoff desktopInstallerHandoff) error {
	raw, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop installer handoff: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(resolveDesktopInstallerHandoffPath()), 0o755); err != nil {
		return fmt.Errorf("create desktop installer handoff dir: %w", err)
	}
	if err := os.WriteFile(resolveDesktopInstallerHandoffPath(), raw, 0o644); err != nil {
		return fmt.Errorf("write desktop installer handoff: %w", err)
	}
	return nil
}

func deleteDesktopInstallerHandoff() error {
	err := os.Remove(resolveDesktopInstallerHandoffPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
