package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func strPtr(s string) *string { return &s }

// ========== Batch C tests ==========

// ---------- system-presence ----------

func TestSystemPresence_EmptyStore(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())
	store := NewSystemPresenceStore()

	req := &RequestFrame{Method: "system-presence", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{PresenceStore: store}, respond)
	if !gotOK {
		t.Error("system-presence should succeed")
	}
	entries, ok := gotPayload.([]*PresenceEntry)
	if !ok {
		t.Fatalf("expected []*PresenceEntry, got %T", gotPayload)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestSystemPresence_NilStore(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())

	req := &RequestFrame{Method: "system-presence", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("system-presence should succeed even with nil store")
	}
}

// ---------- system-event ----------

func TestSystemEvent_TextRequired(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())

	req := &RequestFrame{Method: "system-event", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("system-event should fail without text")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request, got %v", gotErr)
	}
}

func TestSystemEvent_UpdatesPresence(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())
	store := NewSystemPresenceStore()

	req := &RequestFrame{Method: "system-event", Params: map[string]interface{}{
		"text":     "Node: test-device",
		"deviceId": "dev-1",
		"host":     "myhost",
		"ip":       "192.168.1.1",
	}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{PresenceStore: store}, respond)
	if !gotOK {
		t.Error("system-event should succeed")
	}
	entries := store.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Host != "myhost" {
		t.Errorf("expected host=myhost, got %q", entries[0].Host)
	}
}

// ---------- set-heartbeats ----------

func TestSetHeartbeats(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())
	hb := NewHeartbeatState()

	req := &RequestFrame{Method: "set-heartbeats", Params: map[string]interface{}{
		"enabled": false,
	}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{HeartbeatState: hb}, respond)
	if !gotOK {
		t.Error("set-heartbeats should succeed")
	}
	if hb.IsEnabled() {
		t.Error("heartbeat should be disabled")
	}
}

func TestSetHeartbeats_MissingParam(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())

	req := &RequestFrame{Method: "set-heartbeats", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("set-heartbeats should fail without enabled param")
	}
}

// ---------- last-heartbeat ----------

func TestLastHeartbeat(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SystemHandlers())
	hb := NewHeartbeatState()
	hb.SetLast(map[string]interface{}{"ts": int64(12345)})

	req := &RequestFrame{Method: "last-heartbeat", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{HeartbeatState: hb}, respond)
	if !gotOK {
		t.Error("last-heartbeat should succeed")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if m["ts"] != int64(12345) {
		t.Errorf("expected ts=12345, got %v", m["ts"])
	}
}

// ---------- channels.status ----------

func TestChannelsStatus_NoConfig(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChannelsHandlers())

	req := &RequestFrame{Method: "channels.status", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("channels.status should succeed without config")
	}
}

func TestChannelsStatus_WithConfig(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChannelsHandlers())
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Discord: &types.DiscordConfig{DiscordAccountConfig: types.DiscordAccountConfig{Token: strPtr("test-token")}},
		},
	}

	req := &RequestFrame{Method: "channels.status", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{Config: cfg}, respond)
	if !gotOK {
		t.Error("channels.status should succeed")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	channels, ok := m["channels"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels map, got %T", m["channels"])
	}
	if _, ok := channels["discord"]; !ok {
		t.Error("expected discord in channels")
	}
}

// ---------- channels.logout ----------

func TestChannelsLogout_MissingChannel(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChannelsHandlers())

	req := &RequestFrame{Method: "channels.logout", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should fail without channel")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request, got %v", gotErr)
	}
}

func TestChannelsLogout_NoCallback(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChannelsHandlers())

	req := &RequestFrame{Method: "channels.logout", Params: map[string]interface{}{
		"channel": "discord",
	}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	// stub should succeed
	if !gotOK {
		t.Error("channels.logout should succeed (stub)")
	}
}

// ---------- logs.tail ----------

