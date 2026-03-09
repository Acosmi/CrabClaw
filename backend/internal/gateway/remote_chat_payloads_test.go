package gateway

import "testing"

func TestBuildRemoteAssistantMessageIncludesTextAndMedia(t *testing.T) {
	message := buildRemoteAssistantMessage(
		"done",
		123,
		[]ReplyMediaItem{{Base64Data: "abc", MimeType: "image/png"}},
		"",
		"",
	)
	if message == nil {
		t.Fatal("expected message")
	}
	if role, _ := message["role"].(string); role != "assistant" {
		t.Fatalf("unexpected role: %v", message["role"])
	}
	if timestamp, _ := message["timestamp"].(int64); timestamp != 123 {
		t.Fatalf("unexpected timestamp: %v", message["timestamp"])
	}
	content, _ := message["content"].([]interface{})
	if len(content) != 2 {
		t.Fatalf("expected text + image blocks, got %d", len(content))
	}
}

func TestBuildRemoteAssistantChatPayloadCarriesMediaFields(t *testing.T) {
	payload := buildRemoteAssistantChatPayload(
		"feishu:oc_1",
		"feishu",
		"oc_1",
		"done",
		456,
		[]ReplyMediaItem{{Base64Data: "abc", MimeType: "image/png"}},
		"",
		"",
	)
	if payload == nil {
		t.Fatal("expected payload")
	}
	if payload["sessionKey"] != "feishu:oc_1" || payload["channel"] != "feishu" {
		t.Fatalf("unexpected routing payload: %+v", payload)
	}
	if payload["mediaBase64"] != "abc" || payload["mediaMimeType"] != "image/png" {
		t.Fatalf("unexpected media fields: %+v", payload)
	}
	items, _ := payload["mediaItems"].([]map[string]string)
	if len(items) != 1 {
		t.Fatalf("expected one media item, got %+v", payload["mediaItems"])
	}
}
