package email

// imap_client.go — IMAP 连接器接口 + go-imap/v2 实现
// Phase 3: TLS 连接 / 登录 / SELECT / UID FETCH / IDLE / NOOP

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// MailboxStatus IMAP mailbox 元信息
type MailboxStatus struct {
	UIDValidity uint32
	UIDNext     imap.UID
	Messages    uint32
}

// IMAPConnector 抽象 IMAP 操作（便于测试注入 mock）
type IMAPConnector interface {
	Connect(ctx context.Context) error
	Disconnect() error
	SelectMailbox(ctx context.Context, name string) (*MailboxStatus, error)
	FetchNewMessages(ctx context.Context, afterUID uint32, batchSize int) ([]RawEmailMessage, error)
	WaitIdle(ctx context.Context, timeout time.Duration) error
	Noop(ctx context.Context) error
	IsConnected() bool
}

// GoIMAPClient 基于 go-imap/v2 的 IMAP 连接器
type GoIMAPClient struct {
	config    *types.EmailAccountConfig
	mu        sync.Mutex
	client    *imapclient.Client
	connected bool
}

// NewGoIMAPClient 创建 IMAP 连接器
func NewGoIMAPClient(config *types.EmailAccountConfig) *GoIMAPClient {
	return &GoIMAPClient{config: config}
}

// Connect 建立 IMAP TLS 连接并登录。
// 修 L3: ctx 传播到 dial 阶段，取消时立即中断连接而非等待 30s 默认超时。
func (c *GoIMAPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected && c.client != nil {
		return nil
	}

	imapCfg := c.config.IMAP
	if imapCfg == nil {
		return fmt.Errorf("IMAP config is nil")
	}

	addr := fmt.Sprintf("%s:%d", imapCfg.Host, imapCfg.Port)
	slog.Info("email: connecting to IMAP", "addr", addr, "security", imapCfg.Security)

	tlsConfig := &tls.Config{ServerName: imapCfg.Host}
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	opts := &imapclient.Options{TLSConfig: tlsConfig}

	var client *imapclient.Client

	switch imapCfg.Security {
	case types.EmailSecurityTLS, "":
		// 使用 DialContext 传播 ctx（修 L3）
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("IMAP dial %s: %w", addr, err)
		}
		tlsCfg := tlsConfig.Clone()
		if tlsCfg.NextProtos == nil {
			tlsCfg.NextProtos = []string{"imap"}
		}
		tlsConn := tls.Client(rawConn, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return fmt.Errorf("IMAP TLS handshake %s: %w", addr, err)
		}
		client = imapclient.New(tlsConn, opts)

	case types.EmailSecuritySTARTTLS:
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("IMAP dial %s: %w", addr, err)
		}
		var startTLSErr error
		client, startTLSErr = imapclient.NewStartTLS(rawConn, opts)
		if startTLSErr != nil {
			rawConn.Close()
			return fmt.Errorf("IMAP STARTTLS %s: %w", addr, startTLSErr)
		}

	case types.EmailSecurityNone:
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("IMAP dial %s: %w", addr, err)
		}
		client = imapclient.New(rawConn, nil)

	default:
		return fmt.Errorf("unsupported security mode: %s", imapCfg.Security)
	}

	if err := client.WaitGreeting(); err != nil {
		client.Close()
		return fmt.Errorf("IMAP greeting: %w", err)
	}

	// 登录
	login := c.config.Login
	if login == "" {
		login = c.config.Address
	}
	if err := client.Login(login, c.config.Auth.Password).Wait(); err != nil {
		client.Close()
		return fmt.Errorf("IMAP login: %w", err)
	}

	c.client = client
	c.connected = true
	slog.Info("email: IMAP connected and logged in", "addr", addr)
	return nil
}

// Disconnect 断开 IMAP 连接
func (c *GoIMAPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		_ = c.client.Logout().Wait()
		_ = c.client.Close()
		c.client = nil
	}
	c.connected = false
	return nil
}

// SelectMailbox 选择邮箱文件夹
func (c *GoIMAPClient) SelectMailbox(_ context.Context, name string) (*MailboxStatus, error) {
	c.mu.Lock()
	cl := c.client
	c.mu.Unlock()

	if cl == nil {
		return nil, fmt.Errorf("not connected")
	}

	selectData, err := cl.Select(name, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("SELECT %s: %w", name, err)
	}

	return &MailboxStatus{
		UIDValidity: selectData.UIDValidity,
		UIDNext:     selectData.UIDNext,
		Messages:    selectData.NumMessages,
	}, nil
}

