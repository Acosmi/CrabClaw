package runner

// tool_email.go — Phase 7: send_email 工具定义 + executor
// 让智能体能主动发送邮件

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// EmailSender 邮件发送接口（由 gateway 注入 adapter）
type EmailSender interface {
	SendEmail(ctx context.Context, to, subject, body, account, sessionKey, cc string) (messageID string, err error)
}

// SendEmailToolDef 返回 send_email 工具定义
func SendEmailToolDef() llmclient.ToolDef {
	return llmclient.ToolDef{
		Name: "send_email",
		Description: "Send an email message. Supports new messages and thread replies.\n" +
			"Use 'to' for the recipient email address, 'subject' for the email subject, and 'body' for the message body (plain text).\n" +
			"To reply to an existing email thread, provide 'reply_to_session' with the session key.\n" +
			"Optionally specify 'account' to send from a specific email account (defaults to the configured default account).\n" +
			"Use 'cc' for carbon copy recipients (comma-separated).",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"to": {
					"type": "string",
					"description": "Recipient email address"
				},
				"subject": {
					"type": "string",
					"description": "Email subject line"
				},
				"body": {
					"type": "string",
					"description": "Email body (plain text)"
				},
				"account": {
					"type": "string",
					"description": "Email account ID to send from (optional, defaults to defaultAccount)"
				},
				"reply_to_session": {
					"type": "string",
					"description": "Session key to reply to (optional, auto-includes thread headers)"
				},
				"cc": {
					"type": "string",
					"description": "CC recipients, comma-separated (optional)"
				}
			},
			"required": ["to", "subject", "body"]
		}`),
	}
}

// executeSendEmail 执行 send_email 工具调用
func executeSendEmail(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	if params.EmailSender == nil {
		return "[send_email] Email sending is not available. No email account is configured.", nil
	}

	var input struct {
		To             string `json:"to"`
		Subject        string `json:"subject"`
		Body           string `json:"body"`
		Account        string `json:"account"`
		ReplyToSession string `json:"reply_to_session"`
		Cc             string `json:"cc"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Sprintf("[send_email] Invalid input: %v", err), nil
	}

	// 校验必填字段
	if strings.TrimSpace(input.To) == "" {
		return "[send_email] Error: 'to' (recipient email) is required.", nil
	}
	if strings.TrimSpace(input.Subject) == "" {
		return "[send_email] Error: 'subject' is required.", nil
	}
	if strings.TrimSpace(input.Body) == "" {
		return "[send_email] Error: 'body' is required.", nil
	}

	// 发送
	msgID, err := params.EmailSender.SendEmail(ctx, input.To, input.Subject, input.Body, input.Account, input.ReplyToSession, input.Cc)
	if err != nil {
		return fmt.Sprintf("[send_email] Failed to send email: %v", err), nil
	}

	return fmt.Sprintf("Email sent successfully.\nMessage-ID: %s\nTo: %s\nSubject: %s", msgID, input.To, input.Subject), nil
}
