package infra

// device_auth_store.go — 设备认证令牌存储
// 对应 TS: src/infra/device-auth-store.ts
//
// 管理设备认证 token 的持久化存储（stateDir/identity/device-auth.json）。
// 使用 sync.RWMutex 保护内存缓存，原子写文件（临时文件 + rename）。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const deviceAuthFileName = "device-auth.json"

// DeviceAuthEntry 单条设备认证记录。
// 对应 TS: DeviceAuthEntry { token, role, scopes, updatedAtMs }
type DeviceAuthEntry struct {
	Token       string   `json:"token"`
	Role        string   `json:"role"`
	Scopes      []string `json:"scopes"`
	UpdatedAtMs int64    `json:"updatedAtMs"`
}

// deviceAuthStore 磁盘文件的 JSON 结构。
// 对应 TS: DeviceAuthStore { version, deviceId, tokens }
type deviceAuthStore struct {
	Version  int                         `json:"version"`
	DeviceID string                      `json:"deviceId"`
	Tokens   map[string]*DeviceAuthEntry `json:"tokens"`
}

// DeviceAuthStoreManager 提供线程安全的设备认证存储操作。
type DeviceAuthStoreManager struct {
	mu       sync.RWMutex
	filePath string
	cache    *deviceAuthStore
}

// NewDeviceAuthStoreManager 创建存储管理器实例。
// filePath 通常为 stateDir/identity/device-auth.json。
func NewDeviceAuthStoreManager(stateDir string) *DeviceAuthStoreManager {
	return &DeviceAuthStoreManager{
		filePath: filepath.Join(stateDir, "identity", deviceAuthFileName),
	}
}

// LoadDeviceAuthToken 读取指定 deviceId + role 的认证令牌。
// 对应 TS: loadDeviceAuthToken({ deviceId, role })
func (m *DeviceAuthStoreManager) LoadDeviceAuthToken(deviceID, role string) (*DeviceAuthEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	store, err := m.readStore()
	if err != nil || store == nil {
		return nil, nil
	}
	if store.DeviceID != deviceID {
		return nil, nil
	}
	normalizedRole := normalizeRole(role)
	entry := store.Tokens[normalizedRole]
	if entry == nil || entry.Token == "" {
		return nil, nil
	}
	return entry, nil
}

// StoreDeviceAuthToken 写入或更新认证令牌。
// 对应 TS: storeDeviceAuthToken({ deviceId, role, token, scopes? })
func (m *DeviceAuthStoreManager) StoreDeviceAuthToken(deviceID, role, token string, scopes []string) (*DeviceAuthEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, _ := m.readStore()
	normalizedRole := normalizeRole(role)

	next := &deviceAuthStore{
		Version:  1,
		DeviceID: deviceID,
		Tokens:   make(map[string]*DeviceAuthEntry),
	}
	// 保留同 deviceId 的旧 tokens
	if existing != nil && existing.DeviceID == deviceID && existing.Tokens != nil {
		for k, v := range existing.Tokens {
			next.Tokens[k] = v
		}
	}

	entry := &DeviceAuthEntry{
		Token:       token,
		Role:        normalizedRole,
		Scopes:      normalizeScopes(scopes),
		UpdatedAtMs: nowUnixMilliForAuth(),
	}
	next.Tokens[normalizedRole] = entry

	if err := m.writeStore(next); err != nil {
		return nil, err
	}
	// 更新缓存
	m.cache = next
	return entry, nil
}

// ClearDeviceAuthToken 删除指定 role 的认证令牌。
// 对应 TS: clearDeviceAuthToken({ deviceId, role })
func (m *DeviceAuthStoreManager) ClearDeviceAuthToken(deviceID, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	store, err := m.readStore()
	if err != nil || store == nil || store.DeviceID != deviceID {
		return nil
	}
	normalizedRole := normalizeRole(role)
	if _, ok := store.Tokens[normalizedRole]; !ok {
		return nil
	}

	next := &deviceAuthStore{
		Version:  1,
		DeviceID: store.DeviceID,
		Tokens:   make(map[string]*DeviceAuthEntry),
	}
	for k, v := range store.Tokens {
		if k != normalizedRole {
			next.Tokens[k] = v
		}
	}
	if err := m.writeStore(next); err != nil {
		return err
	}
	m.cache = next
	return nil
}

// ---------- 包级别便利函数（无状态，对应 TS 的模块函数） ----------

// LoadDeviceAuthTokenFromDir 从 stateDir 读取指定 deviceId + role 的认证令牌。
// 对应 TS: loadDeviceAuthToken({ deviceId, role })
func LoadDeviceAuthTokenFromDir(stateDir, deviceID, role string) (*DeviceAuthEntry, error) {
	m := NewDeviceAuthStoreManager(stateDir)
	return m.LoadDeviceAuthToken(deviceID, role)
}

// StoreDeviceAuthTokenToDir 向 stateDir 写入认证令牌。
// 对应 TS: storeDeviceAuthToken({ deviceId, role, token, scopes? })
func StoreDeviceAuthTokenToDir(stateDir, deviceID, role, token string, scopes []string) (*DeviceAuthEntry, error) {
	m := NewDeviceAuthStoreManager(stateDir)
	return m.StoreDeviceAuthToken(deviceID, role, token, scopes)
}

// ClearDeviceAuthTokenFromDir 从 stateDir 删除指定 role 的认证令牌。
// 对应 TS: clearDeviceAuthToken({ deviceId, role })
func ClearDeviceAuthTokenFromDir(stateDir, deviceID, role string) error {
	m := NewDeviceAuthStoreManager(stateDir)
	return m.ClearDeviceAuthToken(deviceID, role)
}

// ---------- 内部实现 ----------

// readStore 读取文件或使用缓存。调用方须持锁。
func (m *DeviceAuthStoreManager) readStore() (*deviceAuthStore, error) {
	if m.cache != nil {
		return m.cache, nil
	}
	store, err := readDeviceAuthFile(m.filePath)
	if err != nil {
		return nil, err
	}
	m.cache = store
	return store, nil
}

// writeStore 原子写文件（临时文件 + rename），并保持 0600 权限。
// 调用方须持写锁。
func (m *DeviceAuthStoreManager) writeStore(store *deviceAuthStore) error {
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建 identity 目录失败: %w", err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 device-auth.json 失败: %w", err)
	}
	data = append(data, '\n')

	tmp := m.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	// best-effort chmod
	_ = os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, m.filePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("原子替换 device-auth.json 失败: %w", err)
	}
	return nil
}

// readDeviceAuthFile 从磁盘读取 device-auth.json，文件不存在时返回 nil。
func readDeviceAuthFile(filePath string) (*deviceAuthStore, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 device-auth.json 失败: %w", err)
	}

	var store deviceAuthStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return nil, fmt.Errorf("解析 device-auth.json 失败: %w", err)
	}
	// 基本校验
	if store.Version != 1 || store.DeviceID == "" {
		return nil, nil
	}
	if store.Tokens == nil {
		return nil, nil
	}
	return &store, nil
}

// normalizeRole 规范化 role 字符串（trim 空白）。
// 对应 TS: normalizeRole
func normalizeRole(role string) string {
	return strings.TrimSpace(role)
}

// normalizeScopes 规范化 scopes 列表：trim、去重、排序。
// 对应 TS: normalizeScopes
func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{})
	var out []string
	for _, s := range scopes {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

// nowUnixMilliForAuth 返回当前时间的 Unix 毫秒时间戳（device_auth_store 内部使用）。
func nowUnixMilliForAuth() int64 {
	return timeNowUnixMilli()
}
