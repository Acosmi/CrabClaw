package skills

// skill_store_client.go — nexus-v4 技能商店 REST API 客户端
//
// 端点:
//   GET  /api/v4/skill-store              — 浏览已审核技能
//   GET  /api/v4/skill-store/{id}         — 技能详情
//   GET  /api/v4/skill-store/{id}/download — 下载技能 ZIP

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SkillStoreClient 与 nexus-v4 技能商店 API 通信的 HTTP 客户端。
type SkillStoreClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// RemoteSkillItem nexus-v4 远程技能摘要。
type RemoteSkillItem struct {
	ID            string `json:"id"`
	Key           string `json:"key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Version       string `json:"version"`
	SecurityLevel string `json:"securityLevel"`
	SecurityScore int    `json:"securityScore"`
	DownloadCount int64  `json:"downloadCount"`
	Tags          string `json:"tags"`
	Author        string `json:"author"`
	Icon          string `json:"icon"`
	IsInstalled   bool   `json:"isInstalled"` // 客户端计算：本地是否已存在
}

// NewSkillStoreClient 创建技能商店客户端。
// baseURL 必须以 https:// 开头（安全要求：防止中间人窃取 token）。
// 本地开发环境允许 http://localhost 或 http://127.0.0.1。
func NewSkillStoreClient(baseURL, token string) *SkillStoreClient {
	trimmed := strings.TrimRight(baseURL, "/")
	return &SkillStoreClient{
		baseURL: trimmed,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Available 返回客户端是否已配置且安全可用。
// 强制校验 HTTPS（本地回环地址除外）。
func (c *SkillStoreClient) Available() bool {
	if c == nil || c.baseURL == "" || c.token == "" {
		return false
	}
	// 安全: 强制 HTTPS，仅允许本地回环使用 HTTP
	if strings.HasPrefix(c.baseURL, "https://") {
		return true
	}
	if strings.HasPrefix(c.baseURL, "http://localhost") || strings.HasPrefix(c.baseURL, "http://127.0.0.1") {
		return true
	}
	return false
}

// Browse 浏览远程技能商店。
// category 和 keyword 均为可选过滤参数。
func (c *SkillStoreClient) Browse(category, keyword string) ([]RemoteSkillItem, error) {
	if !c.Available() {
		return nil, fmt.Errorf("skill store client not configured")
	}

	params := url.Values{}
	if category != "" {
		params.Set("category", category)
	}
	if keyword != "" {
		params.Set("keyword", keyword)
	}

	endpoint := c.baseURL + "/api/v4/skill-store"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	body, err := c.doGet(endpoint)
	if err != nil {
		return nil, fmt.Errorf("browse skill store: %w", err)
	}

	// nexus-v4 返回格式: { "skills": [...] } 或直接 [...]
	// 先尝试 wrapped 格式
	var wrapped struct {
		Skills []RemoteSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Skills) > 0 {
		return wrapped.Skills, nil
	}

	// fallback: 直接数组
	var items []RemoteSkillItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse browse response: %w", err)
	}
	return items, nil
}

// Detail 获取单个技能详情。
func (c *SkillStoreClient) Detail(id string) (*RemoteSkillItem, error) {
	if !c.Available() {
		return nil, fmt.Errorf("skill store client not configured")
	}
	if id == "" {
		return nil, fmt.Errorf("skill id is required")
	}

	endpoint := c.baseURL + "/api/v4/skill-store/" + url.PathEscape(id)
	body, err := c.doGet(endpoint)
	if err != nil {
		return nil, fmt.Errorf("detail skill %s: %w", id, err)
	}

	var item RemoteSkillItem
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("parse detail response for %s: %w", id, err)
	}
	return &item, nil
}

// Download 下载技能 ZIP 包。
// 返回: ZIP 内容字节, 文件名, error。
func (c *SkillStoreClient) Download(id string) ([]byte, string, error) {
	if !c.Available() {
		return nil, "", fmt.Errorf("skill store client not configured")
	}
	if id == "" {
		return nil, "", fmt.Errorf("skill id is required")
	}

	endpoint := c.baseURL + "/api/v4/skill-store/" + url.PathEscape(id) + "/download"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download skill %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("download skill %s: HTTP %d: %s", id, resp.StatusCode, string(errBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read download body for %s: %w", id, err)
	}

	// 从 Content-Disposition 提取文件名
	filename := id + ".zip"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if idx := strings.Index(cd, "filename="); idx >= 0 {
			fn := cd[idx+len("filename="):]
			fn = strings.Trim(fn, "\"' ")
			if fn != "" {
				filename = fn
			}
		}
	}

	return data, filename, nil
}

// HealthCheck 连通性检查（HEAD 请求，5s 超时）。
// [FIX P1-L01: 添加 HealthCheck 方法]
func (c *SkillStoreClient) HealthCheck() error {
	if !c.Available() {
		return fmt.Errorf("skill store client not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "HEAD", c.baseURL, nil)
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	resp.Body.Close()
	return nil
}

// doGet 执行带认证的 GET 请求，返回响应体。
// [FIX P1-L02: 单次重试逻辑]
func (c *SkillStoreClient) doGet(endpoint string) ([]byte, error) {
	body, err := c.doGetOnce(endpoint)
	if err != nil {
		// 单次重试，间隔 1s
		time.Sleep(1 * time.Second)
		return c.doGetOnce(endpoint)
	}
	return body, nil
}

// doGetOnce 执行单次带认证的 GET 请求。
func (c *SkillStoreClient) doGetOnce(endpoint string) ([]byte, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBytes(body, 256))
	}

	return body, nil
}

// truncateBytes 截断 byte 切片用于错误消息。
func truncateBytes(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}
