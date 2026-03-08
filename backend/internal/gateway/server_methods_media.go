package gateway

// server_methods_media.go — 媒体子系统 RPC 方法
// 提供 media.trending.fetch / media.trending.sources / media.drafts.list / media.drafts.get / media.drafts.delete 方法
// 遵循 server_methods_image.go 模式

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

var knownMediaSources = []string{"weibo", "baidu", "zhihu"}

// MediaHandlers 返回媒体子系统 RPC 方法处理器。
func MediaHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"media.trending.fetch":   handleMediaTrendingFetch,
		"media.trending.sources": handleMediaTrendingSources,
		"media.trending.health":  handleMediaTrendingHealth,
		"media.drafts.list":      handleMediaDraftsList,
		"media.drafts.get":       handleMediaDraftsGet,
		"media.drafts.delete":    handleMediaDraftsDelete,
		"media.drafts.update":    handleMediaDraftsUpdate,
		"media.drafts.approve":   handleMediaDraftsApprove,
		"media.publish.list":     handleMediaPublishList,
		"media.publish.get":      handleMediaPublishGet,
		"media.config.get":       handleMediaConfigGet,
		"media.config.update":    handleMediaConfigUpdate,
		"media.tools.list":       handleMediaToolsList,
		"media.tools.toggle":     handleMediaToolsToggle,
		"media.sources.toggle":   handleMediaSourcesToggle,
	}
}

// appendSharedMediaTools 追加共享 runner 工具到工具列表（去重 webSearch 逻辑）。
func appendSharedMediaTools(tools []map[string]interface{}, cfg *types.OpenAcosmiConfig) []map[string]interface{} {
	webSearchEnabled := false
	if cfg != nil && cfg.Tools != nil && cfg.Tools.Web != nil &&
		cfg.Tools.Web.Search != nil && cfg.Tools.Web.Search.Bocha != nil {
		webSearchEnabled = cfg.Tools.Web.Search.Bocha.Enabled == nil || *cfg.Tools.Web.Search.Bocha.Enabled
	}
	tools = append(tools, map[string]interface{}{
		"name":        "web_search",
		"description": "Search the web for information and references",
		"enabled":     webSearchEnabled,
		"scope":       "shared",
	})
	tools = append(tools, map[string]interface{}{
		"name":        "report_progress",
		"description": "Report intermediate progress to the user",
		"enabled":     true,
		"scope":       "shared",
	})
	return tools
}

func mediaAgentSettings(cfg *types.OpenAcosmiConfig) *types.MediaAgentSettings {
	if cfg == nil || cfg.SubAgents == nil {
		return nil
	}
	return cfg.SubAgents.MediaAgent
}

func mediaChannels(cfg *types.OpenAcosmiConfig) *types.ChannelsConfig {
	if cfg == nil {
		return nil
	}
	return cfg.Channels
}

