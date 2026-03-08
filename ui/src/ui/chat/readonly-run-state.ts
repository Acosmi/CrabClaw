export type ChatUxMode = "classic" | "codex-readonly";

export type ReadonlyRunPhase =
  | "idle"
  | "starting"
  | "working"
  | "drafting"
  | "finalizing"
  | "aborted"
  | "error";

export type ReadonlyToolStepStatus = "running" | "done" | "error";

export type ReadonlyToolStep = {
  toolCallId: string;
  name: string;
  status: ReadonlyToolStepStatus;
  startedAt: number;
  updatedAt: number;
  outputPreview: string | null;
};

export type ReadonlyRunActivity =
  | {
    id: string;
    kind: "phase";
    phase: Exclude<ReadonlyRunPhase, "idle">;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "progress";
    summary: string;
    progressPhase: string | null;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "tool";
    toolCallId: string;
    name: string;
    status: ReadonlyToolStepStatus;
    phase: string;
    outputPreview: string | null;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "draft";
    text: string;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "error";
    message: string;
    ts: number;
    updatedAt: number;
  };

export type ChatReadonlyRunState = {
  runId: string | null;
  sessionKey: string;
  phase: ReadonlyRunPhase;
  startedAt: number | null;
  updatedAt: number | null;
  latestProgress: string | null;
  progressPhase: string | null;
  draftingText: string | null;
  lastToolName: string | null;
  toolSteps: ReadonlyToolStep[];
  activity: ReadonlyRunActivity[];
  lastError: string | null;
};

export type ChatReadonlyRunHost = {
  sessionKey: string;
  chatReadonlyRun: ChatReadonlyRunState;
};

type RunBindingInput = {
  runId: string;
  sessionKey?: string | null;
  ts: number;
};

type ProgressInput = RunBindingInput & {
  summary: string;
  phase?: string | null;
};

type ToolInput = RunBindingInput & {
  toolCallId: string;
  name: string;
  phase: string;
  isError?: boolean;
  output?: string | null;
};

type ChatInput = {
  runId: string;
  sessionKey: string;
  state: "delta" | "final" | "aborted" | "error";
  ts: number;
  text?: string | null;
  errorMessage?: string | null;
};

const READONLY_TOOL_STEP_LIMIT = 8;
const READONLY_ACTIVITY_LIMIT = 12;
const READONLY_OUTPUT_PREVIEW_LIMIT = 240;

export function createChatReadonlyRunState(sessionKey: string): ChatReadonlyRunState {
  return {
    runId: null,
    sessionKey,
    phase: "idle",
    startedAt: null,
    updatedAt: null,
    latestProgress: null,
    progressPhase: null,
    draftingText: null,
    lastToolName: null,
    toolSteps: [],
    activity: [],
    lastError: null,
  };
}

export function isReadonlyRunVisible(
  run: ChatReadonlyRunState | null | undefined,
  sessionKey: string,
): boolean {
  if (!run) {
    return false;
  }
  if (run.sessionKey !== sessionKey) {
    return false;
  }
  if (run.phase === "idle") {
    return false;
  }
  return run.runId !== null || run.phase === "aborted" || run.phase === "error";
}

export function isReadonlyRunActive(run: ChatReadonlyRunState | null | undefined): boolean {
  if (!run) {
    return false;
  }
  if (!run.runId) {
    return false;
  }
  return (
    run.phase === "starting" ||
    run.phase === "working" ||
    run.phase === "drafting" ||
    run.phase === "finalizing"
  );
}

export function resetChatReadonlyRun(
  host: ChatReadonlyRunHost,
  sessionKey: string = host.sessionKey,
) {
  host.chatReadonlyRun = createChatReadonlyRunState(sessionKey);
}

export function syncChatReadonlyRunSession(host: ChatReadonlyRunHost) {
  const current = host.chatReadonlyRun;
  if (current.runId !== null) {
    return;
  }
  if (current.phase === "aborted" || current.phase === "error") {
    return;
  }
  if (current.sessionKey === host.sessionKey) {
    return;
  }
  host.chatReadonlyRun = createChatReadonlyRunState(host.sessionKey);
}

export function startChatReadonlyRun(
  host: ChatReadonlyRunHost,
  runId: string,
  startedAt: number,
  sessionKey: string = host.sessionKey,
) {
  host.chatReadonlyRun = {
    runId,
    sessionKey,
    phase: "starting",
    startedAt,
    updatedAt: startedAt,
    latestProgress: null,
    progressPhase: null,
    draftingText: null,
    lastToolName: null,
    toolSteps: [],
    activity: [createPhaseActivity("starting", startedAt)],
    lastError: null,
  };
}

