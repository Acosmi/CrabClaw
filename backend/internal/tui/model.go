// model.go — TUI 聊天客户端核心 Model
//
// 对齐 TS: src/tui/tui.ts(710L) + src/tui/tui-types.ts(108L)
// 使用 bubbletea Elm Architecture 实现状态管理。
//
// W1 阶段: 核心骨架 — View 仅返回占位布局，
// 具体渲染组件在 W2-W3 实现。
package tui

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/routing"
)

// ---------- 类型定义（对标 tui-types.ts）----------

// SessionScope 会话作用域。
type SessionScope string

const (
	SessionScopePerSender SessionScope = "per-sender"
	SessionScopeGlobal    SessionScope = "global"
)

// TuiOptions TUI 启动选项（对标 TS TuiOptions）。
type TuiOptions struct {
	URL          string
	Token        string
	Password     string
	Session      string
	Deliver      bool
	Thinking     string
	TimeoutMs    int
	HistoryLimit int
	Message      string
}

// SessionInfo 会话信息（对标 TS SessionInfo）。
type SessionInfo struct {
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
	VerboseLevel   string `json:"verboseLevel,omitempty"`
	ReasoningLevel string `json:"reasoningLevel,omitempty"`
	Model          string `json:"model,omitempty"`
	ModelProvider  string `json:"modelProvider,omitempty"`
	ContextTokens  *int   `json:"contextTokens,omitempty"`
	InputTokens    *int   `json:"inputTokens,omitempty"`
	OutputTokens   *int   `json:"outputTokens,omitempty"`
	TotalTokens    *int   `json:"totalTokens,omitempty"`
	ResponseUsage  string `json:"responseUsage,omitempty"` // "on"|"off"|"tokens"|"full"
	UpdatedAt      *int64 `json:"updatedAt,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
}

// AgentSummary Agent 概要信息。
type AgentSummary struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// ---------- tea.Msg 消息类型 ----------

// GatewayEventMsg Gateway 事件消息。
type GatewayEventMsg struct {
	Event   string
	Payload interface{}
	Seq     *int64
}

// GatewayConnectedMsg Gateway 连接成功。
type GatewayConnectedMsg struct{}

// GatewayDisconnectedMsg Gateway 断开连接。
type GatewayDisconnectedMsg struct {
	Reason string
}

// GatewayGapMsg Gateway 事件序号缺口。
type GatewayGapMsg struct {
	Expected int64
	Received int64
}

// GatewayReadyMsg Gateway 就绪（hello-ok 完成）。
type GatewayReadyMsg struct{}

// GatewayRequestResultMsg RPC 请求结果。
type GatewayRequestResultMsg struct {
	Method  string
	Payload interface{}
	Err     error
}

// ---------- Model ----------

// Model TUI 聊天客户端的核心状态。
// 实现 tea.Model 接口（Init/Update/View）。
type Model struct {
	// ---------- 连接 ----------
	client *GatewayChatClient
	opts   TuiOptions

	// ---------- 会话 ----------
	sessionScope          SessionScope
	sessionMainKey        string
	agentDefaultID        string
	currentAgentID        string
	currentSessionKey     string
	currentSessionID      string
	agents                []AgentSummary
	agentNames            map[string]string
	initialSessionInput   string
	initialSessionAgentID string
	initialSessionApplied bool

	// ---------- 消息状态 ----------
	activeChatRunID string
	historyLoaded   bool
	sessionInfo     SessionInfo

	// ---------- UI 状态 ----------
	isConnected      bool
	wasDisconnected  bool
	toolsExpanded    bool
	showThinking     bool
	autoMessageSent  bool
	connectionStatus string
	activityStatus   string
	lastCtrlCAt      time.Time

	// 差异 T-02: localRunIDs — 追踪本地发起的 run
	localRunIDs map[string]struct{}

	// ---------- W4: 事件处理状态 ----------
	finalizedRuns  map[string]int64 // 差异 E-01
	sessionRuns    map[string]int64 // 差异 E-01
	lastSessionKey string

	// ---------- W4: 本地 shell ----------
	localExecAsked      bool
	localExecAllowed    bool
	pendingShellCommand string

	// ---------- W4: 组件 ----------
	chatLog         *ChatLog
	streamAssembler *TuiStreamAssembler
	inputBox        InputBox
	statusBar       *StatusBar

	// ---------- W5: overlay ----------
	overlay overlayState

	// ---------- 布局 ----------
	width  int
	height int

	// ---------- 程序引用 ----------
	program *tea.Program
}

// NewModel 创建 TUI Model 实例。
func NewModel(opts TuiOptions) Model {
	loader := config.NewConfigLoader()
	cfg, _ := loader.LoadConfig()

	// 解析会话配置
	sessionScope := SessionScopePerSender
	sessionMainKey := routing.DefaultMainKey
	agentDefaultID := routing.DefaultAgentID

	if cfg != nil {
		if cfg.Session != nil {
			if string(cfg.Session.Scope) == "global" {
				sessionScope = SessionScopeGlobal
			}
			sessionMainKey = routing.NormalizeMainKey(cfg.Session.MainKey)
		}
		// 从 agents.list 中找到 default=true 的 agent
		if cfg.Agents != nil {
			for _, agent := range cfg.Agents.List {
				if agent.Default != nil && *agent.Default && agent.ID != "" {
					agentDefaultID = routing.NormalizeAgentID(agent.ID)
					break
				}
			}
		}
	}

	initialSessionInput := strings.TrimSpace(opts.Session)

	m := Model{
		opts:                opts,
		sessionScope:        sessionScope,
		sessionMainKey:      sessionMainKey,
		agentDefaultID:      agentDefaultID,
		currentAgentID:      agentDefaultID,
		agents:              nil,
		agentNames:          make(map[string]string),
		initialSessionInput: initialSessionInput,
		localRunIDs:         make(map[string]struct{}),
		finalizedRuns:       make(map[string]int64),
		sessionRuns:         make(map[string]int64),
		connectionStatus:    "connecting",
		activityStatus:      "idle",
		chatLog:             NewChatLog(),
		streamAssembler:     NewTuiStreamAssembler(),
		statusBar:           NewStatusBar(),
		width:               80,
		height:              24,
	}

	// 解析初始 session key
	m.currentSessionKey = m.resolveSessionKey(initialSessionInput)

	// 创建 gateway 客户端
	m.client = NewGatewayChatClient(GatewayConnectionOptions{
		URL:      opts.URL,
		Token:    opts.Token,
		Password: opts.Password,
	})

	return m
}

// ---------- tea.Model 接口 ----------

// Init 初始化 — 启动 gateway 连接。
func (m Model) Init() tea.Cmd {
	return m.connectGatewayCmd()
}

// Update 事件分发。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.hasOverlay() {
			return m.updateOverlay(msg)
		}
		return m.handleKeyMsg(msg)

	case GatewayConnectedMsg:
		m.isConnected = true
		reconnected := m.wasDisconnected
		m.wasDisconnected = false
		if reconnected {
			m.connectionStatus = "gateway reconnected"
		} else {
			m.connectionStatus = "gateway connected"
		}
		return m, nil

	case GatewayDisconnectedMsg:
		m.isConnected = false
		m.wasDisconnected = true
		m.historyLoaded = false
		reason := strings.TrimSpace(msg.Reason)
		if reason == "" {
			reason = "closed"
		}
		m.connectionStatus = fmt.Sprintf("gateway disconnected: %s", reason)
		m.activityStatus = "idle"
		return m, nil

	case GatewayGapMsg:
		m.connectionStatus = fmt.Sprintf("event gap: expected %d, got %d", msg.Expected, msg.Received)
		return m, nil

	case GatewayEventMsg:
		return m.handleGatewayEvent(msg)

	case GatewayRequestResultMsg:
		return m, nil

	case CommandResultMsg:
		for _, line := range msg.SystemMessages {
			m.chatLog.AddSystem(line)
		}
		if msg.Err != nil {
			if m.activeChatRunID != "" {
				m.forgetLocalRunID(m.activeChatRunID)
			}
			m.activeChatRunID = ""
			m.activityStatus = "error"
		}
		return m, nil

	case LocalShellResultMsg:
		if msg.Err != nil {
			m.chatLog.AddSystem(fmt.Sprintf("[local] error: %s", msg.Err))
		} else {
			for _, line := range msg.OutputLines {
				m.chatLog.AddSystem(fmt.Sprintf("[local] %s", line))
			}
			sig := ""
			if msg.Signal != "" {
				sig = fmt.Sprintf(" (signal %s)", msg.Signal)
			}
			m.chatLog.AddSystem(fmt.Sprintf("[local] exit %d%s", msg.ExitCode, sig))
		}
		return m, nil

	case InputSubmitMsg:
		return m.handleSubmit(msg.Text)

	case SessionInfoRefreshMsg:
		if msg.Info != nil {
			m.sessionInfo = *msg.Info
		}
		return m, nil

	case AgentsResultMsg:
		if msg.Err != nil {
			m.chatLog.AddSystem(fmt.Sprintf("agents list failed: %s", msg.Err))
		} else {
			m.applyAgentsResult(msg.Result)
		}
		return m, nil

	case SessionInfoResultMsg:
		m.handleSessionInfoResult(msg)
		return m, nil

	case HistoryResultMsg:
		cmd := m.handleHistoryResult(msg)
		return m, cmd

	case OverlayItemsMsg:
		if msg.Err != nil {
			m.chatLog.AddSystem(fmt.Sprintf("load failed: %s", msg.Err))
		} else if len(msg.Items) > 0 {
			title := "Select"
			switch msg.Kind {
			case OverlayAgentSelect:
				title = "Select Agent"
			case OverlaySessionSelect:
				title = "Select Session"
			case OverlayModelSelect:
				title = "Select Model"
			}
			m.openOverlay(msg.Kind, title, msg.Items)
		} else {
			m.chatLog.AddSystem("no items available")
		}
		return m, nil
	}

	return m, nil
}

// View 布局渲染。
func (m Model) View() string {
	// overlay 模式：全屏覆盖
	if m.hasOverlay() {
		return m.renderOverlay()
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	status := m.renderStatusLine()

	// 中间区域高度 = 总高度 - header(1) - footer(1) - status(1) - input(3)
	bodyHeight := m.height - 6
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	// W1 占位: 空白聊天区域
	chatArea := strings.Repeat("\n", bodyHeight)

	// W1 占位: 简单输入提示
	inputArea := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(m.width - 2).
		Render("> (input placeholder)")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		chatArea,
		status,
		footer,
		inputArea,
	)
}

// ---------- 键盘事件处理 ----------

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// 双击 Ctrl+C 退出
		now := time.Now()
		if now.Sub(m.lastCtrlCAt) < time.Second {
			m.client.Stop()
			return m, tea.Quit
		}
		m.lastCtrlCAt = now
		m.activityStatus = "press ctrl+c again to exit"
		return m, nil

	case tea.KeyCtrlD:
		m.client.Stop()
		return m, tea.Quit

	case tea.KeyEsc:
		cmd := m.handleAbortActive()
		return m, cmd

	case tea.KeyCtrlO:
		m.toolsExpanded = !m.toolsExpanded
		m.chatLog.SetToolsExpanded(m.toolsExpanded)
		if m.toolsExpanded {
			m.activityStatus = "tools expanded"
		} else {
			m.activityStatus = "tools collapsed"
		}
		return m, nil

	case tea.KeyCtrlT:
		m.showThinking = !m.showThinking
		return m, nil
	}

	// 差异 T-01: 三路分发逻辑
	// InputBox 提交会通过 InputSubmitMsg 处理

	return m, nil
}

// ---------- 输入提交 ----------

// handleSubmit 三路分发: ! → shell / / → command / else → sendMessage。
// TS 参考: tui.ts L480-520 handleSubmit 三路分发
func (m Model) handleSubmit(raw string) (tea.Model, tea.Cmd) {
	if raw == "" {
		return m, nil
	}

	// /yes 和 /no 处理本地 shell 权限响应
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "/yes" && m.pendingShellCommand != "" {
		cmd := m.handleLocalShellPermission(true)
		return m, cmd
	}
	if lower == "/no" && m.pendingShellCommand != "" {
		cmd := m.handleLocalShellPermission(false)
		return m, cmd
	}

	if strings.HasPrefix(raw, "!") {
		cmd := m.runLocalShellLine(raw)
		return m, cmd
	}
	if strings.HasPrefix(raw, "/") {
		cmd := m.handleCommand(raw)
		return m, cmd
	}

	cmd := m.sendMessage(raw)
	return m, cmd
}

// ---------- Gateway 事件处理 ----------

func (m Model) handleGatewayEvent(msg GatewayEventMsg) (tea.Model, tea.Cmd) {
	switch msg.Event {
	case "chat":
		cmd := m.handleChatEvent(msg.Payload)
		return m, cmd
	case "agent":
		cmd := m.handleAgentEvent(msg.Payload)
		return m, cmd
	}
	return m, nil
}

// ---------- Gateway 连接命令 ----------

func (m Model) connectGatewayCmd() tea.Cmd {
	return func() tea.Msg {
		m.client.Start()
		return GatewayReadyMsg{}
	}
}

// ---------- localRunId 管理（差异 T-02）----------

// noteLocalRunID 记录本地发起的 runId。
// 超过 200 条时淘汰最早的（简化：Go 中 map 无序，使用随机淘汰）。
func (m *Model) noteLocalRunID(runID string) {
	if runID == "" {
		return
	}
	m.localRunIDs[runID] = struct{}{}
	if len(m.localRunIDs) > 200 {
		for k := range m.localRunIDs {
			delete(m.localRunIDs, k)
			break
		}
	}
}

// forgetLocalRunID 移除 runId 追踪。
func (m *Model) forgetLocalRunID(runID string) {
	delete(m.localRunIDs, runID)
}

// isLocalRunID 检查是否为本地发起的 run。
func (m *Model) isLocalRunID(runID string) bool {
	_, ok := m.localRunIDs[runID]
	return ok
}

// clearLocalRunIDs 清空所有 localRunId。
func (m *Model) clearLocalRunIDs() {
	m.localRunIDs = make(map[string]struct{})
}

// ---------- Session Key 辅助（差异 T-04）----------

// resolveSessionKey 构建完整的 session key。
// 对标 TS tui.ts L300-318 resolveSessionKey()。
func (m *Model) resolveSessionKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if m.sessionScope == SessionScopeGlobal {
		return "global"
	}
	if trimmed == "" {
		return routing.BuildAgentMainSessionKey(m.currentAgentID, m.sessionMainKey)
	}
	if trimmed == "global" || trimmed == "unknown" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "agent:") {
		return trimmed
	}
	return fmt.Sprintf("agent:%s:%s", m.currentAgentID, trimmed)
}

// formatSessionKey 格式化 session key 用于显示。
// 对标 TS tui.ts L287-293 formatSessionKey()。
func (m *Model) formatSessionKey(key string) string {
	if key == "global" || key == "unknown" {
		return key
	}
	parsed := routing.ParseAgentSessionKey(key)
	if parsed != nil {
		return parsed.Rest
	}
	return key
}

// formatAgentLabel 格式化 agent 标签。
func (m *Model) formatAgentLabel(id string) string {
	if name, ok := m.agentNames[id]; ok && name != "" {
		return fmt.Sprintf("%s (%s)", id, name)
	}
	return id
}

// ---------- 布局渲染辅助 ----------

// renderHeader 渲染顶部标题栏。
func (m Model) renderHeader() string {
	sessionLabel := m.formatSessionKey(m.currentSessionKey)
	agentLabel := m.formatAgentLabel(m.currentAgentID)
	url := ""
	if m.client != nil {
		url = m.client.Connection().URL
	}
	text := fmt.Sprintf("Crab Claw TUI - %s - agent %s - session %s", url, agentLabel, sessionLabel)
	return HeadingStyle.Width(m.width).Render(text)
}

// renderFooter 渲染底部信息栏。
func (m Model) renderFooter() string {
	sessionKeyLabel := m.formatSessionKey(m.currentSessionKey)
	sessionLabel := sessionKeyLabel
	if m.sessionInfo.DisplayName != "" {
		sessionLabel = fmt.Sprintf("%s (%s)", sessionKeyLabel, m.sessionInfo.DisplayName)
	}

	agentLabel := m.formatAgentLabel(m.currentAgentID)

	modelLabel := "unknown"
	if m.sessionInfo.Model != "" {
		if m.sessionInfo.ModelProvider != "" {
			modelLabel = fmt.Sprintf("%s/%s", m.sessionInfo.ModelProvider, m.sessionInfo.Model)
		} else {
			modelLabel = m.sessionInfo.Model
		}
	}

	parts := []string{
		fmt.Sprintf("agent %s", agentLabel),
		fmt.Sprintf("session %s", sessionLabel),
		modelLabel,
	}

	think := m.sessionInfo.ThinkingLevel
	if think == "" {
		think = "off"
	}
	if think != "off" {
		parts = append(parts, fmt.Sprintf("think %s", think))
	}

	verbose := m.sessionInfo.VerboseLevel
	if verbose == "" {
		verbose = "off"
	}
	if verbose != "off" {
		parts = append(parts, fmt.Sprintf("verbose %s", verbose))
	}

	reasoning := m.sessionInfo.ReasoningLevel
	if reasoning == "" {
		reasoning = "off"
	}
	if reasoning == "on" {
		parts = append(parts, "reasoning")
	} else if reasoning == "stream" {
		parts = append(parts, "reasoning:stream")
	}

	return MutedStyle.Width(m.width).Render(strings.Join(parts, " | "))
}

// renderStatusLine 渲染状态行。
func (m Model) renderStatusLine() string {
	text := m.connectionStatus
	if m.activityStatus != "" && m.activityStatus != "idle" {
		text = fmt.Sprintf("%s | %s", m.connectionStatus, m.activityStatus)
	}
	return MutedStyle.Width(m.width).Render(text)
}

// ---------- 公开辅助 ----------

// Platform 返回当前平台标识。
func Platform() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}
