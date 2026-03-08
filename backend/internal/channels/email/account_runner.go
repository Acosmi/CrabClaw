package email

// account_runner.go — 单邮箱账号生命周期状态机
// 每个 runner 持有独立 context，Stop() 时 cancel（修 F-09）
// Phase 3: 完整 IMAP Poll/IDLE 循环 + 指数退避 + UIDVALIDITY + 状态持久化

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// RunnerState 账号 Runner 状态
type RunnerState string

const (
	RunnerStateInit       RunnerState = "init"       // 初始化
	RunnerStateConnecting RunnerState = "connecting" // 正在连接
	RunnerStateSyncing    RunnerState = "syncing"    // 正在同步
	RunnerStateIdle       RunnerState = "idle"       // IDLE 模式等待
	RunnerStatePolling    RunnerState = "polling"    // Poll 模式等待
	RunnerStateDegraded   RunnerState = "degraded"   // 降级（IDLE 失败回退 Poll / 授权失败等）
	RunnerStateBackoff    RunnerState = "backoff"    // 退避中
	RunnerStateStopped    RunnerState = "stopped"    // 已停止
	RunnerStateError      RunnerState = "error"      // 不可恢复错误
)

// AccountRunner 单个邮箱账号的运行时控制器。
// 管理 IMAP 收件循环和运行状态。SMTP 发件按需连接，不在 runner 中持久化。
type AccountRunner struct {
	mu        sync.Mutex
	accountID string
	config    *types.EmailAccountConfig
	state     RunnerState

	ctx    context.Context
	cancel context.CancelFunc

	// 运行时统计
	lastSuccessAt       time.Time
	consecutiveFailures int

	// onNewMail 新邮件回调 — 由 Plugin 注入，将邮件路由到 threading/inbound bridge
	onNewMail func(accountID string, rawMessages []RawEmailMessage)

	// Phase 3: IMAP 连接器 + 状态存储（可注入 mock）
	imapConnector IMAPConnector
	stateStore    *StateStore
}

// RawEmailMessage IMAP 拉取的原始邮件
type RawEmailMessage struct {
	UID    uint32
	Header map[string][]string
	Body   []byte
	Size   uint32
}

// NewAccountRunner 创建账号 Runner
func NewAccountRunner(accountID string, acctCfg *types.EmailAccountConfig) *AccountRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountRunner{
		accountID: accountID,
		config:    acctCfg,
		state:     RunnerStateInit,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// SetOnNewMail 设置新邮件回调
func (r *AccountRunner) SetOnNewMail(fn func(accountID string, rawMessages []RawEmailMessage)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onNewMail = fn
}

// SetIMAPConnector 注入 IMAP 连接器（测试用 mock，生产用 GoIMAPClient）
func (r *AccountRunner) SetIMAPConnector(c IMAPConnector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.imapConnector = c
}

// SetStateStore 注入状态存储
func (r *AccountRunner) SetStateStore(s *StateStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stateStore = s
}

// Start 启动账号 Runner（后台 goroutine）。
func (r *AccountRunner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == RunnerStatePolling || r.state == RunnerStateIdle || r.state == RunnerStateSyncing {
		return nil // 已运行
	}

	r.state = RunnerStateConnecting
	slog.Info("email: account runner starting", "account", r.accountID)

	go r.runLoop()
	return nil
}

// Stop 停止账号 Runner
func (r *AccountRunner) Stop() {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = RunnerStateStopped
	slog.Info("email: account runner stopped", "account", r.accountID)
}

// State 返回当前状态
func (r *AccountRunner) State() RunnerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// AccountID 返回账号 ID
func (r *AccountRunner) AccountID() string {
	return r.accountID
}

// LastSuccessAt 返回最后成功时间
func (r *AccountRunner) LastSuccessAt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSuccessAt
}

// ConsecutiveFailures 返回连续失败次数
func (r *AccountRunner) ConsecutiveFailures() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.consecutiveFailures
}

// UpdateConfig 热更新配置（Stop + 重新 Start 前调用）
func (r *AccountRunner) UpdateConfig(acctCfg *types.EmailAccountConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = acctCfg
}

// setState 内部状态切换
func (r *AccountRunner) setState(s RunnerState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = s
}

// recordSuccess 记录成功
func (r *AccountRunner) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastSuccessAt = time.Now()
	r.consecutiveFailures = 0
}

