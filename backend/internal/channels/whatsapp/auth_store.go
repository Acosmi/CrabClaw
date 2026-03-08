package whatsapp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/utils"
)

// WhatsApp 认证存储 — 继承自 src/web/auth-store.ts (202L)

// ResolveDefaultWebAuthDir 获取默认 Web 认证目录
func ResolveDefaultWebAuthDir() string {
	return filepath.Join(config.ResolveOAuthDir(), "whatsapp", defaultAccountID)
}

// WAWebAuthDir 默认 Web 认证目录（模块级常量，等价 TS export const WA_WEB_AUTH_DIR）
var WAWebAuthDir = ResolveDefaultWebAuthDir()

// ResolveWebCredsPath 获取 creds.json 路径
func ResolveWebCredsPath(authDir string) string {
	return filepath.Join(authDir, "creds.json")
}

// ResolveWebCredsBackupPath 获取 creds.json.bak 路径
func ResolveWebCredsBackupPath(authDir string) string {
	return filepath.Join(authDir, "creds.json.bak")
}

// readCredsJsonRaw 读取凭证文件原始内容
func readCredsJsonRaw(filePath string) string {
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 1 {
		return ""
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// MaybeRestoreCredsFromBackup 如果 creds.json 损坏则从备份恢复
func MaybeRestoreCredsFromBackup(authDir string) {
	credsPath := ResolveWebCredsPath(authDir)
	backupPath := ResolveWebCredsBackupPath(authDir)

	// 检查 creds.json 是否有效
	raw := readCredsJsonRaw(credsPath)
	if raw != "" {
		var test interface{}
		if json.Unmarshal([]byte(raw), &test) == nil {
			return // creds.json 有效，无需恢复
		}
	}

	// 尝试用备份恢复
	backupRaw := readCredsJsonRaw(backupPath)
	if backupRaw == "" {
		return
	}
	var test interface{}
	if json.Unmarshal([]byte(backupRaw), &test) != nil {
		return // 备份也无效
	}
	// 从备份复制
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return
	}
	if err := os.WriteFile(credsPath, data, 0o644); err == nil {
		slog.Warn("whatsapp: restored corrupted creds.json from backup",
			slog.String("path", credsPath),
		)
	}
}

// WebAuthExists 检查 Web 认证是否存在且有效
func WebAuthExists(authDir string) bool {
	resolved := resolveUserPath(authDir)
	MaybeRestoreCredsFromBackup(resolved)
	credsPath := ResolveWebCredsPath(resolved)

	info, err := os.Stat(credsPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 1 {
		return false
	}
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return false
	}
	var test interface{}
	return json.Unmarshal(data, &test) == nil
}

// WebSelfID 缓存的 WhatsApp Web 身份信息
type WebSelfID struct {
	E164 string
	JID  string
}

// ReadWebSelfId 从 creds.json 读取缓存的 WhatsApp Web 身份
func ReadWebSelfId(authDir string) WebSelfID {
	resolved := resolveUserPath(authDir)
	credsPath := ResolveWebCredsPath(resolved)

	data, err := os.ReadFile(credsPath)
	if err != nil {
		return WebSelfID{}
	}

	var parsed struct {
		Me *struct {
			ID string `json:"id"`
		} `json:"me"`
	}
	if json.Unmarshal(data, &parsed) != nil {
		return WebSelfID{}
	}

	jid := ""
	if parsed.Me != nil {
		jid = parsed.Me.ID
	}
	if jid == "" {
		return WebSelfID{}
	}

	// JID → E164
	phone := extractUserJidPhone(jid)
	e164 := ""
	if phone != "" {
		e164 = utils.NormalizeE164(phone)
	}

	return WebSelfID{E164: e164, JID: jid}
}

// GetWebAuthAgeMs 获取 Web 认证文件年龄（毫秒），不存在时返回 -1
func GetWebAuthAgeMs(authDir string) int64 {
	resolved := resolveUserPath(authDir)
	credsPath := ResolveWebCredsPath(resolved)
	info, err := os.Stat(credsPath)
	if err != nil {
		return -1
	}
	return time.Since(info.ModTime()).Milliseconds()
}

// LogoutWeb 清除 WhatsApp Web 凭证
func LogoutWeb(authDir string, isLegacyAuthDir bool) (bool, error) {
	resolved := resolveUserPath(authDir)
	if !WebAuthExists(resolved) {
		slog.Info("whatsapp: no WhatsApp Web session found; nothing to delete")
		return false, nil
	}
	if isLegacyAuthDir {
		ok, err := clearLegacyBaileysAuthState(resolved)
		if ok {
			slog.Info("whatsapp: cleared WhatsApp Web credentials (legacy)")
		}
		return ok, err
	}
	err := os.RemoveAll(resolved)
	if err == nil {
		slog.Info("whatsapp: cleared WhatsApp Web credentials",
			slog.String("authDir", resolved),
		)
	}
	return err == nil, err
}

// clearLegacyBaileysAuthState 清除旧版 Baileys 认证文件
func clearLegacyBaileysAuthState(authDir string) (bool, error) {
	entries, err := os.ReadDir(authDir)
	if err != nil {
		return false, err
	}
	deleted := false
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if !shouldDeleteLegacyFile(name) {
			continue
		}
		if err := os.Remove(filepath.Join(authDir, name)); err == nil {
			deleted = true
		}
	}
	return deleted, nil
}

// shouldDeleteLegacyFile 判断是否应删除旧版认证文件
func shouldDeleteLegacyFile(name string) bool {
	if name == "oauth.json" {
		return false
	}
	if name == "creds.json" || name == "creds.json.bak" {
		return true
	}
	if !strings.HasSuffix(name, ".json") {
		return false
	}
	prefixes := []string{"app-state-sync-", "session-", "sender-key-", "pre-key-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// LogWebSelfId 输出当前已关联的 WhatsApp Web 身份信息（CLI 友好显示）
// 等价 TS auth-store.ts:177 logWebSelfId()
func LogWebSelfId(authDir string, includeChannelPrefix bool) {
	if authDir == "" {
		authDir = WAWebAuthDir
	}
	self := ReadWebSelfId(authDir)
	details := "unknown"
	if self.E164 != "" || self.JID != "" {
		if self.E164 != "" {
			details = self.E164
		} else {
			details = "unknown"
		}
		if self.JID != "" {
			details += fmt.Sprintf(" (jid %s)", self.JID)
		}
	}
	slog.Info("whatsapp: web self id",
		slog.String("details", details),
		slog.Bool("channelPrefix", includeChannelPrefix),
	)
}

// PickWebChannel 选择 Web 频道，验证认证状态
// 等价 TS auth-store.ts:189 pickWebChannel()
func PickWebChannel(pref string, authDir string) (string, error) {
	if authDir == "" {
		authDir = WAWebAuthDir
	}
	choice := pref
	if choice == "auto" {
		choice = "web"
	}
	if !WebAuthExists(authDir) {
		return "", fmt.Errorf("no WhatsApp Web session found; run `crabclaw channels login --channel whatsapp --verbose` to link")
	}
	return choice, nil
}
