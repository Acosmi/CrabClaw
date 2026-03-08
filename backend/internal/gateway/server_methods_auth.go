package gateway

// server_methods_auth.go — OAuth 2.1 认证 RPC 方法。
// [FIX-01: P2A-X01 + X-01 + P0-L03 AuthState 接入]
//
// 方法:
//   auth.state          — 返回当前认证状态 (AuthState)
//   auth.login.start    — 启动 PKCE 授权流程 (生成 authURL + callback server)
//   auth.login.exchange — 用授权码交换 token (手动 code 输入或 callback 自动接收)
//   auth.logout         — 清除 token + 重置状态

import (
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/mcpremote"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// AuthHandlers 返回 OAuth 认证相关 RPC handler。
func AuthHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"auth.state":          handleAuthState,
		"auth.login.start":    handleAuthLoginStart,
		"auth.login.exchange": handleAuthLoginExchange,
		"auth.logout":         handleAuthLogout,
	}
}

// handleAuthState 返回当前认证状态。
// RPC: auth.state → { isAuthenticated, tokenExpiresAt, ... }
func handleAuthState(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(true, types.AuthState{IsAuthenticated: false}, nil)
		return
	}

	authMgr := state.AuthManager()
	if authMgr == nil {
		ctx.Respond(true, types.AuthState{IsAuthenticated: false}, nil)
		return
	}

	token, err := authMgr.GetAccessToken()
	if err != nil || token == "" {
		ctx.Respond(true, types.AuthState{IsAuthenticated: false}, nil)
		return
	}

	authState := types.AuthState{
		IsAuthenticated: true,
	}

	ctx.Respond(true, authState, nil)
}

// pendingAuth 保存进行中的 PKCE 授权流程状态。
type pendingAuth struct {
	pkce           *mcpremote.PKCEPair
	callbackServer *AuthCallbackServer
	createdAt      time.Time
}

// 包级变量: 最多一个进行中的授权流程
var currentPendingAuth *pendingAuth

// handleAuthLoginStart 启动 PKCE 授权流程。
// RPC: auth.login.start → { authURL, redirectURI, callbackPort }
func handleAuthLoginStart(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	// 读取 OAuth 配置
	cfg := ctx.Context.Config
	if cfg == nil || cfg.Skills == nil || cfg.Skills.Store == nil || cfg.Skills.Store.OAuth == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "OAuth not configured — set skills.store.oauth in config"))
		return
	}

	oauthCfg := cfg.Skills.Store.OAuth
	if oauthCfg.IssuerURL == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "OAuth issuerURL not configured"))
		return
	}

	// 清理旧的 pending auth
	if currentPendingAuth != nil && currentPendingAuth.callbackServer != nil {
		currentPendingAuth.callbackServer.Stop()
		currentPendingAuth = nil
	}

	// 1. 生成 PKCE pair
	pkce, err := mcpremote.GeneratePKCE()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "generate PKCE: "+err.Error()))
		return
	}

	// 2. 创建 callback server (RFC 8252 §7.3: 127.0.0.1 + random port)
	callbackSrv, err := NewAuthCallbackServer()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "create callback server: "+err.Error()))
		return
	}
	callbackSrv.Start()

	// 3. 构造授权 URL
	clientID := oauthCfg.ClientID
	if clientID == "" {
		clientID = "crab-claw-desktop" // 默认客户端 ID
	}

	authURL := buildAuthorizationURL(oauthCfg.IssuerURL, clientID, callbackSrv.RedirectURI(), pkce)

	// 4. 保存 pending auth
	currentPendingAuth = &pendingAuth{
		pkce:           pkce,
		callbackServer: callbackSrv,
		createdAt:      time.Now(),
	}

	slog.Info("auth: login flow started", "callbackPort", callbackSrv.Port())

	ctx.Respond(true, map[string]interface{}{
		"authURL":      authURL,
		"redirectURI":  callbackSrv.RedirectURI(),
		"callbackPort": callbackSrv.Port(),
	}, nil)
}

// handleAuthLoginExchange 用授权码交换 token。
// RPC: auth.login.exchange { code?: string }
// 如果 code 为空，等待 callback server 接收（最多 120s）。
func handleAuthLoginExchange(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	if currentPendingAuth == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "no pending auth flow — call auth.login.start first"))
		return
	}

	pending := currentPendingAuth

	// 获取 code: 优先参数传入，否则等待 callback server
	code, _ := ctx.Params["code"].(string)
	if code == "" {
		var err error
		code, err = pending.callbackServer.WaitForCode(120 * time.Second)
		if err != nil {
			pending.callbackServer.Stop()
			currentPendingAuth = nil
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "wait for code: "+err.Error()))
			return
		}
	}

	// 关闭 callback server
	pending.callbackServer.Stop()
	currentPendingAuth = nil

	// 交换 token
	authMgr := state.AuthManager()
	if authMgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "auth manager not initialized"))
		return
	}

	token, err := authMgr.ExchangeCode(code, pending.pkce.CodeVerifier)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "exchange code: "+err.Error()))
		return
	}

	slog.Info("auth: login successful", "expiresIn", token.ExpiresIn)

	ctx.Respond(true, map[string]interface{}{
		"success":   true,
		"expiresIn": token.ExpiresIn,
	}, nil)
}

// handleAuthLogout 清除 token + 重置状态。
// RPC: auth.logout
func handleAuthLogout(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(true, map[string]interface{}{"success": true}, nil)
		return
	}

	authMgr := state.AuthManager()
	if authMgr == nil {
		ctx.Respond(true, map[string]interface{}{"success": true}, nil)
		return
	}

	// 清除 token store
	if store := authMgr.Store(); store != nil {
		if err := store.Clear(); err != nil {
			slog.Warn("auth: clear token failed", "error", err)
		}
	}

	slog.Info("auth: logged out")
	ctx.Respond(true, map[string]interface{}{"success": true}, nil)
}

// buildAuthorizationURL 构造 OAuth 2.1 授权 URL。
func buildAuthorizationURL(issuerURL, clientID, redirectURI string, pkce *mcpremote.PKCEPair) string {
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {pkce.CodeChallenge},
		"code_challenge_method": {pkce.Method},
		"scope":                 {"openid profile"},
	}
	return fmt.Sprintf("%s/oauth/authorize?%s", issuerURL, params.Encode())
}

// ResolveAuthTokenPath 返回 token 文件路径。
func ResolveAuthTokenPath() string {
	return filepath.Join(config.ResolveStateDir(), "auth", "oauth_token.json")
}
