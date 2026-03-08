package infra

// home_dir.go — 用户目录解析 + XDG 支持
// 对应 TS: src/infra/home-dir.ts (77L)
//
// 解析用户主目录和 XDG 数据/配置目录。

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetHomeDir 获取用户主目录。
// 对应 TS: getHomeDir()
func GetHomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if runtime.GOOS == "windows" {
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return userProfile
		}
		drive := os.Getenv("HOMEDRIVE")
		path := os.Getenv("HOMEPATH")
		if drive != "" && path != "" {
			return drive + path
		}
	}
	// 最终回退
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return home
}

// GetXDGDataHome XDG 数据目录。
// 对应 TS: getXDGDataHome()
func GetXDGDataHome() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return xdg
	}
	return filepath.Join(GetHomeDir(), ".local", "share")
}

// GetXDGConfigHome XDG 配置目录。
func GetXDGConfigHome() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}
	return filepath.Join(GetHomeDir(), ".config")
}

// GetXDGCacheHome XDG 缓存目录。
func GetXDGCacheHome() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return xdg
	}
	return filepath.Join(GetHomeDir(), ".cache")
}

// GetOpenAcosmiDataDir OpenAcosmi 数据目录。
func GetOpenAcosmiDataDir() string {
	if custom := preferredEnvValue("CRABCLAW_DATA_DIR", "OPENACOSMI_DATA_DIR"); custom != "" {
		return custom
	}
	return filepath.Join(GetXDGDataHome(), "openacosmi")
}

// GetOpenAcosmiConfigDir OpenAcosmi 配置目录。
func GetOpenAcosmiConfigDir() string {
	if custom := preferredEnvValue("CRABCLAW_CONFIG_DIR", "OPENACOSMI_CONFIG_DIR"); custom != "" {
		return custom
	}
	return filepath.Join(GetXDGConfigHome(), "openacosmi")
}