// recordFailure 记录失败
func (r *AccountRunner) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consecutiveFailures++
}

// getConfig 线程安全地读取配置副本的关键字段
func (r *AccountRunner) getConfig() *types.EmailAccountConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.config
}

// getIMAPConnector 获取 IMAP 连接器（如未注入则创建默认）
func (r *AccountRunner) getIMAPConnector() IMAPConnector {
	r.mu.Lock()
	c := r.imapConnector
	r.mu.Unlock()
	if c != nil {
		return c
	}
	return NewGoIMAPClient(r.getConfig())
}

// runLoop 主运行循环 — IMAP 连接 + Poll/IDLE + 退避重连
func (r *AccountRunner) runLoop() {
	defer r.setState(RunnerStateStopped)

	// 加载持久化状态
	var state *AccountState
	if r.stateStore != nil {
		var err error
		state, err = r.stateStore.Load()
		if err != nil {
			slog.Warn("email: failed to load state, starting fresh",
				"account", r.accountID, "error", err)
		}
	}
	if state == nil {
		state = &AccountState{}
	}

	failureCount := 0

	for {
		if r.ctx.Err() != nil {
			return
		}

		r.setState(RunnerStateConnecting)

		imapClient := r.getIMAPConnector()

		if err := imapClient.Connect(r.ctx); err != nil {
			r.recordFailure()
			failureCount++
			state.LastErrorAt = time.Now()
			state.ConsecutiveFailures = failureCount
			r.saveState(state)
			slog.Warn("email: IMAP connect failed",
				"account", r.accountID, "error", err, "failures", failureCount)
			r.setState(RunnerStateBackoff)
			if !r.sleepBackoff(failureCount) {
				return
			}
			continue
		}

		// 选择 mailbox
		cfg := r.getConfig()
		mailboxes := []string{"INBOX"}
		if cfg.IMAP != nil && len(cfg.IMAP.Mailboxes) > 0 {
			mailboxes = cfg.IMAP.Mailboxes
		}

		mboxStatus, err := imapClient.SelectMailbox(r.ctx, mailboxes[0])
		if err != nil {
			_ = imapClient.Disconnect()
			r.recordFailure()
			failureCount++
			state.LastErrorAt = time.Now()
			r.saveState(state)
			slog.Warn("email: SELECT mailbox failed",
				"account", r.accountID, "mailbox", mailboxes[0], "error", err)
			r.setState(RunnerStateBackoff)
			if !r.sleepBackoff(failureCount) {
				return
			}
			continue
		}

		// UIDVALIDITY 变化检测 → degraded + 重建游标
		if state.UIDValidity != 0 && state.UIDValidity != mboxStatus.UIDValidity {
			slog.Warn("email: UIDVALIDITY changed, resetting cursor",
				"account", r.accountID,
				"old", state.UIDValidity,
				"new", mboxStatus.UIDValidity)
			state.LastSeenUID = 0
			r.setState(RunnerStateDegraded)
		}
		state.UIDValidity = mboxStatus.UIDValidity

		// 首次同步（D-04）: bootstrapMode=latest → 取当前最新 UID
		if state.LastSeenUID == 0 {
			if mboxStatus.UIDNext > 1 {
				state.LastSeenUID = uint32(mboxStatus.UIDNext) - 1
			}
			slog.Info("email: bootstrap mode, starting from latest UID",
				"account", r.accountID,
				"lastSeenUID", state.LastSeenUID,
				"uidNext", mboxStatus.UIDNext)
			r.saveState(state)
		}

		failureCount = 0

		// 进入 Poll/IDLE 收件循环
		err = r.runMailLoop(imapClient, state)
		_ = imapClient.Disconnect()

		if r.ctx.Err() != nil {
			r.saveState(state)
			return
		}

		if err != nil {
			r.recordFailure()
			failureCount++
			state.LastErrorAt = time.Now()
			state.ConsecutiveFailures = failureCount
			r.saveState(state)
			slog.Warn("email: mail loop error, reconnecting",
				"account", r.accountID, "error", err, "failures", failureCount)
			r.setState(RunnerStateBackoff)
			if !r.sleepBackoff(failureCount) {
				return
			}
		}
	}
}

