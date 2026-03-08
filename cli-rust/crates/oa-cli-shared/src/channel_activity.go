package infra

// channel_activity.go — 频道活跃度追踪
// 对应 TS: src/infra/channel-activity.ts (58L)
//
// 内存级频道入站/出站活跃时间追踪。
// 线程安全，供心跳和诊断模块使用。

import (
	"sync"
	"time"
)

// ChannelDirection 频道消息方向。
type ChannelDirection string

const (
	ChannelDirectionInbound  ChannelDirection = "inbound"
	ChannelDirectionOutbound ChannelDirection = "outbound"
)

// ActivityEntry 单个频道的活跃时间记录。
type ActivityEntry struct {
	InboundAt  *time.Time `json:"inboundAt,omitempty"`
	OutboundAt *time.Time `json:"outboundAt,omitempty"`
}

// channelActivityStore 全局频道活跃度存储。
type channelActivityStore struct {
	mu       sync.RWMutex
	activity map[string]*ActivityEntry
}

var globalChannelActivity = &channelActivityStore{
	activity: make(map[string]*ActivityEntry),
}

func channelActivityKey(channel, accountID string) string {
	if accountID == "" {
		accountID = "default"
	}
	return channel + ":" + accountID
}

func (s *channelActivityStore) ensureEntry(channel, accountID string) *ActivityEntry {
	key := channelActivityKey(channel, accountID)
	entry, ok := s.activity[key]
	if !ok {
		entry = &ActivityEntry{}
		s.activity[key] = entry
	}
	return entry
}

// RecordChannelActivityParams 记录频道活跃度的参数。
type RecordChannelActivityParams struct {
	Channel   string
	AccountID string
	Direction ChannelDirection
	At        *time.Time
}

// RecordChannelActivity 记录频道活跃时间。
// 对应 TS: recordChannelActivity(params)
func RecordChannelActivity(params RecordChannelActivityParams) {
	at := time.Now()
	if params.At != nil {
		at = *params.At
	}
	accountID := params.AccountID
	if accountID == "" {
		accountID = "default"
	}

	globalChannelActivity.mu.Lock()
	defer globalChannelActivity.mu.Unlock()

	entry := globalChannelActivity.ensureEntry(params.Channel, accountID)
	switch params.Direction {
	case ChannelDirectionInbound:
		entry.InboundAt = &at
	case ChannelDirectionOutbound:
		entry.OutboundAt = &at
	}
}

// GetChannelActivityParams 查询频道活跃度的参数。
type GetChannelActivityParams struct {
	Channel   string
	AccountID string
}

// GetChannelActivity 获取频道活跃时间。
// 对应 TS: getChannelActivity(params)
func GetChannelActivity(params GetChannelActivityParams) ActivityEntry {
	accountID := params.AccountID
	if accountID == "" {
		accountID = "default"
	}

	globalChannelActivity.mu.RLock()
	defer globalChannelActivity.mu.RUnlock()

	key := channelActivityKey(params.Channel, accountID)
	entry, ok := globalChannelActivity.activity[key]
	if !ok {
		return ActivityEntry{}
	}
	return *entry
}

// ResetChannelActivityForTest 清空活跃度数据（仅测试使用）。
func ResetChannelActivityForTest() {
	globalChannelActivity.mu.Lock()
	defer globalChannelActivity.mu.Unlock()
	globalChannelActivity.activity = make(map[string]*ActivityEntry)
}
