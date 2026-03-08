package infra

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestHeartbeatWaker_RequestAndExecute(t *testing.T) {
	w := NewHeartbeatWaker()
	var called atomic.Int32
	w.SetHandler(func(reason string) HeartbeatRunResult {
		called.Add(1)
		return HeartbeatRunResult{Status: "ran", DurationMs: 10}
	})
	w.RequestNow("test", 10) // 10ms coalesce 加速测试
	time.Sleep(50 * time.Millisecond)
	if called.Load() < 1 {
		t.Fatal("expected handler to be called")
	}
	w.Stop()
}

func TestHeartbeatWaker_NoHandlerNoOp(t *testing.T) {
	w := NewHeartbeatWaker()
	w.RequestNow("test", 10)
	time.Sleep(30 * time.Millisecond)
	// 无 handler 应不 panic
	w.Stop()
}

func TestHeartbeatWaker_StopPreventsExecution(t *testing.T) {
	w := NewHeartbeatWaker()
	var called atomic.Int32
	w.SetHandler(func(reason string) HeartbeatRunResult {
		called.Add(1)
		return HeartbeatRunResult{Status: "ran"}
	})
	w.Stop()
	w.RequestNow("test", 10)
	time.Sleep(30 * time.Millisecond)
	if called.Load() != 0 {
		t.Fatal("handler should not be called after stop")
	}
}

func TestHeartbeatWaker_RetryOnSkip(t *testing.T) {
	w := NewHeartbeatWaker()
	var count atomic.Int32
	w.SetHandler(func(reason string) HeartbeatRunResult {
		c := count.Add(1)
		if c == 1 {
			return HeartbeatRunResult{Status: "skipped", Reason: "requests-in-flight"}
		}
		return HeartbeatRunResult{Status: "ran"}
	})
	w.RequestNow("test", 10)
	time.Sleep(1500 * time.Millisecond)
	if count.Load() < 2 {
		t.Fatalf("expected retry, got %d calls", count.Load())
	}
	w.Stop()
}

func TestHeartbeatWaker_HasPending(t *testing.T) {
	w := NewHeartbeatWaker()
	if w.HasPending() {
		t.Fatal("should not have pending initially")
	}
	w.SetHandler(func(reason string) HeartbeatRunResult {
		return HeartbeatRunResult{Status: "ran"}
	})
	w.RequestNow("test", 500) // 长延迟确保检查时还在 pending
	if !w.HasPending() {
		t.Fatal("should have pending after request")
	}
	w.Stop()
}
