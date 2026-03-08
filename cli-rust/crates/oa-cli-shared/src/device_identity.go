package infra

// device_identity.go — 设备身份管理
// 对应 TS: src/infra/device-identity.ts
//
// 负责生成或加载设备的 ED25519 密钥对，以及派生设备唯一 ID。
// 设备身份文件存储在 stateDir/identity/device.json（权限 0600）。

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// storedIdentity 与磁盘文件对应的 JSON 结构。
// 对应 TS: StoredIdentity { version, deviceId, publicKeyPem, privateKeyPem, createdAtMs }
type storedIdentity struct {
	Version       int    `json:"version"`
	DeviceID      string `json:"deviceId"`
	PublicKeyPem  string `json:"publicKeyPem"`
	PrivateKeyPem string `json:"privateKeyPem"`
	CreatedAtMs   int64  `json:"createdAtMs"`
}

// DeviceIdentity 导出的设备身份（不含 createdAtMs，供上层使用）。
// 对应 TS: DeviceIdentity { deviceId, publicKeyPem, privateKeyPem }
type DeviceIdentity struct {
	DeviceID      string `json:"deviceId"`
	PublicKeyPem  string `json:"publicKeyPem"`
	PrivateKeyPem string `json:"privateKeyPem"`
}

// ed25519SPKIPrefix 是 ED25519 SubjectPublicKeyInfo (SPKI) DER 编码的固定前缀（12 字节）。
// 对应 TS: ED25519_SPKI_PREFIX = Buffer.from("302a300506032b6570032100", "hex")
var ed25519SPKIPrefix = []byte{0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00}

// encodePublicKeyPEM 将 ED25519 公钥编码为 PEM 格式（SPKI/PUBLIC KEY）。
func encodePublicKeyPEM(pub ed25519.PublicKey) string {
	// 构造 SPKI DER: 12 字节前缀 + 32 字节原始公钥
	der := make([]byte, len(ed25519SPKIPrefix)+ed25519.PublicKeySize)
	copy(der, ed25519SPKIPrefix)
	copy(der[len(ed25519SPKIPrefix):], pub)
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}
	return string(pem.EncodeToMemory(block))
}

// encodePrivateKeyPEM 将 ED25519 私钥编码为 PKCS8 PEM 格式。
// ED25519 PKCS8 结构（RFC 8410）:
//
//	SEQUENCE {
//	  INTEGER 0,
//	  SEQUENCE { OID 1.3.101.112 },
//	  OCTET STRING { OCTET STRING { <32 byte seed> } }
//	}
func encodePrivateKeyPEM(priv ed25519.PrivateKey) string {
	seed := priv.Seed() // 32 字节种子

	// 内层 OCTET STRING: 04 20 <32 bytes seed>
	innerOctet := make([]byte, 2+len(seed))
	innerOctet[0] = 0x04
	innerOctet[1] = byte(len(seed))
	copy(innerOctet[2:], seed)

	// 外层 OCTET STRING: 04 <len(innerOctet)> <innerOctet>
	outerOctet := make([]byte, 2+len(innerOctet))
	outerOctet[0] = 0x04
	outerOctet[1] = byte(len(innerOctet))
	copy(outerOctet[2:], innerOctet)

	// AlgorithmIdentifier: SEQUENCE { OID 1.3.101.112 }
	// OID 1.3.101.112 DER: 06 03 2b 65 70
	algID := []byte{0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70}

	// version INTEGER 0: 02 01 00
	version := []byte{0x02, 0x01, 0x00}

	// PKCS8 body
	body := make([]byte, 0, len(version)+len(algID)+len(outerOctet))
	body = append(body, version...)
	body = append(body, algID...)
	body = append(body, outerOctet...)

	// Outer SEQUENCE
	der := derSequence(body)

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}
	return string(pem.EncodeToMemory(block))
}

// derSequence 将 body 包装为 DER SEQUENCE。
func derSequence(body []byte) []byte {
	lenBytes := derEncodeLength(len(body))
	result := make([]byte, 1+len(lenBytes)+len(body))
	result[0] = 0x30
	copy(result[1:], lenBytes)
	copy(result[1+len(lenBytes):], body)
	return result
}

// derEncodeLength 编码 DER 长度字段。
func derEncodeLength(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	var bytes []byte
	tmp := n
	for tmp > 0 {
		bytes = append([]byte{byte(tmp & 0xff)}, bytes...)
		tmp >>= 8
	}
	return append([]byte{byte(0x80 | len(bytes))}, bytes...)
}

