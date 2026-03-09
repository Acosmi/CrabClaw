package llmclient

import (
	"strings"
	"testing"
)

func TestToOllamaMessages_UserImageFallback(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "text", Text: "describe"},
				{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "ZmFrZQ==",
					},
				},
			},
		},
	}

	result := toOllamaMessages("system prompt", msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Fatalf("expected user role, got %q", result[1].Role)
	}
	if !strings.Contains(result[1].Content, "describe") {
		t.Fatalf("expected original text in content, got %q", result[1].Content)
	}
	if !strings.Contains(result[1].Content, ollamaUnsupportedImageNotice) {
		t.Fatalf("expected fallback notice, got %q", result[1].Content)
	}
}

func TestToOllamaMessages_ImageOnlyFallback(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "ZmFrZQ==",
					},
				},
			},
		},
	}

	result := toOllamaMessages("", msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != ollamaUnsupportedImageNotice {
		t.Fatalf("expected fallback-only content, got %q", result[0].Content)
	}
}
