package pairing

// 配对消息测试 — 对齐 src/pairing/pairing-messages.test.ts

import (
	"os"
	"strings"
	"testing"
)

func TestBuildPairingReply(t *testing.T) {
	// 设置 profile 环境变量（对齐 TS 测试中 OPENACOSMI_PROFILE="isolated"）
	previous := os.Getenv("OPENACOSMI_PROFILE")
	os.Setenv("OPENACOSMI_PROFILE", "isolated")
	defer func() {
		if previous == "" {
			os.Unsetenv("OPENACOSMI_PROFILE")
		} else {
			os.Setenv("OPENACOSMI_PROFILE", previous)
		}
	}()

	cases := []struct {
		channel string
		idLine  string
		code    string
	}{
		{"discord", "Your Discord user id: 1", "ABC123"},
		{"slack", "Your Slack user id: U1", "DEF456"},
		{"signal", "Your Signal number: +15550001111", "GHI789"},
		{"imessage", "Your iMessage sender id: +15550002222", "JKL012"},
		{"whatsapp", "Your WhatsApp phone number: +15550003333", "MNO345"},
	}

	for _, tc := range cases {
		t.Run(tc.channel, func(t *testing.T) {
			text := BuildPairingReply(tc.channel, tc.idLine, tc.code)
			if !strings.Contains(text, "Crab Claw（蟹爪）") {
				t.Errorf("missing new brand in pairing reply: %q", text)
			}
			if !strings.Contains(text, tc.idLine) {
				t.Errorf("missing idLine %q", tc.idLine)
			}
			if !strings.Contains(text, "Pairing code: "+tc.code) {
				t.Errorf("missing pairing code %q", tc.code)
			}
			// 验证 --profile isolated 被正确插入
			if !strings.Contains(text, "--profile isolated") {
				t.Error("missing --profile isolated in command")
			}
			if !strings.Contains(text, "pairing approve "+tc.channel+" <code>") {
				t.Errorf("missing approve command for %s", tc.channel)
			}
		})
	}
}

func TestBuildPairingReplyNoProfile(t *testing.T) {
	previous := os.Getenv("OPENACOSMI_PROFILE")
	os.Unsetenv("OPENACOSMI_PROFILE")
	defer func() {
		if previous != "" {
			os.Setenv("OPENACOSMI_PROFILE", previous)
		}
	}()

	text := BuildPairingReply("discord", "id: 1", "CODE")
	if strings.Contains(text, "--profile") {
		t.Error("should not contain --profile when env not set")
	}
	if !strings.Contains(text, "crabclaw pairing approve discord <code>") {
		t.Error("missing base command")
	}
}

func TestPairingApprovedMessage(t *testing.T) {
	if PairingApprovedMessage == "" {
		t.Error("PairingApprovedMessage should not be empty")
	}
	if !strings.Contains(PairingApprovedMessage, "Crab Claw（蟹爪）") {
		t.Error("should contain the new brand")
	}
	if !strings.Contains(PairingApprovedMessage, "approved") {
		t.Error("should contain 'approved'")
	}
}
