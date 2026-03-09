package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

const desktopUpdateDownloadsDirname = "updates"
const desktopPendingUpdateFilename = "desktop-update-pending.json"
const desktopInstallerHandoffFilename = "desktop-update-handoff.json"

var (
	desktopUpdateHTTPClient   = &http.Client{Timeout: 45 * time.Second}
	desktopUpdateOpenArtifact = openDesktopUpdateArtifact
)

type desktopUpdateManifest struct {
	Version     string                              `json:"version,omitempty"`
	Channel     string                              `json:"channel,omitempty"`
	PublishedAt string                              `json:"publishedAt,omitempty"`
	Platforms   map[string]desktopUpdateManifestRef `json:"platforms,omitempty"`
}

type desktopUpdateManifestRef struct {
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Name   string `json:"name,omitempty"`
}

type gatewayDesktopPendingUpdate struct {
	InstallKind    string `json:"installKind,omitempty"`
	CurrentPath    string `json:"currentPath,omitempty"`
	BackupPath     string `json:"backupPath,omitempty"`
	DownloadedFile string `json:"downloadedFile,omitempty"`
	FromVersion    string `json:"fromVersion,omitempty"`
	ToVersion      string `json:"toVersion,omitempty"`
	AppliedAt      string `json:"appliedAt,omitempty"`
}

type gatewayDesktopInstallerHandoff struct {
	InstallKind  string `json:"installKind,omitempty"`
	ArtifactPath string `json:"artifactPath,omitempty"`
	ArtifactName string `json:"artifactName,omitempty"`
	FromVersion  string `json:"fromVersion,omitempty"`
	ToVersion    string `json:"toVersion,omitempty"`
	LaunchedAt   string `json:"launchedAt,omitempty"`
}

func handleDesktopUpdateDownload(ctx *MethodHandlerContext) {
	cfg, state, lastSeenVersion, err := performDesktopUpdateCheck(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "desktop update check failed: "+err.Error()))
		return
	}

	if state.UpdateManager == "system" {
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "managed-by-system"), nil)
		return
	}
	if state.UpdateManager == "package-manager" {
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "package-manager-required"), nil)
		return
	}
	if state.UpdateManager == "source" {
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "source-update-required"), nil)
		return
	}
	if !hasCandidateUpdate(state.CurrentVersion, state.CandidateVersion) {
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "no-update"), nil)
		return
	}
	if state.ReadyToInstall && strings.TrimSpace(state.DownloadedFile) != "" {
		if _, err := os.Stat(state.DownloadedFile); err == nil {
			ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "already-downloaded"), nil)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	progress := 0.0
	state.State = desktopUpdateStageDownloading
	state.Progress = &progress
	state.ReadyToInstall = false
	state.LastError = ""
	state.UpdatedAt = now
	if _, err := writeDesktopUpdateStateFile(state); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write desktop update state: "+err.Error()))
		return
	}

	downloadedFile, downloadedBytes, totalBytes, err := downloadDesktopUpdateArtifact(state)
	if err != nil {
		state.State = desktopUpdateStageFailed
		state.LastError = err.Error()
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_, _ = writeDesktopUpdateStateFile(state)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "desktop update download failed: "+err.Error()))
		return
	}

	state.DownloadedFile = downloadedFile
	state.DownloadedBytes = &downloadedBytes
	if totalBytes > 0 {
		state.TotalBytes = &totalBytes
	} else {
		state.TotalBytes = &downloadedBytes
	}
	progress = 1
	state.Progress = &progress
	state.ReadyToInstall = true
	state.State = desktopUpdateStageReadyToInstall
	state.LastError = ""
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if _, err := writeDesktopUpdateStateFile(state); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to persist downloaded desktop update: "+err.Error()))
		return
	}
	if err := persistGatewayUpdateMetadata(ctx.Context, cfg, state.LastCheckedAt, lastSeenVersion); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
		return
	}
	ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "downloaded"), nil)
}

