package runner

// intent_analysis.go — Phase 5: IntentAnalysis 中间表示
//
// 桥接 classifyIntent() → IntentAnalysis → GeneratePlanSteps() 的完整链路。
// IntentAnalysis 是 PlanSteps 自动生成的唯一输入。
//
// 架构设计: docs/codex/2026-03-09-能力树与自治能力管理系统架构设计-v2.md §4
// Tracking:  docs/claude/tracking/tracking-2026-03-09-capability-tree-implementation.md Phase 5

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
)

// ---------- 包级正则（编译一次，复用多次） ----------

var (
	// 绝对文件路径（Unix 风格）
	reAbsFilePath = regexp.MustCompile(`(?:^|\s)((?:/[^\s/\x{4e00}-\x{9fff}]+)+\.[a-zA-Z0-9]{1,10})(?:\s|$|[，。？！、])`)
	// URL (http/https)
	reURL = regexp.MustCompile(`https?://[^\s，。？！、\x{4e00}-\x{9fff}]+`)
	// 自然语言中的相对文件引用（"桌面上的 logo.png"）
	reRelFile = regexp.MustCompile(`(?:桌面|下载|文档|Documents|Desktop|Downloads)(?:上|里|中|下)?的?\s*([a-zA-Z0-9_\-]+\.[a-zA-Z0-9]{1,10})`)
	// 独立文件名引用（filename.ext 无路径前缀）
	reBareFile = regexp.MustCompile(`\b([a-zA-Z0-9_\-]+\.(?:png|jpg|jpeg|gif|pdf|txt|doc|docx|xls|xlsx|csv|zip|tar|gz|mp3|mp4|mov|avi|html|css|js|go|py|rs|ts|md))\b`)
)

// ---------- P5-1: IntentAnalysis + IntentAction + IntentTarget 结构体 ----------

// IntentAnalysis 是 classifyIntent 的增强返回值。
// 它是 PlanSteps 自动生成的唯一输入。
type IntentAnalysis struct {
	Tier            intentTier     `json:"tier"`
	RequiredActions []IntentAction `json:"required_actions"`
	Targets         []IntentTarget `json:"targets"`
	RiskHints       []string       `json:"risk_hints"`
}

// IntentAction 是一个抽象动作，不直接对应工具名。
type IntentAction struct {
	Action      string `json:"action"`      // "send_file"/"find_file"/"write_code"/"browse_web"/"delete_file"/"read_file"
	Description string `json:"description"` // 人类可读描述: "发送已知本地文件到当前频道"
	ToolHint    string `json:"tool_hint"`   // 建议工具: "send_media"（由树查询得出）
	NeedsAuth   bool   `json:"needs_auth"`  // 是否需要授权
}

// IntentTarget 是动作涉及的资源。
type IntentTarget struct {
	Kind  string `json:"kind"`  // "file"/"url"/"channel"/"command"
	Value string `json:"value"` // "/Users/x/Desktop/logo.png"
	Known bool   `json:"known"` // true = 路径已知, false = 需要发现
}

// ---------- P5-2: extractTargets — 从 prompt 提取文件路径/URL/频道 ----------

// extractTargets 从用户 prompt 中提取涉及的资源目标。
// 使用正则匹配文件路径、URL 和频道标识。
// Known=true 表示路径是明确的绝对路径，Known=false 表示需要发现。
func extractTargets(prompt string) []IntentTarget {
	var targets []IntentTarget
	seen := make(map[string]bool) // 去重

	// 1. 提取绝对文件路径（Unix 风格）
	for _, match := range reAbsFilePath.FindAllStringSubmatch(prompt, -1) {
		p := strings.TrimSpace(match[1])
		if !seen[p] {
			seen[p] = true
			targets = append(targets, IntentTarget{
				Kind:  "file",
				Value: p,
				Known: filepath.IsAbs(p),
			})
		}
	}

	// 2. 提取 URL (http/https)
	for _, u := range reURL.FindAllString(prompt, -1) {
		if !seen[u] {
			seen[u] = true
			targets = append(targets, IntentTarget{
				Kind:  "url",
				Value: u,
				Known: true,
			})
		}
	}

	// 3. 提取自然语言中的相对文件引用（"桌面上的 logo.png"）
	for _, match := range reRelFile.FindAllStringSubmatch(prompt, -1) {
		fileName := match[1]
		if !seen[fileName] {
			seen[fileName] = true
			targets = append(targets, IntentTarget{
				Kind:  "file",
				Value: fileName,
				Known: false, // 需要发现实际路径
			})
		}
	}

	// 4. 提取独立文件名引用（无路径前缀，仅当上面未找到文件时）
	if len(targets) == 0 {
		for _, match := range reBareFile.FindAllStringSubmatch(prompt, -1) {
			fileName := match[1]
			if !seen[fileName] {
				seen[fileName] = true
				targets = append(targets, IntentTarget{
					Kind:  "file",
					Value: fileName,
					Known: false,
				})
			}
		}
	}

	return targets
}

