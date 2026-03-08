package channels

import (
	"fmt"
	"log/slog"
	"sync"
)

// ---------- 频道类型 ----------

// ChannelID 频道标识。
type ChannelID string

const (
	ChannelTelegram   ChannelID = "telegram"
	ChannelWhatsApp   ChannelID = "whatsapp"
	ChannelDiscord    ChannelID = "discord"
	ChannelSlack      ChannelID = "slack"
	ChannelSignal     ChannelID = "signal"
	ChannelIMessage   ChannelID = "imessage"
	ChannelGoogleChat ChannelID = "googlechat"
	ChannelMSTeams    ChannelID = "msteams"
	ChannelWeb        ChannelID = "web"
	ChannelWebchat    ChannelID = "webchat" // 向后兼容
	ChannelFeishu     ChannelID = "feishu"
	ChannelDingTalk   ChannelID = "dingtalk"
	ChannelWeCom      ChannelID = "wecom"
	ChannelEmail      ChannelID = "email"
)

const DefaultAccountID = "default"

// AccountSnapshot 频道账户快照。
type AccountSnapshot struct {
	AccountID string `json:"accountId"`
	Status    string `json:"status,omitempty"` // "running" | "stopped" | "error"
	LoggedIn  bool   `json:"loggedIn,omitempty"`
	Error     string `json:"error,omitempty"`
}

// RuntimeSnapshot 频道运行时综合快照。
type RuntimeSnapshot struct {
	Channels map[ChannelID]*AccountSnapshot            `json:"channels"`
	Accounts map[ChannelID]map[string]*AccountSnapshot `json:"channelAccounts"`
}

// ---------- 频道插件接口 ----------

// Plugin 频道插件接口。
type Plugin interface {
	ID() ChannelID
	Start(accountID string) error
	Stop(accountID string) error
}

// ConfigUpdater 可选接口：插件实现后可支持热重载新凭证，无需全量网关重启。
// 通过类型断言 plugin.(ConfigUpdater) 检查是否支持。
// cfg 实际类型由各插件自行断言（通常为 *types.OpenAcosmiConfig）。
type ConfigUpdater interface {
	UpdateConfig(cfg interface{})
}

// ---------- 频道管理器 ----------

// Manager 管理频道生命周期。
type Manager struct {
	mu        sync.Mutex
	plugins   map[ChannelID]Plugin
	running   map[string]struct{} // "channelID:accountID" → 运行中
	snapshots map[string]*AccountSnapshot
}

// NewManager 创建频道管理器。
func NewManager() *Manager {
	return &Manager{
		plugins:   make(map[ChannelID]Plugin),
		running:   make(map[string]struct{}),
		snapshots: make(map[string]*AccountSnapshot),
	}
}

// RegisterPlugin 注册频道插件。
func (m *Manager) RegisterPlugin(plugin Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[plugin.ID()] = plugin
}

func runtimeKey(channelID ChannelID, accountID string) string {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	return fmt.Sprintf("%s:%s", channelID, accountID)
}

// HasPlugin 检查指定频道插件是否已注册。
// 用于区分首次配置（插件未注册，需全量重启）与热重载（插件已注册，可 stop+start）。
func (m *Manager) HasPlugin(channelID ChannelID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.plugins[channelID]
	return ok
}

// ReloadChannel 以新配置热重载指定频道：先更新插件配置，再 Stop+Start。
// 若插件实现了 ConfigUpdater 接口，则先调用 UpdateConfig(newCfg) 注入新凭证；
// 否则仅执行 Stop+Start（凭证不变，适用于连接断线重连场景）。
// Stop 失败时记录日志并继续 Start，避免因已停止状态导致重载中断。
func (m *Manager) ReloadChannel(channelID ChannelID, newCfg interface{}, accountID string) error {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	m.mu.Lock()
	plugin, ok := m.plugins[channelID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("ReloadChannel: channel %s not registered", channelID)
	}
	if updater, ok := plugin.(ConfigUpdater); ok {
		updater.UpdateConfig(newCfg)
	}
	if err := m.StopChannel(channelID, accountID); err != nil {
		slog.Warn("ReloadChannel: stop failed, proceeding with start",
			"channel", channelID, "account", accountID, "error", err)
	}
	return m.StartChannel(channelID, accountID)
}

