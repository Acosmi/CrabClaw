package mcpinstall

// manager.go — McpLocalManager: manages multiple McpLocalBridge instances.
//
// Responsibilities:
// - Load registry at startup → auto-start configured servers
// - Start/stop individual bridges
// - Provide aggregated tool list for Agent integration
// - Gateway shutdown → graceful stop all bridges

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// McpLocalManager manages all locally installed MCP server bridges.
type McpLocalManager struct {
	mu       sync.RWMutex
	bridges  map[string]*McpLocalBridge
	registry *McpServerRegistry
	regPath  string
}

// NewMcpLocalManager creates a new manager and loads the registry.
func NewMcpLocalManager() (*McpLocalManager, error) {
	regPath, err := DefaultRegistryPath()
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: registry path: %w", err)
	}

	reg, err := LoadRegistry(regPath)
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: load registry: %w", err)
	}

	return &McpLocalManager{
		bridges:  make(map[string]*McpLocalBridge),
		registry: reg,
		regPath:  regPath,
	}, nil
}

// StartAll starts bridges for all registered servers.
// Errors are logged but do not prevent other servers from starting.
func (m *McpLocalManager) StartAll(ctx context.Context) {
	m.mu.Lock()
	servers := make([]InstalledMcpServer, 0, len(m.registry.Servers))
	for _, s := range m.registry.Servers {
		servers = append(servers, s)
	}
	m.mu.Unlock()

	for _, server := range servers {
		if err := m.startBridge(ctx, server); err != nil {
			slog.Warn("mcpinstall: failed to start server",
				"name", server.Name,
				"error", err,
			)
		}
	}
}

// startBridge creates and starts a bridge for a server.
func (m *McpLocalManager) startBridge(ctx context.Context, server InstalledMcpServer) error {
	bridge := NewMcpLocalBridge(McpLocalBridgeConfig{
		Server: server,
	})

	if err := bridge.Start(ctx); err != nil {
		return err
	}

	m.mu.Lock()
	m.bridges[server.Name] = bridge
	m.mu.Unlock()

	return nil
}

// StartServer starts a specific server by name.
func (m *McpLocalManager) StartServer(ctx context.Context, name string) error {
	m.mu.RLock()
	server, ok := m.registry.Servers[name]
	existing := m.bridges[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("mcpinstall: server %q not found in registry", name)
	}
	if existing != nil && existing.State() == BridgeStateReady {
		return fmt.Errorf("mcpinstall: server %q already running", name)
	}

	// Stop existing bridge if in degraded state
	if existing != nil {
		existing.Stop()
	}

	return m.startBridge(ctx, server)
}

// StopServer stops a specific server by name.
func (m *McpLocalManager) StopServer(name string) error {
	m.mu.Lock()
	bridge, ok := m.bridges[name]
	if ok {
		delete(m.bridges, name)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("mcpinstall: server %q not running", name)
	}

	bridge.Stop()
	return nil
}

// StopAll stops all running bridges gracefully.
func (m *McpLocalManager) StopAll() {
	m.mu.Lock()
	bridges := make(map[string]*McpLocalBridge, len(m.bridges))
	for k, v := range m.bridges {
		bridges[k] = v
	}
	m.bridges = make(map[string]*McpLocalBridge)
	m.mu.Unlock()

	for name, bridge := range bridges {
		slog.Info("mcpinstall: stopping server", "name", name)
		bridge.Stop()
	}
}

// RegisterServer adds a server to the registry and optionally starts it.
func (m *McpLocalManager) RegisterServer(ctx context.Context, server InstalledMcpServer, autoStart bool) error {
	m.mu.Lock()
	m.registry.Servers[server.Name] = server
	m.mu.Unlock()

	// Persist
	if err := SaveRegistry(m.regPath, m.registry); err != nil {
		return fmt.Errorf("mcpinstall: save registry: %w", err)
	}

	if autoStart {
		return m.startBridge(ctx, server)
	}
	return nil
}

