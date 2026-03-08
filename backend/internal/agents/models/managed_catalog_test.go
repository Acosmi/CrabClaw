package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestManagedModelCatalogList_Normal(t *testing.T) {
	entries := []types.ManagedModelEntry{
		{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o", IsDefault: true},
		{ID: "m2", Name: "Model B", Provider: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong Authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"models": entries})
	}))
	defer srv.Close()

	mc := NewManagedModelCatalog(srv.URL, func() (string, error) { return "test-token", nil })
	result, err := mc.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[0].ID != "m1" || result[1].ID != "m2" {
		t.Errorf("unexpected entries: %+v", result)
	}
}

func TestManagedModelCatalogList_Cache(t *testing.T) {
	var callCount int32
	entries := []types.ManagedModelEntry{
		{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"models": entries})
	}))
	defer srv.Close()

	mc := NewManagedModelCatalog(srv.URL, func() (string, error) { return "t", nil })

	// First call should hit server
	_, err := mc.List()
	if err != nil {
		t.Fatalf("first List() error: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("expected 1 server call, got %d", atomic.LoadInt32(&callCount))
	}

	// Second call should use cache
	_, err = mc.List()
	if err != nil {
		t.Fatalf("second List() error: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("expected still 1 server call (cached), got %d", atomic.LoadInt32(&callCount))
	}
}

func TestManagedModelCatalogList_FailureDegradation(t *testing.T) {
	var callCount int32
	entries := []types.ManagedModelEntry{
		{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n >= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"models": entries})
	}))
	defer srv.Close()

	mc := NewManagedModelCatalog(srv.URL, func() (string, error) { return "t", nil })

	// First call succeeds and populates cache
	result, err := mc.List()
	if err != nil {
		t.Fatalf("first List() error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	// Expire cache
	mc.mu.Lock()
	mc.cacheAt = time.Now().Add(-10 * time.Minute)
	mc.mu.Unlock()

	// Second call fails but returns stale cache
	result, err = mc.List()
	if err != nil {
		t.Fatalf("second List() should return stale cache, got error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 entry from stale cache, got %d", len(result))
	}
}

func TestManagedModelCatalogRefresh(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		entries := []types.ManagedModelEntry{
			{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o"},
		}
		if n >= 2 {
			entries = append(entries, types.ManagedModelEntry{ID: "m2", Name: "Model B", Provider: "anthropic", ModelID: "claude"})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"models": entries})
	}))
	defer srv.Close()

	mc := NewManagedModelCatalog(srv.URL, func() (string, error) { return "t", nil })

	// Initial list
	result, _ := mc.List()
	if len(result) != 1 {
		t.Fatalf("expected 1 entry initially, got %d", len(result))
	}

	// Force refresh
	if err := mc.Refresh(); err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}

	// Should now have 2 entries
	result, _ = mc.List()
	if len(result) != 2 {
		t.Fatalf("expected 2 entries after refresh, got %d", len(result))
	}
}

func TestManagedModelCatalogDefaultModel(t *testing.T) {
	mc := NewManagedModelCatalog("http://unused", func() (string, error) { return "", nil })

	// No cache → nil
	if d := mc.DefaultModel(); d != nil {
		t.Errorf("expected nil DefaultModel with empty cache, got %+v", d)
	}

	// Populate cache
	mc.mu.Lock()
	mc.cache = []types.ManagedModelEntry{
		{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o"},
		{ID: "m2", Name: "Model B", Provider: "anthropic", ModelID: "claude", IsDefault: true},
	}
	mc.cacheAt = time.Now()
	mc.mu.Unlock()

	// Should return the one marked IsDefault
	d := mc.DefaultModel()
	if d == nil || d.ID != "m2" {
		t.Errorf("expected m2 as default, got %+v", d)
	}
}

func TestManagedModelCatalogDefaultModel_FirstIfNoneMarked(t *testing.T) {
	mc := NewManagedModelCatalog("http://unused", func() (string, error) { return "", nil })

	mc.mu.Lock()
	mc.cache = []types.ManagedModelEntry{
		{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o"},
		{ID: "m2", Name: "Model B", Provider: "anthropic", ModelID: "claude"},
	}
	mc.cacheAt = time.Now()
	mc.mu.Unlock()

	// No IsDefault set → return first
	d := mc.DefaultModel()
	if d == nil || d.ID != "m1" {
		t.Errorf("expected m1 as fallback default, got %+v", d)
	}
}

func TestManagedModelCatalogList_DirectArrayResponse(t *testing.T) {
	entries := []types.ManagedModelEntry{
		{ID: "m1", Name: "Model A", Provider: "openai", ModelID: "gpt-4o"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return direct array (not wrapped in {"models": ...})
		json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	mc := NewManagedModelCatalog(srv.URL, func() (string, error) { return "t", nil })
	result, err := mc.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result) != 1 || result[0].ID != "m1" {
		t.Errorf("unexpected result: %+v", result)
	}
}
