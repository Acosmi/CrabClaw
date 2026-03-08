package email

// plugin.go — Email Channel 插件
// 实现 channels.Plugin + channels.ConfigUpdater + channels.MessageSender 接口

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// EmailPlugin Email 频道插件
type EmailPlugin struct {
	config *types.OpenAcosmiConfig
	mu     sync.Mutex

	// runners 每账号运行时（accountID → runner）
	runners map[string]*AccountRunner

	// rateLimiters 每账号出站频率限制器（Phase 8）
	rateLimiters map[string]*SendRateLimiter

	// threadStores 每账号线程上下文存储（修 F-05: 缓存实例，不再每次 SendMessage 新建）
	threadStores map[string]*ThreadContextStore

	// DispatchMultimodalFunc 多模态消息分发回调 — 由 gateway 注入
	// 收到邮件后通过此回调路由到 autoreply 管线
	DispatchMultimodalFunc channels.DispatchMultimodalFunc

	// StoreRoot 状态存储根目录（如 ~/.openacosmi/store）
	// Phase 3: 每账号状态存储在 <StoreRoot>/email/<accountID>/state.json
	StoreRoot string
}

// NewEmailPlugin 创建 Email 插件
func NewEmailPlugin(cfg *types.OpenAcosmiConfig) *EmailPlugin {
	return &EmailPlugin{
		config:       cfg,
		runners:      make(map[string]*AccountRunner),
		rateLimiters: make(map[string]*SendRateLimiter),
		threadStores: make(map[string]*ThreadContextStore),
	}
}

// ID 返回频道标识
func (p *EmailPlugin) ID() channels.ChannelID {
	return channels.ChannelEmail
}

// UpdateConfig 实现 channels.ConfigUpdater 接口。
// Phase 9: 按账号粒度 diff — 检测新增/移除/变更的账号，局部重启受影响 runner。
func (p *EmailPlugin) UpdateConfig(cfg interface{}) {
	c, ok := cfg.(*types.OpenAcosmiConfig)
	if !ok {
		return
	}

	p.mu.Lock()
	oldConfig := p.config
	p.config = c
	p.mu.Unlock()

	// 提取新旧账号集合
	oldAccounts := resolveAccountIDs(oldConfig)
	newAccounts := resolveAccountIDs(c)

	// 移除已删除的账号
	for acctID := range oldAccounts {
		if _, exists := newAccounts[acctID]; !exists {
			slog.Info("email: hot-reload removing account", "account", acctID)
			_ = p.Stop(acctID)
		}
	}

	// 新增或变更的账号 — 重新启动
	for acctID := range newAccounts {
		if _, existed := oldAccounts[acctID]; !existed {
			// 新增账号
			slog.Info("email: hot-reload adding account", "account", acctID)
			if err := p.Start(acctID); err != nil {
				slog.Warn("email: hot-reload start failed", "account", acctID, "error", err)
			}
		} else if accountConfigChanged(oldConfig, c, acctID) {
			// 凭据或参数变更 — 重启 runner
			slog.Info("email: hot-reload restarting account (config changed)", "account", acctID)
			_ = p.Stop(acctID)
			if err := p.Start(acctID); err != nil {
				slog.Warn("email: hot-reload restart failed", "account", acctID, "error", err)
			}
		}
	}
}

