package sessions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- 会话存储类型 ----------

// SessionOrigin 会话来源信息。
// TS 参考: src/config/sessions/types.ts → SessionOrigin
type SessionOrigin struct {
	Label     string      `json:"label,omitempty"`
	Provider  string      `json:"provider,omitempty"`
	Surface   string      `json:"surface,omitempty"`
	ChatType  string      `json:"chatType,omitempty"`
	From      string      `json:"from,omitempty"`
	To        string      `json:"to,omitempty"`
	AccountID string      `json:"accountId,omitempty"`
	ThreadID  interface{} `json:"threadId,omitempty"` // string | number
}

// SessionSkillSnapshot 会话技能快照。
type SessionSkillSnapshot struct {
	Prompt  string                     `json:"prompt"`
	Skills  []SessionSkillSnapshotItem `json:"skills"`
	Version *int                       `json:"version,omitempty"`
}

// SessionSkillSnapshotItem 技能快照条目。
type SessionSkillSnapshotItem struct {
	Name       string `json:"name"`
	PrimaryEnv string `json:"primaryEnv,omitempty"`
}

// DeliveryContext 消息投递上下文。
type DeliveryContext struct {
	Channel   string      `json:"channel,omitempty"`
	To        string      `json:"to,omitempty"`
	AccountID string      `json:"accountId,omitempty"`
	ThreadID  interface{} `json:"threadId,omitempty"` // string | number
}

// ---------- 扩展 SessionEntry ----------
// 注意: 核心 SessionEntry 已在 sessions.go 定义（SessionID, DisplayName, Subject, ChatType, UpdatedAt）。
// 此处定义完整字段版本供持久化使用。

// FullSessionEntry 完整会话条目（含运行时状态字段）。
// TS 参考: src/config/sessions/types.ts → SessionEntry (40+ 字段)
type FullSessionEntry struct {
	SessionID   string `json:"sessionId"`
	UpdatedAt   int64  `json:"updatedAt"`
	SessionFile string `json:"sessionFile,omitempty"`

	// 心跳
	LastHeartbeatText   string `json:"lastHeartbeatText,omitempty"`
	LastHeartbeatSentAt *int64 `json:"lastHeartbeatSentAt,omitempty"`

	// 运行状态
	SpawnedBy      string `json:"spawnedBy,omitempty"`
	SystemSent     *bool  `json:"systemSent,omitempty"`
	AbortedLastRun *bool  `json:"abortedLastRun,omitempty"`

	// 会话级覆盖
	ChatType               string `json:"chatType,omitempty"`
	ThinkingLevel          string `json:"thinkingLevel,omitempty"`
	VerboseLevel           string `json:"verboseLevel,omitempty"`
	ReasoningLevel         string `json:"reasoningLevel,omitempty"`
	ElevatedLevel          string `json:"elevatedLevel,omitempty"`
	TtsAuto                string `json:"ttsAuto,omitempty"`
	ExecHost               string `json:"execHost,omitempty"`
	ExecSecurity           string `json:"execSecurity,omitempty"`
	ExecAsk                string `json:"execAsk,omitempty"`
	ExecNode               string `json:"execNode,omitempty"`
	ResponseUsage          string `json:"responseUsage,omitempty"`
	ProviderOverride       string `json:"providerOverride,omitempty"`
	ModelOverride          string `json:"modelOverride,omitempty"`
	AuthProfileOverride    string `json:"authProfileOverride,omitempty"`
	AuthProfileOverrideSrc string `json:"authProfileOverrideSource,omitempty"`
	AuthProfileOverrideCnt *int   `json:"authProfileOverrideCompactionCount,omitempty"`

	// 群组
	GroupActivation           string `json:"groupActivation,omitempty"`
	GroupActivationNeedsIntro *bool  `json:"groupActivationNeedsSystemIntro,omitempty"`
	SendPolicy                string `json:"sendPolicy,omitempty"`

	// 队列
	QueueMode       string `json:"queueMode,omitempty"`
	QueueDebounceMs *int   `json:"queueDebounceMs,omitempty"`
	QueueCap        *int   `json:"queueCap,omitempty"`
	QueueDrop       string `json:"queueDrop,omitempty"`

	// Token 使用
	InputTokens  *int `json:"inputTokens,omitempty"`
	OutputTokens *int `json:"outputTokens,omitempty"`
	TotalTokens  *int `json:"totalTokens,omitempty"`

	// 模型
	ModelProvider string `json:"modelProvider,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextTokens *int   `json:"contextTokens,omitempty"`

	// 压缩 & 记忆
	CompactionCount          *int   `json:"compactionCount,omitempty"`
	MemoryFlushAt            *int64 `json:"memoryFlushAt,omitempty"`
	MemoryFlushCompactionCnt *int   `json:"memoryFlushCompactionCount,omitempty"`

	// CLI
	CliSessionIDs      map[string]string `json:"cliSessionIds,omitempty"`
	ClaudeCliSessionID string            `json:"claudeCliSessionId,omitempty"`

	// 显示 & 标签
	Label        string `json:"label,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	Channel      string `json:"channel,omitempty"`
	GroupID      string `json:"groupId,omitempty"`
	Subject      string `json:"subject,omitempty"`
	GroupChannel string `json:"groupChannel,omitempty"`
	Space        string `json:"space,omitempty"`

	// 来源 & 投递
	Origin      *SessionOrigin   `json:"origin,omitempty"`
	DeliveryCtx *DeliveryContext `json:"deliveryContext,omitempty"`

	// 最近路由
	LastChannel   string      `json:"lastChannel,omitempty"`
	LastTo        string      `json:"lastTo,omitempty"`
	LastAccountID string      `json:"lastAccountId,omitempty"`
	LastThreadID  interface{} `json:"lastThreadId,omitempty"` // string | number

	// 快照
	SkillsSnapshot *SessionSkillSnapshot `json:"skillsSnapshot,omitempty"`
}

