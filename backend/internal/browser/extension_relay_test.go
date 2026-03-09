package browser

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtensionRelayWithCORSAllowsExtensionOrigin(t *testing.T) {
	relay := &ExtensionRelay{
		port:      19004,
		authToken: "secret",
		logger:    slog.Default(),
	}

	handler := relay.withCORS(http.HandlerFunc(relay.handleJSONVersion), AllowedExtensionOrigins, true)
	req := httptest.NewRequest(http.MethodGet, "/json/version", nil)
	req.Header.Set("Origin", "chrome-extension://ijkcckheapdhooinidgdccbgabahmgnl")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "chrome-extension://ijkcckheapdhooinidgdccbgabahmgnl" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestExtensionRelayWithCORSRejectsNonExtensionOrigin(t *testing.T) {
	relay := &ExtensionRelay{
		port:      19004,
		authToken: "secret",
		logger:    slog.Default(),
	}

	handler := relay.withCORS(http.HandlerFunc(relay.handleJSONVersion), AllowedExtensionOrigins, true)
	req := httptest.NewRequest(http.MethodGet, "/json/version", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}
