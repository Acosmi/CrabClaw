import { render } from "lit";
import { describe, expect, it, vi } from "vitest";
import { createChatReadonlyRunState } from "../chat/readonly-run-state.ts";
import type { SessionsListResult } from "../types.ts";
import { renderChat, type ChatProps } from "./chat.ts";

function createSessions(): SessionsListResult {
  return {
    ts: 0,
    path: "",
    count: 0,
    defaults: { model: null, contextTokens: null },
    sessions: [],
  };
}

function createProps(overrides: Partial<ChatProps> = {}): ChatProps {
  return {
    sessionKey: "main",
    onSessionKeyChange: () => undefined,
    thinkingLevel: null,
    showThinking: false,
    loading: false,
    sending: false,
    canAbort: false,
    compactionStatus: null,
    messages: [],
    toolMessages: [],
    uxMode: "classic",
    readonlyRun: createChatReadonlyRunState("main"),
    stream: null,
    streamStartedAt: null,
    assistantAvatarUrl: null,
    draft: "",
    queue: [],
    connected: true,
    canSend: true,
    disabledReason: null,
    error: null,
    sessions: createSessions(),
    focusMode: false,
    assistantName: "OpenAcosmi",
    assistantAvatar: null,
    onRefresh: () => undefined,
    onToggleFocusMode: () => undefined,
    onDraftChange: () => undefined,
    onSend: () => undefined,
    onQueueRemove: () => undefined,
    onNewSession: () => undefined,
    ...overrides,
  };
}

describe("chat view", () => {
  it("renders compacting indicator as a badge", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          compactionStatus: {
            active: true,
            startedAt: Date.now(),
            completedAt: null,
          },
        }),
      ),
      container,
    );

    const indicator = container.querySelector(".compaction-indicator--active");
    expect(indicator).not.toBeNull();
    expect(indicator?.textContent).toContain("Compacting context...");
  });

  it("renders completion indicator shortly after compaction", () => {
    const container = document.createElement("div");
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(1_000);
    render(
      renderChat(
        createProps({
          compactionStatus: {
            active: false,
            startedAt: 900,
            completedAt: 900,
          },
        }),
      ),
      container,
    );

    const indicator = container.querySelector(".compaction-indicator--complete");
    expect(indicator).not.toBeNull();
    expect(indicator?.textContent).toContain("Context compacted");
    nowSpy.mockRestore();
  });

  it("hides stale compaction completion indicator", () => {
    const container = document.createElement("div");
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(10_000);
    render(
      renderChat(
        createProps({
          compactionStatus: {
            active: false,
            startedAt: 0,
            completedAt: 0,
          },
        }),
      ),
      container,
    );

    expect(container.querySelector(".compaction-indicator")).toBeNull();
    nowSpy.mockRestore();
  });

  it("shows a stop button when aborting is available", () => {
    const container = document.createElement("div");
    const onAbort = vi.fn();
    render(
      renderChat(
        createProps({
          sending: true,
          canAbort: true,
          onAbort,
        }),
      ),
      container,
    );

    const stopButton = Array.from(container.querySelectorAll("button")).find(
      (btn) => btn.textContent?.trim() === "Stop",
    );
    expect(stopButton).not.toBeUndefined();
    stopButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onAbort).toHaveBeenCalledTimes(1);
    expect(container.textContent).not.toContain("New session");
  });

  it("shows a new session button when aborting is unavailable", () => {
    const container = document.createElement("div");
    const onNewSession = vi.fn();
    render(
      renderChat(
        createProps({
          sending: true,
          canAbort: false,
          onNewSession,
        }),
      ),
      container,
    );

    const newSessionButton = Array.from(container.querySelectorAll("button")).find(
      (btn) => btn.textContent?.trim() === "New session",
    );
    expect(newSessionButton).not.toBeUndefined();
    newSessionButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onNewSession).toHaveBeenCalledTimes(1);
    expect(container.textContent).not.toContain("Stop");
  });

  it("renders the codex readonly surface without the classic processing card", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          stream: "",
          streamStartedAt: 1_000,
          readonlyRun: {
            runId: "run-1",
            sessionKey: "main",
            phase: "working",
            startedAt: 1_000,
            updatedAt: 1_100,
            latestProgress: "Scanning the workspace and gathering context.",
            progressPhase: "analysis",
            draftingText: null,
            lastToolName: "read_files",
            activity: [
              {
                id: "phase:starting",
                kind: "phase",
                phase: "starting",
                ts: 1_000,
                updatedAt: 1_000,
              },
              {
                id: "progress:1100",
                kind: "progress",
                summary: "Scanning the workspace and gathering context.",
                progressPhase: "analysis",
                ts: 1_100,
                updatedAt: 1_100,
              },
            ],
            toolSteps: [
              {
                toolCallId: "tool-1",
                name: "read_files",
                status: "running",
                startedAt: 1_000,
                updatedAt: 1_100,
                outputPreview: null,
              },
            ],
            lastError: null,
          },
        }),
      ),
      container,
    );

    expect(container.querySelector(".chat-readonly-run")).not.toBeNull();
    expect(container.querySelector(".chat-processing-card")).toBeNull();
    expect(container.textContent).toContain("Agent Working");
    expect(container.textContent).toContain("Scanning the workspace and gathering context.");
  });

  it("announces readonly status changes without exposing the timer as a live region update", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          readonlyRun: {
            runId: "run-2",
            sessionKey: "main",
            phase: "working",
            startedAt: 1_000,
            updatedAt: 1_100,
            latestProgress: "Scanning the workspace and gathering context.",
            progressPhase: "analysis",
            draftingText: null,
            lastToolName: "read_files",
            activity: [
              {
                id: "phase:starting",
                kind: "phase",
                phase: "starting",
                ts: 1_000,
                updatedAt: 1_000,
              },
              {
                id: "progress:1100",
                kind: "progress",
                summary: "Scanning the workspace and gathering context.",
                progressPhase: "analysis",
                ts: 1_100,
                updatedAt: 1_100,
              },
            ],
            toolSteps: [],
            lastError: null,
          },
        }),
      ),
      container,
    );

    const surface = container.querySelector(".chat-readonly-run");
    const liveStatus = container.querySelector(".chat-readonly-run__sr-status");
    const timer = container.querySelector(".chat-readonly-run__timer");
    expect(surface?.getAttribute("aria-live")).toBeNull();
    expect(liveStatus?.getAttribute("role")).toBe("status");
    expect(liveStatus?.getAttribute("aria-live")).toBe("polite");
    expect(timer?.getAttribute("aria-hidden")).toBe("true");
  });
});
