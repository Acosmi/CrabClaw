package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// ManagedProxyClient 托管模型供应商 (Phase 5)。
// 通过 nexus-v4 代理端点调用 LLM，扣费在云端完成。
type ManagedProxyClient struct {
	proxyEndpoint string
	tokenProvider func() (string, error)
	client        *http.Client
}

// NewManagedProxyClient 创建托管模型供应商。
func NewManagedProxyClient(proxyEndpoint string, tokenProvider func() (string, error)) *ManagedProxyClient {
	return &ManagedProxyClient{
		proxyEndpoint: proxyEndpoint,
		tokenProvider: tokenProvider,
		client:        &http.Client{Timeout: 120 * time.Second},
	}
}

// ManagedChatRequest 托管模型请求。
type ManagedChatRequest struct {
	ModelID  string      `json:"modelId"`
	Messages interface{} `json:"messages"`
	Stream   bool        `json:"stream,omitempty"`
}

// ManagedChatResponse 托管模型响应。
type ManagedChatResponse struct {
	ID      string                    `json:"id,omitempty"`
	Object  string                    `json:"object,omitempty"`
	Model   string                    `json:"model,omitempty"`
	Choices []ManagedChatChoiceResult `json:"choices,omitempty"`
	Usage   *ManagedUsage             `json:"usage,omitempty"`
	Raw     json.RawMessage           `json:"-"`
}

// ManagedChatChoiceResult 响应选项。
type ManagedChatChoiceResult struct {
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason,omitempty"`
	Message      struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

// ManagedUsage token 用量。
type ManagedUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ManagedProviderError 代理错误。
type ManagedProviderError struct {
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *ManagedProviderError) Error() string {
	return fmt.Sprintf("managed_provider: HTTP %d: %s", e.StatusCode, e.Message)
}

// ChatCompletion 通过 nexus-v4 代理调用 LLM。
// 401 → 需重新登录
// 402 → 余额不足
// 5xx → 服务端错误（可重试）
func (p *ManagedProxyClient) ChatCompletion(req *ManagedChatRequest) (*ManagedChatResponse, error) {
	token, err := p.tokenProvider()
	if err != nil {
		return nil, fmt.Errorf("managed_provider: get token: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("managed_provider: marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", p.proxyEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("managed_provider: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("managed_provider: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("managed_provider: read response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, &ManagedProviderError{
			StatusCode: 401,
			Message:    "认证失败，请重新登录",
			Retryable:  false,
		}
	case resp.StatusCode == http.StatusPaymentRequired:
		return nil, &ManagedProviderError{
			StatusCode: 402,
			Message:    "余额不足，请充值或购买流量包",
			Retryable:  false,
		}
	case resp.StatusCode >= 500:
		msg := string(respBody)
		if len(msg) > 256 {
			msg = msg[:256] + "..."
		}
		return nil, &ManagedProviderError{
			StatusCode: resp.StatusCode,
			Message:    "服务端错误: " + msg,
			Retryable:  true,
		}
	case resp.StatusCode != http.StatusOK:
		msg := string(respBody)
		if len(msg) > 256 {
			msg = msg[:256] + "..."
		}
		return nil, &ManagedProviderError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Retryable:  false,
		}
	}

	var chatResp ManagedChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		slog.Warn("managed_provider: failed to parse structured response, returning raw",
			"error", err)
		chatResp.Raw = respBody
	}

	return &chatResp, nil
}

// IsBalanceInsufficient 检查错误是否为余额不足。
func IsBalanceInsufficient(err error) bool {
	if pe, ok := err.(*ManagedProviderError); ok {
		return pe.StatusCode == 402
	}
	return false
}

// IsAuthRequired 检查错误是否需要重新登录。
func IsAuthRequired(err error) bool {
	if pe, ok := err.(*ManagedProviderError); ok {
		return pe.StatusCode == 401
	}
	return false
}
