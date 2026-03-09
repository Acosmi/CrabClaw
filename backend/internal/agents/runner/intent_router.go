package runner

// intent_router.go — 五维联动架构核心：六级意图分类 + 工具-技能绑定 + 行为指引注入
//
// 架构 v2 核心组件。替代旧三级 (greeting/chat/task) 关键词匹配，
// 实现精细化意图路由，联动工具树、技能树、安全层和提示词系统。
//
// 行业对齐:
//   - Anthropic Progressive Disclosure: 按意图逐级暴露工具
//   - Google ADK Dispatcher Pattern: 意图驱动路由
//   - OpenAI Agents SDK Manager Pattern: 任务委托
//   - Microsoft Zero Trust: 操作分级审批

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// ---------- 意图分级 ----------

// intentTier 六级意图分类（决定工具暴露 + 历史裁剪 + 行为指引）。
type intentTier string

const (
	intentGreeting       intentTier = "greeting"        // 问候 → 零工具
	intentQuestion       intentTier = "question"        // 提问/回顾 → 搜索工具
	intentTaskLight      intentTier = "task_light"      // 轻量查看 → 读取 + bash
	intentTaskWrite      intentTier = "task_write"      // 创建/修改 → 写入 + coder 委托
	intentTaskDelete     intentTier = "task_delete"     // 破坏性操作 → bash(带审批)
	intentTaskMultimodal intentTier = "task_multimodal" // 视觉/浏览器 → 全部工具
)

// classifyIntent 根据用户 prompt 快速分类意图（六级）。
// 优先级: greeting > question > task_delete > task_multimodal > task_write > task_light
//
// Stage 1 纯规则路由（零 LLM 成本，<1ms）。
// Stage 2 轻量 LLM 分类（预留接口，当前未启用）。
func classifyIntent(prompt string) intentTier {
	trimmed := strings.TrimSpace(prompt)
	runes := []rune(trimmed)
	lower := strings.ToLower(trimmed)

	// ── 1. Greeting: 短文本 + 匹配问候词 ──
	if len(runes) <= 10 {
		greetings := []string{
			"你好", "hi", "hello", "嗨", "hey",
			"早上好", "下午好", "晚上好", "早安", "晚安",
			"good morning", "good afternoon", "good evening",
			"哈喽", "嘿", "在吗", "在不在", "nihao",
		}
		for _, g := range greetings {
			if lower == g {
				return intentGreeting
			}
		}
	}

	// ── 2. Question: 疑问标记 + 无祈使前缀 + 无动作动词 ──
	// 核心设计: 先检测提问再检测任务，避免 "代码是谁写的？" 被误判为 task_write
	// Bug#11 修复: 动作动词兜底 — 即使句式是疑问且无祈使前缀，包含诊断/执行类动词也应归类为任务
	if isInterrogative(lower) && !hasImperativePrefix(lower) {
		if !containsActionVerb(lower) {
			return intentQuestion
		}
		// 包含动作动词 → 跳过 question，继续走关键词匹配
	}

	// ── 3. Task Delete: 破坏性操作关键词 ──
	// 高安全优先级，确保删除类意图被正确捕获
	if containsAnyKeyword(lower, deleteKeywords) {
		return intentTaskDelete
	}

	// ── 4. Task Multimodal: 视觉/浏览器交互关键词 ──
	if containsAnyKeyword(lower, multimodalKeywords) {
		return intentTaskMultimodal
	}

	// ── 5. Task Write: 创建/修改关键词 ──
	if containsAnyKeyword(lower, writeKeywords) {
		return intentTaskWrite
	}

	// ── 6. Default: Task Light ──
	// 比旧架构的 intentChat 更安全 — 提供 bash + 读取工具，不会陷入死循环
	return intentTaskLight
}

// ---------- 关键词集 ----------

// P3-6: D4 derivation — intent classification keywords sourced from capability tree
// IntentKeywords + IntentPriority fields, replacing hand-written keyword arrays.
// Priority mapping: 30→task_delete, 20→task_multimodal, 10→task_write.
// Tools with IntentPriority=0 (e.g. send_media) don't contribute to classification
// — their MinTier determines tool availability directly.
var deleteKeywords = capabilities.TreeClassificationKeywords("task_delete")
var multimodalKeywords = capabilities.TreeClassificationKeywords("task_multimodal")
var writeKeywords = capabilities.TreeClassificationKeywords("task_write")

