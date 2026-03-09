package gateway

// server_methods_update.go — update.* / desktop.update.* 方法处理器
// 对应 TS: src/gateway/server-methods/update.ts (132L)
//
// 方法列表 (8): update.check, update.run, desktop.update.status, desktop.update.check,
// desktop.update.download, desktop.update.apply, desktop.update.rollback, desktop.update.dismiss
//
// TS 的 update.run 执行 `npm update`、写重启哨兵、定时重启。
// Go 版本自更新替代为环境模式判断 + 哨兵文件 + 进程信号。

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

const (
	desktopUpdateStateFilename = "desktop-update-state.json"

	desktopInstallKindUnknown            = "unknown"
	desktopInstallKindSource             = "source"
	desktopInstallKindMacOSWails         = "macos-wails"
	desktopInstallKindWindowsMSIX        = "windows-msix"
	desktopInstallKindWindowsNSIS        = "windows-nsis"
	desktopInstallKindLinuxAppImage      = "linux-appimage"
	desktopInstallKindLinuxSystemPackage = "linux-system-package"

	desktopUpdateStageIdle            = "idle"
	desktopUpdateStageChecking        = "checking"
	desktopUpdateStageAvailable       = "available"
	desktopUpdateStageDownloading     = "downloading"
	desktopUpdateStageReadyToInstall  = "ready-to-install"
	desktopUpdateStageManagedBySystem = "managed-by-system"
	desktopUpdateStageApplying        = "applying"
	desktopUpdateStageRolledBack      = "rolled-back"
	desktopUpdateStageFailed          = "failed"
)

var (
	desktopUpdateGOOS       = runtime.GOOS
	desktopUpdateGOARCH     = runtime.GOARCH
	desktopUpdateExecutable = os.Executable
	desktopUpdateGetenv     = os.Getenv
)

type gatewayDesktopUpdateState struct {
	CurrentVersion     string   `json:"currentVersion,omitempty"`
	CandidateVersion   string   `json:"candidateVersion,omitempty"`
	Channel            string   `json:"channel,omitempty"`
	InstallKind        string   `json:"installKind,omitempty"`
	UpdateManager      string   `json:"updateManager,omitempty"`
	ManagedBySystem    bool     `json:"managedBySystem,omitempty"`
	State              string   `json:"state,omitempty"`
	Progress           *float64 `json:"progress,omitempty"`
	DownloadedBytes    *int64   `json:"downloadedBytes,omitempty"`
	TotalBytes         *int64   `json:"totalBytes,omitempty"`
	ManifestURL        string   `json:"manifestURL,omitempty"`
	AssetURL           string   `json:"assetURL,omitempty"`
	AssetName          string   `json:"assetName,omitempty"`
	AssetSHA256        string   `json:"assetSHA256,omitempty"`
	DownloadedFile     string   `json:"downloadedFile,omitempty"`
	PublishedAt        string   `json:"publishedAt,omitempty"`
	ReadyToInstall     bool     `json:"readyToInstall,omitempty"`
	RollbackAvailable  bool     `json:"rollbackAvailable,omitempty"`
	RollbackVersion    string   `json:"rollbackVersion,omitempty"`
	RollbackBackupPath string   `json:"rollbackBackupPath,omitempty"`
	LastCheckedAt      string   `json:"lastCheckedAt,omitempty"`
	LastError          string   `json:"lastError,omitempty"`
	UpdatedAt          string   `json:"updatedAt,omitempty"`
}

