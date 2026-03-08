package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatCompletion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}

		resp := map[string]interface{}{
			"id":     "chatcmpl-123",
			"object": "chat.completion",
			"model":  "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"finish_reason": "stop",
					"message": map[string]string{
						"role":    "assistant",
						"content": "Hello!",
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewManagedProxyClient(server.URL, func() (string, error) {
		return "test-token", nil
	})

	req := &ManagedChatRequest{
		ModelID: "gpt-4o",
		Messages: []map[string]string{
			{"role": "user", "content": "Hi"},
		},
	}

	resp, err := provider.ChatCompletion(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected id chatcmpl-123, got %s", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello!" {
		t.Errorf("expected Hello!, got %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %v", resp.Usage)
	}
}

func TestChatCompletion_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid token"}`))
	}))
	defer server.Close()

	provider := NewManagedProxyClient(server.URL, func() (string, error) {
		return "bad-token", nil
	})

	_, err := provider.ChatCompletion(&ManagedChatRequest{ModelID: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsAuthRequired(err) {
		t.Errorf("expected auth required error, got %v", err)
	}
	if IsBalanceInsufficient(err) {
		t.Errorf("should not be balance insufficient")
	}
}

func TestChatCompletion_PaymentRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error": "insufficient balance"}`))
	}))
	defer server.Close()

	provider := NewManagedProxyClient(server.URL, func() (string, error) {
		return "token", nil
	})

	_, err := provider.ChatCompletion(&ManagedChatRequest{ModelID: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsBalanceInsufficient(err) {
		t.Errorf("expected balance insufficient error, got %v", err)
	}
	if IsAuthRequired(err) {
		t.Errorf("should not be auth required")
	}
}

func TestChatCompletion_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	provider := NewManagedProxyClient(server.URL, func() (string, error) {
		return "token", nil
	})

	_, err := provider.ChatCompletion(&ManagedChatRequest{ModelID: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ManagedProviderError)
	if !ok {
		t.Fatalf("expected ManagedProviderError, got %T", err)
	}
	if pe.StatusCode != 500 {
		t.Errorf("expected 500, got %d", pe.StatusCode)
	}
	if !pe.Retryable {
		t.Error("expected retryable=true for 5xx")
	}
}

func TestIsBalanceInsufficient_NonProviderError(t *testing.T) {
	if IsBalanceInsufficient(nil) {
		t.Error("nil should not be balance insufficient")
	}
	if IsBalanceInsufficient(http.ErrAbortHandler) {
		t.Error("arbitrary error should not be balance insufficient")
	}
}

func TestIsAuthRequired_NonProviderError(t *testing.T) {
	if IsAuthRequired(nil) {
		t.Error("nil should not be auth required")
	}
	if IsAuthRequired(http.ErrAbortHandler) {
		t.Error("arbitrary error should not be auth required")
	}
}