// ---------- 疑问 / 祈使检测 ----------

// isInterrogative 检测是否为疑问句。
func isInterrogative(lower string) bool {
	// 末尾疑问标记
	if strings.HasSuffix(lower, "？") || strings.HasSuffix(lower, "?") {
		return true
	}

	// 中文疑问助词
	interrogativeParticles := []string{
		"吗", "呢", "么",
		"是不是", "能不能", "能否", "有没有",
		"什么", "怎么", "为什么", "哪个", "哪里", "几个", "多少",
	}
	for _, p := range interrogativeParticles {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// 英文疑问词开头
	englishQuestionStarts := []string{
		"what ", "how ", "why ", "where ", "when ", "which ",
		"is ", "are ", "was ", "were ", "did ", "does ", "do ",
		"can ", "could ", "would ", "should ", "will ",
	}
	for _, qs := range englishQuestionStarts {
		if strings.HasPrefix(lower, qs) {
			return true
		}
	}

	return false
}

// hasImperativePrefix 检测是否有祈使/命令标记。
// 祈使标记表示用户在下达指令（非提问），即使句子包含疑问标记也应归类为任务。
//
// Bug#4 修复: "帮我"等强任务标记不仅检查前缀，还检查嵌入位置。
// 例: "嗨，你帮我看下系统资源？" — "帮我"不在开头但明确表示任务委托。
func hasImperativePrefix(lower string) bool {
	// 1. 前缀检测（"给我"/"来" 太通用，仅做前缀匹配）
	prefixOnly := []string{
		"给我", "来",
	}
	for _, p := range prefixOnly {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}

	// 2. 强任务标记: 无论出现在前缀还是句中都表示任务委托
	// "帮我X" = "替我做X"，"帮忙X" = "请做X"，"请帮" = "please help"，"替我" = "on my behalf"
	// Bug#11 修复: 增加中文礼貌祈使句 + 英文对应模式
	strongMarkers := []string{
		"帮我", "帮忙", "麻烦", "请帮", "替我",
		// 礼貌祈使: "你能X吗"/"能不能X"/"可以帮X"/"可以做X"/"能否X"
		"你能", "能不能", "可以帮", "可以做", "能否",
		// 英文礼貌祈使
		"can you", "could you", "would you",
	}
	for _, m := range strongMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}

	// 3. "请" 前缀特殊处理: "请问" 是疑问，"请完成/请执行/请删除" 是祈使
	if strings.HasPrefix(lower, "请") && !strings.HasPrefix(lower, "请问") {
		return true
	}

	return false
}

// ---------- 工具过滤 ----------

// filterToolsByIntent 按意图分级过滤工具。
// P1-6/P1-7: D3 derivation — allowlists from capability tree instead of hand-written map.
// Dynamic prefix tools (argus_/remote_/mcp_) handled via tree.MatchesDynamicGroup().
//
// | Tier            | 工具数 | 策略                    |
// |-----------------|--------|------------------------|
// | greeting        | 0      | 纯文字回复              |
// | question        | 3-5    | 搜索 + 技能查找          |
// | task_light      | 6-9    | 读取 + bash + 搜索       |
// | task_write      | 8-12   | + 写入 + coder 委托 + 截屏 |
// | task_delete     | 4-6    | bash(带审批) + 读取      |
// | task_multimodal | 全部    | 含 argus/browser        |
func filterToolsByIntent(tools []llmclient.ToolDef, tier intentTier) []llmclient.ToolDef {
	if tier == intentGreeting {
		return nil
	}
	if tier == intentTaskMultimodal {
		// task_multimodal 移除直接 argus_* 工具（除 capture_screen），
		// 改由 spawn_argus_agent 子智能体提供完整视觉操作能力。
		filtered := make([]llmclient.ToolDef, 0, len(tools))
		for _, t := range tools {
			if strings.HasPrefix(t.Name, "argus_") && t.Name != "argus_capture_screen" {
				continue
			}
			filtered = append(filtered, t)
		}
		return filtered
	}

	// D3: derive allowlist from capability tree
	tree := capabilities.DefaultTree()
	allowed := tree.AllowlistForTier(string(tier))

	filtered := make([]llmclient.ToolDef, 0, len(allowed))
	for _, t := range tools {
		name := t.Name

		// Dynamic prefix tools: check against tree dynamic groups
		if g := tree.MatchesDynamicGroup(name); g != nil {
			// Dynamic group has its own MinTier in the tree.
			// Check if the dynamic group's MinTier allows this tier.
			if g.Routing != nil && g.Routing.MinTier != "" {
				groupIdx := capabilities.TierIndex(g.Routing.MinTier)
				tierIdx := capabilities.TierIndex(string(tier))
				if groupIdx >= 0 && tierIdx >= 0 && tierIdx >= groupIdx {
					// Special case: argus_ at task_write only allows capture_screen
					if g.Runtime != nil && g.Runtime.NamePrefix == "argus_" && tier == intentTaskWrite {
						if name == "argus_capture_screen" {
							filtered = append(filtered, t)
						}
						continue
					}
					filtered = append(filtered, t)
				}
			}
			continue
		}

		// Static tools: use tree-derived allowlist
		if allowed[name] {
			filtered = append(filtered, t)
		}
	}

	return filtered
}

