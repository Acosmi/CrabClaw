package infra

// TS 对照: src/infra/bonjour.ts (282L) — @homebridge/ciao mDNS 注册
// Go 等价实现：使用 github.com/grandcat/zeroconf 提供实际 mDNS 注册能力。

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"
)

// ZeroconfRegistrar 基于 grandcat/zeroconf 的 BonjourRegistrar 实现。
// 满足 BonjourRegistrar 接口，提供实际的 mDNS/DNS-SD 服务注册。
type ZeroconfRegistrar struct {
	mu      sync.Mutex
	servers []*zeroconf.Server
	logger  *slog.Logger
}

// NewZeroconfRegistrar 创建 zeroconf 注册器。
func NewZeroconfRegistrar(logger *slog.Logger) *ZeroconfRegistrar {
	return &ZeroconfRegistrar{
		logger: logger,
	}
}

// Register 注册一个 mDNS 服务。
// TS 对照: bonjour.ts responder.createService + svc.advertise()
func (r *ZeroconfRegistrar) Register(def BonjourServiceDef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 将 map[string]string TXT 转换为 []string{"k=v"} 格式
	txtRecords := make([]string, 0, len(def.TXT))
	for k, v := range def.TXT {
		txtRecords = append(txtRecords, fmt.Sprintf("%s=%s", k, v))
	}

	// 解析服务类型: "_openacosmi-gw._tcp" → "openacosmi-gw"
	serviceType := strings.TrimPrefix(def.Type, "_")
	serviceType = strings.TrimSuffix(serviceType, "._tcp")
	serviceType = strings.TrimSuffix(serviceType, "._udp")

	// 确定域名（zeroconf 需要不带尾点的域名）
	domain := def.Domain
	if domain == "" {
		domain = "local"
	}
	// 移除可能的尾点（zeroconf.Register 内部会处理）
	domain = strings.TrimSuffix(domain, ".")

	// 确定端口
	port := def.Port
	if port <= 0 {
		return fmt.Errorf("zeroconf register: invalid port %d", port)
	}

	// 确定主机名
	hostname := def.Hostname
	if hostname == "" {
		hostname = "openacosmi"
	}

	// 注册 mDNS 服务
	// zeroconf.Register(instance, service, domain, port, txt, ifaces)
	server, err := zeroconf.Register(
		def.Name,    // 实例名
		serviceType, // 服务类型（不含 _ 前缀和 _tcp 后缀）
		domain,      // 域（local）
		port,        // 端口
		txtRecords,  // TXT 记录
		nil,         // nil = 所有网络接口
	)
	if err != nil {
		return fmt.Errorf("zeroconf register %q: %w", def.Name, err)
	}

	r.servers = append(r.servers, server)

	if r.logger != nil {
		r.logger.Info("mDNS service registered",
			"name", def.Name,
			"type", def.Type,
			"port", port,
			"hostname", hostname,
			"domain", domain,
			"txt_count", len(txtRecords),
		)
	}

	return nil
}

// Shutdown 关闭所有注册的 mDNS 服务。
// TS 对照: bonjour.ts stop() → svc.destroy() + responder.shutdown()
func (r *ZeroconfRegistrar) Shutdown() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range r.servers {
		s.Shutdown()
	}
	r.servers = nil

	if r.logger != nil {
		r.logger.Info("mDNS services stopped")
	}

	return nil
}
