package gateway

// server_methods_update.go — update.* 方法处理器
// 对应 TS: src/gateway/server-methods/update.ts (132L)
//
// 方法列表 (2): update.check, update.run
//
// TS 的 update.run 执行 `npm update`、写重启哨兵、定时重启。
// Go 版本自更新替代为环境模式判断 + 哨兵文件 + 进程信号。

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// UpdateHandlers 返回 update.* 方法映射。
func UpdateHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"update.check": handleUpdateCheck,
		"update.run":   handleUpdateRun,
	}
}

// ---------- update.check ----------

func handleUpdateCheck(ctx *MethodHandlerContext) {
	// TS: 检查配置中的 version / channel 信息
	result := map[string]interface{}{
		"currentVersion":  resolveCurrentVersion(),
		"platform":        runtime.GOOS,
		"arch":            runtime.GOARCH,
		"updateAvailable": false,
	}

	ctx.Respond(true, result, nil)
}

// ---------- update.run ----------
// TS: npm update -> 写重启哨兵 + scheduleRestartMs
// Go: go binary 无 npm update，替代为重启哨兵 + 进程信号方案

func handleUpdateRun(ctx *MethodHandlerContext) {
	baseDir := ctx.Context.PairingBaseDir
	if baseDir == "" {
		baseDir = resolveDefaultStateDir()
	}

	// 仅在开发模式下支持自更新（go install / go build）
	mode, _ := ctx.Params["mode"].(string)
	if mode == "" {
		mode = "default"
	}

	// 确保基目录存在
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to create state dir: "+err.Error()))
		return
	}

	// 选择更新策略
	switch mode {
	case "restart":
		// 仅写重启哨兵（进程监控端负责重启）
		sentinelPath := filepath.Join(baseDir, ".restart-sentinel")
		if err := os.WriteFile(sentinelPath, []byte(fmt.Sprintf("%d", time.Now().UnixMilli())), 0600); err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write restart sentinel: "+err.Error()))
			return
		}
		ctx.Respond(true, map[string]interface{}{
			"ok":           true,
			"action":       "restart-scheduled",
			"sentinelPath": sentinelPath,
		}, nil)

	case "dev":
		// 开发模式：执行 go build / go install
		goPath, err := exec.LookPath("go")
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "go not found in PATH"))
			return
		}

		cmd := exec.Command(goPath, "build", "-o", "openacosmi-gateway", ".")
		cmd.Dir = resolveProjectRoot()
		output, err := cmd.CombinedOutput()
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "build failed: "+string(output)))
			return
		}

		// 写重启哨兵
		sentinelPath := filepath.Join(baseDir, ".restart-sentinel")
		_ = os.WriteFile(sentinelPath, []byte(fmt.Sprintf("%d", time.Now().UnixMilli())), 0600)

		ctx.Respond(true, map[string]interface{}{
			"ok":     true,
			"action": "rebuilt-and-restart-scheduled",
			"output": strings.TrimSpace(string(output)),
		}, nil)

	default:
		// 默认模式：检查 + 写哨兵
		scheduleRestartMs, _ := ctx.Params["scheduleRestartMs"].(float64)
		sentinelPath := filepath.Join(baseDir, ".restart-sentinel")
		payload := fmt.Sprintf(`{"ts":%d,"scheduleRestartMs":%d}`, time.Now().UnixMilli(), int64(scheduleRestartMs))
		if err := os.WriteFile(sentinelPath, []byte(payload), 0600); err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write sentinel: "+err.Error()))
			return
		}

		ctx.Respond(true, map[string]interface{}{
			"ok":                true,
			"action":            "sentinel-written",
			"scheduleRestartMs": int64(scheduleRestartMs),
			"currentVersion":    resolveCurrentVersion(),
		}, nil)
	}
}

// ---------- 辅助 ----------

func resolveCurrentVersion() string {
	v := preferredGatewayEnvValue("CRABCLAW_VERSION", "OPENACOSMI_VERSION")
	if v != "" {
		return v
	}
	return "dev"
}

func resolveDefaultStateDir() string {
	dir := preferredGatewayEnvValue("CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR")
	if dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "openacosmi")
}

func resolveProjectRoot() string {
	dir := preferredGatewayEnvValue("CRABCLAW_PROJECT_ROOT", "OPENACOSMI_PROJECT_ROOT")
	if dir != "" {
		return dir
	}
	// 回退到可执行文件所在目录
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
