package channels

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- Discord ----------

func TestSetDiscordDmPolicy_Open(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetDiscordDmPolicy(cfg, "open")
	if result.Channels == nil || result.Channels.Discord == nil {
		t.Fatal("discord config should be set")
	}
	if result.Channels.Discord.DM == nil {
		t.Fatal("dm config should be set")
	}
	if result.Channels.Discord.DM.Policy != "open" {
		t.Errorf("expected open, got %s", result.Channels.Discord.DM.Policy)
	}
	// should add wildcard
	found := false
	for _, v := range result.Channels.Discord.DM.AllowFrom {
		if s, ok := v.(string); ok && s == "*" {
			found = true
		}
	}
	if !found {
		t.Error("open policy should add wildcard to allowFrom")
	}
}

func TestSetDiscordDmPolicy_Pairing(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetDiscordDmPolicy(cfg, "pairing")
	if result.Channels.Discord.DM.Policy != "pairing" {
		t.Errorf("expected pairing, got %s", result.Channels.Discord.DM.Policy)
	}
}

func TestSetDiscordGroupPolicy_Default(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetDiscordGroupPolicy(cfg, DefaultAccountID, "open")
	if result.Channels.Discord.GroupPolicy == nil || *result.Channels.Discord.GroupPolicy != "open" {
		t.Errorf("expected open, got %v", result.Channels.Discord.GroupPolicy)
	}
}

func TestSetDiscordGroupPolicy_MultiAccount(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetDiscordGroupPolicy(cfg, "mybot", "allowlist")
	if result.Channels.Discord.Accounts == nil {
		t.Fatal("accounts should be set")
	}
	acct := result.Channels.Discord.Accounts["mybot"]
	if acct == nil {
		t.Fatal("mybot account should exist")
	}
	if acct.GroupPolicy == nil || *acct.GroupPolicy != "allowlist" {
		t.Errorf("expected allowlist, got %v", acct.GroupPolicy)
	}
}

func TestSetDiscordGuildChannelAllowlist(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	entries := []DiscordGuildChannelEntry{
		{GuildKey: "guild1", ChannelKey: "ch1"},
		{GuildKey: "guild1", ChannelKey: "ch2"},
		{GuildKey: "guild2"},
	}
	result := SetDiscordGuildChannelAllowlist(cfg, DefaultAccountID, entries)
	if result.Channels.Discord.Guilds == nil {
		t.Fatal("guilds should be set")
	}
	guild1 := result.Channels.Discord.Guilds["guild1"]
	if guild1 == nil {
		t.Fatal("guild1 should exist")
	}
	if len(guild1.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(guild1.Channels))
	}
	guild2 := result.Channels.Discord.Guilds["guild2"]
	if guild2 == nil {
		t.Fatal("guild2 should exist")
	}
}

func TestSetDiscordAllowFrom(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetDiscordAllowFrom(cfg, []string{"123", "456"})
	if result.Channels.Discord.DM == nil {
		t.Fatal("dm should be set")
	}
	if len(result.Channels.Discord.DM.AllowFrom) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result.Channels.Discord.DM.AllowFrom))
	}
}

func TestDisableDiscord(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Discord: &types.DiscordConfig{},
		},
	}
	result := DisableDiscord(cfg)
	if result.Channels.Discord.Enabled == nil || *result.Channels.Discord.Enabled {
		t.Error("discord should be disabled")
	}
}

func TestDisableDiscord_NilChannels(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := DisableDiscord(cfg) // should not panic
	_ = result
}

