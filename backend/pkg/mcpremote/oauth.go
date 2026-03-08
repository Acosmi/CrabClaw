package mcpremote

// oauth.go — OAuth 2.1 Token Manager（精简版）
//
// 支持两种认证模式:
//   1. P1 JWT Token 兼容: 直接使用 skills.store.token 作为 Bearer token
//   2. OAuth 2.1 + PKCE: 完整 RFC 9728/8414 发现 → 授权码 → token 交换
//
// 初期用模式 1，OAuth 2.1 流程预留但尚未集成浏览器授权回调。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ---------- 配置 ----------

// OAuthConfig OAuth 客户端配置。
type OAuthConfig struct {
	// P1 兼容模式: 直接 JWT token
	StaticToken string

	// OAuth 2.1 模式（OAuthEnabled=true 时使用）
	OAuthEnabled bool
	IssuerURL    string // OAuth AS issuer URL
	ClientID     string // 动态注册后获得
	ClientSecret string // 可选
	RedirectURI  string // 授权回调
	Scopes       []string
}

// ---------- Token ----------

// OAuthToken OAuth token 结构。
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"-"` // 计算字段
}

// IsExpired 检查 token 是否过期（预留 30s 缓冲）。
func (t *OAuthToken) IsExpired() bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	if t.ExpiresAt.IsZero() {
		return false // 无过期时间 = 不过期 (P1 JWT)
	}
	return time.Now().After(t.ExpiresAt.Add(-30 * time.Second))
}

// ---------- Token Store ----------

// TokenStore token 持久化接口。
type TokenStore interface {
	Load() (*OAuthToken, error)
	Save(token *OAuthToken) error
	Clear() error
}

// MemoryTokenStore 内存 token 存储（进程级）。
type MemoryTokenStore struct {
	mu    sync.Mutex
	token *OAuthToken
}

// Load 加载 token。
func (s *MemoryTokenStore) Load() (*OAuthToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token, nil
}

// Save 保存 token。
func (s *MemoryTokenStore) Save(token *OAuthToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	return nil
}

// Clear 清除 token。
func (s *MemoryTokenStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = nil
	return nil
}

// ---------- Token Manager ----------

// OAuthTokenManager 管理 OAuth token 的获取、刷新和缓存。
// 并发安全: 使用 RWMutex double-checked locking，HTTP 请求在锁外执行。
type OAuthTokenManager struct {
	cfg    OAuthConfig
	store  TokenStore
	client *http.Client

	mu            sync.RWMutex
	tokenEndpoint string
	authEndpoint  string
	discoveryDone bool

	// refreshMu 保护刷新操作串行化（避免并发 HTTP refresh）
	refreshMu sync.Mutex
}

// NewOAuthTokenManager 创建 Token Manager。
func NewOAuthTokenManager(cfg OAuthConfig, store TokenStore) *OAuthTokenManager {
	if store == nil {
		store = &MemoryTokenStore{}
	}
	return &OAuthTokenManager{
		cfg:   cfg,
		store: store,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Store 返回底层 token store（用于外部 Clear 等操作）。
func (m *OAuthTokenManager) Store() TokenStore {
	return m.store
}

// GetAccessToken 获取有效的 access token。
// P1 模式: 直接返回静态 JWT token。
// OAuth 模式: RWMutex double-checked locking，HTTP 刷新在锁外执行，
// refreshMu 串行化并发刷新避免重复 HTTP 请求。
func (m *OAuthTokenManager) GetAccessToken() (string, error) {
	// P1 兼容模式: 使用静态 token（无锁快路径）
	if !m.cfg.OAuthEnabled && m.cfg.StaticToken != "" {
		return m.cfg.StaticToken, nil
	}

	// OAuth 2.1 模式 — 快路径: RLock 检查缓存 token
	m.mu.RLock()
	token, err := m.store.Load()
	if err != nil {
		m.mu.RUnlock()
		return "", fmt.Errorf("oauth: load token: %w", err)
	}
	if token != nil && !token.IsExpired() {
		accessToken := token.AccessToken
		m.mu.RUnlock()
		return accessToken, nil
	}
	refreshToken := ""
	if token != nil {
		refreshToken = token.RefreshToken
	}
	m.mu.RUnlock()

	// 慢路径: 需要刷新 — refreshMu 串行化并发刷新
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	// Double-check: 另一个 goroutine 可能已完成刷新
	m.mu.RLock()
	token, err = m.store.Load()
	if err != nil {
		m.mu.RUnlock()
		return "", fmt.Errorf("oauth: load token: %w", err)
	}
	if token != nil && !token.IsExpired() {
		accessToken := token.AccessToken
		m.mu.RUnlock()
		return accessToken, nil
	}
	m.mu.RUnlock()

	// 执行 HTTP 刷新（无锁）
	if refreshToken != "" {
		refreshed, err := m.refreshToken(refreshToken)
		if err == nil {
			// 写锁存储新 token
			m.mu.Lock()
			if saveErr := m.store.Save(refreshed); saveErr != nil {
				m.mu.Unlock()
				return "", fmt.Errorf("oauth: save refreshed token: %w", saveErr)
			}
			m.mu.Unlock()
			return refreshed.AccessToken, nil
		}
		// refresh 失败，清除旧 token
		m.mu.Lock()
		m.store.Clear()
		m.mu.Unlock()
	}

	// 无有效 token 且无法 refresh — 需要用户授权
	return "", fmt.Errorf("oauth: no valid token, authorization required")
}

// refreshToken 使用 refresh_token 获取新 access_token。
func (m *OAuthTokenManager) refreshToken(refreshToken string) (*OAuthToken, error) {
	if err := m.ensureDiscovery(); err != nil {
		return nil, err
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {m.cfg.ClientID},
	}
	if m.cfg.ClientSecret != "" {
		data.Set("client_secret", m.cfg.ClientSecret)
	}

	return m.doTokenRequest(data)
}

// ExchangeCode 用授权码交换 token（OAuth 2.1 + PKCE）。
func (m *OAuthTokenManager) ExchangeCode(code, codeVerifier string) (*OAuthToken, error) {
	if err := m.ensureDiscovery(); err != nil {
		return nil, err
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {m.cfg.RedirectURI},
		"client_id":     {m.cfg.ClientID},
		"code_verifier": {codeVerifier},
	}

	token, err := m.doTokenRequest(data)
	if err != nil {
		return nil, err
	}

	if err := m.store.Save(token); err != nil {
		return nil, fmt.Errorf("oauth: save token: %w", err)
	}

	return token, nil
}

// doTokenRequest 执行 token 端点 POST 请求。
// 调用方已持有 refreshMu；此处读取 tokenEndpoint 需 RLock。
func (m *OAuthTokenManager) doTokenRequest(data url.Values) (*OAuthToken, error) {
	m.mu.RLock()
	endpoint := m.tokenEndpoint
	m.mu.RUnlock()

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("oauth: read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth: token endpoint returned %d: %s", resp.StatusCode, truncateString(string(body), 256))
	}

	var token OAuthToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("oauth: parse token response: %w", err)
	}

	// 计算过期时间
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

// ---------- AS 发现 ----------

// AuthServerMetadata OAuth Authorization Server 元数据。
type AuthServerMetadata struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	RegistrationEndpoint   string   `json:"registration_endpoint,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	CodeChallengeSupported []string `json:"code_challenge_methods_supported,omitempty"`
}

