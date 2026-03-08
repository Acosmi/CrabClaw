package infra

// TS 对照: src/infra/widearea-dns.ts (200L)
// 宽域 DNS-SD zone 文件渲染与写入。
// 用于跨子网 Gateway 发现（unicast DNS-SD）。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ---------- 域名处理 ----------

// NormalizeWideAreaDomain 规范化宽域发现域名，确保以 "." 结尾。
// 空字符串返回空。
// TS 对照: normalizeWideAreaDomain (L6-12)
func NormalizeWideAreaDomain(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, ".") {
		return trimmed
	}
	return trimmed + "."
}

// ResolveWideAreaDiscoveryDomain 从配置或环境变量解析宽域发现域名。
// TS 对照: resolveWideAreaDiscoveryDomain (L14-21)
func ResolveWideAreaDiscoveryDomain(configDomain string) string {
	candidate := strings.TrimSpace(configDomain)
	if candidate == "" {
		candidate = preferredEnvValue("CRABCLAW_WIDE_AREA_DOMAIN", "OPENACOSMI_WIDE_AREA_DOMAIN")
	}
	if candidate == "" {
		return ""
	}
	return NormalizeWideAreaDomain(candidate)
}

// GetWideAreaZonePath 返回 zone 文件路径。
// TS 对照: getWideAreaZonePath (L27-29)
func GetWideAreaZonePath(domain string) string {
	filename := zoneFilenameForDomain(domain)
	return filepath.Join(resolveWideAreaConfigDir(), "dns", filename)
}

// ---------- Zone 渲染 ----------

// WideAreaGatewayZoneOpts zone 文件渲染选项。
// TS 对照: WideAreaGatewayZoneOpts (L91-104)
type WideAreaGatewayZoneOpts struct {
	Domain                      string
	GatewayPort                 int
	DisplayName                 string
	TailnetIPv4                 string
	TailnetIPv6                 string
	GatewayTLSEnabled           bool
	GatewayTLSFingerprintSHA256 string
	InstanceLabel               string
	HostLabel                   string
	TailnetDNS                  string
	SSHPort                     int
	CLIPath                     string
}

// WideAreaGatewayZoneResult zone 写入结果。
type WideAreaGatewayZoneResult struct {
	ZonePath string
	Changed  bool
}

// RenderWideAreaGatewayZoneText 渲染 DNS zone 文本（含 serial）。
// TS 对照: renderWideAreaGatewayZoneText (L162-166)
func RenderWideAreaGatewayZoneText(opts WideAreaGatewayZoneOpts, serial int) string {
	return renderZone(opts, serial)
}

// WriteWideAreaGatewayZone 渲染并写入 zone 文件。
// 文件仅在内容变化时更新（content hash 比较）。
// TS 对照: writeWideAreaGatewayZone (L168-199)
func WriteWideAreaGatewayZone(opts WideAreaGatewayZoneOpts) (*WideAreaGatewayZoneResult, error) {
	domain := NormalizeWideAreaDomain(opts.Domain)
	if domain == "" {
		return nil, fmt.Errorf("wide-area discovery domain is required")
	}

	zonePath := GetWideAreaZonePath(domain)

	// 确保目录存在
	dir := filepath.Dir(zonePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dns dir: %w", err)
	}

	// 读取已有 zone 文件
	existing := ""
	if data, err := os.ReadFile(zonePath); err == nil {
		existing = string(data)
	}

	// 用 serial=0 渲染以比较 content hash
	nextNoSerial := RenderWideAreaGatewayZoneText(opts, 0)
	nextHash := extractContentHash(nextNoSerial)
	existingHash := ""
	if existing != "" {
		existingHash = extractContentHash(existing)
	}

	// 内容未变化则跳过写入
	if existing != "" && nextHash != "" && existingHash == nextHash {
		return &WideAreaGatewayZoneResult{ZonePath: zonePath, Changed: false}, nil
	}

	// 计算新 serial
	existingSerial := 0
	if existing != "" {
		existingSerial = extractSerial(existing)
	}
	serial := nextSerialNum(existingSerial, time.Now())

	// 渲染并写入
	next := RenderWideAreaGatewayZoneText(opts, serial)
	if err := os.WriteFile(zonePath, []byte(next), 0644); err != nil {
		return nil, fmt.Errorf("write zone file: %w", err)
	}

	return &WideAreaGatewayZoneResult{ZonePath: zonePath, Changed: true}, nil
}

