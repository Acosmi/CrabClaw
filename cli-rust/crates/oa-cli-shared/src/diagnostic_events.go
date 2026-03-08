package infra

// diagnostic_events.go — 诊断事件系统
// 对应 TS: src/infra/diagnostic-events.ts (179L)
//
// 提供 12 种诊断事件类型的发射/监听系统。
// 供 Gateway 调试面板和运维监控使用。

import (
	"sync"
	"sync/atomic"
	"time"
)

// ─── 事件类型定义 ───

// DiagnosticSessionState 会话状态。
type DiagnosticSessionState string

const (
	DiagSessionIdle       DiagnosticSessionState = "idle"
	DiagSessionProcessing DiagnosticSessionState = "processing"
	DiagSessionWaiting    DiagnosticSessionState = "waiting"
)

// DiagnosticEventType 诊断事件类型。
type DiagnosticEventType string

const (
	DiagEventModelUsage          DiagnosticEventType = "model.usage"
	DiagEventWebhookReceived     DiagnosticEventType = "webhook.received"
	DiagEventWebhookProcessed    DiagnosticEventType = "webhook.processed"
	DiagEventWebhookError        DiagnosticEventType = "webhook.error"
	DiagEventMessageQueued       DiagnosticEventType = "message.queued"
	DiagEventMessageProcessed    DiagnosticEventType = "message.processed"
	DiagEventSessionState        DiagnosticEventType = "session.state"
	DiagEventSessionStuck        DiagnosticEventType = "session.stuck"
	DiagEventQueueLaneEnqueue    DiagnosticEventType = "queue.lane.enqueue"
	DiagEventQueueLaneDequeue    DiagnosticEventType = "queue.lane.dequeue"
	DiagEventRunAttempt          DiagnosticEventType = "run.attempt"
	DiagEventDiagnosticHeartbeat DiagnosticEventType = "diagnostic.heartbeat"
)

// DiagnosticEvent 诊断事件（通用载体）。
type DiagnosticEvent struct {
	Type DiagnosticEventType `json:"type"`
	Ts   int64               `json:"ts"`
	Seq  int64               `json:"seq"`

	// 通用字段（按事件类型可选使用）
	SessionKey string `json:"sessionKey,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`

	// model.usage 字段
	Usage      *DiagnosticUsageData   `json:"usage,omitempty"`
	Context    *DiagnosticContextData `json:"context,omitempty"`
	CostUsd    float64                `json:"costUsd,omitempty"`
	DurationMs int64                  `json:"durationMs,omitempty"`

	// webhook 字段
	UpdateType string `json:"updateType,omitempty"`
	ChatID     string `json:"chatId,omitempty"`
	Error      string `json:"error,omitempty"`

	// message 字段
	MessageID  string `json:"messageId,omitempty"`
	Source     string `json:"source,omitempty"`
	QueueDepth int    `json:"queueDepth,omitempty"`
	Outcome    string `json:"outcome,omitempty"` // "completed" | "skipped" | "error"
	Reason     string `json:"reason,omitempty"`

	// session.state 字段
	PrevState *DiagnosticSessionState `json:"prevState,omitempty"`
	State     DiagnosticSessionState  `json:"state,omitempty"`
	AgeMs     int64                   `json:"ageMs,omitempty"`

	// queue.lane 字段
	Lane      string `json:"lane,omitempty"`
	QueueSize int    `json:"queueSize,omitempty"`
	WaitMs    int64  `json:"waitMs,omitempty"`

	// run.attempt 字段
	RunID   string `json:"runId,omitempty"`
	Attempt int    `json:"attempt,omitempty"`

	// diagnostic.heartbeat 字段
	Webhooks *DiagnosticWebhookCounters `json:"webhooks,omitempty"`
	Active   int                        `json:"active,omitempty"`
	Waiting  int                        `json:"waiting,omitempty"`
	Queued   int                        `json:"queued,omitempty"`
}

// DiagnosticUsageData token 用量数据。
type DiagnosticUsageData struct {
	Input        int `json:"input,omitempty"`
	Output       int `json:"output,omitempty"`
	CacheRead    int `json:"cacheRead,omitempty"`
	CacheWrite   int `json:"cacheWrite,omitempty"`
	PromptTokens int `json:"promptTokens,omitempty"`
	Total        int `json:"total,omitempty"`
}

// DiagnosticContextData 上下文窗口数据。
type DiagnosticContextData struct {
	Limit int `json:"limit,omitempty"`
	Used  int `json:"used,omitempty"`
}

// DiagnosticWebhookCounters webhook 计数器。
type DiagnosticWebhookCounters struct {
	Received  int `json:"received"`
	Processed int `json:"processed"`
	Errors    int `json:"errors"`
}

// ─── 全局事件系统 ───

// DiagnosticEventListener 诊断事件监听器。
type DiagnosticEventListener func(evt DiagnosticEvent)

var (
	diagSeq       atomic.Int64
	diagMu        sync.RWMutex
	diagListeners = make(map[int]DiagnosticEventListener)
	diagNextID    int
	diagEnabled   atomic.Bool
)

// SetDiagnosticsEnabled 启用/禁用诊断事件系统。
func SetDiagnosticsEnabled(enabled bool) {
	diagEnabled.Store(enabled)
}

// IsDiagnosticsEnabled 检查诊断系统是否启用。
// 对应 TS: isDiagnosticsEnabled(config)
func IsDiagnosticsEnabled() bool {
	return diagEnabled.Load()
}

// EmitDiagnosticEvent 发射诊断事件。
// 对应 TS: emitDiagnosticEvent(event)
//
// 自动附加 ts 和 seq 字段。如果诊断未启用则静默忽略。
func EmitDiagnosticEvent(evt DiagnosticEvent) {
	if !diagEnabled.Load() {
		return
	}
	evt.Seq = diagSeq.Add(1)
	evt.Ts = time.Now().UnixMilli()

	diagMu.RLock()
	listeners := make([]DiagnosticEventListener, 0, len(diagListeners))
	for _, l := range diagListeners {
		listeners = append(listeners, l)
	}
	diagMu.RUnlock()

	for _, listener := range listeners {
		func() {
			defer func() { recover() }() // 忽略 listener panic
			listener(evt)
		}()
	}
}

// OnDiagnosticEvent 注册诊断事件监听器。
// 返回取消函数。
// 对应 TS: onDiagnosticEvent(listener)
func OnDiagnosticEvent(listener DiagnosticEventListener) func() {
	diagMu.Lock()
	id := diagNextID
	diagNextID++
	diagListeners[id] = listener
	diagMu.Unlock()

	return func() {
		diagMu.Lock()
		delete(diagListeners, id)
		diagMu.Unlock()
	}
}

// ResetDiagnosticEventsForTest 重置诊断事件系统（仅测试）。
func ResetDiagnosticEventsForTest() {
	diagSeq.Store(0)
	diagMu.Lock()
	diagListeners = make(map[int]DiagnosticEventListener)
	diagNextID = 0
	diagMu.Unlock()
	diagEnabled.Store(false)
}
