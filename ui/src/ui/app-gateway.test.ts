import { beforeEach, describe, expect, it, vi } from "vitest";
import { handleGatewayEvent } from "./app-gateway.ts";
import { createChatReadonlyRunState } from "./chat/readonly-run-state.ts";

function createRemoteHost() {
  const activeRun = {
    ...createChatReadonlyRunState("feishu:chat-a"),
    runId: "remote-123",
    sessionKey: "feishu:chat-a",
    phase: "starting" as const,
    startedAt: 100,
    updatedAt: 100,
  };

  return {
    settings: {
      gatewayUrl: "",
      token: "",
      sessionKey: "feishu:chat-a",
      lastActiveSessionKey: "feishu:chat-a",
      lastSessionByChannel: {},
      theme: "system",
      locale: "zh",
      chatFocusMode: false,
      chatShowThinking: true,
      chatUxMode: "codex-readonly",
      splitRatio: 0.6,
      navCollapsed: false,
      navGroupsCollapsed: {},
    },
    password: "",
    client: null,
    connected: false,
    hello: null,
    lastError: null,
    eventLogBuffer: [] as unknown[],
    eventLog: [] as unknown[],
    tab: "chat" as const,
    presenceEntries: [],
    presenceError: null,
    presenceStatus: null,
    agentsLoading: false,
    agentsList: null,
    agentsError: null,
    debugHealth: null,
    assistantName: "OpenAcosmi",
    assistantAvatar: null,
    assistantAgentId: null,
    sessionKey: "feishu:chat-a",
    chatRunId: "remote-123",
    chatStream: "",
    chatStreamStartedAt: 100,
    chatReadonlyRun: activeRun,
    chatReadonlyRunHistory: [],
    chatMessages: [] as unknown[],
    chatToolMessages: [] as unknown[],
    toolStreamById: new Map(),
    toolStreamOrder: [] as string[],
    toolStreamSyncTimer: null,
    agentProgress: null,
    agentProgressClearTimer: null,
    refreshSessionsAfterChat: new Set<string>(),
    chatQueue: [],
    chatSending: false,
    chatMessage: "",
    chatAttachments: [],
    basePath: "",
    chatAvatarUrl: null,
    requestUpdate: vi.fn(),
    addNotification: vi.fn(),
  };
}

describe("gateway remote workflow handling", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("keeps the remote placeholder workflow alive until the authoritative final chat event arrives", () => {
    const host = createRemoteHost();

    handleGatewayEvent(host as never, {
      type: "event",
      event: "chat.message",
      payload: {
        sessionKey: "feishu:chat-a",
        channel: "feishu",
        role: "assistant",
        text: "已经处理完了",
        ts: 150,
      },
    });

    expect(host.chatMessages).toHaveLength(1);
    expect(host.chatRunId).toBe("remote-123");
    expect(host.chatReadonlyRun.runId).toBe("remote-123");
    expect(host.chatReadonlyRun.phase).toBe("starting");

    handleGatewayEvent(host as never, {
      type: "event",
      event: "chat",
      payload: {
        runId: "run-feishu-1",
        sessionKey: "feishu:chat-a",
        state: "final",
        message: {
          id: "msg-final",
          role: "assistant",
          content: [{ type: "text", text: "已经处理完了" }],
          timestamp: 150,
        },
      },
    });

    expect(host.chatRunId).toBeNull();
    expect(host.chatReadonlyRun.phase).toBe("complete");
    expect(host.chatReadonlyRun.finalMessageId).toBe("msg-final");
    expect(host.chatReadonlyRunHistory).toHaveLength(1);
    expect(host.chatReadonlyRunHistory[0]?.finalMessageId).toBe("msg-final");
  });
});
