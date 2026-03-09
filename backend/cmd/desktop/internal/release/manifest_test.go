package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPlatformKey(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantKey  string
		wantOK   bool
	}{
		{
			name:     "macos archive",
			filename: "CrabClaw-macos-amd64.zip",
			wantKey:  "macos-wails-amd64",
			wantOK:   true,
		},
		{
			name:     "windows nsis installer",
			filename: "CrabClaw-windows-arm64-installer.exe",
			wantKey:  "windows-nsis-arm64",
			wantOK:   true,
		},
		{
			name:     "linux appimage",
			filename: "CrabClaw-linux-amd64.AppImage",
			wantKey:  "linux-appimage-amd64",
			wantOK:   true,
		},
		{
			name:     "ignore windows raw binary",
			filename: "CrabClaw-windows-amd64.exe",
			wantKey:  "",
			wantOK:   false,
		},
		{
			name:     "ignore package manager artifact",
			filename: "CrabClaw-linux-amd64.deb",
			wantKey:  "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotOK := DetectPlatformKey(tt.filename)
			if gotKey != tt.wantKey || gotOK != tt.wantOK {
				t.Fatalf("DetectPlatformKey(%q) = (%q, %v), want (%q, %v)", tt.filename, gotKey, gotOK, tt.wantKey, tt.wantOK)
			}
		})
	}
}

func TestGenerateManifest_AutoDetectArtifacts(t *testing.T) {
	dir := t.TempDir()
	macosPath := filepath.Join(dir, "CrabClaw-macos-amd64.zip")
	windowsPath := filepath.Join(dir, "CrabClaw-windows-amd64-installer.exe")
	linuxPath := filepath.Join(dir, "CrabClaw-linux-arm64.AppImage")
	ignoredPath := filepath.Join(dir, "CrabClaw-linux-amd64.deb")

	writeTestArtifact(t, macosPath, "macos-bytes")
	writeTestArtifact(t, windowsPath, "windows-bytes")
	writeTestArtifact(t, linuxPath, "linux-bytes")
	writeTestArtifact(t, ignoredPath, "ignored")

	manifest, checksums, err := GenerateManifest(GenerateOptions{
		Version:      "1.2.3",
		Channel:      "stable",
		BaseURL:      "https://downloads.example.com/releases/1.2.3",
		ArtifactsDir: dir,
		PublishedAt:  "2026-03-09T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	if manifest.Version != "1.2.3" || manifest.Channel != "stable" {
		t.Fatalf("unexpected manifest identity: %+v", manifest)
	}
	if len(manifest.Platforms) != 3 {
		t.Fatalf("expected 3 platforms, got %d", len(manifest.Platforms))
	}
	if _, ok := manifest.Platforms["linux-appimage-arm64"]; !ok {
		t.Fatal("expected linux-appimage-arm64 platform")
	}
	if _, ok := manifest.Platforms["windows-nsis-amd64"]; !ok {
		t.Fatal("expected windows-nsis-amd64 platform")
	}
	if _, ok := manifest.Platforms["macos-wails-amd64"]; !ok {
		t.Fatal("expected macos-wails-amd64 platform")
	}
	if _, ok := manifest.Platforms["linux-system-package-amd64"]; ok {
		t.Fatal("did not expect package-manager artifact to be included")
	}
	if got := manifest.Platforms["macos-wails-amd64"].URL; got != "https://downloads.example.com/releases/1.2.3/CrabClaw-macos-amd64.zip" {
		t.Fatalf("unexpected macOS URL: %q", got)
	}
	if len(checksums) != 3 {
		t.Fatalf("expected 3 checksums, got %d", len(checksums))
	}

	raw, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatalf("MarshalManifest: %v", err)
	}
	var decoded UpdateManifest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal manifest: %v", err)
	}
	if decoded.Platforms["windows-nsis-amd64"].Name != "CrabClaw-windows-amd64-installer.exe" {
		t.Fatalf("unexpected windows artifact name: %q", decoded.Platforms["windows-nsis-amd64"].Name)
	}
}

func TestGenerateManifest_ExplicitArtifactsOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	externalDir := t.TempDir()
	explicitPath := filepath.Join(externalDir, "CrabClaw.pkg")
	writeTestArtifact(t, explicitPath, "macos-pkg")

	manifest, checksums, err := GenerateManifest(GenerateOptions{
		Version:      "2.0.0",
		Channel:      "beta",
		BaseURL:      "https://cdn.example.com/beta",
		ArtifactsDir: dir,
		PublishedAt:  "2026-03-09T13:00:00Z",
		ExplicitArtifacts: []ExplicitArtifact{
			{PlatformKey: "macos-wails-arm64", Path: explicitPath},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	if len(manifest.Platforms) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(manifest.Platforms))
	}
	platform := manifest.Platforms["macos-wails-arm64"]
	if platform.URL != "https://cdn.example.com/beta/CrabClaw.pkg" {
		t.Fatalf("unexpected explicit artifact URL: %q", platform.URL)
	}
	if len(checksums) != 1 || checksums[0].Path != "CrabClaw.pkg" {
		t.Fatalf("unexpected explicit checksums: %+v", checksums)
	}
}

func TestGenerateManifest_ExplicitArtifactsWithoutArtifactsDir(t *testing.T) {
	externalDir := t.TempDir()
	explicitPath := filepath.Join(externalDir, "CrabClaw-macos-arm64.zip")
	writeTestArtifact(t, explicitPath, "macos-zip")

	manifest, checksums, err := GenerateManifest(GenerateOptions{
		Version:      "2.1.0",
		Channel:      "beta",
		BaseURL:      "https://cdn.example.com/beta/2.1.0",
		ArtifactsDir: filepath.Join(t.TempDir(), "missing"),
		ExplicitArtifacts: []ExplicitArtifact{
			{PlatformKey: "macos-wails-arm64", Path: explicitPath},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if len(manifest.Platforms) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(manifest.Platforms))
	}
	if got := manifest.Platforms["macos-wails-arm64"].URL; got != "https://cdn.example.com/beta/2.1.0/CrabClaw-macos-arm64.zip" {
		t.Fatalf("unexpected explicit artifact URL: %q", got)
	}
	if len(checksums) != 1 || checksums[0].Path != "CrabClaw-macos-arm64.zip" {
		t.Fatalf("unexpected explicit checksums: %+v", checksums)
	}
}

func TestGenerateManifest_DuplicatePlatformKeyFails(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "CrabClaw-windows-amd64-installer.exe")
	second := filepath.Join(dir, "CrabClaw-windows-x64-setup.exe")
	writeTestArtifact(t, first, "first")
	writeTestArtifact(t, second, "second")

	_, _, err := GenerateManifest(GenerateOptions{
		Version:      "3.0.0",
		BaseURL:      "https://downloads.example.com/3.0.0",
		ArtifactsDir: dir,
	})
	if err == nil {
		t.Fatal("expected duplicate platform key error")
	}
}

func writeTestArtifact(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
