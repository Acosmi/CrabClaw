package infra

// heartbeat_delivery_run.go — Heartbeat 单次执行（全量）
// 对应 TS: heartbeat-runner.ts runHeartbeatOnce 核心分支
// FIX-6: 调用 FormatForChannel + RestoreUpdatedAt

import (
	"time"
)

// RunHeartbeatOnce 执行一次心跳（TS L490-660）。
func RunHeartbeatOnce(cfg *HeartbeatAgentConfig, deps HeartbeatDeliveryDeps) HeartbeatDeliveryResult {
	if cfg == nil || !cfg.Enabled {
		return HeartbeatDeliveryResult{Status: HeartbeatStatusSkipped}
	}
	if cfg.Channel == "" || cfg.To == "" {
		return HeartbeatDeliveryResult{Status: HeartbeatStatusOKEmpty}
	}

	prompt := ResolveHeartbeatPrompt(cfg)
	nowMs := deps.NowMs
	if nowMs == nil {
		nowMs = defaultNowMs
	}

	if deps.SendMessage == nil {
		return HeartbeatDeliveryResult{
			Status:   HeartbeatStatusFailed,
			ErrorMsg: "no send function provided",
		}
	}

	// 格式化 prompt（如果有 channel adapter）
	formatted := prompt
	if deps.FormatForChannel != nil {
		formatted = deps.FormatForChannel(cfg.Channel, prompt)
	}

	err := deps.SendMessage(cfg.Channel, cfg.To, formatted)
	if err != nil {
		EmitHeartbeatEvent(HeartbeatEventPayload{
			Status:  HeartbeatStatusFailed,
			Channel: cfg.Channel,
			To:      cfg.To,
			Preview: err.Error(),
		})
		return HeartbeatDeliveryResult{
			Status:   HeartbeatStatusFailed,
			Channel:  cfg.Channel,
			To:       cfg.To,
			ErrorMsg: err.Error(),
		}
	}

	preview := NormalizeHeartbeatReply(prompt)
	ts := nowMs()

	// 恢复 updatedAt（防止心跳触发后续流水线）
	if deps.RestoreUpdatedAt != nil && cfg.SessionKey != "" {
		deps.RestoreUpdatedAt(cfg.SessionKey, ts)
	}

	EmitHeartbeatEvent(HeartbeatEventPayload{
		Status:  HeartbeatStatusSent,
		Channel: cfg.Channel,
		To:      cfg.To,
		Preview: preview,
	})
	return HeartbeatDeliveryResult{
		Status:    HeartbeatStatusSent,
		Channel:   cfg.Channel,
		To:        cfg.To,
		Preview:   preview,
		UpdatedAt: ts,
	}
}

// RunHeartbeatBatch 批量执行多个 agent 的心跳。
func RunHeartbeatBatch(configs map[string]*HeartbeatAgentConfig, deps HeartbeatDeliveryDeps) map[string]HeartbeatDeliveryResult {
	results := make(map[string]HeartbeatDeliveryResult, len(configs))
	for agentID, cfg := range configs {
		results[agentID] = RunHeartbeatOnce(cfg, deps)
	}
	return results
}

// NextHeartbeatDueMs 计算下一次心跳时间。
func NextHeartbeatDueMs(lastRunMs int64, cfg *HeartbeatAgentConfig) int64 {
	interval := ResolveHeartbeatIntervalMs(cfg)
	if lastRunMs <= 0 {
		return time.Now().UnixMilli()
	}
	return lastRunMs + interval
}
