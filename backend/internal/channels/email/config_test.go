package email

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveEmailAccount(t *testing.T) {
	boolTrue := true
	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Email: &types.EmailConfig{
				Enabled: &boolTrue,
				Accounts: map[string]*types.EmailAccountConfig{
					"ali": {Address: "robot@company.com"},
					"qq":  {Address: "user@qq.com"},
				},
			},
		},
	}

	// Existing account
	acct := ResolveEmailAccount(cfg, "ali")
	if acct == nil {
		t.Fatal("ResolveEmailAccount(ali) should not be nil")
	}
	if acct.Config.Address != "robot@company.com" {
		t.Errorf("Address = %q, want %q", acct.Config.Address, "robot@company.com")
	}

	// Non-existent account
	acct = ResolveEmailAccount(cfg, "nonexistent")
	if acct != nil {
		t.Error("ResolveEmailAccount(nonexistent) should be nil")
	}

	// Nil config
	acct = ResolveEmailAccount(nil, "ali")
	if acct != nil {
		t.Error("ResolveEmailAccount(nil) should be nil")
	}
}

func TestApplyProviderDefaults(t *testing.T) {
	acct := &types.EmailAccountConfig{
		Provider: types.EmailProviderAliyun,
		Address:  "test@company.com",
	}

	ApplyProviderDefaults(acct)

	// IMAP defaults from aliyun preset
	if acct.IMAP == nil {
		t.Fatal("IMAP should not be nil after defaults")
	}
	if acct.IMAP.Host != "imap.qiye.aliyun.com" {
		t.Errorf("IMAP.Host = %q, want %q", acct.IMAP.Host, "imap.qiye.aliyun.com")
	}
	if acct.IMAP.Port != 993 {
		t.Errorf("IMAP.Port = %d, want 993", acct.IMAP.Port)
	}
	if acct.IMAP.Mode != types.EmailIMAPModeAuto {
		t.Errorf("IMAP.Mode = %q, want %q", acct.IMAP.Mode, types.EmailIMAPModeAuto)
	}
	if len(acct.IMAP.Mailboxes) != 1 || acct.IMAP.Mailboxes[0] != "INBOX" {
		t.Errorf("IMAP.Mailboxes = %v, want [INBOX]", acct.IMAP.Mailboxes)
	}
	if acct.IMAP.PollIntervalSeconds != 60 {
		t.Errorf("PollIntervalSeconds = %d, want 60", acct.IMAP.PollIntervalSeconds)
	}
	if acct.IMAP.IdleRestartMinutes != 25 {
		t.Errorf("IdleRestartMinutes = %d, want 25", acct.IMAP.IdleRestartMinutes)
	}

	// SMTP defaults
	if acct.SMTP == nil {
		t.Fatal("SMTP should not be nil after defaults")
	}
	if acct.SMTP.Host != "smtp.qiye.aliyun.com" {
		t.Errorf("SMTP.Host = %q, want %q", acct.SMTP.Host, "smtp.qiye.aliyun.com")
	}
	if acct.SMTP.Port != 465 {
		t.Errorf("SMTP.Port = %d, want 465", acct.SMTP.Port)
	}

	// Auth defaults
	if acct.Auth.Mode != types.EmailAuthAppPassword {
		t.Errorf("Auth.Mode = %q, want %q", acct.Auth.Mode, types.EmailAuthAppPassword)
	}

	// Login defaults to Address
	if acct.Login != "test@company.com" {
		t.Errorf("Login = %q, want %q", acct.Login, "test@company.com")
	}

	// Routing defaults
	if acct.Routing == nil {
		t.Fatal("Routing should not be nil")
	}
	if !acct.Routing.IgnoreAutoSubmitted {
		t.Error("IgnoreAutoSubmitted should default to true")
	}

	// Limits defaults
	if acct.Limits == nil {
		t.Fatal("Limits should not be nil")
	}
	if acct.Limits.MaxAttachments != 5 {
		t.Errorf("MaxAttachments = %d, want 5", acct.Limits.MaxAttachments)
	}

	// Behavior defaults
	if acct.Behavior == nil {
		t.Fatal("Behavior should not be nil")
	}
	if acct.Behavior.HTMLMode != types.EmailHTMLSafeText {
		t.Errorf("HTMLMode = %q, want %q", acct.Behavior.HTMLMode, types.EmailHTMLSafeText)
	}
}

