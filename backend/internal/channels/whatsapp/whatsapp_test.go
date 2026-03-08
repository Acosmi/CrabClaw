package whatsapp

import (
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ── WA-E: ResolveWhatsAppTableMode ──

func TestResolveWhatsAppTableMode_Default(t *testing.T) {
	mode := ResolveWhatsAppTableMode(nil)
	if mode != types.MarkdownTableBullets {
		t.Errorf("expected 'bullets', got '%s'", mode)
	}
}

func TestResolveWhatsAppTableMode_EmptyConfig(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	mode := ResolveWhatsAppTableMode(cfg)
	if mode != types.MarkdownTableBullets {
		t.Errorf("expected 'bullets', got '%s'", mode)
	}
}

func TestResolveWhatsAppTableMode_Configured(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Markdown: &types.MarkdownConfig{
			Tables: types.MarkdownTableCode,
		},
	}
	mode := ResolveWhatsAppTableMode(cfg)
	if mode != types.MarkdownTableCode {
		t.Errorf("expected 'code', got '%s'", mode)
	}
}

func TestResolveWhatsAppTableMode_Off(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Markdown: &types.MarkdownConfig{
			Tables: types.MarkdownTableOff,
		},
	}
	mode := ResolveWhatsAppTableMode(cfg)
	if mode != types.MarkdownTableOff {
		t.Errorf("expected 'off', got '%s'", mode)
	}
}

// ── WA-C: ChunkReplyText ──

func TestChunkReplyText_ShortText(t *testing.T) {
	account := ResolvedWhatsAppAccount{}
	chunks := ChunkReplyText("hello", account)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Errorf("expected single chunk 'hello', got %v", chunks)
	}
}

func TestChunkReplyText_LongText_LengthMode(t *testing.T) {
	limit := 10
	account := ResolvedWhatsAppAccount{TextChunkLimit: &limit, ChunkMode: "length"}
	text := "0123456789ABCDEF"
	chunks := ChunkReplyText(text, account)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0] != "0123456789" {
		t.Errorf("chunk[0]=%q, want '0123456789'", chunks[0])
	}
	if chunks[1] != "ABCDEF" {
		t.Errorf("chunk[1]=%q, want 'ABCDEF'", chunks[1])
	}
}

func TestChunkReplyText_LongText_NewlineMode(t *testing.T) {
	limit := 20
	account := ResolvedWhatsAppAccount{TextChunkLimit: &limit, ChunkMode: "newline"}
	text := "line1\nline2\nline3\nline4_is_a_long_line"
	chunks := ChunkReplyText(text, account)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestChunkReplyText_DefaultLimit(t *testing.T) {
	account := ResolvedWhatsAppAccount{}
	// default limit is 4000
	text := strings.Repeat("x", 3999)
	chunks := ChunkReplyText(text, account)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for 3999 chars, got %d", len(chunks))
	}

	text = strings.Repeat("x", 4001)
	chunks = ChunkReplyText(text, account)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks for 4001 chars, got %d", len(chunks))
	}
}

// ── WA-B: BuildWhatsAppPairingReply ──

func TestBuildWhatsAppPairingReply(t *testing.T) {
	reply := BuildWhatsAppPairingReply("+41796666864", "ABC123")
	if !strings.Contains(reply, "👋") {
		t.Error("missing greeting emoji")
	}
	if !strings.Contains(reply, "Crab Claw（蟹爪）") {
		t.Error("missing new brand")
	}
	if !strings.Contains(reply, "+41796666864") {
		t.Error("missing sender ID")
	}
	if !strings.Contains(reply, "ABC123") {
		t.Error("missing pairing code")
	}
	if !strings.Contains(reply, "/pair approve") {
		t.Error("missing approve instruction")
	}
}

func TestBuildWhatsAppPairingReply_EmptySender(t *testing.T) {
	reply := BuildWhatsAppPairingReply("", "XYZ")
	if strings.Contains(reply, "Your WhatsApp sender id:") {
		t.Error("should not include sender id line when empty")
	}
	if !strings.Contains(reply, "XYZ") {
		t.Error("missing pairing code")
	}
}