export function setChatReadonlyRunTerminal(
  host: ChatReadonlyRunHost,
  phase: "aborted" | "error",
  opts?: { runId?: string | null; sessionKey?: string | null; ts?: number; errorMessage?: string | null },
) {
  const ts = opts?.ts ?? Date.now();
  const nextSessionKey = opts?.sessionKey?.trim() || host.chatReadonlyRun.sessionKey || host.sessionKey;
  const currentRunId = host.chatReadonlyRun.runId;
  const targetRunId = opts?.runId?.trim();
  if (targetRunId && currentRunId && targetRunId !== currentRunId) {
    return;
  }
  const message =
    phase === "error"
      ? opts?.errorMessage?.trim() || host.chatReadonlyRun.lastError || null
      : null;
  let activity = upsertActivity(host.chatReadonlyRun.activity, createPhaseActivity(phase, ts));
  if (message) {
    activity = upsertActivity(activity, createErrorActivity(message, ts));
  }
  host.chatReadonlyRun = {
    ...host.chatReadonlyRun,
    runId: null,
    sessionKey: nextSessionKey,
    phase,
    updatedAt: ts,
    activity,
    lastError: message,
  };
}

export function updateChatReadonlyRunFromLifecycle(
  host: ChatReadonlyRunHost,
  input: RunBindingInput & { phase: string },
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, input);
  if (!current) {
    return;
  }

  if (input.phase === "start") {
    host.chatReadonlyRun = {
      ...current,
      phase: current.draftingText?.trim() ? "drafting" : "starting",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("starting", input.ts)),
    };
    return;
  }

  if (input.phase === "end") {
    host.chatReadonlyRun = {
      ...current,
      phase: "finalizing",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("finalizing", input.ts)),
    };
    return;
  }

  if (input.phase === "error") {
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "error",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("error", input.ts)),
    };
  }
}

export function updateChatReadonlyRunFromProgress(
  host: ChatReadonlyRunHost,
  input: ProgressInput,
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, input);
  if (!current) {
    return;
  }

  const summary = input.summary.trim();
  host.chatReadonlyRun = {
    ...current,
    phase: resolveActivePhase(current, "working"),
    updatedAt: input.ts,
    latestProgress: summary,
    progressPhase: input.phase?.trim() || null,
    activity: appendProgressActivity(current.activity, summary, input.phase?.trim() || null, input.ts),
  };
}

export function updateChatReadonlyRunFromTool(
  host: ChatReadonlyRunHost,
  input: ToolInput,
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, input);
  if (!current) {
    return;
  }

  const toolSteps = upsertToolStep(current.toolSteps, input);
  const step = toolSteps.find((item) => item.toolCallId === input.toolCallId) ?? null;
  host.chatReadonlyRun = {
    ...current,
    phase: resolveActivePhase(current, "working"),
    updatedAt: input.ts,
    lastToolName: input.name,
    toolSteps,
    activity: step
      ? upsertToolActivity(current.activity, step, input.phase, input.ts)
      : current.activity,
  };
}

export function updateChatReadonlyRunFromChat(
  host: ChatReadonlyRunHost,
  input: ChatInput,
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, {
    runId: input.runId,
    sessionKey: input.sessionKey,
    ts: input.ts,
  });
  if (!current) {
    return;
  }

  if (input.state === "delta") {
    const nextText = input.text?.trim() ? input.text : current.draftingText;
    const activity = nextText?.trim()
      ? upsertDraftActivity(current.activity, nextText, input.ts)
      : current.activity;
    host.chatReadonlyRun = {
      ...current,
      phase: nextText?.trim() ? "drafting" : current.phase,
      updatedAt: input.ts,
      draftingText: nextText ?? null,
      activity,
    };
    return;
  }

  if (input.state === "final") {
    host.chatReadonlyRun = createChatReadonlyRunState(input.sessionKey);
    return;
  }

  if (input.state === "aborted") {
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "aborted",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("aborted", input.ts)),
    };
    return;
  }

  if (input.state === "error") {
    const message = input.errorMessage?.trim() || current.lastError;
    let activity = upsertActivity(current.activity, createPhaseActivity("error", input.ts));
    if (message) {
      activity = upsertActivity(activity, createErrorActivity(message, input.ts));
    }
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "error",
      updatedAt: input.ts,
      lastError: message,
      activity,
    };
  }
}

function bindReadonlyRun(
  current: ChatReadonlyRunState,
  input: RunBindingInput,
): ChatReadonlyRunState | null {
  if (!input.runId) {
    return null;
  }
  if (current.runId && current.runId !== input.runId) {
    return null;
  }

  const nextSessionKey = input.sessionKey?.trim() || current.sessionKey;
  if (!current.runId) {
    return {
      ...createChatReadonlyRunState(nextSessionKey),
      runId: input.runId,
      sessionKey: nextSessionKey,
      phase: "starting",
      startedAt: input.ts,
      updatedAt: input.ts,
      activity: [createPhaseActivity("starting", input.ts)],
    };
  }

  return {
    ...current,
    sessionKey: nextSessionKey,
    startedAt: current.startedAt ?? input.ts,
    activity: current.activity ?? [],
  };
}

