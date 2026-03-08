package config

import (
	"os"
	"testing"
)

func TestParseShellEnvOutput(t *testing.T) {
	t.Run("typical output", func(t *testing.T) {
		// env -0 produces NUL-separated KEY=VALUE pairs
		data := []byte("HOME=/Users/test\x00PATH=/usr/bin:/bin\x00SHELL=/bin/zsh\x00")
		result := parseShellEnvOutput(data)

		if result["HOME"] != "/Users/test" {
			t.Fatalf("HOME = %q, want /Users/test", result["HOME"])
		}
		if result["PATH"] != "/usr/bin:/bin" {
			t.Fatalf("PATH = %q, want /usr/bin:/bin", result["PATH"])
		}
		if result["SHELL"] != "/bin/zsh" {
			t.Fatalf("SHELL = %q, want /bin/zsh", result["SHELL"])
		}
	})

	t.Run("empty output", func(t *testing.T) {
		result := parseShellEnvOutput([]byte{})
		if len(result) != 0 {
			t.Fatalf("expected empty map, got %v", result)
		}
	})

	t.Run("malformed entries", func(t *testing.T) {
		data := []byte("GOOD=value\x00=no-key\x00badentry\x00ALSO_GOOD=123\x00")
		result := parseShellEnvOutput(data)
		if len(result) != 2 {
			t.Fatalf("expected 2 entries, got %d: %v", len(result), result)
		}
		if result["GOOD"] != "value" {
			t.Fatalf("GOOD = %q, want value", result["GOOD"])
		}
		if result["ALSO_GOOD"] != "123" {
			t.Fatalf("ALSO_GOOD = %q, want 123", result["ALSO_GOOD"])
		}
	})

	t.Run("value with equals", func(t *testing.T) {
		data := []byte("KEY=value=with=equals\x00")
		result := parseShellEnvOutput(data)
		if result["KEY"] != "value=with=equals" {
			t.Fatalf("KEY = %q, want value=with=equals", result["KEY"])
		}
	})
}

func TestResolveShell(t *testing.T) {
	// Save and restore
	orig := os.Getenv("SHELL")
	defer os.Setenv("SHELL", orig)

	t.Run("from env", func(t *testing.T) {
		os.Setenv("SHELL", "/bin/zsh")
		if got := resolveShell(); got != "/bin/zsh" {
			t.Fatalf("got %q, want /bin/zsh", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		os.Setenv("SHELL", "")
		if got := resolveShell(); got != "/bin/sh" {
			t.Fatalf("got %q, want /bin/sh", got)
		}
	})
}

func TestResolveShellEnvFallbackTimeoutMs(t *testing.T) {
	orig := os.Getenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS")
	origCrab := os.Getenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS")
	defer os.Setenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS", orig)
	defer os.Setenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS", origCrab)

	t.Run("default", func(t *testing.T) {
		os.Setenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS", "")
		os.Setenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS", "")
		if got := ResolveShellEnvFallbackTimeoutMs(); got != defaultShellEnvTimeoutMs {
			t.Fatalf("got %d, want %d", got, defaultShellEnvTimeoutMs)
		}
	})

	t.Run("custom", func(t *testing.T) {
		os.Setenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS", "5000")
		os.Setenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS", "")
		if got := ResolveShellEnvFallbackTimeoutMs(); got != 5000 {
			t.Fatalf("got %d, want 5000", got)
		}
	})

	t.Run("crabclaw override wins", func(t *testing.T) {
		os.Setenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS", "5000")
		os.Setenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS", "7000")
		if got := ResolveShellEnvFallbackTimeoutMs(); got != 7000 {
			t.Fatalf("got %d, want 7000", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		os.Setenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS", "abc")
		os.Setenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS", "")
		if got := ResolveShellEnvFallbackTimeoutMs(); got != defaultShellEnvTimeoutMs {
			t.Fatalf("got %d, want %d", got, defaultShellEnvTimeoutMs)
		}
	})

	t.Run("negative clamped to zero", func(t *testing.T) {
		os.Setenv("OPENACOSMI_SHELL_ENV_TIMEOUT_MS", "-100")
		os.Setenv("CRABCLAW_SHELL_ENV_TIMEOUT_MS", "")
		if got := ResolveShellEnvFallbackTimeoutMs(); got != 0 {
			t.Fatalf("got %d, want 0", got)
		}
	})
}

func TestShouldEnableShellEnvFallback(t *testing.T) {
	orig := os.Getenv("OPENACOSMI_LOAD_SHELL_ENV")
	origCrab := os.Getenv("CRABCLAW_LOAD_SHELL_ENV")
	defer os.Setenv("OPENACOSMI_LOAD_SHELL_ENV", orig)
	defer os.Setenv("CRABCLAW_LOAD_SHELL_ENV", origCrab)

	os.Setenv("OPENACOSMI_LOAD_SHELL_ENV", "true")
	os.Setenv("CRABCLAW_LOAD_SHELL_ENV", "")
	if !ShouldEnableShellEnvFallback() {
		t.Fatal("expected true when env=true")
	}

	os.Setenv("OPENACOSMI_LOAD_SHELL_ENV", "")
	os.Setenv("CRABCLAW_LOAD_SHELL_ENV", "")
	if ShouldEnableShellEnvFallback() {
		t.Fatal("expected false when env is empty")
	}

	os.Setenv("OPENACOSMI_LOAD_SHELL_ENV", "")
	os.Setenv("CRABCLAW_LOAD_SHELL_ENV", "true")
	if !ShouldEnableShellEnvFallback() {
		t.Fatal("expected true when crabclaw env=true")
	}
}

func TestLoadShellEnvFallbackDisabled(t *testing.T) {
	result := LoadShellEnvFallback(ShellEnvFallbackOptions{
		Enabled:      false,
		ExpectedKeys: []string{"FOO"},
	})
	if !result.OK {
		t.Fatal("expected OK=true for disabled")
	}
	if result.SkippedReason != "disabled" {
		t.Fatalf("SkippedReason = %q, want disabled", result.SkippedReason)
	}
}

func TestGetShellEnvAppliedKeys(t *testing.T) {
	// After disabled call, applied keys should be empty
	LoadShellEnvFallback(ShellEnvFallbackOptions{Enabled: false})
	keys := GetShellEnvAppliedKeys()
	if len(keys) != 0 {
		t.Fatalf("expected empty applied keys, got %v", keys)
	}
}
