package email

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// --- 辅助函数 ---

// buildRawEmail 构造简单纯文本邮件
func buildRawEmail(subject, from, to, body string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-Id: <test@example.com>\r\nDate: Mon, 08 Mar 2026 10:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, subject, body,
	))
}

// buildMultipartEmail 构造 multipart/alternative 邮件（text + html）
func buildMultipartEmail(subject, from, to, textBody, htmlBody string) []byte {
	boundary := "----=_Part_12345"
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-Id: <mp@example.com>\r\nDate: Mon, 08 Mar 2026 10:00:00 +0800\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--%s--\r\n",
		from, to, subject, boundary,
		boundary, textBody,
		boundary, htmlBody,
		boundary,
	))
}

func TestParseEmail_PlainText(t *testing.T) {
	raw := buildRawEmail("Test Subject", "sender@test.com", "recv@test.com", "Hello World")
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if parsed.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want %q", parsed.Subject, "Test Subject")
	}
	if parsed.From != "sender@test.com" {
		t.Errorf("From = %q, want %q", parsed.From, "sender@test.com")
	}
	if parsed.MessageID != "<test@example.com>" {
		t.Errorf("MessageID = %q, want %q", parsed.MessageID, "<test@example.com>")
	}
	if !strings.Contains(parsed.TextBody, "Hello World") {
		t.Errorf("TextBody = %q, want contains 'Hello World'", parsed.TextBody)
	}
	if parsed.HasHTML {
		t.Error("HasHTML should be false for plain text")
	}
}

func TestParseEmail_MultipartAlternative(t *testing.T) {
	raw := buildMultipartEmail(
		"Multi Test", "a@b.com", "c@d.com",
		"Plain text version",
		"<html><body><p>HTML version</p></body></html>",
	)
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "Plain text version") {
		t.Errorf("TextBody = %q, want 'Plain text version'", parsed.TextBody)
	}
	if !parsed.HasHTML {
		t.Error("HasHTML should be true")
	}
	if !strings.Contains(parsed.HTMLBody, "HTML version") {
		t.Errorf("HTMLBody should contain 'HTML version'")
	}
}

func TestParseEmail_HTMLOnly_SafeText(t *testing.T) {
	htmlContent := `<html><body>
		<script>alert('xss')</script>
		<style>.hidden{display:none}</style>
		<p>Hello <b>World</b></p>
		<a href="https://example.com">Click here</a>
		<img width="1" height="1" src="https://track.example.com/pixel.gif">
	</body></html>`

	raw := []byte(fmt.Sprintf(
		"From: a@b.com\r\nTo: c@d.com\r\nSubject: HTML Only\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		htmlContent,
	))
	limits := DefaultParseLimits()
	limits.HTMLMode = types.EmailHTMLSafeText

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "Hello") {
		t.Errorf("TextBody should contain 'Hello', got %q", parsed.TextBody)
	}
	if !strings.Contains(parsed.TextBody, "World") {
		t.Errorf("TextBody should contain 'World', got %q", parsed.TextBody)
	}
	// 链接应保留 URL
	if !strings.Contains(parsed.TextBody, "Click here") {
		t.Errorf("TextBody should contain 'Click here', got %q", parsed.TextBody)
	}
	if !strings.Contains(parsed.TextBody, "https://example.com") {
		t.Errorf("TextBody should contain URL, got %q", parsed.TextBody)
	}
	// script 应被移除
	if strings.Contains(parsed.TextBody, "alert") {
		t.Errorf("TextBody should not contain script, got %q", parsed.TextBody)
	}
	// tracking pixel 应被移除
	if strings.Contains(parsed.TextBody, "pixel") {
		t.Errorf("TextBody should not contain tracking pixel")
	}
}

