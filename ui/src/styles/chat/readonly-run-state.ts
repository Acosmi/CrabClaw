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
  host.chatReadonlyRun = {
    ...host.chatReadonlyRun,
    runId: null,
    sessionKey: nextSessionKey,
    phase,
    updatedAt: ts,
    lastError: phase === "error" ? opts?.errorMessage?.trim() || host.chatReadonlyRun.lastError : null,
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
    };
    return;
  }

  if (input.phase === "end") {
    host.chatReadonlyRun = {
      ...current,
      phase: current.draftingText?.trim() ? "drafting" : "finalizing",
      updatedAt: input.ts,
    };
    return;
  }

  if (input.phase === "error") {
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "error",
      updatedAt: input.ts,
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

  host.chatReadonlyRun = {
    ...current,
    phase: current.draftingText?.trim() ? "drafting" : "working",
    updatedAt: input.ts,
    latestProgress: input.summary.trim(),
    progressPhase: input.phase?.trim() || null,
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
  host.chatReadonlyRun = {
    ...current,
    phase: current.draftingText?.trim() ? "drafting" : "working",
    updatedAt: input.ts,
    lastToolName: input.name,
    toolSteps,
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
    host.chatReadonlyRun = {
      ...current,
      phase: nextText?.trim() ? "drafting" : current.phase,
      updatedAt: input.ts,
      draftingText: nextText ?? null,
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
    };
    return;
  }

  if (input.state === "error") {
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "error",
      updatedAt: input.ts,
      lastError: input.errorMessage?.trim() || current.lastError,
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
    };
  }

  return {
    ...current,
    sessionKey: nextSessionKey,
    startedAt: current.startedAt ?? input.ts,
  };
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