// UnregisterServer removes a server from the registry and stops it.
func (m *McpLocalManager) UnregisterServer(name string) error {
	// Stop if running
	m.mu.Lock()
	if bridge, ok := m.bridges[name]; ok {
		bridge.Stop()
		delete(m.bridges, name)
	}
	delete(m.registry.Servers, name)
	m.mu.Unlock()

	return SaveRegistry(m.regPath, m.registry)
}

// ListServers returns all registered servers with their runtime status.
func (m *McpLocalManager) ListServers() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ServerStatus
	for _, server := range m.registry.Servers {
		status := ServerStatus{
			Server: server,
			State:  BridgeStateStopped,
		}
		if bridge, ok := m.bridges[server.Name]; ok {
			status.State = bridge.State()
			status.Tools = len(bridge.Tools())
		}
		result = append(result, status)
	}
	return result
}

// ServerStatus combines registry info with runtime state.
type ServerStatus struct {
	Server InstalledMcpServer `json:"server"`
	State  BridgeState        `json:"state"`
	Tools  int                `json:"tools"`
}

// GetServerStatus returns status for a specific server.
func (m *McpLocalManager) GetServerStatus(name string) (*ServerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	server, ok := m.registry.Servers[name]
	if !ok {
		return nil, fmt.Errorf("mcpinstall: server %q not found", name)
	}

	status := &ServerStatus{
		Server: server,
		State:  BridgeStateStopped,
	}
	if bridge, ok := m.bridges[name]; ok {
		status.State = bridge.State()
		status.Tools = len(bridge.Tools())
	}
	return status, nil
}

// AllTools returns all tools from all ready bridges, prefixed with `mcp_{server}_`.
func (m *McpLocalManager) AllTools() []AgentMcpTool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []AgentMcpTool
	for name, bridge := range m.bridges {
		if bridge.State() != BridgeStateReady {
			continue
		}
		for _, tool := range bridge.Tools() {
			result = append(result, AgentMcpTool{
				ServerName:   name,
				Tool:         tool,
				PrefixedName: "mcp_" + name + "_" + tool.Name,
			})
		}
	}
	return result
}

// AgentMcpTool wraps an MCP tool with server context for Agent integration.
type AgentMcpTool struct {
	ServerName   string  `json:"server_name"`
	Tool         McpTool `json:"tool"`
	PrefixedName string  `json:"prefixed_name"`
}

// CallTool calls a tool on the appropriate bridge.
// `prefixedName` is in format `mcp_{server}_{tool}`.
func (m *McpLocalManager) CallTool(ctx context.Context, prefixedName string, arguments json.RawMessage, timeout time.Duration) (string, error) {
	serverName, toolName, err := parsePrefixedToolName(prefixedName)
	if err != nil {
		return "", err
	}

	m.mu.RLock()
	bridge, ok := m.bridges[serverName]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("mcpinstall: server %q not running", serverName)
	}

	result, err := bridge.CallTool(ctx, toolName, arguments, timeout)
	if err != nil {
		return "", err
	}

	return McpToolCallResultToText(result), nil
}

// parsePrefixedToolName extracts server and tool names from "mcp_{server}_{tool}".
func parsePrefixedToolName(prefixed string) (serverName, toolName string, err error) {
	const prefix = "mcp_"
	if len(prefixed) < len(prefix) || prefixed[:len(prefix)] != prefix {
		return "", "", fmt.Errorf("mcpinstall: invalid tool name prefix: %q", prefixed)
	}

	rest := prefixed[len(prefix):]
	// Find the first underscore that separates server name from tool name.
	// Server names may contain hyphens but not underscores.
	idx := -1
	for i, c := range rest {
		if c == '_' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", "", fmt.Errorf("mcpinstall: cannot parse tool name: %q (expected mcp_<server>_<tool>)", prefixed)
	}

	return rest[:idx], rest[idx+1:], nil
}

// ReloadRegistry re-reads registry.json from disk.
func (m *McpLocalManager) ReloadRegistry() error {
	reg, err := LoadRegistry(m.regPath)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.registry = reg
	m.mu.Unlock()
	return nil
}
