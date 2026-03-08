package infra

// transport_ready.go — 传输层就绪等待
// 对应 TS: src/infra/transport-ready.ts (67L)
//
// 轮询等待传输层（如 WebSocket、TCP 端口）就绪，
// 支持超时、取消和周期性日志输出。

import (
	"context"
	"fmt"
	"time"
)

// TransportReadyResult 传输层就绪检查结果。
type TransportReadyResult struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// LogFunc 日志输出函数（避免直接依赖特定日志库）。
type LogFunc func(level string, msg string)

// WaitForTransportReadyParams 传输层就绪等待参数。
type WaitForTransportReadyParams struct {
	// Label 日志标签（如 "Gateway WebSocket"）。
	Label string
	// TimeoutMs 总超时时间（毫秒）。
	TimeoutMs int
	// LogAfterMs 超过此时间后开始输出等待日志（默认等于 TimeoutMs）。
	LogAfterMs int
	// LogIntervalMs 等待日志输出间隔（默认 30s）。
	LogIntervalMs int
	// PollIntervalMs 轮询间隔（默认 150ms）。
	PollIntervalMs int
	// Check 就绪检查函数。
	Check func(ctx context.Context) TransportReadyResult
	// Log 可选日志函数。level: "warn" | "error"。
	Log LogFunc
}

// WaitForTransportReady 轮询等待传输层就绪。
// 对应 TS: waitForTransportReady(params)
//
// 返回 nil 表示就绪，返回 error 表示超时或取消。
func WaitForTransportReady(ctx context.Context, params WaitForTransportReadyParams) error {
	started := time.Now()
	timeoutMs := intMax(0, params.TimeoutMs)
	deadline := started.Add(time.Duration(timeoutMs) * time.Millisecond)

	logAfterMs := params.LogAfterMs
	if logAfterMs <= 0 {
		logAfterMs = timeoutMs
	}
	logIntervalMs := intMax(1000, params.LogIntervalMs)
	if params.LogIntervalMs <= 0 {
		logIntervalMs = 30_000
	}
	pollIntervalMs := intMax(50, params.PollIntervalMs)
	if params.PollIntervalMs <= 0 {
		pollIntervalMs = 150
	}

	nextLogAt := started.Add(time.Duration(logAfterMs) * time.Millisecond)
	var lastError string

	for {
		// 检查 context 取消
		if ctx.Err() != nil {
			return nil // 取消时静默返回（与 TS 行为一致）
		}

		res := params.Check(ctx)
		if res.Ok {
			return nil
		}
		lastError = res.Error

		now := time.Now()
		if now.After(deadline) || now.Equal(deadline) {
			break
		}

		// 周期性日志输出
		if (now.After(nextLogAt) || now.Equal(nextLogAt)) && params.Log != nil {
			elapsed := now.Sub(started).Milliseconds()
			errMsg := lastError
			if errMsg == "" {
				errMsg = "unknown error"
			}
			params.Log("warn", fmt.Sprintf("%s not ready after %dms (%s)", params.Label, elapsed, errMsg))
			nextLogAt = now.Add(time.Duration(logIntervalMs) * time.Millisecond)
		}

		// 轮询等待
		if err := SleepWithCancel(ctx, pollIntervalMs); err != nil {
			if ctx.Err() != nil {
				return nil // 取消时静默返回
			}
			return err
		}
	}

	errMsg := lastError
	if errMsg == "" {
		errMsg = "unknown error"
	}
	if params.Log != nil {
		params.Log("error", fmt.Sprintf("%s not ready after %dms (%s)", params.Label, timeoutMs, errMsg))
	}
	return fmt.Errorf("%s not ready (%s)", params.Label, errMsg)
}
