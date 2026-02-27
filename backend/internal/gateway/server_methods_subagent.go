package gateway

// server_methods_subagent.go — subagent.list / subagent.ctl
// 子智能体状态查询与控制 RPC。

import (
	"fmt"
	"log/slog"

	"github.com/anthropic/open-acosmi/internal/argus"
)

// SubagentHandlers 返回子智能体方法映射。
func SubagentHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"subagent.list": handleSubagentList,
		"subagent.ctl":  handleSubagentCtl,
	}
}

// ---------- subagent.list ----------

type subagentEntry struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"` // "running" | "stopped" | "error" | "degraded" | "starting"
	Error  string `json:"error,omitempty"`
}

func handleSubagentList(ctx *MethodHandlerContext) {
	var entries []subagentEntry

	// 1. Argus 视觉子智能体
	entries = append(entries, buildArgusEntry(ctx.Context.ArgusBridge))

	// 2. oa-coder 编程子智能体
	entries = append(entries, buildCoderEntry(ctx.Context.CoderConfirmMgr != nil))

	ctx.Respond(true, map[string]interface{}{
		"agents": entries,
	}, nil)
}

func buildArgusEntry(bridge *argus.Bridge) subagentEntry {
	entry := subagentEntry{
		ID:    "argus-screen",
		Label: "Vision Observer",
	}
	if bridge == nil {
		entry.Status = "stopped"
		entry.Error = "Argus binary not available"
		return entry
	}
	state := bridge.State()
	switch state {
	case argus.BridgeStateReady:
		entry.Status = "running"
	case argus.BridgeStateDegraded:
		entry.Status = "degraded"
	case argus.BridgeStateStarting:
		entry.Status = "starting"
	default:
		entry.Status = "stopped"
	}
	return entry
}

func buildCoderEntry(confirmMgrAvailable bool) subagentEntry {
	entry := subagentEntry{
		ID:    "oa-coder",
		Label: "Coder Agent",
	}
	// oa-coder 是按需 spawn 的 LLM session，不是持久进程。
	// 只要 CoderConfirmMgr 已初始化，说明 coder 子系统可用。
	if confirmMgrAvailable {
		entry.Status = "running"
	} else {
		entry.Status = "stopped"
	}
	return entry
}

// ---------- subagent.ctl ----------

func handleSubagentCtl(ctx *MethodHandlerContext) {
	agentID, _ := ctx.Params["agent_id"].(string)
	action, _ := ctx.Params["action"].(string)
	if agentID == "" || action == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agent_id and action required"))
		return
	}

	switch agentID {
	case "argus-screen":
		handleArgusCtl(ctx, action)
	case "oa-coder":
		handleCoderCtl(ctx, action)
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown agent: %s", agentID)))
	}
}

func handleArgusCtl(ctx *MethodHandlerContext, action string) {
	bridge := ctx.Context.ArgusBridge
	switch action {
	case "set_enabled":
		enabled, _ := ctx.Params["value"].(bool)
		if enabled {
			if bridge == nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "Argus binary not available"))
				return
			}
			state := bridge.State()
			if state == argus.BridgeStateReady || state == argus.BridgeStateStarting {
				ctx.Respond(true, map[string]interface{}{"ok": true, "state": "already_running"}, nil)
				return
			}
			if err := bridge.Start(); err != nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "argus start failed: "+err.Error()))
				return
			}
			slog.Info("subagent.ctl: argus started via UI")
			ctx.Respond(true, map[string]interface{}{"ok": true, "state": string(bridge.State())}, nil)
		} else {
			if bridge != nil {
				bridge.Stop()
				slog.Info("subagent.ctl: argus stopped via UI")
			}
			ctx.Respond(true, map[string]interface{}{"ok": true, "state": "stopped"}, nil)
		}

	case "set_interval_ms", "set_goal", "set_vla_model":
		// 这些设置目前由前端本地管理，后端仅 ACK。
		// 未来可持久化到配置或转发给 Argus 进程。
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action}, nil)

	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown action for argus: %s", action)))
	}
}

func handleCoderCtl(ctx *MethodHandlerContext, action string) {
	switch action {
	case "set_enabled":
		// oa-coder 是按需 spawn 的子智能体，enable/disable 仅影响前端 UI 状态。
		// 实际的 coder 启停由 agent run 时自动控制。
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": "set_enabled"}, nil)

	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown action for oa-coder: %s", action)))
	}
}