// P1-7: tierToolAllowlist deleted. Allowlists now derived from capability tree (D3).

// ---------- 历史裁剪 ----------

// trimHistoryByIntent 按意图裁剪历史消息，减少不必要的 token 消耗。
//
//   - greeting: 不加载历史（boot brief 在系统提示中提供上下文感知）
//   - question: 最近 4 条消息（2 轮对话，足够回溯上下文回答问题）
//   - task_delete: 全量（需要完整路径历史做安全确认）
//   - task_*: 不裁剪（保持完整上下文供工具决策）
func trimHistoryByIntent(messages []llmclient.ChatMessage, tier intentTier) []llmclient.ChatMessage {
	switch tier {
	case intentGreeting:
		return nil
	case intentQuestion:
		// 比旧 chat 的 2 条多一些（4 条 = 2 轮对话），帮助回答回顾性问题
		if len(messages) > 4 {
			return messages[len(messages)-4:]
		}
		return messages
	default:
		return messages
	}
}

// ---------- 行为指引 ----------

// intentGuidanceText 返回当前意图层级的行为指引文本。
// 注入到系统提示词中，引导 LLM 在特定意图下采取最优策略。
// 这是提示词系统维度与意图维度的联动点。
// P1-8: D8 derivation — group context derived from tree GroupIntro; behavioral rules remain hand-written.
func intentGuidanceText(tier intentTier) string {
	base := intentGuidanceBase(tier)
	if base == "" {
		return ""
	}
	// Append tree-derived group context (P1-8)
	summaries := capabilities.TreeIntentGroupSummaries()
	if gs, ok := summaries[string(tier)]; ok && gs != "" {
		base += "\n- Available tool groups: " + gs
	}
	return base
}

