package infra

// openacosmi_root.go — 项目根目录定位
// 对应 TS: src/infra/openacosmi-root.ts (125L)

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	rootOnce   sync.Once
	rootCached string
)

// GetProjectRoot 获取 OpenAcosmi 项目根目录。
// 对应 TS: getOpenAcosmiRoot()
//
// 优先环境变量 CRABCLAW_ROOT / OPENACOSMI_ROOT，回退到可执行文件所在目录向上搜索 go.mod。
func GetProjectRoot() string {
	rootOnce.Do(func() {
		rootCached = resolveProjectRoot()
	})
	return rootCached
}

func resolveProjectRoot() string {
	// 1. 环境变量
	if envRoot := preferredEnvValue("CRABCLAW_ROOT", "OPENACOSMI_ROOT"); envRoot != "" {
		if DirExists(envRoot) {
			return envRoot
		}
	}

	// 2. 从可执行文件位置向上搜索
	if execPath, err := os.Executable(); err == nil {
		dir := filepath.Dir(execPath)
		if root := findRootMarker(dir); root != "" {
			return root
		}
	}

	// 3. 从当前工作目录向上搜索
	if cwd, err := os.Getwd(); err == nil {
		if root := findRootMarker(cwd); root != "" {
			return root
		}
	}

	return ""
}

// findRootMarker 从 dir 向上搜索项目标记文件。
func findRootMarker(dir string) string {
	markers := []string{"go.mod", "openacosmi.config.json", ".git"}

	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ResetProjectRootForTest 重置缓存（仅测试）。
func ResetProjectRootForTest() {
	rootOnce = sync.Once{}
	rootCached = ""
}
