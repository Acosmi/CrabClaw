package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

func setDesktopUpdateProbeForTest(t *testing.T, goos string, goarch string, exePath string) {
	t.Helper()

	oldGOOS := desktopUpdateGOOS
	oldGOARCH := desktopUpdateGOARCH
	oldExecutable := desktopUpdateExecutable
	t.Cleanup(func() {
		desktopUpdateGOOS = oldGOOS
		desktopUpdateGOARCH = oldGOARCH
		desktopUpdateExecutable = oldExecutable
	})

	desktopUpdateGOOS = goos
	desktopUpdateGOARCH = goarch
	desktopUpdateExecutable = func() (string, error) {
		return exePath, nil
	}
}

func setDesktopUpdateOpenArtifactForTest(t *testing.T, fn func(string, string) error) {
	t.Helper()

	old := desktopUpdateOpenArtifact
	t.Cleanup(func() {
		desktopUpdateOpenArtifact = old
	})
	desktopUpdateOpenArtifact = fn
}

func newDesktopUpdateTestFeed(t *testing.T, platformKey string, version string, assetName string, assetBody []byte) *httptest.Server {
	t.Helper()

	sum := sha256.Sum256(assetBody)
	shaHex := hex.EncodeToString(sum[:])
	mux := http.NewServeMux()
	mux.HandleFunc("/stable/update.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version":     version,
			"channel":     "stable",
			"publishedAt": "2026-03-09T12:00:00Z",
			"platforms": map[string]interface{}{
				platformKey: map[string]interface{}{
					"url":    "/downloads/" + assetName,
					"sha256": shaHex,
					"size":   len(assetBody),
					"name":   assetName,
				},
			},
		})
	})
	mux.HandleFunc("/downloads/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(assetBody)
	})
	return httptest.NewServer(mux)
}

func TestAuthorizeGatewayMethod_DesktopUpdateScopes(t *testing.T) {
	readClient := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{scopeRead},
	}}
	if err := AuthorizeGatewayMethod("desktop.update.status", readClient); err != nil {
		t.Fatalf("desktop.update.status should allow read scope: %v", err)
	}
	if err := AuthorizeGatewayMethod("desktop.update.check", readClient); err == nil {
		t.Fatal("desktop.update.check should require write scope")
	}

	writeClient := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{scopeWrite},
	}}
	if err := AuthorizeGatewayMethod("desktop.update.check", writeClient); err != nil {
		t.Fatalf("desktop.update.check should allow write scope: %v", err)
	}
	for _, method := range []string{"desktop.update.download", "desktop.update.apply", "desktop.update.rollback", "desktop.update.dismiss"} {
		if err := AuthorizeGatewayMethod(method, writeClient); err != nil {
			t.Fatalf("%s should allow write scope: %v", method, err)
		}
	}
}

