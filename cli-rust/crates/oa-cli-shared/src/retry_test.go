package infra

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComputeBackoff(t *testing.T) {
	policy := BackoffPolicy{InitialMs: 100, MaxMs: 5000, Factor: 2, Jitter: 0}

	// attempt 1 → 100ms
	d := ComputeBackoff(policy, 1)
	if d != 100 {
		t.Errorf("attempt 1: got %d, want 100", d)
	}

	// attempt 2 → 200ms
	d = ComputeBackoff(policy, 2)
	if d != 200 {
		t.Errorf("attempt 2: got %d, want 200", d)
	}

	// attempt 3 → 400ms
	d = ComputeBackoff(policy, 3)
	if d != 400 {
		t.Errorf("attempt 3: got %d, want 400", d)
	}

	// attempt 100 → capped at 5000
	d = ComputeBackoff(policy, 100)
	if d != 5000 {
		t.Errorf("attempt 100: got %d, want 5000", d)
	}
}

func TestComputeBackoffWithJitter(t *testing.T) {
	policy := BackoffPolicy{InitialMs: 100, MaxMs: 5000, Factor: 2, Jitter: 0.5}
	d := ComputeBackoff(policy, 1)
	// jitter=0.5 → 100 + 100*0.5*rand → [100, 150]
	if d < 100 || d > 150 {
		t.Errorf("attempt 1 with jitter: got %d, want [100, 150]", d)
	}
}

func TestSleepWithCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := SleepWithCancel(ctx, 10_000)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

func TestSleepWithCancelZero(t *testing.T) {
	err := SleepWithCancel(context.Background(), 0)
	if err != nil {
		t.Errorf("unexpected error for 0ms sleep: %v", err)
	}
}

