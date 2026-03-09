// nativemsg/bridge.go — Bidirectional bridge: Chrome Native Messaging ↔ WebSocket relay.
//
// The bridge translates Chrome's native messaging protocol (4-byte length + JSON on stdio)
// into WebSocket messages for the existing Extension Relay server. This gives the Chrome
// extension a native messaging host that provides "strong keepalive" for the MV3 Service Worker.
package nativemsg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// BridgeConfig configures the native messaging bridge.
type BridgeConfig struct {
	RelayURL  string // WebSocket URL of the Extension Relay (e.g., ws://127.0.0.1:19004/ws)
	AuthToken string // Relay auth token
	Logger    *slog.Logger
	Stdin     io.Reader
	Stdout    io.Writer
}

// Bridge is a bidirectional native-messaging-to-WebSocket bridge.
type Bridge struct {
	cfg    BridgeConfig
	ws     *websocket.Conn
	wsMu   sync.Mutex
	writeMu sync.Mutex // serializes writes to stdout
}

// Run starts the bridge and blocks until ctx is cancelled or stdin is closed.
func (b *Bridge) Run(ctx context.Context) error {
	b.cfg.Logger.Info("native messaging bridge starting", "relayURL", b.cfg.RelayURL)

	if err := b.connectWS(ctx); err != nil {
		return fmt.Errorf("initial WebSocket connect: %w", err)
	}
	defer b.closeWS()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)

	// Goroutine 1: stdin (Chrome) → WebSocket (relay)
	go func() {
		errCh <- b.stdinToWS(ctx)
	}()

	// Goroutine 2: WebSocket (relay) → stdout (Chrome)
	go func() {
		errCh <- b.wsToStdout(ctx)
	}()

	// Wait for first error or context cancellation.
	select {
	case err := <-errCh:
		cancel()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// connectWS dials the Extension Relay WebSocket.
func (b *Bridge) connectWS(ctx context.Context) error {
	url := b.cfg.RelayURL
	if b.cfg.AuthToken != "" {
		url += "?token=" + b.cfg.AuthToken
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("dial relay %s: %w", b.cfg.RelayURL, err)
	}

	b.wsMu.Lock()
	b.ws = conn
	b.wsMu.Unlock()

	b.cfg.Logger.Info("connected to relay")
	return nil
}

func (b *Bridge) closeWS() {
	b.wsMu.Lock()
	defer b.wsMu.Unlock()
	if b.ws != nil {
		b.ws.Close()
		b.ws = nil
	}
}

// stdinToWS reads native messaging frames from stdin and forwards to WebSocket.
func (b *Bridge) stdinToWS(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := ReadMessage(b.cfg.Stdin)
		if err != nil {
			// stdin closed = Chrome disconnected the native port.
			return fmt.Errorf("stdin read: %w", err)
		}

		// Validate JSON.
		if !json.Valid(msg) {
			b.cfg.Logger.Warn("invalid JSON from Chrome, skipping", "size", len(msg))
			continue
		}

		b.wsMu.Lock()
		ws := b.ws
		b.wsMu.Unlock()

		if ws == nil {
			b.cfg.Logger.Warn("WebSocket not connected, dropping message")
			continue
		}

		ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
			return fmt.Errorf("ws write: %w", err)
		}
	}
}

// wsToStdout reads WebSocket messages from relay and writes native messaging frames to stdout.
func (b *Bridge) wsToStdout(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		b.wsMu.Lock()
		ws := b.ws
		b.wsMu.Unlock()

		if ws == nil {
			return fmt.Errorf("WebSocket closed")
		}

		_, msg, err := ws.ReadMessage()
		if err != nil {
			return fmt.Errorf("ws read: %w", err)
		}

		// Serialize stdout writes (only one writer at a time).
		b.writeMu.Lock()
		err = WriteMessage(b.cfg.Stdout, msg)
		b.writeMu.Unlock()

		if err != nil {
			return fmt.Errorf("stdout write: %w", err)
		}
	}
}

// NewBridge creates a new native messaging bridge.
func NewBridge(cfg BridgeConfig) *Bridge {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Bridge{cfg: cfg}
}
