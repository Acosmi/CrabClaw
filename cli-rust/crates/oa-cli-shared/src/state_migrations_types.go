package infra

// state_migrations_types.go — 状态迁移类型定义
// 对应 TS: src/infra/state-migrations.ts 类型部分

// LegacyStateDetection 遗留状态检测结果。
type LegacyStateDetection struct {
	TargetAgentID string                  `json:"targetAgentId"`
	TargetMainKey string                  `json:"targetMainKey"`
	TargetScope   string                  `json:"targetScope,omitempty"`
	StateDir      string                  `json:"stateDir"`
	OAuthDir      string                  `json:"oauthDir"`
	Sessions      LegacySessionsDetection `json:"sessions"`
	AgentDir      LegacyAgentDirDetection `json:"agentDir"`
	WhatsAppAuth  LegacyWhatsAppDetection `json:"whatsappAuth"`
	Preview       []string                `json:"preview"`
}

// LegacySessionsDetection session 迁移检测。
type LegacySessionsDetection struct {
	LegacyDir       string   `json:"legacyDir"`
	LegacyStorePath string   `json:"legacyStorePath"`
	TargetDir       string   `json:"targetDir"`
	TargetStorePath string   `json:"targetStorePath"`
	HasLegacy       bool     `json:"hasLegacy"`
	LegacyKeys      []string `json:"legacyKeys"`
}

// LegacyAgentDirDetection agent 目录迁移检测。
type LegacyAgentDirDetection struct {
	LegacyDir string `json:"legacyDir"`
	TargetDir string `json:"targetDir"`
	HasLegacy bool   `json:"hasLegacy"`
}

// LegacyWhatsAppDetection WhatsApp 认证迁移检测。
type LegacyWhatsAppDetection struct {
	LegacyDir string `json:"legacyDir"`
	TargetDir string `json:"targetDir"`
	HasLegacy bool   `json:"hasLegacy"`
}

// StateDirMigrationResult 状态目录迁移结果。
type StateDirMigrationResult struct {
	Migrated bool     `json:"migrated"`
	Skipped  bool     `json:"skipped"`
	Changes  []string `json:"changes"`
	Warnings []string `json:"warnings"`
}

// MigrationResult 通用迁移结果。
type MigrationResult struct {
	Changes  []string
	Warnings []string
}

// SessionEntryLike 简化的 session entry（用于迁移）。
type SessionEntryLike struct {
	SessionID    string                 `json:"sessionId,omitempty"`
	UpdatedAt    *float64               `json:"updatedAt,omitempty"`
	GroupChannel string                 `json:"groupChannel,omitempty"`
	Room         string                 `json:"room,omitempty"`
	Extra        map[string]interface{} `json:"-"`
}

// MigrationLogger 迁移日志接口。
type MigrationLogger interface {
	Info(message string)
	Warn(message string)
}
