package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/gateway"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

const (
	embeddedDesktopControlUISubdir = "frontend/dist"
	desktopControlUIIndex          = "index.html"
)

var desktopEmbeddedAssetsFunc = desktopEmbeddedControlUIFS

func embeddedDesktopGatewayOptions(root fs.FS, subdir string) (gateway.GatewayServerOptions, error) {
	if root == nil {
		return gateway.GatewayServerOptions{}, fmt.Errorf("embedded control UI fs is required")
	}

	subdir = strings.Trim(strings.TrimSpace(subdir), "/")
	controlUIFS := root
	if subdir != "" {
		sub, err := fs.Sub(root, subdir)
		if err != nil {
			return gateway.GatewayServerOptions{}, fmt.Errorf("resolve embedded control UI subdir %q: %w", subdir, err)
		}
		controlUIFS = sub
	}
	info, err := fs.Stat(controlUIFS, "index.html")
	if err != nil {
		return gateway.GatewayServerOptions{}, fmt.Errorf("embedded control UI missing index.html: %w", err)
	}
	if info.IsDir() {
		return gateway.GatewayServerOptions{}, fmt.Errorf("embedded control UI index.html resolved to a directory")
	}

	return gateway.GatewayServerOptions{
		ControlUIFS:    controlUIFS,
		ControlUIIndex: "index.html",
	}, nil
}

func resolveDesktopGatewayOptions(cfg *types.OpenAcosmiConfig, controlUIDir string, stat statFunc) gateway.GatewayServerOptions {
	if stat == nil {
		stat = os.Stat
	}

	controlUIDir = strings.TrimSpace(controlUIDir)
	if controlUIDir != "" {
		return gateway.GatewayServerOptions{
			ControlUIDir:   controlUIDir,
			ControlUIIndex: desktopControlUIIndex,
		}
	}

	if embedded, ok := resolveEmbeddedDesktopGatewayOptions(); ok {
		return embedded
	}

	if controlUIDir = resolveDesktopControlUIDir(cfg, stat); controlUIDir != "" {
		return gateway.GatewayServerOptions{
			ControlUIDir:   controlUIDir,
			ControlUIIndex: desktopControlUIIndex,
		}
	}

	return gateway.GatewayServerOptions{
		ControlUIIndex: desktopControlUIIndex,
	}
}

func resolveEmbeddedDesktopGatewayOptions() (gateway.GatewayServerOptions, bool) {
	if desktopEmbeddedAssetsFunc == nil {
		return gateway.GatewayServerOptions{}, false
	}
	root := desktopEmbeddedAssetsFunc()
	if root == nil {
		return gateway.GatewayServerOptions{}, false
	}
	opts, err := embeddedDesktopGatewayOptions(root, embeddedDesktopControlUISubdir)
	if err != nil {
		return gateway.GatewayServerOptions{}, false
	}
	return opts, true
}

func resolveDesktopControlUIDir(cfg *types.OpenAcosmiConfig, stat statFunc) string {
	for _, candidate := range desktopControlUICandidates(cfg) {
		if candidate == "" {
			continue
		}
		indexPath := filepath.Join(candidate, desktopControlUIIndex)
		info, err := stat(indexPath)
		if err != nil || info == nil || info.IsDir() {
			continue
		}
		return candidate
	}
	return ""
}

func desktopControlUICandidates(cfg *types.OpenAcosmiConfig) []string {
	seen := map[string]struct{}{}
	var candidates []string
	appendCandidate := func(path string) {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}

	if cfg != nil && cfg.Gateway != nil && cfg.Gateway.ControlUI != nil {
		appendCandidate(cfg.Gateway.ControlUI.Root)
	}

	appendCandidate("dist/control-ui")
	appendCandidate("../dist/control-ui")
	appendCandidate("../../dist/control-ui")
	appendCandidate("../../../dist/control-ui")

	return candidates
}
