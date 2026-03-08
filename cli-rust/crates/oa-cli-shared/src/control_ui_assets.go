package infra

// TS 对照: src/infra/control-ui-assets.ts (275L)
// Control UI 静态资产发现 + 自动构建

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---------- 常量 ----------

var controlUiDistSegments = []string{"dist", "control-ui", "index.html"}

// ---------- 类型 ----------

// ControlUiDistIndexHealth 资产健康状态。
type ControlUiDistIndexHealth struct {
	IndexPath string
	Exists    bool
}

// EnsureControlUiAssetsResult 确保资产结果。
type EnsureControlUiAssetsResult struct {
	OK      bool
	Built   bool
	Message string
}

// ControlUiRootResolveOptions 根目录解析选项。
type ControlUiRootResolveOptions struct {
	Argv1   string
	Cwd     string
	BinPath string
}

// ---------- 路径解析 ----------

// ResolveControlUiDistIndexForRoot 从项目根构建 index.html 路径。
func ResolveControlUiDistIndexForRoot(root string) string {
	return filepath.Join(root, filepath.Join(controlUiDistSegments...))
}

// ResolveControlUiRepoRoot 从入口路径向上查找项目根。
func ResolveControlUiRepoRoot(argv1 string) string {
	if argv1 == "" {
		return ""
	}
	resolved, _ := filepath.Abs(argv1)
	parts := strings.Split(resolved, string(filepath.Separator))

	// 方法1: 找 src 目录的父级
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "src" {
			root := filepath.Join(parts[:i]...)
			if !filepath.IsAbs(root) {
				root = string(filepath.Separator) + root
			}
			if _, err := os.Stat(filepath.Join(root, "ui", "vite.config.ts")); err == nil {
				return root
			}
		}
	}

	// 方法2: 向上 traverse
	dir := filepath.Dir(resolved)
	for i := 0; i < 8; i++ {
		pkg := filepath.Join(dir, "package.json")
		vite := filepath.Join(dir, "ui", "vite.config.ts")
		if fileExists(pkg) && fileExists(vite) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ResolveControlUiDistIndexHealth 获取资产健康状态。
func ResolveControlUiDistIndexHealth(root, argv1 string) ControlUiDistIndexHealth {
	var indexPath string
	if root != "" {
		indexPath = ResolveControlUiDistIndexForRoot(root)
	} else {
		indexPath = resolveControlUiDistIndexPath(argv1)
	}
	return ControlUiDistIndexHealth{
		IndexPath: indexPath,
		Exists:    indexPath != "" && fileExists(indexPath),
	}
}

func resolveControlUiDistIndexPath(argv1 string) string {
	if argv1 == "" {
		return ""
	}
	resolved, _ := filepath.Abs(argv1)
	distDir := filepath.Dir(resolved)
	if filepath.Base(distDir) == "dist" {
		return filepath.Join(distDir, "control-ui", "index.html")
	}

	// traverse up
	dir := filepath.Dir(resolved)
	for i := 0; i < 8; i++ {
		pkgPath := filepath.Join(dir, "package.json")
		indexPath := filepath.Join(dir, "dist", "control-ui", "index.html")
		if fileExists(pkgPath) && fileExists(indexPath) {
			return indexPath
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ResolveControlUiRootSync 同步解析 Control UI 根目录。
func ResolveControlUiRootSync(opts ControlUiRootResolveOptions) string {
	candidates := make([]string, 0, 8)
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	argv1Dir := ""
	if opts.Argv1 != "" {
		p, _ := filepath.Abs(opts.Argv1)
		argv1Dir = filepath.Dir(p)
	}
	binDir := ""
	if opts.BinPath != "" {
		real, err := filepath.EvalSymlinks(opts.BinPath)
		if err == nil {
			binDir = filepath.Dir(real)
		}
	}

	addIfDir := func(p string) {
		if p != "" {
			candidates = append(candidates, p)
		}
	}
	if binDir != "" {
		addIfDir(filepath.Join(binDir, "control-ui"))
	}
	if argv1Dir != "" {
		addIfDir(filepath.Join(argv1Dir, "dist", "control-ui"))
		addIfDir(filepath.Join(argv1Dir, "control-ui"))
	}
	addIfDir(filepath.Join(cwd, "dist", "control-ui"))

	for _, dir := range candidates {
		if fileExists(filepath.Join(dir, "index.html")) {
			return dir
		}
	}
	return ""
}

// ResolveControlUiRootOverrideSync 处理 override 路径。
func ResolveControlUiRootOverrideSync(rootOverride string) string {
	p, _ := filepath.Abs(rootOverride)
	info, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		if fileExists(filepath.Join(p, "index.html")) {
			return p
		}
		return ""
	}
	if filepath.Base(p) == "index.html" {
		return filepath.Dir(p)
	}
	return ""
}

// ---------- 自动构建 ----------

// EnsureControlUiAssetsBuilt 确保资产已构建，自动触发构建。
func EnsureControlUiAssetsBuilt(argv1 string, timeoutMs int) EnsureControlUiAssetsResult {
	health := ResolveControlUiDistIndexHealth("", argv1)
	if health.Exists {
		return EnsureControlUiAssetsResult{OK: true}
	}

	repoRoot := ResolveControlUiRepoRoot(argv1)
	if repoRoot == "" {
		msg := "Missing Control UI assets"
		if health.IndexPath != "" {
			msg = fmt.Sprintf("Missing Control UI assets at %s", health.IndexPath)
		}
		return EnsureControlUiAssetsResult{Message: msg + ". Build with `pnpm ui:build`."}
	}

	indexPath := ResolveControlUiDistIndexForRoot(repoRoot)
	if fileExists(indexPath) {
		return EnsureControlUiAssetsResult{OK: true}
	}

	uiScript := filepath.Join(repoRoot, "scripts", "ui.js")
	if !fileExists(uiScript) {
		return EnsureControlUiAssetsResult{
			Message: fmt.Sprintf("Control UI assets missing but %s is unavailable.", uiScript),
		}
	}

	if timeoutMs <= 0 {
		timeoutMs = 10 * 60 * 1000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// 运行构建
	cmd := exec.Command("node", uiScript, "build")
	cmd.Dir = repoRoot
	out, err := runWithTimeout(cmd, timeout)
	if err != nil {
		return EnsureControlUiAssetsResult{
			Built:   false,
			Message: fmt.Sprintf("Control UI build failed: %s", summarizeOutput(out, err)),
		}
	}

	if !fileExists(indexPath) {
		return EnsureControlUiAssetsResult{
			Built:   true,
			Message: fmt.Sprintf("Build completed but %s still missing.", indexPath),
		}
	}
	return EnsureControlUiAssetsResult{OK: true, Built: true}
}

// ---------- 辅助 ----------

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func summarizeOutput(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			if len(l) > 240 {
				return l[:239] + "…"
			}
			return l
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		done <- result{out, err}
	}()
	select {
	case <-time.After(timeout):
		// Kill the process; do NOT read output to avoid data race with the goroutine.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("timeout after %s", timeout)
	case r := <-done:
		return string(r.output), r.err
	}
}