func TestApplyProviderDefaults_UserOverride(t *testing.T) {
	acct := &types.EmailAccountConfig{
		Provider: types.EmailProviderQQ,
		Address:  "test@qq.com",
		IMAP: &types.EmailIMAPConfig{
			Host: "custom-imap.example.com",
			Port: 143,
		},
		SMTP: &types.EmailSMTPConfig{
			Host: "custom-smtp.example.com",
		},
	}

	ApplyProviderDefaults(acct)

	// User overrides should be preserved
	if acct.IMAP.Host != "custom-imap.example.com" {
		t.Errorf("IMAP.Host should remain user override, got %q", acct.IMAP.Host)
	}
	if acct.IMAP.Port != 143 {
		t.Errorf("IMAP.Port should remain user override, got %d", acct.IMAP.Port)
	}
	if acct.SMTP.Host != "custom-smtp.example.com" {
		t.Errorf("SMTP.Host should remain user override, got %q", acct.SMTP.Host)
	}

	// Zero-value fields should be filled from preset
	if acct.SMTP.Port != 465 {
		t.Errorf("SMTP.Port should be filled from QQ preset, got %d", acct.SMTP.Port)
	}
}

func TestApplyProviderDefaults_UnknownProvider(t *testing.T) {
	acct := &types.EmailAccountConfig{
		Provider: "custom_provider",
		Address:  "test@example.com",
		IMAP: &types.EmailIMAPConfig{
			Host: "imap.example.com",
			Port: 993,
		},
	}

	// Should not panic for unknown provider
	ApplyProviderDefaults(acct)

	// IMAP host from user config should remain
	if acct.IMAP.Host != "imap.example.com" {
		t.Errorf("IMAP.Host = %q, want user-provided value", acct.IMAP.Host)
	}
	// Defaults that don't depend on provider should still apply
	if acct.IMAP.Mode != types.EmailIMAPModeAuto {
		t.Errorf("IMAP.Mode should default to auto, got %q", acct.IMAP.Mode)
	}
}