func TestDetectGatewayDesktopInstallKindFromProbe(t *testing.T) {
	tests := []struct {
		name  string
		probe desktopInstallProbe
		want  string
	}{
		{
			name: "source env override",
			probe: desktopInstallProbe{
				GOOS:    "darwin",
				ExePath: "/tmp/CrabClaw",
				Env:     map[string]string{"CRABCLAW_PROJECT_ROOT": "/workspace"},
			},
			want: desktopInstallKindSource,
		},
		{
			name: "macos bundle",
			probe: desktopInstallProbe{
				GOOS:    "darwin",
				ExePath: "/Applications/CrabClaw.app/Contents/MacOS/CrabClaw",
				Env:     map[string]string{},
			},
			want: desktopInstallKindMacOSWails,
		},
		{
			name: "windows msix",
			probe: desktopInstallProbe{
				GOOS:    "windows",
				ExePath: `C:\Program Files\CrabClaw\CrabClaw.exe`,
				Env:     map[string]string{"APPX_PACKAGE_FAMILY_NAME": "CrabClaw_123"},
			},
			want: desktopInstallKindWindowsMSIX,
		},
		{
			name: "windows nsis",
			probe: desktopInstallProbe{
				GOOS:    "windows",
				ExePath: `C:\Program Files\CrabClaw\CrabClaw.exe`,
				Env:     map[string]string{},
			},
			want: desktopInstallKindWindowsNSIS,
		},
		{
			name: "linux appimage",
			probe: desktopInstallProbe{
				GOOS:    "linux",
				ExePath: "/tmp/.mount_crabclaw/usr/bin/CrabClaw",
				Env:     map[string]string{"APPIMAGE": "/tmp/CrabClaw.AppImage"},
			},
			want: desktopInstallKindLinuxAppImage,
		},
		{
			name: "linux system package",
			probe: desktopInstallProbe{
				GOOS:    "linux",
				ExePath: "/usr/bin/crabclaw",
				Env:     map[string]string{},
			},
			want: desktopInstallKindLinuxSystemPackage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectGatewayDesktopInstallKindFromProbe(tt.probe); got != tt.want {
				t.Fatalf("detectGatewayDesktopInstallKindFromProbe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDesktopUpdateStatusDefaultsFromConfig(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "1.2.3")
	setDesktopUpdateProbeForTest(t, "darwin", "amd64", "/Applications/CrabClaw.app/Contents/MacOS/CrabClaw")

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	req := &RequestFrame{Method: "desktop.update.status", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{
		Config: &types.OpenAcosmiConfig{
			Update: &types.OpenAcosmiUpdateConfig{
				Channel:       "beta",
				LastCheckedAt: "2026-03-08T00:00:00Z",
			},
		},
	}, func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	})
	if !gotOK {
		t.Fatal("desktop.update.status should succeed")
	}

	status, ok := gotPayload.(gatewayDesktopUpdateStatus)
	if !ok {
		t.Fatalf("expected gatewayDesktopUpdateStatus, got %T", gotPayload)
	}
	if status.Channel != "beta" {
		t.Fatalf("expected channel beta, got %q", status.Channel)
	}
	if status.CurrentVersion != "1.2.3" {
		t.Fatalf("expected currentVersion 1.2.3, got %q", status.CurrentVersion)
	}
	if status.InstallKind != desktopInstallKindMacOSWails {
		t.Fatalf("expected install kind %q, got %q", desktopInstallKindMacOSWails, status.InstallKind)
	}
	if status.UpdateManager != "host" {
		t.Fatalf("expected update manager host, got %q", status.UpdateManager)
	}
	if status.State != desktopUpdateStageIdle {
		t.Fatalf("expected idle state, got %q", status.State)
	}
	if status.LastCheckedAt != "2026-03-08T00:00:00Z" {
		t.Fatalf("expected lastCheckedAt from config, got %q", status.LastCheckedAt)
	}
}

func TestDesktopUpdateCheckPersistsConfigAndState(t *testing.T) {
	stateDir := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "2.0.0")
	setDesktopUpdateProbeForTest(t, "darwin", "amd64", "/Applications/CrabClaw.app/Contents/MacOS/CrabClaw")
	feed := newDesktopUpdateTestFeed(t, "macos-wails-amd64", "2.1.0", "CrabClaw-macos-amd64.zip", []byte("desktop-update"))
	defer feed.Close()

	loader := config.NewConfigLoader(config.WithConfigPath(cfgPath))
	if err := loader.WriteConfigFile(&types.OpenAcosmiConfig{
		Update: &types.OpenAcosmiUpdateConfig{
			Channel:   "stable",
			SourceURL: feed.URL,
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	req := &RequestFrame{Method: "desktop.update.check", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	})
	if !gotOK {
		t.Fatal("desktop.update.check should succeed")
	}

	status, ok := gotPayload.(gatewayDesktopUpdateStatus)
	if !ok {
		t.Fatalf("expected gatewayDesktopUpdateStatus, got %T", gotPayload)
	}
	if status.Channel != "stable" {
		t.Fatalf("expected channel stable, got %q", status.Channel)
	}
	if status.InstallKind != desktopInstallKindMacOSWails {
		t.Fatalf("expected install kind %q, got %q", desktopInstallKindMacOSWails, status.InstallKind)
	}
	if status.State != desktopUpdateStageAvailable {
		t.Fatalf("expected available state, got %q", status.State)
	}
	if status.CandidateVersion != "2.1.0" {
		t.Fatalf("expected candidate version 2.1.0, got %q", status.CandidateVersion)
	}
	if !status.UpdateAvailable {
		t.Fatal("expected updateAvailable=true")
	}
	if status.LastCheckedAt == "" {
		t.Fatal("expected lastCheckedAt to be populated")
	}

	loader.ClearCache()
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.Update == nil || cfg.Update.LastCheckedAt == "" {
		t.Fatal("expected update.lastCheckedAt persisted to config")
	}

	state, err := readDesktopUpdateStateFile()
	if err != nil {
		t.Fatalf("read desktop update state: %v", err)
	}
	if state == nil {
		t.Fatal("expected desktop update state file to exist")
	}
	if state.InstallKind != desktopInstallKindMacOSWails {
		t.Fatalf("expected persisted install kind %q, got %q", desktopInstallKindMacOSWails, state.InstallKind)
	}
	if state.State != desktopUpdateStageAvailable {
		t.Fatalf("expected persisted state available, got %q", state.State)
	}
	if state.ManifestURL == "" || state.AssetURL == "" {
		t.Fatal("expected manifest and asset URLs to be persisted")
	}
}

func TestUpdateRun_SourceInstallWritesRestartSentinel(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_PROJECT_ROOT", t.TempDir())

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	req := &RequestFrame{Method: "update.run", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	})
	if !gotOK {
		t.Fatal("update.run should succeed for source installs")
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	if result["action"] != "sentinel-written" {
		t.Fatalf("expected sentinel-written action, got %#v", result["action"])
	}
	if _, err := os.Stat(filepath.Join(stateDir, ".restart-sentinel")); err != nil {
		t.Fatalf("expected restart sentinel to exist: %v", err)
	}
}

func TestUpdateRun_PackagedInstallReturnsDesktopAction(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "3.1.0")
	setDesktopUpdateProbeForTest(t, "darwin", "amd64", "/Applications/CrabClaw.app/Contents/MacOS/CrabClaw")

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	req := &RequestFrame{Method: "update.run", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{
		Config: &types.OpenAcosmiConfig{
			Update: &types.OpenAcosmiUpdateConfig{Channel: "beta"},
		},
	}, func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	})
	if !gotOK {
		t.Fatal("update.run should succeed for packaged installs")
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	if result["action"] != "desktop-update-required" {
		t.Fatalf("expected desktop-update-required action, got %#v", result["action"])
	}
	if result["installKind"] != desktopInstallKindMacOSWails {
		t.Fatalf("expected install kind %q, got %#v", desktopInstallKindMacOSWails, result["installKind"])
	}
	if _, err := os.Stat(filepath.Join(stateDir, ".restart-sentinel")); !os.IsNotExist(err) {
		t.Fatalf("restart sentinel should not be written for packaged installs, err=%v", err)
	}

	state, err := readDesktopUpdateStateFile()
	if err != nil {
		t.Fatalf("read desktop update state: %v", err)
	}
	if state == nil {
		t.Fatal("expected desktop update state to be initialized")
	}
	if state.Channel != "beta" {
		t.Fatalf("expected state channel beta, got %q", state.Channel)
	}
}

func TestUpdateCheck_PackagedInstallUsesDesktopState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "3.1.0")
	setDesktopUpdateProbeForTest(t, "darwin", "amd64", "/Applications/CrabClaw.app/Contents/MacOS/CrabClaw")

	if _, err := writeDesktopUpdateStateFile(gatewayDesktopUpdateState{
		CurrentVersion:   "3.1.0",
		CandidateVersion: "3.2.0",
		Channel:          "stable",
		InstallKind:      desktopInstallKindMacOSWails,
		State:            desktopUpdateStageAvailable,
	}); err != nil {
		t.Fatalf("seed desktop update state: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	req := &RequestFrame{Method: "update.check", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	})
	if !gotOK {
		t.Fatal("update.check should succeed")
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	if result["updateAvailable"] != true {
		t.Fatalf("expected updateAvailable=true, got %#v", result["updateAvailable"])
	}
	if result["candidateVersion"] != "3.2.0" {
		t.Fatalf("expected candidateVersion 3.2.0, got %#v", result["candidateVersion"])
	}
	if result["installKind"] != desktopInstallKindMacOSWails {
		t.Fatalf("expected install kind %q, got %#v", desktopInstallKindMacOSWails, result["installKind"])
	}
}

func TestDesktopUpdateDownloadApplyAndDismissFlow(t *testing.T) {
	stateDir := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "4.0.0")
	setDesktopUpdateProbeForTest(t, "darwin", "amd64", "/Applications/CrabClaw.app/Contents/MacOS/CrabClaw")

	opened := ""
	setDesktopUpdateOpenArtifactForTest(t, func(_ string, artifactPath string) error {
		opened = artifactPath
		return nil
	})

	feed := newDesktopUpdateTestFeed(t, "macos-wails-amd64", "4.1.0", "CrabClaw-macos-amd64.zip", []byte("pkg-bytes"))
	defer feed.Close()

	loader := config.NewConfigLoader(config.WithConfigPath(cfgPath))
	if err := loader.WriteConfigFile(&types.OpenAcosmiConfig{
		Update: &types.OpenAcosmiUpdateConfig{
			Channel:   "stable",
			SourceURL: feed.URL,
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())
	ctx := &GatewayMethodContext{ConfigLoader: loader}

	var checkPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.check", Params: map[string]interface{}{}}, nil, ctx,
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.check failed: %v", err)
			}
			checkPayload = payload
		})
	checkStatus := checkPayload.(gatewayDesktopUpdateStatus)
	if checkStatus.Action != "checked" {
		t.Fatalf("expected checked action, got %q", checkStatus.Action)
	}
	if checkStatus.CandidateVersion != "4.1.0" {
		t.Fatalf("expected candidate version 4.1.0, got %q", checkStatus.CandidateVersion)
	}

	var downloadPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.download", Params: map[string]interface{}{}}, nil, ctx,
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.download failed: %v", err)
			}
			downloadPayload = payload
		})
	downloadStatus := downloadPayload.(gatewayDesktopUpdateStatus)
	if downloadStatus.Action != "downloaded" {
		t.Fatalf("expected downloaded action, got %q", downloadStatus.Action)
	}
	if !downloadStatus.ReadyToInstall {
		t.Fatal("expected readyToInstall=true")
	}

	state, err := readDesktopUpdateStateFile()
	if err != nil {
		t.Fatalf("read desktop update state: %v", err)
	}
	if state == nil || state.DownloadedFile == "" {
		t.Fatal("expected downloaded file to be persisted")
	}
	if _, err := os.Stat(state.DownloadedFile); err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}

	var applyPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.apply", Params: map[string]interface{}{}}, nil, ctx,
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.apply failed: %v", err)
			}
			applyPayload = payload
		})
	applyStatus := applyPayload.(gatewayDesktopUpdateStatus)
	if applyStatus.Action != "installer-launched-restart-required" {
		t.Fatalf("expected installer-launched-restart-required action, got %q", applyStatus.Action)
	}
	if opened == "" {
		t.Fatal("expected downloaded artifact to be opened")
	}
	handoff, err := readDesktopInstallerHandoffFile()
	if err != nil {
		t.Fatalf("read desktop installer handoff: %v", err)
	}
	if handoff == nil {
		t.Fatal("expected desktop installer handoff marker")
	}
	if handoff.ToVersion != "4.1.0" {
		t.Fatalf("expected handoff toVersion 4.1.0, got %q", handoff.ToVersion)
	}

	var dismissPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.dismiss", Params: map[string]interface{}{}}, nil, ctx,
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.dismiss failed: %v", err)
			}
			dismissPayload = payload
		})
	dismissStatus := dismissPayload.(gatewayDesktopUpdateStatus)
	if dismissStatus.Action != "dismissed" {
		t.Fatalf("expected dismissed action, got %q", dismissStatus.Action)
	}
	if dismissStatus.CandidateVersion != "" {
		t.Fatalf("expected candidate version to be cleared, got %q", dismissStatus.CandidateVersion)
	}

	loader.ClearCache()
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.Update == nil || len(cfg.Update.SkippedVersions) != 1 || cfg.Update.SkippedVersions[0] != "4.1.0" {
		t.Fatalf("expected skipped version 4.1.0, got %+v", cfg.Update)
	}
}

