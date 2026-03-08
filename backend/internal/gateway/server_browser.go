package gateway

// TS 对照: src/gateway/server-browser.ts (32L)
// 浏览器控制服务器的条件启停。
// 根据环境变量 OPENACOSMI_SKIP_BROWSER_CONTROL_SERVER 决定是否启用。

import (
	"log/slog"
	"strings"
)

// BrowserControlService 浏览器控制服务接口（DI 注入）。
type BrowserControlService interface {
	Start() error
	Stop() error
}

// BrowserControlServer 浏览器控制服务器封装。
type BrowserControlServer struct {
	service BrowserControlService
	logger  *slog.Logger
}

// StartBrowserControlServerIfEnabled 根据环境变量条件启动浏览器控制服务。
// TS 对照: server-browser.ts startBrowserControlServerIfEnabled (L7-31)
func StartBrowserControlServerIfEnabled(service BrowserControlService, logger *slog.Logger) (*BrowserControlServer, error) {
	if isTruthyEnvValue(preferredGatewayEnvValue("CRABCLAW_SKIP_BROWSER_CONTROL_SERVER", "OPENACOSMI_SKIP_BROWSER_CONTROL_SERVER")) {
		if logger != nil {
			logger.Info("browser control server skipped (CRABCLAW_SKIP_BROWSER_CONTROL_SERVER / OPENACOSMI_SKIP_BROWSER_CONTROL_SERVER)")
		}
		return nil, nil
	}

	if service == nil {
		if logger != nil {
			logger.Debug("browser control server: no service implementation provided")
		}
		return nil, nil
	}

	if err := service.Start(); err != nil {
		if logger != nil {
			logger.Warn("browser control server start failed", "error", err)
		}
		return nil, err
	}

	return &BrowserControlServer{service: service, logger: logger}, nil
}

// Stop 停止浏览器控制服务器。
func (b *BrowserControlServer) Stop() error {
	if b == nil || b.service == nil {
		return nil
	}
	return b.service.Stop()
}

// isTruthyEnvValue 判断环境变量值是否为 truthy。
func isTruthyEnvValue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