func TestParseEmail_HTMLStrip(t *testing.T) {
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Strip\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Hello <b>Bold</b></p>")
	limits := DefaultParseLimits()
	limits.HTMLMode = types.EmailHTMLStrip

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "Hello") || !strings.Contains(parsed.TextBody, "Bold") {
		t.Errorf("TextBody = %q, want 'Hello' and 'Bold'", parsed.TextBody)
	}
}

func TestParseEmail_RFC2047_ChineseSubject(t *testing.T) {
	// =?UTF-8?B?5rWL6K+V5Li76aKY?= = "测试主题"
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: =?UTF-8?B?5rWL6K+V5Li76aKY?=\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nBody")
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if parsed.Subject != "测试主题" {
		t.Errorf("Subject = %q, want '测试主题'", parsed.Subject)
	}
}

func TestParseEmail_GBKBody(t *testing.T) {
	// GBK 编码的 "你好世界"
	gbkHello := []byte{0xc4, 0xe3, 0xba, 0xc3, 0xca, 0xc0, 0xbd, 0xe7}

	raw := append(
		[]byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: GBK\r\nContent-Type: text/plain; charset=GBK\r\n\r\n"),
		gbkHello...,
	)
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "你好世界") {
		t.Errorf("TextBody = %q, want '你好世界'", parsed.TextBody)
	}
}

func TestParseEmail_QuotedPrintable(t *testing.T) {
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: QP\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nHello=20World=0D=0ALine2")
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "Hello World") {
		t.Errorf("TextBody = %q, want 'Hello World'", parsed.TextBody)
	}
	if !strings.Contains(parsed.TextBody, "Line2") {
		t.Errorf("TextBody should contain 'Line2'")
	}
}

func TestParseEmail_Base64Body(t *testing.T) {
	// "Hello World" in base64 = "SGVsbG8gV29ybGQ="
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: B64\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\nSGVsbG8gV29ybGQ=")
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "Hello World") {
		t.Errorf("TextBody = %q, want 'Hello World'", parsed.TextBody)
	}
}

func TestParseEmail_AttachmentWhitelist(t *testing.T) {
	boundary := "----=_Att_01"
	raw := []byte(fmt.Sprintf(
		"From: a@b.com\r\nTo: c@d.com\r\nSubject: Att\r\nContent-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain\r\n\r\nBody text\r\n--%s\r\nContent-Type: image/png\r\nContent-Disposition: attachment; filename=\"photo.png\"\r\n\r\nPNG_DATA\r\n--%s\r\nContent-Type: application/x-executable\r\nContent-Disposition: attachment; filename=\"evil.exe\"\r\n\r\nEXE_DATA\r\n--%s--\r\n",
		boundary, boundary, boundary, boundary, boundary,
	))
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	// image/png 应在白名单内
	if len(parsed.Attachments) != 1 {
		t.Errorf("Attachments count = %d, want 1", len(parsed.Attachments))
	} else if parsed.Attachments[0].Filename != "photo.png" {
		t.Errorf("Attachment filename = %q, want photo.png", parsed.Attachments[0].Filename)
	}

	// application/x-executable 不在白名单
	found := false
	for _, w := range parsed.ParseWarnings {
		if strings.Contains(w, "not in whitelist") {
			found = true
		}
	}
	if !found {
		t.Error("Expected whitelist warning for .exe")
	}
}

func TestParseEmail_AttachmentCountLimit(t *testing.T) {
	boundary := "----=_Limit_01"
	parts := ""
	for i := 0; i < 7; i++ {
		parts += fmt.Sprintf("--%s\r\nContent-Type: image/png\r\nContent-Disposition: attachment; filename=\"img%d.png\"\r\n\r\nDATA%d\r\n", boundary, i, i)
	}
	raw := []byte(fmt.Sprintf(
		"From: a@b.com\r\nTo: c@d.com\r\nSubject: Limit\r\nContent-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain\r\n\r\nBody\r\n%s--%s--\r\n",
		boundary, boundary, parts, boundary,
	))
	limits := DefaultParseLimits()
	limits.MaxAttachments = 3

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	totalAtt := len(parsed.Attachments) + len(parsed.InlineImages)
	if totalAtt > 3 {
		t.Errorf("Total attachments = %d, want <= 3", totalAtt)
	}
}

