package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/gateway"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

type fakeRuntime struct {
	closeCalls []string
}

func (f *fakeRuntime) Close(reason string) error {
	f.closeCalls = append(f.closeCalls, reason)
	return nil
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "index.html" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

func TestResolveDesktopPort(t *testing.T) {
	port := 23456
	cfg := &types.OpenAcosmiConfig{
		Gateway: &types.GatewayConfig{Port: &port},
	}

	if got := resolveDesktopPort(cfg, 45678); got != 45678 {
		t.Fatalf("expected override port 45678, got %d", got)
	}
	if got := resolveDesktopPort(cfg, 0); got != 23456 {
		t.Fatalf("expected config port 23456, got %d", got)
	}
}

func TestBuildDesktopURL(t *testing.T) {
	if got := buildDesktopURL(19001, false); got != "http://127.0.0.1:19001/ui/" {
		t.Fatalf("unexpected desktop url: %s", got)
	}
	if got := buildDesktopURL(19001, true); got != "http://127.0.0.1:19001/ui/?onboarding=true" {
		t.Fatalf("unexpected onboarding url: %s", got)
	}
}

func TestNeedsOnboarding(t *testing.T) {
	notFound := func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	if !needsOnboarding("/tmp/missing.json", notFound) {
		t.Fatal("expected missing config to require onboarding")
	}

	exists := func(string) (os.FileInfo, error) {
		return nil, nil
	}
	if needsOnboarding("/tmp/existing.json", exists) {
		t.Fatal("expected existing config to skip onboarding")
	}
}

func TestPrepareDesktopBootstrap_AttachesExistingGateway(t *testing.T) {
	calledStart := false
	wait := func(port int, timeout time.Duration) bool {
		return true
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		calledStart = true
		return nil, nil
	}

	bootstrap, err := prepareDesktopBootstrap(
		&types.OpenAcosmiConfig{},
		"/tmp/missing.json",
		19001,
		defaultDesktopGatewayOptions(""),
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		wait,
		start,
		nil,
	)
	if err != nil {
		t.Fatalf("prepareDesktopBootstrap returned error: %v", err)
	}
	if calledStart {
		t.Fatal("expected existing gateway attach to skip start")
	}
	if !bootstrap.AttachedExisting {
		t.Fatal("expected attachedExisting=true")
	}
	if bootstrap.Runtime != nil {
		t.Fatal("expected runtime to be nil when attaching existing gateway")
	}
}

func TestPrepareDesktopBootstrap_StartsGatewayAndBuildsURL(t *testing.T) {
	fake := &fakeRuntime{}
	waitCalls := 0
	wait := func(port int, timeout time.Duration) bool {
		waitCalls++
		return waitCalls == 2
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		if opts.ControlUIIndex != "index.html" {
			t.Fatalf("expected default control UI index, got %q", opts.ControlUIIndex)
		}
		return fake, nil
	}

	bootstrap, err := prepareDesktopBootstrap(
		&types.OpenAcosmiConfig{},
		"/tmp/existing.json",
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		func(string) (os.FileInfo, error) { return nil, nil },
		wait,
		start,
		nil,
	)
	if err != nil {
		t.Fatalf("prepareDesktopBootstrap returned error: %v", err)
	}
	if bootstrap.AttachedExisting {
		t.Fatal("expected a newly started runtime")
	}
	if bootstrap.Runtime != fake {
		t.Fatal("expected runtime to be preserved")
	}
	if bootstrap.URL != "http://127.0.0.1:19001/ui/" {
		t.Fatalf("unexpected bootstrap url: %s", bootstrap.URL)
	}
}

func TestStartOrAttachGateway_TimeoutClosesRuntime(t *testing.T) {
	fake := &fakeRuntime{}
	wait := func(port int, timeout time.Duration) bool {
		return false
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		return fake, nil
	}

	runtime, attached, err := startOrAttachGateway(
		19001,
		defaultDesktopGatewayOptions(""),
		time.Millisecond,
		time.Millisecond,
		wait,
		start,
	)
	if err == nil {
		t.Fatal("expected startup timeout error")
	}
	if attached {
		t.Fatal("expected attachedExisting=false")
	}
	if runtime != nil {
		t.Fatal("expected runtime to be nil on timeout")
	}
	if len(fake.closeCalls) != 1 {
		t.Fatalf("expected runtime.Close to be called once, got %d", len(fake.closeCalls))
	}
}

func TestPrepareDesktopBootstrap_ValidatesCallbacks(t *testing.T) {
	_, err := prepareDesktopBootstrap(nil, "", 0, gateway.GatewayServerOptions{}, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when callbacks are missing")
	}
	if err.Error() != "desktop bootstrap requires wait callback" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareDesktopBootstrap_ProbeFailureClosesRuntime(t *testing.T) {
	fake := &fakeRuntime{}
	waitCalls := 0
	wait := func(port int, timeout time.Duration) bool {
		waitCalls++
		return waitCalls == 2
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		return fake, nil
	}
	probe := func(url string, timeout time.Duration) error {
		return os.ErrNotExist
	}

	_, err := prepareDesktopBootstrap(
		&types.OpenAcosmiConfig{},
		"/tmp/existing.json",
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		func(string) (os.FileInfo, error) { return nil, nil },
		wait,
		start,
		probe,
	)
	if err == nil {
		t.Fatal("expected probe failure")
	}
	if len(fake.closeCalls) != 1 {
		t.Fatalf("expected runtime.Close to be called once, got %d", len(fake.closeCalls))
	}
}

func TestStartOrAttachGateway_RequiresControlUIWhenStarting(t *testing.T) {
	wait := func(port int, timeout time.Duration) bool {
		return false
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		t.Fatal("start should not be called without a control UI source")
		return nil, nil
	}

	_, _, err := startOrAttachGateway(
		19001,
		gateway.GatewayServerOptions{},
		time.Millisecond,
		time.Millisecond,
		wait,
		start,
	)
	if err == nil {
		t.Fatal("expected missing control UI source to fail")
	}
}

func TestEmbeddedDesktopGatewayOptions(t *testing.T) {
	opts, err := embeddedDesktopGatewayOptions(fstest.MapFS{
		"frontend/dist/index.html":    &fstest.MapFile{Data: []byte("<html>ok</html>")},
		"frontend/dist/assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}, "frontend/dist")
	if err != nil {
		t.Fatalf("embeddedDesktopGatewayOptions returned error: %v", err)
	}
	if opts.ControlUIFS == nil {
		t.Fatal("expected embedded control UI fs to be set")
	}
	if opts.ControlUIIndex != "index.html" {
		t.Fatalf("unexpected control UI index: %q", opts.ControlUIIndex)
	}
}

func TestEmbeddedDesktopGatewayOptions_RequiresExistingSubdir(t *testing.T) {
	_, err := embeddedDesktopGatewayOptions(fstest.MapFS{}, "frontend/dist")
	if err == nil {
		t.Fatal("expected missing embedded subdir to fail")
	}
}

func TestResolveDesktopGatewayOptions_PrefersOverride(t *testing.T) {
	opts := resolveDesktopGatewayOptions(nil, "/tmp/control-ui", nil)
	if opts.ControlUIDir != "/tmp/control-ui" {
		t.Fatalf("expected override control UI dir, got %q", opts.ControlUIDir)
	}
	if opts.ControlUIFS != nil {
		t.Fatal("expected override to skip embedded fs")
	}
}

func TestResolveDesktopGatewayOptions_UsesEmbeddedAssets(t *testing.T) {
	restore := desktopEmbeddedAssetsFunc
	desktopEmbeddedAssetsFunc = func() fs.FS {
		return fstest.MapFS{
			"frontend/dist/index.html": &fstest.MapFile{Data: []byte("<html>ok</html>")},
		}
	}
	defer func() {
		desktopEmbeddedAssetsFunc = restore
	}()

	opts := resolveDesktopGatewayOptions(nil, "", func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	if opts.ControlUIFS == nil {
		t.Fatal("expected embedded control UI fs to be selected")
	}
	if opts.ControlUIDir != "" {
		t.Fatalf("expected no control UI dir when embedded assets are available, got %q", opts.ControlUIDir)
	}
}

func TestResolveDesktopGatewayOptions_UsesConfiguredDiskRoot(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Gateway: &types.GatewayConfig{
			ControlUI: &types.GatewayControlUiConfig{Root: "/workspace/dist/control-ui"},
		},
	}
	opts := resolveDesktopGatewayOptions(cfg, "", func(name string) (os.FileInfo, error) {
		if filepath.Clean(name) == filepath.Clean("/workspace/dist/control-ui/index.html") {
			return fakeFileInfo{}, nil
		}
		return nil, os.ErrNotExist
	})
	if opts.ControlUIDir != "/workspace/dist/control-ui" {
		t.Fatalf("expected configured control UI dir, got %q", opts.ControlUIDir)
	}
}

func TestResolveDesktopGatewayOptions_LeavesSourceEmptyWhenUnavailable(t *testing.T) {
	restore := desktopEmbeddedAssetsFunc
	desktopEmbeddedAssetsFunc = func() fs.FS { return nil }
	defer func() {
		desktopEmbeddedAssetsFunc = restore
	}()

	opts := resolveDesktopGatewayOptions(nil, "", func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	if opts.ControlUIDir != "" || opts.ControlUIFS != nil {
		t.Fatal("expected no control UI source when nothing is available")
	}
	if opts.ControlUIIndex != "index.html" {
		t.Fatalf("unexpected control UI index: %q", opts.ControlUIIndex)
	}
}
