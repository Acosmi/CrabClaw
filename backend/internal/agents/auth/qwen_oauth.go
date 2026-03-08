package auth

// qwen_oauth.go — Qwen Portal OAuth Token 刷新
// 对应 TS src/providers/qwen-portal-oauth.ts (55L)
//
// 实现 Qwen OAuth2 refresh_token 流程:
//   POST https://chat.qwen.ai/api/v1/oauth2/token
//   grant_type=refresh_token

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

// Qwen OAuth 常量。
const (
	QwenOAuthBaseURL       = "https://chat.qwen.ai"
	QwenOAuthTokenEndpoint = QwenOAuthBaseURL + "/api/v1/oauth2/token"
	QwenOAuthClientID      = "f0304373b74a44d2b584a3fb70ca9e56"
)

// qwenOAuthTokenResponse Qwen OAuth token 端点响应。
type qwenOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // 秒
}

// RefreshQwenPortalCredentials 刷新 Qwen Portal OAuth 凭据。
// 对应 TS: refreshQwenPortalCredentials(credentials)
func RefreshQwenPortalCredentials(ctx context.Context, client *http.Client, creds *OAuthCredentials) (*OAuthCredentials, error) {
	if creds == nil || strings.TrimSpace(creds.Refresh) == "" {
		return nil, fmt.Errorf("Qwen OAuth refresh token 缺失，请重新认证")
	}

	if client == nil {
		client = http.DefaultClient
	}

	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {creds.Refresh},
		"client_id":     {QwenOAuthClientID},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, QwenOAuthTokenEndpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建 Qwen OAuth 刷新请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Qwen OAuth 刷新请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("Qwen OAuth refresh token 已过期或无效，请使用 `crabclaw models auth login --provider qwen-portal` 重新认证")
	}

	if resp.StatusCode != http.StatusOK {
		bodyText, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Qwen OAuth 刷新失败: %s", string(bodyText))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 Qwen OAuth 响应失败: %w", err)
	}

	var tokenResp qwenOAuthTokenResponse
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析 Qwen OAuth 响应失败: %w", err)
	}

	if tokenResp.AccessToken == "" || tokenResp.ExpiresIn == 0 {
		return nil, fmt.Errorf("Qwen OAuth 刷新响应缺少 access token")
	}

	// 构建更新后的凭据
	refreshToken := tokenResp.RefreshToken
	if refreshToken == "" {
		refreshToken = creds.Refresh // 保留原 refresh token
	}

	return &OAuthCredentials{
		Access:  tokenResp.AccessToken,
		Refresh: refreshToken,
		Expires: nowMs() + tokenResp.ExpiresIn*1000,
		Email:   creds.Email,
	}, nil
}

// QwenPortalRefresher 实现 OAuthTokenRefresher 接口。
type QwenPortalRefresher struct {
	Client *http.Client
}

// RefreshToken 实现 OAuthTokenRefresher.RefreshToken。
func (r *QwenPortalRefresher) RefreshToken(provider string, refreshToken string) (*OAuthCredentials, error) {
	return RefreshQwenPortalCredentials(context.Background(), r.Client, &OAuthCredentials{
		Refresh: refreshToken,
	})
}

// nowMs 返回当前时间的毫秒时间戳。
func nowMs() int64 {
	return time.Now().UnixMilli()
}
