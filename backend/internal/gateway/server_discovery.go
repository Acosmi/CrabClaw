package gateway

// TS 对照: src/gateway/server-discovery-runtime.ts (101L)
// Gateway 发现服务运行时 — mDNS/Bonjour 广播 + 宽域 DNS-SD。
// 整合 infra.StartGatewayBonjourAdvertiser 提供完整发现功能。

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// DiscoveryParams 发现服务启动参数。
// TS 对照: server-discovery-runtime.ts startGatewayDiscovery params (L10-20)
type DiscoveryParams struct {
	MachineDisplayName          string
	Port                        int
	GatewayTLSEnabled           bool
	GatewayTLSFingerprintSHA256 string
	CanvasPort                  int
	WideAreaDiscoveryEnabled    bool
	WideAreaDiscoveryDomain     string
	TailscaleMode               TailscaleMode
	MDNSMode                    string // "off", "minimal", "full"; 默认 "minimal"
	Logger                      *slog.Logger
}

// DiscoveryResult 发现服务启动结果。
type DiscoveryResult struct {
	BonjourAdvertiser *infra.GatewayBonjourAdvertiser
}

// Stop 停止所有发现服务。
func (d *DiscoveryResult) Stop() {
	if d == nil {
		return
	}
	if d.BonjourAdvertiser != nil {
		d.BonjourAdvertiser.Stop()
	}
}

// StartGatewayDiscovery 启动 Gateway 发现服务。
// TS 对照: server-discovery-runtime.ts startGatewayDiscovery (L10-100)
func StartGatewayDiscovery(params DiscoveryParams, reg infra.BonjourRegistrar) *DiscoveryResult {
	result := &DiscoveryResult{}

	mdnsMode := params.MDNSMode
	if mdnsMode == "" {
		mdnsMode = "minimal"
	}

	// mDNS 可通过配置（mdnsMode: off）或环境变量禁用
	bonjourEnabled := mdnsMode != "off" &&
		preferredGatewayEnvValue("CRABCLAW_DISABLE_BONJOUR", "OPENACOSMI_DISABLE_BONJOUR") != "1" &&
		os.Getenv("GO_TEST_MODE") != "1"

	mdnsMinimal := mdnsMode != "full"

	// SSH 端口（仅 full 模式）
	sshPort := 0
	if !mdnsMinimal {
		if portStr := preferredGatewayEnvValue("CRABCLAW_SSH_PORT", "OPENACOSMI_SSH_PORT"); portStr != "" {
			fmt.Sscanf(portStr, "%d", &sshPort)
		}
	}

	// 如果调用方未提供 registrar，自动创建 ZeroconfRegistrar
	if bonjourEnabled && reg == nil {
		reg = infra.NewZeroconfRegistrar(params.Logger)
		if params.Logger != nil {
			params.Logger.Info("zeroconf registrar auto-created for mDNS")
		}
	}

	if bonjourEnabled && reg != nil {
		// 尝试获取 TailnetDNS 注入 TXT 记录
		var tailnetDNS string
		if params.TailscaleMode != "" && params.TailscaleMode != TailscaleModeOff {
			hostname, err := getTailnetHostname()
			if err == nil && hostname != "" {
				tailnetDNS = hostname
				if params.Logger != nil {
					params.Logger.Info("tailnet DNS injected into mDNS", "dns", hostname)
				}
			} else if params.Logger != nil {
				params.Logger.Debug("tailnet DNS not available for mDNS", "error", err)
			}
		}

		opts := infra.GatewayBonjourAdvertiseOpts{
			InstanceName:                params.MachineDisplayName,
			GatewayPort:                 params.Port,
			GatewayTLSEnabled:           params.GatewayTLSEnabled,
			GatewayTLSFingerprintSHA256: params.GatewayTLSFingerprintSHA256,
			CanvasPort:                  params.CanvasPort,
			SSHPort:                     sshPort,
			TailnetDNS:                  tailnetDNS,
			Minimal:                     mdnsMinimal,
		}

		adv, err := infra.StartGatewayBonjourAdvertiser(opts, reg)
		if err != nil {
			if params.Logger != nil {
				params.Logger.Warn("bonjour advertising failed", "error", err)
			}
		} else {
			result.BonjourAdvertiser = adv
		}
	}

	// 宽域 DNS-SD（框架桩）
	if params.WideAreaDiscoveryEnabled {
		if params.WideAreaDiscoveryDomain == "" {
			if params.Logger != nil {
				params.Logger.Warn("discovery.wideArea.enabled is true, but no domain was configured; set discovery.wideArea.domain to enable unicast DNS-SD")
			}
		} else {
			// 宽域 DNS-SD zone 写入
			// TS 对照: server-discovery-runtime.ts L73-98
			zoneResult, err := infra.WriteWideAreaGatewayZone(infra.WideAreaGatewayZoneOpts{
				Domain:                      params.WideAreaDiscoveryDomain,
				GatewayPort:                 params.Port,
				DisplayName:                 params.MachineDisplayName,
				TailnetIPv4:                 "", // Tailscale IP 由调用方在需要时提供
				GatewayTLSEnabled:           params.GatewayTLSEnabled,
				GatewayTLSFingerprintSHA256: params.GatewayTLSFingerprintSHA256,
			})
			if err != nil {
				if params.Logger != nil {
					params.Logger.Warn("wide-area DNS-SD zone write failed", "error", err)
				}
			} else if params.Logger != nil {
				params.Logger.Info("wide-area DNS-SD zone written",
					"domain", params.WideAreaDiscoveryDomain,
					"path", zoneResult.ZonePath,
					"changed", zoneResult.Changed)
			}
		}
	}

	return result
}
