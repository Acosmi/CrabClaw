import { html, nothing } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { t } from "../i18n.ts";

/**
 * 渲染 Argus 灵瞳故障 Toast 提醒。
 * 固定在屏幕底部，附带一键重启按钮。
 */
export function renderArgusFailureAlert(state: AppViewState) {
    const alert = state.argusFailureAlert;
    if (!alert?.visible) {
        return nothing;
    }

    const handleRestart = async () => {
        if (!state.client || alert.restarting) {
            return;
        }
        state.argusFailureAlert = { ...alert, restarting: true, error: null };
        (state as any).requestUpdate?.();
        try {
            await state.client.request("subagent.ctl", {
                agent_id: "argus-screen",
                action: "set_enabled",
                value: true,
            });
            // 成功 → 清除 alert（argus.status.changed ready 事件也会清除，双保险）
            state.argusFailureAlert = null;
            if (typeof state.addNotification === "function") {
                state.addNotification(t("argus.alert.restartSuccess"), "success");
            }
        } catch (err: any) {
            const message = err instanceof Error ? err.message : String(err);
            state.argusFailureAlert = {
                ...alert,
                restarting: false,
                error: t("argus.alert.restartFailed", { error: message }),
            };
        }
        (state as any).requestUpdate?.();
    };

    const handleDismiss = () => {
        state.argusFailureAlert = null;
        (state as any).requestUpdate?.();
    };

    return html`
    <div class="argus-failure-toast" role="alert">
      <div class="argus-toast-icon">👁️</div>
      <div class="argus-toast-body">
        <div class="argus-toast-title">${t("argus.alert.title")}</div>
        <div class="argus-toast-reason">${alert.reason}</div>
        ${alert.error
            ? html`<div class="argus-toast-error">${alert.error}</div>`
            : nothing}
      </div>
      <div class="argus-toast-actions">
        <button
          class="argus-toast-btn argus-toast-btn--restart"
          ?disabled=${alert.restarting}
          @click=${handleRestart}
        >
          ${alert.restarting
            ? t("argus.alert.restarting")
            : t("argus.alert.restart")}
        </button>
        <button
          class="argus-toast-btn argus-toast-btn--dismiss"
          @click=${handleDismiss}
        >
          ${t("argus.alert.dismiss")}
        </button>
      </div>
    </div>
  `;
}
