package infra

// update_check.go — Git/版本更新检测
// 对应 TS: src/infra/update-check.ts (415L) — 简化版
//
// 检测 Git 仓库状态（commit/tag/branch/ahead/behind）和版本更新可用性。
// Go 端简化 npm/pnpm 依赖检测部分（Go 使用 go.mod）。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── 类型定义 ───

// GitUpdateStatus Git 仓库更新状态。
type GitUpdateStatus struct {
	Root     string `json:"root"`
	SHA      string `json:"sha,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Dirty    *bool  `json:"dirty,omitempty"`
	Ahead    *int   `json:"ahead,omitempty"`
	Behind   *int   `json:"behind,omitempty"`
	FetchOk  *bool  `json:"fetchOk,omitempty"`
	Error    string `json:"error,omitempty"`
}

// InstallKind 安装方式。
type InstallKind string

const (
	InstallGit     InstallKind = "git"
	InstallPackage InstallKind = "package"
	InstallUnknown InstallKind = "unknown"
)

// UpdateCheckResult 更新检查结果。
type UpdateCheckResult struct {
	Root        string           `json:"root,omitempty"`
	InstallKind InstallKind      `json:"installKind"`
	Git         *GitUpdateStatus `json:"git,omitempty"`
}

// ─── 核心函数 ───

// DetectGitRoot 检测 Git 仓库根目录。
// 对应 TS: detectGitRoot(root)
func DetectGitRoot(root string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CheckGitUpdateStatus 检查 Git 仓库更新状态。
// 对应 TS: checkGitUpdateStatus(params)
func CheckGitUpdateStatus(root string, timeoutMs int, doFetch bool) GitUpdateStatus {
	if timeoutMs <= 0 {
		timeoutMs = 10_000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	status := GitUpdateStatus{Root: root}

	// SHA
	if sha := gitCmd(root, timeout, "rev-parse", "HEAD"); sha != "" {
		status.SHA = sha
	}

	// Tag (最近的标签)
	if tag := gitCmd(root, timeout, "describe", "--tags", "--abbrev=0"); tag != "" {
		status.Tag = tag
	}

	// Branch
	if branch := gitCmd(root, timeout, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" && branch != "HEAD" {
		status.Branch = branch
	}

	// Upstream
	if status.Branch != "" {
		if upstream := gitCmd(root, timeout, "rev-parse", "--abbrev-ref", "@{u}"); upstream != "" {
			status.Upstream = upstream
		}
	}

	// Dirty check
	dirtyOut := gitCmd(root, timeout, "status", "--porcelain", "--untracked-files=no")
	dirty := dirtyOut != ""
	status.Dirty = &dirty

	// Fetch (可选)
	if doFetch && status.Upstream != "" {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "fetch", "--quiet")
		cmd.Dir = root
		err := cmd.Run()
		fetchOk := err == nil
		status.FetchOk = &fetchOk
	}

	// Ahead/Behind
	if status.Upstream != "" {
		raw := gitCmd(root, timeout, "rev-list", "--left-right", "--count", "HEAD...@{u}")
		if counts := parseAheadBehind(raw); counts != nil {
			status.Ahead = &counts[0]
			status.Behind = &counts[1]
		}
	}

	return status
}

// CheckUpdateStatus 综合更新检查。
// 对应 TS: checkUpdateStatus(params)
func CheckUpdateStatus(root string, timeoutMs int, fetchGit bool) UpdateCheckResult {
	result := UpdateCheckResult{
		InstallKind: InstallUnknown,
	}

	if root == "" {
		return result
	}

	// 检测安装方式
	gitRoot := DetectGitRoot(root)
	if gitRoot != "" {
		result.Root = gitRoot
		result.InstallKind = InstallGit
		git := CheckGitUpdateStatus(gitRoot, timeoutMs, fetchGit)
		result.Git = &git
	} else {
		result.Root = root
		// 检查是否有 go.mod（Go 包安装）
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			result.InstallKind = InstallPackage
		}
	}

	return result
}

// ─── Semver 工具 ───

var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// SemverParts 语义版本号各部分。
type SemverParts struct {
	Major int
	Minor int
	Patch int
}

// ParseSemver 解析语义版本号。
// 对应 TS: parseSemver(version)
func ParseSemver(version string) *SemverParts {
	matches := semverRegex.FindStringSubmatch(version)
	if matches == nil {
		return nil
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])
	return &SemverParts{Major: major, Minor: minor, Patch: patch}
}

// CompareSemver 比较两个语义版本号。
// 返回 -1（a < b）、0（a == b）、1（a > b）。
func CompareSemver(a, b string) int {
	pa := ParseSemver(a)
	pb := ParseSemver(b)
	if pa == nil || pb == nil {
		return 0
	}
	if pa.Major != pb.Major {
		return intSign(pa.Major - pb.Major)
	}
	if pa.Minor != pb.Minor {
		return intSign(pa.Minor - pb.Minor)
	}
	if pa.Patch != pb.Patch {
		return intSign(pa.Patch - pb.Patch)
	}
	return 0
}

// ─── 辅助函数 ───

func gitCmd(dir string, timeout time.Duration, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseAheadBehind(raw string) []int {
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return nil
	}
	ahead, err1 := strconv.Atoi(parts[0])
	behind, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	return []int{ahead, behind}
}

func intSign(v int) int {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

// FormatGitUpdateSummary 格式化 Git 更新状态摘要。
func FormatGitUpdateSummary(status GitUpdateStatus) string {
	var parts []string
	if status.Tag != "" {
		parts = append(parts, fmt.Sprintf("tag=%s", status.Tag))
	}
	if status.Branch != "" {
		parts = append(parts, fmt.Sprintf("branch=%s", status.Branch))
	}
	if status.SHA != "" {
		sha := status.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		parts = append(parts, fmt.Sprintf("sha=%s", sha))
	}
	if status.Ahead != nil && *status.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("ahead=%d", *status.Ahead))
	}
	if status.Behind != nil && *status.Behind > 0 {
		parts = append(parts, fmt.Sprintf("behind=%d", *status.Behind))
	}
	if status.Dirty != nil && *status.Dirty {
		parts = append(parts, "dirty")
	}
	return strings.Join(parts, " ")
}
