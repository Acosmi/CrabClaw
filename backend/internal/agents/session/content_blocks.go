package session

import (
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// TextBlock 创建标准 text content block。
func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// BuildChatMessage 将 session/transcript content blocks 转换为 LLM 可消费的消息。
// 当前统一策略：
// 1. text 始终保留。
// 2. user image 走 provider 原生多模态输入。
// 3. document/audio/video 暂不在历史重放中原样送模，继续依赖增强文本。
// 4. assistant image 暂不重放，避免把非稳定 provider 行为引入历史链。
func BuildChatMessage(role string, blocks []ContentBlock) *llmclient.ChatMessage {
	if role != "user" && role != "assistant" {
		return nil
	}

	textBlocks := make([]llmclient.ContentBlock, 0, len(blocks))
	imageBlocks := make([]llmclient.ContentBlock, 0, len(blocks))
	fallbackTexts := make([]string, 0, len(blocks))
	existingTexts := make([]string, 0, len(blocks))

	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			existingTexts = append(existingTexts, block.Text)
			textBlocks = append(textBlocks, llmclient.ContentBlock{
				Type: "text",
				Text: block.Text,
			})
		case "image":
			if role != "user" || block.Source == nil || block.Source.Data == "" || block.Source.MediaType == "" {
				continue
			}
			imageBlocks = append(imageBlocks, llmclient.ContentBlock{
				Type: "image",
				Source: &llmclient.ImageSource{
					Type:      block.Source.Type,
					MediaType: block.Source.MediaType,
					Data:      block.Source.Data,
				},
			})
		case "document", "audio", "video":
			if fallback := attachmentFallbackText(block); fallback != "" &&
				shouldIncludeAttachmentFallback(existingTexts, block) {
				fallbackTexts = append(fallbackTexts, fallback)
			}
		}
	}

	if len(fallbackTexts) > 0 {
		filtered := textBlocks[:0]
		for _, block := range textBlocks {
			if isGenericAttachmentPlaceholderText(block.Text) {
				continue
			}
			filtered = append(filtered, block)
		}
		textBlocks = filtered
	}

	content := make([]llmclient.ContentBlock, 0, len(textBlocks)+len(fallbackTexts)+len(imageBlocks))
	content = append(content, textBlocks...)
	for _, fallback := range fallbackTexts {
		content = append(content, llmclient.ContentBlock{
			Type: "text",
			Text: fallback,
		})
	}
	content = append(content, imageBlocks...)

	if len(content) == 0 {
		return nil
	}
	return &llmclient.ChatMessage{
		Role:    role,
		Content: content,
	}
}

// BuildChatMessageWithTextAndAttachments 构建“文本 + 附件”的统一 user/assistant 消息。
func BuildChatMessageWithTextAndAttachments(role, text string, attachments []ContentBlock) *llmclient.ChatMessage {
	content := make([]ContentBlock, 0, 1+len(attachments))
	if strings.TrimSpace(text) != "" {
		content = append(content, TextBlock(text))
	}
	if len(attachments) > 0 {
		content = append(content, attachments...)
	}
	return BuildChatMessage(role, content)
}

func isGenericAttachmentPlaceholderText(text string) bool {
	return strings.TrimSpace(text) == "[用户发送了附件]"
}

func attachmentFallbackText(block ContentBlock) string {
	name := strings.TrimSpace(block.FileName)
	switch block.Type {
	case "document":
		if name == "" {
			name = "untitled"
		}
		return fmt.Sprintf("[文件: %s]", name)
	case "audio":
		if name != "" {
			return fmt.Sprintf("[语音附件: %s]", name)
		}
		return "[语音附件]"
	case "video":
		if name != "" {
			return fmt.Sprintf("[视频附件: %s]", name)
		}
		return "[视频附件]"
	default:
		return ""
	}
}

func shouldIncludeAttachmentFallback(existingTexts []string, block ContentBlock) bool {
	if len(existingTexts) == 0 {
		return true
	}
	joined := strings.Join(existingTexts, "\n")
	switch block.Type {
	case "document":
		if block.FileName != "" && strings.Contains(joined, "[文件: "+strings.TrimSpace(block.FileName)) {
			return false
		}
		return !strings.Contains(joined, "[文件:")
	case "audio":
		return !strings.Contains(joined, "[语音转录]") && !strings.Contains(joined, "[语音附件:")
	case "video":
		return !strings.Contains(joined, "[视频附件:")
	default:
		return false
	}
}