// ---------- P5-3: inferActions — 从 tier + targets + 树推导抽象动作序列 ----------

// inferActions 从意图分级、提取的目标和能力树推导抽象动作序列。
// 返回的动作序列按执行顺序排列。
func inferActions(tier intentTier, targets []IntentTarget, prompt string, tree *capabilities.CapabilityTree) []IntentAction {
	lower := strings.ToLower(prompt)
	var actions []IntentAction

	switch tier {
	case intentGreeting:
		// 问候无需动作
		return nil

	case intentQuestion:
		// 提问 → 搜索/查找
		actions = append(actions, IntentAction{
			Action:      "search_info",
			Description: "搜索相关信息回答问题",
			ToolHint:    lookupToolHint(tree, "search_skills"),
		})

	case intentTaskLight:
		// 轻量任务
		if hasSendIntent(lower) && len(targets) > 0 {
			// 发送文件场景
			for _, t := range targets {
				if t.Kind == "file" && !t.Known {
					actions = append(actions, IntentAction{
						Action:      "find_file",
						Description: "查找文件 " + t.Value,
						ToolHint:    lookupToolHint(tree, "bash"),
					})
				}
				actions = append(actions, IntentAction{
					Action:      "send_file",
					Description: "发送文件 " + t.Value,
					ToolHint:    lookupToolHint(tree, "send_media"),
					NeedsAuth:   needsAuthForTool(tree, "send_media"),
				})
			}
		} else {
			actions = append(actions, IntentAction{
				Action:      "read_info",
				Description: "读取信息或检查状态",
				ToolHint:    lookupToolHint(tree, "bash"),
			})
		}

	case intentTaskWrite:
		// 写入任务
		if hasSendIntent(lower) && len(targets) > 0 {
			for _, t := range targets {
				if t.Kind == "file" && !t.Known {
					actions = append(actions, IntentAction{
						Action:      "find_file",
						Description: "查找文件 " + t.Value,
						ToolHint:    lookupToolHint(tree, "bash"),
					})
				}
				actions = append(actions, IntentAction{
					Action:      "send_file",
					Description: "发送文件 " + t.Value,
					ToolHint:    lookupToolHint(tree, "send_media"),
					NeedsAuth:   needsAuthForTool(tree, "send_media"),
				})
			}
		} else if hasBrowseIntent(lower) {
			for _, t := range targets {
				if t.Kind == "url" {
					actions = append(actions, IntentAction{
						Action:      "browse_web",
						Description: "浏览网页 " + t.Value,
						ToolHint:    lookupToolHint(tree, "browser"),
						NeedsAuth:   needsAuthForTool(tree, "browser"),
					})
				}
			}
			if len(actions) == 0 {
				actions = append(actions, IntentAction{
					Action:      "browse_web",
					Description: "浏览网页",
					ToolHint:    lookupToolHint(tree, "browser"),
				})
			}
		} else {
			actions = append(actions, IntentAction{
				Action:      "write_code",
				Description: "创建或修改代码/文件",
				ToolHint:    lookupToolHint(tree, "write_file"),
			})
		}

	case intentTaskDelete:
		// 删除任务
		for _, t := range targets {
			if t.Kind == "file" {
				actions = append(actions, IntentAction{
					Action:      "delete_file",
					Description: "删除文件 " + t.Value,
					ToolHint:    lookupToolHint(tree, "bash"),
					NeedsAuth:   true,
				})
			}
		}
		if len(actions) == 0 {
			actions = append(actions, IntentAction{
				Action:      "delete_resource",
				Description: "执行删除操作",
				ToolHint:    lookupToolHint(tree, "bash"),
				NeedsAuth:   true,
			})
		}

	case intentTaskMultimodal:
		// 多模态任务
		hasURL := false
		for _, t := range targets {
			if t.Kind == "url" {
				hasURL = true
				actions = append(actions, IntentAction{
					Action:      "browse_web",
					Description: "浏览并交互网页 " + t.Value,
					ToolHint:    lookupToolHint(tree, "browser"),
				})
			}
		}
		if !hasURL {
			// 默认桌面视觉操作
			actions = append(actions, IntentAction{
				Action:      "visual_interact",
				Description: "桌面视觉交互操作",
				ToolHint:    lookupToolHint(tree, "spawn_argus_agent"),
			})
		}
	}

	return actions
}

