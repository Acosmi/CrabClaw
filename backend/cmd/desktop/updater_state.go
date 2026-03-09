package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

type desktopUpdateStage string

const (
	desktopUpdateStageIdle            desktopUpdateStage = "idle"
	desktopUpdateStageChecking        desktopUpdateStage = "checking"
	desktopUpdateStageAvailable       desktopUpdateStage = "available"
	desktopUpdateStageDownloading     desktopUpdateStage = "downloading"
	desktopUpdateStageReadyToInstall  desktopUpdateStage = "ready-to-install"
	desktopUpdateStageManagedBySystem desktopUpdateStage = "managed-by-system"
	desktopUpdateStageApplying        desktopUpdateStage = "applying"
	desktopUpdateStageRolledBack      desktopUpdateStage = "rolled-back"
	desktopUpdateStageFailed          desktopUpdateStage = "failed"
)

// DesktopUpdateState 是桌面宿主独立维护的更新状态。
type DesktopUpdateState struct {
	CurrentVersion     string             `json:"currentVersion,omitempty"`
	CandidateVersion   string             `json:"candidateVersion,omitempty"`
	Channel            string             `json:"channel,omitempty"`
	InstallKind        desktopInstallKind `json:"installKind,omitempty"`
	UpdateManager      string             `json:"updateManager,omitempty"`
	ManagedBySystem    bool               `json:"managedBySystem,omitempty"`
	State              desktopUpdateStage `json:"state,omitempty"`
	Progress           *float64           `json:"progress,omitempty"`
	DownloadedBytes    *int64             `json:"downloadedBytes,omitempty"`
	TotalBytes         *int64             `json:"totalBytes,omitempty"`
	ManifestURL        string             `json:"manifestURL,omitempty"`
	AssetURL           string             `json:"assetURL,omitempty"`
	AssetName          string             `json:"assetName,omitempty"`
	AssetSHA256        string             `json:"assetSHA256,omitempty"`
	DownloadedFile     string             `json:"downloadedFile,omitempty"`
	PublishedAt        string             `json:"publishedAt,omitempty"`
	ReadyToInstall     bool               `json:"readyToInstall,omitempty"`
	RollbackAvailable  bool               `json:"rollbackAvailable,omitempty"`
	RollbackVersion    string             `json:"rollbackVersion,omitempty"`
	RollbackBackupPath string             `json:"rollbackBackupPath,omitempty"`
	LastCheckedAt      string             `json:"lastCheckedAt,omitempty"`
	LastError          string             `json:"lastError,omitempty"`
	UpdatedAt          string             `json:"updatedAt,omitempty"`
}

const desktopUpdateStateFilename = "desktop-update-state.json"

func resolveDesktopUpdateStatePath() string {
	return filepath.Join(config.ResolveStateDir(), desktopUpdateStateFilename)
}

func readDesktopUpdateState() (*DesktopUpdateState, error) {
	return readDesktopUpdateStateAt(resolveDesktopUpdateStatePath())
}

func writeDesktopUpdateState(state DesktopUpdateState) (string, error) {
	return writeDesktopUpdateStateAt(resolveDesktopUpdateStatePath(), state)
}

func deleteDesktopUpdateState() error {
	path := resolveDesktopUpdateStatePath()
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func readDesktopUpdateStateAt(path string) (*DesktopUpdateState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state DesktopUpdateState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decode desktop update state: %w", err)
	}
	return &state, nil
}

func writeDesktopUpdateStateAt(path string, state DesktopUpdateState) (string, error) {
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if state.State == "" {
		state.State = desktopUpdateStageIdle
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create desktop update state dir: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode desktop update state: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", fmt.Errorf("write desktop update state: %w", err)
	}
	return path, nil
}

func syncDesktopUpdateStateFromConfig(cfg *types.OpenAcosmiConfig) error {
	existing, err := readDesktopUpdateState()
	if err != nil {
		return err
	}

	state := DesktopUpdateState{}
	if existing != nil {
		state = *existing
	}

	installKind := detectDesktopInstallKind()
	managedBySystem := installKind == installKindWindowsMSIX
	state.CurrentVersion = resolveDesktopCurrentVersion()
	state.InstallKind = installKind
	state.UpdateManager = resolveDesktopUpdateManager(string(installKind))
	state.ManagedBySystem = managedBySystem

	if state.Channel == "" {
		state.Channel = resolveDesktopUpdateChannel(cfg)
	}
	if state.LastCheckedAt == "" && cfg != nil && cfg.Update != nil {
		state.LastCheckedAt = strings.TrimSpace(cfg.Update.LastCheckedAt)
	}

	if managedBySystem {
		state.State = desktopUpdateStageManagedBySystem
	} else if state.State == "" {
		state.State = desktopUpdateStageIdle
	}

	_, err = writeDesktopUpdateState(state)
	return err
}

func resolveDesktopUpdateChannel(cfg *types.OpenAcosmiConfig) string {
	if cfg != nil && cfg.Update != nil {
		channel := strings.TrimSpace(cfg.Update.Channel)
		if channel != "" {
			return channel
		}
	}
	return "stable"
}

func resolveDesktopCurrentVersion() string {
	if v := strings.TrimSpace(version); v != "" {
		return v
	}
	return "dev"
}

func resolveDesktopUpdateManager(installKind string) string {
	switch strings.TrimSpace(installKind) {
	case string(installKindSource):
		return "source"
	case string(installKindWindowsMSIX):
		return "system"
	case string(installKindWindowsNSIS):
		return "installer"
	case string(installKindLinuxSystemPackage):
		return "package-manager"
	case string(installKindLinuxAppImage):
		return "appimage"
	case string(installKindMacOSWails):
		return "host"
	default:
		return "unknown"
	}
}

func clearDesktopRollback(state *DesktopUpdateState) {
	state.RollbackAvailable = false
	state.RollbackVersion = ""
	state.RollbackBackupPath = ""
}