func unionMediaSourceNames(runtimeNames []string, explicitNames []string) []string {
	names := make([]string, 0, len(knownMediaSources)+len(runtimeNames)+len(explicitNames))
	for _, name := range knownMediaSources {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	for _, name := range runtimeNames {
		if strings.TrimSpace(name) == "" || slices.Contains(names, name) {
			continue
		}
		names = append(names, name)
	}
	for _, name := range explicitNames {
		if strings.TrimSpace(name) == "" || slices.Contains(names, name) {
			continue
		}
		names = append(names, name)
	}
	return names
}

func resolveMediaSourceState(cfg *types.OpenAcosmiConfig, runtimeNames []string) ([]map[string]interface{}, []string, bool) {
	ma := mediaAgentSettings(cfg)
	allNames := unionMediaSourceNames(runtimeNames, nil)
	enabledSourcesConfigured := ma != nil && ma.EnabledSources != nil
	enabledSet := map[string]bool{}

	if enabledSourcesConfigured {
		allNames = unionMediaSourceNames(runtimeNames, ma.EnabledSources)
		for _, name := range ma.EnabledSources {
			if strings.TrimSpace(name) == "" {
				continue
			}
			enabledSet[name] = true
		}
	} else {
		for _, name := range allNames {
			enabledSet[name] = true
		}
	}

	sort.Strings(allNames)

	enabledSources := make([]string, 0, len(allNames))
	sources := make([]map[string]interface{}, 0, len(allNames))
	for _, name := range allNames {
		enabled := enabledSet[name]
		status := "disabled"
		if enabledSourcesConfigured && enabled {
			status = "configured"
		} else if !enabledSourcesConfigured && enabled {
			status = "default_enabled"
		}
		if enabled {
			enabledSources = append(enabledSources, name)
		}
		sources = append(sources, map[string]interface{}{
			"name":       name,
			"enabled":    enabled,
			"configured": enabledSourcesConfigured,
			"status":     status,
		})
	}

	return sources, enabledSources, enabledSourcesConfigured
}

func isWeChatPublisherConfigured(cfg *types.OpenAcosmiConfig) bool {
	ch := mediaChannels(cfg)
	if ch == nil || ch.WeChatMP == nil {
		return false
	}
	return ch.WeChatMP.Enabled &&
		strings.TrimSpace(ch.WeChatMP.AppID) != "" &&
		strings.TrimSpace(ch.WeChatMP.AppSecret) != ""
}

func isXiaohongshuConfigured(cfg *types.OpenAcosmiConfig) bool {
	ch := mediaChannels(cfg)
	if ch == nil || ch.Xiaohongshu == nil {
		return false
	}
	return ch.Xiaohongshu.Enabled &&
		strings.TrimSpace(ch.Xiaohongshu.CookiePath) != ""
}

func isWebsitePublisherConfigured(cfg *types.OpenAcosmiConfig) bool {
	ch := mediaChannels(cfg)
	if ch == nil || ch.Website == nil {
		return false
	}
	return ch.Website.Enabled &&
		strings.TrimSpace(ch.Website.APIURL) != "" &&
		strings.TrimSpace(ch.Website.AuthType) != "" &&
		strings.TrimSpace(ch.Website.AuthToken) != ""
}

func configuredPublishers(cfg *types.OpenAcosmiConfig) []string {
	publishers := make([]string, 0, 3)
	if isWeChatPublisherConfigured(cfg) {
		publishers = append(publishers, string(media.PlatformWeChat))
	}
	if isXiaohongshuConfigured(cfg) {
		publishers = append(publishers, string(media.PlatformXiaohongshu))
	}
	if isWebsitePublisherConfigured(cfg) {
		publishers = append(publishers, string(media.PlatformWebsite))
	}
	return publishers
}

func mediaToolState(name string, enabled bool, cfg *types.OpenAcosmiConfig) (string, bool) {
	switch name {
	case media.ToolTrendingTopics, media.ToolContentCompose:
		return "builtin", false
	case media.ToolMediaPublish:
		if !enabled {
			return "disabled", false
		}
		configured := len(configuredPublishers(cfg)) > 0
		if configured {
			return "configured", true
		}
		return "needs_configuration", false
	case media.ToolSocialInteract:
		if !enabled {
			return "disabled", false
		}
		configured := isXiaohongshuConfigured(cfg)
		if configured {
			return "configured", true
		}
		return "needs_configuration", false
	default:
		if enabled {
			return "enabled", false
		}
		return "disabled", false
	}
}

// ---------- media.trending.fetch ----------

func handleMediaTrendingFetch(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	source, _ := ctx.Params["source"].(string)
	category, _ := ctx.Params["category"].(string)
	limit := 20
	if l, ok := ctx.Params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	fetchCtx, cancel := context.WithTimeout(ctx.Ctx, 15*time.Second)
	defer cancel()

	if source != "" {
		topics, err := sub.Aggregator.FetchBySource(fetchCtx, source, category, limit)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "fetch trending: "+err.Error()))
			return
		}
		ctx.Respond(true, map[string]interface{}{
			"source": source,
			"topics": topics,
			"count":  len(topics),
		}, nil)
		return
	}

	topics, results := sub.Aggregator.FetchAll(fetchCtx, category, limit)
	var errors []map[string]string
	for _, r := range results {
		if r.Err != nil {
			errors = append(errors, map[string]string{
				"source": r.Source,
				"error":  r.Err.Error(),
			})
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"topics": topics,
		"count":  len(topics),
		"errors": errors,
	}, nil)
}

