package infra

// TS 对照: src/infra/tls/fingerprint.ts
// TLS 证书 SHA-256 指纹规范化与验证。

import (
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"strings"
)

// NormalizeFingerprint 规范化 TLS 指纹字符串。
// 支持输入：带/不带冒号，大/小写，带或不带 "sha256:" 前缀。
// 输出：大写冒号分隔格式 "AA:BB:CC:..."（共 32 组）。
// TS 对照: fingerprint.ts normalizeFingerprint (L1-5)
func NormalizeFingerprint(raw string) string {
	trimmed := strings.TrimSpace(raw)
	// 去除 sha-256 / sha256 前缀（大小写不敏感）
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"sha-256:", "sha256:", "sha-256 ", "sha256 "} {
		if strings.HasPrefix(lower, prefix) {
			trimmed = trimmed[len(prefix):]
			lower = lower[len(prefix):]
			break
		}
	}

	// 保留十六进制字符，去除所有分隔符
	var hexOnly strings.Builder
	for _, ch := range strings.ToUpper(trimmed) {
		if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F') {
			hexOnly.WriteRune(ch)
		}
	}
	hex := hexOnly.String()

	// SHA-256 指纹为 32 字节 = 64 个十六进制字符
	if len(hex) != 64 {
		return ""
	}

	// 拼成 AA:BB:CC:... 格式（32 组）
	parts := make([]string, 32)
	for i := 0; i < 32; i++ {
		parts[i] = hex[i*2 : i*2+2]
	}
	return strings.Join(parts, ":")
}

// GetCertFingerprint 获取 x509 证书的 SHA-256 指纹（规范化格式）。
// 返回 "AA:BB:CC:..." 大写冒号分隔形式。
func GetCertFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// VerifyCertFingerprint 验证 x509 证书的 SHA-256 指纹是否与期望值匹配。
// expectedFP 可以是任意被 NormalizeFingerprint 支持的格式。
// 返回 true 表示指纹匹配。
func VerifyCertFingerprint(cert *x509.Certificate, expectedFP string) bool {
	expected := NormalizeFingerprint(expectedFP)
	if expected == "" {
		return false
	}
	actual := GetCertFingerprint(cert)
	return actual == expected
}
