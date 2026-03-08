package autoreply

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TS 对照: auto-reply/reply/bash-command.ts (425L)
// 完整 bash 命令处理：/bash 和 ! 前缀命令。

// ==================== 常量 ====================

const (
	chatBashScopeKey    = "chat:bash"
	defaultForegroundMs = 2000
	maxForegroundMs     = 30_000
)

// ==================== BashRequest 类型 + 解析器 ====================

// BashAction bash 请求动作类型。
type BashAction string

const (
	BashActionHelp BashAction = "help"
	BashActionRun  BashAction = "run"
	BashActionPoll BashAction = "poll"
	BashActionStop BashAction = "stop"
)

// BashRequest 解析后的 bash 请求。
type BashRequest struct {
	Action    BashAction
	Command   string // action=run 时有值
	SessionID string // action=poll/stop 时可选
}

var bashPrefixRe = regexp.MustCompile(`(?i)^/bash(?:\s*:\s*|\s+|$)([\s\S]*)$`)

// ParseBashRequest 从原始消息体解析 bash 请求。
func ParseBashRequest(raw string) *BashRequest {
	trimmed := strings.TrimLeft(raw, " \t")
	var restSource string
	if strings.HasPrefix(strings.ToLower(trimmed), "/bash") {
		m := bashPrefixRe.FindStringSubmatch(trimmed)
		if m == nil {
			return nil
		}
		restSource = m[1]
	} else if strings.HasPrefix(trimmed, "!") {
		restSource = trimmed[1:]
		stripped := strings.TrimLeft(restSource, " \t")
		if strings.HasPrefix(stripped, ":") {
			restSource = stripped[1:]
		}
	} else {
		return nil
	}
	rest := strings.TrimLeft(restSource, " \t")
	if rest == "" {
		return &BashRequest{Action: BashActionHelp}
	}
	parts := strings.SplitN(rest, " ", 2)
	token := strings.TrimSpace(parts[0])
	remainder := ""
	if len(parts) > 1 {
		remainder = strings.TrimSpace(parts[1])
	}
	switch strings.ToLower(token) {
	case "poll":
		return &BashRequest{Action: BashActionPoll, SessionID: remainder}
	case "stop":
		return &BashRequest{Action: BashActionStop, SessionID: remainder}
	case "help":
		return &BashRequest{Action: BashActionHelp}
	default:
		return &BashRequest{Action: BashActionRun, Command: rest}
	}
}

// ==================== 格式化辅助 ====================

// FormatSessionSnippet 截断 session ID 用于显示。
func FormatSessionSnippet(sessionID string) string {
	trimmed := strings.TrimSpace(sessionID)
	if len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:8] + "…"
}

// FormatOutputBlock 将输出文本格式化为 Markdown 代码块。
func FormatOutputBlock(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "(no output)"
	}
	return "```txt\n" + trimmed + "\n```"
}

// ResolveForegroundMs 从配置解析前台等待毫秒数。
func ResolveForegroundMs(bashForegroundMs *int) int {
	if bashForegroundMs == nil {
		return defaultForegroundMs
	}
	v := *bashForegroundMs
	if v < 0 {
		return 0
	}
	if v > maxForegroundMs {
		return maxForegroundMs
	}
	return v
}

// BuildBashUsageReply 构建 bash 帮助信息回复。
func BuildBashUsageReply() *ReplyPayload {
	return &ReplyPayload{
		Text: strings.Join([]string{
			"⚙️ Usage:",
			"- ! <command>",
			"- !poll | ! poll",
			"- !stop | ! stop",
			"- /bash ... (alias; same subcommands as !)",
		}, "\n"),
	}
}

// ElevatedGateFailure elevated 门控失败项。
type ElevatedGateFailure struct {
	Gate string
	Key  string
}

// BashElevatedInfo elevated 权限信息。
type BashElevatedInfo struct {
	Enabled  bool
	Allowed  bool
	Failures []ElevatedGateFailure
}

