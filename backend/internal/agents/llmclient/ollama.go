package llmclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------- Ollama Chat API 流式客户端 ----------
// 对齐 TS: @mariozechner/pi-ai (ollama provider)
// API 文档: https://github.com/ollama/ollama/blob/main/docs/api.md

const defaultOllamaBaseURL = "http://localhost:11434"

// ollamaStreamChat 调用 Ollama /api/chat (NDJSON 流式)。
func ollamaStreamChat(ctx context.Context, req ChatRequest, onEvent func(StreamEvent)) (*ChatResult, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/chat"

	body := ollamaRequest{
		Model:    req.Model,
		Stream:   true,
		Messages: toOllamaMessages(req.SystemPrompt, req.Messages),
	}
	if req.Temperature != nil {
		body.Options.Temperature = req.Temperature
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal ollama request: %w", err)
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Minute // Ollama 本地推理可能更慢
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient: ollama HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseOllamaError(resp)
	}

	return parseOllamaNDJSON(resp.Body, onEvent)
}

// ---------- Ollama 请求结构 ----------

type ollamaRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages []ollamaMessage `json:"messages"`
	Options  struct {
		Temperature *float64 `json:"temperature,omitempty"`
	} `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ---------- 消息转换 ----------

const ollamaUnsupportedImageNotice = "[用户附带了图片，但当前 Ollama 客户端仅支持文本聊天，图片未发送给模型。]"

func toOllamaMessages(systemPrompt string, msgs []ChatMessage) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(msgs)+1)
	if systemPrompt != "" {
		out = append(out, ollamaMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		var text strings.Builder
		hasImage := false
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				text.WriteString(b.Text)
			case "image":
				if b.Source != nil && b.Source.Data != "" {
					hasImage = true
				}
			}
		}
		if hasImage {
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(ollamaUnsupportedImageNotice)
		}
		out = append(out, ollamaMessage{Role: m.Role, Content: text.String()})
	}
	return out
}

// ---------- NDJSON 解析 ----------

func parseOllamaNDJSON(r io.Reader, onEvent func(StreamEvent)) (*ChatResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		result  ChatResult
		textBuf strings.Builder
	)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done            bool `json:"done"`
			PromptEvalCount int  `json:"prompt_eval_count"`
			EvalCount       int  `json:"eval_count"`
		}
		if json.Unmarshal(line, &chunk) != nil {
			continue
		}

		if chunk.Message.Content != "" {
			textBuf.WriteString(chunk.Message.Content)
			onEvent(StreamEvent{Type: EventText, Text: chunk.Message.Content})
		}

		if chunk.Done {
			result.Usage.InputTokens = chunk.PromptEvalCount
			result.Usage.OutputTokens = chunk.EvalCount
			result.StopReason = "end_turn"
			onEvent(StreamEvent{Type: EventStop, StopReason: "end_turn"})
			onEvent(StreamEvent{Type: EventUsage, Usage: &result.Usage})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("llmclient: ollama NDJSON scan: %w", err)
	}

	result.AssistantMessage = ChatMessage{
		Role:    "assistant",
		Content: []ContentBlock{{Type: "text", Text: textBuf.String()}},
	}
	return &result, nil
}

// ---------- 错误解析 ----------

func parseOllamaError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Type:       "ollama_error",
		Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
	}
	apiErr.Retryable = resp.StatusCode >= 500
	return apiErr
}
