package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter(t *testing.T) {
	r := NewRouter("")
	r.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Errorf("got %d %s", w.Code, w.Body.String())
	}
}

func TestRouter_Prefix(t *testing.T) {
	r := NewRouter("/api")
	r.HandleFunc("/v1/test", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("prefixed route: got %d", w.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := CORSMiddleware("*")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// OPTIONS 预检
	w := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
	allowHeaders := w.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{HeaderToken, HeaderLegacyToken, HeaderAgentID, HeaderLegacyAgentID, HeaderAgent, HeaderLegacyAgent} {
		if !strings.Contains(allowHeaders, header) {
			t.Fatalf("CORS allow headers missing %q in %q", header, allowHeaders)
		}
	}

	// 正常请求
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("GET: got %d", w.Code)
	}
}

func TestAuthMiddleware(t *testing.T) {
	handler := AuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	// 无 token
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("no auth: got %d", w.Code)
	}

	// Bearer token
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("valid bearer: got %d", w.Code)
	}

	// X-OpenAcosmi-Token
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderLegacyToken, "secret123")
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("valid header token: got %d", w.Code)
	}

	// X-CrabClaw-Token
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderToken, "secret123")
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("valid new header token: got %d", w.Code)
	}
}

func TestChain(t *testing.T) {
	var order []string
	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1")
			next.ServeHTTP(w, r)
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2")
			next.ServeHTTP(w, r)
		})
	}
	handler := Chain(m1, m2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if len(order) != 3 || order[0] != "m1" || order[1] != "m2" || order[2] != "handler" {
		t.Errorf("chain order = %v", order)
	}
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	if cfg.Port != 3777 {
		t.Errorf("port = %d", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("host = %s", cfg.Host)
	}
}
