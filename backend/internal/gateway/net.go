// Package gateway 实现网关核心层：HTTP/WS 路由、认证、广播等。
package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	HeaderToken         = "X-CrabClaw-Token"
	HeaderLegacyToken   = "X-OpenAcosmi-Token"
	HeaderAgentID       = "X-CrabClaw-Agent-Id"
	HeaderLegacyAgentID = "X-OpenAcosmi-Agent-Id"
	HeaderAgent         = "X-CrabClaw-Agent"
	HeaderLegacyAgent   = "X-OpenAcosmi-Agent"
)

// ---------- IP 地址工具 ----------

// IsLoopbackAddress 判断 IP 是否为回环地址（127.x.x.x / ::1 / ::ffff:127.x）。
func IsLoopbackAddress(ip string) bool {
	if ip == "" {
		return false
	}
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	if strings.HasPrefix(ip, "127.") {
		return true
	}
	if strings.HasPrefix(ip, "::ffff:127.") {
		return true
	}
	return false
}

// NormalizeIPv4Mapped 去除 IPv4-mapped IPv6 前缀 "::ffff:"。
func NormalizeIPv4Mapped(ip string) string {
	if strings.HasPrefix(ip, "::ffff:") {
		return ip[len("::ffff:"):]
	}
	return ip
}

// NormalizeIP 规范化 IP 地址：trim + lowercase + 去 IPv4-mapped 前缀。
func NormalizeIP(ip string) string {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return ""
	}
	return NormalizeIPv4Mapped(strings.ToLower(trimmed))
}

// StripOptionalPort 从 "ip:port" 或 "[ip]:port" 中提取纯 IP 部分。
func StripOptionalPort(ip string) string {
	if strings.HasPrefix(ip, "[") {
		end := strings.Index(ip, "]")
		if end != -1 {
			return ip[1:end]
		}
	}
	parsed := net.ParseIP(ip)
	if parsed != nil {
		return ip
	}
	lastColon := strings.LastIndex(ip, ":")
	if lastColon > -1 && strings.Contains(ip, ".") && strings.Index(ip, ":") == lastColon {
		candidate := ip[:lastColon]
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return ip
}

// ParseForwardedForClientIP 解析 X-Forwarded-For 头中第一个 IP。
func ParseForwardedForClientIP(forwardedFor string) string {
	parts := strings.Split(forwardedFor, ",")
	raw := strings.TrimSpace(parts[0])
	if raw == "" {
		return ""
	}
	return NormalizeIP(StripOptionalPort(raw))
}

// IsTrustedProxyAddress 检查 IP 是否在信任代理列表中。
func IsTrustedProxyAddress(ip string, trustedProxies []string) bool {
	normalized := NormalizeIP(ip)
	if normalized == "" || len(trustedProxies) == 0 {
		return false
	}
	for _, proxy := range trustedProxies {
		if NormalizeIP(proxy) == normalized {
			return true
		}
	}
	return false
}

// ResolveGatewayClientIP 从请求元数据中解析真实客户端 IP。
// 如果 remoteAddr 是可信代理，则使用 X-Forwarded-For 或 X-Real-IP。
func ResolveGatewayClientIP(remoteAddr, forwardedFor, realIP string, trustedProxies []string) string {
	remote := NormalizeIP(StripOptionalPort(remoteAddr))
	if remote == "" {
		return ""
	}
	if !IsTrustedProxyAddress(remote, trustedProxies) {
		return remote
	}
	if ff := ParseForwardedForClientIP(forwardedFor); ff != "" {
		return ff
	}
	if ri := parseRealIP(realIP); ri != "" {
		return ri
	}
	return remote
}

func parseRealIP(realIP string) string {
	raw := strings.TrimSpace(realIP)
	if raw == "" {
		return ""
	}
	return NormalizeIP(StripOptionalPort(raw))
}

// IsValidIPv4 验证字符串是否为合法 IPv4 地址。
func IsValidIPv4(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return false
		}
		if part != strconv.Itoa(n) {
			return false // 拒绝前导零
		}
	}
	return true
}

// PickPrimaryLanIPv4 选取本机主要 IPv4 LAN 地址。
// 优先 en0/eth0，再 fallback 到任意非内部 IPv4。
func PickPrimaryLanIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	preferred := []string{"en0", "eth0"}
	for _, name := range preferred {
		if addr := findIPv4ForInterface(ifaces, name); addr != "" {
			return addr
		}
	}
	for _, iface := range ifaces {
		if addr := getExternalIPv4(iface); addr != "" {
			return addr
		}
	}
	return ""
}

func findIPv4ForInterface(ifaces []net.Interface, name string) string {
	for _, iface := range ifaces {
		if iface.Name == name {
			return getExternalIPv4(iface)
		}
	}
	return ""
}