func TestDesktopUpdateApply_LinuxAppImageSwapsExecutableAndWritesPendingMarker(t *testing.T) {
	stateDir := t.TempDir()
	currentPath := filepath.Join(t.TempDir(), "CrabClaw.AppImage")
	downloadedPath := filepath.Join(stateDir, "updates", "5.1.0", "CrabClaw.AppImage")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "5.0.0")
	t.Setenv("APPIMAGE", currentPath)
	setDesktopUpdateProbeForTest(t, "linux", "amd64", currentPath)

	if err := os.WriteFile(currentPath, []byte("old-appimage"), 0o755); err != nil {
		t.Fatalf("write current appimage: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(downloadedPath), 0o755); err != nil {
		t.Fatalf("mkdir download dir: %v", err)
	}
	if err := os.WriteFile(downloadedPath, []byte("new-appimage"), 0o755); err != nil {
		t.Fatalf("write downloaded appimage: %v", err)
	}
	if _, err := writeDesktopUpdateStateFile(gatewayDesktopUpdateState{
		CurrentVersion:   "5.0.0",
		CandidateVersion: "5.1.0",
		Channel:          "stable",
		InstallKind:      desktopInstallKindLinuxAppImage,
		UpdateManager:    "appimage",
		State:            desktopUpdateStageReadyToInstall,
		DownloadedFile:   downloadedPath,
		ReadyToInstall:   true,
	}); err != nil {
		t.Fatalf("seed desktop update state: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	var gotPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.apply", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{},
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.apply failed: %v", err)
			}
			gotPayload = payload
		})

	status := gotPayload.(gatewayDesktopUpdateStatus)
	if status.Action != "appimage-applied-restart-required" {
		t.Fatalf("expected appimage-applied-restart-required, got %q", status.Action)
	}
	if status.State != desktopUpdateStageApplying {
		t.Fatalf("expected applying state, got %q", status.State)
	}

	currentData, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current appimage: %v", err)
	}
	if string(currentData) != "new-appimage" {
		t.Fatalf("expected current appimage to be replaced, got %q", string(currentData))
	}

	pending, err := readDesktopPendingUpdateFile()
	if err != nil {
		t.Fatalf("read pending desktop update: %v", err)
	}
	if pending == nil {
		t.Fatal("expected pending desktop update marker")
	}
	if pending.ToVersion != "5.1.0" {
		t.Fatalf("expected pending toVersion 5.1.0, got %q", pending.ToVersion)
	}
	backupData, err := os.ReadFile(pending.BackupPath)
	if err != nil {
		t.Fatalf("read backup appimage: %v", err)
	}
	if string(backupData) != "old-appimage" {
		t.Fatalf("expected backup appimage to contain old bytes, got %q", string(backupData))
	}
}

