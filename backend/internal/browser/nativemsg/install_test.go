package nativemsg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndUninstall(t *testing.T) {
	// Use a temp directory as manifest dir.
	tmpDir := t.TempDir()
	origDirs := manifestDirsOverride
	manifestDirsOverride = []string{tmpDir}
	defer func() { manifestDirsOverride = origDirs }()

	n, err := Install(InstallConfig{
		HostBinaryPath: "/usr/local/bin/crabclaw-native-host",
		ExtensionIDs:   []string{"ijkcckheapdhooinidgdccbgabahmgnl"},
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 install, got %d", n)
	}

	// Verify manifest content.
	path := filepath.Join(tmpDir, HostName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var m HostManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if m.Name != HostName {
		t.Errorf("name = %q, want %q", m.Name, HostName)
	}
	if m.Type != "stdio" {
		t.Errorf("type = %q, want stdio", m.Type)
	}
	if m.Path != "/usr/local/bin/crabclaw-native-host" {
		t.Errorf("path = %q, want /usr/local/bin/crabclaw-native-host", m.Path)
	}
	if len(m.AllowedOrigins) != 1 || m.AllowedOrigins[0] != "chrome-extension://ijkcckheapdhooinidgdccbgabahmgnl/" {
		t.Errorf("allowed_origins = %v", m.AllowedOrigins)
	}

	if !IsInstalled() {
		t.Error("IsInstalled() = false after install")
	}

	Uninstall()

	if IsInstalled() {
		t.Error("IsInstalled() = true after uninstall")
	}
}
