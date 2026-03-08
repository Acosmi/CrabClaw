package errors

// 结构化错误类型系统 — 对应 TS infra/errors.ts
//
// 提供统一的 AppError 类型，支持:
//   - 错误码 (Code) + 人类可读消息 (Message)
//   - HTTP 状态码映射 (HTTPStatus)
//   - 链式包装 (Unwrap, fmt.Errorf %w)
//   - errors.Is / errors.As 兼容
//   - 可重试标记 (Retryable)
//   - 附加详情 (Details)

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ─── 错误码常量 ───

const (
	CodeInvalidConfig    = "INVALID_CONFIG"
	CodeNotFound         = "NOT_FOUND"
	CodeUnauthorized     = "UNAUTHORIZED"
	CodeForbidden        = "FORBIDDEN"
	CodeConflict         = "CONFLICT"
	CodeTimeout          = "TIMEOUT"
	CodeServiceUnavail   = "SERVICE_UNAVAILABLE"
	CodeInternal         = "INTERNAL_ERROR"
	CodeValidation       = "VALIDATION_ERROR"
	CodeRateLimit        = "RATE_LIMIT"
	CodeSignatureInvalid = "SIGNATURE_INVALID"
	CodeBadRequest       = "BAD_REQUEST"
	CodeAlreadyExists    = "ALREADY_EXISTS"
	CodePreconditionFail = "PRECONDITION_FAILED"
)

// ─── Sentinel 错误（用于 errors.Is 判定） ───

var (
	ErrInvalidConfig    = &AppError{Code: CodeInvalidConfig, Message: "invalid configuration", HTTPStatus: http.StatusBadRequest}
	ErrNotFound         = &AppError{Code: CodeNotFound, Message: "not found", HTTPStatus: http.StatusNotFound}
	ErrUnauthorized     = &AppError{Code: CodeUnauthorized, Message: "unauthorized", HTTPStatus: http.StatusUnauthorized}
	ErrForbidden        = &AppError{Code: CodeForbidden, Message: "forbidden", HTTPStatus: http.StatusForbidden}
	ErrConflict         = &AppError{Code: CodeConflict, Message: "conflict", HTTPStatus: http.StatusConflict}
	ErrTimeout          = &AppError{Code: CodeTimeout, Message: "timeout", HTTPStatus: http.StatusGatewayTimeout}
	ErrServiceUnavail   = &AppError{Code: CodeServiceUnavail, Message: "service unavailable", HTTPStatus: http.StatusServiceUnavailable}
	ErrInternal         = &AppError{Code: CodeInternal, Message: "internal error", HTTPStatus: http.StatusInternalServerError}
	ErrValidation       = &AppError{Code: CodeValidation, Message: "validation error", HTTPStatus: http.StatusUnprocessableEntity}
	ErrRateLimit        = &AppError{Code: CodeRateLimit, Message: "rate limit exceeded", HTTPStatus: http.StatusTooManyRequests}
	ErrSignatureInvalid = &AppError{Code: CodeSignatureInvalid, Message: "invalid signature", HTTPStatus: http.StatusUnauthorized}
	ErrBadRequest       = &AppError{Code: CodeBadRequest, Message: "bad request", HTTPStatus: http.StatusBadRequest}
	ErrAlreadyExists    = &AppError{Code: CodeAlreadyExists, Message: "already exists", HTTPStatus: http.StatusConflict}
	ErrPreconditionFail = &AppError{Code: CodePreconditionFail, Message: "precondition failed", HTTPStatus: http.StatusPreconditionFailed}
)

// ─── AppError 核心类型 ───

// AppError 结构化应用错误。
// 实现 error 接口，支持 errors.Is/As 和 Unwrap。
type AppError struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Cause      error          `json:"-"`
	HTTPStatus int            `json:"httpStatus,omitempty"`
	Retryable  bool           `json:"retryable,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

// Error 实现 error 接口。
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is/As 链式解包。
func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is 支持按 Code 匹配 sentinel 错误。
// 当 target 也是 *AppError 时，仅比较 Code 字段。
func (e *AppError) Is(target error) bool {
	var t *AppError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// MarshalJSON 自定义 JSON 序列化。
func (e *AppError) MarshalJSON() ([]byte, error) {
	type alias struct {
		Code       string         `json:"code"`
		Message    string         `json:"message"`
		HTTPStatus int            `json:"httpStatus,omitempty"`
		Retryable  bool           `json:"retryable,omitempty"`
		Details    map[string]any `json:"details,omitempty"`
		Cause      string         `json:"cause,omitempty"`
	}
	a := alias{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		Retryable:  e.Retryable,
		Details:    e.Details,
	}
	if e.Cause != nil {
		a.Cause = e.Cause.Error()
	}
	return json.Marshal(a)
}

// ─── 构造器 ───

// New 创建新的 AppError。
func New(code, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// Wrap 包装已有错误为 AppError。
func Wrap(code, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		Cause:      cause,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// NewFrom 从 sentinel 错误创建新实例，自定义消息。
func NewFrom(sentinel *AppError, message string) *AppError {
	return &AppError{
		Code:       sentinel.Code,
		Message:    message,
		HTTPStatus: sentinel.HTTPStatus,
		Retryable:  sentinel.Retryable,
	}
}

// WrapFrom 从 sentinel 错误创建新实例，携带原始错误。
func WrapFrom(sentinel *AppError, message string, cause error) *AppError {
	return &AppError{
		Code:       sentinel.Code,
		Message:    message,
		Cause:      cause,
		HTTPStatus: sentinel.HTTPStatus,
		Retryable:  sentinel.Retryable,
	}
}

// ─── 链式设置器 ───

// WithHTTPStatus 设置 HTTP 状态码。
func (e *AppError) WithHTTPStatus(status int) *AppError {
	e.HTTPStatus = status
	return e
}

// WithRetryable 标记为可重试。
func (e *AppError) WithRetryable(retryable bool) *AppError {
	e.Retryable = retryable
	return e
}

// WithDetails 附加详情。
func (e *AppError) WithDetails(details map[string]any) *AppError {
	e.Details = details
	return e
}

// WithDetail 附加单个详情键值。
func (e *AppError) WithDetail(key string, value any) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// ─── 辅助函数（对标 TS infra/errors.ts） ───

// ExtractErrorCode 从 error 中提取错误码。
// 对应 TS: extractErrorCode(err)
func ExtractErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

// FormatErrorMessage 格式化错误消息（人类可读）。
// 对应 TS: formatErrorMessage(err)
func FormatErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return err.Error()
}

// FormatUncaughtError 格式化未捕获错误（包含完整堆栈上下文）。
// 对应 TS: formatUncaughtError(err)
// 对 INVALID_CONFIG 仅返回消息（用户可读），其他错误返回完整 Error() 含 cause 链。
func FormatUncaughtError(err error) string {
	if err == nil {
		return ""
	}
	if ExtractErrorCode(err) == CodeInvalidConfig {
		return FormatErrorMessage(err)
	}
	return err.Error()
}

// IsCode 判断 err 是否包含指定错误码。
func IsCode(err error, code string) bool {
	return ExtractErrorCode(err) == code
}

// GetHTTPStatus 从错误中提取 HTTP 状态码，未找到时返回 500。
func GetHTTPStatus(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.HTTPStatus != 0 {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

// IsRetryable 判断错误是否可重试。
func IsRetryable(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Retryable
	}
	return false
}
