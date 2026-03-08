// views/media-manage.ts — 媒体运营管理页面（独立侧栏入口）
// 轻苹果风壳层 + 子 tab 分发，保留现有媒体控制器与业务面板。

import { html, nothing, type TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { t } from "../i18n.ts";
import { openMediaManageWindow } from "../media-manage-window.ts";
import { pathForTab } from "../navigation.ts";
import {
  renderConfigPanel,
  renderPatrolPanel,
  renderProgressBanner,
  renderHeartbeatPanel,
  renderTrendingPanel,
  renderDraftsPanel,
  renderPublishPanel,
  renderDraftDetailModal,
  renderPublishDetailModal,
  renderDraftEditModal,
} from "./media-dashboard.ts";
import {
  toggleMediaTool,
  toggleMediaSource,
  updateMediaConfig,
  type MediaSourceInfo,
  type MediaToolInfo,
} from "../controllers/media-dashboard.ts";

export type MediaSubTab = "overview" | "llm" | "sources" | "tools" | "strategy" | "drafts" | "publish" | "patrol";

const MEDIA_SUBTAB_QUERY = "mediaSubTab";

const SUB_TABS: { id: MediaSubTab; labelKey: string }[] = [
  { id: "overview", labelKey: "media.subtab.overview" },
  { id: "llm", labelKey: "media.subtab.llm" },
  { id: "sources", labelKey: "media.subtab.sources" },
  { id: "tools", labelKey: "media.subtab.tools" },
  { id: "strategy", labelKey: "media.subtab.strategy" },
  { id: "drafts", labelKey: "media.subtab.drafts" },
  { id: "publish", labelKey: "media.subtab.publish" },
  { id: "patrol", labelKey: "media.subtab.patrol" },
];

const SUB_TAB_IDS = new Set<MediaSubTab>(SUB_TABS.map((tab) => tab.id));

const TOOL_ICONS: Record<string, string> = {
  trending_topics: "📊",
  content_compose: "✍️",
  media_publish: "🚀",
  social_interact: "💬",
  web_search: "🔍",
  report_progress: "📢",
};

const TOOL_LABELS: Record<string, string> = {
  trending_topics: "热点发现",
  content_compose: "内容创作",
  media_publish: "多平台发布",
  social_interact: "社交互动",
  web_search: "网页搜索",
  report_progress: "进度汇报",
};

const TOGGLEABLE_TOOLS = new Set(["media_publish", "social_interact"]);

const ALL_SOURCES = ["weibo", "baidu", "zhihu"] as const;

const SOURCE_LABELS: Record<string, string> = {
  weibo: "微博热搜",
  baidu: "百度热搜",
  zhihu: "知乎热榜",
};

function isToolEnabled(tool: Pick<MediaToolInfo, "enabled" | "status">): boolean {
  return tool.enabled !== false && tool.status !== "disabled";
}

function isSourceEnabled(source: Pick<MediaSourceInfo, "enabled" | "status">): boolean {
  if (typeof source.enabled === "boolean") {
    return source.enabled;
  }
  return source.status !== "disabled";
}

function toolStatusMeta(tool: Pick<MediaToolInfo, "enabled" | "status" | "configured" | "scope">): {
  label: string;
  chipClass: string;
} {
  if (!isToolEnabled(tool)) {
    return { label: "未启用", chipClass: "chip-muted" };
  }
  switch (tool.status) {
    case "configured":
      return { label: "已配置", chipClass: "chip-ok" };
    case "needs_configuration":
      return { label: "待配置", chipClass: "chip-warn" };
    case "builtin":
      return { label: "核心能力", chipClass: "chip-muted" };
    default:
      if (tool.scope === "shared") {
        return { label: "运行时提供", chipClass: "chip-muted" };
      }
      return { label: "已启用", chipClass: "chip-muted" };
  }
}

function isMediaSubTab(value: string | null | undefined): value is MediaSubTab {
  return Boolean(value && SUB_TAB_IDS.has(value as MediaSubTab));
}

function normalizeMediaSubTab(value: string | null | undefined): MediaSubTab {
  return isMediaSubTab(value) ? value : "overview";
}

function readMediaSubTabFromUrl(): MediaSubTab | null {
  if (typeof window === "undefined") {
    return null;
  }
  const value = new URL(window.location.href).searchParams.get(MEDIA_SUBTAB_QUERY);
  return isMediaSubTab(value) ? value : null;
}

function syncMediaSubTabInUrl(basePath: string, tab: MediaSubTab) {
  if (typeof window === "undefined") {
    return;
  }
  const url = new URL(window.location.href);
  url.pathname = pathForTab("media", basePath);
  if (tab === "overview") {
    url.searchParams.delete(MEDIA_SUBTAB_QUERY);
  } else {
    url.searchParams.set(MEDIA_SUBTAB_QUERY, tab);
  }
  window.history.replaceState({}, "", url.toString());
}

export function buildMediaManageUrl(basePath: string, tab: MediaSubTab = "overview"): string {
  const targetPath = pathForTab("media", basePath);
  if (typeof window === "undefined") {
    return tab === "overview"
      ? targetPath
      : `${targetPath}?${MEDIA_SUBTAB_QUERY}=${encodeURIComponent(tab)}`;
  }
  const url = new URL(targetPath, window.location.href);
  if (tab !== "overview") {
    url.searchParams.set(MEDIA_SUBTAB_QUERY, tab);
  }
  return url.toString();
}

function formatTimestamp(timestamp: number | null | undefined): string {
  if (!timestamp) {
    return "—";
  }
  try {
    return new Intl.DateTimeFormat(undefined, {
      month: "numeric",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(timestamp);
  } catch {
    return new Date(timestamp).toLocaleString();
  }
}

function renderTagList(items: string[], emptyLabel = "—"): TemplateResult {
  if (items.length === 0) {
    return html`<span class="media-manage__meta-empty">${emptyLabel}</span>`;
  }
  return html`
    <div class="media-manage__tag-list">
      ${items.map((item) => html`<span class="media-manage__tag">${item}</span>`)}
    </div>
  `;
}

export function renderMediaManage(state: AppViewState): TemplateResult {
  const activeSubTab = readMediaSubTabFromUrl() ?? normalizeMediaSubTab(state.mediaManageSubTab);
  const config = state.mediaConfig;
  const isConfigured = config?.status === "configured";
  const toolCount = (config?.tools ?? []).filter((tool: MediaToolInfo) =>
    tool.scope === "shared" ? tool.enabled !== false : isToolEnabled(tool),
  ).length;
  const sourceCount = config?.enabled_sources?.length
    ?? (config?.trending_sources ?? []).filter((source: MediaSourceInfo) => isSourceEnabled(source)).length;
  const draftCount = state.mediaDrafts?.length ?? 0;
  const publisherCount = config?.publishers?.length ?? 0;
  const provider = config?.llm?.provider || "—";
  const model = config?.llm?.model || "—";
  const autoSpawnCount = state.mediaHeartbeat?.autoSpawnCount ?? 0;
  const activeSubTabLabel = t(SUB_TABS.find((tab) => tab.id === activeSubTab)?.labelKey || "media.subtab.overview");

  const setSubTab = (tab: MediaSubTab) => {
    state.mediaManageSubTab = tab;
    syncMediaSubTabInUrl(state.basePath, tab);
    (state as { requestUpdate?: () => void }).requestUpdate?.();
  };

  const configureLlm = () => setSubTab("llm");

  return html`
    <section class="media-manage">
      <div class="media-manage__shell">
        <div class="media-manage__hero">
          <div class="media-manage__hero-copy">
            <div class="media-manage__eyebrow">${t("nav.tab.media")}</div>
            <div class="media-manage__headline-row">
              <div>
                <h1 class="media-manage__headline">${t("media.manage.title")}</h1>
                <p class="media-manage__lede">${t("media.manage.subtitle")}</p>
              </div>
              <span class="media-manage__hero-badge ${isConfigured ? "is-ready" : "is-warning"}">
                <span class="media-manage__hero-badge-dot"></span>
                ${isConfigured ? t("media.manage.statusReady") : t("media.manage.statusSetup")}
              </span>
            </div>

            <p class="media-manage__summary">${t("media.manage.summary")}</p>

            <div class="media-manage__hero-actions">
              <button class="btn primary" @click=${configureLlm}>
                ${t("media.manage.configureModel")}
              </button>
              <button
                class="btn"
                @click=${() => {
                  void openMediaManageWindow(buildMediaManageUrl(state.basePath, activeSubTab), activeSubTab, "media-page");
                }}
              >
                ${t("media.manage.openWindow")}
              </button>
            </div>
          </div>

          <div class="media-manage__hero-panel">
            <div class="media-manage__hero-metrics">
              <div class="media-manage__hero-metric">
                <span>${t("media.subtab.tools")}</span>
                <strong>${toolCount}</strong>
              </div>
              <div class="media-manage__hero-metric">
                <span>${t("media.subtab.sources")}</span>
                <strong>${sourceCount}</strong>
              </div>
              <div class="media-manage__hero-metric">
                <span>${t("media.subtab.drafts")}</span>
                <strong>${draftCount}</strong>
              </div>
              <div class="media-manage__hero-metric">
                <span>${t("media.manage.publishers")}</span>
                <strong>${publisherCount}</strong>
              </div>
            </div>

            <div class="media-manage__hero-facts">
              <div class="media-manage__hero-fact">
                <span>${t("media.subtab.llm")}</span>
                <strong>${provider}</strong>
                <small>${model}</small>
              </div>
              <div class="media-manage__hero-fact">
                <span>${t("media.manage.nextPatrol")}</span>
                <strong>${formatTimestamp(state.mediaHeartbeat?.nextPatrolAt)}</strong>
                <small>${t("media.heartbeat.lastPatrol")} ${formatTimestamp(state.mediaHeartbeat?.lastPatrolAt)}</small>
              </div>
              <div class="media-manage__hero-fact">
                <span>${t("media.heartbeat.autoSpawnCount")}</span>
                <strong>${autoSpawnCount}</strong>
                <small>${t("media.manage.currentWindow")} · ${activeSubTabLabel}</small>
              </div>
            </div>
          </div>
        </div>

        <div class="media-manage__subtabs" role="tablist" aria-label=${t("media.manage.title")}>
          ${SUB_TABS.map(
            (tab) => html`
              <button
                class="media-manage__subtab ${activeSubTab === tab.id ? "is-active" : ""}"
                @click=${() => setSubTab(tab.id)}
              >
                ${t(tab.labelKey)}
              </button>
            `,
          )}
        </div>

        <div class="media-manage__content">
          ${dispatchSubTab(activeSubTab, state, {
            configureLlm,
            openInWindow: () => openMediaManageWindow(
              buildMediaManageUrl(state.basePath, activeSubTab),
              activeSubTab,
              "media-page",
            ),
          })}
        </div>
      </div>
    </section>

    ${renderDraftDetailModal(state)}
    ${renderPublishDetailModal(state)}
    ${renderDraftEditModal(state)}
  `;
}

function dispatchSubTab(
  tab: MediaSubTab,
  state: AppViewState,
  actions: {
    configureLlm: () => void;
    openInWindow: () => void;
  },
): TemplateResult | typeof nothing {
  switch (tab) {
    case "overview":
      return renderOverviewTab(state, actions);
    case "llm":
      return html`<div class="media-manage__panel-stack">${renderConfigPanel(state)}</div>`;
    case "sources":
      return renderSourcesTab(state);
    case "tools":
      return renderToolsTab(state);
    case "strategy":
      return renderStrategyTab(state);
    case "drafts":
      return html`<div class="media-manage__panel-stack">${renderDraftsPanel(state)}</div>`;
    case "publish":
      return html`<div class="media-manage__panel-stack">${renderPublishPanel(state)}</div>`;
    case "patrol":
      return renderPatrolTab(state);
    default:
      return nothing;
  }
}

function renderOverviewTab(
  state: AppViewState,
  actions: {
    configureLlm: () => void;
    openInWindow: () => void;
  },
): TemplateResult {
  const config = state.mediaConfig;
  const isConfigured = config?.status === "configured";
  const toolNames = (config?.tools ?? [])
    .filter((tool: MediaToolInfo) => tool.scope !== "shared" && isToolEnabled(tool))
    .map((tool: MediaToolInfo) => TOOL_LABELS[tool.name] || tool.name);
  const sourceNames = (config?.enabled_sources?.length
    ? (config.enabled_sources as string[])
    : (config?.trending_sources ?? [])
        .filter((source: MediaSourceInfo) => isSourceEnabled(source))
        .map((source: MediaSourceInfo) => source.name))
    .map((name: string) => SOURCE_LABELS[name] || name);
  const publishers = config?.publishers ?? [];

  return html`
    <div class="media-manage__overview">
      <div class="media-manage__overview-grid">
        <section class="media-glass-card media-glass-card--highlight">
          <div class="media-glass-card__eyebrow">${t("media.manage.runbook")}</div>
          <h2 class="media-glass-card__title">
            ${isConfigured ? t("media.manage.statusReady") : t("media.manage.statusSetup")}
          </h2>
          <p class="media-glass-card__body">
            ${isConfigured
              ? `${config?.llm?.provider || "LLM"} · ${config?.llm?.model || "—"}`
              : t("media.overview.notConfigured")}
          </p>
          <div class="media-glass-card__actions">
            <button class="btn primary" @click=${actions.configureLlm}>
              ${t("media.manage.configureModel")}
            </button>
            <button class="btn" @click=${actions.openInWindow}>
              ${t("media.manage.openWindow")}
            </button>
          </div>
        </section>

        ${renderOverviewStat(t("media.subtab.tools"), String(config?.tools?.length ?? 0))}
        ${renderOverviewStat(t("media.subtab.sources"), String(config?.trending_sources?.length ?? 0))}
        ${renderOverviewStat(t("media.subtab.drafts"), String(state.mediaDrafts?.length ?? 0))}
        ${renderOverviewStat(t("media.heartbeat.autoSpawnCount"), String(state.mediaHeartbeat?.autoSpawnCount ?? 0))}
      </div>

      <div class="media-manage__cockpit">
        <section class="media-glass-card">
          <div class="media-glass-card__eyebrow">${t("media.manage.toolsReady")}</div>
          <div class="media-glass-card__body">
            ${renderTagList(toolNames)}
          </div>
        </section>

        <section class="media-glass-card">
          <div class="media-glass-card__eyebrow">${t("media.manage.sourceMix")}</div>
          <div class="media-glass-card__body">
            ${renderTagList(sourceNames)}
          </div>
        </section>

        <section class="media-glass-card">
          <div class="media-glass-card__eyebrow">${t("media.manage.outputLane")}</div>
          <div class="media-glass-card__body">
            ${renderTagList(publishers)}
          </div>
        </section>
      </div>

      ${!isConfigured
        ? html`
            <div class="callout" style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;">
              <span>${t("media.overview.notConfigured")}</span>
              <button class="btn primary" @click=${actions.configureLlm}>
                ${t("media.overview.configureNow")}
              </button>
            </div>
          `
        : nothing}

      <div class="media-manage__panel-stack">
        ${renderProgressBanner(state)}
        ${renderHeartbeatPanel(state.mediaHeartbeat)}
      </div>
    </div>
  `;
}

function renderOverviewStat(label: string, value: string): TemplateResult {
  return html`
    <section class="media-glass-card media-glass-card--stat">
      <div class="media-glass-card__eyebrow">${label}</div>
      <div class="media-glass-card__value">${value}</div>
    </section>
  `;
}


function renderToolsTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  if (!config) {
    return html`<div class="muted">${t("common.loading")}</div>`;
  }

  const mediaTools = (config.tools || []).filter((tool: { scope?: string }) => tool.scope !== "shared");
  const sharedTools = (config.tools || []).filter((tool: { scope?: string }) => tool.scope === "shared");

  const renderToolCard = (tool: MediaToolInfo, toggleable: boolean) => {
    const enabled = isToolEnabled(tool);
    const badge = toolStatusMeta(tool);
    return html`
      <section class="media-manage__toggle-card ${enabled ? "" : "is-muted"}">
        <div class="media-manage__toggle-icon">${TOOL_ICONS[tool.name] || "🔧"}</div>
        <div class="media-manage__toggle-copy">
          <div class="media-manage__toggle-title-row">
            <strong>${TOOL_LABELS[tool.name] || tool.name}</strong>
            <span class="chip ${badge.chipClass}">
              ${badge.label}
            </span>
          </div>
          <p>${tool.description || TOOL_LABELS[tool.name] || tool.name}</p>

          ${toggleable
            ? html`
                <label class="media-manage__switch-row">
                  <input
                    type="checkbox"
                    .checked=${enabled}
                    @change=${(event: Event) => {
                      const checked = (event.target as HTMLInputElement).checked;
                      void toggleMediaTool(state, tool.name, checked);
                    }}
                  />
                  <span>${enabled ? "启用中" : "已关闭"}</span>
                </label>
              `
            : html`
                <div class="media-manage__switch-row media-manage__switch-row--static">
                  <span>${tool.scope === "shared"
                    ? "共享能力，随运行环境自动提供"
                    : tool.status === "builtin"
                      ? "核心能力，默认可用，不依赖单独账号配置"
                      : tool.status === "needs_configuration"
                        ? "需要先补齐媒体账号或发布目标"
                        : "能力已接入，可直接参与媒体流程"}</span>
                </div>
              `}
        </div>
      </section>
    `;
  };

  return html`
    <div class="media-manage__panel-stack">
      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">${t("media.subtab.tools")}</div>
            <div class="media-glass-card__body">${t("media.manage.summary")}</div>
          </div>
        </div>

        <div class="media-manage__collection-grid">
          ${mediaTools.map((tool: MediaToolInfo) =>
            renderToolCard(tool, TOGGLEABLE_TOOLS.has(tool.name)),
          )}
        </div>
      </section>

      ${sharedTools.length > 0
        ? html`
            <section class="media-glass-card">
              <div class="media-glass-card__header">
                <div>
                  <div class="media-glass-card__eyebrow">共享工具</div>
                  <div class="media-glass-card__body">这些能力由主运行时提供，媒体子智能体会按需自动调用。</div>
                </div>
              </div>
              <div class="media-manage__collection-grid">
                ${sharedTools.map((tool: MediaToolInfo) =>
                  renderToolCard(tool, false),
                )}
              </div>
            </section>
          `
        : nothing}
    </div>
  `;
}

function renderSourcesTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  if (!config) {
    return html`<div class="muted">${t("common.loading")}</div>`;
  }

  const registeredSources = Array.isArray(config.enabled_sources)
    ? config.enabled_sources
    : (config.trending_sources || [])
        .filter((source: MediaSourceInfo) => isSourceEnabled(source))
        .map((source: MediaSourceInfo) => source.name);
  const hasExplicitConfig = config.enabled_sources_configured === true;
  const allEnabled = !hasExplicitConfig;

  return html`
    <div class="media-manage__panel-stack">
      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">${t("media.subtab.sources")}</div>
            <div class="media-glass-card__body">${t("media.manage.sourcesHint")}</div>
          </div>
        </div>

        <div class="media-manage__collection-grid">
          ${ALL_SOURCES.map((name) => {
            const source = (config.trending_sources || []).find((entry: MediaSourceInfo) => entry.name === name);
            const enabled = source ? isSourceEnabled(source) : (allEnabled || registeredSources.includes(name));
            return html`
              <label class="media-manage__source-card ${enabled ? "" : "is-muted"}">
                <div class="media-manage__source-card-head">
                  <strong>${SOURCE_LABELS[name] || name}</strong>
                  <input
                    type="checkbox"
                    .checked=${enabled}
                    @change=${(event: Event) => {
                      const checked = (event.target as HTMLInputElement).checked;
                      void toggleMediaSource(state, name, checked);
                    }}
                  />
                </div>
                <span>${enabled
                  ? hasExplicitConfig ? "当前参与热点抓取" : "默认启用，尚未显式配置"
                  : hasExplicitConfig ? "已从抓取名单移除" : "当前未启用"}</span>
              </label>
            `;
          })}
        </div>
      </section>

      ${renderTrendingPanel(state)}
    </div>
  `;
}

function renderStrategyTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  const strategy = config?.trending_strategy;
  const hotKeywords = strategy?.hotKeywords ?? [];
  const monitorInterval = strategy?.monitorIntervalMin ?? 30;
  const threshold = strategy?.trendingThreshold ?? 10000;
  const categories = strategy?.contentCategories ?? [];
  const autoDraft = strategy?.autoDraftEnabled ?? false;

  return html`
    <div style="display:flex;flex-direction:column;gap:16px;">
      <div class="card media-strategy-panel">
        <div class="media-section-title" style="margin-bottom:12px;">热点策略配置</div>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">热度阈值（低于此值的话题将被跳过）</span>
          <input
            type="number"
            class="media-strategy-input"
            .value=${String(threshold)}
            @change=${(e: Event) => {
              const v = parseFloat((e.target as HTMLInputElement).value);
              if (!isNaN(v) && v >= 0) void updateMediaConfig(state, { trendingThreshold: v });
            }}
          />
        </label>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">监控频率（分钟）</span>
          <input
            type="number"
            class="media-strategy-input"
            min="5"
            max="1440"
            .value=${String(monitorInterval)}
            @change=${(e: Event) => {
              const v = parseInt((e.target as HTMLInputElement).value, 10);
              if (!isNaN(v) && v >= 5) void updateMediaConfig(state, { monitorIntervalMin: v });
            }}
          />
        </label>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">自定义关键词（逗号分隔）</span>
          <input
            type="text"
            class="media-strategy-input media-strategy-input--wide"
            .value=${hotKeywords.join(", ")}
            @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              const keywords = v.split(",").map(s => s.trim()).filter(Boolean);
              void updateMediaConfig(state, { hotKeywords: keywords });
            }}
            placeholder="例如: AI, 科技, 创业"
          />
        </label>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">内容领域偏好（逗号分隔）</span>
          <input
            type="text"
            class="media-strategy-input media-strategy-input--wide"
            .value=${categories.join(", ")}
            @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              const cats = v.split(",").map(s => s.trim()).filter(Boolean);
              void updateMediaConfig(state, { contentCategories: cats });
            }}
            placeholder="例如: 科技, 教育, 商业"
          />
        </label>

        <label style="display:flex;align-items:center;gap:8px;cursor:pointer;font-size:13px;">
          <input
            type="checkbox"
            .checked=${autoDraft}
            @change=${(e: Event) => {
              const checked = (e.target as HTMLInputElement).checked;
              void updateMediaConfig(state, { autoDraftEnabled: checked });
            }}
          />
          自动生成草稿（发现匹配热点时自动创建内容草稿）
        </label>
      </div>
    </div>
  `;
}

function renderPatrolTab(state: AppViewState): TemplateResult {
  return html`
    <div class="media-manage__panel-stack">
      ${renderPatrolPanel(state)}
      ${renderProgressBanner(state)}
      ${renderHeartbeatPanel(state.mediaHeartbeat)}
    </div>
  `;
}