// ---------- P5-4: assessRisks — 从 actions + 树节点权限推导风险提示 ----------

// assessRisks 从动作列表和能力树推导风险提示。
func assessRisks(actions []IntentAction, targets []IntentTarget, tree *capabilities.CapabilityTree) []string {
	var risks []string
	seen := make(map[string]bool)

	addRisk := func(r string) {
		if !seen[r] {
			seen[r] = true
			risks = append(risks, r)
		}
	}

	for _, action := range actions {
		// 从树查询工具节点的权限信息
		node := tree.LookupByToolHint(action.ToolHint)
		if node == nil {
			continue
		}

		// 检查 ScopeCheck
		if node.Perms != nil {
			switch node.Perms.ScopeCheck {
			case "mount_required":
				addRisk("需要挂载访问权限")
			case "scoped":
				addRisk("操作范围受限于工作区")
			}

			// 检查审批类型
			switch node.Perms.ApprovalType {
			case "exec_escalation":
				addRisk("需要执行提权审批")
			case "mount_access":
				addRisk("需要挂载访问审批")
			case "data_export":
				addRisk("需要数据导出审批")
			}
		}

		// NeedsAuth
		if action.NeedsAuth {
			addRisk("需要授权确认")
		}
	}

	// 检查目标资源的路径风险
	for _, t := range targets {
		if t.Kind == "file" && t.Known {
			// 检查是否在 workspace 外
			if strings.HasPrefix(t.Value, "/") {
				// 绝对路径 — 可能在 workspace 外
				addRisk("涉及 workspace 外路径: " + t.Value)
			}
		}
		if t.Kind == "file" && !t.Known {
			addRisk("文件路径不确定，需要先发现: " + t.Value)
		}
	}

	return risks
}

// ---------- P5-5: analyzeIntent — 包装 classifyIntent + extractTargets + inferActions + assessRisks ----------

// analyzeIntent 完整的意图分析入口。
// Phase A: classifyIntent 不变，IntentAnalysis 在外部包装。
func analyzeIntent(prompt string) IntentAnalysis {
	tree := capabilities.DefaultTree()

	tier := classifyIntent(prompt)
	targets := extractTargets(prompt)
	actions := inferActions(tier, targets, prompt, tree)
	risks := assessRisks(actions, targets, tree)

	return IntentAnalysis{
		Tier:            tier,
		RequiredActions: actions,
		Targets:         targets,
		RiskHints:       risks,
	}
}

// ---------- P5-6: GeneratePlanSteps — 从 IntentAnalysis + 树生成 PlanStep 序列 ----------

// GeneratePlanSteps 从 IntentAnalysis 和能力树生成 PlanStep 序列。
// 返回人类可读的步骤描述列表，用于填充 PlanConfirmationRequest.PlanSteps。
func GeneratePlanSteps(analysis IntentAnalysis, tree *capabilities.CapabilityTree) []string {
	if analysis.Tier == intentGreeting || analysis.Tier == intentQuestion {
		return nil // 不需要方案确认
	}

	var steps []string

	for _, action := range analysis.RequiredActions {
		node := tree.LookupByToolHint(action.ToolHint)

		// 前置审批步骤
		if node != nil && node.Perms != nil && node.Perms.ApprovalType != "" && node.Perms.ApprovalType != "none" {
			steps = append(steps, "请求 "+node.Perms.ApprovalType+" 授权 ("+node.Name+")")
		}
		if node != nil && node.Perms != nil && node.Perms.ScopeCheck == "mount_required" && hasKnownAbsoluteFileTarget(analysis.Targets) {
			steps = append(steps, "如目标文件位于当前作用域外，先请求 mount_access 授权 ("+node.Name+")")
		}

		// 主执行步骤
		steps = append(steps, action.Description)
	}

	// 添加风险提示步骤
	for _, risk := range analysis.RiskHints {
		steps = append(steps, "⚠ "+risk)
	}

	return steps
}

