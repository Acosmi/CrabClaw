package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// --- CLI-P2-3: RewriteUpdateFlagArgv ---

func TestRewriteUpdateFlagArgv_NoFlag(t *testing.T) {
	argv := []string{"openacosmi", "message", "send"}
	result := RewriteUpdateFlagArgv(argv)
	// 无 --update flag 时返回原始 slice
	if &result[0] != &argv[0] {
		t.Error("expected same slice when no --update flag")
	}
}

func TestRewriteUpdateFlagArgv_WithFlag(t *testing.T) {
	argv := []string{"openacosmi", "--update"}
	result := RewriteUpdateFlagArgv(argv)
	if len(result) != 2 || result[1] != "update" {
		t.Errorf("expected [openacosmi update], got %v", result)
	}
}

func TestRewriteUpdateFlagArgv_WithExtraArgs(t *testing.T) {
	argv := []string{"openacosmi", "--profile", "p", "--update", "--json"}
	result := RewriteUpdateFlagArgv(argv)
	expected := []string{"openacosmi", "--profile", "p", "update", "--json"}
	if len(result) != len(expected) {
		t.Fatalf("length mismatch: got %v", result)
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %q, got %q", i, expected[i], result[i])
		}
	}
}

// --- CLI-P2-4: ClearProgressLine ---

func TestClearProgressLine_NoActiveProgress(t *testing.T) {
	// Should be a no-op when no progress is active
	ClearProgressLine()
}

// --- CLI-P2-2: LoadDotEnv ---

func TestLoadDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte(
		"TEST_DOTENV_A=hello\n"+
			"TEST_DOTENV_B=\"world\"\n"+
			"# comment\n"+
			"\n"+
			"export TEST_DOTENV_C=exported\n"+
			"TEST_DOTENV_D='single'\n",
	), 0644)

	// Clean up after test
	defer func() {
		os.Unsetenv("TEST_DOTENV_A")
		os.Unsetenv("TEST_DOTENV_B")
		os.Unsetenv("TEST_DOTENV_C")
		os.Unsetenv("TEST_DOTENV_D")
	}()

	loadDotEnvFile(envFile, true)

	cases := []struct {
		key, want string
	}{
		{"TEST_DOTENV_A", "hello"},
		{"TEST_DOTENV_B", "world"},
		{"TEST_DOTENV_C", "exported"},
		{"TEST_DOTENV_D", "single"},
	}
	for _, tc := range cases {
		if got := os.Getenv(tc.key); got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.key, tc.want, got)
		}
	}
}

func TestLoadDotEnvFile_NoOverride(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("TEST_NOOVERRIDE=new_value\n"), 0644)

	os.Setenv("TEST_NOOVERRIDE", "existing")
	defer os.Unsetenv("TEST_NOOVERRIDE")

	loadDotEnvFile(envFile, true)

	if got := os.Getenv("TEST_NOOVERRIDE"); got != "existing" {
		t.Errorf("expected existing value preserved, got %q", got)
	}
}

func TestLoadDotEnvFile_MissingFile(t *testing.T) {
	// Should not panic on missing file
	loadDotEnvFile("/tmp/nonexistent-dotenv-file", true)
}

// --- CLI-P2-5: ResolveCliChannelOptions ---

func TestResolveCliChannelOptions_Default(t *testing.T) {
	os.Unsetenv("OPENACOSMI_EAGER_CHANNEL_OPTIONS")
	os.Unsetenv("CRABCLAW_EAGER_CHANNEL_OPTIONS")
	opts := ResolveCliChannelOptions()
	if len(opts) != 6 {
		t.Fatalf("expected 6 channels, got %d: %v", len(opts), opts)
	}
	if opts[0] != "whatsapp" || opts[5] != "imessage" {
		t.Errorf("unexpected order: %v", opts)
	}
}

func TestFormatChannelOptions_WithExtra(t *testing.T) {
	os.Unsetenv("OPENACOSMI_EAGER_CHANNEL_OPTIONS")
	os.Unsetenv("CRABCLAW_EAGER_CHANNEL_OPTIONS")
	result := FormatChannelOptions("custom")
	if result == "" {
		t.Fatal("empty result")
	}
	// "custom" should appear first
	if result[:6] != "custom" {
		t.Errorf("expected custom first, got: %s", result)
	}
}

// --- CLI-P3-1: TryRouteCli ---