// fingerprintPublicKey 从公钥 PEM 派生设备 ID（SHA256 hex，与 TS 一致）。
// TS: fingerprintPublicKey → sha256(raw_public_key_bytes).digest("hex")
func fingerprintPublicKey(pubKeyPem string) (string, error) {
	block, _ := pem.Decode([]byte(pubKeyPem))
	if block == nil {
		return "", fmt.Errorf("无法解析公钥 PEM")
	}
	der := block.Bytes
	// 去掉 SPKI 前缀，取最后 32 字节（原始 ED25519 公钥）
	prefixLen := len(ed25519SPKIPrefix)
	if len(der) == prefixLen+ed25519.PublicKeySize {
		raw := der[prefixLen:]
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:]), nil
	}
	// fallback：对整个 DER 取 hash
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// generateIdentity 生成新的 ED25519 密钥对和设备 ID。
func generateIdentity() (*DeviceIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成 ED25519 密钥对失败: %w", err)
	}
	pubPem := encodePublicKeyPEM(pub)
	privPem := encodePrivateKeyPEM(priv)
	deviceID, err := fingerprintPublicKey(pubPem)
	if err != nil {
		return nil, fmt.Errorf("派生设备 ID 失败: %w", err)
	}
	return &DeviceIdentity{
		DeviceID:      deviceID,
		PublicKeyPem:  pubPem,
		PrivateKeyPem: privPem,
	}, nil
}

// LoadOrCreateDeviceIdentity 读取或生成设备身份。
// 对应 TS: loadOrCreateDeviceIdentity(filePath?)
//
// 文件路径：stateDir/identity/device.json
// 如果文件存在且格式正确，直接返回；否则生成新密钥对并写入。
// 写入权限为 0600（仅当前用户可读）。
func LoadOrCreateDeviceIdentity(stateDir string) (*DeviceIdentity, error) {
	filePath := filepath.Join(stateDir, "identity", "device.json")

	// 尝试读取已有文件
	identity, err := tryLoadIdentity(filePath)
	if err == nil && identity != nil {
		return identity, nil
	}

	// 生成新密钥对
	identity, err = generateIdentity()
	if err != nil {
		return nil, err
	}

	// 写入文件
	if writeErr := writeIdentityFile(filePath, identity); writeErr != nil {
		return nil, writeErr
	}

	return identity, nil
}

// tryLoadIdentity 尝试从文件加载设备身份，失败返回 nil。
func tryLoadIdentity(filePath string) (*DeviceIdentity, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var stored storedIdentity
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("解析 device.json 失败: %w", err)
	}

	// 验证版本和必填字段
	if stored.Version != 1 ||
		stored.DeviceID == "" ||
		stored.PublicKeyPem == "" ||
		stored.PrivateKeyPem == "" {
		return nil, fmt.Errorf("device.json 格式不合法")
	}

	// 重新派生 deviceId 以防止文件被篡改
	derivedID, err := fingerprintPublicKey(stored.PublicKeyPem)
	if err != nil {
		return nil, fmt.Errorf("重新派生设备 ID 失败: %w", err)
	}

	// 如果 deviceId 不一致，修正并写回（与 TS 行为一致）
	if derivedID != stored.DeviceID {
		corrected := &DeviceIdentity{
			DeviceID:      derivedID,
			PublicKeyPem:  stored.PublicKeyPem,
			PrivateKeyPem: stored.PrivateKeyPem,
		}
		// best-effort 写回，忽略错误
		_ = writeIdentityFile(filePath, corrected)
		return corrected, nil
	}

	return &DeviceIdentity{
		DeviceID:      stored.DeviceID,
		PublicKeyPem:  stored.PublicKeyPem,
		PrivateKeyPem: stored.PrivateKeyPem,
	}, nil
}

// writeIdentityFile 将设备身份原子写入文件（0600 权限）。
func writeIdentityFile(filePath string, identity *DeviceIdentity) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建 identity 目录失败: %w", err)
	}

	stored := storedIdentity{
		Version:       1,
		DeviceID:      identity.DeviceID,
		PublicKeyPem:  identity.PublicKeyPem,
		PrivateKeyPem: identity.PrivateKeyPem,
		CreatedAtMs:   time.Now().UnixMilli(),
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 device.json 失败: %w", err)
	}
	// 追加换行（与 TS 格式一致）
	data = append(data, '\n')

	// 原子写：写临时文件后 rename
	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	// best-effort chmod（某些系统 WriteFile 的 mode 可能受 umask 影响）
	_ = os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, filePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("原子替换 device.json 失败: %w", err)
	}
	return nil
}

// SignDevicePayload 使用设备私钥对 payload 签名，返回 base64url 编码。
// 对应 TS: signDevicePayload(privateKeyPem, payload)
func SignDevicePayload(privateKeyPem, payload string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPem))
	if block == nil {
		return "", fmt.Errorf("无法解析私钥 PEM")
	}
	priv, err := parseED25519PrivateKeyFromPKCS8(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("解析 ED25519 私钥失败: %w", err)
	}
	sig := ed25519.Sign(priv, []byte(payload))
	return base64URLEncode(sig), nil
}

