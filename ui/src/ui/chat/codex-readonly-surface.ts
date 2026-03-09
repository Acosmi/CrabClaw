import { html, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import { t } from "../i18n.ts";
import { resolveToolDisplay } from "../tool-display.ts";
import type {
  ChatReadonlyRunState,
  ReadonlyRunActivity,
  ReadonlyRunPhase,
  ReadonlyToolStep,
} from "./readonly-run-state.ts";

function formatElapsed(startedAt: number | null, completedAt: number | null): string {
  if (!startedAt) {
    return "0:00";
  }
  const endAt = completedAt ?? Date.now();
  const totalSeconds = Math.max(0, Math.floor((endAt - startedAt) / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function phaseLabel(phase: ReadonlyRunPhase): string {
  switch (phase) {
    case "starting":
      return t("chat.readonly.phase.starting");
    case "working":
      return t("chat.readonly.phase.working");
    case "drafting":
      return t("chat.readonly.phase.drafting");
    case "finalizing":
      return t("chat.readonly.phase.finalizing");
    case "complete":
      return t("chat.readonly.phase.complete");
    case "aborted":
      return t("chat.readonly.phase.aborted");
    case "error":
      return t("chat.readonly.phase.error");
    default:
      return t("chat.readonly.phase.starting");
  }
}

function stepStatusLabel(status: ReadonlyToolStep["status"]): string {
  if (status === "error") {
    return t("chat.readonly.step.error");
  }
  if (status === "done") {
    return t("chat.readonly.step.done");
  }
  return t("chat.readonly.step.running");
}

function isLivePhase(phase: ReadonlyRunPhase): boolean {
  return phase !== "idle" && phase !== "complete" && phase !== "aborted" && phase !== "error";
}

function statusLabel(phase: ReadonlyRunPhase): string {
  if (phase === "complete") {
    return t("chat.readonly.status.complete");
  }
  if (phase === "aborted") {
    return t("chat.readonly.status.aborted");
  }
  if (phase === "error") {
    return t("chat.readonly.status.error");
  }
  return t("chat.readonly.status.live");
}

function summarizeTool(name: string, detail: string | null): string {
  const display = resolveToolDisplay({ name, meta: detail ?? undefined });
  return display.detail ? `${display.title} · ${display.detail}` : display.title;
}

function classifyToolMetric(name: string): "files" | "searches" | "skills" | "edits" | "commands" | null {
  const normalized = name.trim().toLowerCase();
  if (["read", "read_file", "coder_read", "list_dir", "ls"].includes(normalized)) {
    return "files";
  }
  if (["search", "grep", "glob", "find", "coder_grep", "coder_glob", "web_search", "memory_search"].includes(normalized)) {
    return "searches";
  }
  if (["search_skills", "lookup_skill"].includes(normalized)) {
    return "skills";
  }
  if (["write", "write_file", "edit", "coder_edit", "coder_write", "apply_patch"].includes(normalized)) {
    return "edits";
  }
  if (["bash", "coder_bash", "process"].includes(normalized)) {
    return "commands";
  }
  return null;
}

function formatMetric(key: "files" | "searches" | "skills" | "edits" | "commands" | "tools", count: number): string {
  switch (key) {
    case "files":
      return t("chat.readonly.summary.files", { count });
    case "searches":
      return t("chat.readonly.summary.searches", { count });
    case "skills":
      return t("chat.readonly.summary.skills", { count });
    case "edits":
      return t("chat.readonly.summary.edits", { count });
    case "commands":
      return t("chat.readonly.summary.commands", { count });
    default:
      return t("chat.readonly.summary.tools", { count });
  }
}

function buildSummary(run: ChatReadonlyRunState): string | null {
  if (run.toolSteps.length === 0) {
    return null;
  }

  const metrics = {
    files: 0,
    searches: 0,
    skills: 0,
    edits: 0,
    commands: 0,
    tools: run.toolSteps.length,
  };
  for (const step of run.toolSteps) {
    const key = classifyToolMetric(step.name);
    if (!key) {
      continue;
    }
    metrics[key] += 1;
  }

  const ordered: Array<keyof typeof metrics> = ["files", "searches", "skills", "edits", "commands"];
  const parts = ordered
    .filter((key) => metrics[key] > 0)
    .map((key) => formatMetric(key, metrics[key]));
  if (parts.length === 0) {
    parts.push(formatMetric("tools", metrics.tools));
  }
  return parts.slice(0, 4).join("，");
}

function buildAnnouncement(run: ChatReadonlyRunState, activity: ReadonlyRunActivity[]): string {
  const latest = activity[activity.length - 1];
  if (!latest) {
    return fallbackPhaseNarration(run.phase);
  }
  if (latest.kind === "progress") {
    return `${phaseLabel(run.phase)}. ${latest.summary}`;
  }
  if (latest.kind === "tool") {
    return `${phaseLabel(run.phase)}. ${summarizeTool(latest.name, latest.detail)}`;
  }
  if (latest.kind === "draft") {
    return `${phaseLabel(run.phase)}. ${t("chat.readonly.activity.phase.drafting")}`;
  }
  if (latest.kind === "error") {
    return `${phaseLabel(run.phase)}. ${latest.message}`;
  }
  return `${phaseLabel(run.phase)}. ${phaseNarration(latest.phase)}`;
}

function buildActivities(run: ChatReadonlyRunState): ReadonlyRunActivity[] {
  if (run.activity.length > 0) {
    return run.activity.slice(-8);
  }

  const fallback: ReadonlyRunActivity[] = [];
  if (run.phase !== "idle") {
    fallback.push({
      id: `phase:${run.phase}`,
      kind: "phase",
      phase: run.phase === "idle" ? "starting" : run.phase,
      ts: run.startedAt ?? Date.now(),
      updatedAt: run.updatedAt ?? run.startedAt ?? Date.now(),
    });
  }
  if (run.latestProgress?.trim()) {
    fallback.push({
      id: "progress:fallback",
      kind: "progress",
      summary: run.latestProgress.trim(),
      progressPhase: run.progressPhase,
      ts: run.updatedAt ?? run.startedAt ?? Date.now(),
      updatedAt: run.updatedAt ?? run.startedAt ?? Date.now(),
    });
  }
  for (const step of run.toolSteps.slice(-4)) {
    fallback.push({
      id: `tool:${step.toolCallId}`,
      kind: "tool",
      toolCallId: step.toolCallId,
      name: step.name,
      detail: step.detail,
      status: step.status,
      phase: step.status === "done" ? "result" : "start",
      outputPreview: step.outputPreview,
      ts: step.startedAt,
      updatedAt: step.updatedAt,
    });
  }
  if (run.draftingText?.trim()) {
    fallback.push({
      id: "draft:fallback",
      kind: "draft",
      text: run.draftingText.trim(),
      ts: run.updatedAt ?? run.startedAt ?? Date.now(),
      updatedAt: run.updatedAt ?? run.startedAt ?? Date.now(),
    });
  }
  if (run.lastError?.trim()) {
    fallback.push({
      id: "error:fallback",
      kind: "error",
      message: run.lastError.trim(),
      ts: run.updatedAt ?? run.startedAt ?? Date.now(),
      updatedAt: run.updatedAt ?? run.startedAt ?? Date.now(),
    });
  }
  return fallback.slice(-8);
}

function fallbackPhaseNarration(phase: ReadonlyRunPhase): string {
  switch (phase) {
    case "starting":
      return t("chat.readonly.empty.starting");
    case "drafting":
      return t("chat.readonly.empty.drafting");
    case "finalizing":
      return t("chat.readonly.empty.finalizing");
    case "complete":
      return t("chat.readonly.empty.complete");
    case "aborted":
      return t("chat.readonly.activity.phase.aborted");
    case "error":
      return t("chat.readonly.activity.phase.error");
    default:
      return t("chat.readonly.empty.working");
  }
}

function phaseNarration(phase: Exclude<ReadonlyRunPhase, "idle">): string {
  switch (phase) {
    case "starting":
      return t("chat.readonly.activity.phase.starting");
    case "working":
      return t("chat.readonly.activity.phase.working");
    case "drafting":
      return t("chat.readonly.activity.phase.drafting");
    case "finalizing":
      return t("chat.readonly.activity.phase.finalizing");
    case "complete":
      return t("chat.readonly.activity.phase.complete");
    case "aborted":
      return t("chat.readonly.activity.phase.aborted");
    case "error":
      return t("chat.readonly.activity.phase.error");
  }
}

function toolNarration(name: string, status: ReadonlyToolStep["status"]): string {
  if (status === "error") {
    return t("chat.readonly.activity.tool.error", { tool: name });
  }
  if (status === "done") {
    return t("chat.readonly.activity.tool.done", { tool: name });
  }
  return t("chat.readonly.activity.tool.running", { tool: name });
}

function renderActivityRow(activity: ReadonlyRunActivity, index: number) {
  let label = "";
  let text: string | null = null;
  let detail = nothing;
  let modifier = "neutral";

  if (activity.kind === "phase") {
    label = phaseLabel(activity.phase);
    text = phaseNarration(activity.phase);
    modifier =
      activity.phase === "error"
        ? "danger"
        : activity.phase === "aborted"
          ? "muted"
          : "accent";
  } else if (activity.kind === "progress") {
    label = t("chat.readonly.activity.progressLabel");
    text = activity.summary;
    modifier = "neutral";
  } else if (activity.kind === "tool") {
    label = summarizeTool(activity.name, activity.detail);
    text = `${toolNarration(activity.name, activity.status)} · ${stepStatusLabel(activity.status)}`;
    modifier = activity.status === "error" ? "danger" : activity.status === "done" ? "success" : "tool";
    detail = activity.outputPreview
      ? html`<div class="chat-readonly-run__entry-detail">${activity.outputPreview}</div>`
      : nothing;
  } else if (activity.kind === "draft") {
    label = t("chat.readonly.activity.draftLabel");
    modifier = "draft";
    detail = html`
      <div class="chat-readonly-run__entry-text chat-readonly-run__entry-text--draft">
        ${activity.text}
      </div>
    `;
  } else if (activity.kind === "error") {
    label = t("chat.readonly.activity.errorLabel");
    text = activity.message;
    modifier = "danger";
  }

  return html`
    <li
      class="chat-readonly-run__entry chat-readonly-run__entry--${modifier}"
      style=${`--readonly-delay:${index * 36}ms`}
    >
      <span class="chat-readonly-run__marker" aria-hidden="true"></span>
      <div class="chat-readonly-run__entry-body">
        <div class="chat-readonly-run__entry-label">${label}</div>
        ${text ? html`<div class="chat-readonly-run__entry-text">${text}</div>` : nothing}
        ${detail}
      </div>
    </li>
  `;
}

function renderToolChip(step: ReadonlyToolStep) {
  return html`
    <li class="chat-readonly-run__tool-chip chat-readonly-run__tool-chip--${step.status}">
      <span class="chat-readonly-run__tool-chip-status" aria-hidden="true"></span>
      <span class="chat-readonly-run__tool-chip-text">${summarizeTool(step.name, step.detail)}</span>
    </li>
  `;
}

function renderPresenceRow() {
  return html`
    <div class="chat-readonly-run__presence">
      <span class="chat-readonly-run__presence-dots" aria-hidden="true">
        <span></span>
        <span></span>
        <span></span>
      </span>
      <span class="chat-readonly-run__presence-text">${t("chat.readonly.liveTrail")}</span>
    </div>
  `;
}

export function renderCodexReadonlySurface(run: ChatReadonlyRunState) {
  return html`
    <div class="chat-group assistant">
      <div class="chat-group-messages">
        ${renderCodexReadonlyPanel(run)}
      </div>
    </div>
  `;
}

export function renderCodexReadonlyPanel(
  run: ChatReadonlyRunState,
  options?: { replyContent?: unknown },
) {
  const activity = buildActivities(run);
  const summary = buildSummary(run);
  const recentTools = run.toolSteps.slice(-6);
  const replyContent = options?.replyContent ?? nothing;
  const hasReply = replyContent !== nothing;
  const collapseDetails = run.phase === "complete";
  const detailsContent = html`
    ${run.latestProgress?.trim()
      ? html`<div class="chat-readonly-run__latest">${run.latestProgress.trim()}</div>`
      : nothing}

    ${recentTools.length > 0
      ? html`
          <div class="chat-readonly-run__tools">
            <div class="chat-readonly-run__tools-label">${t("chat.readonly.toolsLabel")}</div>
            <ul class="chat-readonly-run__tools-list">
              ${repeat(recentTools, (step) => step.toolCallId, (step) => renderToolChip(step))}
            </ul>
          </div>
        `
      : nothing}

    <ol class="chat-readonly-run__stream">
      ${repeat(activity, (item) => item.id, (item, index) => renderActivityRow(item, index))}
    </ol>

    ${isLivePhase(run.phase) ? renderPresenceRow() : nothing}
  `;
  const modifierClass =
    run.phase === "error"
      ? "chat-readonly-run--error"
      : run.phase === "aborted"
        ? "chat-readonly-run--aborted"
        : run.phase === "complete"
          ? "chat-readonly-run--complete"
        : "";

  return html`
    <section
      class="chat-readonly-run ${modifierClass}"
      aria-label=${t("chat.readonly.title")}
    >
      <div class="chat-readonly-run__sr-status" role="status" aria-live="polite" aria-atomic="true">
        ${buildAnnouncement(run, activity)}
      </div>

      <div class="chat-readonly-run__meta">
        <div class="chat-readonly-run__meta-main">
          <span class="chat-readonly-run__title">${t("chat.readonly.title")}</span>
          <span class="chat-readonly-run__phase-line">
            <span class="chat-readonly-run__phase-dot" aria-hidden="true"></span>
            <span>${phaseLabel(run.phase)}</span>
          </span>
          <span class="chat-readonly-run__status chat-readonly-run__status--${run.phase}">
            ${statusLabel(run.phase)}
          </span>
        </div>
        <span class="chat-readonly-run__timer" aria-hidden="true">
          ${t("chat.processingTime", { time: formatElapsed(run.startedAt, run.completedAt) })}
        </span>
      </div>

      ${summary
    ? html`<div class="chat-readonly-run__summary">${summary}</div>`
    : nothing}

      ${collapseDetails
        ? html`
            <details class="chat-readonly-run__details">
              <summary class="chat-readonly-run__details-summary">
                ${t("chat.readonly.detailsSummary")}
              </summary>
              <div class="chat-readonly-run__details-body">
                ${detailsContent}
              </div>
            </details>
          `
        : detailsContent}

      ${hasReply
        ? html`
            <div class="chat-readonly-run__reply">
              <div class="chat-readonly-run__reply-label">${t("chat.readonly.replyLabel")}</div>
              <div class="chat-readonly-run__reply-body">${replyContent}</div>
            </div>
          `
        : nothing}
    </section>
  `;
}
