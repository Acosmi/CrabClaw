// Package infra 提供基础设施模块（端口管理、心跳、发现等）。
//
// 本文件对应原版:
//   - src/infra/ports-types.ts (类型定义)
//   - src/infra/ports.ts (核心 API)
//   - src/infra/ports-inspect.ts (lsof/netstat 检查)
//   - src/infra/ports-format.ts (诊断格式化)
//   - src/infra/ports-lsof.ts (lsof 路径解析)
//
// TS 依赖: Node.js net 模块, child_process
// Go 替代: net stdlib, os/exec
package infra

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ─── 类型定义 (ports-types.ts) ───

// PortListener 描述监听指定端口的进程。
type PortListener struct {
	PID         int    `json:"pid,omitempty"`
	Command     string `json:"command,omitempty"`
	CommandLine string `json:"commandLine,omitempty"`
	User        string `json:"user,omitempty"`
	Address     string `json:"address,omitempty"`
}

// PortUsageStatus 端口使用状态。
type PortUsageStatus string

const (
	PortFree    PortUsageStatus = "free"
	PortBusy    PortUsageStatus = "busy"
	PortUnknown PortUsageStatus = "unknown"
)

// PortUsage 端口使用情况的完整诊断结果。
type PortUsage struct {
	Port      int             `json:"port"`
	Status    PortUsageStatus `json:"status"`
	Listeners []PortListener  `json:"listeners"`
	Hints     []string        `json:"hints"`
	Detail    string          `json:"detail,omitempty"`
	Errors    []string        `json:"errors,omitempty"`
}

// PortListenerKind 监听者类型分类。
type PortListenerKind string

const (
	PortKindGateway PortListenerKind = "gateway"
	PortKindSSH     PortListenerKind = "ssh"
	PortKindUnknown PortListenerKind = "unknown"
)

// ─── 错误类型 (ports.ts) ───

// PortInUseError 端口被占用的结构化错误。
type PortInUseError struct {
	Port    int
	Details string
}

func (e *PortInUseError) Error() string {
	msg := fmt.Sprintf("Port %d is already in use", e.Port)
	if e.Details != "" {
		msg += ": " + e.Details
	}
	return msg
}

// ─── 核心 API (ports.ts) ───

// EnsurePortAvailable 检查端口是否可用。
// 如果端口被占用，返回 PortInUseError（附带进程诊断信息）。
func EnsurePortAvailable(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		// 获取占用详情
		details := DescribePortOwner(port)
		return &PortInUseError{Port: port, Details: details}
	}
	_ = ln.Close()
	return nil
}

// DescribePortOwner 返回占用指定端口的进程描述。
// 若无法确定则返回空字符串。
func DescribePortOwner(port int) string {
	usage := InspectPortUsage(port)
	if len(usage.Listeners) == 0 {
		return ""
	}
	lines := FormatPortDiagnostics(usage)
	return strings.Join(lines, "\n")
}

// ─── 端口检查 (ports-inspect.ts) ───

// InspectPortUsage 检查端口的使用情况，包括进程信息。
func InspectPortUsage(port int) PortUsage {
	var errors []string

	listeners, detail, inspectErrors := readListeners(port)
	errors = append(errors, inspectErrors...)

	status := PortUnknown
	if len(listeners) > 0 {
		status = PortBusy
	} else {
		status = checkPortInUse(port)
	}

	if status != PortBusy {
		listeners = nil
	}

	hints := BuildPortHints(listeners, port)
	if status == PortBusy && len(listeners) == 0 {
		hints = append(hints, "Port is in use but process details are unavailable (install lsof or run as an admin user).")
	}

	return PortUsage{
		Port:      port,
		Status:    status,
		Listeners: listeners,
		Hints:     hints,
		Detail:    detail,
		Errors:    errors,
	}
}

// readListeners 根据平台读取端口监听者信息。
func readListeners(port int) (listeners []PortListener, detail string, errors []string) {
	if runtime.GOOS == "windows" {
		return readWindowsListeners(port)
	}
	return readUnixListeners(port)
}

