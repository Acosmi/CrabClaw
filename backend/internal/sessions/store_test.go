package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")

	store := NewSessionStore(storePath)

	// 1. 空 store 读取
	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected empty store, got %d entries", len(all))
	}

	// 2. 写入
	entries := map[string]*FullSessionEntry{
		"main":            {SessionID: "test-001", UpdatedAt: 1000, DisplayName: "Test Session"},
		"agent:bot1:main": {SessionID: "test-002", UpdatedAt: 2000, ChatType: "direct"},
	}
	if err := store.Save(entries); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 3. 重新读取
	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll after save: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded["main"].DisplayName != "Test Session" {
		t.Errorf("DisplayName = %q, want %q", loaded["main"].DisplayName, "Test Session")
	}
	if loaded["agent:bot1:main"].ChatType != "direct" {
		t.Errorf("ChatType = %q, want %q", loaded["agent:bot1:main"].ChatType, "direct")
	}

	// 4. 文件权限检查 (非 Windows)
	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file perm = %o, want 600", perm)
	}
}

func TestSessionStoreUpdate(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")
	store := NewSessionStore(storePath)

	// 初始写入
	_ = store.Save(map[string]*FullSessionEntry{
		"main": {SessionID: "s1", UpdatedAt: 1000},
	})

	// Update (读-改-写)
	err := store.Update(func(s map[string]*FullSessionEntry) error {
		s["main"].Model = "claude-3"
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// 验证
	all, _ := store.LoadAll()
	if all["main"].Model != "claude-3" {
		t.Errorf("Model = %q after update, want %q", all["main"].Model, "claude-3")
	}
}

func TestSessionStoreGet(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")
	store := NewSessionStore(storePath)

	_ = store.Save(map[string]*FullSessionEntry{
		"main": {SessionID: "s1", UpdatedAt: 1000, DisplayName: "Main"},
	})

	entry, err := store.Get("main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry == nil || entry.DisplayName != "Main" {
		t.Fatalf("Get('main') returned unexpected entry: %+v", entry)
	}

	missing, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get nonexistent: %v", err)
	}
	if missing != nil {
		t.Errorf("Get('nonexistent') should return nil, got %+v", missing)
	}
}

func TestMergeSessionEntry(t *testing.T) {
	// New entry from nil
	entry := MergeSessionEntry(nil, map[string]interface{}{
		"displayName": "Hello",
		"chatType":    "direct",
	})
	if entry.DisplayName != "Hello" {
		t.Errorf("DisplayName = %q, want %q", entry.DisplayName, "Hello")
	}
	if entry.SessionID == "" {
		t.Error("SessionID should be auto-generated")
	}

	// Merge into existing
	existing := &FullSessionEntry{
		SessionID:   "old-id",
		UpdatedAt:   5000,
		DisplayName: "Old",
		ChatType:    "group",
	}
	merged := MergeSessionEntry(existing, map[string]interface{}{
		"displayName": "New Name",
	})
	if merged.SessionID != "old-id" {
		t.Errorf("SessionID should not change: got %q", merged.SessionID)
	}
	if merged.DisplayName != "New Name" {
		t.Errorf("DisplayName = %q, want %q", merged.DisplayName, "New Name")
	}
	if merged.ChatType != "group" {
		t.Errorf("ChatType should be preserved: got %q", merged.ChatType)
	}
}

func TestSessionStoreInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")

	// 写入无效 JSON
	_ = os.WriteFile(storePath, []byte("not-json{{{"), 0o600)

	store := NewSessionStore(storePath)
	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll with invalid JSON should not error: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty store for invalid JSON, got %d entries", len(all))
	}
}

func TestGetSessionCacheTTLPrefersCrabClawEnv(t *testing.T) {
	t.Setenv("OPENACOSMI_SESSION_CACHE_TTL_MS", "2000")
	t.Setenv("CRABCLAW_SESSION_CACHE_TTL_MS", "1500")
	if got := getSessionCacheTTL(); got != 1500*time.Millisecond {
		t.Fatalf("got %s, want %s", got, 1500*time.Millisecond)
	}
}
