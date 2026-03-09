package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestServeBrowserExtensionStatusDefaultsToOff(t *testing.T) {
	req := httptest.NewRequest("GET", "/browser-extension/status", nil)
	rec := httptest.NewRecorder()

	serveBrowserExtensionStatus(rec, req, BrowserExtensionHandlerConfig{})

	if rec.Code != 200 {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}

	var info RelayStatusInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if info.Port != 0 {
		t.Fatalf("port = %d, want 0", info.Port)
	}
	if info.RelayURL != "" {
		t.Fatalf("relayUrl = %q, want empty", info.RelayURL)
	}
	if info.Connected {
		t.Fatal("connected = true, want false")
	}
	if info.Token != "" {
		t.Fatalf("token = %q, want empty", info.Token)
	}
}

func TestServeBrowserExtensionStatusUsesLiveRelayInfo(t *testing.T) {
	req := httptest.NewRequest("GET", "/browser-extension/status", nil)
	rec := httptest.NewRecorder()

	serveBrowserExtensionStatus(rec, req, BrowserExtensionHandlerConfig{
		GetRelayInfo: func() *RelayStatusInfo {
			return &RelayStatusInfo{
				Port:      19004,
				Token:     "secret",
				Connected: true,
				RelayURL:  "ws://127.0.0.1:19004/ws",
			}
		},
	})

	if rec.Code != 200 {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}

	var info RelayStatusInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if info.Port != 19004 {
		t.Fatalf("port = %d, want 19004", info.Port)
	}
	if info.Token != "secret" {
		t.Fatalf("token = %q, want %q", info.Token, "secret")
	}
	if !info.Connected {
		t.Fatal("connected = false, want true")
	}
	if info.RelayURL != "ws://127.0.0.1:19004/ws" {
		t.Fatalf("relayUrl = %q, want ws://127.0.0.1:19004/ws", info.RelayURL)
	}
}
