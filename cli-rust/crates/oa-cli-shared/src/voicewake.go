package infra

// voicewake.go — Voice Wake 配置管理
// 对应 TS: src/infra/voicewake.ts (91L)
// 完整实现：加载、保存、原子写入、默认触发词

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// VoiceWakeConfig 语音唤醒配置。
type VoiceWakeConfig struct {
	Triggers    []string `json:"triggers"`
	UpdatedAtMs int64    `json:"updatedAtMs"`
}

// 默认触发词
var DefaultVoiceWakeTriggers = []string{"openacosmi", "claude", "computer"}

var voiceWakeMu sync.Mutex

// resolveVoiceWakePath 解析配置文件路径。
func resolveVoiceWakePath(baseDir string) string {
	if baseDir == "" {
		baseDir = resolveStateDir()
	}
	return filepath.Join(baseDir, "settings", "voicewake.json")
}

// sanitizeTriggers 清理和验证触发词列表。
func sanitizeTriggers(triggers []string) []string {
	cleaned := make([]string, 0, len(triggers))
	for _, w := range triggers {
		w = strings.TrimSpace(w)
		if w != "" {
			cleaned = append(cleaned, w)
		}
	}
	if len(cleaned) == 0 {
		return append([]string{}, DefaultVoiceWakeTriggers...)
	}
	return cleaned
}

// LoadVoiceWakeConfig 加载语音唤醒配置。
// 对应 TS: loadVoiceWakeConfig()
func LoadVoiceWakeConfig(baseDir string) (*VoiceWakeConfig, error) {
	voiceWakeMu.Lock()
	defer voiceWakeMu.Unlock()

	filePath := resolveVoiceWakePath(baseDir)
	data, err := os.ReadFile(filePath)
	if err != nil {
		// 文件不存在时返回默认配置
		return &VoiceWakeConfig{
			Triggers:    append([]string{}, DefaultVoiceWakeTriggers...),
			UpdatedAtMs: 0,
		}, nil
	}

	var cfg VoiceWakeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &VoiceWakeConfig{
			Triggers:    append([]string{}, DefaultVoiceWakeTriggers...),
			UpdatedAtMs: 0,
		}, nil
	}

	cfg.Triggers = sanitizeTriggers(cfg.Triggers)
	if cfg.UpdatedAtMs < 0 {
		cfg.UpdatedAtMs = 0
	}
	return &cfg, nil
}

// SetVoiceWakeTriggers 设置语音唤醒触发词（原子写入）。
// 对应 TS: setVoiceWakeTriggers()
func SetVoiceWakeTriggers(triggers []string, baseDir string) (*VoiceWakeConfig, error) {
	voiceWakeMu.Lock()
	defer voiceWakeMu.Unlock()

	sanitized := sanitizeTriggers(triggers)
	filePath := resolveVoiceWakePath(baseDir)

	next := &VoiceWakeConfig{
		Triggers:    sanitized,
		UpdatedAtMs: time.Now().UnixMilli(),
	}

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serialize voicewake config: %w", err)
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create voicewake dir: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", filePath, os.Getpid())
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return nil, fmt.Errorf("write voicewake tmp: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("rename voicewake config: %w", err)
	}

	return next, nil
}

// resolveStateDir 获取状态目录。
func resolveStateDir() string {
	dir := os.Getenv("OPENACOSMI_STATE_DIR")
	if dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "openacosmi")
}
