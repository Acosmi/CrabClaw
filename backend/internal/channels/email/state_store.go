package email

// state_store.go — Email 账号持久化状态（UID 游标 / 运行时统计）
// 路径: <storeRoot>/email/<account-id>/state.json

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sanitizeAccountID 校验并清理 accountID，防止路径穿越（修 L4）。
// 仅允许字母、数字、连字符、下划线、点号。拒绝空值和含 "/" 或 ".." 的值。
func sanitizeAccountID(accountID string) (string, error) {
	if accountID == "" {
		return "", fmt.Errorf("email: accountID is empty")
	}
	// 拒绝路径穿越
	if strings.Contains(accountID, "/") || strings.Contains(accountID, "\\") || strings.Contains(accountID, "..") {
		return "", fmt.Errorf("email: accountID contains invalid path characters: %q", accountID)
	}
	// filepath.Clean 去掉多余的 . 等
	cleaned := filepath.Clean(accountID)
	if cleaned != accountID {
		return "", fmt.Errorf("email: accountID contains path-special characters: %q", accountID)
	}
	return cleaned, nil
}

// AccountState 持久化的邮箱账号状态
type AccountState struct {
	UIDValidity         uint32    `json:"uidValidity"`
	LastSeenUID         uint32    `json:"lastSeenUID"`
	LastProcessedMsgID  string    `json:"lastProcessedMessageId,omitempty"`
	LastSuccessAt       time.Time `json:"lastSuccessAt,omitempty"`
	LastErrorAt         time.Time `json:"lastErrorAt,omitempty"`
	ConsecutiveFailures int       `json:"consecutiveFailures"`
	CurrentMode         string    `json:"currentMode,omitempty"`
	LastIdleResetAt     time.Time `json:"lastIdleResetAt,omitempty"`
}

// StateStore 邮箱账号状态文件存储
type StateStore struct {
	filePath string
}

// NewStateStore 创建状态存储。
// storeRoot 为全局存储根目录（如 ~/.openacosmi/store），
// accountID 为账号标识。最终路径: <storeRoot>/email/<accountID>/state.json
func NewStateStore(storeRoot, accountID string) *StateStore {
	safe, err := sanitizeAccountID(accountID)
	if err != nil {
		slog.Warn("email: invalid accountID for state store", "accountID", accountID, "error", err)
		return &StateStore{} // filePath 为空，Load/Save 将返回错误
	}
	return &StateStore{
		filePath: filepath.Join(storeRoot, "email", safe, "state.json"),
	}
}

// Load 从磁盘加载状态。文件不存在返回 nil, nil（首次运行）。
func (s *StateStore) Load() (*AccountState, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}
	var state AccountState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

// Save 将状态写入磁盘（原子写入: 先写临时文件再 rename）。
func (s *StateStore) Save(state *AccountState) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmp, s.filePath); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// FilePath 返回状态文件路径（用于调试 / 日志）
func (s *StateStore) FilePath() string {
	return s.filePath
}

// --- ThreadContextStore: 线程上下文持久化（修 D-01） ---
// 路径: <storeRoot>/email/<accountID>/threads/<sessionKeyHash>.json

// ThreadContextStore 线程上下文存储
type ThreadContextStore struct {
	dir string
}

// NewThreadContextStore 创建线程上下文存储
func NewThreadContextStore(storeRoot, accountID string) *ThreadContextStore {
	safe, err := sanitizeAccountID(accountID)
	if err != nil {
		slog.Warn("email: invalid accountID for thread context store", "accountID", accountID, "error", err)
		return &ThreadContextStore{} // dir 为空，Load/Save 将返回错误
	}
	return &ThreadContextStore{
		dir: filepath.Join(storeRoot, "email", safe, "threads"),
	}
}

// Load 加载线程上下文
func (s *ThreadContextStore) Load(sessionKey string) (*ThreadContext, error) {
	path := s.filePath(sessionKey)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ctx ThreadContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}

// Save 保存线程上下文（原子写入）
func (s *ThreadContextStore) Save(sessionKey string, ctx *ThreadContext) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return err
	}
	path := s.filePath(sessionKey)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// filePath 生成文件路径（sessionKey SHA-256 hash 作文件名，避免冲突）
func (s *ThreadContextStore) filePath(sessionKey string) string {
	h := sha256.Sum256([]byte(sessionKey))
	return filepath.Join(s.dir, hex.EncodeToString(h[:16])+".json")
}

// --- DedupCache: 磁盘去重缓存（修 F-06） ---

// DedupEntry 去重缓存条目
type DedupEntry struct {
	SeenAt time.Time `json:"seenAt"`
}

// DedupCache 邮件去重缓存（磁盘持久化 + 7 天 TTL）
// 路径: <storeRoot>/email/<accountID>/seen_ids.json
type DedupCache struct {
	filePath string
	ttl      time.Duration
	entries  map[string]DedupEntry
}

// NewDedupCache 创建去重缓存
func NewDedupCache(storeRoot, accountID string, ttl time.Duration) *DedupCache {
	safe, err := sanitizeAccountID(accountID)
	if err != nil {
		slog.Warn("email: invalid accountID for dedup cache", "accountID", accountID, "error", err)
		safe = "invalid"
	}
	dc := &DedupCache{
		filePath: filepath.Join(storeRoot, "email", safe, "seen_ids.json"),
		ttl:      ttl,
		entries:  make(map[string]DedupEntry),
	}
	dc.loadFromDisk()
	return dc
}

// HasSeen 检查是否已见过
func (dc *DedupCache) HasSeen(key string) bool {
	entry, ok := dc.entries[key]
	if !ok {
		return false
	}
	// 检查 TTL
	if time.Since(entry.SeenAt) > dc.ttl {
		delete(dc.entries, key)
		return false
	}
	return true
}

// MarkSeen 标记为已见
func (dc *DedupCache) MarkSeen(key string) {
	dc.entries[key] = DedupEntry{SeenAt: time.Now()}
	dc.saveToDisk()
}

// Cleanup 清理过期条目
func (dc *DedupCache) Cleanup() int {
	removed := 0
	for key, entry := range dc.entries {
		if time.Since(entry.SeenAt) > dc.ttl {
			delete(dc.entries, key)
			removed++
		}
	}
	if removed > 0 {
		dc.saveToDisk()
	}
	return removed
}

// Len 返回缓存条目数
func (dc *DedupCache) Len() int {
	return len(dc.entries)
}

// loadFromDisk 从磁盘加载
func (dc *DedupCache) loadFromDisk() {
	data, err := os.ReadFile(dc.filePath)
	if err != nil {
		return // 文件不存在或读取失败，使用空缓存
	}
	var entries map[string]DedupEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	dc.entries = entries
	// 加载时清理过期条目
	dc.Cleanup()
}

// saveToDisk 保存到磁盘（原子写入）
func (dc *DedupCache) saveToDisk() {
	dir := filepath.Dir(dc.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.Warn("email: dedup cache mkdir failed", "path", dir, "error", err)
		return
	}
	data, err := json.Marshal(dc.entries)
	if err != nil {
		slog.Warn("email: dedup cache marshal failed", "error", err)
		return
	}
	tmp := dc.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		slog.Warn("email: dedup cache write failed", "path", tmp, "error", err)
		return
	}
	if err := os.Rename(tmp, dc.filePath); err != nil {
		slog.Warn("email: dedup cache rename failed", "error", err)
	}
}
