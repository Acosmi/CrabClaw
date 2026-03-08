package email

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// --- Mock IMAP Connector ---

type mockIMAPConnector struct {
	mu              sync.Mutex
	connected       bool
	connectErr      error
	disconnectCount int
	selectedMailbox string
	mailboxStatus   *MailboxStatus
	selectErr       error

	// FetchNewMessages 返回值
	fetchMessages []RawEmailMessage
	fetchErr      error
	fetchCalls    int
	fetchAfterUID uint32 // 记录最后一次调用的 afterUID

	// IDLE
	idleErr   error
	idleCalls int

	// NOOP
	noopErr   error
	noopCalls int
}

func newMockConnector() *mockIMAPConnector {
	return &mockIMAPConnector{
		mailboxStatus: &MailboxStatus{
			UIDValidity: 1000,
			UIDNext:     imap.UID(100),
			Messages:    50,
		},
	}
}

func (m *mockIMAPConnector) Connect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	return nil
}

func (m *mockIMAPConnector) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	m.disconnectCount++
	return nil
}

func (m *mockIMAPConnector) SelectMailbox(_ context.Context, name string) (*MailboxStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.selectErr != nil {
		return nil, m.selectErr
	}
	m.selectedMailbox = name
	return m.mailboxStatus, nil
}

func (m *mockIMAPConnector) FetchNewMessages(_ context.Context, afterUID uint32, _ int) ([]RawEmailMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchCalls++
	m.fetchAfterUID = afterUID
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	// 每次调用后清空消息（模拟只返回一次新消息）
	msgs := m.fetchMessages
	m.fetchMessages = nil
	return msgs, nil
}

func (m *mockIMAPConnector) WaitIdle(_ context.Context, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idleCalls++
	return m.idleErr
}

func (m *mockIMAPConnector) Noop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.noopCalls++
	return m.noopErr
}

func (m *mockIMAPConnector) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockIMAPConnector) setFetchMessages(msgs []RawEmailMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchMessages = msgs
}

func (m *mockIMAPConnector) getFetchCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetchCalls
}

func (m *mockIMAPConnector) getIdleCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idleCalls
}

func (m *mockIMAPConnector) getNoopCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.noopCalls
}

func (m *mockIMAPConnector) getDisconnectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disconnectCount
}

func (m *mockIMAPConnector) getSelectedMailbox() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.selectedMailbox
}

func (m *mockIMAPConnector) getFetchAfterUID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetchAfterUID
}

// --- Tests ---

func newTestIMAPConfig() *types.EmailAccountConfig {
	boolTrue := true
	return &types.EmailAccountConfig{
		Enabled:  &boolTrue,
		Provider: types.EmailProviderAliyun,
		Address:  "test@company.com",
		Login:    "test@company.com",
		Auth: types.EmailAuthConfig{
			Mode:     types.EmailAuthAppPassword,
			Password: "test-pass",
		},
		IMAP: &types.EmailIMAPConfig{
			Host:                "imap.test.com",
			Port:                993,
			Security:            types.EmailSecurityTLS,
			Mode:                types.EmailIMAPModePoll,
			Mailboxes:           []string{"INBOX"},
			PollIntervalSeconds: 1, // 短间隔方便测试
			IdleRestartMinutes:  25,
			FetchBatchSize:      20,
		},
	}
}

func TestRunner_PollMode_FetchNewMessages(t *testing.T) {
	mock := newMockConnector()
	mock.setFetchMessages([]RawEmailMessage{
		{UID: 101, Header: map[string][]string{"Subject": {"Hello"}}, Size: 100},
		{UID: 102, Header: map[string][]string{"Subject": {"World"}}, Size: 200},
	})

	cfg := newTestIMAPConfig()
	cfg.IMAP.Mode = types.EmailIMAPModePoll

	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)

	// 记录收到的邮件
	var received []RawEmailMessage
	var mu sync.Mutex
	runner.SetOnNewMail(func(_ string, msgs []RawEmailMessage) {
		mu.Lock()
		received = append(received, msgs...)
		mu.Unlock()
	})

	// 使用 state store 验证持久化
	tmp := t.TempDir()
	runner.SetStateStore(NewStateStore(tmp, "test"))

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	// 启动 runner，等一段时间让它跑 poll 循环
	go runner.runLoop()
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// 检查收到邮件
	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 2 {
		t.Errorf("received %d messages, want 2", count)
	}

	// 检查 NOOP 被调用
	if mock.getNoopCalls() == 0 {
		t.Error("NOOP should have been called at least once")
	}

	// 检查 SELECT 了 INBOX
	if mock.getSelectedMailbox() != "INBOX" {
		t.Errorf("selected mailbox = %q, want INBOX", mock.getSelectedMailbox())
	}

	// 检查状态持久化
	store := NewStateStore(tmp, "test")
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	if state == nil {
		t.Fatal("state should be persisted")
	}
	if state.LastSeenUID != 102 {
		t.Errorf("LastSeenUID = %d, want 102", state.LastSeenUID)
	}
	if state.UIDValidity != 1000 {
		t.Errorf("UIDValidity = %d, want 1000", state.UIDValidity)
	}
}

