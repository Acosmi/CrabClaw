package email

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func newTestConfig(accounts map[string]*types.EmailAccountConfig) *types.OpenAcosmiConfig {
	boolTrue := true
	return &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Email: &types.EmailConfig{
				Enabled:        &boolTrue,
				DefaultAccount: "test",
				Accounts:       accounts,
			},
		},
	}
}

func newTestAccount() *types.EmailAccountConfig {
	boolTrue := true
	return &types.EmailAccountConfig{
		Enabled:  &boolTrue,
		Provider: types.EmailProviderAliyun,
		Name:     "测试账号",
		Address:  "test@company.com",
		Login:    "test@company.com",
		Auth: types.EmailAuthConfig{
			Mode:     types.EmailAuthAppPassword,
			Password: "test-password",
		},
	}
}

func TestEmailPlugin_ID(t *testing.T) {
	plugin := NewEmailPlugin(nil)
	if plugin.ID() != channels.ChannelEmail {
		t.Errorf("ID() = %q, want %q", plugin.ID(), channels.ChannelEmail)
	}
}

func TestEmailPlugin_StartStop(t *testing.T) {
	acct := newTestAccount()
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct})
	plugin := NewEmailPlugin(cfg)

	// Start
	if err := plugin.Start("test"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	runner := plugin.GetRunner("test")
	if runner == nil {
		t.Fatal("Runner should not be nil after Start")
	}
	if runner.AccountID() != "test" {
		t.Errorf("AccountID = %q, want %q", runner.AccountID(), "test")
	}

	// Runner should be in connecting or polling (goroutine scheduling race)
	state := runner.State()
	if state != RunnerStateConnecting && state != RunnerStatePolling {
		t.Errorf("Runner state = %q, want connecting or polling", state)
	}

	// Stop
	if err := plugin.Stop("test"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if plugin.GetRunner("test") != nil {
		t.Error("Runner should be nil after Stop")
	}
}

func TestEmailPlugin_StartDisabledAccount(t *testing.T) {
	boolFalse := false
	acct := newTestAccount()
	acct.Enabled = &boolFalse
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"disabled": acct})
	plugin := NewEmailPlugin(cfg)

	// Start disabled account should succeed but not create runner
	if err := plugin.Start("disabled"); err != nil {
		t.Fatalf("Start disabled account should not error: %v", err)
	}
	if plugin.GetRunner("disabled") != nil {
		t.Error("Disabled account should not have a runner")
	}
}

func TestEmailPlugin_StartMissingAccount(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin := NewEmailPlugin(cfg)

	err := plugin.Start("nonexistent")
	if err == nil {
		t.Fatal("Start nonexistent account should fail")
	}
}

func TestEmailPlugin_MultiAccount(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{
		"ali": newTestAccount(),
		"qq": func() *types.EmailAccountConfig {
			a := newTestAccount()
			a.Provider = types.EmailProviderQQ
			a.Address = "user@qq.com"
			return a
		}(),
		"netease": func() *types.EmailAccountConfig {
			a := newTestAccount()
			a.Provider = types.EmailProviderNetease163
			a.Address = "user@163.com"
			return a
		}(),
	})
	plugin := NewEmailPlugin(cfg)

	// Start all
	for _, id := range []string{"ali", "qq", "netease"} {
		if err := plugin.Start(id); err != nil {
			t.Fatalf("Start %q failed: %v", id, err)
		}
	}

	states := plugin.RunnerStates()
	if len(states) != 3 {
		t.Errorf("RunnerStates count = %d, want 3", len(states))
	}

	// Stop one — others should remain
	if err := plugin.Stop("qq"); err != nil {
		t.Fatalf("Stop qq failed: %v", err)
	}
	states = plugin.RunnerStates()
	if len(states) != 2 {
		t.Errorf("RunnerStates after stop = %d, want 2", len(states))
	}
	if _, ok := states["qq"]; ok {
		t.Error("qq should not be in states after stop")
	}

	// Stop remaining
	for _, id := range []string{"ali", "netease"} {
		if err := plugin.Stop(id); err != nil {
			t.Fatalf("Stop %q failed: %v", id, err)
		}
	}
	if len(plugin.RunnerStates()) != 0 {
		t.Error("All runners should be stopped")
	}
}

func TestEmailPlugin_UpdateConfig(t *testing.T) {
	cfg1 := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin := NewEmailPlugin(cfg1)

	// New config with different default account
	boolTrue := true
	cfg2 := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Email: &types.EmailConfig{
				Enabled:        &boolTrue,
				DefaultAccount: "new-default",
				Accounts: map[string]*types.EmailAccountConfig{
					"test": newTestAccount(),
				},
			},
		},
	}
	plugin.UpdateConfig(cfg2)

	// Verify default account ID resolution changed
	if plugin.resolveDefaultAccountID() != "new-default" {
		t.Errorf("DefaultAccount = %q, want %q", plugin.resolveDefaultAccountID(), "new-default")
	}
}

