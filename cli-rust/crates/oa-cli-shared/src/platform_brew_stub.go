//go:build !darwin

// platform_brew_stub.go — 非 macOS 平台 Homebrew 检测桩函数

package infra

// ResolveBrewPrefix 在非 macOS 平台不可用。
func ResolveBrewPrefix() (string, error) {
	return "", nil
}

// IsBrewInstalled 在非 macOS 平台始终返回 false。
func IsBrewInstalled() bool {
	return false
}
