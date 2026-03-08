package infra

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- 指示器类型测试 ----------

func TestResolveIndicatorType(t *testing.T) {
	cases := []struct {
		status HeartbeatStatus
		want   HeartbeatIndicatorType
	}{
		{HeartbeatStatusOKEmpty, HeartbeatIndicatorOK},
		{HeartbeatStatusOKToken, HeartbeatIndicatorOK},
		{HeartbeatStatusSent, HeartbeatIndicatorAlert},
		{HeartbeatStatusFailed, HeartbeatIndicatorError},
		{HeartbeatStatusSkipped, ""},
		{HeartbeatStatus("unknown"), ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got := ResolveIndicatorType(tc.status)
			if got != tc.want {
				t.Errorf("ResolveIndicatorType(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

// ---------- 事件总线测试 ----------

func TestEmitHeartbeatEvent_NotifiesListeners(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	var received HeartbeatEventPayload
	unsub := OnHeartbeatEvent(func(evt HeartbeatEventPayload) {
		received = evt
	})
	defer unsub()

	EmitHeartbeatEvent(HeartbeatEventPayload{
		Status:  HeartbeatStatusSent,
		To:      "user@test.com",
		Preview: "hello",
	})

	if received.Status != HeartbeatStatusSent {
		t.Errorf("expected status %q, got %q", HeartbeatStatusSent, received.Status)
	}
	if received.To != "user@test.com" {
		t.Errorf("expected to %q, got %q", "user@test.com", received.To)
	}
	if received.Ts == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestEmitHeartbeatEvent_SetsTimestamp(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	now := time.Now().UnixMilli()
	EmitHeartbeatEvent(HeartbeatEventPayload{Status: HeartbeatStatusOKEmpty})

	last := GetLastHeartbeatEvent()
	if last == nil {
		t.Fatal("expected last event, got nil")
	}
	if last.Ts < now || last.Ts > now+1000 {
		t.Errorf("timestamp %d not near now %d", last.Ts, now)
	}
}

func TestOnHeartbeatEvent_UnsubWorks(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	var count int32
	unsub := OnHeartbeatEvent(func(evt HeartbeatEventPayload) {
		atomic.AddInt32(&count, 1)
	})
	EmitHeartbeatEvent(HeartbeatEventPayload{Status: HeartbeatStatusSent})
	unsub()
	EmitHeartbeatEvent(HeartbeatEventPayload{Status: HeartbeatStatusSent})

	if c := atomic.LoadInt32(&count); c != 1 {
		t.Errorf("expected 1 call, got %d", c)
	}
}

func TestGetLastHeartbeatEvent_NilWhenNoEvents(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	if last := GetLastHeartbeatEvent(); last != nil {
		t.Errorf("expected nil, got %+v", last)
	}
}

func TestEmitHeartbeatEvent_MultipleListeners(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	var count int32
	for i := 0; i < 5; i++ {
		unsub := OnHeartbeatEvent(func(evt HeartbeatEventPayload) {
			atomic.AddInt32(&count, 1)
		})
		defer unsub()
	}

	EmitHeartbeatEvent(HeartbeatEventPayload{Status: HeartbeatStatusSent})
	if c := atomic.LoadInt32(&count); c != 5 {
		t.Errorf("expected 5 calls, got %d", c)
	}
}

func TestEmitHeartbeatEvent_ListenerPanicDoesNotPropagate(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	OnHeartbeatEvent(func(evt HeartbeatEventPayload) {
		panic("test panic")
	})

	// 不应该 panic
	EmitHeartbeatEvent(HeartbeatEventPayload{Status: HeartbeatStatusSent})
}

func TestEmitHeartbeatEvent_ConcurrentSafety(t *testing.T) {
	ResetHeartbeatEvents()
	defer ResetHeartbeatEvents()

	var count int64
	unsub := OnHeartbeatEvent(func(evt HeartbeatEventPayload) {
		atomic.AddInt64(&count, 1)
	})
	defer unsub()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			EmitHeartbeatEvent(HeartbeatEventPayload{Status: HeartbeatStatusSent})
		}()
	}
	wg.Wait()

	if c := atomic.LoadInt64(&count); c != 100 {
		t.Errorf("expected 100 calls, got %d", c)
	}
}

// ---------- 可见性测试 ----------

type mockVisibilityConfig struct {
	channelDefaults *ChannelHeartbeatVisibilityConfig
	perChannel      map[string]*ChannelHeartbeatVisibilityConfig
	perAccount      map[string]*ChannelHeartbeatVisibilityConfig // key: channel:accountId
}

func (m *mockVisibilityConfig) ChannelDefaultsHeartbeat() *ChannelHeartbeatVisibilityConfig {
	return m.channelDefaults
}

func (m *mockVisibilityConfig) PerChannelHeartbeat(channel string) *ChannelHeartbeatVisibilityConfig {
	if m.perChannel == nil {
		return nil
	}
	return m.perChannel[channel]
}

func (m *mockVisibilityConfig) PerAccountHeartbeat(channel, accountID string) *ChannelHeartbeatVisibilityConfig {
	if m.perAccount == nil {
		return nil
	}
	return m.perAccount[channel+":"+accountID]
}

func boolPtr(v bool) *bool { return &v }

func TestResolveHeartbeatVisibility_Defaults(t *testing.T) {
	cfg := &mockVisibilityConfig{}
	vis := ResolveHeartbeatVisibility(cfg, "slack", "")
	if vis.ShowOk {
		t.Error("expected ShowOk=false by default")
	}
	if !vis.ShowAlerts {
		t.Error("expected ShowAlerts=true by default")
	}
	if !vis.UseIndicator {
		t.Error("expected UseIndicator=true by default")
	}
}

func TestResolveHeartbeatVisibility_ChannelDefaultsOverride(t *testing.T) {
	cfg := &mockVisibilityConfig{
		channelDefaults: &ChannelHeartbeatVisibilityConfig{
			ShowOk: boolPtr(true),
		},
	}
	vis := ResolveHeartbeatVisibility(cfg, "slack", "")
	if !vis.ShowOk {
		t.Error("expected ShowOk=true from channelDefaults")
	}
}

func TestResolveHeartbeatVisibility_PerChannelOverride(t *testing.T) {
	cfg := &mockVisibilityConfig{
		channelDefaults: &ChannelHeartbeatVisibilityConfig{
			ShowOk: boolPtr(false),
		},
		perChannel: map[string]*ChannelHeartbeatVisibilityConfig{
			"slack": {ShowOk: boolPtr(true)},
		},
	}
	vis := ResolveHeartbeatVisibility(cfg, "slack", "")
	if !vis.ShowOk {
		t.Error("expected perChannel to override channelDefaults")
	}
}

func TestResolveHeartbeatVisibility_PerAccountOverride(t *testing.T) {
	cfg := &mockVisibilityConfig{
		perChannel: map[string]*ChannelHeartbeatVisibilityConfig{
			"slack": {ShowAlerts: boolPtr(true)},
		},
		perAccount: map[string]*ChannelHeartbeatVisibilityConfig{
			"slack:acct1": {ShowAlerts: boolPtr(false)},
		},
	}
	vis := ResolveHeartbeatVisibility(cfg, "slack", "acct1")
	if vis.ShowAlerts {
		t.Error("expected perAccount to override perChannel")
	}
}

func TestResolveHeartbeatVisibility_WebchatOnlyUsesChannelDefaults(t *testing.T) {
	cfg := &mockVisibilityConfig{
		channelDefaults: &ChannelHeartbeatVisibilityConfig{
			ShowOk:     boolPtr(true),
			ShowAlerts: boolPtr(false),
		},
		perChannel: map[string]*ChannelHeartbeatVisibilityConfig{
			"webchat": {ShowOk: boolPtr(false)}, // 应被忽略
		},
	}
	vis := ResolveHeartbeatVisibility(cfg, "webchat", "")
	// webchat 只用 channelDefaults
	if !vis.ShowOk {
		t.Error("expected channelDefaults.ShowOk=true for webchat")
	}
	if vis.ShowAlerts {
		t.Error("expected channelDefaults.ShowAlerts=false for webchat")
	}
}
