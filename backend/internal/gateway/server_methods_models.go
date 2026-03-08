package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// [FIX P4-L01 + X-03: nexus API HTTP client 包级复用，避免每次请求创建新连接池]
var nexusHTTPClient = &http.Client{Timeout: 15 * time.Second}

// models.* 方法处理器 — 对应 src/gateway/server-methods/models.ts
//
// 提供模型目录查询功能 + Phase 4 托管模型双轨。
// 依赖: ModelCatalog (models.list), GatewayState.ManagedCatalog (models.managed.*)

// ModelsHandlers 返回 models.* 方法处理器映射。
func ModelsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"models.list":            handleModelsList,
		"models.default.get":     handleModelsDefaultGet,
		"models.default.set":     handleModelsDefaultSet,
		"models.managed.list":    handleManagedModelsList,
		"models.managed.refresh": handleManagedModelsRefresh,
		"models.source.set":      handleModelsSourceSet,
		"models.wallet.balance":  handleModelsWalletBalance,
		"models.wallet.usage":    handleModelsWalletUsage,
	}
}

// ModelEntryWithSource 带 source 标记的模型条目（models.list 响应用）。
type ModelEntryWithSource struct {
	models.ModelCatalogEntry
	Source types.ModelSource `json:"source"`
}

// ---------- models.list ----------
// 返回模型目录全量条目。托管模型启用时追加。

func handleModelsList(ctx *MethodHandlerContext) {
	catalog := ctx.Context.ModelCatalog
	if catalog == nil {
		ctx.Respond(true, map[string]interface{}{
			"models": []interface{}{},
		}, nil)
		return
	}

	entries := catalog.All()
	result := make([]ModelEntryWithSource, 0, len(entries))
	for _, e := range entries {
		result = append(result, ModelEntryWithSource{
			ModelCatalogEntry: e,
			Source:            types.ModelSourceCustom,
		})
	}

	// Phase 4: 追加托管模型条目（State 可能为 nil — 单元测试场景）
	var mc *models.ManagedModelCatalog
	if ctx.Context.State != nil {
		mc = ctx.Context.State.ManagedCatalog()
	}
	if mc != nil {
		managedEntries, err := mc.List()
		if err == nil {
			for _, me := range managedEntries {
				result = append(result, ModelEntryWithSource{
					ModelCatalogEntry: models.ModelCatalogEntry{
						ID:       me.ModelID,
						Name:     me.Name,
						Provider: me.Provider,
					},
					Source: types.ModelSourceManaged,
				})
			}
		} else {
			slog.Warn("models.list: failed to fetch managed models", "error", err)
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"models": result,
	}, nil)
}

// ---------- models.default.get ----------
// 返回当前默认模型配置。

func handleModelsDefaultGet(ctx *MethodHandlerContext) {
	cfg := ctx.Context.Config
	if cfg == nil {
		ctx.Respond(true, map[string]interface{}{
			"model": nil,
		}, nil)
		return
	}

	var primary string
	if cfg.Agents != nil && cfg.Agents.Defaults != nil &&
		cfg.Agents.Defaults.Model != nil && cfg.Agents.Defaults.Model.Primary != "" {
		primary = cfg.Agents.Defaults.Model.Primary
	}

	ctx.Respond(true, map[string]interface{}{
		"model": primary,
	}, nil)
}

// ---------- models.default.set ----------
// 设置默认模型。

func handleModelsDefaultSet(ctx *MethodHandlerContext) {
	model, ok := ctx.Params["model"].(string)
	if !ok || model == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "model must be a non-empty string"))
		return
	}

	cfgLoader := ctx.Context.ConfigLoader
	if cfgLoader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "config loader not available"))
		return
	}

	cfg, err := cfgLoader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config load failed: "+err.Error()))
		return
	}

	if cfg.Agents == nil {
		cfg.Agents = &types.AgentsConfig{}
	}
	if cfg.Agents.Defaults == nil {
		cfg.Agents.Defaults = &types.AgentDefaultsConfig{}
	}
	if cfg.Agents.Defaults.Model == nil {
		cfg.Agents.Defaults.Model = &types.AgentModelListConfig{}
	}
	cfg.Agents.Defaults.Model.Primary = model

	if err := cfgLoader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config save failed: "+err.Error()))
		return
	}

	slog.Info("models.default.set: default model updated", "model", model)
	ctx.Respond(true, map[string]interface{}{
		"model": model,
	}, nil)
}

// ---------- models.managed.list ----------
// 返回托管模型列表。

func handleManagedModelsList(ctx *MethodHandlerContext) {
	if ctx.Context.State == nil {
		ctx.Respond(true, map[string]interface{}{
			"models":  []interface{}{},
			"enabled": false,
		}, nil)
		return
	}
	mc := ctx.Context.State.ManagedCatalog()
	if mc == nil {
		ctx.Respond(true, map[string]interface{}{
			"models":  []interface{}{},
			"enabled": false,
		}, nil)
		return
	}

	entries, err := mc.List()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "managed model fetch failed: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"models":  entries,
		"enabled": true,
	}, nil)
}

// ---------- models.managed.refresh ----------
// 强制刷新托管模型缓存。

