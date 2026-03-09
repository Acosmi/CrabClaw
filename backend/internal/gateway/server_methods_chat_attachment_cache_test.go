package gateway

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

type staticCfgLoader struct {
	cfg *types.OpenAcosmiConfig
}

func (l *staticCfgLoader) LoadConfig() (*types.OpenAcosmiConfig, error) {
	return l.cfg, nil
}

type errCfgLoader struct {
	err error
}

func (l *errCfgLoader) LoadConfig() (*types.OpenAcosmiConfig, error) {
	return nil, l.err
}

type fakeSTTProvider struct {
	transcript string
}

func (f *fakeSTTProvider) Transcribe(_ context.Context, _ []byte, _ string) (string, error) {
	return f.transcript, nil
}

func (f *fakeSTTProvider) Name() string { return "fake-stt" }

func (f *fakeSTTProvider) TestConnection(_ context.Context) error { return nil }

type fakeDocConverter struct {
	markdown  string
	onConvert func()
}

func (f *fakeDocConverter) Convert(_ context.Context, _ []byte, _, _ string) (string, error) {
	if f.onConvert != nil {
		f.onConvert()
	}
	return f.markdown, nil
}

func (f *fakeDocConverter) SupportedFormats() []string { return []string{".txt", ".md", ".pdf"} }

func (f *fakeDocConverter) Name() string { return "fake-docconv" }

func (f *fakeDocConverter) TestConnection(_ context.Context) error { return nil }

func testAttachments() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type":     "audio",
			"content":  base64.StdEncoding.EncodeToString([]byte("audio-bytes")),
			"mimeType": "audio/wav",
			"fileName": "voice.wav",
		},
		{
			"type":     "document",
			"content":  base64.StdEncoding.EncodeToString([]byte("doc-bytes")),
			"mimeType": "application/pdf",
			"fileName": "note.pdf",
		},
	}
}

func testChatAttachmentConfig(sttProvider string) *types.OpenAcosmiConfig {
	return &types.OpenAcosmiConfig{
		STT: &types.STTConfig{
			Provider: sttProvider,
		},
		DocConv: &types.DocConvConfig{
			Provider: "builtin",
		},
	}
}

func TestProcessAttachmentsForChat_ProviderCacheReuse(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	var sttInitCount int32
	var docInitCount int32
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		atomic.AddInt32(&sttInitCount, 1)
		return &fakeSTTProvider{transcript: "hello from stt"}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		atomic.AddInt32(&docInitCount, 1)
		return &fakeDocConverter{markdown: "doc markdown"}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	attachments := testAttachments()

	out1, _ := processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)
	out2, _ := processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)

	if !strings.Contains(out1, "[语音转录]: hello from stt") || !strings.Contains(out1, "[文件: note.pdf]") {
		t.Fatalf("first process output missing expected conversions: %q", out1)
	}
	if !strings.Contains(out2, "[语音转录]: hello from stt") || !strings.Contains(out2, "[文件: note.pdf]") {
		t.Fatalf("second process output missing expected conversions: %q", out2)
	}
	if got := atomic.LoadInt32(&sttInitCount); got != 1 {
		t.Fatalf("stt provider should be reused within TTL, got init count=%d", got)
	}
	if got := atomic.LoadInt32(&docInitCount); got != 1 {
		t.Fatalf("doc converter should be reused within TTL, got init count=%d", got)
	}
}

func TestProcessAttachmentsForChat_ProviderCacheRefreshOnTTL(t *testing.T) {
	cache := newChatAttachmentProviderCache(5 * time.Millisecond)
	var sttInitCount int32
	var docInitCount int32
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		atomic.AddInt32(&sttInitCount, 1)
		return &fakeSTTProvider{transcript: "ttl-stt"}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		atomic.AddInt32(&docInitCount, 1)
		return &fakeDocConverter{markdown: "ttl-doc"}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	attachments := testAttachments()

	_, _ = processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)
	time.Sleep(15 * time.Millisecond)
	_, _ = processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)

	if got := atomic.LoadInt32(&sttInitCount); got < 2 {
		t.Fatalf("stt provider should refresh after TTL expiry, got init count=%d", got)
	}
	if got := atomic.LoadInt32(&docInitCount); got < 2 {
		t.Fatalf("doc converter should refresh after TTL expiry, got init count=%d", got)
	}
}