func TestEmailPlugin_SendMessage_NoRunner(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin := NewEmailPlugin(cfg)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:   "someone@example.com",
		Text: "hello",
	})
	if err == nil {
		t.Fatal("SendMessage without running account should fail")
	}
}

func TestEmailPlugin_SendMessage_EmptyTo(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin := NewEmailPlugin(cfg)
	_ = plugin.Start("test")
	defer func() { _ = plugin.Stop("test") }()

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		AccountID: "test",
		To:        "",
		Text:      "hello",
	})
	if err == nil {
		t.Fatal("SendMessage with empty To should fail")
	}
}

func TestEmailPlugin_SendMessage_EmptyBody(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin := NewEmailPlugin(cfg)
	_ = plugin.Start("test")
	defer func() { _ = plugin.Stop("test") }()

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		AccountID: "test",
		To:        "someone@example.com",
		Text:      "",
	})
	if err == nil {
		t.Fatal("SendMessage with empty body should fail")
	}
}

func TestEmailPlugin_StartAllAccounts(t *testing.T) {
	boolFalse := false
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{
		"active":   newTestAccount(),
		"disabled": func() *types.EmailAccountConfig { a := newTestAccount(); a.Enabled = &boolFalse; return a }(),
	})
	plugin := NewEmailPlugin(cfg)
	plugin.StartAllAccounts()

	states := plugin.RunnerStates()
	// Only active should have a runner
	if len(states) != 1 {
		t.Errorf("RunnerStates = %d, want 1 (only active)", len(states))
	}
	if _, ok := states["active"]; !ok {
		t.Error("active account should be running")
	}

	// Cleanup
	_ = plugin.Stop("active")
}

func TestEmailPlugin_RestartAccount(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin := NewEmailPlugin(cfg)

	// Start
	if err := plugin.Start("test"); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	runner1 := plugin.GetRunner("test")

	// Start again (should replace runner)
	if err := plugin.Start("test"); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}
	runner2 := plugin.GetRunner("test")

	if runner1 == runner2 {
		t.Error("Second Start should create a new runner")
	}

	_ = plugin.Stop("test")
}

// --- Phase 9: 热加载 + 可观测测试 ---

func TestEmailPlugin_UpdateConfig_AddAccount(t *testing.T) {
	cfg1 := newTestConfig(map[string]*types.EmailAccountConfig{"ali": newTestAccount()})
	plugin := NewEmailPlugin(cfg1)
	_ = plugin.Start("ali")

	// New config adds "qq" account
	cfg2 := newTestConfig(map[string]*types.EmailAccountConfig{
		"ali": newTestAccount(),
		"qq": func() *types.EmailAccountConfig {
			a := newTestAccount()
			a.Provider = types.EmailProviderQQ
			a.Address = "user@qq.com"
			return a
		}(),
	})
	plugin.UpdateConfig(cfg2)

	// qq should now be running
	if plugin.GetRunner("qq") == nil {
		t.Error("qq account should be started after hot-reload add")
	}
	// ali should still be running
	if plugin.GetRunner("ali") == nil {
		t.Error("ali account should still be running")
	}

	_ = plugin.Stop("ali")
	_ = plugin.Stop("qq")
}

func TestEmailPlugin_UpdateConfig_RemoveAccount(t *testing.T) {
	cfg1 := newTestConfig(map[string]*types.EmailAccountConfig{
		"ali": newTestAccount(),
		"qq":  func() *types.EmailAccountConfig { a := newTestAccount(); a.Address = "user@qq.com"; return a }(),
	})
	plugin := NewEmailPlugin(cfg1)
	_ = plugin.Start("ali")
	_ = plugin.Start("qq")

	// New config removes "qq"
	cfg2 := newTestConfig(map[string]*types.EmailAccountConfig{"ali": newTestAccount()})
	plugin.UpdateConfig(cfg2)

	// qq should be stopped
	if plugin.GetRunner("qq") != nil {
		t.Error("qq account should be stopped after hot-reload remove")
	}
	// ali should remain
	if plugin.GetRunner("ali") == nil {
		t.Error("ali account should still be running")
	}

	_ = plugin.Stop("ali")
}