// readUnixListeners 通过 lsof 读取 Unix/macOS 上的端口监听者。
func readUnixListeners(port int) ([]PortListener, string, []string) {
	var errors []string
	lsof := resolveLsofCommand()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, lsof, "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpcn")
	output, err := cmd.CombinedOutput()
	stdout := string(output)

	if err == nil {
		listeners := parseLsofFieldOutput(stdout)
		// 补充 commandLine 和 user 信息
		for i := range listeners {
			if listeners[i].PID == 0 {
				continue
			}
			if cmdLine := resolveUnixCommandLine(listeners[i].PID); cmdLine != "" {
				listeners[i].CommandLine = cmdLine
			}
			if user := resolveUnixUser(listeners[i].PID); user != "" {
				listeners[i].User = user
			}
		}
		detail := strings.TrimSpace(stdout)
		return listeners, detail, errors
	}

	// lsof 返回非零退出码
	trimmed := strings.TrimSpace(stdout)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && trimmed == "" {
		// 退出码 1 + 无输出表示端口空闲
		return nil, "", errors
	}

	if trimmed != "" {
		errors = append(errors, trimmed)
	}
	return nil, "", errors
}

// readWindowsListeners 通过 netstat 读取 Windows 上的端口监听者。
func readWindowsListeners(port int) ([]PortListener, string, []string) {
	var errors []string

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "netstat", "-ano", "-p", "tcp")
	output, err := cmd.CombinedOutput()
	stdout := string(output)

	if err != nil {
		errors = append(errors, fmt.Sprintf("netstat failed: %v", err))
		return nil, "", errors
	}

	listeners := parseNetstatListeners(stdout, port)
	detail := strings.TrimSpace(stdout)
	return listeners, detail, errors
}

// parseLsofFieldOutput 解析 lsof -F 格式输出。
// 字段前缀: p=PID, c=command, n=address
func parseLsofFieldOutput(output string) []PortListener {
	lines := strings.Split(output, "\n")
	var listeners []PortListener
	current := PortListener{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "p"):
			if current.PID != 0 || current.Command != "" {
				listeners = append(listeners, current)
			}
			pid, err := strconv.Atoi(line[1:])
			if err == nil {
				current = PortListener{PID: pid}
			} else {
				current = PortListener{}
			}
		case strings.HasPrefix(line, "c"):
			current.Command = line[1:]
		case strings.HasPrefix(line, "n"):
			if current.Address == "" {
				current.Address = line[1:]
			}
		}
	}
	if current.PID != 0 || current.Command != "" {
		listeners = append(listeners, current)
	}
	return listeners
}

// parseNetstatListeners 解析 netstat 输出（Windows 格式）。
func parseNetstatListeners(output string, port int) []PortListener {
	var listeners []PortListener
	portToken := fmt.Sprintf(":%d", port)

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || !strings.Contains(strings.ToLower(line), "listen") || !strings.Contains(line, portToken) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		listener := PortListener{}
		if pid, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			listener.PID = pid
		}
		localAddr := parts[1]
		if strings.Contains(localAddr, portToken) {
			listener.Address = localAddr
		}
		listeners = append(listeners, listener)
	}
	return listeners
}

// checkPortInUse 通过尝试绑定来检测端口是否被占用。
func checkPortInUse(port int) PortUsageStatus {
	hosts := []string{"127.0.0.1", "0.0.0.0", "::1", "::"}
	sawUnknown := false

	for _, host := range hosts {
		result := tryListen(port, host)
		switch result {
		case PortBusy:
			return PortBusy
		case PortUnknown:
			sawUnknown = true
		}
	}
	if sawUnknown {
		return PortUnknown
	}
	return PortFree
}

// tryListen 尝试在指定 host:port 上监听。
func tryListen(port int, host string) PortUsageStatus {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// 区分 "地址不可用" 和 "端口被占用"
		errStr := err.Error()
		if strings.Contains(errStr, "address already in use") {
			return PortBusy
		}
		if strings.Contains(errStr, "cannot assign requested address") ||
			strings.Contains(errStr, "address not available") ||
			strings.Contains(errStr, "address family not supported") {
			return PortFree // host 不可用不代表端口被占
		}
		return PortUnknown
	}
	_ = ln.Close()
	return PortFree
}

