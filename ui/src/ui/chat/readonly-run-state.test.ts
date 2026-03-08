import { describe, expect, it } from "vitest";
import {
  createChatReadonlyRunState,
  isReadonlyRunActive,
  setChatReadonlyRunTerminal,
  startChatReadonlyRun,
  updateChatReadonlyRunFromChat,
  updateChatReadonlyRunFromLifecycle,
  updateChatReadonlyRunFromProgress,
  updateChatReadonlyRunFromTool,
} from "./readonly-run-state.ts";

function createHost() {
  return {
    sessionKey: "main",
    chatReadonlyRun: createChatReadonlyRunState("main"),
  };
}

describe("readonly run state", () => {
  it("transitions from starting to working to drafting", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-1", 100, "main");

    expect(host.chatReadonlyRun.phase).toBe("starting");
    expect(host.chatReadonlyRun.activity.map((item) => item.kind)).toEqual(["phase"]);

    updateChatReadonlyRunFromTool(host, {
      runId: "run-1",
      sessionKey: "main",
      ts: 120,
      toolCallId: "tool-1",
      name: "read_files",
      phase: "start",
    });

    expect(host.chatReadonlyRun.phase).toBe("working");
    expect(host.chatReadonlyRun.toolSteps).toHaveLength(1);
    expect(host.chatReadonlyRun.toolSteps[0]?.status).toBe("running");
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("tool");

    updateChatReadonlyRunFromProgress(host, {
      runId: "run-1",
      sessionKey: "main",
      ts: 150,
      summary: "Collected the relevant files.",
      phase: "analysis",
    });

    expect(host.chatReadonlyRun.latestProgress).toBe("Collected the relevant files.");
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("progress");

    updateChatReadonlyRunFromChat(host, {
      runId: "run-1",
      sessionKey: "main",
      state: "delta",
      ts: 180,
      text: "I found the issue and I'm preparing a fix.",
    });

    expect(host.chatReadonlyRun.phase).toBe("drafting");
    expect(host.chatReadonlyRun.draftingText).toContain("preparing a fix");
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("draft");
  });

  it("resets to idle when the run finalizes", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-2", 100, "main");

    updateChatReadonlyRunFromChat(host, {
      runId: "run-2",
      sessionKey: "main",
      state: "final",
      ts: 220,
      text: "Done.",
    });

    expect(host.chatReadonlyRun.runId).toBeNull();
    expect(host.chatReadonlyRun.phase).toBe("idle");
    expect(host.chatReadonlyRun.sessionKey).toBe("main");
  });

  it("marks terminal errors without keeping the run active", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-3", 100, "main");

    expect(isReadonlyRunActive(host.chatReadonlyRun)).toBe(true);

    setChatReadonlyRunTerminal(host, "error", {
      sessionKey: "main",
      ts: 180,
      errorMessage: "disconnected (1006): no reason",
    });

    expect(host.chatReadonlyRun.runId).toBeNull();
    expect(host.chatReadonlyRun.phase).toBe("error");
    expect(host.chatReadonlyRun.lastError).toBe("disconnected (1006): no reason");
    expect(isReadonlyRunActive(host.chatReadonlyRun)).toBe(false);
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("error");
  });

  it("records a finalizing phase in the activity feed when lifecycle ends", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-4", 100, "main");

    updateChatReadonlyRunFromLifecycle(host, {
      runId: "run-4",
      sessionKey: "main",
      ts: 160,
      phase: "end",
    });

    expect(host.chatReadonlyRun.phase).toBe("finalizing");
    expect(host.chatReadonlyRun.activity.some(
      (item) => item.kind === "phase" && item.phase === "finalizing",
    )).toBe(true);
  });
});
