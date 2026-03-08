package errors

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// ── errors.Is sentinel 判定 ──

func TestIs_SentinelMatch(t *testing.T) {
	err := NewFrom(ErrNotFound, "user not found")
	if !errors.Is(err, ErrNotFound) {
		t.Error("NewFrom(ErrNotFound) should match ErrNotFound via errors.Is")
	}
}

func TestIs_SentinelMismatch(t *testing.T) {
	err := NewFrom(ErrNotFound, "user not found")
	if errors.Is(err, ErrUnauthorized) {
		t.Error("ErrNotFound should not match ErrUnauthorized")
	}
}

func TestIs_WrappedSentinel(t *testing.T) {
	inner := NewFrom(ErrNotFound, "session not found")
	wrapped := fmt.Errorf("lookup failed: %w", inner)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("fmt.Errorf %%w wrapped AppError should match via errors.Is")
	}
}

func TestIs_WrapFrom(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	err := WrapFrom(ErrServiceUnavail, "database unreachable", cause)
	if !errors.Is(err, ErrServiceUnavail) {
		t.Error("WrapFrom(ErrServiceUnavail) should match ErrServiceUnavail")
	}
	if !errors.Is(err, cause) {
		t.Error("WrapFrom should preserve cause via Unwrap")
	}
}

// ── errors.As 类型断言 ──

func TestAs_ExtractAppError(t *testing.T) {
	err := New(CodeValidation, "email format invalid")
	wrapped := fmt.Errorf("user creation: %w", err)

	var appErr *AppError
	if !errors.As(wrapped, &appErr) {
		t.Fatal("errors.As should extract *AppError from wrapped error")
	}
	if appErr.Code != CodeValidation {
		t.Errorf("expected code %q, got %q", CodeValidation, appErr.Code)
	}
	if appErr.Message != "email format invalid" {
		t.Errorf("expected message 'email format invalid', got %q", appErr.Message)
	}
}

// ── Unwrap 链式错误 ──

func TestUnwrap_Chain(t *testing.T) {
	root := fmt.Errorf("disk full")
	mid := Wrap(CodeInternal, "write failed", root)
	top := WrapFrom(ErrServiceUnavail, "save user", mid)

	// top → mid → root
	if !errors.Is(top, root) {
		t.Error("top should unwrap to root")
	}
	if !errors.Is(top, ErrInternal) {
		t.Error("top should match ErrInternal via mid")
	}
	if !errors.Is(top, ErrServiceUnavail) {
		t.Error("top should match ErrServiceUnavail")
	}
}

// ── Error() 格式化 ──

func TestError_WithoutCause(t *testing.T) {
	err := New(CodeBadRequest, "missing field")
	expected := "[BAD_REQUEST] missing field"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestError_WithCause(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	err := Wrap(CodeTimeout, "request timeout", cause)
	got := err.Error()
	if got != "[TIMEOUT] request timeout: connection refused" {
		t.Errorf("unexpected error string: %q", got)
	}
}

// ── 辅助函数测试 ──

func TestExtractErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"plain error", fmt.Errorf("plain"), ""},
		{"AppError", New(CodeNotFound, "x"), CodeNotFound},
		{"wrapped AppError", fmt.Errorf("w: %w", New(CodeConflict, "x")), CodeConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractErrorCode(tt.err)
			if got != tt.want {
				t.Errorf("ExtractErrorCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"plain error", fmt.Errorf("something broke"), "something broke"},
		{"AppError", New(CodeInternal, "db crashed"), "db crashed"},
		{"wrapped AppError", fmt.Errorf("w: %w", New(CodeInternal, "db crashed")), "db crashed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatErrorMessage(tt.err)
			if got != tt.want {
				t.Errorf("FormatErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatUncaughtError(t *testing.T) {
	// INVALID_CONFIG → only message (user-friendly)
	cfgErr := New(CodeInvalidConfig, "missing API key")
	got := FormatUncaughtError(cfgErr)
	if got != "missing API key" {
		t.Errorf("INVALID_CONFIG should return message only, got %q", got)
	}

	// Other errors → full Error() with code prefix
	otherErr := Wrap(CodeInternal, "init failed", fmt.Errorf("segfault"))
	got = FormatUncaughtError(otherErr)
	if got != "[INTERNAL_ERROR] init failed: segfault" {
		t.Errorf("non-config error should return full Error(), got %q", got)
	}

	// nil
	if FormatUncaughtError(nil) != "" {
		t.Error("nil should return empty string")
	}
}

func TestIsCode(t *testing.T) {
	err := NewFrom(ErrRateLimit, "too fast")
	if !IsCode(err, CodeRateLimit) {
		t.Error("IsCode should match RATE_LIMIT")
	}
	if IsCode(err, CodeTimeout) {
		t.Error("IsCode should not match TIMEOUT")
	}
	if IsCode(nil, CodeTimeout) {
		t.Error("IsCode(nil) should be false")
	}
}

func TestGetHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusInternalServerError},
		{"plain", fmt.Errorf("x"), http.StatusInternalServerError},
		{"ErrNotFound", NewFrom(ErrNotFound, "x"), http.StatusNotFound},
		{"ErrRateLimit", NewFrom(ErrRateLimit, "x"), http.StatusTooManyRequests},
		{"custom", New("CUSTOM", "x").WithHTTPStatus(http.StatusTeapot), http.StatusTeapot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetHTTPStatus(tt.err)
			if got != tt.want {
				t.Errorf("GetHTTPStatus() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := New(CodeTimeout, "slow").WithRetryable(true)
	if !IsRetryable(retryable) {
		t.Error("should be retryable")
	}
	nonRetryable := New(CodeBadRequest, "invalid")
	if IsRetryable(nonRetryable) {
		t.Error("should not be retryable")
	}
	if IsRetryable(nil) {
		t.Error("nil should not be retryable")
	}
}

// ── 链式设置器 ──

func TestWithDetails(t *testing.T) {
	err := New(CodeValidation, "invalid").WithDetails(map[string]any{
		"field": "email",
		"got":   "not-an-email",
	})
	if err.Details["field"] != "email" {
		t.Error("details missing 'field'")
	}
}

func TestWithDetail(t *testing.T) {
	err := New(CodeValidation, "invalid").
		WithDetail("field", "name").
		WithDetail("min", 3)
	if err.Details["field"] != "name" {
		t.Error("details missing 'field'")
	}
	if err.Details["min"] != 3 {
		t.Error("details missing 'min'")
	}
}

// ── JSON 序列化 ──

func TestMarshalJSON(t *testing.T) {
	err := Wrap(CodeNotFound, "user not found", fmt.Errorf("sql: no rows")).
		WithHTTPStatus(http.StatusNotFound).
		WithDetail("userId", "abc123")

	data, jsonErr := err.MarshalJSON()
	if jsonErr != nil {
		t.Fatalf("MarshalJSON failed: %v", jsonErr)
	}
	s := string(data)
	if !contains(s, `"code":"NOT_FOUND"`) {
		t.Errorf("JSON missing code: %s", s)
	}
	if !contains(s, `"cause":"sql: no rows"`) {
		t.Errorf("JSON missing cause: %s", s)
	}
	if !contains(s, `"userId":"abc123"`) {
		t.Errorf("JSON missing detail: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
