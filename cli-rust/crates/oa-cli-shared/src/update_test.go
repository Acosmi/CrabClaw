package infra

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── SSH Config Tests ───

func TestParseSshConfigOutput(t *testing.T) {
	output := `user root
hostname example.com
port 2222
identityfile ~/.ssh/id_rsa
identityfile ~/.ssh/id_ed25519
`
	config := ParseSshConfigOutput(output)
	if config.User != "root" {
		t.Errorf("user: got %q, want %q", config.User, "root")
	}
	if config.Host != "example.com" {
		t.Errorf("host: got %q, want %q", config.Host, "example.com")
	}
	if config.Port != 2222 {
		t.Errorf("port: got %d, want 2222", config.Port)
	}
	if len(config.IdentityFiles) != 2 {
		t.Fatalf("identityFiles: got %d, want 2", len(config.IdentityFiles))
	}
	if config.IdentityFiles[0] != "~/.ssh/id_rsa" {
		t.Errorf("identityFiles[0]: got %q", config.IdentityFiles[0])
	}
}

func TestParseSshConfigOutputEmpty(t *testing.T) {
	config := ParseSshConfigOutput("")
	if config.User != "" || config.Host != "" || config.Port != 0 {
		t.Error("expected empty config for empty output")
	}
	if len(config.IdentityFiles) != 0 {
		t.Error("expected empty identityFiles")
	}
}

func TestParseSshConfigOutputNoneIdentity(t *testing.T) {
	output := "identityfile none\n"
	config := ParseSshConfigOutput(output)
	if len(config.IdentityFiles) != 0 {
		t.Error("expected none identityfile to be excluded")
	}
}