func TestDesktopUpdateApply_WindowsNSISWritesInstallerHandoffMarker(t *testing.T) {
	stateDir := t.TempDir()
	downloadedPath := filepath.Join(stateDir, "updates", "9.1.0", "CrabClaw-setup.exe")
	setDesktopUpdateProbeForTest(t, "windows", "amd64", `C:\Program Files\CrabClaw\CrabClaw.exe`)
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "9.0.0")

	opened := ""
	setDesktopUpdateOpenArtifactForTest(t, func(_ string, artifactPath string) error {
		opened = artifactPath
		return nil
	})

	if err := os.MkdirAll(filepath.Dir(downloadedPath), 0o755); err != nil {
		t.Fatalf("mkdir download dir: %v", err)
	}
	if err := os.WriteFile(downloadedPath, []byte("nsis-installer"), 0o755); err != nil {
		t.Fatalf("write installer: %v", err)
	}
	if _, err := writeDesktopUpdateStateFile(gatewayDesktopUpdateState{
		CurrentVersion:   "9.0.0",
		CandidateVersion: "9.1.0",
		Channel:          "stable",
		InstallKind:      desktopInstallKindWindowsNSIS,
		UpdateManager:    "installer",
		State:            desktopUpdateStageReadyToInstall,
		DownloadedFile:   downloadedPath,
		ReadyToInstall:   true,
	}); err != nil {
		t.Fatalf("seed desktop update state: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	var gotPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.apply", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{},
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.apply failed: %v", err)
			}
			gotPayload = payload
		})

	status := gotPayload.(gatewayDesktopUpdateStatus)
	if status.Action != "installer-launched-restart-required" {
		t.Fatalf("expected installer-launched-restart-required, got %q", status.Action)
	}
	if status.State != desktopUpdateStageApplying {
		t.Fatalf("expected applying state, got %q", status.State)
	}
	if opened != downloadedPath {
		t.Fatalf("expected installer handoff to open %q, got %q", downloadedPath, opened)
	}

	handoff, err := readDesktopInstallerHandoffFile()
	if err != nil {
		t.Fatalf("read desktop installer handoff: %v", err)
	}
	if handoff == nil {
		t.Fatal("expected installer handoff marker")
	}
	if handoff.InstallKind != desktopInstallKindWindowsNSIS {
		t.Fatalf("expected handoff install kind %q, got %q", desktopInstallKindWindowsNSIS, handoff.InstallKind)
	}
	if handoff.ToVersion != "9.1.0" {
		t.Fatalf("expected handoff toVersion 9.1.0, got %q", handoff.ToVersion)
	}
}