// ─── 辅助函数 (ports-lsof.ts) ───

// resolveLsofCommand 定位 lsof 可执行文件路径。
func resolveLsofCommand() string {
	var candidates []string
	if runtime.GOOS == "darwin" {
		candidates = []string{"/usr/sbin/lsof", "/usr/bin/lsof"}
	} else {
		candidates = []string{"/usr/bin/lsof", "/usr/sbin/lsof"}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return "lsof" // 回退到 PATH 搜索
}

// resolveUnixCommandLine 通过 ps 获取进程命令行。
func resolveUnixCommandLine(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveUnixUser 通过 ps 获取进程所属用户。
func resolveUnixUser(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "user=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ─── 格式化 (ports-format.ts) ───

// ClassifyPortListener 根据进程信息分类监听者类型。
func ClassifyPortListener(listener PortListener, port int) PortListenerKind {
	raw := strings.ToLower(strings.TrimSpace(listener.CommandLine + " " + listener.Command))
	if strings.Contains(raw, "openacosmi") {
		return PortKindGateway
	}
	if strings.Contains(raw, "ssh") {
		return PortKindSSH
	}
	return PortKindUnknown
}

// BuildPortHints 根据监听者信息生成建议提示。
func BuildPortHints(listeners []PortListener, port int) []string {
	if len(listeners) == 0 {
		return nil
	}

	kinds := make(map[PortListenerKind]bool)
	for _, l := range listeners {
		kinds[ClassifyPortListener(l, port)] = true
	}

	var hints []string
	if kinds[PortKindGateway] {
		hints = append(hints, "Gateway already running locally. Stop it (crabclaw gateway stop) or use a different port.")
	}
	if kinds[PortKindSSH] {
		hints = append(hints, "SSH tunnel already bound to this port. Close the tunnel or use a different local port in -L.")
	}
	if kinds[PortKindUnknown] {
		hints = append(hints, "Another process is listening on this port.")
	}
	if len(listeners) > 1 {
		hints = append(hints, "Multiple listeners detected; ensure only one gateway/tunnel per port unless intentionally running isolated profiles.")
	}
	return hints
}

// FormatPortListener 格式化单个监听者信息。
func FormatPortListener(listener PortListener) string {
	pid := "pid ?"
	if listener.PID > 0 {
		pid = fmt.Sprintf("pid %d", listener.PID)
	}
	user := ""
	if listener.User != "" {
		user = " " + listener.User
	}
	command := listener.CommandLine
	if command == "" {
		command = listener.Command
	}
	if command == "" {
		command = "unknown"
	}
	address := ""
	if listener.Address != "" {
		address = fmt.Sprintf(" (%s)", listener.Address)
	}
	return fmt.Sprintf("%s%s: %s%s", pid, user, command, address)
}

// FormatPortDiagnostics 格式化端口使用诊断结果。
func FormatPortDiagnostics(usage PortUsage) []string {
	if usage.Status != PortBusy {
		return []string{fmt.Sprintf("Port %d is free.", usage.Port)}
	}
	lines := []string{fmt.Sprintf("Port %d is already in use.", usage.Port)}
	for _, l := range usage.Listeners {
		lines = append(lines, "- "+FormatPortListener(l))
	}
	for _, h := range usage.Hints {
		lines = append(lines, "- "+h)
	}
	return lines
}

// ─── 正则 (ports-format.ts classifyPortListener 中使用) ───

var sshTunnelPattern = regexp.MustCompile(`-[lr]\s*\d+\b|:\d+\b`)

// IsSSHTunnel 检查命令行是否为 SSH 隧道。
func IsSSHTunnel(cmdLine string, port int) bool {
	portStr := strconv.Itoa(port)
	pattern := regexp.MustCompile(fmt.Sprintf(`-[lr]\s*%s\b|:%s\b`, portStr, portStr))
	return pattern.MatchString(strings.ToLower(cmdLine))
}