// Start 启动指定邮箱账号
func (p *EmailPlugin) Start(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	acct := ResolveEmailAccount(p.config, accountID)
	if acct == nil {
		return fmt.Errorf("email account %q not found in config", accountID)
	}

	if !channels.IsAccountEnabled(acct.Config.Enabled) {
		slog.Info("email account disabled, skipping", "account", accountID)
		return nil
	}

	// 深拷贝后合并 provider 预置默认值（修 M1: 不修改原始 config 指针）
	acctCfg := CloneEmailAccountConfig(acct.Config)
	ApplyProviderDefaults(acctCfg)

	// 校验配置
	if err := ValidateEmailAccount(acctCfg); err != nil {
		return fmt.Errorf("email config validation: %w", err)
	}

	// 如已有 runner，先停止
	if existing, ok := p.runners[accountID]; ok {
		existing.Stop()
		delete(p.runners, accountID)
	}

	// 创建并启动 runner（使用深拷贝后的配置）
	runner := NewAccountRunner(accountID, acctCfg)

	// 注入状态存储（Phase 3）
	if p.StoreRoot != "" {
		runner.SetStateStore(NewStateStore(p.StoreRoot, accountID))
	}

	// 构造解析限制
	parseLimits := buildParseLimits(acctCfg)

	// 创建去重缓存（7 天 TTL，修 F-06）
	var dedup *DedupCache
	if p.StoreRoot != "" {
		dedup = NewDedupCache(p.StoreRoot, accountID, 7*24*time.Hour)
	}

	// Phase 8: 入站过滤配置 + 出站频率限制
	filterCfg := NewFilterConfigFromAccount(acctCfg)
	var rateLimiter *SendRateLimiter
	if acctCfg.Limits != nil && (acctCfg.Limits.MaxSendPerHour > 0 || acctCfg.Limits.MaxSendPerDay > 0) {
		rateLimiter = NewSendRateLimiter(acctCfg.Limits.MaxSendPerHour, acctCfg.Limits.MaxSendPerDay)
	}
	p.rateLimiters[accountID] = rateLimiter

	// 缓存线程上下文存储（修 F-05）
	if p.StoreRoot != "" {
		p.threadStores[accountID] = NewThreadContextStore(p.StoreRoot, accountID)
	}

	// 获取 UIDVALIDITY（由 runner 运行时传入，此处用闭包捕获 runner 引用）
	providerStr := string(acctCfg.Provider)
	dispatchFn := p.DispatchMultimodalFunc

	// 注入新邮件回调 — Phase 5+8: 过滤 → threading → inbound bridge → DispatchMultimodalFunc
	runner.SetOnNewMail(func(acctID string, rawMsgs []RawEmailMessage) {
		slog.Info("email: new mail received", "account", acctID, "count", len(rawMsgs))

		// Phase 8: 入站过滤（在 MIME 解析之前，降低无效邮件的解析开销）
		passed, filteredCount := FilterInbound(rawMsgs, filterCfg)
		if filteredCount > 0 {
			slog.Info("email: inbound messages filtered",
				"account", acctID, "filtered", filteredCount, "passed", len(passed))
		}
		if len(passed) == 0 {
			return
		}

		// 从 runner 的 state store 获取 UIDVALIDITY
		var uidValidity uint32
		if runner.stateStore != nil {
			if st, err := runner.stateStore.Load(); err == nil && st != nil {
				uidValidity = st.UIDValidity
			}
		}
		ProcessInbound(acctID, providerStr, "INBOX", uidValidity, passed, parseLimits, dedup, dispatchFn)
	})

	if err := runner.Start(); err != nil {
		return fmt.Errorf("email runner start: %w", err)
	}
	p.runners[accountID] = runner

	slog.Info("email channel started",
		"account", accountID,
		"provider", acctCfg.Provider,
		"address", acctCfg.Address,
	)
	return nil
}

// Stop 停止指定邮箱账号
func (p *EmailPlugin) Stop(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	runner, ok := p.runners[accountID]
	if !ok {
		return nil // 未运行
	}

	runner.Stop()
	delete(p.runners, accountID)
	delete(p.rateLimiters, accountID)
	delete(p.threadStores, accountID)

	slog.Info("email channel stopped", "account", accountID)
	return nil
}

