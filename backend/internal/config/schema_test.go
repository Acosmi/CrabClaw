package config

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestValidateOpenAcosmiConfig_Empty(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	errs := ValidateOpenAcosmiConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty config, got %d: %v", len(errs), errs)
	}
}

func TestValidateOpenAcosmiConfig_BrowserProfile(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Browser: &types.BrowserConfig{
			Profiles: map[string]*types.BrowserProfileConfig{
				"bad":       {Color: "#FF0000"}, // 缺少 cdpPort 和 cdpUrl
				"good_port": {Color: "#00FF00", CdpPort: intPtr(9222)},
				"good_url":  {Color: "#0000FF", CdpURL: "ws://localhost:9222"},
			},
		},
	}

	errs := ValidateOpenAcosmiConfig(cfg)
	found := false
	for _, e := range errs {
		if e.Field == "browser.profiles.bad" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected validation error for profile 'bad' missing cdpPort/cdpUrl")
	}
}

func TestValidateOpenAcosmiConfig_AgentToolsMutex(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{
					ID: "agent1",
					Tools: &types.AgentToolsConfig{
						Allow:     []string{"bash"},
						AlsoAllow: []string{"node"},
					},
				},
			},
		},
	}

	errs := ValidateOpenAcosmiConfig(cfg)
	found := false
	for _, e := range errs {
		if e.Tag == "mutex" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected mutex error for allow+alsoAllow on agent1")
	}
}

func TestNewSchemaResponse(t *testing.T) {
	resp := NewSchemaResponse("1.0.0")
	if resp.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", resp.Version)
	}
	if resp.GeneratedAt == "" {
		t.Error("expected non-empty generatedAt")
	}
	if len(resp.UIHints) == 0 {
		t.Error("expected non-empty UIHints")
	}
	// Schema must be non-nil (was the root cause of "Schema unavailable. Use Raw.")
	if resp.Schema == nil {
		t.Fatal("expected non-nil Schema")
	}
	schemaMap, ok := resp.Schema.(map[string]interface{})
	if !ok {
		t.Fatal("expected Schema to be map[string]interface{}")
	}
	// Must have type: object and properties
	if schemaMap["type"] != "object" {
		t.Errorf("expected schema type 'object', got %v", schemaMap["type"])
	}
	props, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected schema properties to be a map")
	}
	// Must include channels sub-property
	if _, ok := props["channels"]; !ok {
		t.Error("expected schema properties to include 'channels'")
	}
}

func TestGenerateConfigSchema(t *testing.T) {
	schema := generateConfigSchema()
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
	if schema["title"] != "Crab Claw Config" {
		t.Errorf("expected title 'Crab Claw Config', got %v", schema["title"])
	}
	if schema["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Errorf("expected $schema draft-07, got %v", schema["$schema"])
	}
	// Verify key top-level properties exist
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	required := []string{"channels", "agents", "tools", "gateway", "models"}
	for _, key := range required {
		if _, exists := props[key]; !exists {
			t.Errorf("expected top-level property %q", key)
		}
	}
	// Verify channels is an object with sub-properties
	channelsProp, ok := props["channels"].(map[string]interface{})
	if !ok {
		t.Fatal("expected channels to be a map")
	}
	if channelsProp["type"] != "object" {
		t.Errorf("expected channels type 'object', got %v", channelsProp["type"])
	}
	channelProps, ok := channelsProp["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected channels properties map")
	}
	// Chinese channels must be present
	for _, ch := range []string{"feishu", "dingtalk", "wecom"} {
		if _, exists := channelProps[ch]; !exists {
			t.Errorf("expected channel %q in schema", ch)
		}
	}
}

func intPtr(v int) *int {
	return &v
}

// TestSensitiveFieldsMarked 验证 sensitive 字段被正确标记
// 回归测试 BUG-3: sensitive 字段标记因 key 不存在于 hints map 而不生效
func TestSensitiveFieldsMarked(t *testing.T) {
	hints := buildUIHints()
	for _, key := range []string{"gateway.remote.token", "gateway.remote.password"} {
		h, exists := hints[key]
		if !exists {
			t.Fatalf("expected hint for %q to exist", key)
		}
		if h.Sensitive == nil || !*h.Sensitive {
			t.Fatalf("expected %q to be marked sensitive", key)
		}
	}
}

