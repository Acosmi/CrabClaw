package infra

// TS 对照: src/infra/ssh-tunnel.ts (214L)
// SSH 端口转发隧道管理

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------- 类型 ----------

// SshParsedTarget 解析后的 SSH 目标。
type SshParsedTarget struct {
	User string
	Host string
	Port int
}

// SshTunnel 活跃的 SSH 隧道。
type SshTunnel struct {
	ParsedTarget SshParsedTarget
	LocalPort    int
	RemotePort   int
	Pid          int
	Stderr       []string
	cmd          *exec.Cmd
	mu           sync.Mutex
	stopped      bool
}

// Stop 停止隧道。
func (t *SshTunnel) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped || t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	t.stopped = true
	_ = t.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = t.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
		_ = t.cmd.Process.Kill()
	}
	return nil
}

// ---------- 解析 ----------

// ParseSshTarget 解析 SSH 目标字符串。
func ParseSshTarget(raw string) *SshParsedTarget {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "ssh ")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil
	}

	var user, hostPart string
	if idx := strings.Index(trimmed, "@"); idx >= 0 {
		user = strings.TrimSpace(trimmed[:idx])
		hostPart = strings.TrimSpace(trimmed[idx+1:])
	} else {
		hostPart = trimmed
	}

	// 安全: 拒绝 - 开头的主机名
	if strings.HasPrefix(hostPart, "-") {
		return nil
	}

	colonIdx := strings.LastIndex(hostPart, ":")
	if colonIdx > 0 && colonIdx < len(hostPart)-1 {
		host := strings.TrimSpace(hostPart[:colonIdx])
		portRaw := strings.TrimSpace(hostPart[colonIdx+1:])
		port, err := strconv.Atoi(portRaw)
		if err != nil || port <= 0 || host == "" {
			return nil
		}
		if strings.HasPrefix(host, "-") {
			return nil
		}
		return &SshParsedTarget{User: user, Host: host, Port: port}
	}

	if hostPart == "" {
		return nil
	}
	return &SshParsedTarget{User: user, Host: hostPart, Port: 22}
}

// ---------- 端口辅助 ----------

func pickEphemeralPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func canConnectLocal(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func waitForLocalListener(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("ssh tunnel did not start on localhost:%d", port)
		default:
			if canConnectLocal(port) {
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// ---------- 启动 ----------

// SshPortForwardOpts 端口转发选项。
type SshPortForwardOpts struct {
	Target             string
	Identity           string
	LocalPortPreferred int
	RemotePort         int
	TimeoutMs          int
}

// StartSshPortForward 启动 SSH 端口转发。
func StartSshPortForward(ctx context.Context, opts SshPortForwardOpts) (*SshTunnel, error) {
	parsed := ParseSshTarget(opts.Target)
	if parsed == nil {
		return nil, fmt.Errorf("invalid SSH target: %s", opts.Target)
	}

	localPort := opts.LocalPortPreferred
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		localPort, err = pickEphemeralPort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}
	} else {
		l.Close()
	}

	userHost := parsed.Host
	if parsed.User != "" {
		userHost = parsed.User + "@" + parsed.Host
	}

	args := []string{
		"-N",
		"-L", fmt.Sprintf("%d:127.0.0.1:%d", localPort, opts.RemotePort),
		"-p", strconv.Itoa(parsed.Port),
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UpdateHostKeys=yes",
		"-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	}
	if id := strings.TrimSpace(opts.Identity); id != "" {
		args = append(args, "-i", id)
	}
	args = append(args, "--", userHost)

	tunnel := &SshTunnel{
		ParsedTarget: *parsed,
		LocalPort:    localPort,
		RemotePort:   opts.RemotePort,
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/ssh", args...)
	tunnel.cmd = cmd

	stderrPipe, stderrErr := cmd.StderrPipe()
	if stderrErr != nil {
		slog.Warn("ssh_tunnel: failed to get stderr pipe", "err", stderrErr)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ssh start failed: %w", err)
	}
	tunnel.Pid = cmd.Process.Pid

	// 后台收集 stderr
	if stderrPipe != nil {
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := stderrPipe.Read(buf)
				if n > 0 {
					lines := strings.Split(strings.TrimSpace(string(buf[:n])), "\n")
					tunnel.mu.Lock()
					tunnel.Stderr = append(tunnel.Stderr, lines...)
					tunnel.mu.Unlock()
				}
				if err != nil {
					break
				}
			}
		}()
	}

	timeoutMs := opts.TimeoutMs
	if timeoutMs < 250 {
		timeoutMs = 250
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// 等待隧道就绪 vs ssh 退出
	errCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			errCh <- fmt.Errorf("ssh exited: %w", err)
		} else {
			errCh <- fmt.Errorf("ssh exited unexpectedly (code 0)")
		}
	}()

	listenerErr := make(chan error, 1)
	go func() {
		listenerErr <- waitForLocalListener(ctx, localPort, timeout)
	}()

	select {
	case err := <-errCh:
		tunnel.mu.Lock()
		stderrMsg := strings.Join(tunnel.Stderr, "\n")
		tunnel.mu.Unlock()
		if stderrMsg != "" {
			return nil, fmt.Errorf("%s\n%s", err.Error(), stderrMsg)
		}
		return nil, err
	case err := <-listenerErr:
		if err != nil {
			_ = tunnel.Stop()
			return nil, err
		}
	}

	return tunnel, nil
}
