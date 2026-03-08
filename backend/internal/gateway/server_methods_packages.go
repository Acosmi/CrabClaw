package gateway

// server_methods_packages.go — packages.catalog.browse / packages.catalog.detail
// + packages.install / packages.update / packages.remove / packages.installed
//
// Phase 3A: 统一应用中心 Catalog 后端。
// 纯增量: 不修改任何现有 skills.* / plugins.* 接口。

import (
	"context"
	"log/slog"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// PackagesHandlers 返回 packages.* 方法处理器映射。
func PackagesHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"packages.catalog.browse": handlePackagesBrowse,
		"packages.catalog.detail": handlePackagesDetail,
		"packages.install":        handlePackagesInstall,
		"packages.update":         handlePackagesUpdate,
		"packages.remove":         handlePackagesRemove,
		"packages.installed":      handlePackagesInstalled,
	}
}

// handlePackagesBrowse 浏览统一目录。
func handlePackagesBrowse(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	catalog := state.PackageCatalog()
	if catalog == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "package catalog not initialized"))
		return
	}

	kind := readString(ctx.Params, "kind")
	keyword := readString(ctx.Params, "keyword")
	category := readString(ctx.Params, "category")
	page := readIntDefault(ctx.Params, "page", 1)
	pageSize := readIntDefault(ctx.Params, "pageSize", 20)

	reqCtx := ctx.Ctx
	if reqCtx == nil {
		reqCtx = context.Background()
	}

	items, total, err := catalog.Browse(reqCtx, kind, keyword, category, page, pageSize)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "browse failed: "+err.Error()))
		return
	}
	if items == nil {
		items = []types.PackageCatalogItem{}
	}

	ctx.Respond(true, map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
	}, nil)
}

// handlePackagesDetail 获取包详情。
func handlePackagesDetail(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	catalog := state.PackageCatalog()
	if catalog == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "package catalog not initialized"))
		return
	}

	id := readString(ctx.Params, "id")
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "id is required"))
		return
	}

	reqCtx := ctx.Ctx
	if reqCtx == nil {
		reqCtx = context.Background()
	}

	item, err := catalog.Detail(reqCtx, id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeNotFound, err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"item": item,
	}, nil)
}

// handlePackagesInstall 安装包。
func handlePackagesInstall(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	installer := state.PackageInstaller()
	if installer == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "package installer not initialized"))
		return
	}

	id := readString(ctx.Params, "id")
	kindStr := readString(ctx.Params, "kind")
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "id is required"))
		return
	}
	if kindStr == "" {
		kindStr = string(types.PackageKindSkill)
	}

	kind := types.PackageKind(kindStr)

	record, err := installer.Install(kind, id)
	if err != nil {
		slog.Warn("packages.install failed", "id", id, "kind", kindStr, "error", err)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "install failed: "+err.Error()))
		return
	}

	slog.Info("packages.install success", "id", id, "kind", kindStr, "key", record.Key)
	ctx.Respond(true, map[string]interface{}{
		"record": record,
	}, nil)
}

// handlePackagesUpdate 更新包（当前等同于重新安装）。
func handlePackagesUpdate(ctx *MethodHandlerContext) {
	handlePackagesInstall(ctx)
}

// handlePackagesRemove 移除包。
func handlePackagesRemove(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	installer := state.PackageInstaller()
	if installer == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "package installer not initialized"))
		return
	}

	id := readString(ctx.Params, "id")
	kindStr := readString(ctx.Params, "kind")
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "id is required"))
		return
	}

	kind := types.PackageKind(kindStr)

	if err := installer.Remove(kind, id); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "remove failed: "+err.Error()))
		return
	}

	slog.Info("packages.remove success", "id", id, "kind", kindStr)
	ctx.Respond(true, map[string]interface{}{
		"success": true,
	}, nil)
}

// handlePackagesInstalled 列出已安装包。
func handlePackagesInstalled(ctx *MethodHandlerContext) {
	state := ctx.Context.State
	if state == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway state not available"))
		return
	}

	ledger := state.PackageLedger()
	if ledger == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "package ledger not initialized"))
		return
	}

	kindStr := readString(ctx.Params, "kind")
	kind := types.PackageKind(kindStr)

	records := ledger.List(kind)
	if records == nil {
		records = []types.PackageInstallRecord{}
	}

	ctx.Respond(true, map[string]interface{}{
		"records": records,
	}, nil)
}

// readIntDefault 读取 int 参数，不存在或类型错误时返回默认值。
func readIntDefault(m map[string]interface{}, key string, defaultVal int) int {
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	}
	return defaultVal
}
