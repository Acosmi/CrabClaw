package main

import "testing"

func TestDetectDesktopInstallKindFromProbe(t *testing.T) {
	t.Run("source from project root env", func(t *testing.T) {
		got := detectDesktopInstallKindFromProbe(desktopInstallProbe{
			GOOS:    "darwin",
			ExePath: "/Applications/Crab Claw.app/Contents/MacOS/CrabClaw",
			Env: map[string]string{
				"CRABCLAW_PROJECT_ROOT": "/Users/dev/Desktop/CrabClaw",
			},
		})
		if got != installKindSource {
			t.Fatalf("expected %q, got %q", installKindSource, got)
		}
	})

	t.Run("macos wails app bundle", func(t *testing.T) {
		got := detectDesktopInstallKindFromProbe(desktopInstallProbe{
			GOOS:    "darwin",
			ExePath: "/Applications/Crab Claw.app/Contents/MacOS/CrabClaw",
			Env:     map[string]string{},
		})
		if got != installKindMacOSWails {
			t.Fatalf("expected %q, got %q", installKindMacOSWails, got)
		}
	})

	t.Run("windows msix", func(t *testing.T) {
		got := detectDesktopInstallKindFromProbe(desktopInstallProbe{
			GOOS:    "windows",
			ExePath: `C:/Program Files/WindowsApps/CrabClaw/CrabClaw.exe`,
			Env: map[string]string{
				"APPX_PACKAGE_FAMILY_NAME": "CrabClaw_123",
			},
		})
		if got != installKindWindowsMSIX {
			t.Fatalf("expected %q, got %q", installKindWindowsMSIX, got)
		}
	})

	t.Run("windows nsis fallback", func(t *testing.T) {
		got := detectDesktopInstallKindFromProbe(desktopInstallProbe{
			GOOS:    "windows",
			ExePath: `C:/Program Files/CrabClaw/CrabClaw.exe`,
			Env:     map[string]string{},
		})
		if got != installKindWindowsNSIS {
			t.Fatalf("expected %q, got %q", installKindWindowsNSIS, got)
		}
	})

	t.Run("linux appimage env", func(t *testing.T) {
		got := detectDesktopInstallKindFromProbe(desktopInstallProbe{
			GOOS:    "linux",
			ExePath: "/tmp/.mount_CrabClaw/usr/bin/CrabClaw",
			Env: map[string]string{
				"APPIMAGE": "/home/user/CrabClaw.AppImage",
			},
		})
		if got != installKindLinuxAppImage {
			t.Fatalf("expected %q, got %q", installKindLinuxAppImage, got)
		}
	})

	t.Run("linux system package fallback", func(t *testing.T) {
		got := detectDesktopInstallKindFromProbe(desktopInstallProbe{
			GOOS:    "linux",
			ExePath: "/usr/bin/crabclaw",
			Env:     map[string]string{},
		})
		if got != installKindLinuxSystemPackage {
			t.Fatalf("expected %q, got %q", installKindLinuxSystemPackage, got)
		}
	})
}
