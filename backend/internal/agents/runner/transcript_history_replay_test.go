package runner

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
)

func TestLoadPriorMessages_ReplaysUserImageHistory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-history-image"
	sessionFile := filepath.Join(tmpDir, sessionID+".jsonl")

	mgr := session.NewSessionManager("")
	if _, err := mgr.EnsureSessionFile(sessionID, sessionFile); err != nil {
		t.Fatalf("ensure session file: %v", err)
	}

	if err := mgr.AppendMessage(sessionID, sessionFile, session.TranscriptEntry{
		Role: "user",
		Content: []session.ContentBlock{
			session.TextBlock("[用户发送了附件]"),
			{
				Type:     "image",
				FileName: "photo.png",
				MimeType: "image/png",
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "iVBORw0KGgo=",
				},
			},
		},
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}

	if err := mgr.AppendMessage(sessionID, sessionFile, session.TranscriptEntry{
		Role: "assistant",
		Content: []session.ContentBlock{
			session.TextBlock("我看到了图片。"),
		},
	}); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	r := &EmbeddedAttemptRunner{}
	msgs := r.loadPriorMessages(AttemptParams{
		SessionID:   sessionID,
		SessionFile: sessionFile,
	}, slog.Default())

	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("msgs[0].role = %q, want user", msgs[0].Role)
	}
	if len(msgs[0].Content) != 2 {
		t.Fatalf("len(msgs[0].content) = %d, want 2", len(msgs[0].Content))
	}
	if msgs[0].Content[1].Type != "image" {
		t.Fatalf("msgs[0].content[1].type = %q, want image", msgs[0].Content[1].Type)
	}
	if msgs[0].Content[1].Source == nil || msgs[0].Content[1].Source.Data != "iVBORw0KGgo=" {
		t.Fatal("expected prior user image to be replayed")
	}
}

func TestLoadPriorMessages_ReplaysVideoAsFallbackSummary(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-history-video"
	sessionFile := filepath.Join(tmpDir, sessionID+".jsonl")

	mgr := session.NewSessionManager("")
	if _, err := mgr.EnsureSessionFile(sessionID, sessionFile); err != nil {
		t.Fatalf("ensure session file: %v", err)
	}

	if err := mgr.AppendMessage(sessionID, sessionFile, session.TranscriptEntry{
		Role: "user",
		Content: []session.ContentBlock{
			session.TextBlock("[用户发送了附件]"),
			{
				Type:     "video",
				FileName: "clip.mp4",
				MimeType: "video/mp4",
			},
		},
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}

	r := &EmbeddedAttemptRunner{}
	msgs := r.loadPriorMessages(AttemptParams{
		SessionID:   sessionID,
		SessionFile: sessionFile,
	}, slog.Default())

	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if len(msgs[0].Content) != 1 {
		t.Fatalf("len(msgs[0].content) = %d, want 1", len(msgs[0].Content))
	}
	if msgs[0].Content[0].Type != "text" {
		t.Fatalf("msgs[0].content[0].type = %q, want text", msgs[0].Content[0].Type)
	}
	if msgs[0].Content[0].Text != "[视频附件: clip.mp4]" {
		t.Fatalf("msgs[0].content[0].text = %q, want video fallback summary", msgs[0].Content[0].Text)
	}
}