// ---------- media.trending.sources ----------

func handleMediaTrendingSources(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	names := sub.Aggregator.SourceNames()
	ctx.Respond(true, map[string]interface{}{
		"sources": names,
	}, nil)
}

// ---------- media.trending.health ----------

func handleMediaTrendingHealth(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	// Probe each source with limit=1 to check health
	probeCtx, cancel := context.WithTimeout(ctx.Ctx, 15*time.Second)
	defer cancel()
	_, results := sub.Aggregator.FetchAll(probeCtx, "all", 1)

	type sourceHealth struct {
		Name   string `json:"name"`
		Status string `json:"status"` // "ok" | "error"
		Error  string `json:"error,omitempty"`
		Count  int    `json:"count"`
	}

	sources := make([]sourceHealth, 0, len(results))
	for _, r := range results {
		h := sourceHealth{
			Name:  r.Source,
			Count: len(r.Topics),
		}
		if r.Err != nil {
			h.Status = "error"
			h.Error = r.Err.Error()
		} else {
			h.Status = "ok"
		}
		sources = append(sources, h)
	}

	ctx.Respond(true, map[string]interface{}{
		"sources": sources,
	}, nil)
}

// ---------- media.drafts.list ----------

func handleMediaDraftsList(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	platform, _ := ctx.Params["platform"].(string)
	drafts, err := sub.DraftStore.List(platform)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "list drafts: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"drafts": drafts,
		"count":  len(drafts),
	}, nil)
}

// ---------- media.drafts.get ----------

func handleMediaDraftsGet(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	draft, err := sub.DraftStore.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get draft: "+err.Error()))
		return
	}

	ctx.Respond(true, draft, nil)
}

// ---------- media.drafts.delete ----------

func handleMediaDraftsDelete(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	if err := sub.DraftStore.Delete(id); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "delete draft: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"deleted": true,
		"id":      id,
	}, nil)
}

// ---------- media.drafts.update ----------

func handleMediaDraftsUpdate(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	// 加载现有草稿
	draft, err := sub.DraftStore.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get draft: "+err.Error()))
		return
	}

	// 应用部分更新
	if title, ok := ctx.Params["title"].(string); ok && title != "" {
		draft.Title = title
	}
	if body, ok := ctx.Params["body"].(string); ok {
		draft.Body = body
	}
	if platform, ok := ctx.Params["platform"].(string); ok && platform != "" {
		draft.Platform = media.Platform(platform)
	}
	if tagsRaw, ok := ctx.Params["tags"].([]interface{}); ok {
		tags := make([]string, 0, len(tagsRaw))
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		draft.Tags = tags
	}

	draft.UpdatedAt = time.Now()

	if err := sub.DraftStore.Save(draft); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "save draft: "+err.Error()))
		return
	}

	ctx.Respond(true, draft, nil)
}

// ---------- media.drafts.approve ----------

func handleMediaDraftsApprove(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	// 加载草稿以验证当前状态
	draft, err := sub.DraftStore.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get draft: "+err.Error()))
		return
	}

	// 仅允许从 pending_review 状态审批
	if draft.Status != media.DraftStatusPendingReview {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"draft cannot be approved: current status is "+string(draft.Status)+", expected pending_review"))
		return
	}

	if err := sub.DraftStore.UpdateStatus(id, media.DraftStatusApproved); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "approve draft: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":     true,
		"id":     id,
		"status": "approved",
	}, nil)
}

