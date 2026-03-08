package email

// integration_test.go — Phase 10: 端到端集成测试
// 使用 imap_client_test.go 中的 mockIMAPConnector 验证完整收件/发件管线

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// --- Helper: 构建测试 RawEmailMessage ---

func buildTestRawEmail(uid uint32, from, to, subject, messageID, body string) RawEmailMessage {
	rawBody := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: %s\r\nDate: Mon, 08 Mar 2026 10:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, subject, messageID, body)

	return RawEmailMessage{
		UID:  uid,
		Size: uint32(len(rawBody)),
		Header: map[string][]string{
			"From":       {from},
			"To":         {to},
			"Subject":    {subject},
			"Message-Id": {messageID},
		},
		Body: []byte(rawBody),
	}
}

// --- Integration Tests ---

// TestInteg_InboundPipeline_EndToEnd 完整收件管线：Filter → Parse → Thread → ChannelMessage
func TestInteg_InboundPipeline_EndToEnd(t *testing.T) {
	rawMsg := buildTestRawEmail(
		100,
		"sender@example.com",
		"bot@company.com",
		"Hello from integration test",
		"<test-001@example.com>",
		"This is a test email body.",
	)

	// 入站过滤
	filterCfg := FilterConfig{
		IgnoreAutoSubmitted: true,
		IgnoreMailingList:   true,
		IgnoreNoReply:       true,
		IgnoreSelfSent:      true,
		SelfAddress:         "bot@company.com",
	}

	passed, filtered := FilterInbound([]RawEmailMessage{rawMsg}, filterCfg)
	if filtered != 0 {
		t.Fatalf("Expected 0 filtered, got %d", filtered)
	}
	if len(passed) != 1 {
		t.Fatalf("Expected 1 passed, got %d", len(passed))
	}

	// MIME 解析
	parsed, err := ParseEmail(passed[0].Body, DefaultParseLimits())
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}
	if parsed.Subject != "Hello from integration test" {
		t.Errorf("Subject = %q, want %q", parsed.Subject, "Hello from integration test")
	}
	if !strings.Contains(parsed.TextBody, "test email body") {
		t.Errorf("TextBody should contain 'test email body', got %q", parsed.TextBody)
	}

	// 构建 InboundMessage
	inbound := BuildInboundMessage("test-account", "aliyun", "INBOX", passed[0], parsed, 12345)
	if inbound.From != "sender@example.com" {
		t.Errorf("From = %q, want %q", inbound.From, "sender@example.com")
	}
	if inbound.Subject != "Hello from integration test" {
		t.Errorf("Subject = %q", inbound.Subject)
	}

	// 线程路由
	sessionKey := ResolveSessionKey("test-account", inbound)
	if sessionKey == "" {
		t.Fatal("ResolveSessionKey should return non-empty key")
	}
	if !strings.HasPrefix(sessionKey, "email:test-account:") {
		t.Errorf("sessionKey should start with 'email:test-account:', got %q", sessionKey)
	}

	// 转换为 ChannelMessage
	chMsg := ToChannelMessage(inbound)
	if chMsg == nil {
		t.Fatal("ToChannelMessage should return non-nil")
	}
	if chMsg.Text == "" {
		t.Error("ChannelMessage.Text should not be empty")
	}
}

// TestInteg_InboundPipeline_FilterAutoSubmitted 验证 auto-submitted 邮件被过滤
func TestInteg_InboundPipeline_FilterAutoSubmitted(t *testing.T) {
	rawMsg := buildTestRawEmail(101, "noreply@system.com", "bot@company.com",
		"Auto notification", "<auto-001@system.com>", "Automated message")
	rawMsg.Header["Auto-Submitted"] = []string{"auto-generated"}

	filterCfg := FilterConfig{
		IgnoreAutoSubmitted: true,
		SelfAddress:         "bot@company.com",
	}

	passed, filtered := FilterInbound([]RawEmailMessage{rawMsg}, filterCfg)
	if filtered != 1 {
		t.Errorf("Expected 1 filtered, got %d", filtered)
	}
	if len(passed) != 0 {
		t.Errorf("Expected 0 passed, got %d", len(passed))
	}
}

// TestInteg_InboundPipeline_FilterSelfSent 验证自发邮件被过滤（防循环）
func TestInteg_InboundPipeline_FilterSelfSent(t *testing.T) {
	rawMsg := buildTestRawEmail(102, "bot@company.com", "user@example.com",
		"Reply", "<reply-001@company.com>", "My own reply")

	filterCfg := FilterConfig{
		IgnoreSelfSent: true,
		SelfAddress:    "bot@company.com",
	}

	passed, filtered := FilterInbound([]RawEmailMessage{rawMsg}, filterCfg)
	if filtered != 1 {
		t.Errorf("Expected 1 filtered, got %d", filtered)
	}
	if len(passed) != 0 {
		t.Errorf("Expected 0 passed, got %d", len(passed))
	}
}