// StartChannel 启动频道。
func (m *Manager) StartChannel(channelID ChannelID, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if accountID == "" {
		accountID = DefaultAccountID
	}
	key := runtimeKey(channelID, accountID)
	if _, ok := m.running[key]; ok {
		return nil // 已运行
	}
	plugin, ok := m.plugins[channelID]
	if !ok {
		return fmt.Errorf("unknown channel: %s", channelID)
	}
	if err := plugin.Start(accountID); err != nil {
		m.snapshots[key] = &AccountSnapshot{AccountID: accountID, Status: "error", Error: err.Error()}
		return err
	}
	m.running[key] = struct{}{}
	m.snapshots[key] = &AccountSnapshot{AccountID: accountID, Status: "running"}
	return nil
}

// StopChannel 停止频道。
func (m *Manager) StopChannel(channelID ChannelID, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if accountID == "" {
		accountID = DefaultAccountID
	}
	key := runtimeKey(channelID, accountID)
	if _, ok := m.running[key]; !ok {
		return nil // 未运行
	}
	plugin, ok := m.plugins[channelID]
	if !ok {
		return fmt.Errorf("unknown channel: %s", channelID)
	}
	err := plugin.Stop(accountID)
	delete(m.running, key)
	m.snapshots[key] = &AccountSnapshot{AccountID: accountID, Status: "stopped"}
	return err
}

// GetSnapshot 获取频道运行时快照。
func (m *Manager) GetSnapshot() *RuntimeSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := &RuntimeSnapshot{
		Channels: make(map[ChannelID]*AccountSnapshot),
		Accounts: make(map[ChannelID]map[string]*AccountSnapshot),
	}
	for key, s := range m.snapshots {
		// 解析 "channelID:accountID"
		var chID ChannelID
		var accID string
		for i := 0; i < len(key); i++ {
			if key[i] == ':' {
				chID = ChannelID(key[:i])
				accID = key[i+1:]
				break
			}
		}
		if chID == "" {
			continue
		}
		if accID == DefaultAccountID {
			snap.Channels[chID] = s
		}
		if snap.Accounts[chID] == nil {
			snap.Accounts[chID] = make(map[string]*AccountSnapshot)
		}
		snap.Accounts[chID][accID] = s
	}
	return snap
}

// MarkLoggedOut 标记频道已登出。
func (m *Manager) MarkLoggedOut(channelID ChannelID, cleared bool, accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if accountID == "" {
		accountID = DefaultAccountID
	}
	key := runtimeKey(channelID, accountID)
	snap := m.snapshots[key]
	if snap == nil {
		snap = &AccountSnapshot{AccountID: accountID}
		m.snapshots[key] = snap
	}
	snap.LoggedIn = false
	if cleared {
		snap.Status = "stopped"
	}
}

// IsAccountEnabled 判断账户是否启用（默认 true）。
func IsAccountEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// GetPlugin 获取指定频道的插件实例。
func (m *Manager) GetPlugin(channelID ChannelID) Plugin {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plugins[channelID]
}

// NormalizeChannelID 规范化频道 ID。
func NormalizeChannelID(raw string) ChannelID {
	if raw == "" {
		return ""
	}
	return ChannelID(raw)
}

// ---------- 消息发送能力 ----------

// MessageSender 消息发送能力（可选，Plugin 可实现）。
// 通过类型断言 Plugin.(MessageSender) 来检查是否支持。
type MessageSender interface {
	SendMessage(params OutboundSendParams) (*OutboundSendResult, error)
}

// SendMessage 通过 ChannelManager 发送消息到指定频道。
// 如果对应 Plugin 实现了 MessageSender 接口，则调用其 SendMessage 方法。
func (m *Manager) SendMessage(channelID ChannelID, params OutboundSendParams) (*OutboundSendResult, error) {
	m.mu.Lock()
	plugin, ok := m.plugins[channelID]
	m.mu.Unlock()
	if !ok {
		return nil, NewSendError(channelID, SendErrUnavailable,
			fmt.Sprintf("channel %s: plugin not registered", channelID)).
			WithOperation("manager.resolve_plugin")
	}
	sender, ok := plugin.(MessageSender)
	if !ok {
		return nil, NewSendError(channelID, SendErrUnsupportedFeature,
			fmt.Sprintf("channel %s: does not support SendMessage", channelID)).
			WithOperation("manager.resolve_sender")
	}
	result, err := sender.SendMessage(params)
	if err != nil {
		if _, ok := AsSendError(err); ok {
			return nil, err
		}
		return nil, WrapSendError(channelID, SendErrUpstream, "manager.send",
			fmt.Sprintf("channel %s: send failed", channelID), err).
			WithRetryable(true)
	}
	return result, nil
}
