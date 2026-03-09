// cmd/native-host — CrabClaw Native Messaging Host for Chrome extension.
//
// This binary is spawned by Chrome when the extension calls
// chrome.runtime.connectNative('com.acosmi.crabclaw'). It bridges
// Chrome's native messaging protocol (stdio) to the Extension Relay (WebSocket).
//
// Usage:
//   crabclaw-native-host              # Normal mode: bridge stdio ↔ WebSocket
//   crabclaw-native-host --install    # Install native messaging host manifest
//   crabclaw-native-host --uninstall  # Remove native messaging host manifest
//
// The host auto-discovers the relay URL and auth token from the CrabClaw state directory.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Acosmi/ClawAcosmi/internal/browser/nativemsg"
	"github.com/Acosmi/ClawAcosmi/internal/config"
)

func main() {
	installFlag := flag.Bool("install", false, "Install native messaging host manifest for Chrome")
	uninstallFlag := flag.Bool("uninstall", false, "Remove native messaging host manifest")
	extensionID := flag.String("extension-id", "ijkcckheapdhooinidgdccbgabahmgnl", "Chrome extension ID (for manifest installation)")
	flag.Parse()

	// All logging goes to stderr (stdout is the native messaging channel).
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *uninstallFlag {
		nativemsg.Uninstall()
		fmt.Fprintln(os.Stderr, "Native messaging host manifest removed.")
		return
	}

	if *installFlag {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot resolve executable path: %v\n", err)
			os.Exit(1)
		}
		n, err := nativemsg.Install(nativemsg.InstallConfig{
			HostBinaryPath: exe,
			ExtensionIDs:   []string{*extensionID},
			Logger:         logger,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Installed %d manifest(s) for extension %s\n", n, *extensionID)
		return
	}

	// Normal bridge mode: stdin/stdout ↔ WebSocket relay.
	relayURL, authToken := discoverRelay(logger)
	if relayURL == "" {
		logger.Error("cannot discover relay URL — is CrabClaw gateway running?")
		os.Exit(1)
	}

	bridge := nativemsg.NewBridge(nativemsg.BridgeConfig{
		RelayURL:  relayURL,
		AuthToken: authToken,
		Logger:    logger,
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := bridge.Run(ctx); err != nil {
		logger.Error("bridge error", "err", err)
		os.Exit(1)
	}
}

// discoverRelay finds the relay URL and auth token from the CrabClaw state directory.
func discoverRelay(logger *slog.Logger) (relayURL, authToken string) {
	stateDir := config.ResolveStateDir()

	// Read persisted relay token.
	tokenFile := filepath.Join(stateDir, "relay-token")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		logger.Warn("cannot read relay token file", "path", tokenFile, "err", err)
	} else {
		authToken = strings.TrimSpace(string(data))
	}

	// Derive relay port from gateway port.
	gatewayPort := config.ResolveGatewayPort(nil)
	relayPort := config.DeriveDefaultBrowserControlPort(gatewayPort) + 1

	relayURL = fmt.Sprintf("ws://127.0.0.1:%d/ws", relayPort)
	logger.Info("discovered relay", "url", relayURL, "hasToken", authToken != "")
	return relayURL, authToken
}
