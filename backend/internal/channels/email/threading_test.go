package email

import (
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

// --- Session Key 测试 ---

func TestResolveSessionKey_WithReferences(t *testing.T) {
	msg := &InboundEmailMessage{
		MessageID:  "<msg3@test.com>",
		InReplyTo:  "<msg2@test.com>",
		References: []string{"<root@test.com>", "<msg2@test.com>"},
	}
	key := ResolveSessionKey("ali-work", msg)
	if !strings.HasPrefix(key, "email:ali-work:thread:") {
		t.Errorf("key = %q, want prefix 'email:ali-work:thread:'", key)
	}
	// 使用 References[0] 作为 root
	expected := "email:ali-work:thread:" + shortHash("<root@test.com>")
	if key != expected {
		t.Errorf("key = %q, want %q", key, expected)
	}
}

func TestResolveSessionKey_WithInReplyToOnly(t *testing.T) {
	msg := &InboundEmailMessage{
		MessageID: "<msg2@test.com>",
		InReplyTo: "<msg1@test.com>",
	}
	key := ResolveSessionKey("ali-work", msg)
	expected := "email:ali-work:thread:" + shortHash("<msg1@test.com>")
	if key != expected {
		t.Errorf("key = %q, want %q", key, expected)
	}
}

func TestResolveSessionKey_FallbackSubjectPeer(t *testing.T) {
	msg := &InboundEmailMessage{
		Subject: "Re: 项目进度更新",
		From:    "Zhang San <zhang@company.com>",
	}
	key := ResolveSessionKey("ali-work", msg)
	if !strings.HasPrefix(key, "email:ali-work:subject:") {
		t.Errorf("key = %q, want prefix 'email:ali-work:subject:'", key)
	}
	if !strings.Contains(key, ":peer:") {
		t.Errorf("key = %q, want ':peer:' segment", key)
	}
}

func TestResolveSessionKey_SameThread(t *testing.T) {
	// 同线程的两封邮件应生成相同的 session key
	msg1 := &InboundEmailMessage{
		MessageID:  "<msg1@test.com>",
		References: []string{"<root@test.com>"},
	}
	msg2 := &InboundEmailMessage{
		MessageID:  "<msg2@test.com>",
		InReplyTo:  "<msg1@test.com>",
		References: []string{"<root@test.com>", "<msg1@test.com>"},
	}
	key1 := ResolveSessionKey("ali", msg1)
	key2 := ResolveSessionKey("ali", msg2)
	if key1 != key2 {
		t.Errorf("Same thread should have same key: %q vs %q", key1, key2)
	}
}

func TestNormalizeSubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Re: Hello", "Hello"},
		{"RE: Hello", "Hello"},
		{"Fwd: Hello", "Hello"},
		{"FW: Hello", "Hello"},
		{"回复: Hello", "Hello"},
		{"转发: Hello", "Hello"},
		{"Re: Re: Fwd: Hello", "Hello"},
		{"Hello", "Hello"},
		{"Re：中文冒号", "中文冒号"},
	}
	for _, tt := range tests {
		got := normalizeSubject(tt.input)
		if got != tt.want {
			t.Errorf("normalizeSubject(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractSenderAddress(t *testing.T) {
	tests := []struct {
		from string
		want string
	}{
		{"user@test.com", "user@test.com"},
		{"User Name <user@test.com>", "user@test.com"},
		{"\"Zhang San\" <Zhang@Company.COM>", "zhang@company.com"},
	}
	for _, tt := range tests {
		got := extractSenderAddress(tt.from)
		if got != tt.want {
			t.Errorf("extractSenderAddress(%q) = %q, want %q", tt.from, got, tt.want)
		}
	}
}

// --- 去重测试 ---

func TestGenerateDedupKey_MessageID(t *testing.T) {
	msg := &InboundEmailMessage{MessageID: "<test@example.com>"}
	key := GenerateDedupKey(msg)
	if key != "msgid:<test@example.com>" {
		t.Errorf("key = %q", key)
	}
}

func TestGenerateDedupKey_UID(t *testing.T) {
	msg := &InboundEmailMessage{UIDValidity: 1000, UID: 42}
	key := GenerateDedupKey(msg)
	if key != "uid:1000:42" {
		t.Errorf("key = %q", key)
	}
}

func TestGenerateDedupKey_Hash(t *testing.T) {
	msg := &InboundEmailMessage{
		ReceivedAt: time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC),
		From:       "user@test.com",
		Subject:    "Hello",
		RawSize:    1234,
	}
	key := GenerateDedupKey(msg)
	if !strings.HasPrefix(key, "hash:") {
		t.Errorf("key = %q, want prefix 'hash:'", key)
	}
}

func TestDedupCache_Basic(t *testing.T) {
	tmp := t.TempDir()
	dc := NewDedupCache(tmp, "test", 7*24*time.Hour)

	if dc.HasSeen("key1") {
		t.Error("new cache should not have any keys")
	}

	dc.MarkSeen("key1")
	if !dc.HasSeen("key1") {
		t.Error("key1 should be seen after MarkSeen")
	}
	if dc.HasSeen("key2") {
		t.Error("key2 should not be seen")
	}

	if dc.Len() != 1 {
		t.Errorf("Len = %d, want 1", dc.Len())
	}
}

func TestDedupCache_TTL(t *testing.T) {
	tmp := t.TempDir()
	dc := NewDedupCache(tmp, "test", 1*time.Millisecond)

	dc.MarkSeen("key1")
	time.Sleep(5 * time.Millisecond)

	if dc.HasSeen("key1") {
		t.Error("key1 should be expired")
	}
}

func TestDedupCache_Persistence(t *testing.T) {
	tmp := t.TempDir()

	// 写入
	dc1 := NewDedupCache(tmp, "test", 7*24*time.Hour)
	dc1.MarkSeen("key1")
	dc1.MarkSeen("key2")

	// 重新加载
	dc2 := NewDedupCache(tmp, "test", 7*24*time.Hour)
	if !dc2.HasSeen("key1") {
		t.Error("key1 should persist across reload")
	}
	if !dc2.HasSeen("key2") {
		t.Error("key2 should persist across reload")
	}
}

func TestDedupCache_Cleanup(t *testing.T) {
	tmp := t.TempDir()
	dc := NewDedupCache(tmp, "test", 1*time.Millisecond)
	dc.MarkSeen("old1")
	dc.MarkSeen("old2")

	time.Sleep(5 * time.Millisecond)
	dc.MarkSeen("new1")

	removed := dc.Cleanup()
	if removed != 2 {
		t.Errorf("Cleanup removed = %d, want 2", removed)
	}
	if dc.Len() != 1 {
		t.Errorf("Len = %d, want 1", dc.Len())
	}
}

// --- ChannelMessage 映射测试 ---

func TestToChannelMessage(t *testing.T) {
	msg := &InboundEmailMessage{
		TextBody:  "Hello World",
		MessageID: "<test@example.com>",
		Attachments: []EmailAttachment{
			{Filename: "doc.pdf", ContentType: "application/pdf", Size: 1024, Data: []byte("pdf-data")},
		},
		InlineImages: []EmailAttachment{
			{Filename: "logo.png", ContentType: "image/png", Size: 512, Data: []byte("png-data"), ContentID: "cid1", Inline: true},
		},
	}

	cm := ToChannelMessage(msg)
	if cm.Text != "Hello World" {
		t.Errorf("Text = %q", cm.Text)
	}
	if cm.MessageType != "email" {
		t.Errorf("MessageType = %q", cm.MessageType)
	}
	if len(cm.Attachments) != 2 {
		t.Errorf("Attachments = %d, want 2", len(cm.Attachments))
	}

	// 检查附件分类
	if cm.Attachments[0].Category != "document" {
		t.Errorf("Attachment[0] category = %q, want document", cm.Attachments[0].Category)
	}
	if cm.Attachments[1].Category != "image" {
		t.Errorf("Attachment[1] category = %q, want image", cm.Attachments[1].Category)
	}
}

func TestCategorizeAttachment(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"image/png", "image"},
		{"image/jpeg", "image"},
		{"audio/mp3", "audio"},
		{"video/mp4", "video"},
		{"application/pdf", "document"},
		{"text/plain", "document"},
	}
	for _, tt := range tests {
		got := categorizeAttachment(tt.mime)
		if got != tt.want {
			t.Errorf("categorizeAttachment(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}

// --- BuildMetadata 测试 ---

func TestBuildMetadata(t *testing.T) {
	msg := &InboundEmailMessage{
		MessageID:  "<test@example.com>",
		InReplyTo:  "<parent@example.com>",
		References: []string{"<root@example.com>"},
		Subject:    "Test",
		From:       "user@test.com",
		To:         []string{"recv@test.com"},
		ReceivedAt: time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC),
		HasHTML:    true,
		Attachments: []EmailAttachment{
			{Filename: "doc.pdf"},
		},
	}
	meta := BuildMetadata(msg, "email:ali:thread:abc123")
	if meta.MessageID != "<test@example.com>" {
		t.Errorf("MessageID = %q", meta.MessageID)
	}
	if meta.SessionKey != "email:ali:thread:abc123" {
		t.Errorf("SessionKey = %q", meta.SessionKey)
	}
	if meta.Attachments != 1 {
		t.Errorf("Attachments = %d", meta.Attachments)
	}
}

// --- ProcessInbound 集成测试 ---

func TestProcessInbound_FullFlow(t *testing.T) {
	raw := []RawEmailMessage{
		{
			UID:  101,
			Size: 500,
			Body: []byte("From: sender@test.com\r\nTo: recv@test.com\r\nSubject: Hello\r\nMessage-Id: <msg101@test.com>\r\nContent-Type: text/plain\r\n\r\nHi there"),
		},
	}

	tmp := t.TempDir()
	dedup := NewDedupCache(tmp, "test", 7*24*time.Hour)
	limits := DefaultParseLimits()

	var dispatched []*channels.ChannelMessage
	var dispatchedKeys []string
	dispatch := func(channel, accountID, chatID, userID string, msg *channels.ChannelMessage) *channels.DispatchReply {
		dispatched = append(dispatched, msg)
		dispatchedKeys = append(dispatchedKeys, chatID)
		return &channels.DispatchReply{Text: "OK"}
	}

	ProcessInbound("ali-work", "aliyun", "INBOX", 1000, raw, limits, dedup, dispatch)

	if len(dispatched) != 1 {
		t.Fatalf("dispatched = %d, want 1", len(dispatched))
	}
	if !strings.Contains(dispatched[0].Text, "Hi there") {
		t.Errorf("Text = %q", dispatched[0].Text)
	}
	if !strings.HasPrefix(dispatchedKeys[0], "email:ali-work:") {
		t.Errorf("sessionKey = %q", dispatchedKeys[0])
	}

	// 去重：再次处理同一消息
	ProcessInbound("ali-work", "aliyun", "INBOX", 1000, raw, limits, dedup, dispatch)
	if len(dispatched) != 1 {
		t.Errorf("after dedup: dispatched = %d, want 1", len(dispatched))
	}
}

func TestProcessInbound_NilDispatch(t *testing.T) {
	raw := []RawEmailMessage{
		{UID: 1, Body: []byte("From: a@b.com\r\nSubject: X\r\nContent-Type: text/plain\r\n\r\nBody")},
	}
	// 不应 panic
	ProcessInbound("test", "aliyun", "INBOX", 1000, raw, DefaultParseLimits(), nil, nil)
}

func TestProcessInbound_MalformedMessage(t *testing.T) {
	raw := []RawEmailMessage{
		{UID: 1, Body: []byte("garbage not an email")},
		{UID: 2, Body: []byte("From: a@b.com\r\nSubject: OK\r\nContent-Type: text/plain\r\n\r\nValid")},
	}

	var dispatched int
	dispatch := func(_, _, _, _ string, _ *channels.ChannelMessage) *channels.DispatchReply {
		dispatched++
		return nil
	}

	ProcessInbound("test", "aliyun", "INBOX", 1000, raw, DefaultParseLimits(), nil, dispatch)
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1 (skip malformed)", dispatched)
	}
}