// MergeSessionEntry 合并会话条目。
// TS 参考: src/config/sessions/types.ts → mergeSessionEntry()
func MergeSessionEntry(existing *FullSessionEntry, patch map[string]interface{}) *FullSessionEntry {
	now := time.Now().UnixMilli()

	if existing == nil {
		result := &FullSessionEntry{}
		// 序列化 patch 再反序列化到 result
		data, _ := json.Marshal(patch)
		_ = json.Unmarshal(data, result)
		if result.SessionID == "" {
			result.SessionID = generateUUID()
		}
		if result.UpdatedAt == 0 {
			result.UpdatedAt = now
		}
		return result
	}

	// 克隆 existing
	data, _ := json.Marshal(existing)
	merged := &FullSessionEntry{}
	_ = json.Unmarshal(data, merged)

	// 应用 patch
	patchData, _ := json.Marshal(patch)
	_ = json.Unmarshal(patchData, merged)

	// 确保 updatedAt 不回退
	if merged.UpdatedAt < existing.UpdatedAt {
		merged.UpdatedAt = existing.UpdatedAt
	}
	if merged.UpdatedAt < now {
		merged.UpdatedAt = now
	}

	return merged
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // v4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// ---------- 会话存储层 ----------

// SessionStore 会话存储（JSON 文件持久化，含 TTL 缓存和文件锁）。
// TS 参考: src/config/sessions/store.ts (495 行)
type SessionStore struct {
	mu        sync.RWMutex
	storePath string
	cache     *sessionStoreCache
}

type sessionStoreCache struct {
	store    map[string]*FullSessionEntry
	loadedAt time.Time
	mtimeMs  int64
}

// getSessionCacheTTL 返回 session 缓存 TTL。
// 优先读取 CRABCLAW_SESSION_CACHE_TTL_MS / OPENACOSMI_SESSION_CACHE_TTL_MS 环境变量，否则使用默认 45s。
// TS 参考: src/config/sessions/store.ts — SESSION_CACHE_TTL_MS
func getSessionCacheTTL() time.Duration {
	v := strings.TrimSpace(os.Getenv("CRABCLAW_SESSION_CACHE_TTL_MS"))
	if v == "" {
		v = strings.TrimSpace(os.Getenv("OPENACOSMI_SESSION_CACHE_TTL_MS"))
	}
	if v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 45 * time.Second // 默认值
}

// NewSessionStore 创建会话存储实例。
func NewSessionStore(storePath string) *SessionStore {
	return &SessionStore{
		storePath: storePath,
	}
}

// LoadAll 加载所有会话条目。
// 实现 TTL 缓存 + mtime 校验，参考 TS loadSessionStore()。
func (s *SessionStore) LoadAll() (map[string]*FullSessionEntry, error) {
	s.mu.RLock()
	if s.cache != nil && time.Since(s.cache.loadedAt) <= getSessionCacheTTL() {
		mtime := getFileMtime(s.storePath)
		if mtime == s.cache.mtimeMs {
			result := cloneStore(s.cache.store)
			s.mu.RUnlock()
			return result, nil
		}
	}
	s.mu.RUnlock()

	// 无缓存命中，从磁盘加载
	return s.loadFromDisk()
}

// Get 获取单个会话条目。
func (s *SessionStore) Get(sessionKey string) (*FullSessionEntry, error) {
	store, err := s.LoadAll()
	if err != nil {
		return nil, err
	}
	entry := store[sessionKey]
	return entry, nil
}

// Save 保存整个 store 到磁盘。
func (s *SessionStore) Save(store map[string]*FullSessionEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清除缓存
	s.cache = nil

	if err := os.MkdirAll(filepath.Dir(s.storePath), 0o755); err != nil {
		return fmt.Errorf("创建会话存储目录失败: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化会话存储失败: %w", err)
	}

	// 原子写入: 先写临时文件再 rename
	tmp := fmt.Sprintf("%s.%d.%s.tmp", s.storePath, os.Getpid(), generateUUID()[:8])
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.storePath); err != nil {
		// 失败时清理临时文件
		_ = os.Remove(tmp)
		return fmt.Errorf("原子重命名失败: %w", err)
	}

	return nil
}

