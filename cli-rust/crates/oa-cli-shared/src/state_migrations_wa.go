package infra

// state_migrations_wa.go — WhatsApp 认证迁移
// 对应 TS: state-migrations.ts migrateLegacyWhatsAppAuth (L834-870)

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrateLegacyWhatsAppAuth 迁移旧版 WhatsApp 认证文件。
func migrateLegacyWhatsAppAuth(d LegacyStateDetection) MigrationResult {
	var r MigrationResult
	if !d.WhatsAppAuth.HasLegacy {
		return r
	}
	ensureDir(d.WhatsAppAuth.TargetDir)

	for _, entry := range safeReadDir(d.WhatsAppAuth.LegacyDir) {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == "oauth.json" {
			continue
		}
		if !isLegacyWhatsAppAuthFile(entry.Name()) {
			continue
		}
		from := filepath.Join(d.WhatsAppAuth.LegacyDir, entry.Name())
		to := filepath.Join(d.WhatsAppAuth.TargetDir, entry.Name())
		if fileExistsMig(to) {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Failed moving %s: %v", from, err))
		} else {
			r.Changes = append(r.Changes, fmt.Sprintf("Moved WhatsApp auth %s → whatsapp/default", entry.Name()))
		}
	}
	return r
}
