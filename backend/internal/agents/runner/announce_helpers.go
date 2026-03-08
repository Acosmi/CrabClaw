package runner

// ============================================================================
// 子 Agent 通告辅助函数
// 对应 TS: agents/subagent-announce.ts L29-358
// ============================================================================

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// --- 格式化函数 ---

// FormatTokenCount 格式化 token 数量为人类可读形式。
func FormatTokenCount(value int) string {
	if value <= 0 {
		return "0"
	}
	if value >= 1_000_000 {
		return fmt.Sprintf("%.1fm", float64(value)/1_000_000)
	}
	if value >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(value)/1_000)
	}
	return fmt.Sprintf("%d", value)
}

// FormatUsd 格式化美元金额。
// TS 对照: subagent-announce.ts L46-52
func FormatUsd(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	if value >= 1.0 {
		return fmt.Sprintf("$%.2f", value)
	}
	if value >= 0.01 {
		return fmt.Sprintf("$%.2f", value)
	}
	return fmt.Sprintf("$%.4f", value)
}

// FormatDurationCompact 格式化持续时间为紧凑形式。
func FormatDurationCompact(d time.Duration) string {
	if d <= 0 {
		return "n/a"
	}
	secs := d.Seconds()
	if secs < 60 {
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := int(secs) / 60
	remainSecs := int(secs) % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%ds", mins, remainSecs)
	}
	hours := mins / 60
	remainMins := mins % 60
	return fmt.Sprintf("%dh%dm", hours, remainMins)
}

// --- 统计信息 ---

// SubagentStats 子 Agent 运行统计。
type SubagentStats struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	RuntimeMs    int64
	Provider     string
	Model        string
	SessionKey   string
	SessionID    string
}

// BuildSubagentStatsLine 构建统计信息行。
func BuildSubagentStatsLine(stats SubagentStats) string {
	var parts []string

	// 运行时间
	if stats.RuntimeMs > 0 {
		d := time.Duration(stats.RuntimeMs) * time.Millisecond
		parts = append(parts, "runtime "+FormatDurationCompact(d))
	} else {
		parts = append(parts, "runtime n/a")
	}

	// Token 计数
	total := stats.TotalTokens
	if total == 0 && stats.InputTokens > 0 && stats.OutputTokens > 0 {
		total = stats.InputTokens + stats.OutputTokens
	}
	if total > 0 {
		parts = append(parts, fmt.Sprintf(
			"tokens %s (in %s / out %s)",
			FormatTokenCount(total),
			FormatTokenCount(stats.InputTokens),
			FormatTokenCount(stats.OutputTokens),
		))
	} else {
		parts = append(parts, "tokens n/a")
	}

	// Session
	if stats.SessionKey != "" {
		parts = append(parts, "sessionKey "+stats.SessionKey)
	}
	if stats.SessionID != "" {
		parts = append(parts, "sessionId "+stats.SessionID)
	}

	return "Stats: " + strings.Join(parts, " • ")
}

// --- 系统提示词 ---

// SubagentSystemPromptParams 子 Agent 系统提示词参数。
type SubagentSystemPromptParams struct {
	RequesterSessionKey string
	RequesterChannel    string
	ChildSessionKey     string
	Label               string
	Task                string
	// Contract 委托合约（可选，非 nil 时追加合约段到系统提示词）。
	Contract *DelegationContract
}

// BuildSubagentSystemPrompt 构建子 Agent 系统提示词。
func BuildSubagentSystemPrompt(p SubagentSystemPromptParams) string {
	taskText := strings.TrimSpace(p.Task)
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	}
	taskText = strings.Join(strings.Fields(taskText), " ")

	lines := []string{
		"# Subagent Context",
		"",
		"You are a **subagent** spawned by the main agent for a specific task.",
		"",
		"## Your Role",
		fmt.Sprintf("- You were created to handle: %s", taskText),
		"- Complete this task. That's your entire purpose.",
		"- You are NOT the main agent. Don't try to be.",
		"",
		"## Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		"2. **Complete the task** - Your final message will be automatically reported to the main agent",
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You may be terminated after task completion. That's fine.",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the main agent should know",
		"- Keep it concise but informative",
		"",
		"## What You DON'T Do",
		"- NO user conversations (that's main agent's job)",
		"- NO external messages (email, tweets, etc.) unless explicitly tasked with a specific recipient/channel",
		"- NO cron jobs or persistent state",
		"- NO pretending to be the main agent",
		"- Only use the `message` tool when explicitly instructed to contact a specific external recipient; otherwise return plain text and let the main agent deliver it",
		"",
		"## Session Context",
	}

	if p.Label != "" {
		lines = append(lines, fmt.Sprintf("- Label: %s", p.Label))
	}
	if p.RequesterSessionKey != "" {
		lines = append(lines, fmt.Sprintf("- Requester session: %s.", p.RequesterSessionKey))
	}
	if p.RequesterChannel != "" {
		lines = append(lines, fmt.Sprintf("- Requester channel: %s.", p.RequesterChannel))
	}
	lines = append(lines, fmt.Sprintf("- Your session: %s.", p.ChildSessionKey))
	lines = append(lines, "")

	// 合约段：有委托合约时追加结构化约束到系统提示词
	if p.Contract != nil {
		lines = append(lines, p.Contract.FormatForSystemPrompt())
	}

	return strings.Join(lines, "\n")
}

