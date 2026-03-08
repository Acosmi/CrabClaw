//go:build !linux

// platform_wsl_stub.go — 非 Linux 平台的 WSL 检测桩函数

package infra

// IsWSL 在非 Linux 平台始终返回 false。
func IsWSL() bool {
	return false
}

// IsWSL2 在非 Linux 平台始终返回 false。
func IsWSL2() bool {
	return false
}
