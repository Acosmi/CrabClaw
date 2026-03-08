package infra

import (
	"sync"
	"time"
)

// ---------- 心跳执行结果 ----------

// HeartbeatRunResult 心跳执行结果。
type HeartbeatRunResult struct {
	Status     string `json:"status"` // "ran", "skipped", "failed"
	DurationMs int64  `json:"durationMs,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// ---------- 唤醒调度器（实例化） ----------

const (
	defaultCoalesceMs = 250
	defaultRetryMs    = 1000
)

// HeartbeatWaker 心跳唤醒调度器（实例级）。
// 支持请求合并、失败重试、并发安全。
type HeartbeatWaker struct {
	mu            sync.Mutex
	handler       func(reason string) HeartbeatRunResult
	pendingReason *string
	scheduled     bool
	running       bool
	timer         *time.Timer
	stopped       bool
}

// NewHeartbeatWaker 创建心跳唤醒调度器。
func NewHeartbeatWaker() *HeartbeatWaker {
	return &HeartbeatWaker{}
}

// SetHandler 设置心跳唤醒处理器。
func (w *HeartbeatWaker) SetHandler(handler func(reason string) HeartbeatRunResult) {
	w.mu.Lock()
	w.handler = handler
	hasPending := w.pendingReason != nil
	w.mu.Unlock()

	if handler != nil && hasPending {
		w.schedule(defaultCoalesceMs)
	}
}

// RequestNow 请求立即执行心跳。coalesceMs 为合并窗口（毫秒）。
func (w *HeartbeatWaker) RequestNow(reason string, coalesceMs int) {
	if coalesceMs <= 0 {
		coalesceMs = defaultCoalesceMs
	}

	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	if reason == "" && w.pendingReason != nil {
		// 保留已有 reason
	} else {
		r := reason
		if r == "" {
			r = "requested"
		}
		w.pendingReason = &r
	}
	w.mu.Unlock()

	w.schedule(coalesceMs)
}

// HasPending 是否有待执行的心跳。
func (w *HeartbeatWaker) HasPending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pendingReason != nil || w.timer != nil || w.scheduled
}

// Stop 停止调度器，取消所有待执行任务。
func (w *HeartbeatWaker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	w.handler = nil
	w.pendingReason = nil
	w.scheduled = false
	w.running = false
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// ---------- 内部调度逻辑 ----------

func (w *HeartbeatWaker) schedule(coalesceMs int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.scheduleLocked(coalesceMs)
}

func (w *HeartbeatWaker) scheduleLocked(coalesceMs int) {
	if w.stopped || w.timer != nil {
		return
	}

	delay := time.Duration(coalesceMs) * time.Millisecond
	w.timer = time.AfterFunc(delay, func() {
		w.executeWake(coalesceMs)
	})
}

func (w *HeartbeatWaker) executeWake(coalesceMs int) {
	w.mu.Lock()
	w.timer = nil
	w.scheduled = false

	if w.stopped {
		w.mu.Unlock()
		return
	}

	active := w.handler
	if active == nil {
		w.mu.Unlock()
		return
	}

	if w.running {
		w.scheduled = true
		w.scheduleLocked(coalesceMs)
		w.mu.Unlock()
		return
	}

	var reason string
	if w.pendingReason != nil {
		reason = *w.pendingReason
		w.pendingReason = nil
	}
	w.running = true
	w.mu.Unlock()

	// 执行 handler（不持锁）
	var res HeartbeatRunResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				w.mu.Lock()
				retryReason := reason
				if retryReason == "" {
					retryReason = "retry"
				}
				w.pendingReason = &retryReason
				w.mu.Unlock()
			}
		}()
		res = active(reason)
	}()

	w.mu.Lock()
	w.running = false

	// requests-in-flight 时重试
	if res.Status == "skipped" && res.Reason == "requests-in-flight" {
		retryReason := reason
		if retryReason == "" {
			retryReason = "retry"
		}
		w.pendingReason = &retryReason
		w.scheduleLocked(defaultRetryMs)
	}

	// 仍有待处理或已调度，继续
	if w.pendingReason != nil || w.scheduled {
		w.scheduleLocked(coalesceMs)
	}
	w.mu.Unlock()
}