function resolveActivePhase(
  current: ChatReadonlyRunState,
  fallback: "working" | "drafting",
): "working" | "drafting" | "finalizing" {
  if (current.phase === "finalizing") {
    return "finalizing";
  }
  if (current.draftingText?.trim()) {
    return "drafting";
  }
  return fallback;
}

function upsertToolStep(
  steps: ReadonlyToolStep[],
  input: ToolInput,
): ReadonlyToolStep[] {
  const startedAt = input.ts;
  const outputPreview = truncatePreview(input.output);
  const nextStatus = resolveToolStepStatus(input.phase, Boolean(input.isError));
  const nextSteps = [...steps];
  const existingIndex = nextSteps.findIndex((item) => item.toolCallId === input.toolCallId);

  if (existingIndex === -1) {
    nextSteps.push({
      toolCallId: input.toolCallId,
      name: input.name,
      status: nextStatus,
      startedAt,
      updatedAt: input.ts,
      outputPreview,
    });
    return trimToolSteps(nextSteps);
  }

  const existing = nextSteps[existingIndex];
  nextSteps[existingIndex] = {
    ...existing,
    name: input.name,
    status: nextStatus,
    updatedAt: input.ts,
    outputPreview: outputPreview ?? existing.outputPreview,
  };
  return trimToolSteps(nextSteps);
}

function trimToolSteps(steps: ReadonlyToolStep[]): ReadonlyToolStep[] {
  if (steps.length <= READONLY_TOOL_STEP_LIMIT) {
    return steps;
  }
  return steps.slice(steps.length - READONLY_TOOL_STEP_LIMIT);
}

function resolveToolStepStatus(
  phase: string,
  isError: boolean,
): ReadonlyToolStepStatus {
  if (phase === "result") {
    return isError ? "error" : "done";
  }
  return "running";
}

function createPhaseActivity(
  phase: Exclude<ReadonlyRunPhase, "idle">,
  ts: number,
): ReadonlyRunActivity {
  return {
    id: `phase:${phase}`,
    kind: "phase",
    phase,
    ts,
    updatedAt: ts,
  };
}

function createErrorActivity(message: string, ts: number): ReadonlyRunActivity {
  return {
    id: "error:terminal",
    kind: "error",
    message,
    ts,
    updatedAt: ts,
  };
}

function appendProgressActivity(
  activity: ReadonlyRunActivity[],
  summary: string,
  progressPhase: string | null,
  ts: number,
): ReadonlyRunActivity[] {
  if (!summary) {
    return activity;
  }
  const next = [...activity];
  const last = next[next.length - 1];
  if (last?.kind === "progress" && last.summary === summary) {
    next[next.length - 1] = {
      ...last,
      progressPhase,
      updatedAt: ts,
    };
    return trimActivity(next);
  }
  next.push({
    id: `progress:${ts}`,
    kind: "progress",
    summary,
    progressPhase,
    ts,
    updatedAt: ts,
  });
  return trimActivity(next);
}

function upsertToolActivity(
  activity: ReadonlyRunActivity[],
  step: ReadonlyToolStep,
  phase: string,
  ts: number,
): ReadonlyRunActivity[] {
  const id = `tool:${step.toolCallId}`;
  const next: ReadonlyRunActivity = {
    id,
    kind: "tool",
    toolCallId: step.toolCallId,
    name: step.name,
    status: step.status,
    phase,
    outputPreview: step.outputPreview,
    ts: step.startedAt,
    updatedAt: ts,
  };
  return upsertActivity(activity, next);
}

function upsertDraftActivity(
  activity: ReadonlyRunActivity[],
  text: string,
  ts: number,
): ReadonlyRunActivity[] {
  const existing = activity.find((item) => item.kind === "draft");
  return upsertActivity(activity, {
    id: "draft:current",
    kind: "draft",
    text,
    ts: existing?.kind === "draft" ? existing.ts : ts,
    updatedAt: ts,
  });
}

function upsertActivity(
  activity: ReadonlyRunActivity[],
  next: ReadonlyRunActivity,
): ReadonlyRunActivity[] {
  const items = [...activity];
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) {
    items.push(next);
    return trimActivity(items);
  }
  items[index] = next;
  return trimActivity(items);
}

function trimActivity(activity: ReadonlyRunActivity[]): ReadonlyRunActivity[] {
  if (activity.length <= READONLY_ACTIVITY_LIMIT) {
    return activity;
  }
  return activity.slice(activity.length - READONLY_ACTIVITY_LIMIT);
}

function truncatePreview(text: string | null | undefined): string | null {
  const value = text?.trim();
  if (!value) {
    return null;
  }
  if (value.length <= READONLY_OUTPUT_PREVIEW_LIMIT) {
    return value;
  }
  return `${value.slice(0, READONLY_OUTPUT_PREVIEW_LIMIT - 1)}…`;
}
