package email

import (
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func newTestSMTPConfig() *types.EmailAccountConfig {
	boolTrue := true
	return &types.EmailAccountConfig{
		Enabled:  &boolTrue,
		Provider: types.EmailProviderAliyun,
		Address:  "robot@company.com",
		Login:    "robot@company.com",
		Auth: types.EmailAuthConfig{
			Mode:     types.EmailAuthAppPassword,
			Password: "test-pass",
		},
		SMTP: &types.EmailSMTPConfig{
			Host:     "smtp.qiye.aliyun.com",
			Port:     465,
			Security: types.EmailSecurityTLS,
			FromName: "OpenAcosmi Bot",
		},
	}
}

func TestBuildMailMessage_Basic(t *testing.T) {
	cfg := newTestSMTPConfig()
	params := SendParams{
		To:      []string{"user@example.com"},
		Subject: "Test Subject",
		Body:    "Hello World",
	}
	msgID := "<test-id@company.com>"

	data := buildMailMessage(cfg, params, msgID)
	s := string(data)

	// From 应包含 FromName
	if !strings.Contains(s, "From: OpenAcosmi Bot <robot@company.com>") {
		t.Errorf("missing From header with name: %s", s)
	}
	if !strings.Contains(s, "To: user@example.com") {
		t.Error("missing To header")
	}
	if !strings.Contains(s, "Subject: Test Subject") {
		t.Error("missing Subject header")
	}
	if !strings.Contains(s, "Message-ID: <test-id@company.com>") {
		t.Error("missing Message-ID header")
	}
	// 防环头
	if !strings.Contains(s, "Auto-Submitted: auto-generated") {
		t.Error("missing Auto-Submitted header")
	}
	if !strings.Contains(s, "X-OpenAcosmi-Channel: email") {
		t.Error("missing X-OpenAcosmi-Channel header")
	}
	if !strings.Contains(s, "X-OpenAcosmi-Account: robot@company.com") {
		t.Error("missing X-OpenAcosmi-Account header")
	}
	// Content-Type
	if !strings.Contains(s, "Content-Type: text/plain; charset=utf-8") {
		t.Error("missing Content-Type")
	}
	if !strings.Contains(s, "Content-Transfer-Encoding: quoted-printable") {
		t.Error("missing CTE")
	}
}

func TestBuildMailMessage_Reply(t *testing.T) {
	cfg := newTestSMTPConfig()
	params := SendParams{
		To:         []string{"user@example.com"},
		Subject:    "Re: Original Topic",
		Body:       "Thanks!",
		InReplyTo:  "<parent@example.com>",
		References: []string{"<root@example.com>", "<parent@example.com>"},
	}

	data := buildMailMessage(cfg, params, "<reply@company.com>")
	s := string(data)

	if !strings.Contains(s, "In-Reply-To: <parent@example.com>") {
		t.Error("missing In-Reply-To")
	}
	if !strings.Contains(s, "References: <root@example.com> <parent@example.com>") {
		t.Error("missing References")
	}
}

func TestBuildMailMessage_Cc(t *testing.T) {
	cfg := newTestSMTPConfig()
	params := SendParams{
		To:      []string{"user@example.com"},
		Cc:      []string{"cc1@example.com", "cc2@example.com"},
		Subject: "CC Test",
		Body:    "Body",
	}

	data := buildMailMessage(cfg, params, "<cc@company.com>")
	s := string(data)

	if !strings.Contains(s, "Cc: cc1@example.com, cc2@example.com") {
		t.Error("missing Cc header")
	}
}

func TestBuildMailMessage_NoFromName(t *testing.T) {
	cfg := newTestSMTPConfig()
	cfg.SMTP.FromName = ""
	params := SendParams{
		To:      []string{"user@example.com"},
		Subject: "No Name",
		Body:    "Body",
	}

	data := buildMailMessage(cfg, params, "<noname@company.com>")
	s := string(data)

	if !strings.Contains(s, "From: robot@company.com\r\n") {
		t.Errorf("From should not have name: %s", s)
	}
}

func TestGenerateMessageID(t *testing.T) {
	id1 := generateMessageID("user@company.com")
	id2 := generateMessageID("user@company.com")

	if !strings.HasPrefix(id1, "<") || !strings.HasSuffix(id1, ">") {
		t.Errorf("MessageID missing angle brackets: %q", id1)
	}
	if !strings.Contains(id1, "@company.com>") {
		t.Errorf("MessageID missing domain: %q", id1)
	}
	if id1 == id2 {
		t.Error("Two generated MessageIDs should be different")
	}
}

func TestEncodeQuotedPrintableBody(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"Hello World", "Hello World"},
		{"Hello=World", "Hello=3DWorld"},
		{"Line1\r\nLine2", "Line1\r\nLine2"},
	}
	for _, tt := range tests {
		result := encodeQuotedPrintableBody(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("QP encode(%q) = %q, want contains %q", tt.input, result, tt.contains)
		}
	}
}

