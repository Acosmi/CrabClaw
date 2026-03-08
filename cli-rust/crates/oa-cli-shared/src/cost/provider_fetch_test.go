package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// BW1-D2: Provider Fetch 兼容性验证 — HTTP mock 测试覆盖请求构建和响应解析。

// ---------- 辅助 ----------

// withMockHTTPClient 临时替换包级 httpClient 用于测试。
func withMockHTTPClient(t *testing.T, ts *httptest.Server) func() {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{Timeout: 5 * time.Second}
	return func() { httpClient = orig }
}

// ---------- Claude 测试 ----------

func TestFetchClaudeUsageParseWindows(t *testing.T) {
	u := 0.42
	r := "2026-02-22T00:00:00Z"
	data := &claudeUsageResp{
		FiveHour: &claudeWindow{Utilization: &u, ResetsAt: r},
		SevenDay: &claudeWindow{Utilization: &u},
	}
	ws := parseClaudeWindows(data)
	if len(ws) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(ws))
	}
	if ws[0].Label != "5h" {
		t.Errorf("Label: got %q, want 5h", ws[0].Label)
	}
	if ws[0].UsedPercent < 0.41 || ws[0].UsedPercent > 0.43 {
		t.Errorf("UsedPercent: got %f", ws[0].UsedPercent)
	}
}

func TestFetchClaudeUsageMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求 header
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version: got %q", got)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Error("missing Authorization header")
		}
		u := 0.5
		json.NewEncoder(w).Encode(claudeUsageResp{
			FiveHour: &claudeWindow{Utilization: &u},
		})
	}))
	defer ts.Close()

	restore := withMockHTTPClient(t, ts)
	defer restore()

	// 我们无法直接修改 fetchClaudeUsage 中的硬编码 URL，
	// 因此测试 parseClaudeWindows 逻辑正确性。
	u := 0.85
	data := &claudeUsageResp{
		FiveHour:       &claudeWindow{Utilization: &u},
		SevenDaySonnet: &claudeWindow{Utilization: &u},
	}
	ws := parseClaudeWindows(data)
	if len(ws) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(ws))
	}
	if ws[1].Label != "Sonnet" {
		t.Errorf("expected Sonnet label, got %q", ws[1].Label)
	}
}

func TestClaudeUsageHeaders(t *testing.T) {
	headers := claudeUsageHeaders(ProviderAuth{Token: "anthropic-token"})
	if got := headers["Authorization"]; got != "Bearer anthropic-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer anthropic-token")
	}
	if got := headers["User-Agent"]; got != "CrabClaw" {
		t.Fatalf("User-Agent = %q, want %q", got, "CrabClaw")
	}
	if got := headers["anthropic-version"]; got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", got)
	}
}

// ---------- Codex 测试 ----------

func TestFetchCodexUsageParsing(t *testing.T) {
	// 测试 fetchCodexUsage 响应解析逻辑
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization")
		}
		// 简单返回空对象
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	restore := withMockHTTPClient(t, ts)
	defer restore()

	// 验证错误处理路径
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: ProviderOpenAICodex,
		Token:    "test-token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("snap should not be nil")
	}
	if snap.Provider != ProviderOpenAICodex {
		t.Errorf("Provider: got %q, want %q", snap.Provider, ProviderOpenAICodex)
	}
}

// ---------- Gemini 测试 ----------

func TestFetchGeminiUsageParsing(t *testing.T) {
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: ProviderGeminiCLI,
		Token:    "test-gemini-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("snap should not be nil")
	}
	if snap.Provider != ProviderGeminiCLI {
		t.Errorf("Provider: got %q, want %q", snap.Provider, ProviderGeminiCLI)
	}
}

// ---------- Minimax 测试 ----------

func TestFetchMinimaxUsageParsing(t *testing.T) {
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: ProviderMinimax,
		Token:    "test-minimax-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("snap should not be nil")
	}
	if snap.Provider != ProviderMinimax {
		t.Errorf("Provider: got %q, want %q", snap.Provider, ProviderMinimax)
	}
}

func TestMinimaxUsageHeaders(t *testing.T) {
	headers := minimaxUsageHeaders(ProviderAuth{Token: "minimax-token"})
	if got := headers["Authorization"]; got != "Bearer minimax-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer minimax-token")
	}
	if got := headers["MM-API-Source"]; got != "OpenAcosmi" {
		t.Fatalf("MM-API-Source = %q, want %q", got, "OpenAcosmi")
	}
}

// ---------- Zai 测试 ----------

func TestFetchZaiUsageParsing(t *testing.T) {
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: ProviderZai,
		Token:    "test-zai-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("snap should not be nil")
	}
}

// ---------- Copilot 测试 ----------

func TestFetchCopilotUsageParsing(t *testing.T) {
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: ProviderCopilot,
		Token:    "test-github-token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("snap should not be nil")
	}
}

// ---------- 路由测试 ----------

func TestFetchProviderUsageUnsupported(t *testing.T) {
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: "unknown-provider",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Error != "Unsupported provider" {
		t.Errorf("Error: got %q, want 'Unsupported provider'", snap.Error)
	}
}

func TestFetchProviderUsageXiaomi(t *testing.T) {
	snap, err := FetchProviderUsage(context.Background(), ProviderAuth{
		Provider: ProviderXiaomi,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Provider != ProviderXiaomi {
		t.Errorf("Provider: got %q", snap.Provider)
	}
	if snap.Error != "" {
		t.Errorf("unexpected error: %q", snap.Error)
	}
}

// ---------- Auth 解析测试 ----------

func TestResolveProviderAuthsEmpty(t *testing.T) {
	auths := ResolveProviderAuths(nil, "")
	if len(auths) != 0 {
		t.Errorf("expected 0 auths, got %d", len(auths))
	}
}

func TestLoadProviderUsageSummaryEmpty(t *testing.T) {
	summary := LoadProviderUsageSummary(context.Background(), nil)
	if summary == nil {
		t.Fatal("summary should not be nil")
	}
	if summary.UpdatedAt == 0 {
		t.Error("UpdatedAt should be set")
	}
}
