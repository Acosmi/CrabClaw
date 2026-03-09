package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBundle_SucceedsForGeneratedBundle(t *testing.T) {
	dir := t.TempDir()
	writeTestArtifact(t, filepath.Join(dir, "CrabClaw-macos-amd64.zip"), "mac")
	writeTestArtifact(t, filepath.Join(dir, "CrabClaw-windows-amd64-installer.exe"), "win")
	writeTestArtifact(t, filepath.Join(dir, "CrabClaw-linux-amd64.AppImage"), "linux")

	manifest, checksums, err := GenerateManifest(GenerateOptions{
		Version:      "9.1.0",
		Channel:      "stable",
		BaseURL:      "https://downloads.example.com/releases/9.1.0",
		ArtifactsDir: dir,
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	manifestPath := filepath.Join(dir, "update.json")
	checksumsPath := filepath.Join(dir, "SHA256SUMS")
	writeManifestFile(t, manifestPath, manifest)
	if err := os.WriteFile(checksumsPath, MarshalChecksums(checksums), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	if err := ValidateBundle(ValidateOptions{
		ManifestPath:    manifestPath,
		ChecksumsPath:   checksumsPath,
		ArtifactsDir:    dir,
		ExpectedVersion: "9.1.0",
		ExpectedChannel: "stable",
	}); err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}
}

func TestValidateBundle_FailsOnChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	writeTestArtifact(t, filepath.Join(dir, "CrabClaw-windows-amd64-installer.exe"), "win")

	manifest, checksums, err := GenerateManifest(GenerateOptions{
		Version:      "9.2.0",
		Channel:      "beta",
		BaseURL:      "https://downloads.example.com/releases/9.2.0",
		ArtifactsDir: dir,
		ExplicitArtifacts: []ExplicitArtifact{
			{PlatformKey: "windows-nsis-amd64", Path: filepath.Join(dir, "CrabClaw-windows-amd64-installer.exe")},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	checksums[0].SHA256 = strings.Repeat("a", 64)
	manifestPath := filepath.Join(dir, "update.json")
	checksumsPath := filepath.Join(dir, "SHA256SUMS")
	writeManifestFile(t, manifestPath, manifest)
	if err := os.WriteFile(checksumsPath, MarshalChecksums(checksums), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	err = ValidateBundle(ValidateOptions{
		ManifestPath:    manifestPath,
		ChecksumsPath:   checksumsPath,
		ArtifactsDir:    dir,
		ExpectedVersion: "9.2.0",
		ExpectedChannel: "beta",
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func writeManifestFile(t *testing.T, path string, manifest UpdateManifest) {
	t.Helper()
	raw, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatalf("MarshalManifest: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
