package infra

// state_migrations_run.go — 执行遗留状态迁移
// 对应 TS: state-migrations.ts runLegacyStateMigrations (L872-885)
//   + migrateLegacyAgentDir (L788-832) + migrateLegacyWhatsAppAuth (L834-870)

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunLegacyStateMigrations 执行全部遗留迁移。
func RunLegacyStateMigrations(detected LegacyStateDetection) MigrationResult {
	now := func() int64 { return time.Now().UnixMilli() }
	sessions := migrateLegacySessions(detected, now)
	agentDir := migrateLegacyAgentDir(detected, now)
	wa := migrateLegacyWhatsAppAuth(detected)
	return MigrationResult{
		Changes:  append(append(sessions.Changes, agentDir.Changes...), wa.Changes...),
		Warnings: append(append(sessions.Warnings, agentDir.Warnings...), wa.Warnings...),
	}
}

// migrateLegacySessions 迁移旧版 session 文件。
func migrateLegacySessions(d LegacyStateDetection, now func() int64) MigrationResult {
	var r MigrationResult
	if !d.Sessions.HasLegacy {
		return r
	}
	ensureDir(d.Sessions.TargetDir)

	// 移动非 sessions.json 的文件
	for _, entry := range safeReadDir(d.Sessions.LegacyDir) {
		if entry.IsDir() || entry.Name() == "sessions.json" {
			continue
		}
		from := filepath.Join(d.Sessions.LegacyDir, entry.Name())
		to := filepath.Join(d.Sessions.TargetDir, entry.Name())
		if fileExistsMig(to) {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Failed moving %s: %v", from, err))
		} else {
			r.Changes = append(r.Changes, fmt.Sprintf("Moved %s → agents/%s/sessions", entry.Name(), d.TargetAgentID))
		}
	}

	removeDirIfEmpty(d.Sessions.LegacyDir)
	// 如果还有残留文件，备份旧目录
	leftover := safeReadDir(d.Sessions.LegacyDir)
	hasFiles := false
	for _, e := range leftover {
		if !e.IsDir() {
			hasFiles = true
			break
		}
	}
	if hasFiles {
		backup := fmt.Sprintf("%s.legacy-%d", d.Sessions.LegacyDir, now())
		if err := os.Rename(d.Sessions.LegacyDir, backup); err == nil {
			r.Warnings = append(r.Warnings, "Left legacy sessions at "+backup)
		}
	}
	return r
}

// migrateLegacyAgentDir 迁移旧版 agent 目录。
func migrateLegacyAgentDir(d LegacyStateDetection, now func() int64) MigrationResult {
	var r MigrationResult
	if !d.AgentDir.HasLegacy {
		return r
	}
	ensureDir(d.AgentDir.TargetDir)

	for _, entry := range safeReadDir(d.AgentDir.LegacyDir) {
		from := filepath.Join(d.AgentDir.LegacyDir, entry.Name())
		to := filepath.Join(d.AgentDir.TargetDir, entry.Name())
		if fileExistsMig(to) || existsDir(to) {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Failed moving %s: %v", from, err))
		} else {
			r.Changes = append(r.Changes,
				fmt.Sprintf("Moved agent file %s → agents/%s/agent", entry.Name(), d.TargetAgentID))
		}
	}

	removeDirIfEmpty(d.AgentDir.LegacyDir)
	if !emptyDirOrMissing(d.AgentDir.LegacyDir) {
		backup := filepath.Join(d.StateDir, "agents", d.TargetAgentID,
			fmt.Sprintf("agent.legacy-%d", now()))
		if err := os.Rename(d.AgentDir.LegacyDir, backup); err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Failed relocating legacy agent dir: %v", err))
		} else {
			r.Warnings = append(r.Warnings, "Left legacy agent dir at "+backup)
		}
	}
	return r
}