func handleDesktopUpdateApply(ctx *MethodHandlerContext) {
	state, err := loadDesktopUpdateStateWithDefaults(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read desktop update state: "+err.Error()))
		return
	}

	switch state.UpdateManager {
	case "system":
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "managed-by-system"), nil)
		return
	case "package-manager":
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "package-manager-required"), nil)
		return
	case "source":
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "source-update-required"), nil)
		return
	}

	if !state.ReadyToInstall || strings.TrimSpace(state.DownloadedFile) == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "desktop update is not ready to install"))
		return
	}
	if _, err := os.Stat(state.DownloadedFile); err != nil {
		state.State = desktopUpdateStageFailed
		state.LastError = "downloaded desktop update artifact missing"
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_, _ = writeDesktopUpdateStateFile(state)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, state.LastError))
		return
	}

	action := "manual-install-required"
	switch state.InstallKind {
	case desktopInstallKindWindowsNSIS, desktopInstallKindMacOSWails:
		action, err = applyDesktopInstallerHandoff(&state)
		if err != nil {
			state.State = desktopUpdateStageFailed
			state.LastError = err.Error()
			state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_, _ = writeDesktopUpdateStateFile(state)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to start installer handoff: "+err.Error()))
			return
		}
	case desktopInstallKindLinuxAppImage:
		action, err = applyLinuxAppImageUpdate(&state)
		if err != nil {
			state.State = desktopUpdateStageFailed
			state.LastError = err.Error()
			state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_, _ = writeDesktopUpdateStateFile(state)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to apply AppImage update: "+err.Error()))
			return
		}
	}

	state.LastError = ""
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if _, err := writeDesktopUpdateStateFile(state); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to persist desktop update state: "+err.Error()))
		return
	}
	ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, action), nil)
}

func handleDesktopUpdateRollback(ctx *MethodHandlerContext) {
	state, err := loadDesktopUpdateStateWithDefaults(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read desktop update state: "+err.Error()))
		return
	}

	switch state.UpdateManager {
	case "system":
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "managed-by-system"), nil)
		return
	case "package-manager":
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "package-manager-required"), nil)
		return
	case "source":
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "source-update-required"), nil)
		return
	}

	if state.InstallKind != desktopInstallKindLinuxAppImage {
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "rollback-not-supported"), nil)
		return
	}

	action, err := rollbackLinuxAppImageUpdate(&state)
	if err != nil {
		state.State = desktopUpdateStageFailed
		state.LastError = err.Error()
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_, _ = writeDesktopUpdateStateFile(state)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to rollback AppImage update: "+err.Error()))
		return
	}

	state.LastError = ""
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if _, err := writeDesktopUpdateStateFile(state); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to persist desktop rollback state: "+err.Error()))
		return
	}
	ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, action), nil)
}

func handleDesktopUpdateDismiss(ctx *MethodHandlerContext) {
	cfg, err := loadGatewayUpdateConfig(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}
	state, err := loadDesktopUpdateStateWithDefaults(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read desktop update state: "+err.Error()))
		return
	}

	version := strings.TrimSpace(state.CandidateVersion)
	if version == "" {
		ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "nothing-to-dismiss"), nil)
		return
	}

	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	if cfg.Update == nil {
		cfg.Update = &types.OpenAcosmiUpdateConfig{}
	}
	if !containsString(cfg.Update.SkippedVersions, version) {
		cfg.Update.SkippedVersions = append(cfg.Update.SkippedVersions, version)
	}

	clearDesktopUpdateCandidate(&state)
	state.LastError = ""
	state.State = desktopUpdateStageIdle
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if _, err := writeDesktopUpdateStateFile(state); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write desktop update state: "+err.Error()))
		return
	}
	if err := persistGatewayUpdateMetadata(ctx.Context, cfg, state.LastCheckedAt, version); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
		return
	}
	ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "dismissed"), nil)
}

