package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// TestProcessAttachmentsForChat_ImageBlock 验证 image 附件返回正确的 ContentBlock。
func TestProcessAttachmentsForChat_ImageBlock(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	imgData := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	attachments := []map[string]interface{}{
		{
			"type":     "image",
			"content":  imgData,
			"mimeType": "image/png",
			"fileName": "screenshot.png",
			"fileSize": float64(1024),
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "look at this", attachments, loader, cache)

	// 增强文本不变（image 不产生文本增强）
	if text != "look at this" {
		t.Fatalf("expected text unchanged, got %q", text)
	}

	// 应有 1 个 image block
	if len(blocks) != 1 {
		t.Fatalf("expected 1 attachment block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Type != "image" {
		t.Fatalf("expected type=image, got %q", b.Type)
	}
	if b.FileName != "screenshot.png" {
		t.Fatalf("expected fileName=screenshot.png, got %q", b.FileName)
	}
	if b.FileSize != 1024 {
		t.Fatalf("expected fileSize=1024, got %d", b.FileSize)
	}
	if b.MimeType != "image/png" {
		t.Fatalf("expected mimeType=image/png, got %q", b.MimeType)
	}
	if b.Source == nil || b.Source.Data != imgData {
		t.Fatalf("expected source with base64 data")
	}
}

// TestProcessAttachmentsForChat_VideoBlock 验证 video 附件返回正确的 ContentBlock。
func TestProcessAttachmentsForChat_VideoBlock(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	vidData := base64.StdEncoding.EncodeToString([]byte("fake-mp4-bytes"))
	attachments := []map[string]interface{}{
		{
			"type":     "video",
			"content":  vidData,
			"mimeType": "video/mp4",
			"fileName": "clip.mp4",
			"fileSize": float64(2048),
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "check video", attachments, loader, cache)

	if text != "check video" {
		t.Fatalf("expected text unchanged, got %q", text)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Type != "video" {
		t.Fatalf("expected type=video, got %q", b.Type)
	}
	if b.Source == nil || b.Source.Data != vidData {
		t.Fatalf("expected source with base64 data")
	}
}

// TestProcessAttachmentsForChat_AudioBlockWithSTT 验证 audio 附件同时产生文本增强和 ContentBlock。
func TestProcessAttachmentsForChat_AudioBlockWithSTT(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		return &fakeSTTProvider{transcript: "hello world"}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		return &fakeDocConverter{markdown: ""}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	audioData := base64.StdEncoding.EncodeToString([]byte("fake-audio"))
	attachments := []map[string]interface{}{
		{
			"type":     "audio",
			"content":  audioData,
			"mimeType": "audio/webm",
			"fileName": "recording.webm",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)

	// 增强文本应包含 STT 结果
	if !strings.Contains(text, "[语音转录]: hello world") {
		t.Fatalf("expected STT transcript in text, got %q", text)
	}

	// 应有 1 个 audio block（含原始数据）
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Type != "audio" {
		t.Fatalf("expected type=audio, got %q", b.Type)
	}
	if b.Source == nil || b.Source.Data != audioData {
		t.Fatalf("expected audio source with base64 data")
	}
}

func TestProcessAttachmentsForChat_AudioWithoutConfigLoaderFallsBackToMarker(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	audioData := base64.StdEncoding.EncodeToString([]byte("fake-audio"))
	attachments := []map[string]interface{}{
		{
			"type":     "audio",
			"content":  audioData,
			"mimeType": "audio/webm",
			"fileName": "recording.webm",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "", attachments, nil, cache)

	if text != "[语音附件: 配置不可用，未执行语音转录]" {
		t.Fatalf("expected deterministic fallback marker, got %q", text)
	}
	if len(blocks) != 1 || blocks[0].Type != "audio" {
		t.Fatalf("expected preserved audio block, got %+v", blocks)
	}
}

func TestProcessAttachmentsForChat_AudioOnConfigLoadErrorFallsBackToMarker(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &errCfgLoader{err: errors.New("boom")}
	audioData := base64.StdEncoding.EncodeToString([]byte("fake-audio"))
	attachments := []map[string]interface{}{
		{
			"type":     "audio",
			"content":  audioData,
			"mimeType": "audio/webm",
			"fileName": "recording.webm",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "listen", attachments, loader, cache)

	if text != "listen\n[语音附件: 配置不可用，未执行语音转录]" {
		t.Fatalf("expected text plus deterministic fallback marker, got %q", text)
	}
	if len(blocks) != 1 || blocks[0].Type != "audio" {
		t.Fatalf("expected preserved audio block, got %+v", blocks)
	}
}

func TestProcessAttachmentsForChat_AudioEmptyTranscriptUsesExplicitMarker(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		return &fakeSTTProvider{transcript: ""}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	audioData := base64.StdEncoding.EncodeToString([]byte("fake-audio"))
	attachments := []map[string]interface{}{
		{
			"type":     "audio",
			"content":  audioData,
			"mimeType": "audio/webm",
			"fileName": "recording.webm",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "", attachments, loader, cache)

	if text != "[语音附件: 转录结果为空]" {
		t.Fatalf("expected explicit empty transcript marker, got %q", text)
	}
	if len(blocks) != 1 || blocks[0].Type != "audio" {
		t.Fatalf("expected preserved audio block, got %+v", blocks)
	}
}

// TestProcessAttachmentsForChat_DocumentBlock 验证 document 附件产生 metadata block。
func TestProcessAttachmentsForChat_DocumentBlock(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		return &fakeSTTProvider{transcript: ""}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		return &fakeDocConverter{markdown: "# Title\ncontent"}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	docData := base64.StdEncoding.EncodeToString([]byte("doc content"))
	attachments := []map[string]interface{}{
		{
			"type":     "document",
			"content":  docData,
			"mimeType": "application/pdf",
			"fileName": "report.pdf",
			"fileSize": float64(512),
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "read this", attachments, loader, cache)

	// 增强文本应包含 DocConv 结果
	if !strings.Contains(text, "[文件: report.pdf]") {
		t.Fatalf("expected doc conv in text, got %q", text)
	}

	// 应有 1 个 document block（metadata only）
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Type != "document" {
		t.Fatalf("expected type=document, got %q", b.Type)
	}
	if b.FileName != "report.pdf" {
		t.Fatalf("expected fileName=report.pdf, got %q", b.FileName)
	}
	if b.FileSize != 512 {
		t.Fatalf("expected fileSize=512, got %d", b.FileSize)
	}
	// document block 不存储原始数据（仅 metadata）
	if b.Source != nil {
		t.Fatalf("expected no source for document block")
	}
}

func TestProcessAttachmentsForChat_TextDocumentInlineWithoutDocConv(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	docConverterUsed := false
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		return &fakeDocConverter{
			markdown: "should not be used",
			onConvert: func() {
				docConverterUsed = true
			},
		}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	docData := base64.StdEncoding.EncodeToString([]byte("# Title\nhello"))
	attachments := []map[string]interface{}{
		{
			"type":     "document",
			"content":  docData,
			"mimeType": "text/markdown",
			"fileName": "readme.md",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "", attachments, loader, cache)

	if docConverterUsed {
		t.Fatalf("expected inline text document path to bypass doc converter")
	}
	if !strings.Contains(text, "[文件: readme.md]") || !strings.Contains(text, "# Title") || !strings.Contains(text, "hello") {
		t.Fatalf("expected inline markdown content, got %q", text)
	}
	if strings.Contains(text, "should not be used") {
		t.Fatalf("unexpected doc converter output in text: %q", text)
	}
	if len(blocks) != 1 || blocks[0].Type != "document" {
		t.Fatalf("expected 1 document block, got %+v", blocks)
	}
}

func TestProcessAttachmentsForChat_BinaryDocumentWithoutConfigFallsBackToFileMarker(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	docData := base64.StdEncoding.EncodeToString([]byte("pdf"))
	attachments := []map[string]interface{}{
		{
			"type":     "document",
			"content":  docData,
			"mimeType": "application/pdf",
			"fileName": "manual.pdf",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "", attachments, nil, cache)

	if text != "[文件: manual.pdf]" {
		t.Fatalf("expected file marker fallback, got %q", text)
	}
	if len(blocks) != 1 || blocks[0].Type != "document" || blocks[0].FileName != "manual.pdf" {
		t.Fatalf("expected preserved document block, got %+v", blocks)
	}
}

// TestProcessAttachmentsForChat_ImageOnlyNoText 验证纯图片（无文字）场景。
func TestProcessAttachmentsForChat_ImageOnlyNoText(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	imgData := base64.StdEncoding.EncodeToString([]byte("png"))
	attachments := []map[string]interface{}{
		{
			"type":    "image",
			"content": imgData,
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "", attachments, loader, cache)

	// text 应为空（image 不产生文本增强）
	if text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
	// 但 blocks 应有 1 个 image block
	if len(blocks) != 1 {
		t.Fatalf("expected 1 image block, got %d", len(blocks))
	}
	if blocks[0].Type != "image" {
		t.Fatalf("expected image type, got %q", blocks[0].Type)
	}
}

func TestProcessAttachmentsForChat_ImagePreservedWithoutConfigLoader(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	imgData := base64.StdEncoding.EncodeToString([]byte("png"))
	attachments := []map[string]interface{}{
		{
			"type":     "image",
			"content":  imgData,
			"mimeType": "image/png",
			"fileName": "raw.png",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "", attachments, nil, cache)

	if text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 image block, got %d", len(blocks))
	}
	if blocks[0].Type != "image" || blocks[0].Source == nil || blocks[0].Source.Data != imgData {
		t.Fatalf("expected preserved image block, got %+v", blocks[0])
	}
}

func TestProcessAttachmentsForChat_ImagePreservedOnConfigLoadError(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &errCfgLoader{err: errors.New("boom")}
	imgData := base64.StdEncoding.EncodeToString([]byte("png"))
	attachments := []map[string]interface{}{
		{
			"type":     "image",
			"content":  imgData,
			"mimeType": "image/png",
			"fileName": "broken.png",
		},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "look", attachments, loader, cache)

	if text != "look" {
		t.Fatalf("expected unchanged text, got %q", text)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 image block, got %d", len(blocks))
	}
	if blocks[0].Type != "image" || blocks[0].Source == nil || blocks[0].Source.Data != imgData {
		t.Fatalf("expected preserved image block, got %+v", blocks[0])
	}
}

// TestProcessAttachmentsForChat_MixedAttachments 验证混合附件场景。
func TestProcessAttachmentsForChat_MixedAttachments(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		return &fakeSTTProvider{transcript: "stt result"}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		return &fakeDocConverter{markdown: "doc text"}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	attachments := []map[string]interface{}{
		{"type": "image", "content": base64.StdEncoding.EncodeToString([]byte("img")), "mimeType": "image/jpeg"},
		{"type": "audio", "content": base64.StdEncoding.EncodeToString([]byte("aud")), "mimeType": "audio/webm"},
		{"type": "document", "content": base64.StdEncoding.EncodeToString([]byte("doc")), "mimeType": "application/pdf", "fileName": "f.pdf"},
		{"type": "video", "content": base64.StdEncoding.EncodeToString([]byte("vid")), "mimeType": "video/mp4"},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "hello", attachments, loader, cache)

	// 应有 4 个 blocks（每种类型 1 个）
	if len(blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(blocks))
	}
	types := map[string]bool{}
	for _, b := range blocks {
		types[b.Type] = true
	}
	for _, expected := range []string{"image", "audio", "document", "video"} {
		if !types[expected] {
			t.Fatalf("missing block type %q", expected)
		}
	}

	// 增强文本应包含 STT + DocConv 结果
	if !strings.Contains(text, "[语音转录]: stt result") {
		t.Fatalf("expected STT in text, got %q", text)
	}
	if !strings.Contains(text, "[文件: f.pdf]") {
		t.Fatalf("expected doc in text, got %q", text)
	}
}

// TestProcessAttachmentsForChat_EmptyContentSkipped 验证空 content 的附件被跳过。
func TestProcessAttachmentsForChat_EmptyContentSkipped(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	attachments := []map[string]interface{}{
		{"type": "image", "content": ""},
	}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "hi", attachments, loader, cache)

	if text != "hi" {
		t.Fatalf("expected unchanged text, got %q", text)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty content, got %d", len(blocks))
	}
}

// TestProcessAttachmentsForChat_NoAttachments 验证无附件时返回 nil blocks。
func TestProcessAttachmentsForChat_NoAttachments(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}

	text, blocks := processAttachmentsForChatWithCache(context.Background(), "hello", nil, loader, cache)

	if text != "hello" {
		t.Fatalf("expected unchanged text, got %q", text)
	}
	if blocks != nil {
		t.Fatalf("expected nil blocks, got %v", blocks)
	}
}
