package infra

// state_migrations_fs.go — 状态迁移 FS 辅助函数
// 对应 TS: src/infra/state-migrations.fs.ts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// parseSessionStoreJSON 解析 session store JSON 文件。
func parseSessionStoreJSON(data []byte) map[string]SessionEntryLike {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	result := make(map[string]SessionEntryLike, len(raw))
	for k, v := range raw {
		var entry SessionEntryLike
		if err := json.Unmarshal(v, &entry); err == nil {
			result[k] = entry
		}
	}
	return result
}

// safeReadDir 安全读取目录，失败返回空切片。
func safeReadDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return entries
}

// existsDir 判断路径是否存在且为目录。
func existsDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ensureDir 确保目录存在。
func ensureDir(dir string) {
	_ = os.MkdirAll(dir, 0o755)
}

// fileExistsMig 判断路径是否存在且为文件（避免与其他包冲突）。
func fileExistsMig(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

var legacyWhatsAppAuthPattern = regexp.MustCompile(
	`^(app-state-sync|session|sender-key|pre-key)-`,
)

// isLegacyWhatsAppAuthFile 判断是否为旧版 WhatsApp 认证文件。
func isLegacyWhatsAppAuthFile(name string) bool {
	if name == "creds.json" || name == "creds.json.bak" {
		return true
	}
	if !strings.HasSuffix(name, ".json") {
		return false
	}
	return legacyWhatsAppAuthPattern.MatchString(name)
}

// emptyDirOrMissing 目录为空或不存在。
func emptyDirOrMissing(dir string) bool {
	if !existsDir(dir) {
		return true
	}
	return len(safeReadDir(dir)) == 0
}

// removeDirIfEmpty 如果目录为空则删除。
func removeDirIfEmpty(dir string) {
	if !emptyDirOrMissing(dir) {
		return
	}
	_ = os.Remove(dir)
}

// resolveSymlinkTarget 解析符号链接目标。
func resolveSymlinkTarget(linkPath string) string {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return ""
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(linkPath), target)
}