// ── WA-B: formatWhatsAppEnvelope ──

func TestFormatWhatsAppEnvelope_Full(t *testing.T) {
	result := formatWhatsAppEnvelope("WhatsApp", "Alice", 1707955200000, "hello world", "direct")
	if !strings.Contains(result, "[WhatsApp]") {
		t.Error("missing channel")
	}
	if !strings.Contains(result, "Alice") {
		t.Error("missing sender")
	}
	if !strings.Contains(result, "(direct)") {
		t.Error("missing chat type")
	}
	if !strings.HasSuffix(result, ": hello world") {
		t.Errorf("expected body suffix, got: %s", result)
	}
}

func TestFormatWhatsAppEnvelope_EmptyBody(t *testing.T) {
	result := formatWhatsAppEnvelope("WhatsApp", "Bob", 0, "", "group")
	if strings.Contains(result, ": ") {
		t.Error("should not have body separator for empty body")
	}
}

// ── WA-B: isSenderAllowed ──

func TestIsSenderAllowed_Wildcard(t *testing.T) {
	if !isSenderAllowed("anyone", []string{"*"}) {
		t.Error("wildcard should allow anyone")
	}
}

func TestIsSenderAllowed_EmptyList(t *testing.T) {
	if isSenderAllowed("someone", nil) {
		t.Error("nil allow list should not allow anyone")
	}
	if isSenderAllowed("someone", []string{}) {
		t.Error("empty allow list should not allow anyone")
	}
}

func TestIsSenderAllowed_ExactMatch(t *testing.T) {
	if !isSenderAllowed("+41796666864", []string{"+41796666864"}) {
		t.Error("exact E.164 match should be allowed")
	}
}

func TestIsSenderAllowed_NormalizedMatch(t *testing.T) {
	// Both should normalize to the same E.164
	if !isSenderAllowed("41796666864@s.whatsapp.net", []string{"+41796666864"}) {
		t.Error("JID vs E.164 should match after normalization")
	}
}

// ── WA-B: truncateForDedupe ──

