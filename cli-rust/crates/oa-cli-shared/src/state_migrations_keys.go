package infra

// state_migrations_keys.go — session key 规范化与合并
// 对应 TS: state-migrations.ts L68-279

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/routing"
	"github.com/Acosmi/ClawAcosmi/internal/sessions"
)

// isSurfaceGroupKey 判断是否为 surface group key。
func isSurfaceGroupKey(key string) bool {
	return strings.Contains(key, ":group:") || strings.Contains(key, ":channel:")
}

// isLegacyGroupKey 判断是否为旧版 group key。
func isLegacyGroupKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "group:") {
		return true
	}
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "@g.us") {
		return false
	}
	if !strings.Contains(trimmed, ":") {
		return true
	}
	if strings.HasPrefix(lower, "whatsapp:") && !strings.Contains(trimmed, ":group:") {
		return true
	}
	return false
}

// canonicalizeSessionKeyForAgent 规范化 session key。
func canonicalizeSessionKeyForAgent(key, agentID, mainKey, scope string) string {
	agentID = routing.NormalizeAgentID(agentID)
	raw := strings.TrimSpace(key)
	if raw == "" {
		return raw
	}
	lower := strings.ToLower(raw)
	if lower == "global" || lower == "unknown" {
		return lower
	}

	// 尝试 main session alias 规范化 (TS L109-116)
	sessionCfg := &sessions.SessionScopeConfig{Scope: scope, MainKey: mainKey}
	canonicalMain := sessions.CanonicalizeMainSessionAlias(sessionCfg, agentID, raw)
	if canonicalMain != raw {
		return strings.ToLower(canonicalMain)
	}

	// agent: prefix — already canonical
	if strings.HasPrefix(lower, "agent:") {
		return lower
	}
	// subagent: prefix — prefix with agent
	if strings.HasPrefix(lower, "subagent:") {
		rest := raw[len("subagent:"):]
		return strings.ToLower("agent:" + agentID + ":subagent:" + rest)
	}
	// group: prefix
	if strings.HasPrefix(raw, "group:") {
		id := strings.TrimSpace(raw[len("group:"):])
		if id == "" {
			return raw
		}
		channel := "unknown"
		if strings.Contains(strings.ToLower(id), "@g.us") {
			channel = "whatsapp"
		}
		return strings.ToLower("agent:" + agentID + ":" + channel + ":group:" + id)
	}
	// bare WhatsApp JID
	if !strings.Contains(raw, ":") && strings.Contains(lower, "@g.us") {
		return strings.ToLower("agent:" + agentID + ":whatsapp:group:" + raw)
	}
	// whatsapp:xxx@g.us
	if strings.HasPrefix(lower, "whatsapp:") && strings.Contains(lower, "@g.us") {
		remainder := strings.TrimSpace(raw[len("whatsapp:"):])
		cleaned := remainder
		if strings.HasPrefix(strings.ToLower(cleaned), "group:") {
			cleaned = strings.TrimSpace(cleaned[len("group:"):])
		}
		if cleaned != "" && !isSurfaceGroupKey(raw) {
			return strings.ToLower("agent:" + agentID + ":whatsapp:group:" + cleaned)
		}
	}
	// surface group key
	if isSurfaceGroupKey(raw) {
		return strings.ToLower("agent:" + agentID + ":" + raw)
	}
	return strings.ToLower("agent:" + agentID + ":" + raw)
}