func TestLogsTail_NoFile(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(LogsHandlers())

	req := &RequestFrame{Method: "logs.tail", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("logs.tail should succeed without file")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	lines, ok := m["lines"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", m["lines"])
	}
	if len(lines) != 0 {
		t.Errorf("expected 0 lines, got %d", len(lines))
	}
}

func TestLogsTail_WithFile(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(LogsHandlers())

	// Create temp log file
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")
	os.WriteFile(logFile, []byte("line1\nline2\nline3\n"), 0644)

	req := &RequestFrame{Method: "logs.tail", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{LogFilePath: logFile}, respond)
	if !gotOK {
		t.Error("logs.tail should succeed with file")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	lines, ok := m["lines"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", m["lines"])
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

// ========== Batch D tests ==========

func TestDW1Handlers_AllRegistered(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(CronHandlers())
	r.RegisterAll(TtsHandlers())
	r.RegisterAll(SkillsHandlers())
	r.RegisterAll(NodeHandlers())
	r.RegisterAll(DeviceHandlers())
	r.RegisterAll(VoiceWakeHandlers())
	r.RegisterAll(UpdateHandlers())
	r.RegisterAll(BrowserHandlers())
	r.RegisterAll(TalkHandlers())
	r.RegisterAll(WebHandlers())

	// 验证所有 DW1 方法均已注册
	testMethods := []string{
		"wake", "cron.list", "cron.status", "cron.runs",
		"cron.add", "cron.update", "cron.remove", "cron.run",
		"tts.status", "tts.providers", "tts.enable", "tts.disable",
		"tts.convert", "tts.setProvider",
		"skills.status", "skills.bins", "skills.install", "skills.update",
		"node.list", "node.describe", "node.invoke",
		"node.invoke.result", "node.event", "node.rename",
		"node.pair.request", "node.pair.list",
		"node.pair.approve", "node.pair.reject", "node.pair.verify",
		"device.pair.list", "device.pair.approve", "device.pair.reject",
		"device.token.rotate", "device.token.revoke",
		"voicewake.get", "voicewake.set",
		"update.check", "update.run",
		"desktop.update.status", "desktop.update.check",
		"desktop.update.download", "desktop.update.apply", "desktop.update.rollback", "desktop.update.dismiss",
		"browser.request",
		"talk.mode",
		"web.login.start", "web.login.wait",
	}
	for _, m := range testMethods {
		handler := r.Get(m)
		if handler == nil {
			t.Errorf("DW1 method %q not registered", m)
		}
	}
}

// ========== Batch B tests ==========

// ---------- chat.history ----------

func TestChatHistory_NoStore(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChatHandlers())

	req := &RequestFrame{Method: "chat.history", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("chat.history should fail without session store")
	}
}

func TestChatHistory_EmptySession(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChatHandlers())
	store := NewSessionStore("")

	req := &RequestFrame{Method: "chat.history", Params: map[string]interface{}{
		"sessionKey": "test:main",
	}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{SessionStore: store}, respond)
	if !gotOK {
		t.Error("chat.history should succeed")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if m["total"] != 0 {
		t.Errorf("expected total=0, got %v", m["total"])
	}
}

// ---------- chat.abort ----------

func TestChatAbort(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChatHandlers())
	chatState := NewChatRunState()

	req := &RequestFrame{Method: "chat.abort", Params: map[string]interface{}{
		"sessionKey": "test:main",
		"runId":      "run-123",
	}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ChatState: chatState}, respond)
	if !gotOK {
		t.Error("chat.abort should succeed")
	}

	// Check that the run is marked as aborted
	_, loaded := chatState.AbortedRuns.Load("run-123")
	if !loaded {
		t.Error("run should be marked as aborted")
	}
}

// ---------- chat.send ----------

func TestChatSend(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChatHandlers())
	chatState := NewChatRunState()

	req := &RequestFrame{Method: "chat.send", Params: map[string]interface{}{
		"text":       "Hello",
		"sessionKey": "test:main",
	}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ChatState: chatState}, respond)
	if !gotOK {
		t.Error("chat.send should succeed")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if m["status"] != "started" {
		t.Errorf("expected status=started, got %v", m["status"])
	}
}

// ---------- chat.inject ----------

func TestChatInject_MissingText(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ChatHandlers())

	req := &RequestFrame{Method: "chat.inject", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{SessionStore: NewSessionStore("")}, respond)
	if gotOK {
		t.Error("chat.inject should fail without text")
	}
}

// ---------- send ----------

func TestSend_MissingText(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SendHandlers())

	req := &RequestFrame{Method: "send", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("send should fail without text")
	}
}

func TestSend_MissingChannel(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SendHandlers())

	req := &RequestFrame{Method: "send", Params: map[string]interface{}{"text": "hi"}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) { gotOK = ok }
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("send should fail without channel")
	}
}

func TestSend_ValidRequest(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(SendHandlers())

	req := &RequestFrame{Method: "send", Params: map[string]interface{}{
		"message": "hello world",
		"to":      "user123",
		"channel": "discord",
	}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("send should succeed")
	}
	m, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if m["channel"] != "discord" {
		t.Errorf("expected channel=discord, got %v", m["channel"])
	}
}