// FormatElevatedUnavailableMessage 构建 elevated 不可用消息。
func FormatElevatedUnavailableMessage(
	runtimeSandboxed bool, failures []ElevatedGateFailure, sessionKey string,
) string {
	runtimeLabel := "direct"
	if runtimeSandboxed {
		runtimeLabel = "sandboxed"
	}
	lines := []string{
		fmt.Sprintf("elevated is not available right now (runtime=%s).", runtimeLabel),
	}
	if len(failures) > 0 {
		parts := make([]string, len(failures))
		for i, f := range failures {
			parts[i] = fmt.Sprintf("%s (%s)", f.Gate, f.Key)
		}
		lines = append(lines, "Failing gates: "+strings.Join(parts, ", "))
	} else {
		lines = append(lines,
			"Failing gates: enabled (tools.elevated.enabled / agents.list[].tools.elevated.enabled), "+
				"allowFrom (tools.elevated.allowFrom.<provider>).")
	}
	lines = append(lines, "Fix-it keys:",
		"- tools.elevated.enabled",
		"- tools.elevated.allowFrom.<provider>",
		"- agents.list[].tools.elevated.enabled",
		"- agents.list[].tools.elevated.allowFrom.<provider>",
	)
	if sessionKey != "" {
		lines = append(lines,
			fmt.Sprintf("See: `crabclaw sandbox explain --session %s`", sessionKey))
	}
	return strings.Join(lines, "\n")
}

// ==================== ActiveBashJob 状态管理 ====================

// BashJobState 活跃 bash 任务状态。
type BashJobState string

const (
	BashJobStarting BashJobState = "starting"
	BashJobRunning  BashJobState = "running"
)

// ActiveBashJob 当前活跃的 bash 任务。
type ActiveBashJob struct {
	State           BashJobState
	SessionID       string
	StartedAt       int64
	Command         string
	WatcherAttached bool
}

var (
	activeBashJob   *ActiveBashJob
	activeBashJobMu sync.Mutex
)

func getActiveBashJob() *ActiveBashJob {
	activeBashJobMu.Lock()
	defer activeBashJobMu.Unlock()
	return activeBashJob
}

func setActiveBashJob(job *ActiveBashJob) {
	activeBashJobMu.Lock()
	defer activeBashJobMu.Unlock()
	activeBashJob = job
}

func clearActiveBashJob() {
	activeBashJobMu.Lock()
	defer activeBashJobMu.Unlock()
	activeBashJob = nil
}

func clearActiveBashJobIfSession(sessionID string) {
	activeBashJobMu.Lock()
	defer activeBashJobMu.Unlock()
	if activeBashJob != nil &&
		activeBashJob.State == BashJobRunning &&
		activeBashJob.SessionID == sessionID {
		activeBashJob = nil
	}
}

// ResetBashStateForTests 测试用重置。
func ResetBashStateForTests() {
	activeBashJobMu.Lock()
	defer activeBashJobMu.Unlock()
	activeBashJob = nil
}

// ==================== 主入口 ====================

// HandleBashCommand 处理 /bash 和 ! 命令。
func HandleBashCommand(ctx context.Context, params *HandleCommandsParams, allowTextCommands bool) (*CommandHandlerResult, error) {
	if !allowTextCommands {
		return nil, nil
	}
	cmd := params.Command
	body := cmd.CommandBodyNormalized
	bashSlashRequested := body == "/bash" || strings.HasPrefix(body, "/bash ")
	bashBangRequested := strings.HasPrefix(body, "!")
	if !bashSlashRequested && !(bashBangRequested && cmd.IsAuthorizedSender) {
		return nil, nil
	}
	if !cmd.IsAuthorizedSender {
		return &CommandHandlerResult{ShouldContinue: false}, nil
	}

	request := ParseBashRequest(body)
	if request == nil {
		return &CommandHandlerResult{
			ShouldContinue: false,
			Reply:          &ReplyPayload{Text: "⚠️ Unrecognized bash request."},
		}, nil
	}
	liveJob := getActiveBashJob()

	switch request.Action {
	case BashActionHelp:
		return &CommandHandlerResult{ShouldContinue: false, Reply: BuildBashUsageReply()}, nil
	case BashActionPoll:
		return &CommandHandlerResult{ShouldContinue: false, Reply: handleBashPoll(request, liveJob)}, nil
	case BashActionStop:
		return &CommandHandlerResult{ShouldContinue: false, Reply: handleBashStop(ctx, request, liveJob, params)}, nil
	default:
		return &CommandHandlerResult{ShouldContinue: false, Reply: handleBashRun(ctx, request, liveJob, params)}, nil
	}
}