// SendMessage 实现 channels.MessageSender 接口。
// Phase 6 将填充完整 SMTP 发送逻辑。当前为骨架。
func (p *EmailPlugin) SendMessage(params channels.OutboundSendParams) (*channels.OutboundSendResult, error) {
	accountID := params.AccountID
	if accountID == "" {
		// 使用 defaultAccount
		accountID = p.resolveDefaultAccountID()
	}

	p.mu.Lock()
	runner, ok := p.runners[accountID]
	rl := p.rateLimiters[accountID]  // 在持锁区间内读取，避免 data race
	tcs := p.threadStores[accountID] // 缓存的线程上下文存储（修 F-05）
	p.mu.Unlock()

	if !ok || runner == nil {
		return nil, channels.NewSendError(channels.ChannelEmail, channels.SendErrUnavailable,
			fmt.Sprintf("email account %s not running", accountID)).
			WithOperation("send.init").
			WithRetryable(true)
	}

	if strings.TrimSpace(params.To) == "" {
		return nil, channels.NewSendError(channels.ChannelEmail, channels.SendErrInvalidRequest,
			"email: recipient (To) is required").
			WithOperation("send.validate")
	}

	if strings.TrimSpace(params.Text) == "" {
		return nil, channels.NewSendError(channels.ChannelEmail, channels.SendErrInvalidRequest,
			"email: message body is empty").
			WithOperation("send.validate")
	}

	// Phase 8: 出站频率限制检查
	if rl != nil && !rl.Allow() {
		hourly, daily := rl.Stats()
		slog.Warn("email: send rate limit exceeded",
			"account", accountID, "hourly", hourly, "daily", daily)
		return nil, channels.NewSendError(channels.ChannelEmail, channels.SendErrRateLimited,
			fmt.Sprintf("email send rate limit exceeded (hourly=%d, daily=%d)", hourly, daily)).
			WithOperation("send.ratelimit")
	}

	// Phase 6: SMTP 发送（深拷贝后合并默认值，修 M1）
	cfg := CloneEmailAccountConfig(runner.getConfig())
	ApplyProviderDefaults(cfg)

	sender := NewSMTPSender(cfg)

	// 构造发送参数
	toAddrs := strings.Split(params.To, ",")
	for i := range toAddrs {
		toAddrs[i] = strings.TrimSpace(toAddrs[i])
	}

	sendParams := SendParams{
		To:      toAddrs,
		Subject: params.Subject,
		Body:    params.Text,
	}

	// Cc
	if params.Cc != "" {
		ccAddrs := strings.Split(params.Cc, ",")
		for i := range ccAddrs {
			ccAddrs[i] = strings.TrimSpace(ccAddrs[i])
		}
		sendParams.Cc = ccAddrs
	}

	// 线程头恢复（修 D-01）: 从 ThreadContextStore 恢复 In-Reply-To / References
	if params.SessionKey != "" && tcs != nil {
		if tc, err := tcs.Load(params.SessionKey); err == nil && tc != nil {
			sendParams.InReplyTo = tc.LastMessageID
			sendParams.References = tc.References
			if sendParams.Subject == "" && tc.Subject != "" {
				sendParams.Subject = "Re: " + tc.Subject
			}
		}
	}

	result, err := sender.Send(sendParams)
	if err != nil {
		return nil, channels.NewSendError(channels.ChannelEmail, channels.SendErrUpstream,
			fmt.Sprintf("email send failed: %v", err)).
			WithOperation("send.smtp").
			WithRetryable(true)
	}

	// Phase 8: 记录发送（频率限制滑动窗口）
	if rl != nil {
		rl.Record()
	}

	// 更新线程上下文（发送后持久化）
	if params.SessionKey != "" && tcs != nil {
		refs := sendParams.References
		refs = append(refs, result.MessageID)
		tc := &ThreadContext{
			LastMessageID: result.MessageID,
			References:    refs,
			Subject:       sendParams.Subject,
		}
		if err := tcs.Save(params.SessionKey, tc); err != nil {
			slog.Warn("email: failed to save thread context",
				"account", accountID, "sessionKey", params.SessionKey, "error", err)
		}
	}

	slog.Info("email: message sent",
		"account", accountID,
		"messageID", result.MessageID,
		"recipients", result.Recipients,
	)

	return &channels.OutboundSendResult{
		MessageID: result.MessageID,
		Timestamp: result.SentAt.Unix(),
	}, nil
}

// GetRunner 获取指定账号的 Runner
func (p *EmailPlugin) GetRunner(accountID string) *AccountRunner {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runners[accountID]
}

// RunnerStates 返回所有 runner 的状态快照（用于健康检查 / 可观测）
func (p *EmailPlugin) RunnerStates() map[string]RunnerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	states := make(map[string]RunnerState, len(p.runners))
	for id, r := range p.runners {
		states[id] = r.State()
	}
	return states
}

// resolveDefaultAccountID 解析默认账号 ID
func (p *EmailPlugin) resolveDefaultAccountID() string {
	if p.config != nil && p.config.Channels != nil &&
		p.config.Channels.Email != nil && p.config.Channels.Email.DefaultAccount != "" {
		return p.config.Channels.Email.DefaultAccount
	}
	return channels.DefaultAccountID
}

// StartAllAccounts 启动所有已配置且启用的账号（供 gateway 调用）
func (p *EmailPlugin) StartAllAccounts() {
	if p.config == nil || p.config.Channels == nil || p.config.Channels.Email == nil {
		return
	}
	emailCfg := p.config.Channels.Email
	if emailCfg.Accounts == nil {
		return
	}
	for accountID := range emailCfg.Accounts {
		if err := p.Start(accountID); err != nil {
			slog.Warn("email: failed to start account", "account", accountID, "error", err)
		}
	}
}