func getExternalIPv4(iface net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

// CanBindToHost 测试是否能绑定到指定 host 地址（使用 ephemeral port）。
func CanBindToHost(host string) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// BindMode 网关绑定模式。
type BindMode string

const (
	BindLoopback BindMode = "loopback"
	BindLAN      BindMode = "lan"
	BindTailnet  BindMode = "tailnet"
	BindAuto     BindMode = "auto"
	BindCustom   BindMode = "custom"
)

// ResolveGatewayBindHost 根据绑定模式解析绑定地址。
func ResolveGatewayBindHost(mode BindMode, customHost string) string {
	switch mode {
	case BindLoopback, "":
		if CanBindToHost("127.0.0.1") {
			return "127.0.0.1"
		}
		return "0.0.0.0"
	case BindLAN:
		return "0.0.0.0"
	case BindTailnet:
		if tailIP := resolveTailnetIP(); tailIP != "" && CanBindToHost(tailIP) {
			return tailIP
		}
		if CanBindToHost("127.0.0.1") {
			return "127.0.0.1"
		}
		return "0.0.0.0"
	case BindAuto:
		if CanBindToHost("127.0.0.1") {
			return "127.0.0.1"
		}
		return "0.0.0.0"
	case BindCustom:
		host := strings.TrimSpace(customHost)
		if host == "" {
			return "0.0.0.0"
		}
		if IsValidIPv4(host) && CanBindToHost(host) {
			return host
		}
		return "0.0.0.0"
	default:
		return "0.0.0.0"
	}
}

// ResolveGatewayListenHosts 解析监听地址列表。
// 如果绑定 127.0.0.1 且 ::1 可用，则同时监听两者。
func ResolveGatewayListenHosts(bindHost string) []string {
	if bindHost != "127.0.0.1" {
		return []string{bindHost}
	}
	if CanBindToHost("::1") {
		return []string{bindHost, "::1"}
	}
	return []string{bindHost}
}

// resolveTailnetIP 通过 tailscale status --json 查询本机 Tailscale IP。
// 返回第一个 IPv4 地址，失败时返回空字符串。
func resolveTailnetIP() string {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return ""
	}

	// 解析 JSON 中的 Self.TailscaleIPs
	var status struct {
		Self struct {
			TailscaleIPs []string `json:"TailscaleIPs"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return ""
	}

	// 优先选择 IPv4 地址
	for _, ip := range status.Self.TailscaleIPs {
		if IsValidIPv4(ip) {
			return ip
		}
	}
	// 降级：返回任意可用 IP
	if len(status.Self.TailscaleIPs) > 0 {
		return status.Self.TailscaleIPs[0]
	}
	return ""
}

// ---------- HTTP 请求工具 ----------

// GetHeader 获取请求头的单个值（大小写不敏感）。
func GetHeader(r *http.Request, name string) string {
	return r.Header.Get(name)
}

// GetBearerToken 从 Authorization 头提取 Bearer token。
func GetBearerToken(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return ""
	}
	token := strings.TrimSpace(raw[7:])
	return token
}

// GetCompatHeader 按顺序读取兼容 header 名，返回首个非空值。
func GetCompatHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(GetHeader(r, name)); value != "" {
			return value
		}
	}
	return ""
}

// GetGatewayToken 从 Authorization 或兼容 token header 提取网关认证 token。
func GetGatewayToken(r *http.Request) string {
	if token := GetBearerToken(r); token != "" {
		return token
	}
	return GetCompatHeader(r, HeaderToken, HeaderLegacyToken)
}

// GetHostName 从 Host 头提取主机名（去端口、去 IPv6 括号）。
func GetHostName(hostHeader string) string {
	host := strings.TrimSpace(strings.ToLower(hostHeader))
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "[") {
		end := strings.Index(host, "]")
		if end != -1 {
			return host[1:end]
		}
	}
	parts := strings.SplitN(host, ":", 2)
	return parts[0]
}

// ReadJSONBody 读取 HTTP 请求体并解析为 JSON。
// maxBytes 限制最大读取字节数。
func ReadJSONBody(r *http.Request, maxBytes int64) (interface{}, error) {
	if r.Body == nil {
		return map[string]interface{}{}, nil
	}
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrPayloadTooLarge
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return map[string]interface{}{}, nil
	}
	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return result, nil
}

// ErrPayloadTooLarge body 超过大小限制。
var ErrPayloadTooLarge = fmt.Errorf("payload too large")

// ---------- HTTP 响应工具 ----------

// SendJSON 发送 JSON 响应。
func SendJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	w.Write(data)
}

// SendText 发送纯文本响应。
func SendText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// SendMethodNotAllowed 发送 405 响应。
func SendMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	SendText(w, http.StatusMethodNotAllowed, "Method Not Allowed")
}

// SendUnauthorized 发送 401 JSON 响应。
func SendUnauthorized(w http.ResponseWriter) {
	SendJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"error": map[string]string{
			"message": "Unauthorized",
			"type":    "unauthorized",
		},
	})
}

// SendInvalidRequest 发送 400 JSON 响应。
func SendInvalidRequest(w http.ResponseWriter, message string) {
	SendJSON(w, http.StatusBadRequest, map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"type":    "invalid_request_error",
		},
	})
}

// SetSSEHeaders 设置 Server-Sent Events 响应头。
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// WriteSSEDone 写入 SSE 结束标记。
func WriteSSEDone(w http.ResponseWriter) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------- 环境变量工具 ----------

// GetEnvOrDefault 获取环境变量，不存在则返回默认值。
func GetEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