func TestTryRouteCli_NoRoutes(t *testing.T) {
	ClearRoutedCommandsForTest()
	routed, err := TryRouteCli([]string{"openacosmi", "unknown"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routed {
		t.Error("expected false for unknown command")
	}
}

func TestTryRouteCli_HelpSkipped(t *testing.T) {
	ClearRoutedCommandsForTest()
	RegisterRoutedCommand([]string{"test"}, RoutedCommandHandler{
		Run: func(argv []string) (bool, error) { return true, nil },
	})
	routed, err := TryRouteCli([]string{"test", "--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routed {
		t.Error("expected false when --help present")
	}
	ClearRoutedCommandsForTest()
}

func TestTryRouteCli_Matched(t *testing.T) {
	ClearRoutedCommandsForTest()
	os.Unsetenv("OPENACOSMI_DISABLE_ROUTE_FIRST")
	os.Unsetenv("CRABCLAW_DISABLE_ROUTE_FIRST")

	called := false
	RegisterRoutedCommand([]string{"msg"}, RoutedCommandHandler{
		Run: func(argv []string) (bool, error) {
			called = true
			return true, nil
		},
	})

	routed, err := TryRouteCli([]string{"msg", "send"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !routed || !called {
		t.Error("expected route to be matched and called")
	}
	ClearRoutedCommandsForTest()
}

func TestTryRouteCli_DisabledByEnv(t *testing.T) {
	ClearRoutedCommandsForTest()
	RegisterRoutedCommand([]string{"test"}, RoutedCommandHandler{
		Run: func(argv []string) (bool, error) { return true, nil },
	})

	os.Setenv("OPENACOSMI_DISABLE_ROUTE_FIRST", "1")
	defer os.Unsetenv("OPENACOSMI_DISABLE_ROUTE_FIRST")

	routed, err := TryRouteCli([]string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routed {
		t.Error("expected false when OPENACOSMI_DISABLE_ROUTE_FIRST=1")
	}
	ClearRoutedCommandsForTest()
}

func TestTryRouteCli_TwoLevelMatch(t *testing.T) {
	ClearRoutedCommandsForTest()
	os.Unsetenv("OPENACOSMI_DISABLE_ROUTE_FIRST")
	os.Unsetenv("CRABCLAW_DISABLE_ROUTE_FIRST")

	called := false
	RegisterRoutedCommand([]string{"msg", "send"}, RoutedCommandHandler{
		Run: func(argv []string) (bool, error) {
			called = true
			return true, nil
		},
	})

	routed, err := TryRouteCli([]string{"msg", "send"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !routed || !called {
		t.Error("expected 2-level route match")
	}
	ClearRoutedCommandsForTest()
}

func TestTryRouteCli_DisabledByCrabClawEnv(t *testing.T) {
	ClearRoutedCommandsForTest()
	RegisterRoutedCommand([]string{"test"}, RoutedCommandHandler{
		Run: func(argv []string) (bool, error) { return true, nil },
	})

	os.Setenv("CRABCLAW_DISABLE_ROUTE_FIRST", "1")
	defer os.Unsetenv("CRABCLAW_DISABLE_ROUTE_FIRST")

	routed, err := TryRouteCli([]string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routed {
		t.Error("expected false when CRABCLAW_DISABLE_ROUTE_FIRST=1")
	}
	ClearRoutedCommandsForTest()
}

// --- unquoteDotEnvValue ---

// --- CLI profile validation (TS-MIG-CLI2) ---

func TestIsValidProfileName(t *testing.T) {
	valid := []string{"dev", "my-profile", "test_123", "a", "A-b_C-1"}
	for _, name := range valid {
		if !IsValidProfileName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	invalid := []string{
		"",
		"has space",
		"has/slash",
		"has.dot",
		"../escape",
		string(make([]byte, 65)), // 65 chars
	}
	for _, name := range invalid {
		if IsValidProfileName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestResolveProfile_Valid(t *testing.T) {
	profile, err := ResolveProfile([]string{"openacosmi", "--profile", "my-test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile != "my-test" {
		t.Errorf("expected my-test, got %q", profile)
	}
}

func TestResolveProfile_Dev(t *testing.T) {
	profile, err := ResolveProfile([]string{"openacosmi", "--dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile != "dev" {
		t.Errorf("expected dev, got %q", profile)
	}
}

func TestResolveProfile_InvalidName(t *testing.T) {
	_, err := ResolveProfile([]string{"openacosmi", "--profile", "../escape"})
	if err == nil {
		t.Fatal("expected error for invalid profile name")
	}
}

func TestResolveProfile_Empty(t *testing.T) {
	profile, err := ResolveProfile([]string{"openacosmi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile != "" {
		t.Errorf("expected empty profile, got %q", profile)
	}
}

func TestResolveStateDirPrefersCrabClawEnv(t *testing.T) {
	t.Setenv("OPENACOSMI_STATE_DIR", "/tmp/open-state")
	t.Setenv("CRABCLAW_STATE_DIR", "/tmp/crab-state")
	if got := ResolveStateDir(""); got != "/tmp/crab-state" {
		t.Fatalf("got %q, want %q", got, "/tmp/crab-state")
	}
}

// --- unquoteDotEnvValue ---

func TestUnquoteDotEnvValue(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`noQuotes`, "noQuotes"},
		{`""`, ""},
		{`a`, "a"},
		{``, ""},
	}
	for _, tc := range cases {
		got := unquoteDotEnvValue(tc.in)
		if got != tc.want {
			t.Errorf("unquoteDotEnvValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