// ─── Update Check Tests ───

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  *SemverParts
	}{
		{"v1.2.3", &SemverParts{1, 2, 3}},
		{"1.0.0", &SemverParts{1, 0, 0}},
		{"v0.10.20-beta.1", &SemverParts{0, 10, 20}},
		{"invalid", nil},
		{"", nil},
	}
	for _, tt := range tests {
		got := ParseSemver(tt.input)
		if tt.want == nil && got != nil {
			t.Errorf("ParseSemver(%q): got %+v, want nil", tt.input, got)
		} else if tt.want != nil {
			if got == nil {
				t.Errorf("ParseSemver(%q): got nil, want %+v", tt.input, tt.want)
			} else if *got != *tt.want {
				t.Errorf("ParseSemver(%q): got %+v, want %+v", tt.input, got, tt.want)
			}
		}
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.1", "v1.0.0", 1},
		{"v1.0.0", "v1.0.1", -1},
		{"v2.0.0", "v1.9.9", 1},
	}
	for _, tt := range tests {
		got := CompareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("CompareSemver(%q, %q): got %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// ─── Fetch Guard / SSRF Tests ───

func TestIsPrivateIPAddress(t *testing.T) {
	private := []string{
		"127.0.0.1", "10.0.0.1", "10.255.255.255",
		"172.16.0.1", "172.31.255.255",
		"192.168.0.1", "192.168.255.255",
		"100.64.0.1", "169.254.0.1",
		"::1", "::",
	}
	for _, ip := range private {
		if !IsPrivateIPAddress(ip) {
			t.Errorf("expected %q to be private", ip)
		}
	}

	public := []string{
		"1.1.1.1", "8.8.8.8", "203.0.113.1",
		"172.32.0.1", "11.0.0.1",
	}
	for _, ip := range public {
		if IsPrivateIPAddress(ip) {
			t.Errorf("expected %q to be public", ip)
		}
	}
}

func TestIsBlockedHostname(t *testing.T) {
	blocked := []string{
		"localhost", "LOCALHOST",
		"metadata.google.internal",
		"foo.localhost", "bar.local", "baz.internal",
	}
	for _, h := range blocked {
		if !IsBlockedHostname(h) {
			t.Errorf("expected %q to be blocked", h)
		}
	}

	allowed := []string{
		"example.com", "google.com", "api.example.org",
	}
	for _, h := range allowed {
		if IsBlockedHostname(h) {
			t.Errorf("expected %q to be allowed", h)
		}
	}
}

func TestValidateURLForSSRF(t *testing.T) {
	// 默认策略应拦截私有
	if err := ValidateURLForSSRF("127.0.0.1", nil); err == nil {
		t.Error("expected SSRF block for 127.0.0.1")
	}

	// 允许私有网络
	policy := &SsrfPolicy{AllowPrivateNetwork: true}
	if err := ValidateURLForSSRF("127.0.0.1", policy); err != nil {
		t.Errorf("expected allow: %v", err)
	}

	// 允许白名单
	policy2 := &SsrfPolicy{AllowedHostnames: []string{"localhost"}}
	if err := ValidateURLForSSRF("localhost", policy2); err != nil {
		t.Errorf("expected allow for whitelisted: %v", err)
	}
}

// ─── Update Channels Tests ───

func TestNormalizeUpdateChannel(t *testing.T) {
	tests := []struct {
		input string
		want  UpdateChannel
		ok    bool
	}{
		{"stable", ChannelStable, true},
		{"BETA", ChannelBeta, true},
		{"dev", ChannelDev, true},
		{"invalid", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizeUpdateChannel(tt.input)
		if ok != tt.ok || got != tt.want {
			t.Errorf("NormalizeUpdateChannel(%q): got (%q, %v), want (%q, %v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIsBetaTag(t *testing.T) {
	if !IsBetaTag("v1.0.0-beta.1") {
		t.Error("expected beta tag")
	}
	if IsBetaTag("v1.0.0") {
		t.Error("expected non-beta tag")
	}
}

// ─── Restart Sentinel Tests ───

func TestWriteAndReadRestartSentinel(t *testing.T) {
	dir := t.TempDir()
	payload := RestartSentinelPayload{
		Kind:   RestartKindUpdate,
		Status: RestartStatusOk,
		Ts:     1234567890,
	}

	path, err := WriteRestartSentinel(dir, payload)
	if err != nil {
		t.Fatalf("WriteRestartSentinel: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	sentinel := ReadRestartSentinel(dir)
	if sentinel == nil {
		t.Fatal("expected non-nil sentinel")
	}
	if sentinel.Version != 1 {
		t.Errorf("version: got %d, want 1", sentinel.Version)
	}
	if sentinel.Payload.Kind != RestartKindUpdate {
		t.Errorf("kind: got %q", sentinel.Payload.Kind)
	}
}

func TestConsumeRestartSentinel(t *testing.T) {
	dir := t.TempDir()
	payload := RestartSentinelPayload{Kind: RestartKindRestart, Status: RestartStatusOk, Ts: 1}
	WriteRestartSentinel(dir, payload)

	sentinel := ConsumeRestartSentinel(dir)
	if sentinel == nil {
		t.Fatal("expected non-nil sentinel")
	}

	// 消费后文件应该已被删除
	if _, err := os.Stat(filepath.Join(dir, sentinelFilename)); !os.IsNotExist(err) {
		t.Error("expected sentinel file to be deleted after consume")
	}
}

func TestTrimLogTail(t *testing.T) {
	if TrimLogTail("short", 100) != "short" {
		t.Error("expected unchanged short text")
	}
	long := "abcdefghij"
	if got := TrimLogTail(long, 5); got != "…fghij" {
		t.Errorf("got %q, want %q", got, "…fghij")
	}
}

func TestSummarizeRestartSentinel(t *testing.T) {
	payload := RestartSentinelPayload{
		Kind:   RestartKindUpdate,
		Status: RestartStatusOk,
		Stats:  &RestartSentinelStats{Mode: "git-pull"},
	}
	got := SummarizeRestartSentinel(payload)
	if got != "Gateway restart update ok (git-pull)" {
		t.Errorf("got %q", got)
	}
}
