package infra

// ssh_config.go — SSH 配置解析
// 对应 TS: src/infra/ssh-config.ts (105L)
//
// 通过调用 `ssh -G` 命令解析 ~/.ssh/config 中的配置。
// 不引入外部库，保持与 TS 端 1:1 一致。

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SshResolvedConfig 解析后的 SSH 配置。
type SshResolvedConfig struct {
	User          string   `json:"user,omitempty"`
	Host          string   `json:"host,omitempty"`
	Port          int      `json:"port,omitempty"`
	IdentityFiles []string `json:"identityFiles"`
}

// SshParsedTarget 定义在 ssh_tunnel.go 中，此处复用。

// parseSshPort 安全解析 SSH 端口号。
func parseSshPort(value string) int {
	if value == "" {
		return 0
	}
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 {
		return 0
	}
	return port
}

// ParseSshConfigOutput 解析 `ssh -G` 命令的输出。
// 对应 TS: parseSshConfigOutput(output)
func ParseSshConfigOutput(output string) SshResolvedConfig {
	result := SshResolvedConfig{IdentityFiles: []string{}}
	lines := strings.Split(output, "\n")

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			continue
		}

		switch strings.ToLower(key) {
		case "user":
			result.User = value
		case "hostname":
			result.Host = value
		case "port":
			result.Port = parseSshPort(value)
		case "identityfile":
			if value != "none" {
				result.IdentityFiles = append(result.IdentityFiles, value)
			}
		}
	}
	return result
}

// ResolveSshConfigOpts SSH 配置解析选项。
type ResolveSshConfigOpts struct {
	Identity  string
	TimeoutMs int
}

// ResolveSshConfig 通过 `ssh -G` 命令解析 SSH 配置。
// 对应 TS: resolveSshConfig(target, opts)
//
// 返回 nil 表示解析失败（超时、命令不存在等）。
func ResolveSshConfig(target SshParsedTarget, opts ResolveSshConfigOpts) *SshResolvedConfig {
	sshPath := "/usr/bin/ssh"
	args := []string{"-G"}

	if target.Port > 0 && target.Port != 22 {
		args = append(args, "-p", strconv.Itoa(target.Port))
	}
	if identity := strings.TrimSpace(opts.Identity); identity != "" {
		args = append(args, "-i", identity)
	}

	userHost := target.Host
	if target.User != "" {
		userHost = target.User + "@" + target.Host
	}
	args = append(args, "--", userHost)

	timeoutMs := opts.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 800
	}
	timeoutMs = intMax(200, timeoutMs)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, sshPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	stdout := strings.TrimSpace(string(out))
	if stdout == "" {
		return nil
	}

	result := ParseSshConfigOutput(stdout)
	return &result
}
