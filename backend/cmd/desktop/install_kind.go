package main

import (
	"path/filepath"
	"runtime"
	"strings"
)

// desktopInstallKind 表示桌面宿主当前的安装形态。
type desktopInstallKind string

const (
	installKindUnknown            desktopInstallKind = "unknown"
	installKindSource             desktopInstallKind = "source"
	installKindMacOSWails         desktopInstallKind = "macos-wails"
	installKindWindowsMSIX        desktopInstallKind = "windows-msix"
	installKindWindowsNSIS        desktopInstallKind = "windows-nsis"
	installKindLinuxAppImage      desktopInstallKind = "linux-appimage"
	installKindLinuxSystemPackage desktopInstallKind = "linux-system-package"
)

type desktopInstallProbe struct {
	GOOS    string
	ExePath string
	Env     map[string]string
}

func detectDesktopInstallKind() desktopInstallKind {
	exe, _ := osExecutable()
	return detectDesktopInstallKindFromProbe(desktopInstallProbe{
		GOOS:    runtime.GOOS,
		ExePath: exe,
		Env: map[string]string{
			"APPX_PACKAGE_FAMILY_NAME": getenv("APPX_PACKAGE_FAMILY_NAME"),
			"APPIMAGE":                 getenv("APPIMAGE"),
			"APPDIR":                   getenv("APPDIR"),
			"CRABCLAW_PROJECT_ROOT":    getenv("CRABCLAW_PROJECT_ROOT"),
			"OPENACOSMI_PROJECT_ROOT":  getenv("OPENACOSMI_PROJECT_ROOT"),
		},
	})
}

func detectDesktopInstallKindFromProbe(probe desktopInstallProbe) desktopInstallKind {
	goos := strings.TrimSpace(strings.ToLower(probe.GOOS))
	exePath := strings.ToLower(filepath.ToSlash(strings.TrimSpace(probe.ExePath)))

	if probe.Env["CRABCLAW_PROJECT_ROOT"] != "" || probe.Env["OPENACOSMI_PROJECT_ROOT"] != "" {
		return installKindSource
	}
	if strings.Contains(exePath, "/backend/cmd/desktop/") {
		return installKindSource
	}

	switch goos {
	case "darwin":
		if strings.Contains(exePath, ".app/contents/macos/") {
			return installKindMacOSWails
		}
	case "windows":
		if probe.Env["APPX_PACKAGE_FAMILY_NAME"] != "" {
			return installKindWindowsMSIX
		}
		return installKindWindowsNSIS
	case "linux":
		if probe.Env["APPIMAGE"] != "" || probe.Env["APPDIR"] != "" {
			return installKindLinuxAppImage
		}
		if strings.HasPrefix(exePath, "/usr/") || strings.HasPrefix(exePath, "/opt/") {
			return installKindLinuxSystemPackage
		}
	}

	return installKindUnknown
}
