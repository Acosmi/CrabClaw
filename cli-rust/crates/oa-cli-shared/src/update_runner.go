package infra

// update_runner.go — 更新执行器
// 对应 TS: src/infra/update-runner.ts (912L) — 简化版
//
// Go 端简化：不需要 npm/pnpm 依赖安装逻辑。
// 核心功能：git pull + 重新构建 + sentinel 写入 + 重启。

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// UpdateMode 更新模式。
type UpdateMode string

const (
	UpdateModeGitPull UpdateMode = "git-pull"
	UpdateModeInPlace UpdateMode = "in-place"
)

// UpdateRunParams 更新执行参数。
type UpdateRunParams struct {
	Root      string
	StateDir  string
	Mode      UpdateMode
	TimeoutMs int
	DryRun    bool
}

// UpdateRunResult 更新执行结果。
type UpdateRunResult struct {
	Success  bool                  `json:"success"`
	Mode     UpdateMode            `json:"mode"`
	Steps    []RestartSentinelStep `json:"steps,omitempty"`
	Duration time.Duration         `json:"duration"`
	Error    string                `json:"error,omitempty"`
}

// RunUpdate 执行更新流程。
// 对应 TS: runUpdate(params) — 简化版
func RunUpdate(ctx context.Context, params UpdateRunParams) UpdateRunResult {
	started := time.Now()
	result := UpdateRunResult{Mode: params.Mode}

	timeoutMs := params.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 120_000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	switch params.Mode {
	case UpdateModeGitPull:
		result = runGitPullUpdate(ctx, params, timeout)
	default:
		result.Error = fmt.Sprintf("unsupported update mode: %s", params.Mode)
	}

	result.Duration = time.Since(started)
	return result
}

func runGitPullUpdate(ctx context.Context, params UpdateRunParams, timeout time.Duration) UpdateRunResult {
	result := UpdateRunResult{Mode: UpdateModeGitPull}
	var steps []RestartSentinelStep

	// Step 1: git fetch
	step, err := runUpdateStep(ctx, params.Root, timeout, "git-fetch", "git", "fetch", "--all", "--prune")
	steps = append(steps, step)
	if err != nil {
		result.Steps = steps
		result.Error = fmt.Sprintf("git fetch failed: %v", err)
		return result
	}

	// Step 2: git pull --rebase
	if !params.DryRun {
		step, err = runUpdateStep(ctx, params.Root, timeout, "git-pull", "git", "pull", "--rebase")
		steps = append(steps, step)
		if err != nil {
			result.Steps = steps
			result.Error = fmt.Sprintf("git pull failed: %v", err)
			return result
		}
	}

	// Step 3: go build (重新构建)
	if !params.DryRun {
		step, err = runUpdateStep(ctx, params.Root, timeout, "go-build", "go", "build", "./...")
		steps = append(steps, step)
		if err != nil {
			result.Steps = steps
			result.Error = fmt.Sprintf("go build failed: %v", err)
			return result
		}
	}

	result.Steps = steps
	result.Success = true
	return result
}

func runUpdateStep(ctx context.Context, dir string, timeout time.Duration, name string, cmdName string, args ...string) (RestartSentinelStep, error) {
	started := time.Now()
	step := RestartSentinelStep{
		Name:    name,
		Command: fmt.Sprintf("%s %s", cmdName, joinArgs(args)),
		Cwd:     dir,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	durationMs := time.Since(started).Milliseconds()
	step.DurationMs = &durationMs

	log := &RestartSentinelLog{}
	if len(out) > 0 {
		log.StdoutTail = TrimLogTail(string(out), 4000)
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			log.ExitCode = &code
			log.StderrTail = TrimLogTail(string(exitErr.Stderr), 4000)
		}
	}
	step.Log = log

	return step, err
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