// FetchNewMessages 拉取 UID > afterUID 的新邮件（最多 batchSize 封）
func (c *GoIMAPClient) FetchNewMessages(ctx context.Context, afterUID uint32, batchSize int) ([]RawEmailMessage, error) {
	c.mu.Lock()
	cl := c.client
	c.mu.Unlock()

	if cl == nil {
		return nil, fmt.Errorf("not connected")
	}

	// UID SEARCH: UID > afterUID
	criteria := &imap.SearchCriteria{
		UID: []imap.UIDSet{
			{imap.UIDRange{Start: imap.UID(afterUID + 1), Stop: 0}},
		},
	}
	searchData, err := cl.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("UID SEARCH: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}

	// 限制 batch size
	if batchSize > 0 && len(uids) > batchSize {
		uids = uids[:batchSize]
	}

	// UID FETCH
	fetchOpts := &imap.FetchOptions{
		UID:        true,
		Envelope:   true,
		RFC822Size: true,
		BodySection: []*imap.FetchItemBodySection{
			{Peek: true}, // BODY.PEEK[] 不标记 \Seen
		},
	}

	msgs, err := cl.Fetch(imap.UIDSetNum(uids...), fetchOpts).Collect()
	if err != nil {
		return nil, fmt.Errorf("UID FETCH: %w", err)
	}

	result := make([]RawEmailMessage, 0, len(msgs))
	bodySection := &imap.FetchItemBodySection{Peek: true}

	for _, msg := range msgs {
		raw := RawEmailMessage{
			UID:    uint32(msg.UID),
			Size:   uint32(msg.RFC822Size),
			Header: make(map[string][]string),
		}

		// 从 Envelope 提取常用 header
		if msg.Envelope != nil {
			env := msg.Envelope
			raw.Header["Subject"] = []string{env.Subject}
			raw.Header["Message-Id"] = []string{env.MessageID}
			if !env.Date.IsZero() {
				raw.Header["Date"] = []string{env.Date.Format(time.RFC1123Z)}
			}
			if len(env.From) > 0 {
				raw.Header["From"] = addressesToStrings(env.From)
			}
			if len(env.To) > 0 {
				raw.Header["To"] = addressesToStrings(env.To)
			}
			if len(env.Cc) > 0 {
				raw.Header["Cc"] = addressesToStrings(env.Cc)
			}
			if len(env.InReplyTo) > 0 {
				raw.Header["In-Reply-To"] = env.InReplyTo
			}
		}

		// 提取完整 body
		body := msg.FindBodySection(bodySection)
		if body != nil {
			raw.Body = body
		}

		result = append(result, raw)
	}

	return result, nil
}

// WaitIdle 进入 IDLE 模式等待新邮件，超时后自动退出。
// RFC 2177: IDLE 应在 29 分钟内重启，默认 25 分钟（配置 idleRestartMinutes）。
func (c *GoIMAPClient) WaitIdle(ctx context.Context, timeout time.Duration) error {
	c.mu.Lock()
	cl := c.client
	c.mu.Unlock()

	if cl == nil {
		return fmt.Errorf("not connected")
	}

	idleCmd, err := cl.Idle()
	if err != nil {
		return fmt.Errorf("IDLE start: %w", err)
	}

	// 在后台等待 IDLE 完成
	done := make(chan error, 1)
	go func() {
		done <- idleCmd.Wait()
	}()

	// 等待超时、context 取消、或 IDLE 意外中断
	select {
	case <-time.After(timeout):
		_ = idleCmd.Close()
		return waitDoneWithTimeout(done, 30*time.Second)
	case <-ctx.Done():
		_ = idleCmd.Close()
		_ = waitDoneWithTimeout(done, 30*time.Second)
		return ctx.Err()
	case err := <-done:
		// IDLE 意外结束（连接断开等）
		return err
	}
}

// Noop 发送 NOOP 保活命令（修 F-09）
func (c *GoIMAPClient) Noop(_ context.Context) error {
	c.mu.Lock()
	cl := c.client
	c.mu.Unlock()

	if cl == nil {
		return fmt.Errorf("not connected")
	}

	return cl.Noop().Wait()
}

// IsConnected 检查连接状态
func (c *GoIMAPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && c.client != nil
}

// waitDoneWithTimeout 在限定时间内等待 done 信号（修 M4: 防止 TCP 半开导致 <-done 永久阻塞）
func waitDoneWithTimeout(done <-chan error, timeout time.Duration) error {
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		slog.Warn("email: IDLE Wait() timed out after Close(), possible TCP half-open")
		return fmt.Errorf("IDLE cleanup timeout after %v", timeout)
	}
}

// addressesToStrings 将 imap.Address 切片转为字符串
func addressesToStrings(addrs []imap.Address) []string {
	ss := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a.Name != "" {
			ss = append(ss, fmt.Sprintf("%s <%s>", a.Name, a.Addr()))
		} else {
			ss = append(ss, a.Addr())
		}
	}
	return ss
}