// ==================== 子命令处理 ====================

func handleBashPoll(req *BashRequest, liveJob *ActiveBashJob) *ReplyPayload {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" && liveJob != nil && liveJob.State == BashJobRunning {
		sessionID = liveJob.SessionID
	}
	if sessionID == "" {
		return &ReplyPayload{Text: "⚙️ No active bash job."}
	}
	job := getActiveBashJob()
	if job != nil && job.State == BashJobRunning && job.SessionID == sessionID {
		runtimeSec := int(math.Max(0, float64(time.Now().UnixMilli()-job.StartedAt)/1000))
		return &ReplyPayload{
			Text: strings.Join([]string{
				fmt.Sprintf("⚙️ bash still running (session %s, %ds).", FormatSessionSnippet(sessionID), runtimeSec),
				FormatOutputBlock("(streaming output not yet captured)"),
				"Hint: !stop (or /bash stop)",
			}, "\n"),
		}
	}
	clearActiveBashJobIfSession(sessionID)
	return &ReplyPayload{
		Text: fmt.Sprintf("⚙️ No bash session found for %s.", FormatSessionSnippet(sessionID)),
	}
}

func handleBashStop(ctx context.Context, req *BashRequest, liveJob *ActiveBashJob, params *HandleCommandsParams) *ReplyPayload {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" && liveJob != nil && liveJob.State == BashJobRunning {
		sessionID = liveJob.SessionID
	}
	if sessionID == "" {
		return &ReplyPayload{Text: "⚙️ No active bash job."}
	}
	if params.BashExecutor != nil {
		_, _ = params.BashExecutor.HandleBashChatCommand(ctx, map[string]any{
			"action": "stop", "sessionId": sessionID,
		})
	}
	clearActiveBashJobIfSession(sessionID)
	return &ReplyPayload{
		Text: fmt.Sprintf("⚙️ bash stopped (session %s).", FormatSessionSnippet(sessionID)),
	}
}

func handleBashRun(ctx context.Context, req *BashRequest, liveJob *ActiveBashJob, params *HandleCommandsParams) *ReplyPayload {
	if liveJob != nil {
		label := "starting"
		if liveJob.State == BashJobRunning {
			label = FormatSessionSnippet(liveJob.SessionID)
		}
		return &ReplyPayload{
			Text: fmt.Sprintf("⚠️ A bash job is already running (%s). Use !poll / !stop.", label),
		}
	}
	commandText := strings.TrimSpace(req.Command)
	if commandText == "" {
		return BuildBashUsageReply()
	}
	setActiveBashJob(&ActiveBashJob{
		State: BashJobStarting, StartedAt: time.Now().UnixMilli(), Command: commandText,
	})
	if params.BashExecutor == nil {
		clearActiveBashJob()
		return &ReplyPayload{Text: "⚠️ Bash execution not available."}
	}
	reply, err := params.BashExecutor.HandleBashChatCommand(ctx, map[string]any{
		"command": commandText, "sessionKey": params.SessionKey, "channel": params.Command.Channel,
	})
	if err != nil {
		clearActiveBashJob()
		return &ReplyPayload{
			Text: strings.Join([]string{
				fmt.Sprintf("⚠️ bash failed: %s", commandText),
				FormatOutputBlock(err.Error()),
			}, "\n"),
		}
	}
	clearActiveBashJob()
	if reply == nil {
		return &ReplyPayload{
			Text: strings.Join([]string{
				fmt.Sprintf("⚙️ bash: %s", commandText),
				"Exit: 0", FormatOutputBlock("(no output)"),
			}, "\n"),
		}
	}
	return reply
}
