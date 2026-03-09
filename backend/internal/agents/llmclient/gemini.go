package llmclient

// ---------- Google Gemini REST API 流式客户端 ----------
// 对齐 TS: @mariozechner/pi-ai streamSimple (Google provider)
// API 文档: https://ai.google.dev/api/generate-content
//
// Gemini SSE 格式:
//   URL: POST .../models/{model}:streamGenerateContent?alt=sse
//   认证: x-goog-api-key header
//   每行: data: {"candidates":[...],"usageMetadata":{...}}
//
// 与 OpenAI SSE 的关键差异:
//   1. 无 [DONE] 终止符 — 流结束时连接关闭
//   2. candidates[].content.parts[] 可包含多种类型 (text/functionCall)
//   3. finishReason 在 candidates[0] 中，非 choices[0].finish_reason
//   4. usageMetadata 有 promptTokenCount / candidatesTokenCount

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// geminiStreamChat 调用 Gemini generateContent API (流式)。
func geminiStreamChat(ctx context.Context, req ChatRequest, onEvent func(StreamEvent)) (*ChatResult, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	// Gemini 端点: POST /models/{model}:streamGenerateContent?alt=sse
	endpoint := strings.TrimRight(baseURL, "/") +
		"/models/" + req.Model + ":streamGenerateContent?alt=sse"

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	body := geminiRequest{
		GenerationConfig: geminiGenerationConfig{
			MaxOutputTokens: maxTokens,
		},
	}

	// System instruction (Gemini 顶层字段)
	if req.SystemPrompt != "" {
		body.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.SystemPrompt}},
		}
	}

	// 消息转换
	body.Contents = toGeminiContents(req.Messages)

	// 工具定义
	if len(req.Tools) > 0 {
		body.Tools = toGeminiTools(req.Tools)
	}

	// 温度
	if req.Temperature != nil {
		body.GenerationConfig.Temperature = req.Temperature
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal gemini request: %w", err)
	}

	// ===== 诊断日志：请求大小 =====
	// Bug#11: fmt.Fprintf → slog.Debug，避免与 slog 输出并发交错
	slog.Debug("gemini request",
		"subsystem", "gemini-diag",
		"endpoint", endpoint,
		"bodySize", len(bodyBytes),
		"systemPromptLen", len(req.SystemPrompt),
		"messageCount", len(req.Messages),
		"toolCount", len(req.Tools),
		"model", req.Model,
	)
	// 写请求体到临时文件（方便离线检查）
	if tmpFile, tmpErr := os.CreateTemp("", "gemini-req-*.json"); tmpErr == nil {
		tmpFile.Write(bodyBytes)
		tmpFile.Close()
		slog.Debug("gemini request body dumped", "subsystem", "gemini-diag", "path", tmpFile.Name())
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	// 网络瞬时错误自动重试（unexpected EOF, connection reset 等）
	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			retryDelay := time.Duration(attempt) * 2 * time.Second
			slog.Debug("gemini retry", "subsystem", "gemini-diag", "attempt", attempt, "maxRetries", maxRetries, "delay", retryDelay, "prevError", lastErr)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("llmclient: gemini request cancelled: %w", ctx.Err())
			case <-time.After(retryDelay):
			}
		}

		attemptCtx, attemptCancel := context.WithTimeout(ctx, timeout)

		httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			attemptCancel()
			return nil, fmt.Errorf("llmclient: create gemini request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		// OAuth token 使用 Bearer 认证; API key 使用 x-goog-api-key header
		if req.AuthMode == "oauth" {
			httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
		} else {
			httpReq.Header.Set("x-goog-api-key", req.APIKey)
		}

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			attemptCancel()
			slog.Debug("gemini HTTP error", "subsystem", "gemini-diag", "attempt", attempt, "error", err)
			// 判断是否为可重试的瞬时错误
			errStr := err.Error()
			if strings.Contains(errStr, "unexpected EOF") ||
				strings.Contains(errStr, "connection reset") ||
				strings.Contains(errStr, "broken pipe") ||
				strings.Contains(errStr, "connection refused") {
				lastErr = err
				continue // 重试
			}
			return nil, fmt.Errorf("llmclient: gemini HTTP error: %w", err)
		}

		slog.Debug("gemini HTTP status", "subsystem", "gemini-diag", "status", resp.StatusCode, "attempt", attempt)

		if resp.StatusCode != http.StatusOK {
			apiErr := parseGeminiError(resp)
			resp.Body.Close()
			attemptCancel()
			// 5xx 服务器错误可重试
			if resp.StatusCode >= 500 && attempt < maxRetries {
				lastErr = apiErr
				continue
			}
			return nil, apiErr
		}

		result, parseErr := parseGeminiSSE(resp.Body, onEvent)
		resp.Body.Close()
		attemptCancel()
		return result, parseErr
	}

	return nil, fmt.Errorf("llmclient: gemini request failed after %d retries: %w", maxRetries+1, lastErr)
}

