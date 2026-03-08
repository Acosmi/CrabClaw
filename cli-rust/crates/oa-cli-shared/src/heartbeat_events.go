package infra

import (
	"sync"
	"time"
)

// ---------- 心跳指示器类型 ----------

// HeartbeatIndicatorType 指示器类型，用于 UI 状态显示。
type HeartbeatIndicatorType string

const (
	HeartbeatIndicatorOK    HeartbeatIndicatorType = "ok"
	HeartbeatIndicatorAlert HeartbeatIndicatorType = "alert"
	HeartbeatIndicatorError HeartbeatIndicatorType = "error"
)

// ---------- 心跳事件状态 ----------

// HeartbeatStatus 心跳执行结果状态。
type HeartbeatStatus string

const (
	HeartbeatStatusSent    HeartbeatStatus = "sent"
	HeartbeatStatusOKEmpty HeartbeatStatus = "ok-empty"
	HeartbeatStatusOKToken HeartbeatStatus = "ok-token"
	HeartbeatStatusSkipped HeartbeatStatus = "skipped"
	HeartbeatStatusFailed  HeartbeatStatus = "failed"
)

// ---------- 心跳事件载荷 ----------

// HeartbeatEventPayload 心跳事件数据。
type HeartbeatEventPayload struct {
	Ts            int64                  `json:"ts"`
	Status        HeartbeatStatus        `json:"status"`
	To            string                 `json:"to,omitempty"`
	AccountID     string                 `json:"accountId,omitempty"`
	Preview       string                 `json:"preview,omitempty"`
	DurationMs    int64                  `json:"durationMs,omitempty"`
	HasMedia      bool                   `json:"hasMedia,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Channel       string                 `json:"channel,omitempty"`
	Silent        bool                   `json:"silent,omitempty"`
	IndicatorType HeartbeatIndicatorType `json:"indicatorType,omitempty"`
}

// ---------- 指示器类型解析 ----------

// ResolveIndicatorType 根据心跳状态解析指示器类型。
func ResolveIndicatorType(status HeartbeatStatus) HeartbeatIndicatorType {
	switch status {
	case HeartbeatStatusOKEmpty, HeartbeatStatusOKToken:
		return HeartbeatIndicatorOK
	case HeartbeatStatusSent:
		return HeartbeatIndicatorAlert
	case HeartbeatStatusFailed:
		return HeartbeatIndicatorError
	case HeartbeatStatusSkipped:
		return ""
	default:
		return ""
	}
}

// ---------- 事件总线 ----------

// HeartbeatEventListener 心跳事件监听器。
type HeartbeatEventListener func(evt HeartbeatEventPayload)

var (
	heartbeatMu         sync.RWMutex
	lastHeartbeatEvent  *HeartbeatEventPayload
	heartbeatListeners  = make(map[int]HeartbeatEventListener)
	heartbeatListenerID int
)

// EmitHeartbeatEvent 发出心跳事件，通知所有监听器。
func EmitHeartbeatEvent(evt HeartbeatEventPayload) {
	evt.Ts = time.Now().UnixMilli()

	heartbeatMu.Lock()
	copied := HeartbeatEventPayload(evt)
	lastHeartbeatEvent = &copied

	// 复制 listener map 以避免持锁回调
	listeners := make([]HeartbeatEventListener, 0, len(heartbeatListeners))
	for _, l := range heartbeatListeners {
		listeners = append(listeners, l)
	}
	heartbeatMu.Unlock()

	for _, listener := range listeners {
		func() {
			defer func() { recover() }() // 忽略 listener panic
			listener(evt)
		}()
	}
}

// OnHeartbeatEvent 注册心跳事件监听器。返回注销函数。
func OnHeartbeatEvent(listener HeartbeatEventListener) func() {
	heartbeatMu.Lock()
	heartbeatListenerID++
	id := heartbeatListenerID
	heartbeatListeners[id] = listener
	heartbeatMu.Unlock()

	return func() {
		heartbeatMu.Lock()
		delete(heartbeatListeners, id)
		heartbeatMu.Unlock()
	}
}

// GetLastHeartbeatEvent 获取最近一次心跳事件。
func GetLastHeartbeatEvent() *HeartbeatEventPayload {
	heartbeatMu.RLock()
	defer heartbeatMu.RUnlock()
	if lastHeartbeatEvent == nil {
		return nil
	}
	copied := *lastHeartbeatEvent
	return &copied
}

// ResetHeartbeatEvents 重置心跳事件状态（用于测试）。
func ResetHeartbeatEvents() {
	heartbeatMu.Lock()
	defer heartbeatMu.Unlock()
	lastHeartbeatEvent = nil
	heartbeatListeners = make(map[int]HeartbeatEventListener)
	heartbeatListenerID = 0
}