func TestRetryAsyncSuccess(t *testing.T) {
	calls := 0
	result, err := RetryAsync(context.Background(), func() (string, error) {
		calls++
		return "ok", nil
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("got %q, want %q", result, "ok")
	}
	if calls != 1 {
		t.Errorf("called %d times, want 1", calls)
	}
}

func TestRetryAsyncFailThenSucceed(t *testing.T) {
	calls := 0
	result, err := RetryAsync(context.Background(), func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("temporary")
		}
		return 42, nil
	}, &RetryOptions{
		RetryConfig: RetryConfig{Attempts: 5, MinDelay: 1, MaxDelay: 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("got %d, want 42", result)
	}
	if calls != 3 {
		t.Errorf("called %d times, want 3", calls)
	}
}

func TestRetryAsyncExhausted(t *testing.T) {
	calls := 0
	_, err := RetryAsync(context.Background(), func() (string, error) {
		calls++
		return "", errors.New("permanent")
	}, &RetryOptions{
		RetryConfig: RetryConfig{Attempts: 3, MinDelay: 1, MaxDelay: 5},
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != 3 {
		t.Errorf("called %d times, want 3", calls)
	}
}

func TestRetryAsyncShouldRetry(t *testing.T) {
	calls := 0
	_, err := RetryAsync(context.Background(), func() (string, error) {
		calls++
		return "", errors.New("stop now")
	}, &RetryOptions{
		RetryConfig: RetryConfig{Attempts: 5, MinDelay: 1},
		ShouldRetry: func(_ error, attempt int) bool {
			return attempt < 2 // 仅重试 1 次
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 2 {
		t.Errorf("called %d times, want 2", calls)
	}
}

func TestRetryAsyncOnRetryCallback(t *testing.T) {
	var infos []RetryInfo
	_, _ = RetryAsync(context.Background(), func() (string, error) {
		return "", errors.New("fail")
	}, &RetryOptions{
		RetryConfig: RetryConfig{Attempts: 3, MinDelay: 1, MaxDelay: 5},
		Label:       "test-op",
		OnRetry: func(info RetryInfo) {
			infos = append(infos, info)
		},
	})
	if len(infos) != 2 { // 3 attempts → 2 retries
		t.Errorf("got %d retries, want 2", len(infos))
	}
	if infos[0].Label != "test-op" {
		t.Errorf("label: got %q, want %q", infos[0].Label, "test-op")
	}
}

func TestRetryAsyncContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := RetryAsync(ctx, func() (string, error) {
		return "", errors.New("should not reach here repeatedly")
	}, &RetryOptions{
		RetryConfig: RetryConfig{Attempts: 100, MinDelay: 1000},
	})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestResolveRetryConfig(t *testing.T) {
	result := ResolveRetryConfig(DefaultRetryConfig, &RetryConfig{Attempts: 5})
	if result.Attempts != 5 {
		t.Errorf("attempts: got %d, want 5", result.Attempts)
	}
	if result.MinDelay != 300 {
		t.Errorf("minDelay: got %d, want 300 (default)", result.MinDelay)
	}
}

func TestApplyJitter(t *testing.T) {
	// jitter=0 → 不变
	d := applyJitter(1000, 0)
	if d != 1000 {
		t.Errorf("jitter=0: got %d, want 1000", d)
	}

	// jitter=1 → [0, 2000]
	for i := 0; i < 100; i++ {
		d := applyJitter(1000, 1)
		if d < 0 || d > 2000 {
			t.Errorf("jitter=1: got %d, want [0, 2000]", d)
		}
	}
}

// ─── Dedupe 测试 ───

func TestDedupeCacheBasic(t *testing.T) {
	cache := NewDedupeCache(DedupeCacheOptions{TTLMs: 1000, MaxSize: 10})

	// 首次 → false
	if cache.Check("a") {
		t.Error("expected false for first check")
	}
	// 第二次 → true（重复）
	if !cache.Check("a") {
		t.Error("expected true for duplicate check")
	}
}

func TestDedupeCacheEmptyKey(t *testing.T) {
	cache := NewDedupeCache(DedupeCacheOptions{TTLMs: 1000, MaxSize: 10})
	if cache.Check("") {
		t.Error("expected false for empty key")
	}
}

func TestDedupeCacheTTLExpiry(t *testing.T) {
	cache := NewDedupeCache(DedupeCacheOptions{TTLMs: 100, MaxSize: 10})
	now := time.Now()

	cache.CheckAt("a", now)

	// 在 TTL 内 → true
	if !cache.CheckAt("a", now.Add(50*time.Millisecond)) {
		t.Error("expected true within TTL")
	}

	// 过 TTL → false
	if cache.CheckAt("a", now.Add(200*time.Millisecond)) {
		t.Error("expected false after TTL expiry")
	}
}

func TestDedupeCacheMaxSize(t *testing.T) {
	cache := NewDedupeCache(DedupeCacheOptions{TTLMs: 60000, MaxSize: 3})
	cache.Check("a")
	cache.Check("b")
	cache.Check("c")
	cache.Check("d") // 应淘汰 "a"

	if cache.Size() > 3 {
		t.Errorf("size=%d, want <=3", cache.Size())
	}
}

func TestDedupeCacheClear(t *testing.T) {
	cache := NewDedupeCache(DedupeCacheOptions{TTLMs: 60000, MaxSize: 10})
	cache.Check("a")
	cache.Check("b")
	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("size=%d after clear, want 0", cache.Size())
	}
}

// ─── MachineNameTest ───

func TestGetMachineDisplayName(t *testing.T) {
	ResetMachineNameForTest()
	name := GetMachineDisplayName()
	if name == "" {
		t.Error("expected non-empty machine name")
	}
	// 第二次调用应返回相同值（缓存）
	name2 := GetMachineDisplayName()
	if name2 != name {
		t.Errorf("cached: got %q, want %q", name2, name)
	}
}

func TestFallbackHostName(t *testing.T) {
	name := fallbackHostName()
	if name == "" {
		t.Error("expected non-empty hostname")
	}
}