// TestBuildUIHints_Coverage 验证 hints 数量足够（≥250，防止数据丢失）
func TestBuildUIHints_Coverage(t *testing.T) {
	hints := buildUIHints()
	if len(hints) < 250 {
		t.Errorf("expected at least 250 hints, got %d", len(hints))
	}
}

// TestGroupHints 验证 group labels 和 order 正确填充
func TestGroupHints(t *testing.T) {
	hints := buildUIHints()
	groups := []string{"agents", "channels", "gateway", "models", "session", "tools"}
	for _, g := range groups {
		h, exists := hints[g]
		if !exists {
			t.Errorf("expected group hint for %q", g)
			continue
		}
		if h.Label == "" {
			t.Errorf("expected label for group %q", g)
		}
		if h.Order == nil {
			t.Errorf("expected order for group %q", g)
		}
	}
}

// TestIsSensitivePath 验证 sensitive 路径正则匹配
func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"gateway.remote.token", true},
		{"gateway.remote.password", true},
		{"models.providers.apiKey", true},
		{"models.providers.api_key", true},
		{"auth.secret", true},
		{"agents.defaults.model.primary", false},
		{"gateway.port", false},
	}
	for _, tt := range tests {
		got := isSensitivePath(tt.path)
		if got != tt.want {
			t.Errorf("isSensitivePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// ----- H7-3: Semantic Validation Tests -----

func TestValidateIdentityAvatar_Valid(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "a1", Identity: &types.IdentityConfig{Avatar: "https://example.com/avatar.png"}},
				{ID: "a2", Identity: &types.IdentityConfig{Avatar: "data:image/png;base64,abc"}},
				{ID: "a3", Identity: &types.IdentityConfig{Avatar: "images/bot.png"}},
				{ID: "a4", Identity: &types.IdentityConfig{Avatar: ""}}, // empty is ok
			},
		},
	}
	errs := ValidateOpenAcosmiConfig(cfg)
	for _, e := range errs {
		if e.Tag == "avatar_path" {
			t.Errorf("unexpected avatar error: %v", e)
		}
	}
}

func TestValidateIdentityAvatar_Invalid(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "bad1", Identity: &types.IdentityConfig{Avatar: "~/photos/me.png"}},
				{ID: "bad2", Identity: &types.IdentityConfig{Avatar: "ftp://server/avatar.png"}},
			},
		},
	}
	errs := ValidateOpenAcosmiConfig(cfg)
	count := 0
	for _, e := range errs {
		if e.Tag == "avatar_path" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 avatar_path errors, got %d", count)
	}
}

func TestValidateHeartbeatTarget(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Heartbeat: &types.HeartbeatConfig{Target: "last"},
			},
			List: []types.AgentListItemConfig{
				{ID: "a1", Heartbeat: &types.HeartbeatConfig{Target: "telegram"}},
				{ID: "a2", Heartbeat: &types.HeartbeatConfig{Target: "none"}},
			},
		},
	}
	errs := ValidateOpenAcosmiConfig(cfg)
	for _, e := range errs {
		if e.Tag == "heartbeat_target" {
			t.Errorf("unexpected heartbeat error: %v", e)
		}
	}

	// Unknown target
	cfg2 := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Heartbeat: &types.HeartbeatConfig{Target: "nonexistent_channel"},
			},
		},
	}
	errs2 := ValidateOpenAcosmiConfig(cfg2)
	found := false
	for _, e := range errs2 {
		if e.Tag == "heartbeat_target" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected heartbeat_target error for unknown channel")
	}
}

func TestValidateAgentDirDuplicates(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "agent1", AgentDir: "/workspace/a"},
				{ID: "agent2", AgentDir: "/workspace/a"}, // duplicate
			},
		},
	}
	errs := ValidateOpenAcosmiConfig(cfg)
	found := false
	for _, e := range errs {
		if e.Tag == "duplicate_dir" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected duplicate_dir error for shared agent directory")
	}
}