// intentGuidanceBase returns the hand-written behavioral guidance per tier.
func intentGuidanceBase(tier intentTier) string {
	switch tier {
	case intentGreeting:
		return "" // 无需指引
	case intentQuestion:
		return `## Intent Guidance (Question Mode)
This is an informational question, NOT a task execution request.
- Answer from conversation history and memory FIRST — do not call tools to verify known information.
- If the answer exists in prior messages, respond directly without tool calls.
- Only use search tools if the history is truly insufficient.
- Keep responses concise (under 200 chars for simple factual questions).`
	case intentTaskLight:
		return `## Intent Guidance (Light Task Mode)
This is a read/check operation.
- For system status checks (memory, CPU, disk, processes): use bash directly with standard commands (top, vm_stat, df, ps, sysctl, etc.). Do NOT search for skills first.
- Use known file paths from history — avoid broad searches like 'find ~'.
- Prefer read_file/list_dir for direct access over bash for file operations.
- For sending/sharing an existing local file or image whose path/name is already known, use 'send_media' directly. Do NOT delegate to 'spawn_argus_agent' just to transmit an already-known file.
- If a file must be located first, do the minimal file discovery needed, then call 'send_media' once the path is known.
- If the user is checking status, provide a brief summary.
- When the user's request matches a known skill topic (e.g., system diagnostics, deployment, debugging, monitoring), use search_skills first to leverage specialized knowledge and best practices.
- For common system commands (ls, top, df, cat, grep, etc.), use tools directly without searching skills.
- NEVER execute system diagnostics, service start/stop/restart, or environment repair commands that were NOT explicitly requested by the user.`
	case intentTaskWrite:
		return `## Intent Guidance (Write Task Mode)
This is a creation/modification task.
- Simple edits (single-file, <50 lines, clear target): execute directly with write_file/bash tools.
- Complex/multi-file coding tasks: delegate to spawn_coder_agent (Open Coder).
- Media operations (content creation, publishing, trending analysis): delegate to spawn_media_agent.
- After creating visual artifacts (HTML/web), use browser screenshot for verification.
- Combine related write operations into fewer steps.
- Complex tasks may go through plan confirmation — wait for user approval before executing.`
	case intentTaskDelete:
		return `## Intent Guidance (Delete Task Mode)
This is a destructive operation requiring caution.
- Use file paths from conversation history — NEVER use 'find ~' or broad searches.
  The target path should already be known from prior context.
- Combine deletion steps into one command (e.g., 'rm file && rmdir dir').
- Deletion commands will trigger security approval — this is expected behavior.
- After deletion, a brief confirmation is sufficient (no need for ls verification).
- Destructive operations require plan confirmation — wait for user approval before executing.`
	case intentTaskMultimodal:
		return `## Intent Guidance (Multimodal Task Mode)
This task involves visual or browser interaction.
- For opening web pages / URLs: ALWAYS use 'browser' tool with 'navigate' action. NEVER use bash 'open' command for URLs.
- For web page screenshots: use 'browser' tool with 'screenshot' action (NOT argus_capture_screen).
- For web page interaction (click, type, scroll): use 'browser' tool with CSS selectors or ARIA refs.
- For complex multi-step web tasks: use 'browser' tool with 'ai_browse' action.
- For sending an existing local file or screenshot to a channel, use 'send_media'. Only use 'spawn_argus_agent' if the file must first be discovered or produced through native desktop interaction.
- For desktop application interaction (native apps, not web): use 'spawn_argus_agent' to delegate to 灵瞳 sub-agent.
- For full desktop screenshots (not web): use 'argus_capture_screen' directly.
- Rule: if the target is a URL or web page, use 'browser'. Only use argus for native desktop apps.
- If the browser tool returns 'not available', tell the user to visit the browser extension setup guide at /browser-extension/ on the Gateway to install the Chrome extension. Include the full URL in your reply.
- Complex multimodal tasks may go through plan confirmation — wait for user approval before executing.`
	default:
		return ""
	}
}

// ---------- 方案确认门控 ----------

// needsPlanConfirmation 判断当前意图层级是否需要方案确认门控。
// task_write / task_delete / task_multimodal 需要用户确认方案后才执行。
// greeting / question / task_light 直接处理，不走门控。
func needsPlanConfirmation(tier intentTier) bool {
	switch tier {
	case intentTaskWrite, intentTaskDelete, intentTaskMultimodal:
		return true
	default:
		return false
	}
}

// ---------- 辅助函数 ----------

// ---------- 动作动词检测（Bug#11 修复） ----------

// actionVerbs 诊断/执行类动作动词 — 表示用户期望 agent 采取行动，
// 即使句式是疑问也不应归类为 question。
var actionVerbs = []string{
	// 诊断类
	"排查", "诊断", "调试", "检查", "分析", "定位", "排错", "修复",
	"troubleshoot", "investigate", "diagnose", "debug",
	// 执行类
	"执行", "运行", "启动", "停止", "重启", "部署", "安装", "卸载",
	"execute", "restart",
}

// containsActionVerb 检测消息是否包含动作动词。
func containsActionVerb(lower string) bool {
	return containsAnyKeyword(lower, actionVerbs)
}

// containsAnyKeyword 检查 lower 是否包含关键词列表中的任何一个。
func containsAnyKeyword(lower string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