type gatewayDesktopUpdateStatus struct {
	CurrentVersion    string   `json:"currentVersion,omitempty"`
	CandidateVersion  string   `json:"candidateVersion,omitempty"`
	Channel           string   `json:"channel,omitempty"`
	InstallKind       string   `json:"installKind,omitempty"`
	UpdateManager     string   `json:"updateManager,omitempty"`
	ManagedBySystem   bool     `json:"managedBySystem,omitempty"`
	State             string   `json:"state,omitempty"`
	Progress          *float64 `json:"progress,omitempty"`
	DownloadedBytes   *int64   `json:"downloadedBytes,omitempty"`
	TotalBytes        *int64   `json:"totalBytes,omitempty"`
	PublishedAt       string   `json:"publishedAt,omitempty"`
	ReadyToInstall    bool     `json:"readyToInstall,omitempty"`
	RollbackAvailable bool     `json:"rollbackAvailable,omitempty"`
	RollbackVersion   string   `json:"rollbackVersion,omitempty"`
	LastCheckedAt     string   `json:"lastCheckedAt,omitempty"`
	LastError         string   `json:"lastError,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
	UpdateAvailable   bool     `json:"updateAvailable"`
	Action            string   `json:"action,omitempty"`
}

type desktopInstallProbe struct {
	GOOS    string
	ExePath string
	Env     map[string]string
}

// UpdateHandlers 返回 update.* 方法映射。
func UpdateHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"update.check":            handleUpdateCheck,
		"update.run":              handleUpdateRun,
		"desktop.update.status":   handleDesktopUpdateStatus,
		"desktop.update.check":    handleDesktopUpdateCheck,
		"desktop.update.download": handleDesktopUpdateDownload,
		"desktop.update.apply":    handleDesktopUpdateApply,
		"desktop.update.rollback": handleDesktopUpdateRollback,
		"desktop.update.dismiss":  handleDesktopUpdateDismiss,
	}
}

// ---------- update.check ----------

func handleUpdateCheck(ctx *MethodHandlerContext) {
	installKind := detectGatewayDesktopInstallKind()
	if isPackagedDesktopInstallKind(installKind) {
		status, err := loadDesktopUpdateStatus(ctx.Context)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read desktop update status: "+err.Error()))
			return
		}
		ctx.Respond(true, map[string]interface{}{
			"currentVersion":   status.CurrentVersion,
			"candidateVersion": status.CandidateVersion,
			"channel":          status.Channel,
			"platform":         runtime.GOOS,
			"arch":             runtime.GOARCH,
			"installKind":      status.InstallKind,
			"managedBySystem":  status.ManagedBySystem,
			"state":            status.State,
			"readyToInstall":   status.ReadyToInstall,
			"lastCheckedAt":    status.LastCheckedAt,
			"updateAvailable":  status.UpdateAvailable,
		}, nil)
		return
	}

	// TS: 检查配置中的 version / channel 信息
	result := map[string]interface{}{
		"currentVersion":  resolveCurrentVersion(),
		"platform":        runtime.GOOS,
		"arch":            runtime.GOARCH,
		"installKind":     installKind,
		"updateAvailable": false,
	}

	ctx.Respond(true, result, nil)
}

// ---------- desktop.update.status ----------

func handleDesktopUpdateStatus(ctx *MethodHandlerContext) {
	status, err := loadDesktopUpdateStatus(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read desktop update status: "+err.Error()))
		return
	}
	ctx.Respond(true, status, nil)
}

// ---------- desktop.update.check ----------

func handleDesktopUpdateCheck(ctx *MethodHandlerContext) {
	_, state, _, err := performDesktopUpdateCheck(ctx.Context)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "desktop update check failed: "+err.Error()))
		return
	}
	ctx.Respond(true, buildDesktopUpdateStatusWithAction(state, "checked"), nil)
}

// ---------- update.run ----------
// TS: npm update -> 写重启哨兵 + scheduleRestartMs
// Go: go binary 无 npm update，替代为重启哨兵 + 进程信号方案

func handleUpdateRun(ctx *MethodHandlerContext) {
	installKind := detectGatewayDesktopInstallKind()

	baseDir := ""
	if ctx.Context != nil {
		baseDir = ctx.Context.PairingBaseDir
	}
	if baseDir == "" {
		baseDir = resolveDefaultStateDir()
	}

	// 仅在开发模式下支持自更新（go install / go build）
	mode, _ := ctx.Params["mode"].(string)
	if mode == "" {
		mode = "default"
	}

	if isPackagedDesktopInstallKind(installKind) && mode != "restart" {
		state, err := loadDesktopUpdateStateWithDefaults(ctx.Context)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read desktop update state: "+err.Error()))
			return
		}
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if state.ManagedBySystem {
			state.State = desktopUpdateStageManagedBySystem
		} else if state.State == "" {
			state.State = desktopUpdateStageIdle
		}
		_, _ = writeDesktopUpdateStateFile(state)

		action := "desktop-update-required"
		if state.ManagedBySystem {
			action = "managed-by-system"
		}
		ctx.Respond(true, map[string]interface{}{
			"ok":              true,
			"action":          action,
			"installKind":     installKind,
			"updateManager":   state.UpdateManager,
			"managedBySystem": state.ManagedBySystem,
			"state":           state.State,
			"currentVersion":  state.CurrentVersion,
			"nextMethod":      "desktop.update.check",
		}, nil)
		return
	}

	// 确保基目录存在
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to create state dir: "+err.Error()))
		return
	}

	// 选择更新策略
	switch mode {
	case "restart":
		// 仅写重启哨兵（进程监控端负责重启）
		sentinelPath := filepath.Join(baseDir, ".restart-sentinel")
		if err := os.WriteFile(sentinelPath, []byte(fmt.Sprintf("%d", time.Now().UnixMilli())), 0600); err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write restart sentinel: "+err.Error()))
			return
		}
		ctx.Respond(true, map[string]interface{}{
			"ok":           true,
			"action":       "restart-scheduled",
			"sentinelPath": sentinelPath,
		}, nil)

	case "dev":
		// 开发模式：执行 go build / go install
		goPath, err := exec.LookPath("go")
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "go not found in PATH"))
			return
		}

		cmd := exec.Command(goPath, "build", "-o", "openacosmi-gateway", ".")
		cmd.Dir = resolveProjectRoot()
		output, err := cmd.CombinedOutput()
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "build failed: "+string(output)))
			return
		}

		// 写重启哨兵
		sentinelPath := filepath.Join(baseDir, ".restart-sentinel")
		_ = os.WriteFile(sentinelPath, []byte(fmt.Sprintf("%d", time.Now().UnixMilli())), 0600)

		ctx.Respond(true, map[string]interface{}{
			"ok":     true,
			"action": "rebuilt-and-restart-scheduled",
			"output": strings.TrimSpace(string(output)),
		}, nil)

	default:
		// 默认模式：检查 + 写哨兵
		scheduleRestartMs, _ := ctx.Params["scheduleRestartMs"].(float64)
		sentinelPath := filepath.Join(baseDir, ".restart-sentinel")
		payload := fmt.Sprintf(`{"ts":%d,"scheduleRestartMs":%d}`, time.Now().UnixMilli(), int64(scheduleRestartMs))
		if err := os.WriteFile(sentinelPath, []byte(payload), 0600); err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write sentinel: "+err.Error()))
			return
		}

		ctx.Respond(true, map[string]interface{}{
			"ok":                true,
			"action":            "sentinel-written",
			"scheduleRestartMs": int64(scheduleRestartMs),
			"currentVersion":    resolveCurrentVersion(),
		}, nil)
	}
}

// ---------- 辅助 ----------

func resolveCurrentVersion() string {
	v := preferredGatewayEnvValue("CRABCLAW_VERSION", "OPENACOSMI_VERSION")
	if v != "" {
		return v
	}
	return "dev"
}

func resolveDefaultStateDir() string {
	dir := preferredGatewayEnvValue("CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR")
	if dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "openacosmi")
}

func resolveProjectRoot() string {
	dir := preferredGatewayEnvValue("CRABCLAW_PROJECT_ROOT", "OPENACOSMI_PROJECT_ROOT")
	if dir != "" {
		return dir
	}
	// 回退到可执行文件所在目录
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func loadGatewayUpdateConfig(ctx *GatewayMethodContext) (*types.OpenAcosmiConfig, error) {
	if ctx == nil {
		return &types.OpenAcosmiConfig{}, nil
	}
	if ctx.ConfigLoader != nil {
		cfg, err := ctx.ConfigLoader.LoadConfig()
		if err != nil {
			return nil, err
		}
		if cfg == nil {
			cfg = &types.OpenAcosmiConfig{}
		}
		ctx.Config = cfg
		return cfg, nil
	}
	if ctx.Config != nil {
		return ctx.Config, nil
	}
	return &types.OpenAcosmiConfig{}, nil
}

func persistGatewayUpdateMetadata(
	ctx *GatewayMethodContext,
	cfg *types.OpenAcosmiConfig,
	lastCheckedAt string,
	lastSeenVersion string,
) error {
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	if cfg.Update == nil {
		cfg.Update = &types.OpenAcosmiUpdateConfig{}
	}
	cfg.Update.LastCheckedAt = strings.TrimSpace(lastCheckedAt)
	if candidate := strings.TrimSpace(lastSeenVersion); candidate != "" {
		cfg.Update.LastSeenVersion = candidate
	}
	if ctx != nil {
		ctx.Config = cfg
	}
	if ctx != nil && ctx.ConfigLoader != nil {
		if err := ctx.ConfigLoader.WriteConfigFile(cfg); err != nil {
			return fmt.Errorf("failed to persist desktop update metadata: %w", err)
		}
		ctx.ConfigLoader.ClearCache()
	}
	return nil
}

func loadDesktopUpdateStatus(ctx *GatewayMethodContext) (gatewayDesktopUpdateStatus, error) {
	state, err := loadDesktopUpdateStateWithDefaults(ctx)
	if err != nil {
		return gatewayDesktopUpdateStatus{}, err
	}
	if state.State == desktopUpdateStageChecking {
		state.State = desktopUpdateStageIdle
	}
	return buildDesktopUpdateStatus(state), nil
}

func buildDesktopUpdateStatus(state gatewayDesktopUpdateState) gatewayDesktopUpdateStatus {
	return gatewayDesktopUpdateStatus{
		CurrentVersion:    state.CurrentVersion,
		CandidateVersion:  state.CandidateVersion,
		Channel:           state.Channel,
		InstallKind:       state.InstallKind,
		UpdateManager:     state.UpdateManager,
		ManagedBySystem:   state.ManagedBySystem,
		State:             state.State,
		Progress:          state.Progress,
		DownloadedBytes:   state.DownloadedBytes,
		TotalBytes:        state.TotalBytes,
		PublishedAt:       state.PublishedAt,
		ReadyToInstall:    state.ReadyToInstall,
		RollbackAvailable: state.RollbackAvailable,
		RollbackVersion:   state.RollbackVersion,
		LastCheckedAt:     state.LastCheckedAt,
		LastError:         state.LastError,
		UpdatedAt:         state.UpdatedAt,
		UpdateAvailable:   state.ReadyToInstall || hasCandidateUpdate(state.CurrentVersion, state.CandidateVersion),
	}
}

func hasCandidateUpdate(currentVersion string, candidateVersion string) bool {
	candidateVersion = strings.TrimSpace(candidateVersion)
	if candidateVersion == "" {
		return false
	}
	currentVersion = strings.TrimSpace(currentVersion)
	return currentVersion == "" || candidateVersion != currentVersion
}

func resolveDesktopUpdateChannel(cfg *types.OpenAcosmiConfig) string {
	if cfg != nil && cfg.Update != nil {
		if channel := strings.TrimSpace(cfg.Update.Channel); channel != "" {
			return channel
		}
	}
	return "stable"
}

func resolveDesktopUpdateStatePath() string {
	return filepath.Join(config.ResolveStateDir(), desktopUpdateStateFilename)
}

func readDesktopUpdateStateFile() (*gatewayDesktopUpdateState, error) {
	raw, err := os.ReadFile(resolveDesktopUpdateStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state gatewayDesktopUpdateState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decode desktop update state: %w", err)
	}
	return &state, nil
}

func writeDesktopUpdateStateFile(state gatewayDesktopUpdateState) (string, error) {
	path := resolveDesktopUpdateStatePath()
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

func detectGatewayDesktopInstallKind() string {
	exe, _ := desktopUpdateExecutable()
	return detectGatewayDesktopInstallKindFromProbe(desktopInstallProbe{
		GOOS:    desktopUpdateGOOS,
		ExePath: exe,
		Env: map[string]string{
			"APPX_PACKAGE_FAMILY_NAME": desktopUpdateGetenv("APPX_PACKAGE_FAMILY_NAME"),
			"APPIMAGE":                 desktopUpdateGetenv("APPIMAGE"),
			"APPDIR":                   desktopUpdateGetenv("APPDIR"),
			"CRABCLAW_PROJECT_ROOT":    desktopUpdateGetenv("CRABCLAW_PROJECT_ROOT"),
			"OPENACOSMI_PROJECT_ROOT":  desktopUpdateGetenv("OPENACOSMI_PROJECT_ROOT"),
		},
	})
}

func detectGatewayDesktopInstallKindFromProbe(probe desktopInstallProbe) string {
	goos := strings.TrimSpace(strings.ToLower(probe.GOOS))
	exePath := strings.ToLower(filepath.ToSlash(strings.TrimSpace(probe.ExePath)))

	if probe.Env["CRABCLAW_PROJECT_ROOT"] != "" || probe.Env["OPENACOSMI_PROJECT_ROOT"] != "" {
		return desktopInstallKindSource
	}
	if strings.Contains(exePath, "/backend/cmd/desktop/") {
		return desktopInstallKindSource
	}

	switch goos {
	case "darwin":
		if strings.Contains(exePath, ".app/contents/macos/") {
			return desktopInstallKindMacOSWails
		}
	case "windows":
		if probe.Env["APPX_PACKAGE_FAMILY_NAME"] != "" {
			return desktopInstallKindWindowsMSIX
		}
		return desktopInstallKindWindowsNSIS
	case "linux":
		if probe.Env["APPIMAGE"] != "" || probe.Env["APPDIR"] != "" {
			return desktopInstallKindLinuxAppImage
		}
		if strings.HasPrefix(exePath, "/usr/") || strings.HasPrefix(exePath, "/opt/") {
			return desktopInstallKindLinuxSystemPackage
		}
	}

	return desktopInstallKindUnknown
}

func isPackagedDesktopInstallKind(kind string) bool {
	switch kind {
	case desktopInstallKindMacOSWails,
		desktopInstallKindWindowsMSIX,
		desktopInstallKindWindowsNSIS,
		desktopInstallKindLinuxAppImage,
		desktopInstallKindLinuxSystemPackage:
		return true
	default:
		return false
	}
}

func isSystemManagedDesktopInstall(kind string) bool {
	return kind == desktopInstallKindWindowsMSIX
}

func resolveGatewayDesktopUpdateManager(installKind string) string {
	switch strings.TrimSpace(installKind) {
	case desktopInstallKindSource:
		return "source"
	case desktopInstallKindWindowsMSIX:
		return "system"
	case desktopInstallKindWindowsNSIS:
		return "installer"
	case desktopInstallKindLinuxSystemPackage:
		return "package-manager"
	case desktopInstallKindLinuxAppImage:
		return "appimage"
	case desktopInstallKindMacOSWails:
		return "host"
	default:
		return "unknown"
	}
}