// VerifyDeviceSignature 验证设备签名。
// publicKey 可以是 PEM 格式或 base64url 原始公钥。
// 对应 TS: verifyDeviceSignature(publicKey, payload, signatureBase64Url)
func VerifyDeviceSignature(publicKey, payload, signatureBase64URL string) bool {
	sig, err := base64URLDecode(signatureBase64URL)
	if err != nil {
		// 兼容标准 base64
		sig, err = base64.StdEncoding.DecodeString(signatureBase64URL)
		if err != nil {
			return false
		}
	}

	var pub ed25519.PublicKey
	if strings.HasPrefix(strings.TrimSpace(publicKey), "-----") {
		block, _ := pem.Decode([]byte(publicKey))
		if block == nil {
			return false
		}
		der := block.Bytes
		prefixLen := len(ed25519SPKIPrefix)
		if len(der) < prefixLen+ed25519.PublicKeySize {
			return false
		}
		pub = ed25519.PublicKey(der[prefixLen:])
	} else {
		raw, err := base64URLDecode(publicKey)
		if err != nil {
			return false
		}
		pub = ed25519.PublicKey(raw)
	}

	return ed25519.Verify(pub, []byte(payload), sig)
}

// PublicKeyRawBase64URLFromPem 从 PEM 提取原始公钥字节并编码为 base64url。
// 对应 TS: publicKeyRawBase64UrlFromPem(publicKeyPem)
func PublicKeyRawBase64URLFromPem(publicKeyPem string) (string, error) {
	block, _ := pem.Decode([]byte(publicKeyPem))
	if block == nil {
		return "", fmt.Errorf("无法解析公钥 PEM")
	}
	der := block.Bytes
	prefixLen := len(ed25519SPKIPrefix)
	if len(der) < prefixLen+ed25519.PublicKeySize {
		return "", fmt.Errorf("公钥 DER 长度不足")
	}
	raw := der[prefixLen:]
	return base64URLEncode(raw), nil
}

// DeriveDeviceIDFromPublicKey 从公钥（PEM 或 base64url）派生设备 ID。
// 对应 TS: deriveDeviceIdFromPublicKey(publicKey)
func DeriveDeviceIDFromPublicKey(publicKey string) (string, error) {
	var raw []byte
	if strings.HasPrefix(strings.TrimSpace(publicKey), "-----") {
		block, _ := pem.Decode([]byte(publicKey))
		if block == nil {
			return "", fmt.Errorf("无法解析公钥 PEM")
		}
		der := block.Bytes
		prefixLen := len(ed25519SPKIPrefix)
		if len(der) < prefixLen+ed25519.PublicKeySize {
			return "", fmt.Errorf("公钥 DER 长度不足")
		}
		raw = der[prefixLen:]
	} else {
		var err error
		raw, err = base64URLDecode(publicKey)
		if err != nil {
			return "", fmt.Errorf("base64url 解码失败: %w", err)
		}
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ---------- base64url 辅助函数 ----------

// base64URLEncode 编码为不带填充的 base64url。
// 对应 TS: base64UrlEncode(buf)
func base64URLEncode(b []byte) string {
	s := base64.StdEncoding.EncodeToString(b)
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.TrimRight(s, "=")
	return s
}

// base64URLDecode 解码 base64url（带/不带填充均支持）。
// 对应 TS: base64UrlDecode(input)
func base64URLDecode(s string) ([]byte, error) {
	normalized := strings.ReplaceAll(s, "-", "+")
	normalized = strings.ReplaceAll(normalized, "_", "/")
	pad := (4 - len(normalized)%4) % 4
	normalized += strings.Repeat("=", pad)
	return base64.StdEncoding.DecodeString(normalized)
}

// ---------- 私钥解析 ----------

// parseED25519PrivateKeyFromPKCS8 从 PKCS8 DER 解析 ED25519 私钥。
// 找最后一个 04 20 前缀后的 32 字节作为种子。
func parseED25519PrivateKeyFromPKCS8(der []byte) (ed25519.PrivateKey, error) {
	for i := len(der) - ed25519.SeedSize - 2; i >= 0; i-- {
		if der[i] == 0x04 && der[i+1] == byte(ed25519.SeedSize) {
			seed := der[i+2 : i+2+ed25519.SeedSize]
			return ed25519.NewKeyFromSeed(seed), nil
		}
	}
	return nil, fmt.Errorf("未能从 PKCS8 DER 提取 ED25519 种子")
}