func TestDesktopUpdateRollback_LinuxAppImageRestoresBackupAndClearsPendingMarker(t *testing.T) {
	stateDir := t.TempDir()
	currentPath := filepath.Join(t.TempDir(), "CrabClaw.AppImage")
	downloadedPath := filepath.Join(stateDir, "updates", "7.1.0", "CrabClaw.AppImage")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "7.0.0")
	t.Setenv("APPIMAGE", currentPath)
	setDesktopUpdateProbeForTest(t, "linux", "amd64", currentPath)

	if err := os.WriteFile(currentPath, []byte("old-appimage"), 0o755); err != nil {
		t.Fatalf("write current appimage: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(downloadedPath), 0o755); err != nil {
		t.Fatalf("mkdir download dir: %v", err)
	}
	if err := os.WriteFile(downloadedPath, []byte("new-appimage"), 0o755); err != nil {
		t.Fatalf("write downloaded appimage: %v", err)
	}
	if _, err := writeDesktopUpdateStateFile(gatewayDesktopUpdateState{
		CurrentVersion:   "7.0.0",
		CandidateVersion: "7.1.0",
		Channel:          "stable",
		InstallKind:      desktopInstallKindLinuxAppImage,
		UpdateManager:    "appimage",
		State:            desktopUpdateStageReadyToInstall,
		DownloadedFile:   downloadedPath,
		ReadyToInstall:   true,
	}); err != nil {
		t.Fatalf("seed desktop update state: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.apply", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{},
		func(ok bool, _ interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.apply failed: %v", err)
			}
		})

	var gotPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.rollback", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{},
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.rollback failed: %v", err)
			}
			gotPayload = payload
		})

	status := gotPayload.(gatewayDesktopUpdateStatus)
	if status.Action != "rollback-completed" {
		t.Fatalf("expected rollback-completed, got %q", status.Action)
	}
	if status.State != desktopUpdateStageRolledBack {
		t.Fatalf("expected rolled-back state, got %q", status.State)
	}
	if status.CandidateVersion != "" {
		t.Fatalf("expected candidate version to be cleared, got %q", status.CandidateVersion)
	}
	if status.RollbackAvailable {
		t.Fatal("expected rollback metadata to be cleared")
	}

	currentData, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current appimage: %v", err)
	}
	if string(currentData) != "old-appimage" {
		t.Fatalf("expected current appimage to be restored, got %q", string(currentData))
	}
	if pending, err := readDesktopPendingUpdateFile(); err != nil {
		t.Fatalf("read pending desktop update: %v", err)
	} else if pending != nil {
		t.Fatal("expected pending desktop update marker to be deleted")
	}
}

