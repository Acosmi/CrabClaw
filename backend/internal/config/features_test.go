package config

import "testing"

func TestParseMultimodalChannelsSwitch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		raw          string
		wantAll      bool
		wantEnabled  []string
		wantDisabled []string
	}{
		{
			name:        "empty means all",
			raw:         "",
			wantAll:     true,
			wantEnabled: []string{"feishu", "dingtalk", "wecom"},
		},
		{
			name:         "none disables all",
			raw:          "none",
			wantAll:      false,
			wantDisabled: []string{"feishu", "dingtalk", "wecom"},
		},
		{
			name:        "all enables all",
			raw:         "all",
			wantAll:     true,
			wantEnabled: []string{"feishu", "dingtalk", "wecom"},
		},
		{
			name:         "comma list",
			raw:          "feishu,wecom",
			wantAll:      false,
			wantEnabled:  []string{"feishu", "wecom"},
			wantDisabled: []string{"dingtalk"},
		},
		{
			name:         "channel aliases",
			raw:          "lark,wechatwork",
			wantAll:      false,
			wantEnabled:  []string{"feishu", "wecom"},
			wantDisabled: []string{"dingtalk"},
		},
		{
			name:         "mixed delimiters",
			raw:          "feishu; dingtalk | wecom",
			wantAll:      false,
			wantEnabled:  []string{"feishu", "dingtalk", "wecom"},
			wantDisabled: []string{"slack"},
		},
		{
			name:        "invalid fallback to all",
			raw:         ", , ;",
			wantAll:     true,
			wantEnabled: []string{"feishu", "dingtalk", "wecom"},
		},
		{
			name:        "unknown token fallback to all",
			raw:         "foo",
			wantAll:     true,
			wantEnabled: []string{"feishu", "dingtalk", "wecom"},
		},
		{
			name:         "mixed known and unknown keeps known",
			raw:          "feishu,foo",
			wantAll:      false,
			wantEnabled:  []string{"feishu"},
			wantDisabled: []string{"dingtalk", "wecom"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotAll, gotList := parseMultimodalChannelsSwitch(tc.raw)
			if gotAll != tc.wantAll {
				t.Fatalf("allowAll=%v, want=%v", gotAll, tc.wantAll)
			}
			for _, ch := range tc.wantEnabled {
				if !isMultimodalChannelEnabled(ch, gotAll, gotList) {
					t.Fatalf("channel %q should be enabled for raw=%q", ch, tc.raw)
				}
			}
			for _, ch := range tc.wantDisabled {
				if isMultimodalChannelEnabled(ch, gotAll, gotList) {
					t.Fatalf("channel %q should be disabled for raw=%q", ch, tc.raw)
				}
			}
		})
	}
}

func TestMultimodalRolloutDrillSequence(t *testing.T) {
	t.Parallel()

	type step struct {
		switchValue string
		wantEnabled map[string]bool
	}

	steps := []step{
		{
			switchValue: "feishu",
			wantEnabled: map[string]bool{
				"feishu":   true,
				"dingtalk": false,
				"wecom":    false,
			},
		},
		{
			switchValue: "feishu,dingtalk",
			wantEnabled: map[string]bool{
				"feishu":   true,
				"dingtalk": true,
				"wecom":    false,
			},
		},
		{
			switchValue: "all",
			wantEnabled: map[string]bool{
				"feishu":   true,
				"dingtalk": true,
				"wecom":    true,
			},
		},
		{
			switchValue: "none",
			wantEnabled: map[string]bool{
				"feishu":   false,
				"dingtalk": false,
				"wecom":    false,
			},
		},
	}

	for _, s := range steps {
		allowAll, allowList := parseMultimodalChannelsSwitch(s.switchValue)
		for channel, want := range s.wantEnabled {
			got := isMultimodalChannelEnabled(channel, allowAll, allowList)
			if got != want {
				t.Fatalf("switch=%q channel=%q enabled=%v, want %v", s.switchValue, channel, got, want)
			}
		}
	}
}

func TestLoadFeatureFlagsPrefersCrabClawEnv(t *testing.T) {
	env := map[string]string{
		"CRABCLAW_SKIP_CRON":             "1",
		"OPENACOSMI_SKIP_CRON":           "",
		"CRABCLAW_MULTIMODAL_CHANNELS":   "feishu",
		"OPENACOSMI_MULTIMODAL_CHANNELS": "wecom",
	}
	flags := loadFeatureFlags(func(key string) string {
		return env[key]
	})
	if !flags.skipCron {
		t.Fatal("expected crabclaw skip cron env to be honored")
	}
	if flags.multimodalSwitch != "feishu" {
		t.Fatalf("got multimodal switch %q, want feishu", flags.multimodalSwitch)
	}
	if !isMultimodalChannelEnabled("feishu", flags.multimodalAllowAll, flags.multimodalAllowedNames) {
		t.Fatal("expected feishu enabled")
	}
	if isMultimodalChannelEnabled("wecom", flags.multimodalAllowAll, flags.multimodalAllowedNames) {
		t.Fatal("expected wecom disabled")
	}
}
