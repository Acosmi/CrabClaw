package runner

import (
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
)

// ---------- P5-12: IntentAnalysis "桌面上的 logo.png 发给我" 产出含 send_file action ----------

func TestAnalyzeIntent_SendFileDesktop(t *testing.T) {
	analysis := analyzeIntent("桌面上的 logo.png 发给我")

	// 应该分类为 task_light 或 task_write（含发送意图）
	if analysis.Tier != intentTaskLight && analysis.Tier != intentTaskWrite {
		t.Errorf("expected task_light or task_write, got %s", analysis.Tier)
	}

	// 应该有目标
	if len(analysis.Targets) == 0 {
		t.Fatal("expected at least one target, got none")
	}

	// 目标应该包含 logo.png
	foundLogo := false
	for _, target := range analysis.Targets {
		if target.Kind == "file" && target.Value == "logo.png" {
			foundLogo = true
			if target.Known {
				t.Error("logo.png should have Known=false (relative path)")
			}
			break
		}
	}
	if !foundLogo {
		t.Errorf("expected target with value 'logo.png', got targets: %+v", analysis.Targets)
	}

	// 应该有 send_file action
	foundSend := false
	for _, action := range analysis.RequiredActions {
		if action.Action == "send_file" {
			foundSend = true
			if action.ToolHint != "send_media" {
				t.Errorf("send_file action should hint send_media, got %s", action.ToolHint)
			}
			break
		}
	}
	if !foundSend {
		t.Errorf("expected send_file action, got actions: %+v", analysis.RequiredActions)
	}
}

// ---------- P5-14: Known=false target 允许降级（extractTargets 不可靠路径） ----------

func TestExtractTargets_UnknownPathDegrades(t *testing.T) {
	targets := extractTargets("桌面上的 logo.png 发给我")

	if len(targets) == 0 {
		t.Fatal("expected at least one target")
	}

	for _, target := range targets {
		if target.Value == "logo.png" {
			if target.Known {
				t.Error("logo.png from natural language should have Known=false")
			}
			return
		}
	}
	t.Error("expected to find logo.png target")
}

func TestExtractTargets_AbsolutePathKnown(t *testing.T) {
	targets := extractTargets("把 /Users/test/Desktop/logo.png 发给我")

	if len(targets) == 0 {
		t.Fatal("expected at least one target")
	}

	foundAbs := false
	for _, target := range targets {
		if target.Value == "/Users/test/Desktop/logo.png" {
			foundAbs = true
			if !target.Known {
				t.Error("absolute path should have Known=true")
			}
		}
	}
	if !foundAbs {
		t.Errorf("expected absolute path target, got: %+v", targets)
	}
}

func TestExtractTargets_URL(t *testing.T) {
	targets := extractTargets("打开 https://example.com/page 看看")

	foundURL := false
	for _, target := range targets {
		if target.Kind == "url" && target.Value == "https://example.com/page" {
			foundURL = true
			if !target.Known {
				t.Error("URL should have Known=true")
			}
		}
	}
	if !foundURL {
		t.Errorf("expected URL target, got: %+v", targets)
	}
}

func TestExtractTargets_EmptyPrompt(t *testing.T) {
	targets := extractTargets("你好")
	if len(targets) != 0 {
		t.Errorf("greeting should have no targets, got: %+v", targets)
	}
}

// ---------- inferActions tests ----------

func TestInferActions_Greeting(t *testing.T) {
	tree := capabilities.DefaultTree()
	actions := inferActions(intentGreeting, nil, "你好", tree)
	if len(actions) != 0 {
		t.Errorf("greeting should have no actions, got: %+v", actions)
	}
}

func TestInferActions_TaskDeleteHasAuth(t *testing.T) {
	tree := capabilities.DefaultTree()
	targets := []IntentTarget{{Kind: "file", Value: "/tmp/test.txt", Known: true}}
	actions := inferActions(intentTaskDelete, targets, "删除 /tmp/test.txt", tree)

	if len(actions) == 0 {
		t.Fatal("expected at least one action")
	}

	for _, action := range actions {
		if !action.NeedsAuth {
			t.Errorf("delete action should need auth: %+v", action)
		}
	}
}

// ---------- assessRisks tests ----------

func TestAssessRisks_WorkspaceOutsidePath(t *testing.T) {
	tree := capabilities.DefaultTree()
	actions := []IntentAction{{
		Action:   "send_file",
		ToolHint: "send_media",
	}}
	targets := []IntentTarget{{
		Kind:  "file",
		Value: "/Users/test/Desktop/secret.pdf",
		Known: true,
	}}

	risks := assessRisks(actions, targets, tree)

	foundWorkspaceRisk := false
	for _, r := range risks {
		if strContains(r, "workspace 外路径") {
			foundWorkspaceRisk = true
			break
		}
	}
	if !foundWorkspaceRisk {
		t.Errorf("expected workspace outside path risk, got: %v", risks)
	}
}

func TestAssessRisks_UnknownFileRisk(t *testing.T) {
	tree := capabilities.DefaultTree()
	actions := []IntentAction{{Action: "send_file", ToolHint: "send_media"}}
	targets := []IntentTarget{{Kind: "file", Value: "logo.png", Known: false}}

	risks := assessRisks(actions, targets, tree)

	foundDiscoverRisk := false
	for _, r := range risks {
		if strContains(r, "不确定") {
			foundDiscoverRisk = true
			break
		}
	}
	if !foundDiscoverRisk {
		t.Errorf("expected file discovery risk, got: %v", risks)
	}
}

