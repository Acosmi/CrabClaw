package infra

// state_migrations_detect.go — 遗留状态检测
// 对应 TS: state-migrations.ts detectLegacyStateMigrations (L572-660)

import (
	"os"
	"path/filepath"

	"github.com/Acosmi/ClawAcosmi/internal/routing"
)

const (
	defaultAccountID = "default"
)

// DetectLegacyStateMigrations 检测需要迁移的遗留状态。
func DetectLegacyStateMigrations(stateDir, oauthDir, agentID, mainKey, scope string) LegacyStateDetection {
	agentID = routing.NormalizeAgentID(agentID)
	if mainKey == "" {
		mainKey = routing.DefaultMainKey
	}

	sessLegacyDir := filepath.Join(stateDir, "sessions")
	sessLegacyStore := filepath.Join(sessLegacyDir, "sessions.json")
	sessTargetDir := filepath.Join(stateDir, "agents", agentID, "sessions")
	sessTargetStore := filepath.Join(sessTargetDir, "sessions.json")

	// 检查旧版 session 目录
	hasLegacySessions := fileExistsMig(sessLegacyStore)
	if !hasLegacySessions {
		for _, e := range safeReadDir(sessLegacyDir) {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
				hasLegacySessions = true
				break
			}
		}
	}

	// 检查目标 store 中的旧版 key
	var legacyKeys []string
	if fileExistsMig(sessTargetStore) {
		legacyKeys = listLegacySessionKeys(sessTargetStore, agentID, mainKey, scope)
	}

	// 检查旧版 agent 目录
	legacyAgentDir := filepath.Join(stateDir, "agent")
	targetAgentDir := filepath.Join(stateDir, "agents", agentID, "agent")
	hasLegacyAgent := existsDir(legacyAgentDir)

	// 检查旧版 WhatsApp 认证
	targetWADir := filepath.Join(oauthDir, "whatsapp", defaultAccountID)
	hasLegacyWA := fileExistsMig(filepath.Join(oauthDir, "creds.json")) &&
		!fileExistsMig(filepath.Join(targetWADir, "creds.json"))

	// 构建预览
	var preview []string
	if hasLegacySessions {
		preview = append(preview, "- Sessions: "+sessLegacyDir+" → "+sessTargetDir)
	}
	if len(legacyKeys) > 0 {
		preview = append(preview, "- Sessions: canonicalize legacy keys in "+sessTargetStore)
	}
	if hasLegacyAgent {
		preview = append(preview, "- Agent dir: "+legacyAgentDir+" → "+targetAgentDir)
	}
	if hasLegacyWA {
		preview = append(preview, "- WhatsApp auth: "+oauthDir+" → "+targetWADir)
	}

	return LegacyStateDetection{
		TargetAgentID: agentID,
		TargetMainKey: mainKey,
		TargetScope:   scope,
		StateDir:      stateDir,
		OAuthDir:      oauthDir,
		Sessions: LegacySessionsDetection{
			LegacyDir:       sessLegacyDir,
			LegacyStorePath: sessLegacyStore,
			TargetDir:       sessTargetDir,
			TargetStorePath: sessTargetStore,
			HasLegacy:       hasLegacySessions || len(legacyKeys) > 0,
			LegacyKeys:      legacyKeys,
		},
		AgentDir: LegacyAgentDirDetection{
			LegacyDir: legacyAgentDir,
			TargetDir: targetAgentDir,
			HasLegacy: hasLegacyAgent,
		},
		WhatsAppAuth: LegacyWhatsAppDetection{
			LegacyDir: oauthDir,
			TargetDir: targetWADir,
			HasLegacy: hasLegacyWA,
		},
		Preview: preview,
	}
}

// listLegacySessionKeys 列出 store 中的旧版 key。
func listLegacySessionKeys(storePath, agentID, mainKey, scope string) []string {
	data, err := os.ReadFile(storePath)
	if err != nil {
		return nil
	}
	store := parseSessionStoreJSON(data)
	var legacy []string
	for key := range store {
		canonical := canonicalizeSessionKeyForAgent(key, agentID, mainKey, scope)
		if canonical != key {
			legacy = append(legacy, key)
		}
	}
	return legacy
}