func TestParseDiscordAllowFromID(t *testing.T) {
	tests := []struct{ input, want string }{
		{"123456", "123456"},
		{"<@123456>", "123456"},
		{"<@!789>", "789"},
		{"user:999", "999"},
		{"discord:111", "111"},
		{"invalid", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ParseDiscordAllowFromID(tt.input)
		if got != tt.want {
			t.Errorf("ParseDiscordAllowFromID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildDiscordOnboardingStatus(t *testing.T) {
	status := BuildDiscordOnboardingStatus(true)
	if !status.Configured {
		t.Error("should be configured")
	}
	if status.Channel != ChannelDiscord {
		t.Error("wrong channel")
	}
}

// ---------- Slack ----------

func TestSetSlackDmPolicy_Open(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetSlackDmPolicy(cfg, "open")
	if result.Channels.Slack.DM.Policy != "open" {
		t.Errorf("expected open, got %s", result.Channels.Slack.DM.Policy)
	}
}

func TestSetSlackGroupPolicy(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetSlackGroupPolicy(cfg, DefaultAccountID, "open")
	if result.Channels.Slack.GroupPolicy != "open" {
		t.Errorf("expected open, got %s", result.Channels.Slack.GroupPolicy)
	}
}

func TestSetSlackChannelAllowlist(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetSlackChannelAllowlist(cfg, DefaultAccountID, []string{"#general", "#random"})
	if len(result.Channels.Slack.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(result.Channels.Slack.Channels))
	}
}

func TestDisableSlack(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Slack: &types.SlackConfig{},
		},
	}
	result := DisableSlack(cfg)
	if result.Channels.Slack.Enabled == nil || *result.Channels.Slack.Enabled {
		t.Error("slack should be disabled")
	}
}

func TestBuildSlackAppManifest(t *testing.T) {
	manifest := BuildSlackAppManifest("TestBot")
	if manifest == "" {
		t.Error("manifest should not be empty")
	}
	if !contains(manifest, "TestBot") {
		t.Error("manifest should contain bot name")
	}
	if !contains(manifest, "Crab Claw（蟹爪）") {
		t.Error("manifest should contain the new brand description")
	}
}

// ---------- Telegram ----------

func TestSetTelegramDmPolicy(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetTelegramDmPolicy(cfg, "open")
	if result.Channels.Telegram.DmPolicy != "open" {
		t.Errorf("expected open, got %s", result.Channels.Telegram.DmPolicy)
	}
}

func TestSetTelegramGroupPolicy(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetTelegramGroupPolicy(cfg, DefaultAccountID, "allowlist")
	if result.Channels.Telegram.GroupPolicy != "allowlist" {
		t.Errorf("expected allowlist, got %s", result.Channels.Telegram.GroupPolicy)
	}
}

func TestSetTelegramAllowFrom(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetTelegramAllowFrom(cfg, DefaultAccountID, []string{"123", "456"})
	if len(result.Channels.Telegram.AllowFrom) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result.Channels.Telegram.AllowFrom))
	}
}

func TestDisableTelegram(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Telegram: &types.TelegramConfig{},
		},
	}
	result := DisableTelegram(cfg)
	if result.Channels.Telegram.Enabled == nil || *result.Channels.Telegram.Enabled {
		t.Error("telegram should be disabled")
	}
}

