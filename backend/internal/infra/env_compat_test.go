package infra

import "testing"

func TestPreferredEnvValuePrefersCrabClawPrefix(t *testing.T) {
	t.Setenv("OPENACOSMI_CONFIG_DIR", "/tmp/open")
	t.Setenv("CRABCLAW_CONFIG_DIR", "/tmp/crab")
	if got := preferredEnvValue("CRABCLAW_CONFIG_DIR", "OPENACOSMI_CONFIG_DIR"); got != "/tmp/crab" {
		t.Fatalf("got %q, want %q", got, "/tmp/crab")
	}
}