func hasKnownAbsoluteFileTarget(targets []IntentTarget) bool {
	for _, target := range targets {
		if target.Kind == "file" && target.Known && strings.HasPrefix(target.Value, "/") {
			return true
		}
	}
	return false
}

// ---------- P5-9: EstimatedScopeFromAnalysis — 从树的 Perms.ScopeCheck 推导 ----------

// EstimatedScopeFromAnalysis 从 IntentAnalysis 和能力树推导预估的操作范围。
func EstimatedScopeFromAnalysis(analysis IntentAnalysis, tree *capabilities.CapabilityTree) []ScopeEntry {
	var scope []ScopeEntry
	seen := make(map[string]bool)

	for _, target := range analysis.Targets {
		if target.Kind == "file" && target.Known {
			dir := filepath.Dir(target.Value)
			if !seen[dir] {
				seen[dir] = true
				// 根据动作类型确定权限（去重 PermWrite）
				perms := []ScopePermission{PermRead}
				needsWrite := false
				for _, action := range analysis.RequiredActions {
					switch action.Action {
					case "write_code", "delete_file", "delete_resource":
						needsWrite = true
					}
				}
				if needsWrite {
					perms = append(perms, PermWrite)
				}
				scope = append(scope, ScopeEntry{
					Path:        dir,
					Permissions: perms,
				})
			}
		}
	}

	// 如果没有明确文件目标，从树的 ScopeCheck 推导
	if len(scope) == 0 {
		for _, action := range analysis.RequiredActions {
			node := tree.LookupByToolHint(action.ToolHint)
			if node != nil && node.Perms != nil {
				switch node.Perms.ScopeCheck {
				case "workspace":
					if !seen["workspace"] {
						seen["workspace"] = true
						scope = append(scope, ScopeEntry{
							Path:        ".",
							Permissions: []ScopePermission{PermRead, PermWrite},
						})
					}
				}
			}
		}
	}

	return scope
}

// ---------- 辅助函数 ----------

// Stage 4 Phase B: send/browse intent keywords derived from capability tree IntentKeywords
// instead of hand-written arrays. ToolIntentKeywords returns combined ZH+EN for a tool.
var sendIntentKeywords = capabilities.ToolIntentKeywords("send_media")
var browseIntentKeywords = capabilities.ToolIntentKeywords("browser")

// hasSendIntent 检测 prompt 是否包含发送/传输意图。
// Phase B: keywords derived from tree node send_media.IntentKeywords.
func hasSendIntent(lower string) bool {
	return containsAnyKeyword(lower, sendIntentKeywords)
}

// hasBrowseIntent 检测 prompt 是否包含浏览网页意图。
// Phase B: keywords derived from tree node browser.IntentKeywords.
func hasBrowseIntent(lower string) bool {
	return containsAnyKeyword(lower, browseIntentKeywords)
}

// lookupToolHint 从树查询工具提示名称。
// 如果树中存在该工具节点则返回其名称，否则返回原始 hint。
func lookupToolHint(tree *capabilities.CapabilityTree, toolName string) string {
	node := tree.LookupByToolHint(toolName)
	if node != nil {
		return node.Name
	}
	return toolName
}

// needsAuthForTool 从树查询工具是否需要授权。
func needsAuthForTool(tree *capabilities.CapabilityTree, toolName string) bool {
	node := tree.LookupByToolHint(toolName)
	if node == nil {
		return false
	}
	if node.Perms == nil {
		return false
	}
	return node.Perms.ApprovalType != "" && node.Perms.ApprovalType != "none"
}
