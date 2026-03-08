package infra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------- 心跳配置 ----------

// HeartbeatConfig 心跳配置。
type HeartbeatConfig struct {
	Enabled    bool     `json:"enabled"`
	IntervalMs int      `json:"intervalMs,omitempty"`
	ActiveFrom string   `json:"activeFrom,omitempty"` // "HH:MM"
	ActiveTo   string   `json:"activeTo,omitempty"`   // "HH:MM"
	Channels   []string `json:"channels,omitempty"`
	FilePath   string   `json:"filePath,omitempty"` // HEARTBEAT.md 路径
}

// isEnabled 心跳是否启用。
func (c *HeartbeatConfig) isEnabled() bool {
	return c != nil && c.Enabled
}

func (c *HeartbeatConfig) interval() time.Duration {
	if c == nil || c.IntervalMs <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.IntervalMs) * time.Millisecond
}

// ---------- Agent 状态 ----------

type heartbeatAgentState struct {
	AgentID          string
	Heartbeat        HeartbeatConfig
	LastRunAt        time.Time
	NextDueAt        time.Time
	ConsecutiveFails int
}

// ---------- 心跳调度器 ----------

// HeartbeatRunner 心跳调度器。
type HeartbeatRunner struct {
	mu       sync.Mutex
	agents   map[string]*heartbeatAgentState
	waker    *HeartbeatWaker
	cfg      HeartbeatRunnerConfig
	stopped  bool
	stopOnce sync.Once
	cancel   context.CancelFunc
}

// HeartbeatRunnerConfig 调度器配置。
type HeartbeatRunnerConfig struct {
	DataDir        string // agent 数据目录基路径
	RunOnce        HeartbeatRunOnceFunc
	OnConfigUpdate func() // 配置更新回调（可选）
}

// HeartbeatRunOnceFunc 单次心跳执行函数签名。
type HeartbeatRunOnceFunc func(ctx context.Context, opts HeartbeatRunOnceOpts) HeartbeatRunResult

// HeartbeatRunOnceOpts 单次心跳选项。
type HeartbeatRunOnceOpts struct {
	AgentID   string
	Heartbeat HeartbeatConfig
	Reason    string
}

// StartHeartbeatRunner 启动心跳调度器。
func StartHeartbeatRunner(cfg HeartbeatRunnerConfig) *HeartbeatRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &HeartbeatRunner{
		agents: make(map[string]*heartbeatAgentState),
		waker:  NewHeartbeatWaker(),
		cfg:    cfg,
		cancel: cancel,
	}

	r.waker.SetHandler(func(reason string) HeartbeatRunResult {
		return r.runAll(ctx, reason)
	})

	return r
}

// UpdateAgents 更新被调度的 agent 列表。
func (r *HeartbeatRunner) UpdateAgents(configs map[string]HeartbeatConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 添加 / 更新
	now := time.Now()
	for agentID, hb := range configs {
		existing, ok := r.agents[agentID]
		if ok {
			existing.Heartbeat = hb
		} else {
			r.agents[agentID] = &heartbeatAgentState{
				AgentID:   agentID,
				Heartbeat: hb,
				NextDueAt: now.Add(hb.interval()),
			}
		}
	}
	// 移除不存在的
	for id := range r.agents {
		if _, ok := configs[id]; !ok {
			delete(r.agents, id)
		}
	}

	// 立即触发一次检查
	r.waker.RequestNow("config-update", 100)
}

// Stop 停止调度器。
func (r *HeartbeatRunner) Stop() {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		r.stopped = true
		r.mu.Unlock()
		r.cancel()
		r.waker.Stop()
	})
}

// ---------- 内部调度 ----------

func (r *HeartbeatRunner) runAll(ctx context.Context, reason string) HeartbeatRunResult {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return HeartbeatRunResult{Status: "skipped", Reason: "stopped"}
	}
	// 快照 agent 列表
	agents := make([]*heartbeatAgentState, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	r.mu.Unlock()

	now := time.Now()
	var lastResult HeartbeatRunResult
	ranAny := false

	for _, agent := range agents {
		if ctx.Err() != nil {
			break
		}
		if !agent.Heartbeat.isEnabled() {
			continue
		}
		if !r.isDue(agent, now, reason) {
			continue
		}
		if !isInActiveHours(agent.Heartbeat, now) {
			continue
		}

		result := r.executeOne(ctx, agent, reason)
		lastResult = result

		r.mu.Lock()
		if result.Status == "ran" {
			agent.LastRunAt = time.Now()
			agent.ConsecutiveFails = 0
		} else if result.Status == "failed" {
			agent.ConsecutiveFails++
		}
		agent.NextDueAt = time.Now().Add(agent.Heartbeat.interval())
		r.mu.Unlock()

		ranAny = true
	}

	// 调度下一次
	r.scheduleNext()

	if !ranAny {
		return HeartbeatRunResult{Status: "skipped", Reason: "no-agents-due"}
	}
	return lastResult
}