func TestParseTelegramUserID(t *testing.T) {
	tests := []struct{ input, want string }{
		{"123456", "123456"},
		{"@mybot", "mybot"},
		{"user:999", "999"},
		{"telegram:111", "111"},
		{"abc", ""},
		{"mybot_name", "mybot_name"},
	}
	for _, tt := range tests {
		got := ParseTelegramUserID(tt.input)
		if got != tt.want {
			t.Errorf("ParseTelegramUserID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------- WhatsApp ----------

func TestSetWhatsAppDmPolicy(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetWhatsAppDmPolicy(cfg, "open")
	if result.Channels.WhatsApp.DmPolicy != "open" {
		t.Errorf("expected open, got %s", result.Channels.WhatsApp.DmPolicy)
	}
}

func TestSetWhatsAppSelfChatMode(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetWhatsAppSelfChatMode(cfg, true)
	if result.Channels.WhatsApp.SelfChatMode == nil || !*result.Channels.WhatsApp.SelfChatMode {
		t.Error("self-chat mode should be enabled")
	}
}

func TestDisableWhatsApp(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			WhatsApp: &types.WhatsAppConfig{},
		},
	}
	result := DisableWhatsApp(cfg)
	if result.Channels.WhatsApp.Enabled == nil || *result.Channels.WhatsApp.Enabled {
		t.Error("whatsapp should be disabled")
	}
}

func TestNormalizeWhatsAppPhone(t *testing.T) {
	tests := []struct{ input, want string }{
		{"+15555550123", "+15555550123"},
		{"15555550123", "+15555550123"},
		{"+1 (555) 555-0123", "+15555550123"},
		{"", ""},
		{"abc", ""},
	}
	for _, tt := range tests {
		got := NormalizeWhatsAppPhone(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeWhatsAppPhone(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------- Signal ----------

func TestSetSignalDmPolicy(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetSignalDmPolicy(cfg, "pairing")
	if result.Channels.Signal.DmPolicy != "pairing" {
		t.Errorf("expected pairing, got %s", result.Channels.Signal.DmPolicy)
	}
}

func TestSetSignalAllowFrom(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetSignalAllowFrom(cfg, DefaultAccountID, []string{"+15555550123", "uuid:abc"})
	if len(result.Channels.Signal.AllowFrom) != 2 {
		t.Errorf("expected 2, got %d", len(result.Channels.Signal.AllowFrom))
	}
}

func TestDisableSignal(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Signal: &types.SignalConfig{},
		},
	}
	result := DisableSignal(cfg)
	if result.Channels.Signal.Enabled == nil || *result.Channels.Signal.Enabled {
		t.Error("signal should be disabled")
	}
}

func TestIsUUIDLike(t *testing.T) {
	if !IsUUIDLike("123e4567-e89b-12d3-a456-426614174000") {
		t.Error("should be UUID-like")
	}
	if IsUUIDLike("not-a-uuid") {
		t.Error("should not be UUID-like")
	}
}

func TestNormalizeE164(t *testing.T) {
	tests := []struct{ input, want string }{
		{"+15555550123", "+15555550123"},
		{"15555550123", "+15555550123"},
		{"+1 555 555 0123", "+15555550123"},
		{"", ""},
		{"short", ""},
	}
	for _, tt := range tests {
		got := NormalizeE164(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeE164(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------- iMessage ----------

func TestSetIMessageDmPolicy(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetIMessageDmPolicy(cfg, "allowlist")
	if result.Channels.IMessage.DmPolicy != "allowlist" {
		t.Errorf("expected allowlist, got %s", result.Channels.IMessage.DmPolicy)
	}
}

func TestSetIMessageAllowFrom(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := SetIMessageAllowFrom(cfg, DefaultAccountID, []string{"+15555550123", "user@example.com"})
	if len(result.Channels.IMessage.AllowFrom) != 2 {
		t.Errorf("expected 2, got %d", len(result.Channels.IMessage.AllowFrom))
	}
}

func TestDisableIMessage(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			IMessage: &types.IMessageConfig{},
		},
	}
	result := DisableIMessage(cfg)
	if result.Channels.IMessage.Enabled == nil || *result.Channels.IMessage.Enabled {
		t.Error("imessage should be disabled")
	}
}

func TestValidateIMessageAllowFromEntry(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"*", true},
		{"+15555550123", true},
		{"user@example.com", true},
		{"chat_id:123", true},
		{"chat_id:abc", false},
		{"chat_guid:some-guid", true},
		{"chat_guid:", false},
	}
	for _, tt := range tests {
		errMsg := ValidateIMessageAllowFromEntry(tt.input)
		if tt.valid && errMsg != "" {
			t.Errorf("ValidateIMessageAllowFromEntry(%q) should be valid, got: %s", tt.input, errMsg)
		}
		if !tt.valid && errMsg == "" {
			t.Errorf("ValidateIMessageAllowFromEntry(%q) should be invalid", tt.input)
		}
	}
}

// ---------- Channel Access ----------

func TestUniqueStrings(t *testing.T) {
	result := UniqueStrings([]string{"a", "b", "a", "c", "b", " d "})
	if len(result) != 4 {
		t.Errorf("expected 4, got %d", len(result))
	}
}

func TestUniqueStrings_Empty(t *testing.T) {
	result := UniqueStrings(nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

// ---------- internal helpers ----------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
