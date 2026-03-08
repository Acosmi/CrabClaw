package mcpremote

// token_store_file.go — 基于文件系统的 OAuth Token 持久化。
// [FIX-01: P2A-X01 FileTokenStore 实现]
//
// 路径: <configDir>/auth/oauth_token.json
// 原子写: temp-file + rename (复用 ledger.go 模式)
// 权限: 目录 0o700, 文件 0o600 (token 是敏感数据)

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileTokenStore 基于 VFS 的 token 持久化。
type FileTokenStore struct {
	filePath string
	mu       sync.Mutex
}

// NewFileTokenStore 创建文件 token 存储。
// filePath 示例: ~/.openacosmi/auth/oauth_token.json
func NewFileTokenStore(filePath string) *FileTokenStore {
	return &FileTokenStore{filePath: filePath}
}

// Load 加载 token。文件不存在返回 nil（首次启动正常路径）。
func (s *FileTokenStore) Load() (*OAuthToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 首次启动，无 token
		}
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var token OAuthToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}

	return &token, nil
}

// Save 原子写入 token 到文件。
// 使用 temp-file + rename 确保写入原子性。
func (s *FileTokenStore) Save(token *OAuthToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return fmt.Errorf("write token tmp: %w", err)
	}
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename token: %w", err)
	}

	return nil
}

// Clear 删除 token 文件。
func (s *FileTokenStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := os.Remove(s.filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove token file: %w", err)
	}
	return nil
}
