package infra

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ---------- 发现结果 ----------

// GatewayBonjourBeacon 发现的网关信标。
type GatewayBonjourBeacon struct {
	InstanceName                string            `json:"instanceName"`
	Domain                      string            `json:"domain,omitempty"`
	DisplayName                 string            `json:"displayName,omitempty"`
	Host                        string            `json:"host,omitempty"`
	Port                        int               `json:"port,omitempty"`
	LanHost                     string            `json:"lanHost,omitempty"`
	TailnetDNS                  string            `json:"tailnetDns,omitempty"`
	GatewayPort                 int               `json:"gatewayPort,omitempty"`
	SSHPort                     int               `json:"sshPort,omitempty"`
	GatewayTLS                  bool              `json:"gatewayTls,omitempty"`
	GatewayTLSFingerprintSHA256 string            `json:"gatewayTlsFingerprintSha256,omitempty"`
	CLIPath                     string            `json:"cliPath,omitempty"`
	Role                        string            `json:"role,omitempty"`
	Transport                   string            `json:"transport,omitempty"`
	TXT                         map[string]string `json:"txt,omitempty"`
}

// DiscoverOpts 发现选项。
type DiscoverOpts struct {
	TimeoutMs int
	Domains   []string
	Platform  string // "darwin", "linux"
}

const (
	defaultDiscoverTimeoutMs = 2000
	gatewayServiceType       = "_openacosmi-gw._tcp"
)

// ---------- 顶层 API ----------

// DiscoverGatewayBeacons 发现局域网网关集群。
func DiscoverGatewayBeacons(ctx context.Context, opts DiscoverOpts) ([]GatewayBonjourBeacon, error) {
	timeoutMs := opts.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultDiscoverTimeoutMs
	}
	platform := opts.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	domains := opts.Domains
	if len(domains) == 0 {
		domains = []string{"local."}
	}
	// 规范化 domain (确保以 . 结尾)
	for i, d := range domains {
		d = strings.TrimSpace(d)
		if !strings.HasSuffix(d, ".") {
			d += "."
		}
		domains[i] = d
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch platform {
	case "darwin":
		return discoverViaDnsSd(ctx, domains, timeoutMs)
	case "linux":
		return discoverViaAvahi(ctx, domains, timeoutMs)
	default:
		return nil, nil
	}
}

// ---------- macOS dns-sd ----------

func discoverViaDnsSd(ctx context.Context, domains []string, timeoutMs int) ([]GatewayBonjourBeacon, error) {
	var all []GatewayBonjourBeacon
	for _, domain := range domains {
		beacons, err := discoverDnsSdDomain(ctx, domain, timeoutMs)
		if err != nil {
			continue // best-effort
		}
		all = append(all, beacons...)
	}
	return all, nil
}

func discoverDnsSdDomain(ctx context.Context, domain string, timeoutMs int) ([]GatewayBonjourBeacon, error) {
	out, err := runCmd(ctx, timeoutMs, "dns-sd", "-B", gatewayServiceType, domain)
	if err != nil {
		return nil, err
	}
	instances := parseDnsSdBrowse(out)
	var results []GatewayBonjourBeacon
	for _, inst := range instances {
		resolved, err := runCmd(ctx, timeoutMs, "dns-sd", "-L", inst, gatewayServiceType, domain)
		if err != nil {
			continue
		}
		beacon := parseDnsSdResolve(resolved, inst)
		if beacon != nil {
			beacon.Domain = domain
			results = append(results, *beacon)
		}
	}
	return results, nil
}

// ---------- Linux avahi ----------

func discoverViaAvahi(ctx context.Context, domains []string, timeoutMs int) ([]GatewayBonjourBeacon, error) {
	var all []GatewayBonjourBeacon
	for _, domain := range domains {
		args := []string{"avahi-browse", "-rt", gatewayServiceType}
		if domain != "local." {
			args = append(args, "-d", strings.TrimSuffix(domain, "."))
		}
		out, err := runCmd(ctx, timeoutMs, args[0], args[1:]...)
		if err != nil {
			continue
		}
		beacons := parseAvahiBrowse(out)
		for i := range beacons {
			beacons[i].Domain = domain
		}
		all = append(all, beacons...)
	}
	return all, nil
}

// ---------- 命令执行 ----------

func runCmd(ctx context.Context, timeoutMs int, name string, args ...string) (string, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cmd %s: %w", name, err)
	}
	return string(out), nil
}

// ---------- 解析函数 ----------

var dnsSdAddRE = regexp.MustCompile(`_openacosmi-gw\._tcp\.?\s+(.+)$`)

func parseDnsSdBrowse(stdout string) []string {
	seen := map[string]struct{}{}
	var instances []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, gatewayServiceType) || !strings.Contains(line, "Add") {
			continue
		}
		m := dnsSdAddRE.FindStringSubmatch(line)
		if m != nil && m[1] != "" {
			name := decodeDnsSdEscapes(strings.TrimSpace(m[1]))
			if _, dup := seen[name]; !dup {
				seen[name] = struct{}{}
				instances = append(instances, name)
			}
		}
	}
	return instances
}