// ---------- media.publish.list ----------

func handleMediaPublishList(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	if sub.PublishHistory == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "publish history not available"))
		return
	}

	var opts *media.PublishListOptions
	limit, hasLimit := ctx.Params["limit"].(float64)
	offset, hasOffset := ctx.Params["offset"].(float64)
	if hasLimit || hasOffset {
		opts = &media.PublishListOptions{}
		if hasLimit && limit > 0 {
			opts.Limit = int(limit)
		}
		if hasOffset && offset > 0 {
			opts.Offset = int(offset)
		}
	}

	records, err := sub.PublishHistory.List(opts)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "list publish history: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"records": records,
		"count":   len(records),
	}, nil)
}

// ---------- media.publish.get ----------

func handleMediaPublishGet(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	if sub.PublishHistory == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "publish history not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing publish record id"))
		return
	}

	record, err := sub.PublishHistory.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get publish record: "+err.Error()))
		return
	}

	ctx.Respond(true, record, nil)
}

// ---------- media.config.get ----------

func handleMediaConfigGet(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	// 热加载最新配置
	liveCfg := ctx.Context.Config
	if ctx.Context.ConfigLoader != nil {
		if fresh, err := ctx.Context.ConfigLoader.LoadConfig(); err == nil {
			liveCfg = fresh
		}
	}

	// 热点来源
	sources, enabledSources, enabledSourcesConfigured := resolveMediaSourceState(liveCfg, sub.Aggregator.SourceNames())

	// 工具列表 — 使用 DefaultMediaToolDefs 获取 enabled/scope 完整信息
	toolsCfg := media.MediaToolsConfig{
		DraftStore:     sub.DraftStore,
		Aggregator:     sub.Aggregator,
		EnablePublish:  sub.PublishHistory != nil,
		EnableInteract: sub.GetTool(media.ToolSocialInteract) != nil,
	}
	toolDefs := media.DefaultMediaToolDefs(toolsCfg)
	tools := make([]map[string]interface{}, 0, len(toolDefs)+2)
	for _, d := range toolDefs {
		status, configured := mediaToolState(d.Name, d.Enabled, liveCfg)
		tools = append(tools, map[string]interface{}{
			"name":        d.Name,
			"description": d.Description,
			"enabled":     d.Enabled,
			"configured":  configured,
			"scope":       "media",
			"status":      status,
		})
	}
	// 共享 runner 工具
	tools = appendSharedMediaTools(tools, liveCfg)

	// 发布器
	publishers := configuredPublishers(liveCfg)
	publishConfigured := len(publishers) > 0

	// LLM 配置（从 live config 读取）
	llmConfig := map[string]interface{}{
		"provider":            "",
		"model":               "",
		"apiKey":              "",
		"baseUrl":             "",
		"autoSpawnEnabled":    false,
		"maxAutoSpawnsPerDay": 5,
	}
	configStatus := "default"
	if liveCfg != nil && liveCfg.SubAgents != nil && liveCfg.SubAgents.MediaAgent != nil {
		ma := liveCfg.SubAgents.MediaAgent
		llmConfig["provider"] = ma.Provider
		llmConfig["model"] = ma.Model
		// API key 脱敏
		if ma.APIKey != "" {
			if len(ma.APIKey) > 8 {
				llmConfig["apiKey"] = ma.APIKey[:4] + "****" + ma.APIKey[len(ma.APIKey)-4:]
			} else {
				llmConfig["apiKey"] = "****"
			}
		}
		llmConfig["baseUrl"] = ma.BaseURL
		llmConfig["autoSpawnEnabled"] = ma.AutoSpawnEnabled
		if ma.MaxAutoSpawnsPerDay > 0 {
			llmConfig["maxAutoSpawnsPerDay"] = ma.MaxAutoSpawnsPerDay
		}
		if ma.Provider != "" || ma.Model != "" {
			configStatus = "configured"
		}
	}

	// 高级热点策略配置
	trendingStrategy := map[string]interface{}{
		"hotKeywords":        []string{},
		"monitorIntervalMin": 30,
		"trendingThreshold":  float64(10000),
		"contentCategories":  []string{},
		"autoDraftEnabled":   false,
	}
	// 来源配置状态（区分 nil 未配置 vs [] 显式全禁用）
	if liveCfg != nil && liveCfg.SubAgents != nil && liveCfg.SubAgents.MediaAgent != nil {
		ma := liveCfg.SubAgents.MediaAgent
		enabledSourcesConfigured = ma.EnabledSources != nil
		if len(ma.HotKeywords) > 0 {
			trendingStrategy["hotKeywords"] = ma.HotKeywords
		}
		if ma.MonitorIntervalMin > 0 {
			trendingStrategy["monitorIntervalMin"] = ma.MonitorIntervalMin
		}
		if ma.TrendingThreshold != nil {
			trendingStrategy["trendingThreshold"] = *ma.TrendingThreshold
		}
		if len(ma.ContentCategories) > 0 {
			trendingStrategy["contentCategories"] = ma.ContentCategories
		}
		trendingStrategy["autoDraftEnabled"] = ma.AutoDraftEnabled
	}

	ctx.Respond(true, map[string]interface{}{
		"agent_id":                   "oa-media",
		"label":                      "媒体运营智能体",
		"status":                     configStatus,
		"trending_sources":           sources,
		"tools":                      tools,
		"publishers":                 publishers,
		"publish_enabled":            sub.PublishHistory != nil,
		"publish_configured":         publishConfigured,
		"llm":                        llmConfig,
		"trending_strategy":          trendingStrategy,
		"enabled_sources":            enabledSources,
		"enabled_sources_configured": enabledSourcesConfigured,
	}, nil)
}