func (r *HeartbeatRunner) isDue(agent *heartbeatAgentState, now time.Time, reason string) bool {
	if reason == "config-update" || reason == "manual" {
		return true
	}
	return !now.Before(agent.NextDueAt)
}

func (r *HeartbeatRunner) executeOne(ctx context.Context, agent *heartbeatAgentState, reason string) HeartbeatRunResult {
	runOnce := r.cfg.RunOnce
	if runOnce == nil {
		return r.defaultRunOnce(ctx, agent, reason)
	}
	return runOnce(ctx, HeartbeatRunOnceOpts{
		AgentID:   agent.AgentID,
		Heartbeat: agent.Heartbeat,
		Reason:    reason,
	})
}

func (r *HeartbeatRunner) defaultRunOnce(ctx context.Context, agent *heartbeatAgentState, reason string) HeartbeatRunResult {
	start := time.Now()

	// 读取 HEARTBEAT.md
	content, err := r.readHeartbeatFile(agent)
	if err != nil {
		EmitHeartbeatEvent(HeartbeatEventPayload{
			Status: HeartbeatStatusFailed,
			Reason: fmt.Sprintf("read-file: %v", err),
		})
		return HeartbeatRunResult{Status: "failed", Reason: err.Error()}
	}

	if isHeartbeatContentEffectivelyEmpty(content) {
		EmitHeartbeatEvent(HeartbeatEventPayload{
			Status:        HeartbeatStatusOKEmpty,
			IndicatorType: ResolveIndicatorType(HeartbeatStatusOKEmpty),
		})
		return HeartbeatRunResult{
			Status:     "ran",
			DurationMs: time.Since(start).Milliseconds(),
			Reason:     "empty-content",
		}
	}

	// 发出 sent 事件
	EmitHeartbeatEvent(HeartbeatEventPayload{
		Status:        HeartbeatStatusSent,
		Preview:       truncatePreview(content, 120),
		Reason:        reason,
		IndicatorType: ResolveIndicatorType(HeartbeatStatusSent),
	})

	return HeartbeatRunResult{
		Status:     "ran",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func (r *HeartbeatRunner) readHeartbeatFile(agent *heartbeatAgentState) (string, error) {
	filePath := agent.Heartbeat.FilePath
	if filePath == "" {
		if r.cfg.DataDir != "" {
			filePath = filepath.Join(r.cfg.DataDir, agent.AgentID, "HEARTBEAT.md")
		} else {
			return "", fmt.Errorf("no heartbeat file path configured")
		}
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 无文件视为空
		}
		return "", err
	}
	return string(data), nil
}

func (r *HeartbeatRunner) scheduleNext() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}

	now := time.Now()
	var minDelay time.Duration
	found := false

	for _, agent := range r.agents {
		if !agent.Heartbeat.isEnabled() {
			continue
		}
		delay := agent.NextDueAt.Sub(now)
		if delay < 0 {
			delay = 100 * time.Millisecond
		}
		if !found || delay < minDelay {
			minDelay = delay
			found = true
		}
	}

	if found {
		r.waker.RequestNow("interval", int(minDelay.Milliseconds()))
	}
}

// ---------- 工具函数 ----------

func isInActiveHours(cfg HeartbeatConfig, now time.Time) bool {
	from := strings.TrimSpace(cfg.ActiveFrom)
	to := strings.TrimSpace(cfg.ActiveTo)
	if from == "" && to == "" {
		return true
	}
	parseHM := func(hm string) (int, int, bool) {
		parts := strings.SplitN(hm, ":", 2)
		if len(parts) != 2 {
			return 0, 0, false
		}
		h, m := 0, 0
		fmt.Sscanf(parts[0], "%d", &h)
		fmt.Sscanf(parts[1], "%d", &m)
		return h, m, true
	}

	nowMinutes := now.Hour()*60 + now.Minute()

	if from != "" {
		fh, fm, ok := parseHM(from)
		if ok && nowMinutes < fh*60+fm {
			return false
		}
	}
	if to != "" {
		th, tm, ok := parseHM(to)
		if ok && nowMinutes >= th*60+tm {
			return false
		}
	}
	return true
}

func isHeartbeatContentEffectivelyEmpty(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return true
	}
	// 仅包含空 markdown 标题
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "---") {
			return false
		}
	}
	return true
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