func handleManagedModelsRefresh(ctx *MethodHandlerContext) {
	if ctx.Context.State == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "gateway state not available"))
		return
	}
	mc := ctx.Context.State.ManagedCatalog()
	if mc == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "managed models not enabled"))
		return
	}

	if err := mc.Refresh(); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "managed model refresh failed: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"refreshed": true,
	}, nil)
}

// ---------- models.source.set ----------
// 设置模型来源偏好（启用/禁用托管模型）。

func handleModelsSourceSet(ctx *MethodHandlerContext) {
	enabled, ok := ctx.Params["enabled"].(bool)
	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "enabled must be a boolean"))
		return
	}

	cfgLoader := ctx.Context.ConfigLoader
	if cfgLoader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "config loader not available"))
		return
	}

	cfg, err := cfgLoader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config load failed: "+err.Error()))
		return
	}

	if cfg.Models == nil {
		cfg.Models = &types.ModelsConfig{}
	}
	if cfg.Models.ManagedModels == nil {
		cfg.Models.ManagedModels = &types.ManagedModelsConfig{}
	}
	cfg.Models.ManagedModels.Enabled = enabled

	if err := cfgLoader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config save failed: "+err.Error()))
		return
	}

	slog.Info("models.source.set: managed models preference updated", "enabled", enabled)
	ctx.Respond(true, map[string]interface{}{
		"enabled": enabled,
	}, nil)
}

// ---------- models.wallet.balance ----------
// 通过 nexus-v4 查询钱包余额。

func handleModelsWalletBalance(ctx *MethodHandlerContext) {
	cfg := ctx.Context.Config
	if cfg == nil || cfg.Models == nil || cfg.Models.ManagedModels == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "managed models not configured"))
		return
	}

	proxyEndpoint := cfg.Models.ManagedModels.ProxyEndpoint
	if proxyEndpoint == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "proxy endpoint not configured"))
		return
	}

	// 从 proxyEndpoint 推导 wallet stats URL
	// proxyEndpoint 形如 https://nexus.example.com/api/v4/models/proxy
	// wallet stats 形如 https://nexus.example.com/api/v4/wallet/stats
	baseURL := deriveNexusBaseURL(proxyEndpoint)
	if baseURL == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "cannot derive nexus base URL"))
		return
	}

	var tokenProvider func() (string, error)
	if ctx.Context.State != nil {
		mc := ctx.Context.State.ManagedCatalog()
		if mc != nil {
			tokenProvider = mc.TokenProvider()
		}
	}
	if tokenProvider == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "auth not available"))
		return
	}

	result, err := fetchNexusAPI(baseURL+"/wallet/stats", tokenProvider)
	if err != nil {
		slog.Warn("models.wallet.balance: fetch failed", "error", err)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "wallet fetch failed: "+err.Error()))
		return
	}

	ctx.Respond(true, result, nil)
}

// ---------- models.wallet.usage ----------
// 通过 nexus-v4 查询交易记录。

func handleModelsWalletUsage(ctx *MethodHandlerContext) {
	cfg := ctx.Context.Config
	if cfg == nil || cfg.Models == nil || cfg.Models.ManagedModels == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "managed models not configured"))
		return
	}

	proxyEndpoint := cfg.Models.ManagedModels.ProxyEndpoint
	if proxyEndpoint == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "proxy endpoint not configured"))
		return
	}

	baseURL := deriveNexusBaseURL(proxyEndpoint)
	if baseURL == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "cannot derive nexus base URL"))
		return
	}

	var tokenProvider func() (string, error)
	if ctx.Context.State != nil {
		mc := ctx.Context.State.ManagedCatalog()
		if mc != nil {
			tokenProvider = mc.TokenProvider()
		}
	}
	if tokenProvider == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "auth not available"))
		return
	}

	result, err := fetchNexusAPI(baseURL+"/wallet/transactions", tokenProvider)
	if err != nil {
		slog.Warn("models.wallet.usage: fetch failed", "error", err)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "wallet fetch failed: "+err.Error()))
		return
	}

	ctx.Respond(true, result, nil)
}

// deriveNexusBaseURL 从 proxyEndpoint 推导 nexus-v4 base URL。
// 输入: https://nexus.example.com/api/v4/models/proxy
// 输出: https://nexus.example.com/api/v4
func deriveNexusBaseURL(proxyEndpoint string) string {
	// 查找 /api/v4/ 后的路径部分并截断
	const marker = "/api/v4/"
	idx := 0
	for i := 0; i <= len(proxyEndpoint)-len(marker); i++ {
		if proxyEndpoint[i:i+len(marker)] == marker {
			idx = i + len(marker) - 1 // 保留 /api/v4
			break
		}
	}
	if idx > 0 {
		return proxyEndpoint[:idx]
	}
	return ""
}

// fetchNexusAPI 向 nexus-v4 发送 GET 请求。
func fetchNexusAPI(url string, tokenProvider func() (string, error)) (interface{}, error) {
	token, err := tokenProvider()
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := nexusHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := string(body)
		if len(msg) > 256 {
			msg = msg[:256] + "..."
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}

	// nexus-v4 response 格式: {"code":0,"message":"success","data":...}
	var wrapper struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Data != nil {
		var result interface{}
		if err := json.Unmarshal(wrapper.Data, &result); err == nil {
			return result, nil
		}
	}

	// 直接返回响应
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return result, nil
}
