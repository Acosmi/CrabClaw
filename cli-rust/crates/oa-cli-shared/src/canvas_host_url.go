package infra

// canvas_host_url.go — Canvas 主机 URL 解析
// 对应 TS: src/infra/canvas-host-url.ts
//
// 解析 Canvas 宿主服务的访问 URL，支持 TLS 和明文两种模式。
// Canvas 是 openacosmi 的 Web 界面渲染服务，端口默认为 gateway+4。

import (
	"fmt"
	"strconv"
	"strings"
)

// CanvasHostURLOpts Canvas URL 解析选项。
type CanvasHostURLOpts struct {
	// GatewayPort gateway 监听端口（用于派生 canvas 端口）。
	GatewayPort int
	// CanvasPort 显式指定 canvas 端口（>0 时覆盖派生值）。
	CanvasPort int
	// TLSEnabled 是否启用 TLS。
	TLSEnabled bool
	// Host 主机地址（默认 "127.0.0.1"）。
	Host string
	// ExternalHost 外部可访问主机（用于局域网/tailnet 场景）。
	ExternalHost string
}

// CanvasHostURLResult Canvas URL 解析结果。
type CanvasHostURLResult struct {
	// LocalURL 本地访问 URL（127.0.0.1）。
	LocalURL string `json:"localUrl"`
	// ExternalURL 外部访问 URL（局域网或 tailnet，可能为空）。
	ExternalURL string `json:"externalUrl,omitempty"`
	// Port 实际使用的端口号。
	Port int `json:"port"`
	// TLSEnabled 是否使用 TLS。
	TLSEnabled bool `json:"tlsEnabled"`
}

// ResolveCanvasHostURL 解析 Canvas 主机 URL。
// 对应 TS: resolveCanvasHostUrl(opts)
func ResolveCanvasHostURL(opts CanvasHostURLOpts) *CanvasHostURLResult {
	port := resolveCanvasPort(opts)
	scheme := "http"
	if opts.TLSEnabled {
		scheme = "https"
	}

	host := opts.Host
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}

	localURL := fmt.Sprintf("%s://%s:%d", scheme, host, port)

	var externalURL string
	if extHost := strings.TrimSpace(opts.ExternalHost); extHost != "" {
		externalURL = fmt.Sprintf("%s://%s:%d", scheme, extHost, port)
	}

	return &CanvasHostURLResult{
		LocalURL:    localURL,
		ExternalURL: externalURL,
		Port:        port,
		TLSEnabled:  opts.TLSEnabled,
	}
}

// resolveCanvasPort 解析 canvas 实际端口。
// 优先级: 环境变量 > opts.CanvasPort > 从 gateway 端口派生（+4）。
func resolveCanvasPort(opts CanvasHostURLOpts) int {
	// 环境变量覆盖
	if v := preferredEnvValue("CRABCLAW_CANVAS_PORT", "OPENACOSMI_CANVAS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 65535 {
			return n
		}
	}

	if opts.CanvasPort > 0 && opts.CanvasPort <= 65535 {
		return opts.CanvasPort
	}

	// 从 gateway 端口派生（+4，与 portdefaults.go 一致）
	if opts.GatewayPort > 0 {
		derived := opts.GatewayPort + 4
		if derived <= 65535 {
			return derived
		}
	}

	// fallback 到默认值 18793
	return 18793
}

// CanvasURLFromGatewayPort 便捷函数：从 gateway 端口直接得到 canvas 本地 URL。
// 对应 TS: canvasUrlFromGatewayPort(gatewayPort, tls?)
func CanvasURLFromGatewayPort(gatewayPort int, tlsEnabled bool) string {
	result := ResolveCanvasHostURL(CanvasHostURLOpts{
		GatewayPort: gatewayPort,
		TLSEnabled:  tlsEnabled,
	})
	return result.LocalURL
}

// IsCanvasSkipped 检查环境变量是否要求跳过 Canvas 主机服务。
// 对应 TS: OPENACOSMI_SKIP_CANVAS_HOST 检查
func IsCanvasSkipped() bool {
	v := strings.ToLower(preferredEnvValue("CRABCLAW_SKIP_CANVAS_HOST", "OPENACOSMI_SKIP_CANVAS_HOST"))
	return v == "1" || v == "true" || v == "yes"
}