func performDesktopUpdateCheck(ctx *GatewayMethodContext) (*types.OpenAcosmiConfig, gatewayDesktopUpdateState, string, error) {
	cfg, err := loadGatewayUpdateConfig(ctx)
	if err != nil {
		return nil, gatewayDesktopUpdateState{}, "", err
	}
	state, err := loadDesktopUpdateStateWithDefaults(ctx)
	if err != nil {
		return nil, gatewayDesktopUpdateState{}, "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state.CurrentVersion = resolveCurrentVersion()
	state.InstallKind = detectGatewayDesktopInstallKind()
	state.UpdateManager = resolveGatewayDesktopUpdateManager(state.InstallKind)
	state.ManagedBySystem = isSystemManagedDesktopInstall(state.InstallKind)
	if state.Channel == "" {
		state.Channel = resolveDesktopUpdateChannel(cfg)
	}
	state.LastCheckedAt = now
	state.UpdatedAt = now

	lastSeenVersion := strings.TrimSpace(state.CandidateVersion)

	switch state.UpdateManager {
	case "source":
		state.State = desktopUpdateStageIdle
		state.LastError = ""
		clearDesktopUpdateCandidate(&state)
	case "system":
		state.State = desktopUpdateStageManagedBySystem
		state.LastError = ""
		clearDesktopUpdateCandidate(&state)
	case "package-manager":
		state.State = desktopUpdateStageIdle
		state.LastError = ""
		clearDesktopUpdateCandidate(&state)
	default:
		state.State = desktopUpdateStageChecking
		state.LastError = ""
		manifestURL, manifest, asset, err := fetchDesktopUpdateManifest(cfg, state.Channel, state.InstallKind)
		if err != nil {
			state.State = desktopUpdateStageFailed
			state.LastError = err.Error()
			_, _ = writeDesktopUpdateStateFile(state)
			_ = persistGatewayUpdateMetadata(ctx, cfg, state.LastCheckedAt, lastSeenVersion)
			return cfg, state, lastSeenVersion, err
		}

		remoteVersion := strings.TrimSpace(manifest.Version)
		if remoteVersion == "" {
			state.State = desktopUpdateStageFailed
			state.LastError = "desktop update manifest missing version"
			_, _ = writeDesktopUpdateStateFile(state)
			_ = persistGatewayUpdateMetadata(ctx, cfg, state.LastCheckedAt, lastSeenVersion)
			return cfg, state, lastSeenVersion, fmt.Errorf("%s", state.LastError)
		}
		lastSeenVersion = remoteVersion
		state.ManifestURL = manifestURL
		state.AssetURL = strings.TrimSpace(asset.URL)
		state.AssetSHA256 = strings.ToLower(strings.TrimSpace(asset.SHA256))
		state.AssetName = resolveDesktopUpdateAssetName(asset)
		if asset.Size > 0 {
			size := asset.Size
			state.TotalBytes = &size
		} else {
			state.TotalBytes = nil
		}
		state.PublishedAt = strings.TrimSpace(manifest.PublishedAt)

		if !hasCandidateUpdate(state.CurrentVersion, remoteVersion) || isSkippedDesktopUpdateVersion(cfg, remoteVersion) {
			clearDesktopUpdateCandidate(&state)
			state.State = desktopUpdateStageIdle
		} else {
			if state.CandidateVersion != remoteVersion {
				clearDesktopUpdateDownload(&state)
			}
			state.CandidateVersion = remoteVersion
			if state.ReadyToInstall && strings.TrimSpace(state.DownloadedFile) != "" {
				state.State = desktopUpdateStageReadyToInstall
			} else {
				state.State = desktopUpdateStageAvailable
			}
		}
	}

	if _, err := writeDesktopUpdateStateFile(state); err != nil {
		return cfg, state, lastSeenVersion, err
	}
	if err := persistGatewayUpdateMetadata(ctx, cfg, state.LastCheckedAt, lastSeenVersion); err != nil {
		return cfg, state, lastSeenVersion, err
	}
	return cfg, state, lastSeenVersion, nil
}

func fetchDesktopUpdateManifest(
	cfg *types.OpenAcosmiConfig,
	channel string,
	installKind string,
) (string, desktopUpdateManifest, desktopUpdateManifestRef, error) {
	manifestURL, err := resolveDesktopUpdateManifestURL(cfg, channel)
	if err != nil {
		return "", desktopUpdateManifest{}, desktopUpdateManifestRef{}, err
	}
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return "", desktopUpdateManifest{}, desktopUpdateManifestRef{}, err
	}
	resp, err := desktopUpdateHTTPClient.Do(req)
	if err != nil {
		return "", desktopUpdateManifest{}, desktopUpdateManifestRef{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", desktopUpdateManifest{}, desktopUpdateManifestRef{}, fmt.Errorf("desktop update manifest request failed with status %d", resp.StatusCode)
	}

	var manifest desktopUpdateManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", desktopUpdateManifest{}, desktopUpdateManifestRef{}, fmt.Errorf("decode desktop update manifest: %w", err)
	}
	asset, err := resolveDesktopUpdateManifestAsset(manifest, manifestURL, installKind, desktopUpdateGOARCH)
	if err != nil {
		return "", desktopUpdateManifest{}, desktopUpdateManifestRef{}, err
	}
	return manifestURL, manifest, asset, nil
}

