package llmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- Gemini SSE 解析测试 ----------

func TestGeminiStreamChat_TextOnly(t *testing.T) {
	// 构造 Gemini SSE 响应（text-only，两个增量块）
	sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":0,"totalTokenCount":10}}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":" world!"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing x-goog-api-key header")
		}
		// 验证 URL 包含 streamGenerateContent
		if !strings.Contains(r.URL.Path, "streamGenerateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// 验证 alt=sse 查询参数
		if r.URL.Query().Get("alt") != "sse" {
			t.Error("missing alt=sse query param")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	var events []StreamEvent
	result, err := StreamChat(context.Background(), ChatRequest{
		Provider:     "gemini",
		Model:        "gemini-2.0-flash",
		SystemPrompt: "be helpful",
		Messages:     []ChatMessage{TextMessage("user", "hi")},
		APIKey:       "test-key",
		BaseURL:      server.URL + "/v1beta",
	}, func(evt StreamEvent) {
		events = append(events, evt)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop_reason=end_turn, got %q", result.StopReason)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens=10, got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens=5, got %d", result.Usage.OutputTokens)
	}

	// 验证 assistant message
	msg := result.AssistantMessage
	if msg.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "Hello world!" {
		t.Errorf("unexpected content: %+v", msg.Content)
	}

	// 验证流事件
	textEvents := 0
	for _, evt := range events {
		if evt.Type == EventText {
			textEvents++
		}
	}
	if textEvents != 2 {
		t.Errorf("expected 2 text events, got %d", textEvents)
	}
}

func TestGeminiStreamChat_FunctionCall(t *testing.T) {
	// Gemini functionCall: 完整的 functionCall 在单个 part 中返回
	sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Let me check."}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"command":"ls -la"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":15,"totalTokenCount":35}}

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	var toolStartCount int
	result, err := StreamChat(context.Background(), ChatRequest{
		Provider: "gemini",
		Model:    "gemini-2.0-flash",
		Messages: []ChatMessage{TextMessage("user", "list files")},
		APIKey:   "test-key",
		BaseURL:  server.URL + "/v1beta",
	}, func(evt StreamEvent) {
		if evt.Type == EventToolUseStart {
			toolStartCount++
		}
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toolStartCount != 1 {
		t.Errorf("expected 1 tool_use_start event, got %d", toolStartCount)
	}

	// 验证 content blocks: text + tool_use
	msg := result.AssistantMessage
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "text" || msg.Content[0].Text != "Let me check." {
		t.Errorf("unexpected first block: %+v", msg.Content[0])
	}
	if msg.Content[1].Type != "tool_use" || msg.Content[1].Name != "bash" {
		t.Errorf("unexpected tool_use block: %+v", msg.Content[1])
	}

	// 验证 tool input
	var input map[string]string
	if err := json.Unmarshal(msg.Content[1].Input, &input); err != nil {
		t.Fatalf("failed to unmarshal tool input: %v", err)
	}
	if input["command"] != "ls -la" {
		t.Errorf("expected command='ls -la', got %q", input["command"])
	}
}

func TestGeminiStreamChat_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}`)
	}))
	defer server.Close()

	_, err := StreamChat(context.Background(), ChatRequest{
		Provider: "gemini",
		Model:    "gemini-2.0-flash",
		Messages: []ChatMessage{TextMessage("user", "hi")},
		APIKey:   "test-key",
		BaseURL:  server.URL + "/v1beta",
	}, nil)

	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsRateLimit() {
		t.Error("expected rate limit error")
	}
	if !apiErr.Retryable {
		t.Error("expected retryable")
	}
	if apiErr.Type != "RESOURCE_EXHAUSTED" {
		t.Errorf("expected type RESOURCE_EXHAUSTED, got %q", apiErr.Type)
	}
}

// ---------- Gemini 消息转换测试 ----------

func TestToGeminiContents_BasicConversation(t *testing.T) {
	msgs := []ChatMessage{
		TextMessage("system", "be helpful"),  // 应被跳过
		TextMessage("user", "hello"),         // user → user
		TextMessage("assistant", "hi there"), // assistant → model
		TextMessage("user", "how are you?"),  // user → user
	}

	contents := toGeminiContents(msgs)
	if len(contents) != 3 {
		t.Fatalf("expected 3 contents (system skipped), got %d", len(contents))
	}

	if contents[0].Role != "user" || contents[0].Parts[0].Text != "hello" {
		t.Errorf("unexpected first content: %+v", contents[0])
	}
	if contents[1].Role != "model" || contents[1].Parts[0].Text != "hi there" {
		t.Errorf("unexpected second content: %+v", contents[1])
	}
	if contents[2].Role != "user" || contents[2].Parts[0].Text != "how are you?" {
		t.Errorf("unexpected third content: %+v", contents[2])
	}
}

func TestToGeminiContents_ToolUseConversion(t *testing.T) {
	msgs := []ChatMessage{
		TextMessage("user", "run ls"),
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "text", Text: "Sure"},
				{Type: "tool_use", ID: "call_1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
			},
		},
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "tool_result", Name: "bash", ToolUseID: "call_1", ResultText: "file1.txt"},
			},
		},
	}

	contents := toGeminiContents(msgs)
	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	// assistant message → model with text + functionCall
	modelMsg := contents[1]
	if modelMsg.Role != "model" {
		t.Errorf("expected model role, got %q", modelMsg.Role)
	}
	if len(modelMsg.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(modelMsg.Parts))
	}
	if modelMsg.Parts[0].Text != "Sure" {
		t.Errorf("unexpected text part: %q", modelMsg.Parts[0].Text)
	}
	if modelMsg.Parts[1].FunctionCall == nil || modelMsg.Parts[1].FunctionCall.Name != "bash" {
		t.Errorf("unexpected functionCall part: %+v", modelMsg.Parts[1])
	}

	// tool_result → user with functionResponse
	toolMsg := contents[2]
	if toolMsg.Role != "user" {
		t.Errorf("expected user role for tool_result, got %q", toolMsg.Role)
	}
	if len(toolMsg.Parts) != 1 || toolMsg.Parts[0].FunctionResp == nil {
		t.Fatalf("expected 1 functionResponse part, got %+v", toolMsg.Parts)
	}
	if toolMsg.Parts[0].FunctionResp.Name != "bash" {
		t.Errorf("unexpected functionResponse name: %q", toolMsg.Parts[0].FunctionResp.Name)
	}
}

func TestToGeminiContents_UserImageConversion(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "text", Text: "describe"},
				{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "ZmFrZQ==",
					},
				},
			},
		},
	}

	contents := toGeminiContents(msgs)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	if len(contents[0].Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(contents[0].Parts))
	}
	if contents[0].Parts[0].Text != "describe" {
		t.Fatalf("unexpected text part: %+v", contents[0].Parts[0])
	}
	if contents[0].Parts[1].InlineData == nil {
		t.Fatalf("expected inlineData part, got %+v", contents[0].Parts[1])
	}
	if contents[0].Parts[1].InlineData.MimeType != "image/png" || contents[0].Parts[1].InlineData.Data != "ZmFrZQ==" {
		t.Fatalf("unexpected inlineData: %+v", contents[0].Parts[1].InlineData)
	}
}

// S1-4: 验证 tool_result Name 为空时的 fallback 行为
func TestToGeminiContents_ToolResultNameFallback(t *testing.T) {
	msgs := []ChatMessage{
		TextMessage("user", "run ls"),
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "tool_use", ID: "call_1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
			},
		},
		{
			Role: "user",
			Content: []ContentBlock{
				// Name 字段故意留空 — 模拟旧版 attempt_runner 输出
				{Type: "tool_result", ToolUseID: "call_1", ResultText: "file1.txt"},
			},
		},
	}

	contents := toGeminiContents(msgs)
	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	toolMsg := contents[2]
	if len(toolMsg.Parts) != 1 || toolMsg.Parts[0].FunctionResp == nil {
		t.Fatalf("expected 1 functionResponse part, got %+v", toolMsg.Parts)
	}
	// 验证 Name fallback 为 "unknown_function" 而非空字符串
	if toolMsg.Parts[0].FunctionResp.Name != "unknown_function" {
		t.Errorf("expected Name fallback='unknown_function', got %q", toolMsg.Parts[0].FunctionResp.Name)
	}
	if toolMsg.Parts[0].FunctionResp.Response["result"] != "file1.txt" {
		t.Errorf("expected result='file1.txt', got %v", toolMsg.Parts[0].FunctionResp.Response["result"])
	}
}

// ---------- Gemini Provider 路由测试 ----------

func TestStreamChat_GeminiProviderRouting(t *testing.T) {
	providers := []string{"gemini", "google", "google-gemini", "google-gemini-cli",
		"google-generative-ai", "google-antigravity"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}

`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, sseData)
			}))
			defer server.Close()

			result, err := StreamChat(context.Background(), ChatRequest{
				Provider: provider,
				Model:    "gemini-2.0-flash",
				Messages: []ChatMessage{TextMessage("user", "hi")},
				APIKey:   "test-key",
				BaseURL:  server.URL + "/v1beta",
			}, nil)

			if err != nil {
				t.Fatalf("provider %q: unexpected error: %v", provider, err)
			}
			if result.AssistantMessage.Content[0].Text != "ok" {
				t.Errorf("provider %q: unexpected text: %q", provider, result.AssistantMessage.Content[0].Text)
			}
		})
	}
}

func TestIsGeminiCompatible(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://generativelanguage.googleapis.com/v1beta", true},
		{"https://aiplatform.googleapis.com/v1", true},
		{"https://my-gemini-proxy.example.com", true},
		{"https://api.openai.com/v1", false},
		{"https://api.anthropic.com", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isGeminiCompatible(tt.url)
		if got != tt.want {
			t.Errorf("isGeminiCompatible(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}