// ensureDiscovery 确保已完成 AS 发现。
// 调用方已持有 refreshMu（串行化），此处用 RWMutex 保护共享状态读写。
func (m *OAuthTokenManager) ensureDiscovery() error {
	m.mu.RLock()
	done := m.discoveryDone
	m.mu.RUnlock()
	if done {
		return nil
	}

	// HTTP 请求在锁外执行
	meta, err := DiscoverAuthServer(m.client, m.cfg.IssuerURL)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.tokenEndpoint = meta.TokenEndpoint
	m.authEndpoint = meta.AuthorizationEndpoint
	m.discoveryDone = true
	m.mu.Unlock()
	return nil
}

// DiscoverAuthServer 执行 OAuth AS 发现。
// 流程: RFC 9728 (.well-known/oauth-protected-resource) → RFC 8414 (.well-known/oauth-authorization-server)
func DiscoverAuthServer(client *http.Client, issuerURL string) (*AuthServerMetadata, error) {
	issuerURL = strings.TrimRight(issuerURL, "/")

	// 尝试 RFC 8414 发现
	wellKnown := issuerURL + "/.well-known/oauth-authorization-server"
	req, err := http.NewRequest("GET", wellKnown, nil)
	if err != nil {
		return nil, fmt.Errorf("oauth discovery: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth discovery: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth discovery: HTTP %d from %s", resp.StatusCode, wellKnown)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("oauth discovery: read body: %w", err)
	}

	var meta AuthServerMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("oauth discovery: parse metadata: %w", err)
	}

	if meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("oauth discovery: missing token_endpoint")
	}

	// [FIX-01: RFC 8414 §3.3 issuer 验证 — 返回值必须与请求的 issuerURL 一致]
	if meta.Issuer != "" && meta.Issuer != issuerURL {
		return nil, fmt.Errorf("oauth discovery: issuer mismatch: expected %q, got %q (RFC 8414 §3.3)", issuerURL, meta.Issuer)
	}

	return &meta, nil
}

// ---------- PKCE ----------

// PKCEPair PKCE code verifier + code challenge。
type PKCEPair struct {
	CodeVerifier  string
	CodeChallenge string
	Method        string // "S256"
}

// GeneratePKCE 生成 PKCE S256 验证对。
func GeneratePKCE() (*PKCEPair, error) {
	// 生成 43-128 字节的随机 code verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("pkce: generate verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// S256: SHA-256(verifier) → base64url
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCEPair{
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
		Method:        "S256",
	}, nil
}

// ---------- 辅助 ----------

// truncateString UTF-8 安全截断：不分割多字节字符。
func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	truncated := s[:maxBytes]
	// 回退直到最后一个完整的 UTF-8 字符
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "..."
}
