package infra

// git_commit.go — Git 提交信息工具
// 对应 TS: src/infra/git-commit.ts (128L)

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// GitCommitInfo Git 提交信息。
type GitCommitInfo struct {
	SHA      string `json:"sha"`
	ShortSHA string `json:"shortSha"`
	Subject  string `json:"subject"`
	Author   string `json:"author"`
	Date     string `json:"date"`
	Branch   string `json:"branch,omitempty"`
}

// GetCurrentCommitInfo 获取当前 Git 提交信息。
// 对应 TS: getCurrentCommitInfo(dir)
func GetCurrentCommitInfo(dir string) *GitCommitInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sha := gitCmdCtx(ctx, dir, "rev-parse", "HEAD")
	if sha == "" {
		return nil
	}

	info := &GitCommitInfo{
		SHA: sha,
	}
	if len(sha) >= 8 {
		info.ShortSHA = sha[:8]
	} else {
		info.ShortSHA = sha
	}

	info.Subject = gitCmdCtx(ctx, dir, "log", "-1", "--format=%s")
	info.Author = gitCmdCtx(ctx, dir, "log", "-1", "--format=%an <%ae>")
	info.Date = gitCmdCtx(ctx, dir, "log", "-1", "--format=%ci")
	info.Branch = gitCmdCtx(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if info.Branch == "HEAD" {
		info.Branch = ""
	}

	return info
}

// FormatCommitOneLiner 格式化提交信息为单行。
func FormatCommitOneLiner(info *GitCommitInfo) string {
	if info == nil {
		return "unknown"
	}
	parts := []string{info.ShortSHA}
	if info.Subject != "" {
		parts = append(parts, info.Subject)
	}
	return strings.Join(parts, " ")
}

func gitCmdCtx(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