func resolveDesktopUpdateManifestURL(cfg *types.OpenAcosmiConfig, channel string) (string, error) {
	if v := strings.TrimSpace(preferredGatewayEnvValue("CRABCLAW_UPDATE_MANIFEST_URL", "OPENACOSMI_UPDATE_MANIFEST_URL")); v != "" {
		return v, nil
	}

	sourceURL := strings.TrimSpace(preferredGatewayEnvValue("CRABCLAW_UPDATE_SOURCE_URL", "OPENACOSMI_UPDATE_SOURCE_URL"))
	if sourceURL == "" && cfg != nil && cfg.Update != nil {
		sourceURL = strings.TrimSpace(cfg.Update.SourceURL)
	}
	if sourceURL == "" {
		return "", fmt.Errorf("desktop update source not configured")
	}
	if strings.HasSuffix(strings.ToLower(sourceURL), ".json") {
		return sourceURL, nil
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "stable"
	}
	return strings.TrimRight(sourceURL, "/") + "/" + channel + "/update.json", nil
}

func resolveDesktopUpdateManifestAsset(
	manifest desktopUpdateManifest,
	manifestURL string,
	installKind string,
	arch string,
) (desktopUpdateManifestRef, error) {
	if len(manifest.Platforms) == 0 {
		return desktopUpdateManifestRef{}, fmt.Errorf("desktop update manifest has no platforms")
	}
	for _, key := range desktopUpdatePlatformKeys(installKind, arch) {
		if asset, ok := manifest.Platforms[key]; ok {
			asset.URL = resolveDesktopUpdateAssetURL(manifestURL, asset.URL)
			return asset, nil
		}
	}
	return desktopUpdateManifestRef{}, fmt.Errorf("desktop update manifest missing platform entry for %s/%s", installKind, arch)
}

func desktopUpdatePlatformKeys(installKind string, arch string) []string {
	kind := strings.TrimSpace(installKind)
	arch = strings.TrimSpace(strings.ToLower(arch))
	if arch == "" {
		arch = "amd64"
	}
	return []string{
		fmt.Sprintf("%s-%s", kind, arch),
		kind,
	}
}

func resolveDesktopUpdateAssetURL(manifestURL string, assetURL string) string {
	assetURL = strings.TrimSpace(assetURL)
	if assetURL == "" {
		return ""
	}
	parsed, err := neturl.Parse(assetURL)
	if err == nil && parsed.IsAbs() {
		return assetURL
	}
	base, err := neturl.Parse(manifestURL)
	if err != nil {
		return assetURL
	}
	ref, err := neturl.Parse(assetURL)
	if err != nil {
		return assetURL
	}
	return base.ResolveReference(ref).String()
}

func resolveDesktopUpdateAssetName(asset desktopUpdateManifestRef) string {
	if name := strings.TrimSpace(asset.Name); name != "" {
		return name
	}
	parsed, err := neturl.Parse(strings.TrimSpace(asset.URL))
	if err == nil && parsed.Path != "" {
		if base := path.Base(parsed.Path); base != "." && base != "/" {
			return base
		}
	}
	return "update.bin"
}

