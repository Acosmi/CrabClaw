// Package cli 提供 Crab Claw（蟹爪）CLI 框架的工具函数和共享组件。
// 对应 TS 端 src/cli/ 目录的基础设施层。
package cli

import (
	"fmt"
	"os/exec"
	"strings"
)

// 版本信息 — 通过 -ldflags 在构建时注入
// go build -ldflags "-X github.com/Acosmi/ClawAcosmi/internal/cli.Version=1.0.0
//
//	-X github.com/Acosmi/ClawAcosmi/internal/cli.CommitHash=abc1234"
var (
	// Version 当前版本号（构建时注入）
	Version = "dev"
	// CommitHash 当前 commit hash（构建时注入）
	CommitHash = ""
)

// CLIName CLI 程序名称（与 TS DEFAULT_CLI_NAME 对应）
const CLIName = "openacosmi"

// ResolveCommitHash 解析 commit hash（构建注入优先，否则尝试 git）。
// 对应 TS infra/git-commit.ts resolveCommitHash()。
func ResolveCommitHash() string {
	if CommitHash != "" {
		return CommitHash
	}
	// 尝试从环境变量读取
	if envHash := envValueCompat("CRABCLAW_COMMIT", "OPENACOSMI_COMMIT"); envHash != "" {
		return envHash
	}
	// 尝试 git rev-parse
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// FormatVersionLine 格式化版本行（单行版本信息）。
func FormatVersionLine() string {
	commit := ResolveCommitHash()
	return fmt.Sprintf("%s %s (%s)", CLIName, Version, commit)
}