func TestProcessAttachmentsForChat_ProviderCacheRefreshOnConfigChange(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	var sttInitCount int32
	var docInitCount int32
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		atomic.AddInt32(&sttInitCount, 1)
		return &fakeSTTProvider{transcript: "cfg-" + strings.TrimSpace(cfg.Provider)}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		atomic.AddInt32(&docInitCount, 1)
		return &fakeDocConverter{markdown: "cfg-doc"}, nil
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	attachments := testAttachments()

	out1, _ := processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)
	if !strings.Contains(out1, "cfg-openai") {
		t.Fatalf("expected first output to use openai config, got %q", out1)
	}

	loader.cfg = testChatAttachmentConfig("qwen")
	out2, _ := processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)
	if !strings.Contains(out2, "cfg-qwen") {
		t.Fatalf("expected second output to use updated config, got %q", out2)
	}

	if got := atomic.LoadInt32(&sttInitCount); got < 2 {
		t.Fatalf("stt provider should refresh on config change, got init count=%d", got)
	}
	if got := atomic.LoadInt32(&docInitCount); got < 2 {
		t.Fatalf("doc converter should refresh on config change, got init count=%d", got)
	}
}

func TestProcessAttachmentsForChat_ProviderCacheConcurrentReuse(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	var sttInitCount int32
	var docInitCount int32
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		atomic.AddInt32(&sttInitCount, 1)
		return &fakeSTTProvider{transcript: "concurrent-stt"}, nil
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		atomic.AddInt32(&docInitCount, 1)
		return &fakeDocConverter{markdown: "concurrent-doc"}, nil
	}

	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	attachments := testAttachments()

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			out, _ := processAttachmentsForChatWithCache(context.Background(), "base", attachments, loader, cache)
			if !strings.Contains(out, "concurrent-stt") {
				t.Errorf("expected concurrent stt transcript in output, got %q", out)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&sttInitCount); got != 1 {
		t.Fatalf("stt provider should initialize once under concurrency, got=%d", got)
	}
	if got := atomic.LoadInt32(&docInitCount); got != 1 {
		t.Fatalf("doc converter should initialize once under concurrency, got=%d", got)
	}
}

func TestChatAttachmentConfigSignatureStable(t *testing.T) {
	cfg1 := &types.STTConfig{Provider: "openai"}
	cfg2 := &types.STTConfig{Provider: "openai"}
	cfg3 := &types.STTConfig{Provider: "qwen"}

	s1 := chatAttachmentConfigSignature(cfg1)
	s2 := chatAttachmentConfigSignature(cfg2)
	s3 := chatAttachmentConfigSignature(cfg3)
	if s1 == "" || s2 == "" || s3 == "" {
		t.Fatalf("signature should not be empty: s1=%q s2=%q s3=%q", s1, s2, s3)
	}
	if s1 != s2 {
		t.Fatalf("same config should generate same signature: %q vs %q", s1, s2)
	}
	if s1 == s3 {
		t.Fatalf("different config should generate different signature: %q vs %q", s1, s3)
	}
	if chatAttachmentConfigSignature(nil) != "" {
		t.Fatalf("nil config signature should be empty")
	}
}

func TestProcessAttachmentsForChat_ProviderInitFailureMessage(t *testing.T) {
	cache := newChatAttachmentProviderCache(3 * time.Second)
	cache.newSTTProvider = func(cfg *types.STTConfig) (media.STTProvider, error) {
		return nil, fmt.Errorf("mock stt init failure")
	}
	cache.newDocConverter = func(cfg *types.DocConvConfig) (media.DocConverter, error) {
		return nil, fmt.Errorf("mock docconv init failure")
	}
	loader := &staticCfgLoader{cfg: testChatAttachmentConfig("openai")}
	out, _ := processAttachmentsForChatWithCache(context.Background(), "base", testAttachments(), loader, cache)
	if !strings.Contains(out, "[语音附件: STT 初始化失败]") {
		t.Fatalf("expected STT init failure hint, got %q", out)
	}
	if !strings.Contains(out, "[文件: note.pdf, 转换器初始化失败]") {
		t.Fatalf("expected DocConv init failure hint, got %q", out)
	}
}
