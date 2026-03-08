package daemon

import "testing"

func TestNormalizeGatewayProfile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"default keyword", "default", ""},
		{"default uppercase", "DEFAULT", ""},
		{"default mixed case", "Default", ""},
		{"whitespace", "  ", ""},
		{"custom profile", "myprofile", "myprofile"},
		{"custom with spaces", "  myprofile  ", "myprofile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeGatewayProfile(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeGatewayProfile(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveGatewayProfileSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"default", ""},
		{"myprofile", "-myprofile"},
	}
	for _, tt := range tests {
		result := ResolveGatewayProfileSuffix(tt.input)
		if result != tt.expected {
			t.Errorf("ResolveGatewayProfileSuffix(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestResolveGatewayLaunchAgentLabel(t *testing.T) {
	tests := []struct {
		profile  string
		expected string
	}{
		{"", "ai.openacosmi.gateway"},
		{"default", "ai.openacosmi.gateway"},
		{"staging", "ai.openacosmi.staging"},
	}
	for _, tt := range tests {
		result := ResolveGatewayLaunchAgentLabel(tt.profile)
		if result != tt.expected {
			t.Errorf("ResolveGatewayLaunchAgentLabel(%q) = %q, want %q", tt.profile, result, tt.expected)
		}
	}
}

func TestResolveGatewaySystemdServiceName(t *testing.T) {
	tests := []struct {
		profile  string
		expected string
	}{
		{"", "openacosmi-gateway"},
		{"default", "openacosmi-gateway"},
		{"staging", "openacosmi-gateway-staging"},
	}
	for _, tt := range tests {
		result := ResolveGatewaySystemdServiceName(tt.profile)
		if result != tt.expected {
			t.Errorf("ResolveGatewaySystemdServiceName(%q) = %q, want %q", tt.profile, result, tt.expected)
		}
	}
}

func TestResolveGatewayWindowsTaskName(t *testing.T) {
	tests := []struct {
		profile  string
		expected string
	}{
		{"", "OpenAcosmi Gateway"},
		{"default", "OpenAcosmi Gateway"},
		{"staging", "OpenAcosmi Gateway (staging)"},
	}
	for _, tt := range tests {
		result := ResolveGatewayWindowsTaskName(tt.profile)
		if result != tt.expected {
			t.Errorf("ResolveGatewayWindowsTaskName(%q) = %q, want %q", tt.profile, result, tt.expected)
		}
	}
}

func TestResolveCompatibleGatewayLaunchAgentLabels(t *testing.T) {
	tests := []struct {
		profile  string
		expected []string
	}{
		{"", []string{"ai.openacosmi.gateway", "ai.crabclaw.gateway"}},
		{"default", []string{"ai.openacosmi.gateway", "ai.crabclaw.gateway"}},
		{"staging", []string{"ai.openacosmi.staging", "ai.crabclaw.staging"}},
	}
	for _, tt := range tests {
		result := ResolveCompatibleGatewayLaunchAgentLabels(tt.profile)
		if len(result) != len(tt.expected) {
			t.Fatalf("ResolveCompatibleGatewayLaunchAgentLabels(%q) len = %d, want %d", tt.profile, len(result), len(tt.expected))
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Fatalf("ResolveCompatibleGatewayLaunchAgentLabels(%q)[%d] = %q, want %q", tt.profile, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestResolveCompatibleGatewaySystemdServiceNames(t *testing.T) {
	tests := []struct {
		profile  string
		expected []string
	}{
		{"", []string{"openacosmi-gateway", "crabclaw-gateway"}},
		{"default", []string{"openacosmi-gateway", "crabclaw-gateway"}},
		{"staging", []string{"openacosmi-gateway-staging", "crabclaw-gateway-staging"}},
	}
	for _, tt := range tests {
		result := ResolveCompatibleGatewaySystemdServiceNames(tt.profile)
		if len(result) != len(tt.expected) {
			t.Fatalf("ResolveCompatibleGatewaySystemdServiceNames(%q) len = %d, want %d", tt.profile, len(result), len(tt.expected))
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Fatalf("ResolveCompatibleGatewaySystemdServiceNames(%q)[%d] = %q, want %q", tt.profile, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestResolveCompatibleGatewayWindowsTaskNames(t *testing.T) {
	tests := []struct {
		profile  string
		expected []string
	}{
		{"", []string{"OpenAcosmi Gateway", "Crab Claw Gateway"}},
		{"default", []string{"OpenAcosmi Gateway", "Crab Claw Gateway"}},
		{"staging", []string{"OpenAcosmi Gateway (staging)", "Crab Claw Gateway (staging)"}},
	}
	for _, tt := range tests {
		result := ResolveCompatibleGatewayWindowsTaskNames(tt.profile)
		if len(result) != len(tt.expected) {
			t.Fatalf("ResolveCompatibleGatewayWindowsTaskNames(%q) len = %d, want %d", tt.profile, len(result), len(tt.expected))
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Fatalf("ResolveCompatibleGatewayWindowsTaskNames(%q)[%d] = %q, want %q", tt.profile, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestResolveCompatibleNodeIdentifiers(t *testing.T) {
	launchd := ResolveCompatibleNodeLaunchAgentLabels()
	if launchd[0] != "ai.openacosmi.node" || launchd[1] != "ai.crabclaw.node" {
		t.Fatalf("ResolveCompatibleNodeLaunchAgentLabels() = %#v", launchd)
	}
	systemd := ResolveCompatibleNodeSystemdServiceNames()
	if systemd[0] != "openacosmi-node" || systemd[1] != "crabclaw-node" {
		t.Fatalf("ResolveCompatibleNodeSystemdServiceNames() = %#v", systemd)
	}
	tasks := ResolveCompatibleNodeWindowsTaskNames()
	if tasks[0] != "OpenAcosmi Node" || tasks[1] != "Crab Claw Node" {
		t.Fatalf("ResolveCompatibleNodeWindowsTaskNames() = %#v", tasks)
	}
}

func TestFormatGatewayServiceDescription(t *testing.T) {
	tests := []struct {
		profile  string
		version  string
		expected string
	}{
		{"", "", "Crab Claw Gateway"},
		{"", "1.2.3", "Crab Claw Gateway (v1.2.3)"},
		{"staging", "", "Crab Claw Gateway (profile: staging)"},
		{"staging", "1.2.3", "Crab Claw Gateway (profile: staging, v1.2.3)"},
	}
	for _, tt := range tests {
		result := FormatGatewayServiceDescription(tt.profile, tt.version)
		if result != tt.expected {
			t.Errorf("FormatGatewayServiceDescription(%q, %q) = %q, want %q", tt.profile, tt.version, result, tt.expected)
		}
	}
}

func TestFormatNodeServiceDescription(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"", "Crab Claw Node Host"},
		{"1.2.3", "Crab Claw Node Host (v1.2.3)"},
	}
	for _, tt := range tests {
		result := FormatNodeServiceDescription(tt.version)
		if result != tt.expected {
			t.Errorf("FormatNodeServiceDescription(%q) = %q, want %q", tt.version, result, tt.expected)
		}
	}
}

func TestNeedsNodeRuntimeMigration(t *testing.T) {
	tests := []struct {
		name     string
		issues   []ServiceConfigIssue
		expected bool
	}{
		{"no issues", nil, false},
		{"unrelated issue", []ServiceConfigIssue{{Code: AuditCodeGatewayPathMissing}}, false},
		{"bun runtime", []ServiceConfigIssue{{Code: AuditCodeGatewayRuntimeBun}}, true},
		{"version manager", []ServiceConfigIssue{{Code: AuditCodeGatewayRuntimeNodeVersionManager}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NeedsNodeRuntimeMigration(tt.issues)
			if result != tt.expected {
				t.Errorf("NeedsNodeRuntimeMigration() = %v, want %v", result, tt.expected)
			}
		})
	}
}