func TestValidateEmailAccount(t *testing.T) {
	tests := []struct {
		name    string
		acct    *types.EmailAccountConfig
		wantErr bool
	}{
		{
			name:    "nil",
			acct:    nil,
			wantErr: true,
		},
		{
			name:    "empty address",
			acct:    &types.EmailAccountConfig{Auth: types.EmailAuthConfig{Password: "pass"}},
			wantErr: true,
		},
		{
			name:    "empty password",
			acct:    &types.EmailAccountConfig{Address: "test@test.com"},
			wantErr: true,
		},
		{
			name:    "keyring sentinel",
			acct:    &types.EmailAccountConfig{Address: "test@test.com", Auth: types.EmailAuthConfig{Password: config.KeyringSentinel}},
			wantErr: true,
		},
		{
			name: "valid",
			acct: &types.EmailAccountConfig{
				Address:  "test@test.com",
				Provider: types.EmailProviderQQ,
				Auth:     types.EmailAuthConfig{Password: "real-password"},
			},
			wantErr: false,
		},
		{
			name: "no provider no host",
			acct: &types.EmailAccountConfig{
				Address: "test@test.com",
				Auth:    types.EmailAuthConfig{Password: "pass"},
				IMAP:    &types.EmailIMAPConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmailAccount(tt.acct)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmailAccount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- M1 修复验证: CloneEmailAccountConfig 深拷贝 ---

func TestCloneEmailAccountConfig_Nil(t *testing.T) {
	if CloneEmailAccountConfig(nil) != nil {
		t.Error("CloneEmailAccountConfig(nil) should return nil")
	}
}

func TestCloneEmailAccountConfig_DeepCopy(t *testing.T) {
	boolTrue := true
	src := &types.EmailAccountConfig{
		Enabled:  &boolTrue,
		Provider: types.EmailProviderAliyun,
		Name:     "Test",
		Address:  "test@company.com",
		Login:    "test@company.com",
		Auth: types.EmailAuthConfig{
			Mode:     types.EmailAuthAppPassword,
			Password: "secret",
		},
		IMAP: &types.EmailIMAPConfig{
			Host:      "imap.test.com",
			Port:      993,
			Mailboxes: []string{"INBOX", "Sent"},
		},
		SMTP: &types.EmailSMTPConfig{
			Host: "smtp.test.com",
			Port: 465,
		},
		Routing: &types.EmailRoutingConfig{
			IgnoreAutoSubmitted: true,
			IgnoreMailingList:   true,
		},
		Limits: &types.EmailLimitsConfig{
			MaxAttachments:              5,
			AllowAttachmentMimePrefixes: []string{"image/", "text/"},
		},
		Behavior: &types.EmailBehaviorConfig{
			DownloadAttachments: true,
			HTMLMode:            types.EmailHTMLSafeText,
		},
	}

	dst := CloneEmailAccountConfig(src)

	// 值相等
	if dst.Address != src.Address {
		t.Errorf("Address mismatch: %q vs %q", dst.Address, src.Address)
	}
	if dst.Auth.Password != src.Auth.Password {
		t.Errorf("Password mismatch")
	}
	if dst.IMAP.Host != src.IMAP.Host {
		t.Errorf("IMAP.Host mismatch")
	}

	// 指针独立: 修改 dst 不影响 src
	*dst.Enabled = false
	if *src.Enabled != true {
		t.Error("Modifying dst.Enabled should not affect src")
	}

	dst.IMAP.Host = "changed"
	if src.IMAP.Host != "imap.test.com" {
		t.Error("Modifying dst.IMAP.Host should not affect src")
	}

	dst.IMAP.Mailboxes[0] = "Drafts"
	if src.IMAP.Mailboxes[0] != "INBOX" {
		t.Error("Modifying dst.IMAP.Mailboxes should not affect src")
	}

	dst.SMTP.Host = "changed"
	if src.SMTP.Host != "smtp.test.com" {
		t.Error("Modifying dst.SMTP.Host should not affect src")
	}

	dst.Routing.IgnoreAutoSubmitted = false
	if !src.Routing.IgnoreAutoSubmitted {
		t.Error("Modifying dst.Routing should not affect src")
	}

	dst.Limits.AllowAttachmentMimePrefixes[0] = "application/"
	if src.Limits.AllowAttachmentMimePrefixes[0] != "image/" {
		t.Error("Modifying dst.Limits.AllowAttachmentMimePrefixes should not affect src")
	}

	dst.Behavior.DownloadAttachments = false
	if !src.Behavior.DownloadAttachments {
		t.Error("Modifying dst.Behavior should not affect src")
	}
}

func TestCloneEmailAccountConfig_NilSubfields(t *testing.T) {
	src := &types.EmailAccountConfig{
		Address: "test@test.com",
		Auth:    types.EmailAuthConfig{Password: "pass"},
		// All pointer fields nil
	}
	dst := CloneEmailAccountConfig(src)
	if dst.IMAP != nil || dst.SMTP != nil || dst.Routing != nil || dst.Limits != nil || dst.Behavior != nil || dst.Enabled != nil {
		t.Error("Nil pointer fields should remain nil in clone")
	}
}

// M1 集成验证: ApplyProviderDefaults 不修改原始 config
func TestApplyProviderDefaults_DoesNotMutateOriginal(t *testing.T) {
	src := &types.EmailAccountConfig{
		Provider: types.EmailProviderAliyun,
		Address:  "test@company.com",
		Auth:     types.EmailAuthConfig{Password: "pass"},
	}
	// 验证原始 IMAP 为 nil
	if src.IMAP != nil {
		t.Fatal("Precondition: src.IMAP should be nil")
	}

	// 深拷贝后 apply
	clone := CloneEmailAccountConfig(src)
	ApplyProviderDefaults(clone)

	// 原始不受影响
	if src.IMAP != nil {
		t.Error("Original src.IMAP should remain nil after ApplyProviderDefaults on clone")
	}
	if src.SMTP != nil {
		t.Error("Original src.SMTP should remain nil")
	}
	if src.Routing != nil {
		t.Error("Original src.Routing should remain nil")
	}

	// Clone 已填充
	if clone.IMAP == nil || clone.IMAP.Host == "" {
		t.Error("Clone IMAP should be filled after ApplyProviderDefaults")
	}
}

func TestGetProviderPreset(t *testing.T) {
	providers := []types.EmailProviderID{
		types.EmailProviderAliyun,
		types.EmailProviderQQ,
		types.EmailProviderTencentExmail,
		types.EmailProviderNetease163,
	}

	for _, id := range providers {
		p := GetProviderPreset(id)
		if p == nil {
			t.Errorf("GetProviderPreset(%q) returned nil", id)
			continue
		}
		if p.IMAPHost == "" {
			t.Errorf("Provider %q: IMAPHost is empty", id)
		}
		if p.SMTPHost == "" {
			t.Errorf("Provider %q: SMTPHost is empty", id)
		}
		if p.IMAPPort == 0 {
			t.Errorf("Provider %q: IMAPPort is zero", id)
		}
		if p.SMTPPort == 0 {
			t.Errorf("Provider %q: SMTPPort is zero", id)
		}
	}

	// Unknown provider
	if GetProviderPreset("unknown") != nil {
		t.Error("GetProviderPreset(unknown) should return nil")
	}
}