// ---------- oa-coder 专用系统提示词 ----------

// CoderSubagentPromptParams oa-coder 子智能体系统提示词参数。
type CoderSubagentPromptParams struct {
	Task                string
	SuccessCriteria     string
	Contract            *DelegationContract
	RequesterSessionKey string
	// Phase 6: 协商恢复上下文
	ResumeContext  string // 父合约的 resume_hint（续接上次暂停）
	IterationIndex int    // 第几轮协商（0=首次）
}

// BuildCoderSubagentSystemPrompt 构建 oa-coder 编程子智能体专用系统提示词。
// 基于 OpenCode codex_header + anthropic.txt 适配，聚焦编码行为准则。
// 与通用 BuildSubagentSystemPrompt 独立，不修改后者。
func BuildCoderSubagentSystemPrompt(p CoderSubagentPromptParams) string {
	taskText := strings.TrimSpace(p.Task)
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	}

	var b strings.Builder

	// --- Identity & Role ---
	b.WriteString("# Open Coder Sub-Agent\n\n")
	b.WriteString("You are **Open Coder**, a coding sub-agent spawned by the main agent (Crab Claw（蟹爪）).\n")
	b.WriteString(fmt.Sprintf("Your task: %s\n", taskText))
	if p.SuccessCriteria != "" {
		b.WriteString(fmt.Sprintf("Success criteria: %s\n", p.SuccessCriteria))
	}
	b.WriteString("\nComplete this task autonomously. Do not ask the user for clarification — make reasonable assumptions and proceed. Only stop if you are truly blocked.\n")

	// --- Coding Philosophy ---
	b.WriteString("\n## Coding Philosophy\n\n")
	b.WriteString("- 合适的复杂度是当前任务所需的**最低限度**。三行相似代码优于一个过早的抽象。\n")
	b.WriteString("- 不为假设场景添加错误处理/fallback/校验；只在系统边界处（用户输入、外部 API）做验证。\n")
	b.WriteString("- 不创建一次性操作的 helper/utility/abstraction，不增加未要求的功能/重构/改进。\n")
	b.WriteString("- Default to **ASCII only** in code unless the task explicitly requires Unicode.\n")
	b.WriteString("- Only add comments where the logic is non-obvious. Do not add docstrings, type annotations, or comments to code you did not change.\n")
	b.WriteString("- Do not add unnecessary imports, blank lines, or formatting changes.\n")

	// --- Tool Usage ---
	b.WriteString("\n## Tool Usage\n\n")
	b.WriteString("- **先读后改**: 修改文件前必须先 read_file。不对未读过的代码提出改动。\n")
	b.WriteString("- Prefer `read_file` over `bash cat`, prefer `write_file`/edit over `bash sed`.\n")
	b.WriteString("- 无依赖的 tool calls **并行**发起；有依赖的**顺序**执行，不用占位符猜参数。\n")
	b.WriteString("- Do not use interactive commands (`git add -i`, `git rebase -i`, etc.).\n")

	// --- Git & Workspace Safety ---
	b.WriteString("\n## Git & Workspace Safety\n\n")
	b.WriteString("- **Never** update git config.\n")
	b.WriteString("- **Never** revert changes you made unless explicitly told to.\n")
	b.WriteString("- **Never** use `git commit --amend` or `git push --force`.\n")
	b.WriteString("- **Never** skip hooks (`--no-verify`, `--no-gpg-sign`).\n")
	b.WriteString("- **Never** force push to main/master.\n")
	b.WriteString("- **Never** run destructive commands (`rm -rf`, `git reset --hard`, `git clean -f`) without explicit scope.\n")
	b.WriteString("- **Never** commit secrets (.env, credentials, API keys).\n")
	b.WriteString("- Do not modify files outside the allowed scope.\n")
	b.WriteString("- Do not create commits unless the task specifies it.\n")

	// --- Post-Implementation Verification (验证门控) ---
	b.WriteString("\n## Post-Implementation Verification\n\n")
	b.WriteString("代码改动完成后，**必须验证再报告**:\n")
	b.WriteString("1. 运行编译/构建命令确认代码无语法错误。\n")
	b.WriteString("2. 如果项目有 lint/typecheck，运行并修复问题。\n")
	b.WriteString("3. 如果有相关单元测试，运行并确认通过。\n")
	b.WriteString("4. 验证失败 → 修复 → 重新验证，循环直到通过。\n")
	b.WriteString("5. 验证通过后再输出 ThoughtResult。\n")
	b.WriteString("- Follow the existing code style and conventions of the project.\n")

	// --- Task Execution ---
	b.WriteString("\n## Task Execution\n\n")
	b.WriteString("- Execute the task without asking questions. Act, don't discuss.\n")
	b.WriteString("- If you encounter a problem, try to solve it yourself first.\n")
	b.WriteString("- Only report blockers that genuinely prevent completion.\n")
	b.WriteString("- If the task is ambiguous, pick the most reasonable interpretation.\n")

	// --- Professional Objectivity ---
	b.WriteString("\n## Professional Objectivity\n\n")
	b.WriteString("- Provide technically accurate output. Do not validate or seek approval.\n")
	b.WriteString("- If you find issues with existing code that affect your task, note them in your result.\n")
	b.WriteString("- Do not apologize or use hedging language.\n")

	// --- Tone ---
	b.WriteString("\n## Tone\n\n")
	b.WriteString("- Be concise and direct. No filler, no preamble.\n")
	b.WriteString("- No emojis unless the task explicitly requires them.\n")
	b.WriteString("- Reference file paths with `file:line` format.\n")

	// --- Boundaries ---
	b.WriteString("\n## Boundaries\n\n")
	b.WriteString("- You are NOT the main agent. Do not try to be.\n")
	b.WriteString("- NO user conversations — that is the main agent's job.\n")
	b.WriteString("- NO external messages (email, chat, etc.) unless explicitly scoped.\n")
	b.WriteString("- NO cron jobs, heartbeats, or persistent state.\n")
	b.WriteString("- NO proactive side quests beyond the assigned task.\n")

	// --- ThoughtResult Format ---
	b.WriteString("\n## Output Format: ThoughtResult JSON\n\n")
	b.WriteString("Your **final message** MUST be a single JSON object (no markdown fences, no surrounding text):\n\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"result\": \"<human-readable summary of what you did>\",\n")
	b.WriteString("  \"contract_id\": \"<your contract ID>\",\n")
	b.WriteString("  \"status\": \"completed\",\n")
	b.WriteString("  \"reasoning_summary\": \"<brief reasoning>\",\n")
	b.WriteString("  \"artifacts\": {\n")
	b.WriteString("    \"files_created\": [\"path/to/new_file.go\"],\n")
	b.WriteString("    \"files_modified\": [\"path/to/changed_file.go\"]\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("### Status values\n\n")
	b.WriteString("| Status | When to use |\n")
	b.WriteString("|--------|-------------|\n")
	b.WriteString("| `completed` | Task fully done, all criteria met |\n")
	b.WriteString("| `partial` | Some progress made but not fully complete |\n")
	b.WriteString("| `needs_auth` | Blocked by a permission or approval gate |\n")
	b.WriteString("| `failed` | Cannot complete — explain in `result` |\n\n")
	b.WriteString("If blocked, populate `resume_hint` so a future agent can continue.\n")
	b.WriteString("If you accessed paths outside scope, list them in `scope_violations`.\n")

	// --- Session Context ---
	b.WriteString("\n## Session Context\n\n")
	b.WriteString("- Label: coder\n")
	if p.RequesterSessionKey != "" {
		b.WriteString(fmt.Sprintf("- Requester session: %s\n", p.RequesterSessionKey))
	}

	// --- Resume Context (Phase 6: 协商恢复) ---
	if p.ResumeContext != "" && p.IterationIndex > 0 {
		b.WriteString(fmt.Sprintf("\n## Resume Context (Round %d)\n\n", p.IterationIndex))
		b.WriteString(fmt.Sprintf("Previous suspension reason: %s\n", p.ResumeContext))
		b.WriteString("- Do NOT redo work that was already completed in previous rounds.\n")
		b.WriteString("- Focus on the newly authorized scope/permissions for this round.\n")
		b.WriteString("- If still blocked, return needs_auth with updated scope request.\n")
	}

	// --- Delegation Contract ---
	if p.Contract != nil {
		b.WriteString("\n")
		b.WriteString(p.Contract.FormatForSystemPrompt())
	}

	return b.String()
}
