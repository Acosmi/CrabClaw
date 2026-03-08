package infra

// TS 对照: src/infra/tls/gateway.ts
// Gateway TLS 自签名证书生成与加载。

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GenerateGatewayCert 生成 Gateway TLS 自签名证书。
// 包含 localhost、openacosmi.local SAN 及 127.0.0.1、::1 IP SAN。
// 使用 ECDSA P-256 密钥，有效期 10 年。
// TS 对照: gateway.ts generateSelfSignedCert (L33-65)
func GenerateGatewayCert() (*tls.Certificate, error) {
	// 生成 ECDSA P-256 私钥
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// 序列号使用随机大整数
	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialMax)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "openacosmi-gateway",
			Organization: []string{"openacosmi"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		// SAN: DNS names
		DNSNames: []string{
			"localhost",
			"openacosmi.local",
		},
		// SAN: IP addresses
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}

	// 自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// SaveGatewayCert 将证书和私钥保存到 PEM 文件。
func SaveGatewayCert(cert *tls.Certificate, certFile, keyFile string) error {
	if len(cert.Certificate) == 0 {
		return errors.New("tls_gateway: empty certificate")
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		return err
	}

	// 将私钥序列化
	ecKey, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return errors.New("tls_gateway: unsupported private key type")
	}
	privDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})
	return os.WriteFile(keyFile, keyPEM, 0o600)
}

// isCertExpired 检查 DER 证书是否已过期。
func isCertExpired(certDER []byte) bool {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return true
	}
	return time.Now().After(cert.NotAfter)
}

// LoadOrGenerateGatewayCert 从文件加载 TLS 证书；
// 若不存在或已过期则生成新证书并保存到文件。
// TS 对照: gateway.ts loadGatewayTlsRuntime (L67-150)
func LoadOrGenerateGatewayCert(certFile, keyFile string) (*tls.Certificate, error) {
	// 尝试加载现有证书
	if _, errCert := os.Stat(certFile); errCert == nil {
		if _, errKey := os.Stat(keyFile); errKey == nil {
			existing, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err == nil && len(existing.Certificate) > 0 && !isCertExpired(existing.Certificate[0]) {
				return &existing, nil
			}
		}
	}

	// 生成新证书
	cert, err := GenerateGatewayCert()
	if err != nil {
		return nil, err
	}

	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(certFile), 0o700); err != nil {
		return nil, err
	}
	if dir := filepath.Dir(keyFile); dir != filepath.Dir(certFile) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}

	if err := SaveGatewayCert(cert, certFile, keyFile); err != nil {
		return nil, err
	}
	return cert, nil
}