// ---------- 内部函数 ----------

func zoneFilenameForDomain(domain string) string {
	name := strings.TrimSuffix(domain, ".")
	return name + ".db"
}

// dnsLabel 将原始字符串规范化为合法的 DNS label。
// TS 对照: dnsLabel (L31-40)
func dnsLabel(raw, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	// 替换非 [a-z0-9-] 为 "-"
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	normalized = re.ReplaceAllString(normalized, "-")
	normalized = strings.TrimLeft(normalized, "-")
	normalized = strings.TrimRight(normalized, "-")

	out := normalized
	if out == "" {
		out = fallback
	}
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

// txtQuote 对 TXT 记录值进行 DNS 转义和引号包裹。
// TS 对照: txtQuote (L42-45)
func txtQuote(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `"` + escaped + `"`
}

// computeContentHash 计算内容的 FNV-1a 哈希（与 TS 端一致）。
// TS 对照: computeContentHash (L81-89)
func computeContentHash(body string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(body); i++ {
		h ^= uint32(body[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// formatYyyyMmDd 格式化日期为 YYYYMMDD。
func formatYyyyMmDd(t time.Time) string {
	return fmt.Sprintf("%04d%02d%02d", t.UTC().Year(), int(t.UTC().Month()), t.UTC().Day())
}

// nextSerialNum 计算下一个 SOA serial。
// TS 对照: nextSerial (L54-65)
func nextSerialNum(existingSerial int, now time.Time) int {
	today := formatYyyyMmDd(now)
	base := 0
	fmt.Sscanf(today+"01", "%d", &base)

	if existingSerial <= 0 {
		return base
	}
	existingStr := fmt.Sprintf("%d", existingSerial)
	if strings.HasPrefix(existingStr, today) {
		return existingSerial + 1
	}
	return base
}

// extractSerial 从 zone 文本中提取 SOA serial 号。
// TS 对照: extractSerial (L67-74)
func extractSerial(zoneText string) int {
	re := regexp.MustCompile(`(?m)^\s*@\s+IN\s+SOA\s+\S+\s+\S+\s+(\d+)\s+`)
	match := re.FindStringSubmatch(zoneText)
	if len(match) < 2 {
		return 0
	}
	var serial int
	fmt.Sscanf(match[1], "%d", &serial)
	return serial
}

// extractContentHash 从 zone 文本中提取 openacosmi-content-hash 注释。
// TS 对照: extractContentHash (L76-79)
func extractContentHash(zoneText string) string {
	re := regexp.MustCompile(`(?m)^\s*;\s*openacosmi-content-hash:\s*(\S+)\s*$`)
	match := re.FindStringSubmatch(zoneText)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// renderZone 渲染完整的 DNS zone 文本。
// TS 对照: renderZone (L106-160)
func renderZone(opts WideAreaGatewayZoneOpts, serial int) string {
	hostname, _ := os.Hostname()
	if idx := strings.Index(hostname, "."); idx > 0 {
		hostname = hostname[:idx]
	}
	if hostname == "" {
		hostname = "openacosmi"
	}

	hostLabel := dnsLabel(opts.HostLabel, hostname)
	if opts.HostLabel == "" {
		hostLabel = dnsLabel(hostname, "openacosmi")
	}

	instanceLabel := dnsLabel(opts.InstanceLabel, hostname+"-gateway")
	if opts.InstanceLabel == "" {
		instanceLabel = dnsLabel(hostname+"-gateway", "openacosmi-gw")
	}

	domain := NormalizeWideAreaDomain(opts.Domain)
	if domain == "" {
		domain = "local."
	}

	displayName := strings.TrimSpace(opts.DisplayName)
	if displayName == "" {
		displayName = hostname
	}

	// 构建 TXT 记录
	txt := []string{
		fmt.Sprintf("displayName=%s", displayName),
		"role=gateway",
		"transport=gateway",
		fmt.Sprintf("gatewayPort=%d", opts.GatewayPort),
	}
	if opts.GatewayTLSEnabled {
		txt = append(txt, "gatewayTls=1")
		if opts.GatewayTLSFingerprintSHA256 != "" {
			txt = append(txt, fmt.Sprintf("gatewayTlsSha256=%s", opts.GatewayTLSFingerprintSHA256))
		}
	}
	if strings.TrimSpace(opts.TailnetDNS) != "" {
		txt = append(txt, fmt.Sprintf("tailnetDns=%s", strings.TrimSpace(opts.TailnetDNS)))
	}
	if opts.SSHPort > 0 {
		txt = append(txt, fmt.Sprintf("sshPort=%d", opts.SSHPort))
	}
	if strings.TrimSpace(opts.CLIPath) != "" {
		txt = append(txt, fmt.Sprintf("cliPath=%s", strings.TrimSpace(opts.CLIPath)))
	}

	// 构建 zone records
	var records []string
	records = append(records, fmt.Sprintf("$ORIGIN %s", domain))
	records = append(records, "$TTL 60")
	soaLine := fmt.Sprintf("@ IN SOA ns1 hostmaster %d 7200 3600 1209600 60", serial)
	records = append(records, soaLine)
	records = append(records, "@ IN NS ns1")
	records = append(records, fmt.Sprintf("ns1 IN A %s", opts.TailnetIPv4))
	records = append(records, fmt.Sprintf("%s IN A %s", hostLabel, opts.TailnetIPv4))
	if opts.TailnetIPv6 != "" {
		records = append(records, fmt.Sprintf("%s IN AAAA %s", hostLabel, opts.TailnetIPv6))
	}

	records = append(records, fmt.Sprintf("_openacosmi-gw._tcp IN PTR %s._openacosmi-gw._tcp", instanceLabel))
	records = append(records, fmt.Sprintf("%s._openacosmi-gw._tcp IN SRV 0 0 %d %s", instanceLabel, opts.GatewayPort, hostLabel))

	// TXT 记录拼接
	txtParts := make([]string, len(txt))
	for i, v := range txt {
		txtParts[i] = txtQuote(v)
	}
	records = append(records, fmt.Sprintf("%s._openacosmi-gw._tcp IN TXT %s", instanceLabel, strings.Join(txtParts, " ")))

	contentBody := strings.Join(records, "\n") + "\n"

	// 计算 content hash （将 SOA serial 替换为 SERIAL 以保持稳定）
	soaHashLine := "@ IN SOA ns1 hostmaster SERIAL 7200 3600 1209600 60"
	var hashRecords []string
	for _, line := range records {
		if line == soaLine {
			hashRecords = append(hashRecords, soaHashLine)
		} else {
			hashRecords = append(hashRecords, line)
		}
	}
	hashBody := strings.Join(hashRecords, "\n") + "\n"
	contentHash := computeContentHash(hashBody)

	return fmt.Sprintf("; openacosmi-content-hash: %s\n%s", contentHash, contentBody)
}

// resolveWideAreaConfigDir 解析配置目录（复用 plugins 的 resolveConfigDir 逻辑）。
func resolveWideAreaConfigDir() string {
	if override := preferredEnvValue("CRABCLAW_CONFIG_DIR", "OPENACOSMI_CONFIG_DIR"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "openacosmi")
	}
	return filepath.Join(home, ".config", "openacosmi")
}
