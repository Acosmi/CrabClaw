package models

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ManagedModelCatalog 从 nexus-v4 拉取托管模型目录，带缓存。
// 并发安全: RWMutex 保护缓存读写，HTTP 请求在锁外执行。
type ManagedModelCatalog struct {
	catalogURL    string
	tokenProvider func() (string, error)
	client        *http.Client

	mu       sync.RWMutex
	cache    []types.ManagedModelEntry
	cacheAt  time.Time
	cacheTTL time.Duration
}

// NewManagedModelCatalog 创建托管模型目录。
func NewManagedModelCatalog(catalogURL string, tokenProvider func() (string, error)) *ManagedModelCatalog {
	return &ManagedModelCatalog{
		catalogURL:    catalogURL,
		tokenProvider: tokenProvider,
		client:        &http.Client{Timeout: 15 * time.Second},
		cacheTTL:      5 * time.Minute,
	}
}

// List 返回托管模型列表（带缓存，缓存命中时直接返回）。
func (c *ManagedModelCatalog) List() ([]types.ManagedModelEntry, error) {
	c.mu.RLock()
	if len(c.cache) > 0 && time.Since(c.cacheAt) < c.cacheTTL {
		result := make([]types.ManagedModelEntry, len(c.cache))
		copy(result, c.cache)
		c.mu.RUnlock()
		return result, nil
	}
	c.mu.RUnlock()

	entries, err := c.fetchRemote()
	if err != nil {
		slog.Warn("managed_catalog: fetch failed, returning stale cache", "error", err)
		c.mu.RLock()
		if len(c.cache) > 0 {
			result := make([]types.ManagedModelEntry, len(c.cache))
			copy(result, c.cache)
			c.mu.RUnlock()
			return result, nil
		}
		c.mu.RUnlock()
		return nil, err
	}

	c.mu.Lock()
	c.cache = entries
	c.cacheAt = time.Now()
	c.mu.Unlock()

	result := make([]types.ManagedModelEntry, len(entries))
	copy(result, entries)
	return result, nil
}

// Refresh 强制刷新缓存。
func (c *ManagedModelCatalog) Refresh() error {
	entries, err := c.fetchRemote()
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.cache = entries
	c.cacheAt = time.Now()
	c.mu.Unlock()
	return nil
}

// TokenProvider 返回 token 提供函数（供钱包查询等共用）。
func (c *ManagedModelCatalog) TokenProvider() func() (string, error) {
	return c.tokenProvider
}

// DefaultModel 返回标记为 IsDefault 的托管模型（如有）。
func (c *ManagedModelCatalog) DefaultModel() *types.ManagedModelEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.cache {
		if c.cache[i].IsDefault {
			entry := c.cache[i]
			return &entry
		}
	}
	if len(c.cache) > 0 {
		entry := c.cache[0]
		return &entry
	}
	return nil
}

func (c *ManagedModelCatalog) fetchRemote() ([]types.ManagedModelEntry, error) {
	token, err := c.tokenProvider()
	if err != nil {
		return nil, fmt.Errorf("managed_catalog: get token: %w", err)
	}

	req, err := http.NewRequest("GET", c.catalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("managed_catalog: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("managed_catalog: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("managed_catalog: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := string(body)
		if len(msg) > 256 {
			msg = msg[:256] + "..."
		}
		return nil, fmt.Errorf("managed_catalog: HTTP %d: %s", resp.StatusCode, msg)
	}

	var wrapper struct {
		Models []types.ManagedModelEntry `json:"models"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Models) > 0 {
		return wrapper.Models, nil
	}

	var entries []types.ManagedModelEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("managed_catalog: parse response: %w", err)
	}
	return entries, nil
}
