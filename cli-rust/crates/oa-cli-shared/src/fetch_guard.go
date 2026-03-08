package infra

// fetch_guard.go — 网络请求安全守卫 + SSRF 防护
// 对应 TS:
//   - src/infra/net/fetch-guard.ts (171L)
//   - src/infra/net/ssrf.ts (308L)
//
// 防止 SSRF 攻击：私有 IP 地址校验 + 危险主机名检测。

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ─── SSRF 检测 ───

// SsrfBlockedError SSRF 拦截错误。
type SsrfBlockedError struct {
	Message string
}

func (e *SsrfBlockedError) Error() string {
	return e.Message
}

// SsrfPolicy SSRF 策略。
type SsrfPolicy struct {
	AllowPrivateNetwork bool     `json:"allowPrivateNetwork,omitempty"`
	AllowedHostnames    []string `json:"allowedHostnames,omitempty"`
}

// blockedHostnames 危险主机名集合。
var blockedHostnames = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true,
}

// blockedSuffixes 危险主机名后缀。
var blockedSuffixes = []string{
	".localhost",
	".local",
	".internal",
}

// NormalizeHostname 规范化主机名。
func NormalizeHostname(hostname string) string {
	normalized := strings.TrimSpace(strings.ToLower(hostname))
	normalized = strings.TrimSuffix(normalized, ".")
	if strings.HasPrefix(normalized, "[") && strings.HasSuffix(normalized, "]") {
		normalized = normalized[1 : len(normalized)-1]
	}
	return normalized
}

// IsPrivateIPAddress 检查 IP 地址是否为私有/内部地址。
// 对应 TS: isPrivateIpAddress(address)
func IsPrivateIPAddress(address string) bool {
	normalized := NormalizeHostname(address)
	if normalized == "" {
		return false
	}

	ip := net.ParseIP(normalized)
	if ip == nil {
		return false
	}

	// RFC 1918 + 链路本地 + 环回
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	// 常见私有 CIDR 范围
	privateCIDRs := []string{
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"100.64.0.0/10",  // RFC 6598 (CGNAT)
		"169.254.0.0/16", // RFC 3927 (link-local)
	}

	for _, cidrStr := range privateCIDRs {
		_, cidr, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	// IPv6 私有范围
	if ip4 := ip.To4(); ip4 == nil {
		// 纯 IPv6
		normalized := ip.String()
		privatePrefixes := []string{"fe80:", "fec0:", "fc", "fd"}
		for _, prefix := range privatePrefixes {
			if strings.HasPrefix(normalized, prefix) {
				return true
			}
		}
	}

	return false
}

// IsBlockedHostname 检查主机名是否被拦截。
// 对应 TS: isBlockedHostname(hostname)
func IsBlockedHostname(hostname string) bool {
	normalized := NormalizeHostname(hostname)
	if normalized == "" {
		return false
	}
	if blockedHostnames[normalized] {
		return true
	}
	for _, suffix := range blockedSuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

// ValidateURLForSSRF 验证 URL 是否安全（非 SSRF）。
func ValidateURLForSSRF(hostname string, policy *SsrfPolicy) error {
	if policy != nil && policy.AllowPrivateNetwork {
		return nil
	}

	normalized := NormalizeHostname(hostname)
	if normalized == "" {
		return &SsrfBlockedError{Message: "invalid hostname"}
	}

	isAllowed := false
	if policy != nil {
		for _, allowed := range policy.AllowedHostnames {
			if NormalizeHostname(allowed) == normalized {
				isAllowed = true
				break
			}
		}
	}

	if !isAllowed {
		if IsBlockedHostname(normalized) {
			return &SsrfBlockedError{Message: fmt.Sprintf("blocked hostname: %s", hostname)}
		}
		if IsPrivateIPAddress(normalized) {
			return &SsrfBlockedError{Message: "blocked: private/internal IP address"}
		}
	}

	return nil
}

// ─── 安全 HTTP 客户端 ───

// SafeHTTPClient 创建带 SSRF 防护的 HTTP 客户端。
// 对应 TS: createSafeHttpClient(policy)
func SafeHTTPClient(policy *SsrfPolicy, timeoutMs int) *http.Client {
	if timeoutMs <= 0 {
		timeoutMs = 10_000
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
	}

	return &http.Client{
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// 对重定向目标也做 SSRF 检查
			if err := ValidateURLForSSRF(req.URL.Hostname(), policy); err != nil {
				return err
			}
			return nil
		},
	}
}