func TestParseEmail_AttachmentSizeLimit(t *testing.T) {
	boundary := "----=_Size_01"
	bigData := strings.Repeat("X", 100)
	raw := []byte(fmt.Sprintf(
		"From: a@b.com\r\nTo: c@d.com\r\nSubject: Size\r\nContent-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain\r\n\r\nBody\r\n--%s\r\nContent-Type: image/png\r\nContent-Disposition: attachment; filename=\"big.png\"\r\n\r\n%s\r\n--%s--\r\n",
		boundary, boundary, boundary, bigData, boundary,
	))
	limits := DefaultParseLimits()
	limits.MaxAttachmentBytes = 50 // very small

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if len(parsed.Attachments) != 0 {
		t.Errorf("Attachments = %d, want 0 (too large)", len(parsed.Attachments))
	}
}

func TestParseEmail_RFC822Summary(t *testing.T) {
	nestedEmail := "From: nested@test.com\r\nTo: recv@test.com\r\nSubject: Forwarded Topic\r\nContent-Type: text/plain\r\n\r\nNested body"

	boundary := "----=_RFC822_01"
	raw := []byte(fmt.Sprintf(
		"From: a@b.com\r\nTo: c@d.com\r\nSubject: FWD\r\nContent-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain\r\n\r\nPlease see attached.\r\n--%s\r\nContent-Type: message/rfc822\r\n\r\n%s\r\n--%s--\r\n",
		boundary, boundary, boundary, nestedEmail, boundary,
	))
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if !strings.Contains(parsed.TextBody, "[转发邮件: Forwarded Topic]") {
		t.Errorf("TextBody should contain forwarded summary, got %q", parsed.TextBody)
	}
}

func TestParseEmail_InlineImage_CID(t *testing.T) {
	boundary := "----=_CID_01"
	raw := []byte(fmt.Sprintf(
		"From: a@b.com\r\nTo: c@d.com\r\nSubject: CID\r\nContent-Type: multipart/related; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/html\r\n\r\n<img src=\"cid:logo123\">\r\n--%s\r\nContent-Type: image/png\r\nContent-Id: <logo123>\r\nContent-Disposition: inline\r\n\r\nPNG_INLINE_DATA\r\n--%s--\r\n",
		boundary, boundary, boundary, boundary,
	))
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if len(parsed.InlineImages) != 1 {
		t.Errorf("InlineImages = %d, want 1", len(parsed.InlineImages))
	} else {
		img := parsed.InlineImages[0]
		if img.ContentID != "logo123" {
			t.Errorf("ContentID = %q, want 'logo123'", img.ContentID)
		}
		if !img.Inline {
			t.Error("Inline should be true")
		}
	}
}

func TestParseEmail_EmptyBody(t *testing.T) {
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Empty\r\nContent-Type: text/plain\r\n\r\n")
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if parsed.TextBody != "" {
		t.Errorf("TextBody = %q, want empty", parsed.TextBody)
	}
}

func TestParseEmail_References(t *testing.T) {
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Refs\r\nIn-Reply-To: <parent@test.com>\r\nReferences: <root@test.com> <mid@test.com> <parent@test.com>\r\nContent-Type: text/plain\r\n\r\nReply body")
	limits := DefaultParseLimits()

	parsed, err := ParseEmail(raw, limits)
	if err != nil {
		t.Fatalf("ParseEmail: %v", err)
	}

	if parsed.InReplyTo != "<parent@test.com>" {
		t.Errorf("InReplyTo = %q", parsed.InReplyTo)
	}
	if len(parsed.References) != 3 {
		t.Errorf("References count = %d, want 3", len(parsed.References))
	}
}