// runMailLoop 收件主循环（Poll 或 IDLE 模式）
func (r *AccountRunner) runMailLoop(client IMAPConnector, state *AccountState) error {
	cfg := r.getConfig()

	// 解析配置
	configMode := types.EmailIMAPModeAuto
	pollInterval := 60 * time.Second
	idleTimeout := 25 * time.Minute
	batchSize := 20

	if cfg.IMAP != nil {
		if cfg.IMAP.Mode != "" {
			configMode = cfg.IMAP.Mode
		}
		if cfg.IMAP.PollIntervalSeconds > 0 {
			pollInterval = time.Duration(cfg.IMAP.PollIntervalSeconds) * time.Second
		}
		if cfg.IMAP.IdleRestartMinutes > 0 {
			idleTimeout = time.Duration(cfg.IMAP.IdleRestartMinutes) * time.Minute
		}
		if cfg.IMAP.FetchBatchSize > 0 {
			batchSize = cfg.IMAP.FetchBatchSize
		}
	}

	useIdle := configMode == types.EmailIMAPModeAuto || configMode == types.EmailIMAPModeIdle
	idleDegraded := false // IDLE 失败后降级标记

	for {
		if r.ctx.Err() != nil {
			return nil
		}

		// 拉取新邮件
		r.setState(RunnerStateSyncing)
		msgs, err := client.FetchNewMessages(r.ctx, state.LastSeenUID, batchSize)
		if err != nil {
			return fmt.Errorf("fetch new messages: %w", err)
		}

		if len(msgs) > 0 {
			// 更新 lastSeenUID
			for _, msg := range msgs {
				if msg.UID > state.LastSeenUID {
					state.LastSeenUID = msg.UID
				}
			}

			// 投递到回调
			r.mu.Lock()
			fn := r.onNewMail
			r.mu.Unlock()
			if fn != nil {
				fn(r.accountID, msgs)
			}

			r.recordSuccess()
			state.LastSuccessAt = time.Now()
			state.ConsecutiveFailures = 0
			r.saveState(state)

			// 如果拉满 batch，可能还有更多
			if len(msgs) >= batchSize {
				continue
			}
		}

		// 等待新邮件
		if useIdle && !idleDegraded {
			r.setState(RunnerStateIdle)
			state.CurrentMode = string(types.EmailIMAPModeIdle)
			state.LastIdleResetAt = time.Now()

			// IDLE 等待（RFC 2177: 25 分钟后重启）
			err := client.WaitIdle(r.ctx, idleTimeout)
			if err != nil {
				if r.ctx.Err() != nil {
					return nil
				}
				if configMode == types.EmailIMAPModeAuto {
					// 自动降级到 Poll
					slog.Warn("email: IDLE failed, degrading to poll",
						"account", r.accountID, "error", err)
					idleDegraded = true
					r.setState(RunnerStateDegraded)
					state.CurrentMode = string(types.EmailIMAPModePoll)
					continue
				}
				return fmt.Errorf("IDLE: %w", err)
			}
		} else {
			r.setState(RunnerStatePolling)
			state.CurrentMode = string(types.EmailIMAPModePoll)

			// NOOP 保活心跳（修 F-09）
			if err := client.Noop(r.ctx); err != nil {
				return fmt.Errorf("NOOP: %w", err)
			}

			// 等待 poll 间隔
			select {
			case <-time.After(pollInterval):
			case <-r.ctx.Done():
				return nil
			}
		}
	}
}

// sleepBackoff 指数退避等待，返回 false 表示 context 已取消
func (r *AccountRunner) sleepBackoff(failures int) bool {
	d := backoffDuration(failures)
	slog.Info("email: backing off", "account", r.accountID, "duration", d, "failures", failures)
	select {
	case <-time.After(d):
		return true
	case <-r.ctx.Done():
		return false
	}
}

// backoffDuration 计算指数退避时间
func backoffDuration(failures int) time.Duration {
	const (
		base = 5 * time.Second
		max  = 5 * time.Minute
	)
	shift := failures
	if shift > 6 {
		shift = 6
	}
	d := base * (1 << shift)
	if d > max {
		d = max
	}
	return d
}

// saveState 保存状态（忽略错误，仅日志警告）
func (r *AccountRunner) saveState(state *AccountState) {
	if r.stateStore == nil {
		return
	}
	if err := r.stateStore.Save(state); err != nil {
		slog.Warn("email: failed to save state",
			"account", r.accountID, "error", err)
	}
}