// ---------- GeneratePlanSteps tests ----------

func TestGeneratePlanSteps_TaskWriteNonEmpty(t *testing.T) {
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "write_code", Description: "创建或修改代码/文件", ToolHint: "write_file"},
		},
	}
	tree := capabilities.DefaultTree()
	steps := GeneratePlanSteps(analysis, tree)

	if len(steps) == 0 {
		t.Error("PlanSteps should be non-empty for task_write")
	}
}

func TestGeneratePlanSteps_GreetingEmpty(t *testing.T) {
	analysis := IntentAnalysis{Tier: intentGreeting}
	tree := capabilities.DefaultTree()
	steps := GeneratePlanSteps(analysis, tree)

	if len(steps) != 0 {
		t.Errorf("PlanSteps should be empty for greeting, got: %v", steps)
	}
}

func TestGeneratePlanSteps_QuestionEmpty(t *testing.T) {
	analysis := IntentAnalysis{Tier: intentQuestion}
	tree := capabilities.DefaultTree()
	steps := GeneratePlanSteps(analysis, tree)

	if len(steps) != 0 {
		t.Errorf("PlanSteps should be empty for question, got: %v", steps)
	}
}

func TestGeneratePlanSteps_DeleteIncludesAuth(t *testing.T) {
	analysis := IntentAnalysis{
		Tier: intentTaskDelete,
		RequiredActions: []IntentAction{
			{Action: "delete_file", Description: "删除文件 test.txt", ToolHint: "bash", NeedsAuth: true},
		},
		RiskHints: []string{"需要授权确认"},
	}
	tree := capabilities.DefaultTree()
	steps := GeneratePlanSteps(analysis, tree)

	if len(steps) == 0 {
		t.Fatal("PlanSteps should be non-empty for task_delete")
	}

	// Should contain risk hints
	foundRisk := false
	for _, s := range steps {
		if strContains(s, "⚠") {
			foundRisk = true
			break
		}
	}
	if !foundRisk {
		t.Errorf("expected risk hint in plan steps, got: %v", steps)
	}
}

func TestGeneratePlanSteps_SendFileIncludesConditionalMountStep(t *testing.T) {
	analysis := IntentAnalysis{
		Tier: intentTaskLight,
		RequiredActions: []IntentAction{
			{Action: "send_file", Description: "发送文件 /Users/test/Desktop/logo.png", ToolHint: "send_media"},
		},
		Targets: []IntentTarget{
			{Kind: "file", Value: "/Users/test/Desktop/logo.png", Known: true},
		},
	}
	tree := capabilities.DefaultTree()
	steps := GeneratePlanSteps(analysis, tree)

	foundExport := false
	foundMount := false
	for _, step := range steps {
		if strContains(step, "data_export") {
			foundExport = true
		}
		if strContains(step, "mount_access") {
			foundMount = true
		}
	}
	if !foundExport {
		t.Fatalf("expected data_export step, got: %v", steps)
	}
	if !foundMount {
		t.Fatalf("expected conditional mount_access step, got: %v", steps)
	}
}

// ---------- EstimatedScopeFromAnalysis tests ----------

func TestEstimatedScope_AbsolutePath(t *testing.T) {
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "send_file", ToolHint: "send_media"},
		},
		Targets: []IntentTarget{
			{Kind: "file", Value: "/Users/test/Desktop/logo.png", Known: true},
		},
	}
	tree := capabilities.DefaultTree()
	scope := EstimatedScopeFromAnalysis(analysis, tree)

	if len(scope) == 0 {
		t.Fatal("expected scope entries for absolute path")
	}

	if scope[0].Path != "/Users/test/Desktop" {
		t.Errorf("expected parent directory, got: %s", scope[0].Path)
	}
}

// ---------- analyzeIntent integration test ----------

func TestAnalyzeIntent_FullPipeline(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		wantTier   intentTier
		wantAction string // at least one action with this type
	}{
		{
			name:     "greeting",
			prompt:   "你好",
			wantTier: intentGreeting,
		},
		{
			name:     "question",
			prompt:   "这个项目用什么语言？",
			wantTier: intentQuestion,
		},
		{
			name:       "send file",
			prompt:     "把桌面上的 report.pdf 发给我",
			wantTier:   intentTaskLight,
			wantAction: "send_file",
		},
		{
			name:       "delete",
			prompt:     "删除 /tmp/old.log",
			wantTier:   intentTaskDelete,
			wantAction: "delete_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeIntent(tt.prompt)

			if analysis.Tier != tt.wantTier {
				t.Errorf("tier: want %s, got %s", tt.wantTier, analysis.Tier)
			}

			if tt.wantAction != "" {
				found := false
				for _, a := range analysis.RequiredActions {
					if a.Action == tt.wantAction {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected action %s, got: %+v", tt.wantAction, analysis.RequiredActions)
				}
			}
		})
	}
}

// strContains checks if s contains substr (test helper, avoids name collision with intent_router_test).
func strContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