func TestParseEmail_PanicRecovery(t *testing.T) {
	// 畸形邮件不应导致 panic
	malformed := []byte("From: a@b.com\r\nContent-Type: multipart/mixed; boundary=\"bad\r\n\r\ngarbage")
	limits := DefaultParseLimits()

	_, err := ParseEmail(malformed, limits)
	// 应该返回错误或正常解析，不应 panic
	_ = err
}

func TestDecodeCharset_GBK(t *testing.T) {
	// "你好" in GBK
	gbk := []byte{0xc4, 0xe3, 0xba, 0xc3}
	result := decodeCharset(gbk, "GBK")
	if result != "你好" {
		t.Errorf("decodeCharset GBK = %q, want '你好'", result)
	}
}

func TestDecodeCharset_GB18030(t *testing.T) {
	// "你好" in GB18030 (same as GBK for these chars)
	gb := []byte{0xc4, 0xe3, 0xba, 0xc3}
	result := decodeCharset(gb, "GB18030")
	if result != "你好" {
		t.Errorf("decodeCharset GB18030 = %q, want '你好'", result)
	}
}

func TestDecodeCharset_AutoFallback(t *testing.T) {
	// 无效 UTF-8 + charset="" → 自动尝试 GBK
	gbk := []byte{0xc4, 0xe3, 0xba, 0xc3}
	result := decodeCharset(gbk, "")
	if result != "你好" {
		t.Errorf("decodeCharset auto = %q, want '你好' (GBK fallback)", result)
	}
}

func TestHTMLToSafeText_TrackingPixel(t *testing.T) {
	h := `<p>Hello</p><img width="1" height="1" src="https://track.com/p.gif"><p>World</p>`
	text := htmlToSafeText(h, types.EmailHTMLSafeText)
	if strings.Contains(text, "track.com") {
		t.Errorf("should not contain tracking pixel URL: %q", text)
	}
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "World") {
		t.Errorf("should contain text: %q", text)
	}
}

func TestHTMLToSafeText_Links(t *testing.T) {
	h := `<a href="https://example.com">Visit us</a>`
	text := htmlToSafeText(h, types.EmailHTMLSafeText)
	if !strings.Contains(text, "Visit us") {
		t.Errorf("should contain link text: %q", text)
	}
	if !strings.Contains(text, "https://example.com") {
		t.Errorf("should contain URL: %q", text)
	}
}

func TestBackoffDuration_Values(t *testing.T) {
	// This test already exists in imap_client_test.go, just sanity check here
	d := backoffDuration(0)
	if d.Seconds() != 5 {
		t.Errorf("backoff(0) = %v, want 5s", d)
	}
}

func TestIsMimeAllowed(t *testing.T) {
	prefixes := []string{"image/", "text/", "application/pdf"}

	tests := []struct {
		mime string
		want bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"text/plain", true},
		{"application/pdf", true},
		{"application/x-executable", false},
		{"application/zip", false},
	}
	for _, tt := range tests {
		got := isMimeAllowed(tt.mime, prefixes)
		if got != tt.want {
			t.Errorf("isMimeAllowed(%q) = %v, want %v", tt.mime, got, tt.want)
		}
	}
}

func TestDecodeBase64(t *testing.T) {
	// "Hello World" = SGVsbG8gV29ybGQ=
	data := []byte("SGVsbG8g\r\nV29ybGQ=")
	result := decodeBase64(data)
	if string(result) != "Hello World" {
		t.Errorf("decodeBase64 = %q, want 'Hello World'", result)
	}
}

func TestDecodeQuotedPrintable(t *testing.T) {
	data := []byte("Hello=20World=0D=0ASoft=\r\nLine")
	result := decodeQuotedPrintable(data)
	if !strings.Contains(string(result), "Hello World") {
		t.Errorf("QP decode = %q, want 'Hello World'", result)
	}
	if !strings.Contains(string(result), "SoftLine") {
		t.Errorf("QP soft line break = %q, want 'SoftLine'", result)
	}
}
