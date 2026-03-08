package infra

import (
	"strings"
	"sync"
	"time"
)

// ---------- 系统事件队列 (移植自 system-events.ts) ----------

const maxSystemEvents = 20

// SystemEvent 系统事件。
type SystemEvent struct {
	Text string `json:"text"`
	Ts   int64  `json:"ts"`
}

// sessionQueue 某个 session 的事件队列。
type sessionQueue struct {
	queue          []SystemEvent
	lastText       string
	lastContextKey string
}

// SystemEventQueue 会话级内存事件队列（线程安全）。
// 移植自 TS system-events.ts 的全局 queues Map。
type SystemEventQueue struct {
	mu     sync.Mutex
	queues map[string]*sessionQueue
}

// NewSystemEventQueue 创建系统事件队列。
func NewSystemEventQueue() *SystemEventQueue {
	return &SystemEventQueue{queues: make(map[string]*sessionQueue)}
}

// requireSessionKey 验证并清理 session key。
func requireSessionKey(key string) (string, bool) {
	trimmed := strings.TrimSpace(key)
	return trimmed, trimmed != ""
}

// normalizeContextKey 规范化上下文 key。
func normalizeContextKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

// IsContextChanged 检查指定 session 的上下文是否变更。
func (q *SystemEventQueue) IsContextChanged(sessionKey string, contextKey string) bool {
	key, ok := requireSessionKey(sessionKey)
	if !ok {
		return false
	}
	normalized := normalizeContextKey(contextKey)
	q.mu.Lock()
	defer q.mu.Unlock()
	existing, exists := q.queues[key]
	if !exists {
		return normalized != ""
	}
	return normalized != existing.lastContextKey
}

// Enqueue 添加系统事件到指定 session 的队列。
// 自动去重连续相同文本，超过 maxSystemEvents 时丢弃最旧的事件。
func (q *SystemEventQueue) Enqueue(text string, sessionKey string, contextKey string) {
	key, ok := requireSessionKey(sessionKey)
	if !ok {
		return
	}
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	entry, exists := q.queues[key]
	if !exists {
		entry = &sessionQueue{}
		q.queues[key] = entry
	}
	entry.lastContextKey = normalizeContextKey(contextKey)
	// 跳过连续相同文本
	if entry.lastText == cleaned {
		return
	}
	entry.lastText = cleaned
	entry.queue = append(entry.queue, SystemEvent{Text: cleaned, Ts: time.Now().UnixMilli()})
	if len(entry.queue) > maxSystemEvents {
		entry.queue = entry.queue[1:]
	}
}

// Drain 消费并清空指定 session 的所有事件。
func (q *SystemEventQueue) Drain(sessionKey string) []SystemEvent {
	key, ok := requireSessionKey(sessionKey)
	if !ok {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.queues[key]
	if !exists || len(entry.queue) == 0 {
		return nil
	}
	out := make([]SystemEvent, len(entry.queue))
	copy(out, entry.queue)
	delete(q.queues, key)
	return out
}

// DrainTexts 消费并返回指定 session 的事件文本。
func (q *SystemEventQueue) DrainTexts(sessionKey string) []string {
	events := q.Drain(sessionKey)
	texts := make([]string, len(events))
	for i, e := range events {
		texts[i] = e.Text
	}
	return texts
}

// Peek 查看指定 session 的事件文本（不消费）。
func (q *SystemEventQueue) Peek(sessionKey string) []string {
	key, ok := requireSessionKey(sessionKey)
	if !ok {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.queues[key]
	if !exists {
		return nil
	}
	texts := make([]string, len(entry.queue))
	for i, e := range entry.queue {
		texts[i] = e.Text
	}
	return texts
}

// Has 检查指定 session 是否有待处理事件。
func (q *SystemEventQueue) Has(sessionKey string) bool {
	key, ok := requireSessionKey(sessionKey)
	if !ok {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.queues[key]
	return exists && len(entry.queue) > 0
}

// Reset 清除所有队列（用于测试）。
func (q *SystemEventQueue) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queues = make(map[string]*sessionQueue)
}
