package channels

import (
	"errors"
	"fmt"
	"strings"
)

// SendErrorCode 渠道发送错误分类码。
// 供 gateway 层映射为统一 ErrorShape 语义。
type SendErrorCode string

const (
	SendErrInvalidRequest     SendErrorCode = "invalid_request"
	SendErrInvalidTarget      SendErrorCode = "invalid_target"
	SendErrUnsupportedFeature SendErrorCode = "unsupported_feature"
	SendErrPayloadTooLarge    SendErrorCode = "payload_too_large"
	SendErrUnauthorized       SendErrorCode = "unauthorized"
	SendErrUnavailable        SendErrorCode = "unavailable"
	SendErrUpstream           SendErrorCode = "upstream_error"
	SendErrRateLimited        SendErrorCode = "rate_limited"
)

// SendError 标准化渠道发送错误。
// 通过 errors.As 可被 gateway 识别并映射为用户态错误码/详情。
type SendError struct {
	Channel   ChannelID
	Code      SendErrorCode
	Operation string
	Message   string
	Retryable bool
	Details   map[string]interface{}
	Cause     error
}

// SendCode 返回标准化发送错误码（用于跨包错误识别，避免强依赖）。
func (e *SendError) SendCode() string {
	if e == nil {
		return ""
	}
	return string(e.Code)
}

// SendChannel 返回渠道 ID（用于跨包错误识别）。
func (e *SendError) SendChannel() string {
	if e == nil {
		return ""
	}
	return string(e.Channel)
}

// SendOperation 返回失败操作名（用于跨包错误识别）。
func (e *SendError) SendOperation() string {
	if e == nil {
		return ""
	}
	return e.Operation
}

// SendRetryable 返回是否可重试（用于跨包错误识别）。
func (e *SendError) SendRetryable() bool {
	if e == nil {
		return false
	}
	return e.Retryable
}

// SendUserMessage 返回用户态错误消息（用于跨包错误识别）。
func (e *SendError) SendUserMessage() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *SendError) Error() string {
	if e == nil {
		return "channel send error"
	}

	base := strings.TrimSpace(e.Message)
	if base == "" {
		base = "channel send failed"
	}
	if strings.TrimSpace(e.Operation) != "" {
		base = fmt.Sprintf("%s (%s)", base, e.Operation)
	}
	if e.Cause != nil {
		return base + ": " + e.Cause.Error()
	}
	return base
}

func (e *SendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewSendError 创建渠道发送错误。
func NewSendError(channel ChannelID, code SendErrorCode, message string) *SendError {
	return &SendError{
		Channel: channel,
		Code:    code,
		Message: strings.TrimSpace(message),
	}
}

// WrapSendError 创建带底层 cause 的渠道发送错误。
func WrapSendError(channel ChannelID, code SendErrorCode, operation, message string, cause error) *SendError {
	return &SendError{
		Channel:   channel,
		Code:      code,
		Operation: strings.TrimSpace(operation),
		Message:   strings.TrimSpace(message),
		Cause:     cause,
	}
}

func (e *SendError) WithRetryable(retryable bool) *SendError {
	if e == nil {
		return nil
	}
	e.Retryable = retryable
	return e
}

func (e *SendError) WithOperation(operation string) *SendError {
	if e == nil {
		return nil
	}
	e.Operation = strings.TrimSpace(operation)
	return e
}

func (e *SendError) WithDetails(details map[string]interface{}) *SendError {
	if e == nil || len(details) == 0 {
		return e
	}
	if e.Details == nil {
		e.Details = make(map[string]interface{}, len(details))
	}
	for k, v := range details {
		if strings.TrimSpace(k) == "" {
			continue
		}
		e.Details[k] = v
	}
	return e
}

// AsSendError 解析错误链中的 SendError。
func AsSendError(err error) (*SendError, bool) {
	if err == nil {
		return nil, false
	}
	var sendErr *SendError
	if errors.As(err, &sendErr) && sendErr != nil {
		return sendErr, true
	}
	return nil, false
}
