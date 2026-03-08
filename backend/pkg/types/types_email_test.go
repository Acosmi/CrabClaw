package types

import (
	"encoding/json"
	"testing"
)

func TestEmailConfig_MarshalRoundTrip(t *testing.T) {
	boolTrue := true
	cfg := &EmailConfig{
		Enabled:        &boolTrue,
		DefaultAccount: "ali-work",
		Accounts: map[string]*EmailAccountConfig{
			"ali-work": {
				Enabled:  &boolTrue,
				Provider: EmailProviderAliyun,
				Name:     "阿里企业邮箱",
				Address:  "robot@company.com",
				Login:    "robot@company.com",
				Auth: EmailAuthConfig{
					Mode:     EmailAuthAppPassword,
					Password: "test-secret-123",
				},
				IMAP: &EmailIMAPConfig{
					Host:                "imap.qiye.aliyun.com",
					Port:                993,
					Security:            EmailSecurityTLS,
					Mode:                EmailIMAPModeAuto,
					Mailboxes:           []string{"INBOX"},
					PollIntervalSeconds: 60,
					IdleRestartMinutes:  25,
					FetchBatchSize:      20,
					MaxFetchBytes:       5242880,
				},
				SMTP: &EmailSMTPConfig{
					Host:     "smtp.qiye.aliyun.com",
					Port:     465,
					Security: EmailSecurityTLS,
					FromName: "OpenAcosmi Bot",
				},
				Routing: &EmailRoutingConfig{
					SessionBy:           "references_or_message_id",
					SubjectTag:          "[OA]",
					IgnoreAutoSubmitted: true,
					IgnoreMailingList:   true,
					IgnoreNoReply:       true,
					IgnoreSelfSent:      true,
				},
				Limits: &EmailLimitsConfig{
					MaxMessageBytes:             5242880,
					MaxAttachmentBytes:          10485760,
					MaxAttachments:              5,
					AllowAttachmentMimePrefixes: []string{"image/", "text/", "application/pdf"},
					MaxSendPerHour:              30,
					MaxSendPerDay:               200,
				},
				Behavior: &EmailBehaviorConfig{
					DownloadAttachments: true,
					HTMLMode:            EmailHTMLSafeText,
					SendReplyAsText:     true,
					AutoReplyEnabled:    true,
				},
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored EmailConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.DefaultAccount != "ali-work" {
		t.Errorf("DefaultAccount = %q, want %q", restored.DefaultAccount, "ali-work")
	}
	if restored.Enabled == nil || !*restored.Enabled {
		t.Error("Enabled should be true")
	}

	acct, ok := restored.Accounts["ali-work"]
	if !ok {
		t.Fatal("Account ali-work not found")
	}
	if acct.Provider != EmailProviderAliyun {
		t.Errorf("Provider = %q, want %q", acct.Provider, EmailProviderAliyun)
	}
	if acct.Auth.Password != "test-secret-123" {
		t.Errorf("Auth.Password = %q, want %q", acct.Auth.Password, "test-secret-123")
	}
	if acct.Auth.Mode != EmailAuthAppPassword {
		t.Errorf("Auth.Mode = %q, want %q", acct.Auth.Mode, EmailAuthAppPassword)
	}
	if acct.IMAP.Port != 993 {
		t.Errorf("IMAP.Port = %d, want 993", acct.IMAP.Port)
	}
	if acct.SMTP.Port != 465 {
		t.Errorf("SMTP.Port = %d, want 465", acct.SMTP.Port)
	}
	if acct.Limits.MaxSendPerHour != 30 {
		t.Errorf("Limits.MaxSendPerHour = %d, want 30", acct.Limits.MaxSendPerHour)
	}
	if acct.Limits.MaxSendPerDay != 200 {
		t.Errorf("Limits.MaxSendPerDay = %d, want 200", acct.Limits.MaxSendPerDay)
	}
	if !acct.Behavior.AutoReplyEnabled {
		t.Error("Behavior.AutoReplyEnabled should be true")
	}
}

func TestEmailConfig_MultiAccount(t *testing.T) {
	boolTrue := true
	boolFalse := false
	cfg := &EmailConfig{
		Enabled:        &boolTrue,
		DefaultAccount: "ali-work",
		Accounts: map[string]*EmailAccountConfig{
			"ali-work": {
				Enabled:  &boolTrue,
				Provider: EmailProviderAliyun,
				Address:  "robot@company.com",
			},
			"qq-personal": {
				Enabled:  &boolFalse,
				Provider: EmailProviderQQ,
				Address:  "user@qq.com",
			},
			"netease": {
				Provider: EmailProviderNetease163,
				Address:  "user@163.com",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored EmailConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Accounts) != 3 {
		t.Errorf("Accounts count = %d, want 3", len(restored.Accounts))
	}

	// qq-personal should be disabled
	qq := restored.Accounts["qq-personal"]
	if qq == nil {
		t.Fatal("Account qq-personal not found")
	}
	if qq.Enabled == nil || *qq.Enabled {
		t.Error("qq-personal should be disabled")
	}

	// netease should default to enabled (nil = enabled via IsAccountEnabled)
	ne := restored.Accounts["netease"]
	if ne == nil {
		t.Fatal("Account netease not found")
	}
	if ne.Enabled != nil {
		t.Error("netease Enabled should be nil (defaults to enabled)")
	}
	// IsAccountEnabled is in channels package; nil Enabled means enabled by convention
	if ne.Enabled != nil && !*ne.Enabled {
		t.Error("netease should default to enabled")
	}
}

func TestEmailConfig_ChannelsConfigIntegration(t *testing.T) {
	jsonData := `{
		"email": {
			"enabled": true,
			"defaultAccount": "test",
			"accounts": {
				"test": {
					"provider": "qq",
					"address": "test@qq.com",
					"auth": {
						"mode": "app_password",
						"password": "secret123"
					}
				}
			}
		}
	}`

	var cc ChannelsConfig
	if err := json.Unmarshal([]byte(jsonData), &cc); err != nil {
		t.Fatalf("Unmarshal ChannelsConfig failed: %v", err)
	}

	if cc.Email == nil {
		t.Fatal("Email config should not be nil")
	}
	if cc.Email.DefaultAccount != "test" {
		t.Errorf("DefaultAccount = %q, want %q", cc.Email.DefaultAccount, "test")
	}
	acct := cc.Email.Accounts["test"]
	if acct == nil {
		t.Fatal("Account test not found")
	}
	if acct.Auth.Password != "secret123" {
		t.Errorf("Auth.Password = %q, want %q", acct.Auth.Password, "secret123")
	}

	// Re-marshal and verify round-trip
	out, err := json.Marshal(cc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var cc2 ChannelsConfig
	if err := json.Unmarshal(out, &cc2); err != nil {
		t.Fatalf("Re-unmarshal failed: %v", err)
	}
	if cc2.Email == nil || cc2.Email.Accounts["test"] == nil {
		t.Fatal("Round-trip failed: email config lost")
	}
}

func TestEmailConfig_EmptyAccountsNilSafe(t *testing.T) {
	cfg := &EmailConfig{}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal empty config failed: %v", err)
	}

	var restored EmailConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal empty config failed: %v", err)
	}

	if restored.Accounts != nil {
		t.Errorf("Empty config Accounts should be nil, got %v", restored.Accounts)
	}
}

func TestEmailConfig_AccountMapKeyStability(t *testing.T) {
	// Verify map keys are stable across serialization (critical for Keyring path stability)
	boolTrue := true
	cfg := &EmailConfig{
		Enabled: &boolTrue,
		Accounts: map[string]*EmailAccountConfig{
			"account-a": {Address: "a@test.com", Auth: EmailAuthConfig{Password: "pass-a"}},
			"account-b": {Address: "b@test.com", Auth: EmailAuthConfig{Password: "pass-b"}},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored EmailConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Each account's password must be accessible by its stable key
	if restored.Accounts["account-a"].Auth.Password != "pass-a" {
		t.Error("account-a password mismatch after round-trip")
	}
	if restored.Accounts["account-b"].Auth.Password != "pass-b" {
		t.Error("account-b password mismatch after round-trip")
	}
}

func TestEmailConfig_ProviderIDValues(t *testing.T) {
	tests := []struct {
		id   EmailProviderID
		want string
	}{
		{EmailProviderAliyun, "aliyun"},
		{EmailProviderQQ, "qq"},
		{EmailProviderTencentExmail, "tencent_exmail"},
		{EmailProviderNetease163, "netease163"},
	}
	for _, tt := range tests {
		if string(tt.id) != tt.want {
			t.Errorf("ProviderID = %q, want %q", tt.id, tt.want)
		}
	}
}
