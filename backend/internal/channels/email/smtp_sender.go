package email

// smtp_sender.go — Phase 6: SMTP 发件
// TLS/STARTTLS 连接 + AUTH LOGIN + 回复构造 + 线程头 + Message-ID 生成
// 按需连接（不持久 SMTP 连接），修 F-09

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"net/smtp"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// SMTPSender SMTP 发件器
type SMTPSender struct {
	config *types.EmailAccountConfig
}

// NewSMTPSender 创建 SMTP 发件器
func NewSMTPSender(config *types.EmailAccountConfig) *SMTPSender {
	return &SMTPSender{config: config}
}

// SendParams 发件参数
type SendParams struct {
	To      []string
	Cc      []string
	Subject string
	Body    string

	// 线程头（回复时填充，修 D-01）
	InReplyTo  string
	References []string

	// 调用方覆盖（可选）
	FromName string
}

// SendResult 发件结果
type SendResult struct {
	MessageID  string
	SentAt     time.Time
	Recipients int
}

// ThreadContext 线程上下文（用于 state store 持久化 + 回复头恢复）
type ThreadContext struct {
	LastMessageID string   `json:"lastMessageId"`
	References    []string `json:"references,omitempty"`
	Subject       string   `json:"subject,omitempty"`
}

// Send 发送邮件（按需连接，发完即断）
func (s *SMTPSender) Send(params SendParams) (*SendResult, error) {
	smtpCfg := s.config.SMTP
	if smtpCfg == nil {
		return nil, fmt.Errorf("SMTP config is nil")
	}

	if len(params.To) == 0 {
		return nil, fmt.Errorf("no recipients")
	}

	// 生成 Message-ID
	msgID := generateMessageID(s.config.Address)

	// 构造邮件
	mailData := buildMailMessage(s.config, params, msgID)

	// 收集所有收件人
	allRecipients := make([]string, 0, len(params.To)+len(params.Cc))
	allRecipients = append(allRecipients, params.To...)
	allRecipients = append(allRecipients, params.Cc...)

	// SMTP 连接 + 发送
	addr := fmt.Sprintf("%s:%d", smtpCfg.Host, smtpCfg.Port)
	slog.Info("email: SMTP sending",
		"addr", addr,
		"to", strings.Join(params.To, ","),
		"subject", params.Subject,
		"msgID", msgID,
	)

	if err := smtpSend(s.config, addr, s.config.Address, allRecipients, mailData); err != nil {
		return nil, fmt.Errorf("SMTP send: %w", err)
	}

	return &SendResult{
		MessageID:  msgID,
		SentAt:     time.Now(),
		Recipients: len(allRecipients),
	}, nil
}

// smtpSend 底层 SMTP 发送（TLS/STARTTLS/Plain）
func smtpSend(config *types.EmailAccountConfig, addr, from string, to []string, data []byte) error {
	smtpCfg := config.SMTP
	login := config.Login
	if login == "" {
		login = config.Address
	}
	password := config.Auth.Password

	tlsConfig := &tls.Config{ServerName: smtpCfg.Host}

	switch smtpCfg.Security {
	case types.EmailSecurityTLS, "":
		return smtpSendTLS(addr, login, password, from, to, data, tlsConfig)
	case types.EmailSecuritySTARTTLS:
		return smtpSendSTARTTLS(addr, login, password, from, to, data, tlsConfig)
	case types.EmailSecurityNone:
		return smtpSendPlain(addr, login, password, smtpCfg.Host, from, to, data)
	default:
		return fmt.Errorf("unsupported SMTP security: %s", smtpCfg.Security)
	}
}

// smtpSendTLS 直连 TLS (端口 465)
func smtpSendTLS(addr, login, password, from string, to []string, data []byte, tlsConfig *tls.Config) error {
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial %s: %w", addr, err)
	}
	defer conn.Close()

	host := tlsConfig.ServerName
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP client: %w", err)
	}
	defer client.Close()

	return smtpClientSend(client, login, password, tlsConfig.ServerName, from, to, data)
}

// smtpSendSTARTTLS STARTTLS 升级
func smtpSendSTARTTLS(addr, login, password, from string, to []string, data []byte, tlsConfig *tls.Config) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP dial %s: %w", addr, err)
	}
	defer client.Close()

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS: %w", err)
	}

	return smtpClientSend(client, login, password, tlsConfig.ServerName, from, to, data)
}

