package infra

// path_env.go — PATH 环境变量管理
// 对应 TS: src/infra/path-env.ts (120L)

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PathSeparator 当前平台的 PATH 分隔符。
var PathSeparator = string(os.PathListSeparator)

// GetPathDirs 获取 PATH 中的所有目录。
func GetPathDirs() []string {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}
	return filepath.SplitList(pathEnv)
}

// PrependToPath 将目录添加到 PATH 前面（去重）。
func PrependToPath(dir string) {
	current := os.Getenv("PATH")
	dirs := filepath.SplitList(current)

	// 去重
	filtered := make([]string, 0, len(dirs)+1)
	filtered = append(filtered, dir)
	for _, d := range dirs {
		if d != dir {
			filtered = append(filtered, d)
		}
	}
	os.Setenv("PATH", strings.Join(filtered, PathSeparator))
}

// FindBinary 在 PATH 中查找可执行文件。
// 对应 TS: findBinary(name)
func FindBinary(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// FindBinaryInDir 在指定目录中查找可执行文件。
func FindBinaryInDir(name, dir string) string {
	candidate := filepath.Join(dir, name)
	if runtime.GOOS == "windows" && !strings.Contains(name, ".") {
		candidate += ".exe"
	}
	if FileExists(candidate) {
		return candidate
	}
	return ""
}

// EnsureInPath 确保指定目录在 PATH 中。
func EnsureInPath(dirs ...string) {
	current := os.Getenv("PATH")
	pathDirs := filepath.SplitList(current)
	dirSet := make(map[string]bool, len(pathDirs))
	for _, d := range pathDirs {
		dirSet[d] = true
	}

	var toAdd []string
	for _, dir := range dirs {
		if dir != "" && !dirSet[dir] {
			toAdd = append(toAdd, dir)
		}
	}
	if len(toAdd) == 0 {
		return
	}

	newPath := strings.Join(toAdd, PathSeparator) + PathSeparator + current
	os.Setenv("PATH", newPath)
}