func parseDnsSdResolve(stdout, instanceName string) *GatewayBonjourBeacon {
	decoded := decodeDnsSdEscapes(instanceName)
	beacon := &GatewayBonjourBeacon{InstanceName: decoded}
	var txt map[string]string

	reachRE := regexp.MustCompile(`can be reached at\s+([^\s:]+):(\d+)`)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := reachRE.FindStringSubmatch(line); m != nil {
			beacon.Host = strings.TrimSuffix(m[1], ".")
			beacon.Port, _ = strconv.Atoi(m[2])
			continue
		}
		if strings.HasPrefix(line, "txt") || strings.Contains(line, "txtvers=") {
			tokens := strings.Fields(line)
			txt = parseTxtTokens(tokens)
		}
	}

	applyTxtToBeacon(beacon, txt)
	if beacon.DisplayName == "" {
		beacon.DisplayName = decoded
	}
	return beacon
}

func parseAvahiBrowse(stdout string) []GatewayBonjourBeacon {
	var results []GatewayBonjourBeacon
	var current *GatewayBonjourBeacon

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "=") && strings.Contains(line, gatewayServiceType) {
			if current != nil {
				results = append(results, *current)
			}
			marker := " " + gatewayServiceType
			idx := strings.Index(line, marker)
			left := line
			if idx >= 0 {
				left = strings.TrimSpace(line[:idx])
			}
			parts := strings.Fields(left)
			name := left
			if len(parts) > 3 {
				name = strings.Join(parts[3:], " ")
			}
			current = &GatewayBonjourBeacon{InstanceName: name, DisplayName: name}
			continue
		}
		if current == nil {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "hostname =") {
			if m := regexp.MustCompile(`hostname\s*=\s*\[([^\]]+)\]`).FindStringSubmatch(trimmed); m != nil {
				current.Host = m[1]
			}
		} else if strings.HasPrefix(trimmed, "port =") {
			if m := regexp.MustCompile(`port\s*=\s*\[(\d+)\]`).FindStringSubmatch(trimmed); m != nil {
				current.Port, _ = strconv.Atoi(m[1])
			}
		} else if strings.HasPrefix(trimmed, "txt =") {
			tokens := regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(trimmed, -1)
			var flat []string
			for _, t := range tokens {
				flat = append(flat, t[1])
			}
			txt := parseTxtTokens(flat)
			applyTxtToBeacon(current, txt)
		}
	}
	if current != nil {
		results = append(results, *current)
	}
	return results
}

// ---------- TXT 解析 ----------

func parseTxtTokens(tokens []string) map[string]string {
	txt := map[string]string{}
	for _, token := range tokens {
		idx := strings.Index(token, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(token[:idx])
		value := decodeDnsSdEscapes(strings.TrimSpace(token[idx+1:]))
		if key != "" {
			txt[key] = value
		}
	}
	return txt
}

func applyTxtToBeacon(b *GatewayBonjourBeacon, txt map[string]string) {
	if len(txt) > 0 {
		b.TXT = txt
	}
	if v := txt["displayName"]; v != "" {
		b.DisplayName = decodeDnsSdEscapes(v)
	}
	if v := txt["lanHost"]; v != "" {
		b.LanHost = v
	}
	if v := txt["tailnetDns"]; v != "" {
		b.TailnetDNS = v
	}
	if v := txt["cliPath"]; v != "" {
		b.CLIPath = v
	}
	b.GatewayPort = parseIntOr(txt["gatewayPort"], 0)
	b.SSHPort = parseIntOr(txt["sshPort"], 0)
	if v := txt["gatewayTls"]; isTruthyEnv(v) {
		b.GatewayTLS = true
	}
	if v := txt["gatewayTlsSha256"]; v != "" {
		b.GatewayTLSFingerprintSHA256 = v
	}
	if v := txt["role"]; v != "" {
		b.Role = v
	}
	if v := txt["transport"]; v != "" {
		b.Transport = v
	}
}

func parseIntOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

// decodeDnsSdEscapes 解码 DNS-SD 转义序列 (\NNN)。
func decodeDnsSdEscapes(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var buf []byte
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+3 < len(value) {
			escaped := value[i+1 : i+4]
			if isDigit3(escaped) {
				n, _ := strconv.Atoi(escaped)
				if n >= 0 && n <= 255 {
					buf = append(buf, byte(n))
					i += 3
					continue
				}
			}
		}
		buf = append(buf, value[i])
	}
	return string(buf)
}

func isDigit3(s string) bool {
	return len(s) == 3 && s[0] >= '0' && s[0] <= '9' && s[1] >= '0' && s[1] <= '9' && s[2] >= '0' && s[2] <= '9'
}
