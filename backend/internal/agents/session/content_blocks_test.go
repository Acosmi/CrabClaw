package session

import "testing"

func TestBuildChatMessage_UserImagePreserved(t *testing.T) {
	msg := BuildChatMessage("user", []ContentBlock{
		TextBlock("[用户发送了附件]"),
		{
			Type:     "image",
			FileName: "photo.png",
			MimeType: "image/png",
			Source: &MediaSource{
				Type:      "base64",
				MediaType: "image/png",
				Data:      "iVBORw0KGgo=",
			},
		},
	})

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("len(content) = %d, want 2", len(msg.Content))
	}
	if msg.Content[1].Type != "image" {
		t.Fatalf("content[1].type = %q, want image", msg.Content[1].Type)
	}
	if msg.Content[1].Source == nil || msg.Content[1].Source.Data != "iVBORw0KGgo=" {
		t.Fatal("expected image source data to be preserved")
	}
}

func TestBuildChatMessage_AssistantImageIgnored(t *testing.T) {
	msg := BuildChatMessage("assistant", []ContentBlock{
		TextBlock("这是图片"),
		{
			Type:     "image",
			FileName: "photo.png",
			MimeType: "image/png",
			Source: &MediaSource{
				Type:      "base64",
				MediaType: "image/png",
				Data:      "iVBORw0KGgo=",
			},
		},
	})

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(msg.Content))
	}
	if msg.Content[0].Type != "text" {
		t.Fatalf("content[0].type = %q, want text", msg.Content[0].Type)
	}
}

func TestBuildChatMessageWithTextAndAttachments_User(t *testing.T) {
	msg := BuildChatMessageWithTextAndAttachments("user", "look at this", []ContentBlock{
		{
			Type:     "image",
			FileName: "photo.png",
			MimeType: "image/png",
			Source: &MediaSource{
				Type:      "base64",
				MediaType: "image/png",
				Data:      "iVBORw0KGgo=",
			},
		},
	})

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("len(content) = %d, want 2", len(msg.Content))
	}
	if msg.Content[0].Text != "look at this" {
		t.Fatalf("content[0].text = %q, want %q", msg.Content[0].Text, "look at this")
	}
	if msg.Content[1].Type != "image" {
		t.Fatalf("content[1].type = %q, want image", msg.Content[1].Type)
	}
}

func TestBuildChatMessageWithTextAndAttachments_VideoAddsFallbackSummary(t *testing.T) {
	msg := BuildChatMessageWithTextAndAttachments("user", "看下这个", []ContentBlock{
		{
			Type:     "video",
			FileName: "clip.mp4",
			MimeType: "video/mp4",
			Source: &MediaSource{
				Type:      "base64",
				MediaType: "video/mp4",
				Data:      "ZmFrZS12aWRlby1ieXRlcw==",
			},
		},
	})

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("len(content) = %d, want 2", len(msg.Content))
	}
	if msg.Content[1].Type != "text" {
		t.Fatalf("content[1].type = %q, want text", msg.Content[1].Type)
	}
	if msg.Content[1].Text != "[视频附件: clip.mp4]" {
		t.Fatalf("content[1].text = %q, want video fallback summary", msg.Content[1].Text)
	}
}

func TestBuildChatMessageWithTextAndAttachments_VideoPlaceholderReplaced(t *testing.T) {
	msg := BuildChatMessageWithTextAndAttachments("user", "[用户发送了附件]", []ContentBlock{
		{
			Type:     "video",
			FileName: "clip.mp4",
			MimeType: "video/mp4",
		},
	})

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(msg.Content))
	}
	if msg.Content[0].Text != "[视频附件: clip.mp4]" {
		t.Fatalf("content[0].text = %q, want placeholder replaced by fallback summary", msg.Content[0].Text)
	}
}

func TestBuildChatMessage_DocumentMarkerNotDuplicated(t *testing.T) {
	msg := BuildChatMessageWithTextAndAttachments("user", "[文件: report.pdf]\n内容摘要", []ContentBlock{
		{
			Type:     "document",
			FileName: "report.pdf",
			MimeType: "application/pdf",
		},
	})

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if len(msg.Content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(msg.Content))
	}
	if msg.Content[0].Text != "[文件: report.pdf]\n内容摘要" {
		t.Fatalf("unexpected text duplication: %q", msg.Content[0].Text)
	}
}