func downloadDesktopUpdateArtifact(state gatewayDesktopUpdateState) (string, int64, int64, error) {
	assetURL := strings.TrimSpace(state.AssetURL)
	if assetURL == "" {
		return "", 0, 0, fmt.Errorf("desktop update asset URL missing")
	}
	req, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return "", 0, 0, err
	}
	resp, err := desktopUpdateHTTPClient.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, 0, fmt.Errorf("desktop update asset request failed with status %d", resp.StatusCode)
	}

	destPath := resolveDesktopUpdateDownloadPath(state.CandidateVersion, state.AssetName)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", 0, 0, err
	}
	tmpPath := destPath + ".partial"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, 0, err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)
	written, copyErr := io.Copy(writer, resp.Body)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", 0, 0, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", 0, 0, closeErr
	}

	expectedSHA := strings.ToLower(strings.TrimSpace(state.AssetSHA256))
	if expectedSHA != "" {
		actualSHA := hex.EncodeToString(hasher.Sum(nil))
		if actualSHA != expectedSHA {
			_ = os.Remove(tmpPath)
			return "", 0, 0, fmt.Errorf("desktop update checksum mismatch: expected %s got %s", expectedSHA, actualSHA)
		}
	}

	if state.InstallKind == desktopInstallKindLinuxAppImage {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			_ = os.Remove(tmpPath)
			return "", 0, 0, err
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, 0, err
	}
	return destPath, written, resp.ContentLength, nil
}

func resolveDesktopUpdateDownloadPath(version string, assetName string) string {
	version = sanitizeDesktopUpdatePathPart(version)
	if version == "" {
		version = "unknown"
	}
	assetName = sanitizeDesktopUpdateAssetName(assetName)
	return filepath.Join(filepath.Dir(resolveDesktopUpdateStatePath()), desktopUpdateDownloadsDirname, version, assetName)
}

func applyLinuxAppImageUpdate(state *gatewayDesktopUpdateState) (string, error) {
	currentPath, err := resolveGatewayCurrentAppImagePath()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state.DownloadedFile) == "" {
		return "", fmt.Errorf("downloaded AppImage not found")
	}

	backupPath := resolveDesktopUpdateBackupPath(currentPath, state.CurrentVersion)
	if err := copyDesktopFile(currentPath, backupPath, 0o755); err != nil {
		return "", fmt.Errorf("backup current AppImage: %w", err)
	}

	tmpPath := currentPath + ".update-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".tmp"
	if err := copyDesktopFile(state.DownloadedFile, tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("stage downloaded AppImage: %w", err)
	}

	pending := gatewayDesktopPendingUpdate{
		InstallKind:    state.InstallKind,
		CurrentPath:    currentPath,
		BackupPath:     backupPath,
		DownloadedFile: state.DownloadedFile,
		FromVersion:    state.CurrentVersion,
		ToVersion:      state.CandidateVersion,
		AppliedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeDesktopPendingUpdateFile(pending); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, currentPath); err != nil {
		_ = os.Remove(tmpPath)
		_ = deleteDesktopPendingUpdateFile()
		return "", fmt.Errorf("replace current AppImage: %w", err)
	}

	state.RollbackAvailable = true
	state.RollbackVersion = strings.TrimSpace(state.CurrentVersion)
	state.RollbackBackupPath = backupPath
	state.State = desktopUpdateStageApplying
	state.ReadyToInstall = false
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return "appimage-applied-restart-required", nil
}

