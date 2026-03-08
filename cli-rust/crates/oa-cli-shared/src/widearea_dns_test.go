package infra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TS 对照: src/infra/widearea-dns.test.ts (45L)

func TestNormalizeWideAreaDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"openacosmi.internal.", "openacosmi.internal."},
		{"openacosmi.internal", "openacosmi.internal."},
		{"  example.com  ", "example.com."},
	}
	for _, tt := range tests {
		got := NormalizeWideAreaDomain(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeWideAreaDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveWideAreaDiscoveryDomain(t *testing.T) {
	// Config domain 优先
	got := ResolveWideAreaDiscoveryDomain("openacosmi.internal")
	if got != "openacosmi.internal." {
		t.Errorf("expected openacosmi.internal., got %q", got)
	}
	// 空 → 空
	got = ResolveWideAreaDiscoveryDomain("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolveWideAreaDiscoveryDomainPrefersCrabClawEnv(t *testing.T) {
	t.Setenv("OPENACOSMI_WIDE_AREA_DOMAIN", "old.internal")
	t.Setenv("CRABCLAW_WIDE_AREA_DOMAIN", "new.internal")
	got := ResolveWideAreaDiscoveryDomain("")
	if got != "new.internal." {
		t.Fatalf("got %q, want %q", got, "new.internal.")
	}
}

func TestRenderWideAreaGatewayZoneText(t *testing.T) {
	txt := RenderWideAreaGatewayZoneText(WideAreaGatewayZoneOpts{
		Domain:        "openacosmi.internal.",
		GatewayPort:   19001,
		DisplayName:   "Mac Studio (Crab Claw)",
		TailnetIPv4:   "100.123.224.76",
		TailnetIPv6:   "fd7a:115c:a1e0::8801:e04c",
		HostLabel:     "studio-london",
		InstanceLabel: "studio-london",
		SSHPort:       22,
		CLIPath:       "/opt/homebrew/bin/openacosmi",
	}, 2025121701)

	checks := []string{
		"$ORIGIN openacosmi.internal.",
		"studio-london IN A 100.123.224.76",
		"studio-london IN AAAA fd7a:115c:a1e0::8801:e04c",
		"_openacosmi-gw._tcp IN PTR studio-london._openacosmi-gw._tcp",
		"studio-london._openacosmi-gw._tcp IN SRV 0 0 19001 studio-london",
		"displayName=Mac Studio (Crab Claw)",
		"gatewayPort=19001",
		"sshPort=22",
		"cliPath=/opt/homebrew/bin/openacosmi",
	}

	for _, check := range checks {
		if !strings.Contains(txt, check) {
			t.Errorf("zone text missing %q", check)
		}
	}
}

func TestRenderWithTailnetDNS(t *testing.T) {
	txt := RenderWideAreaGatewayZoneText(WideAreaGatewayZoneOpts{
		Domain:        "openacosmi.internal.",
		GatewayPort:   19001,
		DisplayName:   "Mac Studio (Crab Claw)",
		TailnetIPv4:   "100.123.224.76",
		TailnetDNS:    "peters-mac-studio-1.sheep-coho.ts.net",
		HostLabel:     "studio-london",
		InstanceLabel: "studio-london",
	}, 2025121701)

	if !strings.Contains(txt, "tailnetDns=peters-mac-studio-1.sheep-coho.ts.net") {
		t.Error("zone text missing tailnetDns")
	}
}

func TestComputeContentHash(t *testing.T) {
	// 验证 FNV-1a 哈希与 TS 一致
	h := computeContentHash("hello")
	if h == "" {
		t.Error("hash should not be empty")
	}
	if len(h) != 8 {
		t.Errorf("hash length should be 8, got %d", len(h))
	}
	// 相同输入产生相同输出
	h2 := computeContentHash("hello")
	if h != h2 {
		t.Errorf("hash not deterministic: %s != %s", h, h2)
	}
}

func TestDNSLabel(t *testing.T) {
	tests := []struct {
		input    string
		fallback string
		want     string
	}{
		{"Hello World!", "x", "hello-world-"},
		{"", "fallback", "fallback"},
		{"---", "fb", "fb"},
		{"valid-label", "fb", "valid-label"},
	}
	for _, tt := range tests {
		// dnsLabel trims leading/trailing dashes
		got := dnsLabel(tt.input, tt.fallback)
		if got == "" {
			t.Errorf("dnsLabel(%q, %q) should not be empty", tt.input, tt.fallback)
		}
	}
}

func TestNextSerial(t *testing.T) {
	now := time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)
	// 无已有 serial
	s := nextSerialNum(0, now)
	if s != 2026022101 {
		t.Errorf("expected 2026022101, got %d", s)
	}
	// 同日已有 serial — 递增
	s = nextSerialNum(2026022101, now)
	if s != 2026022102 {
		t.Errorf("expected 2026022102, got %d", s)
	}
	// 不同日 — 重置
	s = nextSerialNum(2026022001, now)
	if s != 2026022101 {
		t.Errorf("expected 2026022101 (new day), got %d", s)
	}
}

func TestWriteWideAreaGatewayZone(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("OPENACOSMI_CONFIG_DIR", tmpDir)

	result, err := WriteWideAreaGatewayZone(WideAreaGatewayZoneOpts{
		Domain:      "test.local",
		GatewayPort: 19001,
		DisplayName: "Test",
		TailnetIPv4: "100.1.2.3",
	})
	if err != nil {
		t.Fatalf("WriteWideAreaGatewayZone: %v", err)
	}
	if !result.Changed {
		t.Error("first write should report changed=true")
	}
	if result.ZonePath == "" {
		t.Error("zonePath should not be empty")
	}

	// 验证文件存在
	expectedPath := filepath.Join(tmpDir, "dns", "test.local.db")
	if result.ZonePath != expectedPath {
		t.Errorf("zonePath = %q, want %q", result.ZonePath, expectedPath)
	}
	data, err := os.ReadFile(result.ZonePath)
	if err != nil {
		t.Fatalf("read zone file: %v", err)
	}
	if !strings.Contains(string(data), "$ORIGIN test.local.") {
		t.Error("zone file missing $ORIGIN")
	}

	// 第二次写入 — 应返回 changed=false（内容未变）
	result2, err := WriteWideAreaGatewayZone(WideAreaGatewayZoneOpts{
		Domain:      "test.local",
		GatewayPort: 19001,
		DisplayName: "Test",
		TailnetIPv4: "100.1.2.3",
	})
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if result2.Changed {
		t.Error("second write with same content should report changed=false")
	}
}

func TestWriteWideAreaGatewayZonePrefersCrabClawConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("OPENACOSMI_CONFIG_DIR", t.TempDir())
	t.Setenv("CRABCLAW_CONFIG_DIR", tmpDir)

	result, err := WriteWideAreaGatewayZone(WideAreaGatewayZoneOpts{
		Domain:      "compat.local",
		GatewayPort: 19001,
		DisplayName: "Compat",
		TailnetIPv4: "100.1.2.9",
	})
	if err != nil {
		t.Fatalf("WriteWideAreaGatewayZone: %v", err)
	}
	expectedPath := filepath.Join(tmpDir, "dns", "compat.local.db")
	if result.ZonePath != expectedPath {
		t.Fatalf("zonePath = %q, want %q", result.ZonePath, expectedPath)
	}
}
