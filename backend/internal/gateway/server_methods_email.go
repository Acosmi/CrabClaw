package gateway

// server_methods_email.go — email.test: 邮箱连接验证 RPC
// Phase 10: 验证 IMAP 连接 + SMTP 握手，返回诊断结果

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/channels/email"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// EmailHandlers 返回 email.* 方法处理器映射
func EmailHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"email.test": handleEmailTest,
	}
}

// handleEmailTest 邮箱连接验证。
// 参数: { accountId?: string }
// 返回: { imap: { ok, host, latencyMs, error? }, smtp: { ok, host, latencyMs, error? } }
func handleEmailTest(ctx *MethodHandlerContext) {
	accountID := readString(ctx.Params, "accountId")
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	// 从 ConfigLoader 或 Config 读取最新配置
	cfg := ctx.Context.Config
	if loader := ctx.Context.ConfigLoader; loader != nil {
		if loaded, err := loader.LoadConfig(); err == nil {
			cfg = loaded
		}
	}
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	acct := email.ResolveEmailAccount(cfg, accountID)
	if acct == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			fmt.Sprintf("email account %q not found in config", accountID)))
		return
	}

	// 深拷贝后合并默认值
	acctCfg := email.CloneEmailAccountConfig(acct.Config)
	email.ApplyProviderDefaults(acctCfg)

	if err := email.ValidateEmailAccount(acctCfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			fmt.Sprintf("email config validation failed: %v", err)))
		return
	}

	// 并行测试 IMAP 和 SMTP
	type testResult struct {
		OK        bool   `json:"ok"`
		Host      string `json:"host"`
		LatencyMs int64  `json:"latencyMs"`
		Error     string `json:"error,omitempty"`
	}

	imapCh := make(chan testResult, 1)
	smtpCh := make(chan testResult, 1)

	testCtx, cancel := context.WithTimeout(ctx.Ctx, 30*time.Second)
	defer cancel()

	// IMAP 连接测试
	go func() {
		r := testResult{}
		if acctCfg.IMAP == nil {
			r.Error = "IMAP config is nil"
			imapCh <- r
			return
		}
		r.Host = fmt.Sprintf("%s:%d", acctCfg.IMAP.Host, acctCfg.IMAP.Port)
		start := time.Now()
		err := testIMAPConnection(testCtx, acctCfg)
		r.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			r.Error = err.Error()
		} else {
			r.OK = true
		}
		imapCh <- r
	}()

	// SMTP 连接测试
	go func() {
		r := testResult{}
		if acctCfg.SMTP == nil {
			r.Error = "SMTP config is nil"
			smtpCh <- r
			return
		}
		r.Host = fmt.Sprintf("%s:%d", acctCfg.SMTP.Host, acctCfg.SMTP.Port)
		start := time.Now()
		err := testSMTPConnection(testCtx, acctCfg)
		r.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			r.Error = err.Error()
		} else {
			r.OK = true
		}
		smtpCh <- r
	}()

	imapResult := <-imapCh
	smtpResult := <-smtpCh

	allOK := imapResult.OK && smtpResult.OK

	ctx.Respond(true, map[string]interface{}{
		"ok":        allOK,
		"accountId": accountID,
		"address":   acctCfg.Address,
		"provider":  string(acctCfg.Provider),
		"imap":      imapResult,
		"smtp":      smtpResult,
	}, nil)
}

// testIMAPConnection 测试 IMAP 连接（TLS 握手 + 登录 + SELECT INBOX）
func testIMAPConnection(ctx context.Context, acctCfg *types.EmailAccountConfig) error {
	imapClient := email.NewGoIMAPClient(acctCfg)
	if err := imapClient.Connect(ctx); err != nil {
		return fmt.Errorf("IMAP connect: %w", err)
	}
	defer imapClient.Disconnect()

	// SELECT INBOX 验证
	_, err := imapClient.SelectMailbox(ctx, "INBOX")
	if err != nil {
		return fmt.Errorf("IMAP SELECT INBOX: %w", err)
	}

	return nil
}

// testSMTPConnection 测试 SMTP 连接（TLS 握手 + AUTH LOGIN + QUIT）
func testSMTPConnection(ctx context.Context, acctCfg *types.EmailAccountConfig) error {
	smtpCfg := acctCfg.SMTP
	if smtpCfg == nil {
		return fmt.Errorf("SMTP config is nil")
	}

	addr := fmt.Sprintf("%s:%d", smtpCfg.Host, smtpCfg.Port)
	login := acctCfg.Login
	if login == "" {
		login = acctCfg.Address
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	tlsConfig := &tls.Config{ServerName: smtpCfg.Host}

	var client *smtp.Client

	switch smtpCfg.Security {
	case types.EmailSecurityTLS, "":
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("SMTP dial %s: %w", addr, err)
		}
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return fmt.Errorf("SMTP TLS handshake: %w", err)
		}
		c, err := smtp.NewClient(tlsConn, smtpCfg.Host)
		if err != nil {
			tlsConn.Close()
			return fmt.Errorf("SMTP client: %w", err)
		}
		client = c

	case types.EmailSecuritySTARTTLS:
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("SMTP dial %s: %w", addr, err)
		}
		c, err := smtp.NewClient(rawConn, smtpCfg.Host)
		if err != nil {
			rawConn.Close()
			return fmt.Errorf("SMTP client: %w", err)
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			c.Close()
			return fmt.Errorf("SMTP STARTTLS: %w", err)
		}
		client = c

	case types.EmailSecurityNone:
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("SMTP dial %s: %w", addr, err)
		}
		c, err := smtp.NewClient(rawConn, smtpCfg.Host)
		if err != nil {
			rawConn.Close()
			return fmt.Errorf("SMTP client: %w", err)
		}
		client = c

	default:
		return fmt.Errorf("unsupported SMTP security: %s", smtpCfg.Security)
	}

	defer client.Close()

	// AUTH LOGIN 验证 — host 使用 SMTP 服务器名（smtp.PlainAuth 用于 TLS 身份验证）
	auth := smtp.PlainAuth("", login, acctCfg.Auth.Password, acctCfg.SMTP.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP AUTH: %w", err)
	}

	return client.Quit()
}