// ---------- Gemini 请求结构 ----------

type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Tools             []geminiToolDef        `json:"tools,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	InlineData       *geminiInlineData   `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResp     *geminiFunctionResp `json:"functionResponse,omitempty"`
	ThoughtSignature string              `json:"thoughtSignature,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type geminiFunctionResp struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiToolDef struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

// ---------- Gemini SSE 响应结构 ----------

type geminiSSEResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ---------- 消息转换 ----------

// toGeminiContents 将统一 ChatMessage 转换为 Gemini Contents 格式。
// Gemini 角色: "user" 和 "model"（非 "assistant"）。
// geminiToolResultText 提取 tool_result 的文本内容（Gemini 不支持 image tool results）。
func geminiToolResultText(b ContentBlock) string {
	if b.ResultText != "" {
		return b.ResultText
	}
	if len(b.ResultBlocks) > 0 {
		var parts []string
		for _, rb := range b.ResultBlocks {
			if rb.Type == "text" && rb.Text != "" {
				parts = append(parts, rb.Text)
			} else if rb.Type == "image" {
				parts = append(parts, "[image attached]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func toGeminiContents(msgs []ChatMessage) []geminiContent {
	out := make([]geminiContent, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue // system 走 systemInstruction
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		gc := geminiContent{Role: role}
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if b.Text != "" {
					gc.Parts = append(gc.Parts, geminiPart{Text: b.Text})
				}
			case "image":
				if b.Source != nil && b.Source.Data != "" && b.Source.MediaType != "" {
					gc.Parts = append(gc.Parts, geminiPart{
						InlineData: &geminiInlineData{
							MimeType: b.Source.MediaType,
							Data:     b.Source.Data,
						},
					})
				}
			case "tool_use":
				// assistant tool_use → Gemini functionCall
				var args map[string]interface{}
				if len(b.Input) > 0 {
					_ = json.Unmarshal(b.Input, &args)
				}
				gc.Parts = append(gc.Parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: b.Name,
						Args: args,
					},
					ThoughtSignature: b.ThinkingSignature,
				})
			case "tool_result":
				// tool_result → Gemini functionResponse
				// S1-4: Name 字段 fallback — 确保不为空
				fnName := b.Name
				if fnName == "" {
					fnName = "unknown_function"
				}
				gc.Parts = append(gc.Parts, geminiPart{
					FunctionResp: &geminiFunctionResp{
						Name: fnName,
						Response: map[string]interface{}{
							"result": geminiToolResultText(b),
						},
					},
				})
			}
		}

		if len(gc.Parts) > 0 {
			out = append(out, gc)
		}
	}
	return out
}

// toGeminiTools 将 ToolDef 转换为 Gemini FunctionDeclarations 格式。
// 自动清洗 schema 以兼容 Gemini API 严格校验。
func toGeminiTools(tools []ToolDef) []geminiToolDef {
	decls := make([]geminiFunctionDecl, len(tools))
	for i, t := range tools {
		decls[i] = geminiFunctionDecl{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  fixGeminiSchema(t.InputSchema),
		}
	}
	return []geminiToolDef{{FunctionDeclarations: decls}}
}

// geminiUnsupportedKeywords Gemini API 不支持的 JSON Schema 关键字。
var geminiUnsupportedKeywords = map[string]bool{
	"patternProperties": true, "additionalProperties": true,
	"$schema": true, "$id": true, "$ref": true, "$defs": true, "definitions": true,
	"examples": true, "minLength": true, "maxLength": true,
	"minimum": true, "maximum": true, "multipleOf": true,
	"pattern": true, "format": true, "minItems": true, "maxItems": true,
	"uniqueItems": true, "minProperties": true, "maxProperties": true,
}

// fixGeminiSchema 递归清洗 JSON Schema 以兼容 Gemini API。
// 1. 移除不支持的关键字
// 2. 为 type=array 属性补全缺失的 items 字段（Gemini 强制要求）
func fixGeminiSchema(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 {
		return schema
	}
	var raw interface{}
	if err := json.Unmarshal(schema, &raw); err != nil {
		return schema
	}
	fixed := fixGeminiSchemaRecursive(raw)
	result, err := json.Marshal(fixed)
	if err != nil {
		return schema
	}
	return result
}

func fixGeminiSchemaRecursive(v interface{}) interface{} {
	switch val := v.(type) {
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = fixGeminiSchemaRecursive(item)
		}
		return out
	case map[string]interface{}:
		return fixGeminiSchemaObject(val)
	default:
		return v
	}
}

func fixGeminiSchemaObject(obj map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})
	for key, value := range obj {
		if geminiUnsupportedKeywords[key] {
			continue
		}
		switch key {
		case "properties":
			if props, ok := value.(map[string]interface{}); ok {
				cp := make(map[string]interface{})
				for k, pv := range props {
					cp[k] = fixGeminiSchemaRecursive(pv)
				}
				cleaned[key] = cp
			} else {
				cleaned[key] = value
			}
		case "items":
			cleaned[key] = fixGeminiSchemaRecursive(value)
		case "anyOf", "oneOf", "allOf":
			if arr, ok := value.([]interface{}); ok {
				out := make([]interface{}, len(arr))
				for i, item := range arr {
					out[i] = fixGeminiSchemaRecursive(item)
				}
				cleaned[key] = out
			} else {
				cleaned[key] = value
			}
		default:
			cleaned[key] = value
		}
	}

	// Gemini 强制要求: type=array 必须有 items 字段
	if t, ok := cleaned["type"]; ok {
		if ts, ok := t.(string); ok && ts == "array" {
			if _, hasItems := cleaned["items"]; !hasItems {
				cleaned["items"] = map[string]interface{}{"type": "string"}
			}
		}
	}

	return cleaned
}

// ---------- SSE 解析 ----------

// geminiSSEErrorResponse Gemini SSE 内嵌错误格式。
type geminiSSEErrorResponse struct {
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// parseGeminiSSE 解析 Gemini SSE 流。
// Gemini SSE 格式: 每行 "data: {JSON}" + 空行分隔。无 [DONE] 终止符。
func parseGeminiSSE(r io.Reader, onEvent func(StreamEvent)) (*ChatResult, error) {
	scanner := bufio.NewScanner(r)
	// Gemini 可能返回较大的 JSON 块（含完整 functionCall 参数）
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024) // 2MB max line

	var (
		result    ChatResult
		textBuf   strings.Builder
		toolCalls []ContentBlock
		dataLines int // SSE data 行计数
		parseErrs int // JSON 解析失败计数
		sseErrMsg string
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}
		dataLines++

		// 调试: 记录前 2 个 SSE data 行，帮助诊断空响应问题
		if dataLines <= 2 {
			preview := data
			if len(preview) > 500 {
				preview = preview[:500] + "...(truncated)"
			}
			slog.Debug("gemini SSE data line", "lineNum", dataLines, "preview", preview)
		}

		// 检测 Gemini 内嵌错误响应（SSE 200 但内容是 error）
		var errResp geminiSSEErrorResponse
		if json.Unmarshal([]byte(data), &errResp) == nil && errResp.Error != nil {
			sseErrMsg = fmt.Sprintf("Gemini SSE error %d (%s): %s",
				errResp.Error.Code, errResp.Error.Status, errResp.Error.Message)
			onEvent(StreamEvent{Type: EventError, Error: sseErrMsg})
			continue
		}

		var resp geminiSSEResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			parseErrs++
			continue
		}

		// 处理 usageMetadata
		if resp.UsageMetadata != nil {
			result.Usage.InputTokens = resp.UsageMetadata.PromptTokenCount
			result.Usage.OutputTokens = resp.UsageMetadata.CandidatesTokenCount
			onEvent(StreamEvent{Type: EventUsage, Usage: &result.Usage})
		}

		// 处理 candidates
		if len(resp.Candidates) == 0 {
			continue
		}
		candidate := resp.Candidates[0]

		// 逐 part 处理
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textBuf.WriteString(part.Text)
				onEvent(StreamEvent{Type: EventText, Text: part.Text})
			}

			if part.FunctionCall != nil {
				// Gemini functionCall 在单个 part 中完整返回（非增量）
				callID := fmt.Sprintf("gemini_call_%d", len(toolCalls))
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)

				toolCalls = append(toolCalls, ContentBlock{
					Type:              "tool_use",
					ID:                callID,
					Name:              part.FunctionCall.Name,
					Input:             json.RawMessage(argsJSON),
					ThinkingSignature: part.ThoughtSignature,
				})

				onEvent(StreamEvent{
					Type: EventToolUseStart,
					ToolUse: &ToolUseEvent{
						ID:   callID,
						Name: part.FunctionCall.Name,
					},
				})
				onEvent(StreamEvent{
					Type: EventToolUseInput,
					ToolUse: &ToolUseEvent{
						InputDelta: string(argsJSON),
						InputFull:  argsJSON,
					},
				})
			}
		}

		// 处理 finishReason
		if candidate.FinishReason != "" {
			var stopReason string
			switch candidate.FinishReason {
			case "STOP":
				stopReason = "end_turn"
			case "MAX_TOKENS":
				stopReason = "max_tokens"
			case "SAFETY":
				stopReason = "safety"
			case "RECITATION":
				stopReason = "recitation"
			default:
				stopReason = strings.ToLower(candidate.FinishReason)
			}
			result.StopReason = stopReason
			onEvent(StreamEvent{Type: EventStop, StopReason: stopReason})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("llmclient: gemini SSE scan: %w", err)
	}

	// SSE 内嵌错误 → 返回 API 错误
	if sseErrMsg != "" {
		return nil, &APIError{
			Type:    "gemini_sse_error",
			Message: sseErrMsg,
		}
	}

	slog.Info("gemini SSE parse summary",
		"dataLines", dataLines,
		"parseErrors", parseErrs,
		"textLen", textBuf.Len(),
		"toolCalls", len(toolCalls),
		"stopReason", result.StopReason,
		"inputTokens", result.Usage.InputTokens,
		"outputTokens", result.Usage.OutputTokens,
	)

	// 构建最终 assistant message
	blocks := make([]ContentBlock, 0)
	if textBuf.Len() > 0 {
		blocks = append(blocks, ContentBlock{Type: "text", Text: textBuf.String()})
	}
	blocks = append(blocks, toolCalls...)
	result.AssistantMessage = ChatMessage{Role: "assistant", Content: blocks}

	// 空响应诊断: SSE 有数据但无内容提取 → 返回可感知的错误
	if len(blocks) == 0 && dataLines > 0 {
		reason := result.StopReason
		if reason == "" {
			reason = "unknown"
		}
		// 安全过滤或其他非正常终止
		if reason == "safety" || reason == "recitation" {
			return nil, &APIError{
				Type:    "content_filtered",
				Message: fmt.Sprintf("Gemini blocked response (reason: %s). Try rephrasing the prompt.", reason),
			}
		}
		// 其他空响应（如模型不支持工具调用的场景）
		return nil, &APIError{
			Type:    "empty_response",
			Message: fmt.Sprintf("Gemini returned empty response (dataLines=%d, parseErrors=%d, stopReason=%s)", dataLines, parseErrs, reason),
		}
	}

	return &result, nil
}

// ---------- 错误解析 ----------

func parseGeminiError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	apiErr := &APIError{StatusCode: resp.StatusCode}

	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		apiErr.Type = errResp.Error.Status
		apiErr.Message = errResp.Error.Message
	} else {
		apiErr.Type = "http_error"
		apiErr.Message = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	apiErr.Retryable = apiErr.IsRateLimit() || resp.StatusCode >= 500
	return apiErr
}

// ---------- 兼容检测 ----------

// isGeminiCompatible 检查 BaseURL 是否为 Gemini/Google AI 兼容 API。
func isGeminiCompatible(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "generativelanguage.googleapis.com") ||
		strings.Contains(lower, "aiplatform.googleapis.com") ||
		strings.Contains(lower, "gemini")
}
