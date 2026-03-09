// permission-popup.ts — 权限审批弹窗组件
// 当 AI 工具调用被权限拒绝时，在聊天窗口弹出审批弹窗。
// 参考: Claude Code 3 层规则 + GitHub Copilot 3 级审批

import { html, nothing } from "lit";
import { t } from "../i18n.ts";

/** 权限拒绝事件数据 */
export interface PermissionDeniedEvent {
    tool: string;
    detail: string;
    level: string;
    runId?: string;
}

/** 弹窗操作回调 */
export interface PermissionPopupCallbacks {
    /** 本次放行 */
    onAllowOnce: (event: PermissionDeniedEvent) => void;
    /** 临时授权（会话级） */
    onAllowSession: (event: PermissionDeniedEvent) => void;
    /** 永久授权（修改配置） */
    onAllowPermanent: (event: PermissionDeniedEvent, confirmText: string) => void;
    /** 拒绝 */
    onDeny: () => void;
}

type PopupState = {
    event: PermissionDeniedEvent;
    showConfirm: boolean;
    confirmInput: string;
};

let _state: PopupState | null = null;

/** 显示权限弹窗 */
export function showPermissionPopup(event: PermissionDeniedEvent): void {
    _state = { event, showConfirm: false, confirmInput: "" };
}

/** 隐藏权限弹窗 */
export function hidePermissionPopup(): void {
    _state = null;
}

/** 是否显示中 */
export function isPermissionPopupVisible(): boolean {
    return _state !== null;
}

/** 获取工具描述 */
function toolLabel(tool: string): string {
    switch (tool) {
        case "bash":
            return "bash — " + t("permission.popup.toolBash");
        case "write_file":
            return "write_file — " + t("permission.popup.toolWriteFile");
        default:
            return tool;
    }
}

/** 获取安全级别描述 */
function levelLabel(level: string): string {
    switch (level) {
        case "deny":
            return "L0 " + t("permission.popup.levelDeny");
        case "allowlist":
            return "L1 " + t("permission.popup.levelAllowlist");
        case "sandboxed":
            return "L2 " + t("permission.popup.levelSandboxed");
        case "full":
            return "L3 " + t("permission.popup.levelFull");
        default:
            return level;
    }
}

/** 渲染权限弹窗 */
export function renderPermissionPopup(
    callbacks: PermissionPopupCallbacks,
    requestUpdate: () => void,
) {
    if (!_state) {
        return nothing;
    }

    const state = _state;
    const ev = state.event;
    const permanentOnly = ev.level === "full";

    const handleAllowOnce = () => {
        callbacks.onAllowOnce(ev);
        hidePermissionPopup();
        requestUpdate();
    };

    const handleAllowSession = () => {
        callbacks.onAllowSession(ev);
        hidePermissionPopup();
        requestUpdate();
    };

    const handleShowConfirm = () => {
        state.showConfirm = true;
        state.confirmInput = "";
        requestUpdate();
    };

    const handleConfirmPermanent = () => {
        if (state.confirmInput.trim().toUpperCase() !== "CONFIRM") {
            return;
        }
        callbacks.onAllowPermanent(ev, state.confirmInput);
        hidePermissionPopup();
        requestUpdate();
    };

    const handleDeny = () => {
        callbacks.onDeny();
        hidePermissionPopup();
        requestUpdate();
    };

    const handleOverlayClick = (e: Event) => {
        if ((e.target as HTMLElement).classList.contains("permission-popup-overlay")) {
            handleDeny();
        }
    };

    return html`
    <div class="permission-popup-overlay" @click=${handleOverlayClick}></div>
    <div class="permission-popup" role="alertdialog" aria-modal="true">
      <div class="permission-popup__header">
        <span class="permission-popup__icon">🚫</span>
        <h3 class="permission-popup__title">${t("permission.popup.title")}</h3>
      </div>

      <div class="permission-popup__body">
        <div class="permission-popup__detail">
          <div class="permission-popup__row">
            <span class="permission-popup__label">${t("permission.popup.tool")}</span>
            <span class="permission-popup__value">${toolLabel(ev.tool)}</span>
          </div>
          <div class="permission-popup__row">
            <span class="permission-popup__label">${t("permission.popup.target")}</span>
            <span class="permission-popup__value" title=${ev.detail}>${ev.detail}</span>
          </div>
          <div class="permission-popup__row">
            <span class="permission-popup__label">${t("permission.popup.level")}</span>
            <span class="permission-popup__level">${levelLabel(ev.level)}</span>
          </div>
        </div>
      </div>

      ${state.showConfirm
            ? html`
            <div class="permission-popup__confirm-section">
              <p class="permission-popup__confirm-warn">
                ⚠️ ${t("permission.popup.permanentWarn")}
              </p>
              <input
                class="permission-popup__confirm-input"
                type="text"
                placeholder='${t("permission.popup.typeConfirm")}'
                .value=${state.confirmInput}
                @input=${(e: Event) => {
                    state.confirmInput = (e.target as HTMLInputElement).value;
                    requestUpdate();
                }}
                @keydown=${(e: KeyboardEvent) => {
                    if (e.key === "Enter") {
                        handleConfirmPermanent();
                    }
                }}
              />
              <div class="permission-popup__confirm-actions">
                <button
                  class="permission-popup__btn permission-popup__btn--permanent"
                  ?disabled=${state.confirmInput.trim().toUpperCase() !== "CONFIRM"}
                  @click=${handleConfirmPermanent}
                >
                  ${t("permission.popup.confirmPermanent")}
                </button>
                <button
                  class="permission-popup__btn permission-popup__btn--deny"
                  @click=${() => {
                    state.showConfirm = false;
                    requestUpdate();
                }}
                >
                  ${t("permission.popup.cancel")}
                </button>
              </div>
            </div>
          `
            : html`
            <div class="permission-popup__actions">
              ${permanentOnly ? nothing : html`
                <button
                  class="permission-popup__btn permission-popup__btn--once"
                  @click=${handleAllowOnce}
                >
                  ${t("permission.popup.allowOnce")}
                </button>
                <button
                  class="permission-popup__btn permission-popup__btn--session"
                  @click=${handleAllowSession}
                >
                  ${t("permission.popup.allowSession")}
                </button>
              `}
              <button
                class="permission-popup__btn permission-popup__btn--permanent"
                @click=${handleShowConfirm}
              >
                ${t("permission.popup.allowPermanent")}
              </button>
              <button
                class="permission-popup__btn permission-popup__btn--deny"
                @click=${handleDeny}
              >
                ${t("permission.popup.deny")}
              </button>
            </div>
          `}
    </div>
  `;
}
