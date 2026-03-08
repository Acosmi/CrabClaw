package infra

// runtime_guard.go — Go 运行时版本检查
// 对应 TS: src/infra/runtime-guard.ts (99L)

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// MinGoVersion 最低支持的 Go 版本。
const MinGoVersion = "1.22"

// CheckGoVersion 检查 Go 运行时版本是否满足要求。
// 对应 TS: checkNodeVersion(minVersion)
func CheckGoVersion(minVersion string) error {
	current := runtime.Version()
	currentParts := parseGoVersion(current)
	minParts := parseGoVersion(minVersion)

	if currentParts == nil {
		return nil // 无法解析，跳过检查
	}
	if minParts == nil {
		return nil
	}

	if currentParts[0] < minParts[0] || (currentParts[0] == minParts[0] && currentParts[1] < minParts[1]) {
		return fmt.Errorf("Go %s required, current: %s", minVersion, current)
	}
	return nil
}

// GetGoVersionInfo 获取 Go 版本信息。
func GetGoVersionInfo() string {
	return fmt.Sprintf("%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// parseGoVersion 解析 Go 版本号。
// "go1.22.1" → [1, 22]
func parseGoVersion(version string) []int {
	v := strings.TrimPrefix(version, "go")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return nil
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	return []int{major, minor}
}
