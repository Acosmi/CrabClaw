package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeriveNexusBaseURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://nexus.example.com/api/v4/models/proxy", "https://nexus.example.com/api/v4"},
		{"http://localhost:8080/api/v4/models/proxy", "http://localhost:8080/api/v4"},
		{"https://host/api/v4/", "https://host/api/v4"},
		{"https://host/api/v4/wallet/stats", "https://host/api/v4"},
		{"https://host/no-match", ""},
	}

	for _, tt := range tests {
		result := deriveNexusBaseURL(tt.input)
		if result != tt.expected {
			t.Errorf("deriveNexusBaseURL(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFetchNexusAPI_WrappedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token")
		}
		resp := map[string]interface{}{
			"code":    0,
			"message": "success",
			"data": map[string]interface{}{
				"balance":            42.5,
				"monthlyConsumption": 10.0,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tokenProvider := func() (string, error) { return "test-token", nil }
	result, err := fetchNexusAPI(server.URL, tokenProvider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if data["balance"] != 42.5 {
		t.Errorf("expected balance 42.5, got %v", data["balance"])
	}
}

func TestFetchNexusAPI_DirectResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"balance": 100.0,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tokenProvider := func() (string, error) { return "t", nil }
	result, err := fetchNexusAPI(server.URL, tokenProvider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if data["balance"] != 100.0 {
		t.Errorf("expected balance 100.0, got %v", data["balance"])
	}
}

func TestFetchNexusAPI_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	tokenProvider := func() (string, error) { return "t", nil }
	_, err := fetchNexusAPI(server.URL, tokenProvider)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}
