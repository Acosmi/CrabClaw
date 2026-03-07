package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/gateway"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

const (
	desktopAttachProbeTimeout = 1500 * time.Millisecond
	desktopReadyTimeout       = 10 * time.Second
	desktopUIProbeTimeout     = 3 * time.Second
)

type runtimeCloser interface {
	Close(reason string) error
}

type gatewayStartFunc func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error)
type gatewayWaitFunc func(port int, timeout time.Duration) bool
type statFunc func(name string) (os.FileInfo, error)
type desktopURLProbeFunc func(url string, timeout time.Duration) error

type desktopBootstrap struct {
	Port             int
	URL              string
	NeedsOnboarding  bool
	AttachedExisting bool
	Runtime          runtimeCloser
}

func defaultDesktopGatewayOptions(controlUIDir string) gateway.GatewayServerOptions {
	return resolveDesktopGatewayOptions(nil, controlUIDir, os.Stat)
}

func startGatewayRuntime(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
	return gateway.StartGatewayServer(port, opts)
}

func prepareDesktopBootstrap(
	cfg *types.OpenAcosmiConfig,
	configPath string,
	portOverride int,
	opts gateway.GatewayServerOptions,
	stat statFunc,
	wait gatewayWaitFunc,
	start gatewayStartFunc,
	probe desktopURLProbeFunc,
) (*desktopBootstrap, error) {
	if wait == nil {
		return nil, errors.New("desktop bootstrap requires wait callback")
	}
	if start == nil {
		return nil, errors.New("desktop bootstrap requires start callback")
	}
	if stat == nil {
		stat = os.Stat
	}

	port := resolveDesktopPort(cfg, portOverride)
	runtime, attachedExisting, err := startOrAttachGateway(port, opts, desktopAttachProbeTimeout, desktopReadyTimeout, wait, start)
	if err != nil {
		return nil, err
	}

	onboarding := needsOnboarding(configPath, stat)
	bootstrap := &desktopBootstrap{
		Port:             port,
		URL:              buildDesktopURL(port, onboarding),
		NeedsOnboarding:  onboarding,
		AttachedExisting: attachedExisting,
		Runtime:          runtime,
	}
	if probe != nil {
		if err := probe(bootstrap.URL, desktopUIProbeTimeout); err != nil {
			if bootstrap.Runtime != nil {
				_ = bootstrap.Runtime.Close("desktop control UI probe failed")
			}
			return nil, fmt.Errorf("desktop control UI probe failed: %w", err)
		}
	}
	return bootstrap, nil
}

func resolveDesktopPort(cfg *types.OpenAcosmiConfig, portOverride int) int {
	if portOverride > 0 {
		return portOverride
	}
	var cfgPort *int
	if cfg != nil && cfg.Gateway != nil && cfg.Gateway.Port != nil {
		cfgPort = cfg.Gateway.Port
	}
	return config.ResolveGatewayPort(cfgPort)
}

func needsOnboarding(configPath string, stat statFunc) bool {
	if configPath == "" {
		configPath = config.ResolveConfigPath()
	}
	_, err := stat(configPath)
	return errors.Is(err, os.ErrNotExist)
}

func buildDesktopURL(port int, onboarding bool) string {
	if onboarding {
		return fmt.Sprintf("http://127.0.0.1:%d/ui/?onboarding=true", port)
	}
	return fmt.Sprintf("http://127.0.0.1:%d/ui/", port)
}

func startOrAttachGateway(
	port int,
	opts gateway.GatewayServerOptions,
	attachProbeTimeout time.Duration,
	readyTimeout time.Duration,
	wait gatewayWaitFunc,
	start gatewayStartFunc,
) (runtimeCloser, bool, error) {
	if wait(port, attachProbeTimeout) {
		return nil, true, nil
	}
	if !hasDesktopControlUISource(opts) {
		return nil, false, errors.New("desktop control UI assets not found; use -control-ui-dir, gateway.controlUi.root, or build with desktopembed")
	}

	runtime, err := start(port, opts)
	if err != nil {
		return nil, false, err
	}
	if wait(port, readyTimeout) {
		return runtime, false, nil
	}
	_ = runtime.Close("desktop bootstrap startup timeout")
	return nil, false, fmt.Errorf("gateway startup timed out on port %d", port)
}

func hasDesktopControlUISource(opts gateway.GatewayServerOptions) bool {
	return opts.ControlUIDir != "" || opts.ControlUIFS != nil
}

func probeDesktopURL(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
