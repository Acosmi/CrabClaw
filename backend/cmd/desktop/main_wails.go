//go:build desktopwails

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

var version = "dev"

func main() {
	portOverride := flag.Int("port", 0, "Desktop gateway port override")
	controlUIDir := flag.String("control-ui-dir", "", "Path to control UI static files for desktop dev mode")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("desktop %s\n", version)
		return
	}

	config.BuildVersion = version

	loader := config.NewConfigLoader()
	cfg, err := loader.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load config failed, continuing with defaults: %v\n", err)
	}
	opts := resolveDesktopGatewayOptions(cfg, *controlUIDir, os.Stat)

	bootstrap, err := prepareDesktopBootstrap(
		cfg,
		loader.ConfigPath(),
		*portOverride,
		opts,
		os.Stat,
		infra.WaitForGateway,
		startGatewayRuntime,
		probeDesktopURL,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "desktop bootstrap failed: %v\n", err)
		os.Exit(1)
	}

	if err := runDesktopWailsApp(bootstrap); err != nil {
		fmt.Fprintf(os.Stderr, "desktop GUI failed: %v\n", err)
		os.Exit(1)
	}
}
