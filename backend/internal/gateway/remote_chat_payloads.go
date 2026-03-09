package gateway

func buildRemoteAssistantMessage(
	replyText string,
	ts int64,
	mediaItems []ReplyMediaItem,
	mediaBase64 string,
	mediaMime string,
) map[string]interface{} {
	if replyText == "" && mediaBase64 == "" && len(mediaItems) == 0 {
		return nil
	}

	content := make([]interface{}, 0, 1+len(mediaItems))
	if replyText != "" {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": replyText,
		})
	}

	if len(mediaItems) > 0 {
		for _, item := range mediaItems {
			if item.Base64Data == "" {
				continue
			}
			mime := item.MimeType
			if mime == "" {
				mime = "image/png"
			}
			content = append(content, map[string]interface{}{
				"type": "image",
				"source": map[string]interface{}{
					"type":       "base64",
					"data":       item.Base64Data,
					"media_type": mime,
				},
			})
		}
	} else if mediaBase64 != "" {
		mime := mediaMime
		if mime == "" {
			mime = "image/png"
		}
		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"data":       mediaBase64,
				"media_type": mime,
			},
		})
	}

	return map[string]interface{}{
		"role":       "assistant",
		"content":    content,
		"timestamp":  ts,
		"stopReason": "stop",
		"usage": map[string]interface{}{
			"input":       0,
			"output":      0,
			"totalTokens": 0,
		},
	}
}

func buildRemoteAssistantChatPayload(
	sessionKey string,
	channel string,
	chatID string,
	replyText string,
	ts int64,
	mediaItems []ReplyMediaItem,
	mediaBase64 string,
	mediaMime string,
) map[string]interface{} {
	if replyText == "" && mediaBase64 == "" && len(mediaItems) == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"sessionKey": sessionKey,
		"channel":    channel,
		"role":       "assistant",
		"text":       replyText,
		"chatId":     chatID,
		"ts":         ts,
	}

	if len(mediaItems) > 0 {
		items := make([]map[string]string, 0, len(mediaItems))
		for _, item := range mediaItems {
			if item.Base64Data == "" {
				continue
			}
			items = append(items, map[string]string{
				"mediaBase64":   item.Base64Data,
				"mediaMimeType": item.MimeType,
			})
		}
		if len(items) > 0 {
			payload["mediaItems"] = items
			payload["mediaBase64"] = items[0]["mediaBase64"]
			payload["mediaMimeType"] = items[0]["mediaMimeType"]
		}
	} else if mediaBase64 != "" {
		payload["mediaBase64"] = mediaBase64
		payload["mediaMimeType"] = mediaMime
	}

	return payload
}