func TestEncodeQuotedPrintableBody_LongLine(t *testing.T) {
	long := strings.Repeat("A", 200)
	result := encodeQuotedPrintableBody(long)
	// 应包含软换行
	if !strings.Contains(result, "=\r\n") {
		t.Error("Long line should have soft line breaks")
	}
	// 每行不超过 76 字符
	for _, line := range strings.Split(result, "\r\n") {
		if len(line) > 76 {
			t.Errorf("Line too long (%d): %q", len(line), line)
		}
	}
}

func TestExtractEmailAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@test.com", "user@test.com"},
		{"User <user@test.com>", "user@test.com"},
		{"\"Display Name\" <user@test.com>", "user@test.com"},
	}
	for _, tt := range tests {
		got := extractEmailAddr(tt.input)
		if got != tt.want {
			t.Errorf("extractEmailAddr(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestThreadContextStore_SaveLoad(t *testing.T) {
	tmp := t.TempDir()
	store := NewThreadContextStore(tmp, "test")

	tc := &ThreadContext{
		LastMessageID: "<msg1@test.com>",
		References:    []string{"<root@test.com>", "<msg1@test.com>"},
		Subject:       "Test Thread",
	}

	if err := store.Save("email:test:thread:abc", tc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("email:test:thread:abc")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded should not be nil")
	}
	if loaded.LastMessageID != tc.LastMessageID {
		t.Errorf("LastMessageID = %q", loaded.LastMessageID)
	}
	if len(loaded.References) != 2 {
		t.Errorf("References len = %d", len(loaded.References))
	}
	if loaded.Subject != tc.Subject {
		t.Errorf("Subject = %q", loaded.Subject)
	}
}

func TestThreadContextStore_LoadNotFound(t *testing.T) {
	tmp := t.TempDir()
	store := NewThreadContextStore(tmp, "test")

	tc, err := store.Load("nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tc != nil {
		t.Error("should return nil for nonexistent")
	}
}

func TestSMTPSender_NilConfig(t *testing.T) {
	cfg := newTestSMTPConfig()
	cfg.SMTP = nil
	sender := NewSMTPSender(cfg)

	_, err := sender.Send(SendParams{To: []string{"a@b.com"}, Subject: "X", Body: "Y"})
	if err == nil {
		t.Error("should fail with nil SMTP config")
	}
}

func TestSMTPSender_NoRecipients(t *testing.T) {
	cfg := newTestSMTPConfig()
	sender := NewSMTPSender(cfg)

	_, err := sender.Send(SendParams{Subject: "X", Body: "Y"})
	if err == nil {
		t.Error("should fail with no recipients")
	}
}

// TestSMTPSender_BuildAndVerify 验证完整邮件构造（不实际连接 SMTP）
func TestSMTPSender_BuildAndVerify(t *testing.T) {
	cfg := newTestSMTPConfig()
	params := SendParams{
		To:         []string{"recipient@example.com"},
		Cc:         []string{"cc@example.com"},
		Subject:    "Re: Important Discussion",
		Body:       "Thank you for your message.\n\nBest regards,\nOpenAcosmi",
		InReplyTo:  "<original@example.com>",
		References: []string{"<thread-root@example.com>", "<original@example.com>"},
	}

	msgID := generateMessageID(cfg.Address)
	data := buildMailMessage(cfg, params, msgID)
	s := string(data)

	// 验证完整性
	requiredHeaders := []string{
		"From:", "To:", "Cc:", "Subject:", "Date:", "Message-ID:",
		"In-Reply-To:", "References:", "Auto-Submitted:", "X-OpenAcosmi-Channel:",
		"MIME-Version: 1.0", "Content-Type: text/plain; charset=utf-8",
	}
	for _, h := range requiredHeaders {
		if !strings.Contains(s, h) {
			t.Errorf("missing header: %s", h)
		}
	}

	// body 应在空行后
	parts := strings.SplitN(s, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("missing header/body separator")
	}
	if parts[1] == "" {
		t.Error("body should not be empty")
	}
}

// 占位：SMTP 实际发送需要真实 SMTP 服务器，V10 联调测试覆盖
func TestSMTPSender_Placeholder_V10(t *testing.T) {
	_ = time.Now() // avoid unused import
	t.Log("SMTP actual send tests will be covered in Phase 10 integration testing")
}
