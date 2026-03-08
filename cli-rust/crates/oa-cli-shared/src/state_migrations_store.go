package infra

// state_migrations_store.go — Session Store 读写 + 合并
// 对应 TS: state-migrations.ts L149-278 + state-migrations.fs.ts readSessionStoreJson5
// FIX-2: 补全 SaveSessionStore + JSON5 读取 + 合并逻辑

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/routing"

	"github.com/tailscale/hujson"
)

// SaveSessionStore 保存 session store 到文件。
func SaveSessionStore(path string, store map[string]SessionEntryLike) error {
	dir := path[:strings.LastIndex(path, "/")]
	ensureDir(dir)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ReadSessionStoreJSON5 使用 hujson 解析 JSON5 格式 session store。
func ReadSessionStoreJSON5(storePath string) (map[string]SessionEntryLike, bool) {
	data, err := os.ReadFile(storePath)
	if err != nil {
		return nil, false
	}
	// hujson: 标准化为 JSON
	standardized, err := hujson.Standardize(data)
	if err != nil {
		// 回退到纯 JSON
		result := parseSessionStoreJSON(data)
		return result, result != nil
	}
	result := parseSessionStoreJSON(standardized)
	return result, result != nil
}

// resolveUpdatedAt 提取 updatedAt 值。
func resolveUpdatedAt(entry SessionEntryLike) float64 {
	if entry.UpdatedAt != nil && math.IsInf(*entry.UpdatedAt, 0) == false {
		return *entry.UpdatedAt
	}
	return 0
}

// MergeSessionEntry 合并两个 session entry，保留较新的。
func MergeSessionEntry(existing, incoming SessionEntryLike, preferIncoming bool) SessionEntryLike {
	if existing.SessionID == "" {
		return incoming
	}
	eu := resolveUpdatedAt(existing)
	iu := resolveUpdatedAt(incoming)
	if iu > eu {
		return incoming
	}
	if iu < eu {
		return existing
	}
	if preferIncoming {
		return incoming
	}
	return existing
}

// NormalizeSessionEntry 标准化 session entry（room→groupChannel）。
func NormalizeSessionEntry(entry SessionEntryLike) *SessionEntryLike {
	if entry.SessionID == "" {
		return nil
	}
	if entry.GroupChannel == "" && entry.Room != "" {
		entry.GroupChannel = entry.Room
	}
	entry.Room = ""
	return &entry
}

// CanonicalizeSessionStore 规范化整个 store 的 key。
func CanonicalizeSessionStore(
	store map[string]SessionEntryLike,
	agentID, mainKey, scope string,
) (canonical map[string]SessionEntryLike, legacyKeys []string) {
	canonical = make(map[string]SessionEntryLike, len(store))
	type meta struct {
		isCanonical bool
		updatedAt   float64
	}
	metas := make(map[string]meta)

	for key, entry := range store {
		cKey := canonicalizeSessionKeyForAgent(key, agentID, mainKey, scope)
		isCan := cKey == key
		if !isCan {
			legacyKeys = append(legacyKeys, key)
		}
		if _, ok := canonical[cKey]; !ok {
			canonical[cKey] = entry
			metas[cKey] = meta{isCanonical: isCan, updatedAt: resolveUpdatedAt(entry)}
			continue
		}
		em := metas[cKey]
		iu := resolveUpdatedAt(entry)
		if iu > em.updatedAt {
			canonical[cKey] = entry
			metas[cKey] = meta{isCanonical: isCan, updatedAt: iu}
		} else if iu == em.updatedAt && !em.isCanonical && isCan {
			canonical[cKey] = entry
			metas[cKey] = meta{isCanonical: isCan, updatedAt: iu}
		}
	}
	return
}

// PickLatestLegacyDirectEntry 从 store 中选最新的非 agent/group 条目。
func PickLatestLegacyDirectEntry(store map[string]SessionEntryLike) *SessionEntryLike {
	var best *SessionEntryLike
	bestUpdated := float64(-1)
	for key, entry := range store {
		n := strings.TrimSpace(key)
		if n == "" || n == "global" {
			continue
		}
		if strings.HasPrefix(n, "agent:") || strings.HasPrefix(strings.ToLower(n), "subagent:") {
			continue
		}
		if isLegacyGroupKey(n) || isSurfaceGroupKey(n) {
			continue
		}
		u := resolveUpdatedAt(entry)
		if u > bestUpdated {
			bestUpdated = u
			e := entry
			best = &e
		}
	}
	return best
}

// BuildAgentMainSessionKey 构建 agent main session key（委托 routing 包）。
func BuildAgentMainSessionKey(agentID, mainKey string) string {
	return routing.BuildAgentMainSessionKey(agentID, mainKey)
}