// ---------- media.config.update ----------

func handleMediaConfigUpdate(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	// 加载最新配置
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// 确保 SubAgents.MediaAgent 存在
	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.MediaAgent == nil {
		cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
	}
	ma := cfg.SubAgents.MediaAgent

	// 合入参数
	if v, ok := ctx.Params["provider"].(string); ok {
		ma.Provider = v
	}
	if v, ok := ctx.Params["model"].(string); ok {
		ma.Model = v
	}
	if v, ok := ctx.Params["apiKey"].(string); ok && v != "" {
		// 只有非空且不含 **** 才更新（避免脱敏值覆盖）
		if !strings.Contains(v, "****") {
			ma.APIKey = v
		}
	}
	if v, ok := ctx.Params["baseUrl"].(string); ok {
		ma.BaseURL = v
	}
	if v, ok := ctx.Params["autoSpawnEnabled"].(bool); ok {
		ma.AutoSpawnEnabled = v
	}
	if v, ok := ctx.Params["maxAutoSpawnsPerDay"].(float64); ok && v > 0 {
		if v > 100 {
			v = 100
		}
		ma.MaxAutoSpawnsPerDay = int(v)
	}

	// 高级热点策略字段
	if v, ok := ctx.Params["hotKeywords"].([]interface{}); ok {
		keywords := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				keywords = append(keywords, s)
			}
		}
		ma.HotKeywords = keywords
	}
	if v, ok := ctx.Params["monitorIntervalMin"].(float64); ok && v > 0 {
		if v < 5 {
			v = 5 // 最小 5 分钟
		}
		if v > 1440 { // 最大 24 小时
			v = 1440
		}
		ma.MonitorIntervalMin = int(v)
	}
	if v, ok := ctx.Params["trendingThreshold"].(float64); ok && v >= 0 {
		ma.TrendingThreshold = &v
	}
	if v, ok := ctx.Params["contentCategories"].([]interface{}); ok {
		cats := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				cats = append(cats, s)
			}
		}
		ma.ContentCategories = cats
	}
	if v, ok := ctx.Params["autoDraftEnabled"].(bool); ok {
		ma.AutoDraftEnabled = v
	}

	// 写入配置文件
	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}

	slog.Info("media config updated",
		"provider", ma.Provider,
		"model", ma.Model,
		"autoSpawn", ma.AutoSpawnEnabled,
	)

	ctx.Respond(true, map[string]interface{}{
		"ok": true,
	}, nil)
}

