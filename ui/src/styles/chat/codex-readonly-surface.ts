import { html, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import { t } from "../i18n.ts";
import type { ChatReadonlyRunState, ReadonlyToolStep } from "./readonly-run-state.ts";

function formatElapsed(startedAt: number | null): string {
  if (!startedAt) {
    return "0:00";
  }
  const totalSeconds = Math.max(0, Math.floor((Date.now() - startedAt) / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function phaseLabel(run: ChatReadonlyRunState): string {
  switch (run.phase) {
    case "starting":
      return t("chat.readonly.phase.starting");
    case "working":
      return t("chat.readonly.phase.working");
    case "drafting":
      return t("chat.readonly.phase.drafting");
    case "finalizing":
      return t("chat.readonly.phase.finalizing");
    case "aborted":
      return t("chat.readonly.phase.aborted");
    case "error":
      return t("chat.readonly.phase.error");
    default:
      return t("chat.readonly.phase.starting");
  }
}

function stepStatusLabel(step: ReadonlyToolStep): string {
  if (step.status === "error") {
    return t("chat.readonly.step.error");
  }
  if (step.status === "done") {
    return t("chat.readonly.step.done");
  }
  return t("chat.readonly.step.running");
}

function renderToolStep(step: ReadonlyToolStep) {
  return html`
    <li class="chat-readonly-run__step chat-readonly-run__step--${step.status}">
      <div class="chat-readonly-run__step-main">
        <span class="chat-readonly-run__step-dot" aria-hidden="true"></span>
        <span class="chat-readonly-run__step-name">${step.name}</span>
        <span class="chat-readonly-run__step-status">${stepStatusLabel(step)}</span>
      </div>
      ${step.outputPreview
        ? html`<div class="chat-readonly-run__step-detail">${step.outputPreview}</div>`
        : nothing}
    </li>
  `;
}

export function renderCodexReadonlySurface(run: ChatReadonlyRunState) {
  const summary =
    run.latestProgress?.trim() ||
    (run.phase === "starting"
      ? t("chat.readonly.empty.starting")
      : run.phase === "drafting"
        ? t("chat.readonly.empty.drafting")
        : run.phase === "finalizing"
          ? t("chat.readonly.empty.finalizing")
          : t("chat.readonly.empty.working"));

  const steps = run.toolSteps.slice(-5);
  const hasDraftingText = Boolean(run.draftingText?.trim());
  const modifierClass =
    run.phase === "error"
      ? "chat-readonly-run--error"
      : run.phase === "aborted"
        ? "chat-readonly-run--aborted"
        : "";

  return html`
    <div class="chat-group assistant">
      <div class="chat-group-messages">
        <section
          class="chat-readonly-run ${modifierClass}"
          aria-label=${t("chat.readonly.title")}
        >
          <div class="chat-readonly-run__sr-status" role="status" aria-live="polite" aria-atomic="true">
            ${phaseLabel(run)}. ${summary}
          </div>
          <div class="chat-readonly-run__header">
            <div class="chat-readonly-run__eyebrow">${t("chat.readonly.title")}</div>
            <div class="chat-readonly-run__topline">
              <span class="chat-readonly-run__phase">${phaseLabel(run)}</span>
              <span class="chat-readonly-run__timer" aria-hidden="true">
                ${t("chat.processingTime", { time: formatElapsed(run.startedAt) })}
              </span>
            </div>
            <div class="chat-readonly-run__summary">${summary}</div>
            ${run.lastError?.trim()
              ? html`<div class="chat-readonly-run__error">${run.lastError}</div>`
              : nothing}
          </div>

          <div class="chat-readonly-run__body">
            ${steps.length
              ? html`
                  <div class="chat-readonly-run__section">
                    <div class="chat-readonly-run__section-title">
                      ${t("chat.readonly.stepsLabel")}
                    </div>
                    <ol class="chat-readonly-run__steps">
                      ${repeat(steps, (step) => step.toolCallId, (step) => renderToolStep(step))}
                    </ol>
                  </div>
                `
              : nothing}

            ${hasDraftingText
              ? html`
                  <div class="chat-readonly-run__section">
                    <div class="chat-readonly-run__section-title">
                      ${t("chat.readonly.draftLabel")}
                    </div>
                    <div class="chat-readonly-run__draft">
                      ${run.draftingText}
                    </div>
                  </div>
                `
              : nothing}
          </div>
        </section>
      </div>
    </div>
  `;
}
