package infra

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ─── JSON File Tests ───

func TestWriteAndReadJSONFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	input := map[string]interface{}{"key": "value", "num": float64(42)}
	if err := WriteJSONFile(path, input); err != nil {
		t.Fatalf("WriteJSONFile: %v", err)
	}

	var output map[string]interface{}
	if err := ReadJSONFile(path, &output); err != nil {
		t.Fatalf("ReadJSONFile: %v", err)
	}
	if output["key"] != "value" {
		t.Errorf("got key=%v", output["key"])
	}
}

func TestReadJSONFileSafeNotExist(t *testing.T) {
	var output map[string]interface{}
	if ReadJSONFileSafe("/nonexistent/path.json", &output) {
		t.Error("expected false for nonexistent file")
	}
}

// ─── Home Dir Tests ───

func TestGetHomeDir(t *testing.T) {
	home := GetHomeDir()
	if home == "" {
		t.Error("expected non-empty home dir")
	}
}

func TestGetXDGDataHome(t *testing.T) {
	dataHome := GetXDGDataHome()
	if dataHome == "" {
		t.Error("expected non-empty XDG data home")
	}
}

func TestGetOpenAcosmiConfigDirPrefersCrabClawEnv(t *testing.T) {
	t.Setenv("OPENACOSMI_CONFIG_DIR", "/tmp/open")
	t.Setenv("CRABCLAW_CONFIG_DIR", "/tmp/crab")
	if got := GetOpenAcosmiConfigDir(); got != "/tmp/crab" {
		t.Fatalf("got %q, want %q", got, "/tmp/crab")
	}
}

func TestGetProjectRootPrefersCrabClawRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENACOSMI_ROOT", t.TempDir())
	t.Setenv("CRABCLAW_ROOT", dir)
	ResetProjectRootForTest()
	defer ResetProjectRootForTest()
	if got := GetProjectRoot(); got != dir {
		t.Fatalf("got %q, want %q", got, dir)
	}
}

// ─── OS Summary Tests ───

func TestGetOsSummary(t *testing.T) {
	summary := GetOsSummary()
	if summary.OS != runtime.GOOS {
		t.Errorf("os: got %q, want %q", summary.OS, runtime.GOOS)
	}
	if summary.Arch != runtime.GOARCH {
		t.Errorf("arch: got %q, want %q", summary.Arch, runtime.GOARCH)
	}
	if summary.GoVer == "" {
		t.Error("expected non-empty Go version")
	}
}

func TestFormatOsSummary(t *testing.T) {
	s := GetOsSummary()
	formatted := FormatOsSummary(s)
	if formatted == "" {
		t.Error("expected non-empty summary")
	}
}

// ─── FS Safe Tests ───

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")

	err := WriteFileAtomic(path, []byte("hello world"), 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("got %q", string(data))
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte(""), 0o644)

	if !FileExists(path) {
		t.Error("expected FileExists=true")
	}
	if FileExists(filepath.Join(dir, "nope.txt")) {
		t.Error("expected FileExists=false for missing file")
	}
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	if !DirExists(dir) {
		t.Error("expected DirExists=true")
	}
	if DirExists(filepath.Join(dir, "nope")) {
		t.Error("expected DirExists=false")
	}
}

// ─── Warning Filter Tests ───

func TestWarningFilter(t *testing.T) {
	f := NewWarningFilter(100)

	// 首次 → true
	if !f.ShouldEmit("warn-1") {
		t.Error("expected true for first emit")
	}
	// 重复 → false
	if f.ShouldEmit("warn-1") {
		t.Error("expected false for duplicate")
	}

	if f.Size() != 1 {
		t.Errorf("size: got %d, want 1", f.Size())
	}

	f.Reset()
	if f.Size() != 0 {
		t.Error("expected size=0 after reset")
	}
}

// ─── Runtime Guard Tests ───

func TestCheckGoVersion(t *testing.T) {
	// 当前版本应满足 1.22
	if err := CheckGoVersion("1.22"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetGoVersionInfo(t *testing.T) {
	info := GetGoVersionInfo()
	if info == "" {
		t.Error("expected non-empty version info")
	}
}

// ─── Path Env Tests ───

func TestFindBinary(t *testing.T) {
	// ls 应该在所有 Unix 系统上可找到
	if runtime.GOOS != "windows" {
		path := FindBinary("ls")
		if path == "" {
			t.Error("expected to find ls")
		}
	}
}

// ─── Shell Env Tests ───

func TestGetDefaultShell(t *testing.T) {
	shell := GetDefaultShell()
	if shell == "" {
		t.Error("expected non-empty shell")
	}
}

// ─── Retry Policy Tests ───

func TestGetRetryPolicy(t *testing.T) {
	policies := []string{"fast", "standard", "patient", "llm", "webhook"}
	for _, name := range policies {
		p := GetRetryPolicy(name)
		if p == nil {
			t.Errorf("expected non-nil policy for %q", name)
		}
	}

	if GetRetryPolicy("nonexistent") != nil {
		t.Error("expected nil for unknown policy")
	}
}
