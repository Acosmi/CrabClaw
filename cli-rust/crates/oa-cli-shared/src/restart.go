package infra

// restart.go — 进程重启管理 + sentinel 文件
// 对应 TS:
//   - src/infra/restart.ts (222L)
//   - src/infra/restart-sentinel.ts (131L)
//
// Sentinel 文件机制：重启前写入 sentinel，重启后读取并消费，
// 确保重启上下文（频道、线程 ID 等）跨进程传递。

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ─── Sentinel 类型 ───

// RestartSentinelKind 重启类型。
type RestartSentinelKind string

const (
	RestartKindConfigApply RestartSentinelKind = "config-apply"
	RestartKindUpdate      RestartSentinelKind = "update"
	RestartKindRestart     RestartSentinelKind = "restart"
)

// RestartSentinelStatus 重启结果状态。
type RestartSentinelStatus string

const (
	RestartStatusOk      RestartSentinelStatus = "ok"
	RestartStatusError   RestartSentinelStatus = "error"
	RestartStatusSkipped RestartSentinelStatus = "skipped"
)

// RestartSentinelLog 重启步骤日志。
type RestartSentinelLog struct {
	StdoutTail string `json:"stdoutTail,omitempty"`
	StderrTail string `json:"stderrTail,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
}

// RestartSentinelStep 重启执行步骤。
type RestartSentinelStep struct {
	Name       string              `json:"name"`
	Command    string              `json:"command"`
	Cwd        string              `json:"cwd,omitempty"`
	DurationMs *int64              `json:"durationMs,omitempty"`
	Log        *RestartSentinelLog `json:"log,omitempty"`
}

// RestartSentinelStats 重启统计信息。
type RestartSentinelStats struct {
	Mode       string                 `json:"mode,omitempty"`
	Root       string                 `json:"root,omitempty"`
	Before     map[string]interface{} `json:"before,omitempty"`
	After      map[string]interface{} `json:"after,omitempty"`
	Steps      []RestartSentinelStep  `json:"steps,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
	DurationMs *int64                 `json:"durationMs,omitempty"`
}

// RestartDeliveryContext 重启时捕获的投递上下文。
type RestartDeliveryContext struct {
	Channel   string `json:"channel,omitempty"`
	To        string `json:"to,omitempty"`
	AccountID string `json:"accountId,omitempty"`
}

// RestartSentinelPayload sentinel 载荷。
type RestartSentinelPayload struct {
	Kind            RestartSentinelKind     `json:"kind"`
	Status          RestartSentinelStatus   `json:"status"`
	Ts              int64                   `json:"ts"`
	SessionKey      string                  `json:"sessionKey,omitempty"`
	DeliveryContext *RestartDeliveryContext `json:"deliveryContext,omitempty"`
	ThreadID        string                  `json:"threadId,omitempty"`
	Message         string                  `json:"message,omitempty"`
	DoctorHint      string                  `json:"doctorHint,omitempty"`
	Stats           *RestartSentinelStats   `json:"stats,omitempty"`
}

// RestartSentinel sentinel 文件结构。
type RestartSentinel struct {
	Version int                    `json:"version"`
	Payload RestartSentinelPayload `json:"payload"`
}

const sentinelFilename = "restart-sentinel.json"

// ─── Sentinel 操作 ───

// WriteRestartSentinel 写入重启 sentinel 文件。
// 对应 TS: writeRestartSentinel(payload, env)
func WriteRestartSentinel(stateDir string, payload RestartSentinelPayload) (string, error) {
	filePath := filepath.Join(stateDir, sentinelFilename)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return "", fmt.Errorf("create sentinel dir: %w", err)
	}

	data := RestartSentinel{Version: 1, Payload: payload}
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal sentinel: %w", err)
	}
	jsonBytes = append(jsonBytes, '\n')
	if err := os.WriteFile(filePath, jsonBytes, 0o644); err != nil {
		return "", fmt.Errorf("write sentinel: %w", err)
	}
	return filePath, nil
}

// ReadRestartSentinel 读取重启 sentinel 文件。
// 对应 TS: readRestartSentinel(env)
func ReadRestartSentinel(stateDir string) *RestartSentinel {
	filePath := filepath.Join(stateDir, sentinelFilename)
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var sentinel RestartSentinel
	if err := json.Unmarshal(raw, &sentinel); err != nil {
		os.Remove(filePath)
		return nil
	}
	if sentinel.Version != 1 {
		os.Remove(filePath)
		return nil
	}
	return &sentinel
}

// ConsumeRestartSentinel 读取并删除 sentinel 文件。
// 对应 TS: consumeRestartSentinel(env)
func ConsumeRestartSentinel(stateDir string) *RestartSentinel {
	sentinel := ReadRestartSentinel(stateDir)
	if sentinel == nil {
		return nil
	}
	os.Remove(filepath.Join(stateDir, sentinelFilename))
	return sentinel
}

// SummarizeRestartSentinel 格式化重启摘要。
// 对应 TS: summarizeRestartSentinel(payload)
func SummarizeRestartSentinel(payload RestartSentinelPayload) string {
	mode := ""
	if payload.Stats != nil && payload.Stats.Mode != "" {
		mode = " (" + payload.Stats.Mode + ")"
	}
	return strings.TrimSpace(fmt.Sprintf("Gateway restart %s %s%s", payload.Kind, payload.Status, mode))
}

// TrimLogTail 截取日志尾部。
// 对应 TS: trimLogTail(input, maxChars)
func TrimLogTail(input string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 8000
	}
	text := strings.TrimRight(input, " \t\n\r")
	if len(text) <= maxChars {
		return text
	}
	return "…" + text[len(text)-maxChars:]
}

// FormatRestartSentinelMessage 格式化重启 sentinel 消息。
// 对应 TS: formatRestartSentinelMessage(payload)
func FormatRestartSentinelMessage(payload RestartSentinelPayload) string {
	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("GatewayRestart:\n{\"error\": \"%s\"}", err.Error())
	}
	return fmt.Sprintf("GatewayRestart:\n%s", string(jsonBytes))
}

// ─── 重启执行 ───

// RestartOptions 重启选项。
type RestartOptions struct {
	StateDir string
	Reason   string
	Args     []string
}

// ScheduleRestart 调度进程重启。
// 对应 TS: restart.ts 中的重启逻辑
//
// 写入 sentinel 后通过 exec 替换当前进程。
func ScheduleRestart(opts RestartOptions, deliveryCtx *RestartDeliveryContext) error {
	payload := RestartSentinelPayload{
		Kind:            RestartKindRestart,
		Status:          RestartStatusOk,
		Ts:              time.Now().UnixMilli(),
		DeliveryContext: deliveryCtx,
		Stats:           &RestartSentinelStats{Reason: opts.Reason},
	}

	if _, err := WriteRestartSentinel(opts.StateDir, payload); err != nil {
		return fmt.Errorf("write restart sentinel: %w", err)
	}

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	args := opts.Args
	if len(args) == 0 {
		args = os.Args[1:]
	}

	// 使用 exec 启动新进程
	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new process: %w", err)
	}

	return nil
}