// resolveAccountIDs 提取配置中所有邮箱账号 ID 集合
func resolveAccountIDs(cfg *types.OpenAcosmiConfig) map[string]struct{} {
	result := make(map[string]struct{})
	if cfg == nil || cfg.Channels == nil || cfg.Channels.Email == nil || cfg.Channels.Email.Accounts == nil {
		return result
	}
	for id := range cfg.Channels.Email.Accounts {
		result[id] = struct{}{}
	}
	return result
}

// accountConfigChanged 检测指定账号的配置是否变更（简化比较：地址+密码+IMAP/SMTP 端口+provider）
func accountConfigChanged(oldCfg, newCfg *types.OpenAcosmiConfig, accountID string) bool {
	oldAcct := ResolveEmailAccount(oldCfg, accountID)
	newAcct := ResolveEmailAccount(newCfg, accountID)
	if oldAcct == nil || newAcct == nil {
		return true // 一侧不存在视为变更
	}
	o, n := oldAcct.Config, newAcct.Config
	if o.Address != n.Address || o.Provider != n.Provider {
		return true
	}
	// Auth 变更
	if o.Auth.Mode != n.Auth.Mode || o.Auth.Password != n.Auth.Password {
		return true
	}
	// IMAP 变更 — 仅两侧都显式配置时比较（nil 表示依赖 provider 默认值）
	if o.IMAP != nil && n.IMAP != nil {
		if o.IMAP.Host != n.IMAP.Host || o.IMAP.Port != n.IMAP.Port {
			return true
		}
	}
	// SMTP 变更
	if o.SMTP != nil && n.SMTP != nil {
		if o.SMTP.Host != n.SMTP.Host || o.SMTP.Port != n.SMTP.Port {
			return true
		}
	}
	// Enabled 变更
	if (o.Enabled == nil) != (n.Enabled == nil) {
		return true
	}
	if o.Enabled != nil && n.Enabled != nil && *o.Enabled != *n.Enabled {
		return true
	}
	return false
}

// AccountHealth 账号健康状态快照（Phase 9 可观测）
type AccountHealth struct {
	AccountID           string `json:"accountId"`
	State               string `json:"state"`
	LastSuccessAt       int64  `json:"lastSuccessAt,omitempty"` // unix ms, 0=never
	ConsecutiveFailures int    `json:"consecutiveFailures"`
	Provider            string `json:"provider,omitempty"`
	Address             string `json:"address,omitempty"`
}

// HealthSnapshot 返回所有账号的健康快照（供 channels.status RPC 调用）
func (p *EmailPlugin) HealthSnapshot() []AccountHealth {
	p.mu.Lock()
	defer p.mu.Unlock()

	var result []AccountHealth
	for id, r := range p.runners {
		h := AccountHealth{
			AccountID:           id,
			State:               string(r.State()),
			ConsecutiveFailures: r.ConsecutiveFailures(),
		}
		lastSuccess := r.LastSuccessAt()
		if !lastSuccess.IsZero() {
			h.LastSuccessAt = lastSuccess.UnixMilli()
		}
		// 从配置中提取 provider/address
		acct := ResolveEmailAccount(p.config, id)
		if acct != nil {
			h.Provider = string(acct.Config.Provider)
			h.Address = acct.Config.Address
		}
		result = append(result, h)
	}
	return result
}

// buildParseLimits 从账号配置构造 MIME 解析限制
func buildParseLimits(cfg *types.EmailAccountConfig) ParseLimits {
	limits := DefaultParseLimits()
	if cfg.Limits != nil {
		if cfg.Limits.MaxAttachmentBytes > 0 {
			limits.MaxAttachmentBytes = cfg.Limits.MaxAttachmentBytes
		}
		if cfg.Limits.MaxAttachments > 0 {
			limits.MaxAttachments = cfg.Limits.MaxAttachments
		}
		if len(cfg.Limits.AllowAttachmentMimePrefixes) > 0 {
			limits.AllowAttachmentMimePrefixes = cfg.Limits.AllowAttachmentMimePrefixes
		}
	}
	if cfg.Behavior != nil && cfg.Behavior.HTMLMode != "" {
		limits.HTMLMode = cfg.Behavior.HTMLMode
	}
	return limits
}