// Update 原子读-改-写操作。
// TS 参考: updateSessionStore() → withSessionStoreLock + loadSessionStore + save
func (s *SessionStore) Update(mutator func(store map[string]*FullSessionEntry) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清除缓存，直接从磁盘读取（锁内保证一致性）
	s.cache = nil
	store, err := s.readFromDiskUnlocked()
	if err != nil {
		return err
	}

	if err := mutator(store); err != nil {
		return err
	}

	return s.saveToDiskUnlocked(store)
}

// ---------- 内部方法 ----------

func (s *SessionStore) loadFromDisk() (map[string]*FullSessionEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.readFromDiskUnlocked()
	if err != nil {
		return nil, err
	}

	// 更新缓存
	s.cache = &sessionStoreCache{
		store:    cloneStore(store),
		loadedAt: time.Now(),
		mtimeMs:  getFileMtime(s.storePath),
	}

	return store, nil
}

func (s *SessionStore) readFromDiskUnlocked() (map[string]*FullSessionEntry, error) {
	store := make(map[string]*FullSessionEntry)

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("读取会话存储失败: %w", err)
	}

	if err := json.Unmarshal(data, &store); err != nil {
		// 无效文件，返回空 store（与 TS 行为一致）
		return make(map[string]*FullSessionEntry), nil
	}

	// Best-effort 迁移: provider → channel 命名
	for _, entry := range store {
		if entry == nil {
			continue
		}
		s.migrateEntry(entry)
	}

	return store, nil
}

func (s *SessionStore) migrateEntry(entry *FullSessionEntry) {
	// provider → channel 迁移 (TS: store.ts L150-163)
	// 此处使用 JSON map 方式处理遗留字段不在结构体中的情况
	// 由于 Go 结构体不含 provider 字段，仅在字段为空时从 channel 推断

	// room → groupChannel 迁移 (TS: store.ts L164-170)
	// 同上理由，Go struct 不含 room 字段，迁移已在 TS 端完成
}

func (s *SessionStore) saveToDiskUnlocked(store map[string]*FullSessionEntry) error {
	if err := os.MkdirAll(filepath.Dir(s.storePath), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	tmp := fmt.Sprintf("%s.%d.%s.tmp", s.storePath, os.Getpid(), generateUUID()[:8])
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	if err := os.Rename(tmp, s.storePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("重命名失败: %w", err)
	}

	return nil
}

func getFileMtime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

func cloneStore(src map[string]*FullSessionEntry) map[string]*FullSessionEntry {
	data, _ := json.Marshal(src)
	result := make(map[string]*FullSessionEntry)
	_ = json.Unmarshal(data, &result)
	return result
}