func TestTruncateForDedupe_Short(t *testing.T) {
	result := truncateForDedupe("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestTruncateForDedupe_Long(t *testing.T) {
	result := truncateForDedupe("0123456789ABCDEF", 10)
	if result != "0123456789" {
		t.Errorf("expected '0123456789', got '%s'", result)
	}
}

// ── WA-D: ClampImageDimensions ──

func TestClampImageDimensions_WithinLimit(t *testing.T) {
	w, h, resize := ClampImageDimensions(1920, 1080)
	if resize {
		t.Error("image within limit should not need resize")
	}
	if w != 1920 || h != 1080 {
		t.Errorf("expected 1920x1080, got %dx%d", w, h)
	}
}

func TestClampImageDimensions_ExactLimit(t *testing.T) {
	w, h, resize := ClampImageDimensions(4096, 4096)
	if resize {
		t.Error("image at exact limit should not need resize")
	}
	if w != 4096 || h != 4096 {
		t.Errorf("expected 4096x4096, got %dx%d", w, h)
	}
}

func TestClampImageDimensions_OverLimit_Landscape(t *testing.T) {
	w, h, resize := ClampImageDimensions(8192, 4096)
	if !resize {
		t.Error("landscape image over limit should need resize")
	}
	if w != 4096 {
		t.Errorf("expected width=4096, got %d", w)
	}
	if h != 2048 {
		t.Errorf("expected height=2048, got %d", h)
	}
}

func TestClampImageDimensions_OverLimit_Portrait(t *testing.T) {
	w, h, resize := ClampImageDimensions(2000, 8000)
	if !resize {
		t.Error("portrait image over limit should need resize")
	}
	if h != 4096 {
		t.Errorf("expected height=4096, got %d", h)
	}
	if w != 1024 {
		t.Errorf("expected width=1024, got %d", w)
	}
}

// ── WA-D: OptimizeWebMedia ──

func TestOptimizeWebMedia_NilMedia(t *testing.T) {
	result := OptimizeWebMedia(nil, nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestOptimizeWebMedia_NonImage(t *testing.T) {
	media := &WebMedia{
		Buffer:      []byte("test"),
		ContentType: "video/mp4",
		Kind:        "video",
	}
	result := OptimizeWebMedia(media, nil)
	if result != media {
		t.Error("non-image media should be returned as-is")
	}
}

func TestOptimizeWebMedia_HEIC_NoOptimizer(t *testing.T) {
	media := &WebMedia{
		Buffer:      []byte("heic-data"),
		ContentType: "image/heic",
		Kind:        "image",
	}
	result := OptimizeWebMedia(media, nil)
	if result != media {
		t.Error("HEIC without optimizer should return original")
	}
}

func TestOptimizeWebMedia_JPEG_NoOptimizer(t *testing.T) {
	media := &WebMedia{
		Buffer:      []byte("jpeg-data"),
		ContentType: "image/jpeg",
		Kind:        "image",
	}
	result := OptimizeWebMedia(media, nil)
	if result != media {
		t.Error("JPEG without optimizer should return original")
	}
}

// ── WA-D: isHEICContentType ──

func TestIsHEICContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"image/heic", true},
		{"image/heif", true},
		{"IMAGE/HEIC", true},
		{"image/jpeg", false},
		{"image/png", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isHEICContentType(tt.ct)
		if got != tt.want {
			t.Errorf("isHEICContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

// ── WA-D: replaceExt ──

func TestReplaceExt(t *testing.T) {
	tests := []struct {
		filename, newExt, want string
	}{
		{"photo.heic", ".jpg", "photo.jpg"},
		{"photo.HEIC", ".jpg", "photo.jpg"},
		{"photo", ".jpg", "photo.jpg"},
		{"", ".jpg", ""},
		{"dir/photo.png", ".jpg", "dir/photo.jpg"},
	}
	for _, tt := range tests {
		got := replaceExt(tt.filename, tt.newExt)
		if got != tt.want {
			t.Errorf("replaceExt(%q, %q) = %q, want %q", tt.filename, tt.newExt, got, tt.want)
		}
	}
}

// ── WA-B: resolveSenderDisplay ──

func TestResolveSenderDisplay(t *testing.T) {
	tests := []struct {
		name string
		msg  *WebInboundMessage
		want string
	}{
		{
			name: "pushName",
			msg:  &WebInboundMessage{PushName: "Alice", SenderName: "Bob", From: "123"},
			want: "Alice",
		},
		{
			name: "senderName",
			msg:  &WebInboundMessage{SenderName: "Bob", From: "123"},
			want: "Bob",
		},
		{
			name: "senderE164",
			msg:  &WebInboundMessage{SenderE164: "+41796666864", From: "123"},
			want: "+41796666864",
		},
		{
			name: "from fallback",
			msg:  &WebInboundMessage{From: "123"},
			want: "123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSenderDisplay(tt.msg)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ── chunkByLength ──

func TestChunkByLength_Exact(t *testing.T) {
	chunks := chunkByLength("12345", 5)
	if len(chunks) != 1 || chunks[0] != "12345" {
		t.Errorf("exact limit should produce 1 chunk, got %v", chunks)
	}
}

func TestChunkByLength_Multiple(t *testing.T) {
	chunks := chunkByLength("123456", 2)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %v", len(chunks), chunks)
	}
	expected := []string{"12", "34", "56"}
	for i, want := range expected {
		if chunks[i] != want {
			t.Errorf("chunk[%d]=%q, want %q", i, chunks[i], want)
		}
	}
}

// ── chunkByNewline ──

func TestChunkByNewline_FitsInOne(t *testing.T) {
	chunks := chunkByNewline("a\nb\nc", 100)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d: %v", len(chunks), chunks)
	}
}

func TestChunkByNewline_SplitsAtNewline(t *testing.T) {
	chunks := chunkByNewline("hello\nworld\nfoo", 6)
	// "hello" (5) fits; "world" would make 11 > 6 → split
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "hello" {
		t.Errorf("chunk[0]=%q, want 'hello'", chunks[0])
	}
}