func TestDesktopUpdateRollback_LinuxAppImageAfterSuccessfulUpdateRequiresRestart(t *testing.T) {
	stateDir := t.TempDir()
	currentPath := filepath.Join(t.TempDir(), "CrabClaw.AppImage")
	backupPath := filepath.Join(stateDir, "updates", "backups", "8.0.0-CrabClaw.AppImage")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("CRABCLAW_VERSION", "8.1.0")
	t.Setenv("APPIMAGE", currentPath)
	setDesktopUpdateProbeForTest(t, "linux", "amd64", currentPath)

	if err := os.WriteFile(currentPath, []byte("new-appimage"), 0o755); err != nil {
		t.Fatalf("write current appimage: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("old-appimage"), 0o755); err != nil {
		t.Fatalf("write backup appimage: %v", err)
	}
	if _, err := writeDesktopUpdateStateFile(gatewayDesktopUpdateState{
		CurrentVersion:     "8.1.0",
		Channel:            "stable",
		InstallKind:        desktopInstallKindLinuxAppImage,
		UpdateManager:      "appimage",
		State:              desktopUpdateStageIdle,
		RollbackAvailable:  true,
		RollbackVersion:    "8.0.0",
		RollbackBackupPath: backupPath,
	}); err != nil {
		t.Fatalf("seed desktop update state: %v", err)
	}

	r := NewMethodRegistry()
	r.RegisterAll(UpdateHandlers())

	var gotPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "desktop.update.rollback", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{},
		func(ok bool, payload interface{}, err *ErrorShape) {
			if !ok || err != nil {
				t.Fatalf("desktop.update.rollback failed: %v", err)
			}
			gotPayload = payload
		})

	status := gotPayload.(gatewayDesktopUpdateStatus)
	if status.Action != "rollback-applied-restart-required" {
		t.Fatalf("expected rollback-applied-restart-required, got %q", status.Action)
	}
	if status.State != desktopUpdateStageRolledBack {
		t.Fatalf("expected rolled-back state, got %q", status.State)
	}
	if status.RollbackAvailable {
		t.Fatal("expected rollback metadata to be cleared")
	}

	currentData, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current appimage: %v", err)
	}
	if string(currentData) != "old-appimage" {
		t.Fatalf("expected current appimage to be restored, got %q", string(currentData))
	}
}
