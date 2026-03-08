package infra

// retry.go — 重试/退避机制
// 对应 TS:
//   - src/infra/retry.ts (重试逻辑 + 配置解析)
//   - src/infra/backoff.ts (退避策略 + 可取消 sleep)
//
// 设计：自写实现保持 TS 1:1 对齐，不引入外部库。
// 支持指数退避 + jitter + context 取消 + shouldRetry 回调。

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// ─── 退避策略 (backoff.ts) ───

// BackoffPolicy 退避策略参数。
type BackoffPolicy struct {
	InitialMs int     `json:"initialMs"`
	MaxMs     int     `json:"maxMs"`
	Factor    float64 `json:"factor"`
	Jitter    float64 `json:"jitter"`
}

// ComputeBackoff 计算第 attempt 次重试的退避时间（毫秒）。
// 对应 TS: computeBackoff(policy, attempt)
func ComputeBackoff(policy BackoffPolicy, attempt int) int {
	exp := math.Max(float64(attempt-1), 0)
	base := float64(policy.InitialMs) * math.Pow(policy.Factor, exp)
	jitter := base * policy.Jitter * rand.Float64()
	result := math.Min(float64(policy.MaxMs), math.Round(base+jitter))
	return int(result)
}

// SleepWithCancel 可取消的 sleep。
// 对应 TS: sleepWithAbort(ms, abortSignal)
func SleepWithCancel(ctx context.Context, ms int) error {
	if ms <= 0 {
		return nil
	}
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ─── 重试配置 (retry.ts) ───

// RetryConfig 可覆盖的重试参数。
type RetryConfig struct {
	Attempts  int     `json:"attempts,omitempty"`
	MinDelay  int     `json:"minDelayMs,omitempty"`
	MaxDelay  int     `json:"maxDelayMs,omitempty"`
	Jitter    float64 `json:"jitter,omitempty"`
	jitterSet bool    // 区分 jitter=0（显式）和未设置
}

// SetJitter 显式设置 jitter 值（含 0）。
func (c *RetryConfig) SetJitter(j float64) {
	c.Jitter = j
	c.jitterSet = true
}

// ResolvedRetryConfig 解析后的重试配置（所有字段必填）。
type ResolvedRetryConfig struct {
	Attempts int
	MinDelay int
	MaxDelay int
	Jitter   float64
}

// DefaultRetryConfig 默认重试配置。
var DefaultRetryConfig = ResolvedRetryConfig{
	Attempts: 3,
	MinDelay: 300,
	MaxDelay: 30_000,
	Jitter:   0,
}

// ResolveRetryConfig 合并默认值和覆盖值。
// 对应 TS: resolveRetryConfig(defaults, overrides)
func ResolveRetryConfig(defaults ResolvedRetryConfig, overrides *RetryConfig) ResolvedRetryConfig {
	result := defaults
	if overrides == nil {
		return result
	}
	if overrides.Attempts > 0 {
		result.Attempts = intMax(1, overrides.Attempts)
	}
	if overrides.MinDelay > 0 {
		result.MinDelay = intMax(0, overrides.MinDelay)
	}
	if overrides.MaxDelay > 0 {
		result.MaxDelay = intMax(result.MinDelay, overrides.MaxDelay)
	}
	if overrides.jitterSet {
		result.Jitter = clampFloat(overrides.Jitter, 0, 1)
	}
	return result
}

// ─── 重试信息 ───

// RetryInfo 单次重试的上下文信息。
type RetryInfo struct {
	Attempt     int
	MaxAttempts int
	DelayMs     int
	Err         error
	Label       string
}

// ─── 重试选项 ───

// RetryOptions 完整重试选项。
type RetryOptions struct {
	RetryConfig
	Label        string
	ShouldRetry  func(err error, attempt int) bool
	RetryAfterMs func(err error) (int, bool)
	OnRetry      func(info RetryInfo)
}

// ─── 核心重试函数 ───

// RetryAsync 带指数退避的通用重试函数。
// 对应 TS: retryAsync(fn, attemptsOrOptions, initialDelayMs)
//
// 简单模式：RetryAsync(ctx, fn, nil) 使用默认 3 次重试
// 配置模式：RetryAsync(ctx, fn, &RetryOptions{...})
func RetryAsync[T any](ctx context.Context, fn func() (T, error), opts *RetryOptions) (T, error) {
	var zero T

	if opts == nil {
		opts = &RetryOptions{}
	}

	resolved := ResolveRetryConfig(DefaultRetryConfig, &opts.RetryConfig)
	maxAttempts := resolved.Attempts
	minDelay := resolved.MinDelay
	maxDelay := resolved.MaxDelay
	if maxDelay <= 0 {
		maxDelay = math.MaxInt32
	}
	jitter := resolved.Jitter

	shouldRetry := opts.ShouldRetry
	if shouldRetry == nil {
		shouldRetry = func(_ error, _ int) bool { return true }
	}

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			if lastErr != nil {
				return zero, lastErr
			}
			return zero, ctx.Err()
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		if attempt >= maxAttempts || !shouldRetry(err, attempt) {
			break
		}

		// 计算延迟
		var baseDelay int
		if opts.RetryAfterMs != nil {
			if retryAfter, ok := opts.RetryAfterMs(err); ok && retryAfter > 0 {
				baseDelay = intMax(retryAfter, minDelay)
			} else {
				baseDelay = minDelay * intPow(2, attempt-1)
			}
		} else {
			baseDelay = minDelay * intPow(2, attempt-1)
		}

		delay := intMin(baseDelay, maxDelay)
		delay = applyJitter(delay, jitter)
		delay = intMin(intMax(delay, minDelay), maxDelay)

		// 回调通知
		if opts.OnRetry != nil {
			opts.OnRetry(RetryInfo{
				Attempt:     attempt,
				MaxAttempts: maxAttempts,
				DelayMs:     delay,
				Err:         err,
				Label:       opts.Label,
			})
		}

		if err := SleepWithCancel(ctx, delay); err != nil {
			return zero, lastErr
		}
	}

	if lastErr == nil {
		lastErr = errors.New("retry failed")
	}
	return zero, lastErr
}

// ─── 辅助函数 ───

func applyJitter(delayMs int, jitter float64) int {
	if jitter <= 0 {
		return delayMs
	}
	offset := (rand.Float64()*2 - 1) * jitter
	result := float64(delayMs) * (1 + offset)
	return intMax(0, int(math.Round(result)))
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intPow(base, exp int) int {
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