func applyDesktopInstallerHandoff(state *gatewayDesktopUpdateState) (string, error) {
	artifactPath := strings.TrimSpace(state.DownloadedFile)
	if artifactPath == "" {
		return "", fmt.Errorf("downloaded installer artifact missing")
	}
	if _, err := os.Stat(artifactPath); err != nil {
		return "", fmt.Errorf("downloaded installer artifact missing: %w", err)
	}
	if err := validateDesktopInstallerArtifact(state.InstallKind, artifactPath); err != nil {
		return "", err
	}

	handoff := gatewayDesktopInstallerHandoff{
		InstallKind:  state.InstallKind,
		ArtifactPath: artifactPath,
		ArtifactName: filepath.Base(artifactPath),
		FromVersion:  strings.TrimSpace(state.CurrentVersion),
		ToVersion:    strings.TrimSpace(state.CandidateVersion),
		LaunchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeDesktopInstallerHandoffFile(handoff); err != nil {
		return "", err
	}
	if err := desktopUpdateOpenArtifact(state.InstallKind, artifactPath); err != nil {
		_ = deleteDesktopInstallerHandoffFile()
		return "", err
	}

	state.State = desktopUpdateStageApplying
	state.ReadyToInstall = false
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return "installer-launched-restart-required", nil
}

func validateDesktopInstallerArtifact(installKind string, artifactPath string) error {
	lowerPath := strings.ToLower(strings.TrimSpace(artifactPath))
	switch installKind {
	case desktopInstallKindWindowsNSIS:
		if strings.HasSuffix(lowerPath, ".exe") || strings.HasSuffix(lowerPath, ".msi") {
			return nil
		}
		return fmt.Errorf("windows installer handoff requires .exe or .msi artifact")
	case desktopInstallKindMacOSWails:
		if strings.HasSuffix(lowerPath, ".app") ||
			strings.HasSuffix(lowerPath, ".pkg") ||
			strings.HasSuffix(lowerPath, ".dmg") ||
			strings.HasSuffix(lowerPath, ".zip") {
			return nil
		}
		return fmt.Errorf("macOS installer handoff requires .app, .pkg, .dmg, or .zip artifact")
	default:
		return fmt.Errorf("installer handoff is not supported for %s", installKind)
	}
}

func rollbackLinuxAppImageUpdate(state *gatewayDesktopUpdateState) (string, error) {
	target, err := resolveLinuxAppImageRollbackTarget(*state)
	if err != nil {
		return "", err
	}

	tmpPath := target.currentPath + ".rollback-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".tmp"
	if err := copyDesktopFile(target.backupPath, tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("stage AppImage rollback: %w", err)
	}
	if err := os.Rename(tmpPath, target.currentPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("restore previous AppImage: %w", err)
	}
	if err := deleteDesktopPendingUpdateFile(); err != nil {
		return "", err
	}

	clearDesktopUpdateCandidate(state)
	clearDesktopUpdateRollback(state)
	state.State = desktopUpdateStageRolledBack
	state.ReadyToInstall = false

	if target.rollbackVersion != "" && strings.TrimSpace(state.CurrentVersion) == target.rollbackVersion {
		state.CurrentVersion = target.rollbackVersion
		return "rollback-completed", nil
	}
	return "rollback-applied-restart-required", nil
}

type linuxAppImageRollbackTarget struct {
	currentPath     string
	backupPath      string
	rollbackVersion string
}

func resolveLinuxAppImageRollbackTarget(state gatewayDesktopUpdateState) (linuxAppImageRollbackTarget, error) {
	if pending, err := readDesktopPendingUpdateFile(); err != nil {
		return linuxAppImageRollbackTarget{}, err
	} else if pending != nil &&
		strings.TrimSpace(pending.InstallKind) == desktopInstallKindLinuxAppImage &&
		strings.TrimSpace(pending.BackupPath) != "" {
		currentPath := strings.TrimSpace(pending.CurrentPath)
		if currentPath == "" {
			currentPath, err = resolveGatewayCurrentAppImagePath()
			if err != nil {
				return linuxAppImageRollbackTarget{}, err
			}
		}
		if _, err := os.Stat(pending.BackupPath); err != nil {
			return linuxAppImageRollbackTarget{}, fmt.Errorf("AppImage rollback backup missing: %w", err)
		}
		return linuxAppImageRollbackTarget{
			currentPath:     currentPath,
			backupPath:      strings.TrimSpace(pending.BackupPath),
			rollbackVersion: strings.TrimSpace(pending.FromVersion),
		}, nil
	}

	backupPath := strings.TrimSpace(state.RollbackBackupPath)
	if !state.RollbackAvailable || backupPath == "" {
		return linuxAppImageRollbackTarget{}, fmt.Errorf("no AppImage rollback target available")
	}
	if _, err := os.Stat(backupPath); err != nil {
		return linuxAppImageRollbackTarget{}, fmt.Errorf("AppImage rollback backup missing: %w", err)
	}
	currentPath, err := resolveGatewayCurrentAppImagePath()
	if err != nil {
		return linuxAppImageRollbackTarget{}, err
	}
	return linuxAppImageRollbackTarget{
		currentPath:     currentPath,
		backupPath:      backupPath,
		rollbackVersion: strings.TrimSpace(state.RollbackVersion),
	}, nil
}

func resolveGatewayCurrentAppImagePath() (string, error) {
	if appImagePath := strings.TrimSpace(desktopUpdateGetenv("APPIMAGE")); appImagePath != "" {
		return appImagePath, nil
	}
	return desktopUpdateExecutable()
}

func resolveDesktopUpdateBackupPath(currentPath string, currentVersion string) string {
	name := sanitizeDesktopUpdateAssetName(filepath.Base(currentPath))
	version := sanitizeDesktopUpdatePathPart(currentVersion)
	if version == "" {
		version = "unknown"
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	return filepath.Join(
		filepath.Dir(resolveDesktopUpdateStatePath()),
		desktopUpdateDownloadsDirname,
		"backups",
		version+"-"+stamp+"-"+name,
	)
}

func copyDesktopFile(srcPath string, destPath string, mode os.FileMode) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}

	copyErr := func() error {
		_, err := io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}
		return destFile.Close()
	}()
	if copyErr != nil {
		_ = destFile.Close()
		_ = os.Remove(destPath)
		return copyErr
	}
	if mode != 0 {
		if err := os.Chmod(destPath, mode); err != nil {
			_ = os.Remove(destPath)
			return err
		}
	}
	return nil
}

