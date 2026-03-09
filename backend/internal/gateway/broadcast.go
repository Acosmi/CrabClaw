package gateway

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// ---------- 常量 ----------

const (
	// MaxBufferedBytes 每连接发送缓冲上限（1.5 MB）。
	MaxBufferedBytes = 1572864 // 1.5 * 1024 * 1024
	// MaxPayloadBytes WebSocket 最大消息大小（512 KB，对齐 TS server-constants.ts）。
	MaxPayloadBytes = 512 * 1024
)

// ---------- WebSocket 客户端类型 ----------

// ConnectParams 连接参数（来自 protocol 层）。
type ConnectParams struct {
	Role   string   `json:"role,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
}

// WsClient 表示一个已连接的 WebSocket 客户端。
type WsClient struct {
	ConnID      string
	Connect     ConnectParams
	PresenceKey string
	ClientIP    string

	// Send 发送消息到客户端。返回 error 表示发送失败。
	Send func(data []byte) error
	// Close 关闭连接。
	Close func(code int, reason string) error
	// BufferedAmount 返回当前写缓冲区字节数。
	BufferedAmount func() int64
}

// ---------- 事件范围守卫 ----------

const (
	scopeAdmin     = "operator.admin"
	scopeApprovals = "operator.approvals"
	scopePairing   = "operator.pairing"
)

var eventScopeGuards = map[string][]string{
	"exec.approval.requested":   {scopeApprovals},
	"exec.approval.resolved":    {scopeApprovals},
	"coder.confirm.requested":   {scopeApprovals},
	"coder.confirm.resolved":    {scopeApprovals},
	"approval.workflow.updated": {scopeApprovals},
	"device.pair.requested":     {scopePairing},
	"device.pair.resolved":      {scopePairing},
	"node.pair.requested":       {scopePairing},
	"node.pair.resolved":        {scopePairing},
}

func hasEventScope(client *WsClient, event string) bool {
	required, exists := eventScopeGuards[event]
	if !exists {
		return true
	}
	role := client.Connect.Role
	if role == "" {
		role = "operator"
	}
	if role != "operator" {
		return false
	}
	for _, scope := range client.Connect.Scopes {
		if scope == scopeAdmin {
			return true
		}
		for _, req := range required {
			if scope == req {
				return true
			}
		}
	}
	return false
}

// ---------- 广播器 ----------

// BroadcastOptions 广播选项。
type BroadcastOptions struct {
	DropIfSlow   bool
	StateVersion *StateVersion
}

// StateVersion 状态版本。
type StateVersion struct {
	Presence int64 `json:"presence,omitempty"`
	Health   int64 `json:"health,omitempty"`
}

// eventFrame 是广播的 JSON 帧结构。
type eventFrame struct {
	Type         string        `json:"type"`
	Event        string        `json:"event"`
	Payload      interface{}   `json:"payload"`
	Seq          *int64        `json:"seq,omitempty"`
	StateVersion *StateVersion `json:"stateVersion,omitempty"`
}

// Broadcaster 网关广播器，向所有或目标连接广播事件。
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[string]*WsClient // connID → client
	seq     atomic.Int64
}

// NewBroadcaster 创建广播器。
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[string]*WsClient),
	}
}

// AddClient 注册客户端。
func (b *Broadcaster) AddClient(client *WsClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[client.ConnID] = client
}

// RemoveClient 移除客户端。
func (b *Broadcaster) RemoveClient(connID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, connID)
}

// ClientCount 返回当前客户端数量。
func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// Broadcast 向所有已连接客户端广播事件。
func (b *Broadcaster) Broadcast(event string, payload interface{}, opts *BroadcastOptions) {
	b.broadcastInternal(event, payload, opts, nil)
}

// BroadcastToConnIDs 向指定连接 ID 集合广播事件。
func (b *Broadcaster) BroadcastToConnIDs(event string, payload interface{}, connIDs map[string]struct{}, opts *BroadcastOptions) {
	if len(connIDs) == 0 {
		return
	}
	b.broadcastInternal(event, payload, opts, connIDs)
}

func (b *Broadcaster) broadcastInternal(event string, payload interface{}, opts *BroadcastOptions, targetConnIDs map[string]struct{}) {
	isTargeted := targetConnIDs != nil

	var seqVal *int64
	if !isTargeted {
		v := b.seq.Add(1)
		seqVal = &v
	}

	var sv *StateVersion
	if opts != nil {
		sv = opts.StateVersion
	}

	frame := eventFrame{
		Type:         "event",
		Event:        event,
		Payload:      payload,
		Seq:          seqVal,
		StateVersion: sv,
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}

	dropIfSlow := opts != nil && opts.DropIfSlow

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, c := range b.clients {
		if targetConnIDs != nil {
			if _, ok := targetConnIDs[c.ConnID]; !ok {
				continue
			}
		}
		if !hasEventScope(c, event) {
			continue
		}

		buffered := int64(0)
		if c.BufferedAmount != nil {
			buffered = c.BufferedAmount()
		}
		slow := buffered > MaxBufferedBytes

		if slow && dropIfSlow {
			continue
		}
		if slow {
			if c.Close != nil {
				c.Close(1008, "slow consumer")
			}
			continue
		}
		if c.Send != nil {
			c.Send(data)
		}
	}
}