func TestEmailPlugin_UpdateConfig_ChangeCredentials(t *testing.T) {
	acct1 := newTestAccount()
	cfg1 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct1})
	plugin := NewEmailPlugin(cfg1)
	_ = plugin.Start("test")
	runner1 := plugin.GetRunner("test")

	// Change password
	acct2 := newTestAccount()
	acct2.Auth.Password = "new-password"
	cfg2 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct2})
	plugin.UpdateConfig(cfg2)

	runner2 := plugin.GetRunner("test")
	if runner1 == runner2 {
		t.Error("Runner should be replaced after credential change")
	}
	if runner2 == nil {
		t.Error("Runner should be restarted after credential change")
	}

	_ = plugin.Stop("test")
}

func TestEmailPlugin_UpdateConfig_NoChange(t *testing.T) {
	acct := newTestAccount()
	cfg1 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct})
	plugin := NewEmailPlugin(cfg1)
	_ = plugin.Start("test")
	runner1 := plugin.GetRunner("test")

	// Same config — runner should NOT be restarted
	cfg2 := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})
	plugin.UpdateConfig(cfg2)

	runner2 := plugin.GetRunner("test")
	if runner1 != runner2 {
		t.Error("Runner should NOT be replaced when config is unchanged")
	}

	_ = plugin.Stop("test")
}

func TestEmailPlugin_UpdateConfig_InvalidType(t *testing.T) {
	plugin := NewEmailPlugin(nil)
	// Should not panic
	plugin.UpdateConfig("invalid")
	plugin.UpdateConfig(42)
	plugin.UpdateConfig(nil)
}

func TestEmailPlugin_HealthSnapshot(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{
		"ali": newTestAccount(),
		"qq": func() *types.EmailAccountConfig {
			a := newTestAccount()
			a.Provider = types.EmailProviderQQ
			a.Address = "user@qq.com"
			return a
		}(),
	})
	plugin := NewEmailPlugin(cfg)
	_ = plugin.Start("ali")
	_ = plugin.Start("qq")

	health := plugin.HealthSnapshot()
	if len(health) != 2 {
		t.Fatalf("HealthSnapshot count = %d, want 2", len(health))
	}

	// Verify fields
	found := map[string]bool{}
	for _, h := range health {
		found[h.AccountID] = true
		if h.State == "" {
			t.Errorf("account %s: State should not be empty", h.AccountID)
		}
	}
	if !found["ali"] || !found["qq"] {
		t.Error("HealthSnapshot should contain both ali and qq")
	}

	_ = plugin.Stop("ali")
	_ = plugin.Stop("qq")
}

func TestEmailPlugin_HealthSnapshot_Empty(t *testing.T) {
	plugin := NewEmailPlugin(nil)
	health := plugin.HealthSnapshot()
	if health != nil && len(health) != 0 {
		t.Errorf("HealthSnapshot for empty plugin should be empty, got %d", len(health))
	}
}

func TestAccountConfigChanged(t *testing.T) {
	acct := newTestAccount()
	cfg1 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct})
	cfg2 := newTestConfig(map[string]*types.EmailAccountConfig{"test": newTestAccount()})

	if accountConfigChanged(cfg1, cfg2, "test") {
		t.Error("Same config should not be detected as changed")
	}

	// Change password
	acct3 := newTestAccount()
	acct3.Auth.Password = "different"
	cfg3 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct3})
	if !accountConfigChanged(cfg1, cfg3, "test") {
		t.Error("Password change should be detected")
	}

	// Change address
	acct4 := newTestAccount()
	acct4.Address = "other@company.com"
	cfg4 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct4})
	if !accountConfigChanged(cfg1, cfg4, "test") {
		t.Error("Address change should be detected")
	}

	// Change provider
	acct5 := newTestAccount()
	acct5.Provider = types.EmailProviderQQ
	cfg5 := newTestConfig(map[string]*types.EmailAccountConfig{"test": acct5})
	if !accountConfigChanged(cfg1, cfg5, "test") {
		t.Error("Provider change should be detected")
	}

	// Missing account
	cfg6 := newTestConfig(map[string]*types.EmailAccountConfig{})
	if !accountConfigChanged(cfg1, cfg6, "test") {
		t.Error("Missing account should be detected as changed")
	}
}

func TestResolveAccountIDs(t *testing.T) {
	cfg := newTestConfig(map[string]*types.EmailAccountConfig{
		"ali": newTestAccount(),
		"qq":  newTestAccount(),
	})
	ids := resolveAccountIDs(cfg)
	if len(ids) != 2 {
		t.Errorf("resolveAccountIDs = %d, want 2", len(ids))
	}
	if _, ok := ids["ali"]; !ok {
		t.Error("should contain ali")
	}
	if _, ok := ids["qq"]; !ok {
		t.Error("should contain qq")
	}

	// Nil config
	nilIDs := resolveAccountIDs(nil)
	if len(nilIDs) != 0 {
		t.Errorf("nil config should return empty, got %d", len(nilIDs))
	}
}
