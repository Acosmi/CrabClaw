/**
 * Email Channel Card — renders email account status, test connection, and zero-config guidance.
 *
 * Pattern: follows channels.telegram.ts
 */
import { html, nothing } from "lit";
import type { ChannelAccountSnapshot } from "../types.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { formatRelativeTimestamp } from "../format.ts";
import { t } from "../i18n.ts";

/** Provider display labels */
const PROVIDER_LABELS: Record<string, string> = {
    aliyun: "阿里企业邮箱",
    qq: "QQ 邮箱",
    tencent_exmail: "企业微信邮箱",
    netease163: "网易 163",
};

/** Map runner states to i18n display */
function stateLabel(state?: string): string {
    if (!state) return "n/a";
    const key = `channels.email.state.${state}`;
    const label = t(key);
    return label !== key ? label : state;
}

export function renderEmailCard(params: {
    props: ChannelsProps;
    emailAccounts: ChannelAccountSnapshot[];
    accountCountLabel: unknown;
    onTestConnection?: (accountId: string) => void;
    emailTestLoading?: boolean;
    emailTestResult?: EmailTestResult | null;
}) {
    const { props, emailAccounts, accountCountLabel, onTestConnection, emailTestLoading, emailTestResult } = params;
    const hasAccounts = emailAccounts.length > 0;

    // ─── Zero-config guidance ───
    if (!hasAccounts) {
        return html`
      <div class="callout" style="margin-top: 8px;">
        <div style="font-weight: 600; margin-bottom: 6px;">${t("channels.email.noAccounts")}</div>
        <div style="font-size: 13px; line-height: 1.6; color: var(--text-muted, #888);">
          ${t("channels.email.guideSummary")}
          <ul style="margin: 8px 0 4px 0; padding-left: 18px; font-size: 12px;">
            <li><strong>${PROVIDER_LABELS.aliyun}</strong> — ${t("channels.email.guideAliyun")}</li>
            <li><strong>${PROVIDER_LABELS.qq}</strong> — ${t("channels.email.guideQQ")}</li>
            <li><strong>${PROVIDER_LABELS.tencent_exmail}</strong> — ${t("channels.email.guideExmail")}</li>
            <li><strong>${PROVIDER_LABELS.netease163}</strong> — ${t("channels.email.guideNetease")}</li>
          </ul>
          <div style="margin-top: 6px; font-size: 12px;">${t("channels.email.guideAction")}</div>
        </div>
      </div>
    `;
    }

    // ─── Account cards ───
    const renderAccountCard = (account: ChannelAccountSnapshot) => {
        const provider = (account as Record<string, unknown>).provider as string | undefined;
        const address = (account as Record<string, unknown>).address as string | undefined;
        const state = (account as Record<string, unknown>).state as string | undefined;
        const consecutiveFailures = (account as Record<string, unknown>).consecutiveFailures as number | undefined;
        const label = account.name || address || account.accountId;
        const providerLabel = provider ? (PROVIDER_LABELS[provider] ?? provider) : "";
        const accountId = account.accountId;
        const isTestingThis = emailTestLoading && emailTestResult === null;

        return html`
      <div class="account-card">
        <div class="account-card-header">
          <div class="account-card-title">${label}</div>
          <div class="account-card-id">
            ${providerLabel ? html`<span style="margin-right: 6px; opacity: 0.7;">${providerLabel}</span>` : nothing}
            ${accountId}
          </div>
        </div>
        <div class="status-list account-card-status">
          <div>
            <span class="label">${t("channels.email.state")}</span>
            <span>${stateLabel(state)}</span>
          </div>
          <div>
            <span class="label">${t("channels.running")}</span>
            <span>${account.running ? t("channels.yes") : t("channels.no")}</span>
          </div>
          <div>
            <span class="label">${t("channels.configured")}</span>
            <span>${account.configured ? t("channels.yes") : t("channels.no")}</span>
          </div>
          <div>
            <span class="label">${t("channels.lastInbound")}</span>
            <span>${account.lastInboundAt ? formatRelativeTimestamp(account.lastInboundAt) : "n/a"}</span>
          </div>
          ${consecutiveFailures && consecutiveFailures > 0
                ? html`
              <div>
                <span class="label">${t("channels.email.failures")}</span>
                <span style="color: var(--danger, #e53e3e);">${consecutiveFailures}</span>
              </div>
            `
                : nothing
            }
          ${account.lastError
                ? html`
              <div class="account-card-error">
                ${account.lastError}
              </div>
            `
                : nothing
            }
        </div>
        ${onTestConnection
                ? html`
            <div class="row" style="margin-top: 8px;">
              <button
                class="btn btn--sm"
                ?disabled=${emailTestLoading}
                @click=${(e: Event) => { e.stopPropagation(); onTestConnection(accountId); }}
              >
                ${isTestingThis ? t("channels.email.testing") : t("channels.email.testConnection")}
              </button>
            </div>
          `
                : nothing
            }
      </div>
    `;
    };

    return html`
    ${accountCountLabel}

    <div class="account-card-list">
      ${emailAccounts.map((account) => renderAccountCard(account))}
    </div>

    ${emailTestResult ? renderTestResult(emailTestResult) : nothing}
  `;
}

// ─── Test Result Display ───

export type EmailTestResult = {
    ok: boolean;
    accountId: string;
    address?: string;
    provider?: string;
    imap: { ok: boolean; host: string; latencyMs: number; error?: string };
    smtp: { ok: boolean; host: string; latencyMs: number; error?: string };
};

function renderTestResult(result: EmailTestResult) {
    return html`
    <div class="callout ${result.ok ? "" : "danger"}" style="margin-top: 12px;">
      <div style="font-weight: 600; margin-bottom: 4px;">
        ${result.ok ? t("channels.email.testSuccess") : t("channels.email.testFailed")}
        ${result.address ? html` · <span style="opacity: 0.7;">${result.address}</span>` : nothing}
      </div>
      <div class="status-list" style="font-size: 12px;">
        <div>
          <span class="label">IMAP</span>
          <span>
            ${result.imap.ok ? "✓" : "✗"} ${result.imap.host}
            · ${result.imap.latencyMs}ms
            ${result.imap.error ? html` · <span style="color: var(--danger, #e53e3e);">${result.imap.error}</span>` : nothing}
          </span>
        </div>
        <div>
          <span class="label">SMTP</span>
          <span>
            ${result.smtp.ok ? "✓" : "✗"} ${result.smtp.host}
            · ${result.smtp.latencyMs}ms
            ${result.smtp.error ? html` · <span style="color: var(--danger, #e53e3e);">${result.smtp.error}</span>` : nothing}
          </span>
        </div>
      </div>
    </div>
  `;
}