func resolveDesktopPendingUpdatePath() string {
	return filepath.Join(filepath.Dir(resolveDesktopUpdateStatePath()), desktopPendingUpdateFilename)
}

func resolveDesktopInstallerHandoffPath() string {
	return filepath.Join(filepath.Dir(resolveDesktopUpdateStatePath()), desktopInstallerHandoffFilename)
}

func writeDesktopPendingUpdateFile(pending gatewayDesktopPendingUpdate) error {
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

func writeDesktopInstallerHandoffFile(handoff gatewayDesktopInstallerHandoff) error {
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

func readDesktopPendingUpdateFile() (*gatewayDesktopPendingUpdate, error) {
	raw, err := os.ReadFile(resolveDesktopPendingUpdatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pending gatewayDesktopPendingUpdate
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("decode pending desktop update: %w", err)
	}
	return &pending, nil
}

func readDesktopInstallerHandoffFile() (*gatewayDesktopInstallerHandoff, error) {
	raw, err := os.ReadFile(resolveDesktopInstallerHandoffPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var handoff gatewayDesktopInstallerHandoff
	if err := json.Unmarshal(raw, &handoff); err != nil {
		return nil, fmt.Errorf("decode desktop installer handoff: %w", err)
	}
	return &handoff, nil
}

func deleteDesktopPendingUpdateFile() error {
	err := os.Remove(resolveDesktopPendingUpdatePath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func deleteDesktopInstallerHandoffFile() error {
	err := os.Remove(resolveDesktopInstallerHandoffPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func sanitizeDesktopUpdatePathPart(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "..", "-")
	value = replacer.Replace(value)
	return strings.Trim(value, ". ")
}

func sanitizeDesktopUpdateAssetName(assetName string) string {
	assetName = path.Base(strings.TrimSpace(assetName))
	if assetName == "." || assetName == "/" || assetName == "" {
		return "update.bin"
	}
	return sanitizeDesktopUpdatePathPart(assetName)
}

func openDesktopUpdateArtifact(installKind string, artifactPath string) error {
	var cmd *exec.Cmd
	switch desktopUpdateGOOS {
	case "darwin":
		cmd = exec.Command("open", artifactPath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", artifactPath)
	default:
		if installKind == desktopInstallKindLinuxAppImage {
			return nil
		}
		cmd = exec.Command("xdg-open", artifactPath)
	}
	return cmd.Start()
}

func buildDesktopUpdateStatusWithAction(state gatewayDesktopUpdateState, action string) gatewayDesktopUpdateStatus {
	status := buildDesktopUpdateStatus(state)
	status.Action = action
	return status
}

func desktopUpdateStateFromStatus(status gatewayDesktopUpdateStatus) gatewayDesktopUpdateState {
	return gatewayDesktopUpdateState{
		CurrentVersion:    status.CurrentVersion,
		CandidateVersion:  status.CandidateVersion,
		Channel:           status.Channel,
		InstallKind:       status.InstallKind,
		UpdateManager:     status.UpdateManager,
		ManagedBySystem:   status.ManagedBySystem,
		State:             status.State,
		Progress:          status.Progress,
		DownloadedBytes:   status.DownloadedBytes,
		TotalBytes:        status.TotalBytes,
		PublishedAt:       status.PublishedAt,
		ReadyToInstall:    status.ReadyToInstall,
		RollbackAvailable: status.RollbackAvailable,
		RollbackVersion:   status.RollbackVersion,
		LastCheckedAt:     status.LastCheckedAt,
		LastError:         status.LastError,
		UpdatedAt:         status.UpdatedAt,
	}
}

func loadDesktopUpdateStateWithDefaults(ctx *GatewayMethodContext) (gatewayDesktopUpdateState, error) {
	cfg, err := loadGatewayUpdateConfig(ctx)
	if err != nil {
		return gatewayDesktopUpdateState{}, err
	}
	state, err := readDesktopUpdateStateFile()
	if err != nil {
		return gatewayDesktopUpdateState{}, err
	}
	if state == nil {
		state = &gatewayDesktopUpdateState{}
	}

	state.CurrentVersion = resolveCurrentVersion()
	state.InstallKind = detectGatewayDesktopInstallKind()
	state.UpdateManager = resolveGatewayDesktopUpdateManager(state.InstallKind)
	state.ManagedBySystem = isSystemManagedDesktopInstall(state.InstallKind)
	if state.Channel == "" {
		state.Channel = resolveDesktopUpdateChannel(cfg)
	}
	if state.LastCheckedAt == "" && cfg != nil && cfg.Update != nil {
		state.LastCheckedAt = strings.TrimSpace(cfg.Update.LastCheckedAt)
	}
	if state.ManagedBySystem {
		state.State = desktopUpdateStageManagedBySystem
	} else if state.State == "" {
		switch {
		case state.ReadyToInstall:
			state.State = desktopUpdateStageReadyToInstall
		case hasCandidateUpdate(state.CurrentVersion, state.CandidateVersion):
			state.State = desktopUpdateStageAvailable
		default:
			state.State = desktopUpdateStageIdle
		}
	}
	return *state, nil
}

func clearDesktopUpdateCandidate(state *gatewayDesktopUpdateState) {
	state.CandidateVersion = ""
	state.ManifestURL = ""
	state.AssetURL = ""
	state.AssetName = ""
	state.AssetSHA256 = ""
	state.PublishedAt = ""
	clearDesktopUpdateDownload(state)
}

func clearDesktopUpdateDownload(state *gatewayDesktopUpdateState) {
	state.DownloadedFile = ""
	state.ReadyToInstall = false
	state.Progress = nil
	state.DownloadedBytes = nil
	state.TotalBytes = nil
}

func clearDesktopUpdateRollback(state *gatewayDesktopUpdateState) {
	state.RollbackAvailable = false
	state.RollbackVersion = ""
	state.RollbackBackupPath = ""
}

func isSkippedDesktopUpdateVersion(cfg *types.OpenAcosmiConfig, version string) bool {
	if cfg == nil || cfg.Update == nil {
		return false
	}
	return containsString(cfg.Update.SkippedVersions, version)
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}