func TestRunner_IdleMode_DegradeOnFailure(t *testing.T) {
	mock := newMockConnector()
	mock.idleErr = fmt.Errorf("IDLE not supported")

	cfg := newTestIMAPConfig()
	cfg.IMAP.Mode = types.EmailIMAPModeAuto // auto: 先尝试 IDLE，失败降级 Poll

	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	go runner.runLoop()
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// IDLE 失败后应降级到 Poll，NOOP 应被调用
	if mock.getIdleCalls() == 0 {
		t.Error("IDLE should have been attempted")
	}
	if mock.getNoopCalls() == 0 {
		t.Error("After IDLE degradation, NOOP (poll) should run")
	}
}

func TestRunner_ConnectFailure_Backoff(t *testing.T) {
	mock := newMockConnector()
	mock.connectErr = fmt.Errorf("connection refused")

	cfg := newTestIMAPConfig()
	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	go runner.runLoop()
	// 等待足够让第一次退避开始
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// 应该进入 backoff 状态
	state := runner.State()
	if state != RunnerStateStopped {
		t.Errorf("State = %q, want stopped after cancel", state)
	}

	// 应该记录了失败
	if runner.ConsecutiveFailures() == 0 {
		t.Error("ConsecutiveFailures should be > 0")
	}
}

func TestRunner_UIDValidityChange(t *testing.T) {
	mock := newMockConnector()
	mock.mailboxStatus = &MailboxStatus{
		UIDValidity: 2000, // 不同于 state 中的 1000
		UIDNext:     imap.UID(50),
		Messages:    30,
	}

	cfg := newTestIMAPConfig()
	cfg.IMAP.Mode = types.EmailIMAPModePoll

	tmp := t.TempDir()
	store := NewStateStore(tmp, "test")

	// 预写一个 state（UIDValidity=1000）
	existing := &AccountState{
		UIDValidity: 1000,
		LastSeenUID: 500,
	}
	if err := store.Save(existing); err != nil {
		t.Fatalf("Save: %v", err)
	}

	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)
	runner.SetStateStore(store)

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	go runner.runLoop()
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// 检查 state 被重置
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state == nil {
		t.Fatal("state should exist")
	}
	if state.UIDValidity != 2000 {
		t.Errorf("UIDValidity = %d, want 2000", state.UIDValidity)
	}
	// lastSeenUID 应被重置为 uidNext-1 = 49 (bootstrap)
	if state.LastSeenUID != 49 {
		t.Errorf("LastSeenUID = %d, want 49 (bootstrap after UIDVALIDITY change)", state.LastSeenUID)
	}
}

func TestRunner_BootstrapMode(t *testing.T) {
	mock := newMockConnector()
	mock.mailboxStatus = &MailboxStatus{
		UIDValidity: 1000,
		UIDNext:     imap.UID(200),
		Messages:    100,
	}

	cfg := newTestIMAPConfig()
	cfg.IMAP.Mode = types.EmailIMAPModePoll

	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)

	tmp := t.TempDir()
	store := NewStateStore(tmp, "test")
	runner.SetStateStore(store)

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	go runner.runLoop()
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Bootstrap: lastSeenUID = uidNext - 1 = 199
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state == nil {
		t.Fatal("state should exist")
	}
	if state.LastSeenUID != 199 {
		t.Errorf("LastSeenUID = %d, want 199 (bootstrap)", state.LastSeenUID)
	}

	// FetchNewMessages 应以 afterUID=199 调用
	if mock.getFetchAfterUID() != 199 {
		t.Errorf("fetchAfterUID = %d, want 199", mock.getFetchAfterUID())
	}
}

func TestRunner_SelectFailure_Backoff(t *testing.T) {
	mock := newMockConnector()
	mock.selectErr = fmt.Errorf("mailbox not found")

	cfg := newTestIMAPConfig()
	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	go runner.runLoop()
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// 连接成功但 SELECT 失败 → 退避重试
	if mock.getDisconnectCount() == 0 {
		t.Error("Disconnect should have been called after SELECT failure")
	}
}

func TestRunner_FetchFailure_Reconnect(t *testing.T) {
	mock := newMockConnector()
	mock.fetchErr = fmt.Errorf("connection reset")

	cfg := newTestIMAPConfig()
	cfg.IMAP.Mode = types.EmailIMAPModePoll

	runner := NewAccountRunner("test", cfg)
	runner.SetIMAPConnector(mock)

	ctx, cancel := context.WithCancel(context.Background())
	runner.ctx = ctx
	runner.cancel = cancel

	go runner.runLoop()
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Fetch 失败应导致断开并重连
	if mock.getDisconnectCount() < 1 {
		t.Error("Disconnect should have been called after fetch failure")
	}
}

func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		failures int
		min      time.Duration
		max      time.Duration
	}{
		{0, 5 * time.Second, 5 * time.Second},
		{1, 10 * time.Second, 10 * time.Second},
		{2, 20 * time.Second, 20 * time.Second},
		{3, 40 * time.Second, 40 * time.Second},
		{6, 5 * time.Minute, 5 * time.Minute},  // capped
		{10, 5 * time.Minute, 5 * time.Minute}, // capped
	}
	for _, tt := range tests {
		d := backoffDuration(tt.failures)
		if d < tt.min || d > tt.max {
			t.Errorf("backoffDuration(%d) = %v, want [%v, %v]", tt.failures, d, tt.min, tt.max)
		}
	}
}
