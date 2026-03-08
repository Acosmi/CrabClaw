import { html, nothing, type TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { t } from "../i18n.ts";
import {
  closeDraftDetail,
  closePublishDetail,
  closeDraftEdit,
  updateDraft,
} from "../controllers/media-dashboard.ts";

type MediaTone = "ok" | "warn" | "danger" | "info" | "muted";

const STATUS_FLOW = ["draft", "pending_review", "approved", "published"] as const;
const STATUS_LABELS: Record<string, string> = {
  draft: "草稿",
  pending_review: "待审批",
  approved: "已审批",
  published: "已发布",
};
const STATUS_COLORS: Record<string, string> = {
  draft: "#6b7280",
  pending_review: "#f59e0b",
  approved: "#3b82f6",
  published: "#22c55e",
};

const OVERLAY_STYLE = "position:fixed;inset:0;background:rgba(15,23,42,0.42);display:flex;align-items:center;justify-content:center;z-index:1000;padding:24px;backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);";
const MODAL_STYLE = "background:linear-gradient(180deg,rgba(255,255,255,0.12) 0%,rgba(255,255,255,0.03) 100%), color-mix(in srgb, var(--card) 94%, transparent);border:1px solid color-mix(in srgb, var(--border-strong) 88%, transparent);border-radius:28px;max-width:720px;width:min(92vw,720px);max-height:85vh;overflow-y:auto;box-shadow:var(--shadow-xl), inset 0 1px 0 var(--card-highlight);";

function toneChipClass(tone: MediaTone): string {
  switch (tone) {
    case "ok":
      return "chip-ok";
    case "warn":
      return "chip-warn";
    case "danger":
      return "chip-danger";
    case "info":
      return "media-dashboard__chip--info";
    default:
      return "chip-muted";
  }
}

function renderStatusChip(label: string, tone: MediaTone, dense = false): TemplateResult {
  return html`
    <span class="chip ${toneChipClass(tone)} ${dense ? "media-dashboard__chip--dense" : ""}">
      ${label}
    </span>
  `;
}

function renderModalCloseButton(onClose: () => void): TemplateResult {
  return html`
    <button type="button" class="media-dashboard-modal__close" @click=${onClose} aria-label="Close">
      &times;
    </button>
  `;
}

function renderLoadingModal(onClose: () => void): TemplateResult {
  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${onClose}>
      <div class="modal-content media-dashboard-modal" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <div class="media-dashboard-modal__loading">Loading…</div>
      </div>
    </div>
  `;
}

function renderStatusFlow(currentStatus: string): TemplateResult {
  const activeIndex = Math.max(STATUS_FLOW.indexOf(currentStatus as typeof STATUS_FLOW[number]), 0);
  return html`
    <div class="media-dashboard-flow">
      ${STATUS_FLOW.map((status, index) => html`
        ${index > 0
          ? html`
              <div
                class="media-dashboard-flow__line ${index <= activeIndex ? "is-active" : ""}"
                style=${index <= activeIndex ? `--flow-color:${STATUS_COLORS[status]};` : ""}
              ></div>
            `
          : nothing}
        <div class="media-dashboard-flow__step">
          <div
            class="media-dashboard-flow__dot ${index <= activeIndex ? "is-active" : ""}"
            style=${index <= activeIndex ? `--flow-color:${STATUS_COLORS[status]};` : ""}
          ></div>
          <span
            class="media-dashboard-flow__label ${index <= activeIndex ? "is-active" : ""}"
            style=${index <= activeIndex ? `--flow-color:${STATUS_COLORS[status]};` : ""}
          >
            ${STATUS_LABELS[status]}
          </span>
        </div>
      `)}
    </div>
  `;
}

function publishStatusMeta(status: string): { label: string; tone: MediaTone } {
  if (status === "published") {
    return { label: "已发布", tone: "ok" };
  }
  if (status === "failed") {
    return { label: "失败", tone: "danger" };
  }
  return { label: status, tone: "warn" };
}

export function renderDraftDetailModal(state: AppViewState): TemplateResult | typeof nothing {
  const draft = state.mediaDraftDetail;
  if (!draft) return nothing;
  const loading = state.mediaDraftDetailLoading;
  const close = () => {
    closeDraftDetail(state);
    state.requestUpdate();
  };

  if (loading) {
    return renderLoadingModal(close);
  }

  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${close}>
      <div class="modal-content media-dashboard-modal" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <header class="media-dashboard-modal__header">
          <div class="media-dashboard-modal__title-block">
            <div class="media-dashboard-modal__eyebrow">草稿详情</div>
            <h2 class="media-dashboard-modal__title">${draft.title || "(untitled)"}</h2>
            <div class="media-dashboard-modal__meta">
              <span class="pill">${draft.platform}</span>
              ${draft.style ? html`<span class="pill">${draft.style}</span>` : nothing}
              ${renderStatusChip(
                STATUS_LABELS[draft.status] || draft.status,
                draft.status === "published"
                  ? "ok"
                  : draft.status === "approved"
                    ? "info"
                    : draft.status === "pending_review"
                      ? "warn"
                      : "muted",
              )}
            </div>
          </div>
          ${renderModalCloseButton(close)}
        </header>

        <section class="media-dashboard-modal__section">
          <div class="media-dashboard-modal__section-title">审批流程</div>
          ${renderStatusFlow(draft.status)}
        </section>

        <section class="media-dashboard-modal__section">
          <div class="media-dashboard-modal__section-title">正文</div>
          ${draft.body
            ? html`<div class="media-dashboard-modal__content">${draft.body}</div>`
            : html`<div class="media-dashboard-empty">无正文内容</div>`}
        </section>

        ${draft.tags && draft.tags.length > 0
          ? html`
              <section class="media-dashboard-modal__section">
                <div class="media-dashboard-modal__section-title">标签</div>
                <div class="media-dashboard-tag-list">
                  ${draft.tags.map((tag: string) => html`<span class="pill">#${tag}</span>`)}
                </div>
              </section>
            `
          : nothing}

        ${draft.images && draft.images.length > 0
          ? html`
              <section class="media-dashboard-modal__section">
                <div class="media-dashboard-modal__section-title">图片 (${draft.images.length})</div>
                <div class="media-dashboard-modal__images">
                  ${draft.images.map((url: string) => html`
                    <img src=${url} class="media-dashboard-modal__image" alt="draft image" />
                  `)}
                </div>
              </section>
            `
          : nothing}

        <footer class="media-dashboard-modal__footer">
          <span>创建: ${new Date(draft.created_at).toLocaleString()}</span>
          <span>更新: ${new Date(draft.updated_at).toLocaleString()}</span>
          <span class="media-dashboard-modal__mono">ID: ${draft.id}</span>
        </footer>
      </div>
    </div>
  `;
}

export function renderPublishDetailModal(state: AppViewState): TemplateResult | typeof nothing {
  const record = state.mediaPublishDetail;
  if (!record) return nothing;
  const loading = state.mediaPublishDetailLoading;
  const close = () => {
    closePublishDetail(state);
    state.requestUpdate();
  };

  if (loading) {
    return renderLoadingModal(close);
  }

  const status = publishStatusMeta(record.status);

  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${close}>
      <div class="modal-content media-dashboard-modal" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <header class="media-dashboard-modal__header">
          <div class="media-dashboard-modal__title-block">
            <div class="media-dashboard-modal__eyebrow">发布详情</div>
            <h2 class="media-dashboard-modal__title">${record.title || "(untitled)"}</h2>
            <div class="media-dashboard-modal__meta">
              <span class="pill">${record.platform}</span>
              ${renderStatusChip(status.label, status.tone)}
            </div>
          </div>
          ${renderModalCloseButton(close)}
        </header>

        <section class="media-dashboard-modal__section">
          <div class="media-dashboard-modal__section-title">发布信息</div>
          <div class="media-dashboard-modal__kv">
            ${record.post_id
              ? html`
                  <div class="media-dashboard-modal__kv-row">
                    <span>Post ID</span>
                    <strong class="media-dashboard-modal__mono">${record.post_id}</strong>
                  </div>
                `
              : nothing}
            ${record.url
              ? html`
                  <div class="media-dashboard-modal__kv-row">
                    <span>链接</span>
                    <a href=${record.url} target="_blank" rel="noopener" class="media-dashboard-row__link">${record.url}</a>
                  </div>
                `
              : nothing}
            ${record.published_at
              ? html`
                  <div class="media-dashboard-modal__kv-row">
                    <span>发布时间</span>
                    <strong>${new Date(record.published_at).toLocaleString()}</strong>
                  </div>
                `
              : nothing}
            <div class="media-dashboard-modal__kv-row">
              <span>草稿 ID</span>
              <strong class="media-dashboard-modal__mono">${record.draft_id}</strong>
            </div>
          </div>
        </section>

        <footer class="media-dashboard-modal__footer">
          <span class="media-dashboard-modal__mono">ID: ${record.id}</span>
        </footer>
      </div>
    </div>
  `;
}

export function renderDraftEditModal(state: AppViewState): TemplateResult | typeof nothing {
  const draft = state.mediaDraftEdit;
  if (!draft) return nothing;
  const close = () => {
    closeDraftEdit(state);
    state.requestUpdate();
  };

  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${close}>
      <div class="modal-content media-dashboard-modal" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <header class="media-dashboard-modal__header">
          <div class="media-dashboard-modal__title-block">
            <div class="media-dashboard-modal__eyebrow">草稿编辑</div>
            <h2 class="media-dashboard-modal__title">编辑草稿</h2>
            <div class="media-dashboard-modal__sub">修改标题、正文、平台与标签，保存后会直接回写草稿库。</div>
          </div>
          ${renderModalCloseButton(close)}
        </header>

        <div class="media-dashboard-modal__form">
          <label class="field media-dashboard-field media-dashboard-modal__form-span">
            <span>标题</span>
            <input
              type="text"
              .value=${draft.title || ""}
              @input=${(e: Event) => {
                draft.title = (e.target as HTMLInputElement).value;
              }}
            />
          </label>

          <label class="field media-dashboard-field media-dashboard-modal__form-span">
            <span>正文</span>
            <textarea
              .value=${draft.body || ""}
              @input=${(e: Event) => {
                draft.body = (e.target as HTMLTextAreaElement).value;
              }}
              rows="8"
            ></textarea>
          </label>

          <label class="field media-dashboard-field">
            <span>平台</span>
            <select
              .value=${draft.platform || ""}
              @change=${(e: Event) => {
                draft.platform = (e.target as HTMLSelectElement).value;
              }}
            >
              <option value="xiaohongshu">小红书</option>
              <option value="wechat">微信</option>
              <option value="website">Website</option>
            </select>
          </label>

          <label class="field media-dashboard-field">
            <span>标签 (逗号分隔)</span>
            <input
              type="text"
              .value=${(draft.tags || []).join(", ")}
              @input=${(e: Event) => {
                draft.tags = (e.target as HTMLInputElement).value
                  .split(",")
                  .map((tag: string) => tag.trim())
                  .filter(Boolean);
              }}
              placeholder="标签1, 标签2"
            />
          </label>
        </div>

        <footer class="media-dashboard-modal__footer media-dashboard-modal__footer--actions">
          <button class="btn" @click=${close}>取消</button>
          <button
            class="btn primary"
            @click=${() => {
              void updateDraft(state, draft.id, {
                title: draft.title,
                body: draft.body,
                platform: draft.platform,
                tags: draft.tags,
              });
            }}
          >
            保存
          </button>
        </footer>
      </div>
    </div>
  `;
}
