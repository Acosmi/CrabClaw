package infra

// os_summary.go — 操作系统摘要
// 对应 TS: src/infra/os-summary.ts (35L)

import (
	"fmt"
	"runtime"
)

// OsSummary 操作系统摘要信息。
type OsSummary struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Version string `json:"version,omitempty"`
	GoVer   string `json:"goVersion"`
}

// GetOsSummary 获取操作系统摘要。
// 对应 TS: getOsSummary()
func GetOsSummary() OsSummary {
	return OsSummary{
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
		GoVer: runtime.Version(),
	}
}

// FormatOsSummary 格式化 OS 摘要为可读字符串。
func FormatOsSummary(s OsSummary) string {
	return fmt.Sprintf("%s/%s (%s)", s.OS, s.Arch, s.GoVer)
}
