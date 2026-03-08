package email

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateStore_SaveLoad(t *testing.T) {
	tmp := t.TempDir()
	store := NewStateStore(tmp, "test-account")

	// Load 不存在的文件 → nil, nil
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if state != nil {
		t.Fatal("Load empty should return nil")
	}

	// Save
	now := time.Now().Truncate(time.Second)
	original := &AccountState{
		UIDValidity:         12345,
		LastSeenUID:         999,
		LastProcessedMsgID:  "<msg-001@example.com>",
		LastSuccessAt:       now,
		ConsecutiveFailures: 0,
		CurrentMode:         "idle",
		LastIdleResetAt:     now.Add(-5 * time.Minute),
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 文件应该存在
	expectedPath := filepath.Join(tmp, "email", "test-account", "state.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("State file not found at %s: %v", expectedPath, err)
	}

	// Load 回来
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load should return non-nil")
	}
	if loaded.UIDValidity != 12345 {
		t.Errorf("UIDValidity = %d, want 12345", loaded.UIDValidity)
	}
	if loaded.LastSeenUID != 999 {
		t.Errorf("LastSeenUID = %d, want 999", loaded.LastSeenUID)
	}
	if loaded.LastProcessedMsgID != "<msg-001@example.com>" {
		t.Errorf("LastProcessedMsgID = %q, want %q", loaded.LastProcessedMsgID, "<msg-001@example.com>")
	}
	if loaded.CurrentMode != "idle" {
		t.Errorf("CurrentMode = %q, want %q", loaded.CurrentMode, "idle")
	}
}

func TestStateStore_Overwrite(t *testing.T) {
	tmp := t.TempDir()
	store := NewStateStore(tmp, "overwrite-test")

	// Save first version
	v1 := &AccountState{UIDValidity: 100, LastSeenUID: 50}
	if err := store.Save(v1); err != nil {
		t.Fatalf("Save v1: %v", err)
	}

	// Save second version
	v2 := &AccountState{UIDValidity: 100, LastSeenUID: 200}
	if err := store.Save(v2); err != nil {
		t.Fatalf("Save v2: %v", err)
	}

	// Load should return v2
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.LastSeenUID != 200 {
		t.Errorf("LastSeenUID = %d, want 200", loaded.LastSeenUID)
	}
}

func TestStateStore_CorruptFile(t *testing.T) {
	tmp := t.TempDir()
	store := NewStateStore(tmp, "corrupt")

	// 写入无效 JSON
	dir := filepath.Join(tmp, "email", "corrupt")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load()
	if err == nil {
		t.Fatal("Load corrupt should error")
	}
}

func TestStateStore_FilePath(t *testing.T) {
	store := NewStateStore("/data/store", "my-account")
	expected := filepath.Join("/data/store", "email", "my-account", "state.json")
	if store.FilePath() != expected {
		t.Errorf("FilePath = %q, want %q", store.FilePath(), expected)
	}
}

// --- L4 修复验证: sanitizeAccountID 路径穿越防护 ---

func TestSanitizeAccountID_Valid(t *testing.T) {
	valid := []string{"ali", "qq", "test-account", "my_acct", "account.work", "abc123"}
	for _, id := range valid {
		result, err := sanitizeAccountID(id)
		if err != nil {
			t.Errorf("sanitizeAccountID(%q) unexpected error: %v", id, err)
		}
		if result != id {
			t.Errorf("sanitizeAccountID(%q) = %q, want %q", id, result, id)
		}
	}
}

func TestSanitizeAccountID_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"../escape",
		"../../etc/passwd",
		"dir/sub",
		"dir\\sub",
		"foo/../bar",
	}
	for _, id := range invalid {
		_, err := sanitizeAccountID(id)
		if err == nil {
			t.Errorf("sanitizeAccountID(%q) should fail, but got nil error", id)
		}
	}
}

func TestNewStateStore_PathTraversal(t *testing.T) {
	store := NewStateStore("/data/store", "../escape")
	// filePath 应为空（返回降级的空 store）
	if store.FilePath() != "" {
		t.Errorf("Path traversal accountID should result in empty filePath, got %q", store.FilePath())
	}
}

func TestNewThreadContextStore_PathTraversal(t *testing.T) {
	tcs := NewThreadContextStore("/data/store", "../escape")
	// dir 应为空
	if tcs.dir != "" {
		t.Errorf("Path traversal accountID should result in empty dir, got %q", tcs.dir)
	}
}