// smtpSendPlain 明文连接（不推荐）
func smtpSendPlain(addr, login, password, smtpHost, from string, to []string, data []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP dial %s: %w", addr, err)
	}
	defer client.Close()

	return smtpClientSend(client, login, password, smtpHost, from, to, data)
}

// smtpClientSend SMTP 客户端发送流程
// smtpHost: SMTP 服务器主机名，用于 smtp.PlainAuth TLS 身份验证
func smtpClientSend(client *smtp.Client, login, password, smtpHost, from string, to []string, data []byte) error {
	// AUTH LOGIN — host 使用 SMTP 服务器名（smtp.PlainAuth 用于 TLS 身份验证）
	auth := smtp.PlainAuth("", login, password, smtpHost)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP AUTH: %w", err)
	}

	// MAIL FROM
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	// RCPT TO
	for _, rcpt := range to {
		addr := extractEmailAddr(rcpt)
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", addr, err)
		}
	}

	// DATA
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	return client.Quit()
}

// buildMailMessage 构造完整邮件原文
func buildMailMessage(config *types.EmailAccountConfig, params SendParams, messageID string) []byte {
	var buf strings.Builder

	// From
	fromName := params.FromName
	if fromName == "" && config.SMTP != nil {
		fromName = config.SMTP.FromName
	}
	if fromName != "" {
		buf.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromName, config.Address))
	} else {
		buf.WriteString(fmt.Sprintf("From: %s\r\n", config.Address))
	}

	// To
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(params.To, ", ")))

	// Cc
	if len(params.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(params.Cc, ", ")))
	}

	// Subject (RFC 2047 编码非 ASCII 字符)
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", encodeSubjectRFC2047(params.Subject)))

	// Date
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))

	// Message-ID
	buf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))

	// 线程头（回复时填充，修 D-01）
	if params.InReplyTo != "" {
		buf.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", params.InReplyTo))
	}
	if len(params.References) > 0 {
		buf.WriteString(fmt.Sprintf("References: %s\r\n", strings.Join(params.References, " ")))
	}

	// 防环 + 追踪头
	buf.WriteString("Auto-Submitted: auto-generated\r\n")
	buf.WriteString("X-OpenAcosmi-Channel: email\r\n")
	buf.WriteString(fmt.Sprintf("X-OpenAcosmi-Account: %s\r\n", config.Address))

	// Content-Type
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")

	// 空行 + body
	buf.WriteString("\r\n")
	buf.WriteString(encodeQuotedPrintableBody(params.Body))

	return []byte(buf.String())
}

// encodeSubjectRFC2047 对含非 ASCII 字符的 Subject 进行 RFC 2047 B 编码
func encodeSubjectRFC2047(subject string) string {
	for i := 0; i < len(subject); i++ {
		if subject[i] > 127 {
			return mime.BEncoding.Encode("UTF-8", subject)
		}
	}
	return subject // 纯 ASCII 无需编码
}

// generateMessageID 生成唯一 Message-ID
func generateMessageID(fromAddr string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	domain := "openacosmi.local"
	if idx := strings.LastIndex(fromAddr, "@"); idx >= 0 {
		domain = fromAddr[idx+1:]
	}
	return fmt.Sprintf("<%s.%d@%s>", hex.EncodeToString(b), time.Now().UnixNano(), domain)
}

// encodeQuotedPrintableBody 将正文编码为 quoted-printable
func encodeQuotedPrintableBody(body string) string {
	var buf strings.Builder
	lineLen := 0
	for i := 0; i < len(body); i++ {
		b := body[i]
		var encoded string
		if b == '\r' || b == '\n' {
			if b == '\r' && i+1 < len(body) && body[i+1] == '\n' {
				buf.WriteString("\r\n")
				i++
			} else {
				buf.WriteString("\r\n")
			}
			lineLen = 0
			continue
		}
		if b == '\t' || (b >= 32 && b <= 126 && b != '=') {
			encoded = string(b)
		} else {
			encoded = fmt.Sprintf("=%02X", b)
		}
		// 软换行 (RFC 2045: 行长不超过 76)
		if lineLen+len(encoded) > 75 {
			buf.WriteString("=\r\n")
			lineLen = 0
		}
		buf.WriteString(encoded)
		lineLen += len(encoded)
	}
	return buf.String()
}

// extractEmailAddr 从 "Name <addr>" 格式提取纯邮箱地址
func extractEmailAddr(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.LastIndex(s, "<"); idx >= 0 {
		end := strings.Index(s[idx:], ">")
		if end > 0 {
			return s[idx+1 : idx+end]
		}
	}
	return s
}