// ---------- media.tools.list ----------

func handleMediaToolsList(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	cfg := media.MediaToolsConfig{
		DraftStore:     sub.DraftStore,
		Aggregator:     sub.Aggregator,
		EnablePublish:  sub.PublishHistory != nil,
		EnableInteract: sub.GetTool(media.ToolSocialInteract) != nil,
	}
	defs := media.DefaultMediaToolDefs(cfg)

	result := make([]map[string]interface{}, 0, len(defs)+3)
	for _, d := range defs {
		result = append(result, map[string]interface{}{
			"name":        d.Name,
			"description": d.Description,
			"enabled":     d.Enabled,
			"scope":       "media",
		})
	}

	// 共享 runner 工具（媒体子智能体运行时自动获得）— 热加载配置
	liveCfg := ctx.Context.Config
	if ctx.Context.ConfigLoader != nil {
		if fresh, err := ctx.Context.ConfigLoader.LoadConfig(); err == nil {
			liveCfg = fresh
		}
	}
	result = appendSharedMediaTools(result, liveCfg)

	ctx.Respond(true, map[string]interface{}{
		"tools": result,
		"count": len(result),
	}, nil)
}

// ---------- media.tools.toggle ----------

func handleMediaToolsToggle(ctx *MethodHandlerContext) {
	tool, _ := ctx.Params["tool"].(string)
	enabled, hasEnabled := ctx.Params["enabled"].(bool)
	if tool == "" || !hasEnabled {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "tool and enabled required"))
		return
	}

	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.MediaAgent == nil {
		cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
	}
	ma := cfg.SubAgents.MediaAgent

	switch tool {
	case "media_publish":
		ma.EnablePublish = &enabled
	case "social_interact":
		ma.EnableInteract = &enabled
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "tool toggle not supported for: "+tool))
		return
	}

	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}

	slog.Info("media tool toggled", "tool", tool, "enabled", enabled)
	ctx.Respond(true, map[string]interface{}{"ok": true, "tool": tool, "enabled": enabled}, nil)
}

// ---------- media.sources.toggle ----------

func handleMediaSourcesToggle(ctx *MethodHandlerContext) {
	source, _ := ctx.Params["source"].(string)
	enabled, hasEnabled := ctx.Params["enabled"].(bool)
	if source == "" || !hasEnabled {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "source and enabled required"))
		return
	}

	validSources := map[string]bool{"weibo": true, "baidu": true, "zhihu": true}
	if !validSources[source] {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unknown source: "+source))
		return
	}

	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.MediaAgent == nil {
		cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
	}
	ma := cfg.SubAgents.MediaAgent

	// nil 语义: nil=全部启用。首次 toggle 时展开为完整列表再操作。
	if ma.EnabledSources == nil {
		allSources := []string{"weibo", "baidu", "zhihu"}
		ma.EnabledSources = allSources
	}

	if enabled {
		// 从 EnabledSources 中确保存在
		found := false
		for _, s := range ma.EnabledSources {
			if s == source {
				found = true
				break
			}
		}
		if !found {
			ma.EnabledSources = append(ma.EnabledSources, source)
		}
	} else {
		// 从 EnabledSources 中移除
		filtered := make([]string, 0, len(ma.EnabledSources))
		for _, s := range ma.EnabledSources {
			if s != source {
				filtered = append(filtered, s)
			}
		}
		ma.EnabledSources = filtered
	}

	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}

	slog.Info("media source toggled", "source", source, "enabled", enabled)
	ctx.Respond(true, map[string]interface{}{"ok": true, "source": source, "enabled": enabled}, nil)
}