// TestInteg_DedupCache_PersistAcrossRestart 验证去重缓存跨重启持久化
func TestInteg_DedupCache_PersistAcrossRestart(t *testing.T) {
	tmp := t.TempDir()
	dedup := NewDedupCache(tmp, "dedup-test", 7*24*time.Hour)

	msgID := "<dedup-001@example.com>"

	if dedup.HasSeen(msgID) {
		t.Fatal("First check should return false")
	}

	dedup.MarkSeen(msgID)

	if !dedup.HasSeen(msgID) {
		t.Fatal("Second check should return true")
	}

	// 新建 cache 实例（模拟重启）— 应从磁盘恢复
	dedup2 := NewDedupCache(tmp, "dedup-test", 7*24*time.Hour)
	if !dedup2.HasSeen(msgID) {
		t.Fatal("After reload, should still be seen")
	}
}

// TestInteg_SendMessage_NoRunner 验证 runner 未启动时 SendMessage 报错
func TestInteg_SendMessage_NoRunner(t *testing.T) {
	boolTrue := true
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Email: &types.EmailConfig{
				Enabled: &boolTrue,
				Accounts: map[string]*types.EmailAccountConfig{
					"test": {
						Enabled:  &boolTrue,
						Provider: types.EmailProviderQQ,
						Address:  "bot@qq.com",
						Auth:     types.EmailAuthConfig{Mode: types.EmailAuthAppPassword, Password: "test-pass"},
					},
				},
			},
		},
	}

	plugin := NewEmailPlugin(cfg)
	plugin.StoreRoot = t.TempDir()

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		AccountID: "test",
		To:        "user@example.com",
		Subject:   "Test",
		Text:      "Hello",
	})
	if err == nil {
		t.Fatal("SendMessage with no running account should return error")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("Error should mention 'not running', got: %v", err)
	}
}

// TestInteg_RateLimiting 验证出站频率限制
func TestInteg_RateLimiting(t *testing.T) {
	rl := NewSendRateLimiter(3, 10)

	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("Send %d should be allowed", i+1)
		}
		rl.Record()
	}

	if rl.Allow() {
		t.Error("Send 4 should be rate limited (hourly limit = 3)")
	}

	hourly, daily := rl.Stats()
	if hourly != 3 {
		t.Errorf("hourly = %d, want 3", hourly)
	}
	if daily != 3 {
		t.Errorf("daily = %d, want 3", daily)
	}
}

// TestInteg_Threading_ReplyPreservation 验证发送回复时线程头恢复
func TestInteg_Threading_ReplyPreservation(t *testing.T) {
	tmp := t.TempDir()
	tcs := NewThreadContextStore(tmp, "thread-test")

	sessionKey := "email:thread-test:thread:abc123"

	// 模拟收件后保存线程上下文
	ctx := &ThreadContext{
		LastMessageID: "<original@example.com>",
		References:    []string{"<original@example.com>"},
		Subject:       "Hello",
	}
	if err := tcs.Save(sessionKey, ctx); err != nil {
		t.Fatalf("Save thread context: %v", err)
	}

	// 模拟发件时恢复线程上下文
	loaded, err := tcs.Load(sessionKey)
	if err != nil {
		t.Fatalf("Load thread context: %v", err)
	}
	if loaded == nil {
		t.Fatal("Loaded thread context should not be nil")
	}

	// 构造发送参数并恢复线程头
	sendParams := SendParams{
		To:   []string{"sender@example.com"},
		Body: "Re: Hello",
	}
	sendParams.InReplyTo = loaded.LastMessageID
	sendParams.References = loaded.References
	if sendParams.Subject == "" && loaded.Subject != "" {
		sendParams.Subject = "Re: " + loaded.Subject
	}

	if sendParams.InReplyTo != "<original@example.com>" {
		t.Errorf("InReplyTo = %q, want %q", sendParams.InReplyTo, "<original@example.com>")
	}
	if sendParams.Subject != "Re: Hello" {
		t.Errorf("Subject = %q, want %q", sendParams.Subject, "Re: Hello")
	}
	if len(sendParams.References) != 1 || sendParams.References[0] != "<original@example.com>" {
		t.Errorf("References = %v, want [<original@example.com>]", sendParams.References)
	}
}

// TestInteg_AccountRunner_MockIMAPLoop 验证 AccountRunner mock IMAP 完整循环
func TestInteg_AccountRunner_MockIMAPLoop(t *testing.T) {
	mock := newMockConnector()
	mock.setFetchMessages([]RawEmailMessage{
		buildTestRawEmail(50, "user@example.com", "bot@qq.com",
			"Test", "<test-50@example.com>", "Integration test body"),
	})

	boolTrue := true
	acctCfg := &types.EmailAccountConfig{
		Enabled:  &boolTrue,
		Provider: types.EmailProviderQQ,
		Address:  "bot@qq.com",
		Auth:     types.EmailAuthConfig{Password: "pass"},
		IMAP: &types.EmailIMAPConfig{
			Host:                "imap.qq.com",
			Port:                993,
			Mode:                types.EmailIMAPModePoll,
			PollIntervalSeconds: 1,
		},
	}

	ApplyProviderDefaults(acctCfg)
	runner := NewAccountRunner("test-acct", acctCfg)
	runner.SetIMAPConnector(mock)

	received := make(chan []RawEmailMessage, 1)
	runner.SetOnNewMail(func(_ string, msgs []RawEmailMessage) {
		received <- msgs
	})

	if err := runner.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case msgs := <-received:
		if len(msgs) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(msgs))
		}
		if msgs[0].UID != 50 {
			t.Errorf("UID = %d, want 50", msgs[0].UID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for new mail callback")
	}

	runner.Stop()
}
