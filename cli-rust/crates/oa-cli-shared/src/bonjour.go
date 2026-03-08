package infra

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---------- Bonjour mDNS 广播 ----------

// GatewayBonjourAdvertiser mDNS 服务广播器。
type GatewayBonjourAdvertiser struct {
	mu       sync.Mutex
	stopped  bool
	watchdog *time.Ticker
	stopChan chan struct{}
	services []bonjourServiceEntry
}

// bonjourServiceEntry 已注册的服务条目。
type bonjourServiceEntry struct {
	Label  string
	Active bool
}

// GatewayBonjourAdvertiseOpts Bonjour 广播选项。
type GatewayBonjourAdvertiseOpts struct {
	InstanceName                string
	GatewayPort                 int
	SSHPort                     int
	GatewayTLSEnabled           bool
	GatewayTLSFingerprintSHA256 string
	CanvasPort                  int
	TailnetDNS                  string
	CLIPath                     string
	Minimal                     bool // 精简模式：省略 cliPath/sshPort
}

// ---------- 工具函数 ----------

var spaceRE = regexp.MustCompile(`\s+`)
var bonjourBrandSuffixRE = regexp.MustCompile(`(?i)\s+\((?:OpenAcosmi|Crab Claw(?:（蟹爪）)?)\)\s*$`)

func isBonjourDisabledByEnv() bool {
	v := preferredEnvValue("CRABCLAW_DISABLE_BONJOUR", "OPENACOSMI_DISABLE_BONJOUR")
	return isTruthyEnv(v) || os.Getenv("GO_TEST_MODE") == "1"
}

func isTruthyEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func safeServiceName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Crab Claw"
	}
	return trimmed
}

func prettifyInstanceName(name string) string {
	normalized := strings.TrimSpace(spaceRE.ReplaceAllString(name, " "))
	result := strings.TrimSpace(bonjourBrandSuffixRE.ReplaceAllString(normalized, ""))
	if result == "" {
		return normalized
	}
	return result
}

func defaultBonjourInstanceName(hostname string) string {
	return fmt.Sprintf("%s (Crab Claw)", hostname)
}

func resolveHostname() string {
	if v := preferredEnvValue("CRABCLAW_MDNS_HOSTNAME", "OPENACOSMI_MDNS_HOSTNAME"); v != "" {
		return cleanHostname(v)
	}
	if v := strings.TrimSpace(os.Getenv("CLAWDBOT_MDNS_HOSTNAME")); v != "" {
		return cleanHostname(v)
	}
	return "openacosmi"
}

func cleanHostname(raw string) string {
	h := regexp.MustCompile(`(?i)\.local$`).ReplaceAllString(raw, "")
	parts := strings.SplitN(h, ".", 2)
	result := strings.TrimSpace(parts[0])
	if result == "" {
		return "openacosmi"
	}
	return result
}

// BuildBonjourTXTRecords 构建 mDNS TXT 记录。
func BuildBonjourTXTRecords(opts GatewayBonjourAdvertiseOpts) map[string]string {
	hostname := resolveHostname()
	instanceName := opts.InstanceName
	if strings.TrimSpace(instanceName) == "" {
		instanceName = defaultBonjourInstanceName(hostname)
	}
	displayName := prettifyInstanceName(instanceName)

	txt := map[string]string{
		"role":        "gateway",
		"gatewayPort": fmt.Sprintf("%d", opts.GatewayPort),
		"lanHost":     hostname + ".local",
		"displayName": displayName,
		"transport":   "gateway",
	}
	if opts.GatewayTLSEnabled {
		txt["gatewayTls"] = "1"
		if opts.GatewayTLSFingerprintSHA256 != "" {
			txt["gatewayTlsSha256"] = opts.GatewayTLSFingerprintSHA256
		}
	}
	if opts.CanvasPort > 0 {
		txt["canvasPort"] = fmt.Sprintf("%d", opts.CanvasPort)
	}
	if strings.TrimSpace(opts.TailnetDNS) != "" {
		txt["tailnetDns"] = strings.TrimSpace(opts.TailnetDNS)
	}
	if !opts.Minimal {
		txt["sshPort"] = fmt.Sprintf("%d", max(opts.SSHPort, 22))
		if strings.TrimSpace(opts.CLIPath) != "" {
			txt["cliPath"] = strings.TrimSpace(opts.CLIPath)
		}
	}
	return txt
}

// StartGatewayBonjourAdvertiser 启动 mDNS 服务广播。
// 注意：实际 mDNS 广播依赖 github.com/grandcat/zeroconf（或等价包）。
// 此函数提供框架结构和 TXT record 构建，实际 mDNS 注册由调用方传入 Registrar。
func StartGatewayBonjourAdvertiser(opts GatewayBonjourAdvertiseOpts, reg BonjourRegistrar) (*GatewayBonjourAdvertiser, error) {
	if isBonjourDisabledByEnv() {
		return &GatewayBonjourAdvertiser{stopped: true}, nil
	}

	hostname := resolveHostname()
	instanceName := opts.InstanceName
	if strings.TrimSpace(instanceName) == "" {
		instanceName = defaultBonjourInstanceName(hostname)
	}
	txt := BuildBonjourTXTRecords(opts)

	adv := &GatewayBonjourAdvertiser{
		stopChan: make(chan struct{}),
	}

	// 注册 gateway 服务
	if reg != nil {
		err := reg.Register(BonjourServiceDef{
			Name:     safeServiceName(instanceName),
			Type:     "_openacosmi-gw._tcp",
			Domain:   "local",
			Port:     opts.GatewayPort,
			Hostname: hostname,
			TXT:      txt,
		})
		if err != nil {
			return nil, fmt.Errorf("bonjour register: %w", err)
		}
		adv.services = append(adv.services, bonjourServiceEntry{Label: "gateway", Active: true})
	}

	// watchdog: 每 60s 检查服务状态
	adv.watchdog = time.NewTicker(60 * time.Second)
	go func() {
		for {
			select {
			case <-adv.stopChan:
				return
			case <-adv.watchdog.C:
				// 占位: 实际 watchdog 逻辑在有完整 zeroconf 集成后实现
			}
		}
	}()

	return adv, nil
}

// Stop 停止广播。
func (a *GatewayBonjourAdvertiser) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return
	}
	a.stopped = true
	if a.watchdog != nil {
		a.watchdog.Stop()
	}
	close(a.stopChan)
}

// ---------- 依赖接口 ----------

// BonjourRegistrar mDNS 服务注册器（依赖注入）。
type BonjourRegistrar interface {
	Register(def BonjourServiceDef) error
	Shutdown() error
}

// BonjourServiceDef mDNS 服务定义。
type BonjourServiceDef struct {
	Name     string
	Type     string
	Domain   string
	Port     int
	Hostname string
	TXT      map[string]string
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
