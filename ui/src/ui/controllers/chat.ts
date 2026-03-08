import type { GatewayBrowserClient } from "../gateway.ts";
import type { ChatReadonlyRunState } from "../chat/readonly-run-state.ts";
import type { ChatAttachment } from "../ui-types.ts";
import { extractText } from "../chat/message-extract.ts";
import {
  setChatReadonlyRunTerminal,
  startChatReadonlyRun,
  updateChatReadonlyRunFromChat,
} from "../chat/readonly-run-state.ts";
import { generateUUID } from "../uuid.ts";

export type ChatState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  sessionKey: string;
  chatLoading: boolean;
  chatMessages: unknown[];
  chatThinkingLevel: string | null;
  chatSending: boolean;
  chatMessage: string;
  chatAttachments: ChatAttachment[];
  chatRunId: string | null;
  chatStream: string | null;
  chatStreamStartedAt: number | null;
  chatReadonlyRun?: ChatReadonlyRunState;
  lastError: string | null;
};

export type ChatEventPayload = {
  runId: string;
  sessionKey: string;
  state: "delta" | "final" | "aborted" | "error";
  message?: unknown;
  errorMessage?: string;
};

export async function loadChatHistory(state: ChatState) {
  if (!state.client || !state.connected) {
    return;
  }
  state.chatLoading = true;
  state.lastError = null;
  try {
    const res = await state.client.request<{ messages?: Array<unknown>; thinkingLevel?: string }>(
      "chat.history",
      {
        sessionKey: state.sessionKey,
        limit: 200,
      },
    );
    const messages = Array.isArray(res.messages) ? res.messages : [];
    if ((state as any)._skipEmptyHistory) {
      (state as any)._skipEmptyHistory = false;
      if (messages.length === 0) {
        // transcript 尚未写入，保留预填充的用户消息不清空
      } else {
        // 有历史记录：若末尾已是 user 消息（transcript 已写入新消息），直接用 messages；
        // 否则将预填充的用户消息追加到历史末尾，避免在旧会话切换时新消息不可见（根因 A 修复）
        const pendingMsg = state.chatMessages[0];
        const lastMsg = messages[messages.length - 1] as any;
        if (pendingMsg && lastMsg?.role !== "user") {
          state.chatMessages = [...messages, pendingMsg];
        } else {
          state.chatMessages = messages;
        }
      }
    } else {
      state.chatMessages = messages;
    }
    state.chatThinkingLevel = res.thinkingLevel ?? null;
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.chatLoading = false;
  }
}

function dataUrlToBase64(dataUrl: string): { content: string; mimeType: string } | null {
  const match = /^data:([^;]+);base64,(.+)$/.exec(dataUrl);
  if (!match) {
    return null;
  }
  return { mimeType: match[1], content: match[2] };
}

export async function sendChatMessage(
  state: ChatState,
  message: string,
  attachments?: ChatAttachment[],
): Promise<string | null> {
  if (!state.client || !state.connected) {
    return null;
  }
  const msg = message.trim();
  const hasAttachments = attachments && attachments.length > 0;
  if (!msg && !hasAttachments) {
    return null;
  }

  const now = Date.now();

  // Build user message content blocks
  const contentBlocks: Array<{ type: string; text?: string; source?: unknown }> = [];
  if (msg) {
    contentBlocks.push({ type: "text", text: msg });
  }
  // Add image previews to the message for display
  if (hasAttachments) {
    for (const att of attachments) {
      contentBlocks.push({
        type: att.category ?? "image",
        source: { type: "base64", media_type: att.mimeType, data: att.dataUrl },
        ...(att.fileName ? { fileName: att.fileName } : {}),
        ...(att.fileSize ? { fileSize: att.fileSize } : {}),
      });
    }
  }

  state.chatMessages = [
    ...state.chatMessages,
    {
      role: "user",
      content: contentBlocks,
      timestamp: now,
    },
  ];

  state.chatSending = true;
  state.lastError = null;
  const runId = generateUUID();
  state.chatRunId = runId;
  state.chatStream = "";
  state.chatStreamStartedAt = now;
  if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
    startChatReadonlyRun(
      state as unknown as Parameters<typeof startChatReadonlyRun>[0],
      runId,
      now,
      state.sessionKey,
    );
  }

  // Convert attachments to API format
  const apiAttachments = hasAttachments
    ? attachments
      .map((att) => {
        const parsed = dataUrlToBase64(att.dataUrl);
        if (!parsed) {
          return null;
        }
        return {
          type: att.category ?? "image",
          mimeType: parsed.mimeType,
          content: parsed.content,
          ...(att.fileName ? { fileName: att.fileName } : {}),
          ...(att.fileSize ? { fileSize: att.fileSize } : {}),
        };
      })
      .filter((a): a is NonNullable<typeof a> => a !== null)
    : undefined;

  try {
    await state.client.request("chat.send", {
      sessionKey: state.sessionKey,
      message: msg,
      deliver: false,
      idempotencyKey: runId,
      attachments: apiAttachments,
    });
    return runId;
  } catch (err) {
    const error = String(err);
    state.chatRunId = null;
    state.chatStream = null;
    state.chatStreamStartedAt = null;
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      setChatReadonlyRunTerminal(
        state as unknown as Parameters<typeof setChatReadonlyRunTerminal>[0],
        "error",
        { runId, sessionKey: state.sessionKey, ts: Date.now(), errorMessage: error },
      );
    }
    state.lastError = error;
    state.chatMessages = [
      ...state.chatMessages,
      {
        role: "assistant",
        content: [{ type: "text", text: "Error: " + error }],
        timestamp: Date.now(),
      },
    ];
    return null;
  } finally {
    state.chatSending = false;
  }
}

export async function abortChatRun(state: ChatState): Promise<boolean> {
  if (!state.client || !state.connected) {
    return false;
  }
  const runId = state.chatRunId;
  try {
    await state.client.request(
      "chat.abort",
      runId ? { sessionKey: state.sessionKey, runId } : { sessionKey: state.sessionKey },
    );
    return true;
  } catch (err) {
    state.lastError = String(err);
    return false;
  }
}

export function handleChatEvent(state: ChatState, payload?: ChatEventPayload) {
  if (!payload) {
    return null;
  }
  if (payload.sessionKey !== state.sessionKey) {
    return null;
  }

  // Final from another run (e.g. sub-agent announce): refresh history to show new message.
  // See https://github.com/openacosmi/openacosmi/issues/1909
  if (payload.runId && state.chatRunId && payload.runId !== state.chatRunId) {
    if (payload.state === "final") {
      // Bug #9 fix: immediately append the assistant message to avoid gap
      // between clearing stream and loadChatHistory completing.
      if (payload.message) {
        state.chatMessages = [...state.chatMessages, payload.message];
      }
      return "final";
    }
    return null;
  }

  if (payload.state === "delta") {
    // Re-bind to the active run ID if we missed it (e.g. cross channel switch)
    if (payload.runId && !state.chatRunId) {
      state.chatRunId = payload.runId;
      state.chatStreamStartedAt = state.chatStreamStartedAt ?? Date.now();
    }
    const next = extractText(payload.message);
    if (typeof next === "string") {
      const current = state.chatStream ?? "";
      if (!current || next.length >= current.length) {
        state.chatStream = next;
      }
      if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
        updateChatReadonlyRunFromChat(
          state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
          {
            runId: payload.runId,
            sessionKey: payload.sessionKey,
            state: "delta",
            ts: Date.now(),
            text: next,
          },
        );
      }
    }
  } else if (payload.state === "final") {
    // Bug #9 fix: immediately append the final assistant message so it's
    // visible without waiting for loadChatHistory() to complete.
    // The subsequent loadChatHistory() call (in app-gateway.ts) will replace
    // chatMessages with the canonical backend transcript, which already
    // includes this message (transcript is persisted before broadcast).
    if (payload.message) {
      state.chatMessages = [...state.chatMessages, payload.message];
    }
    state.chatStream = null;
    state.chatRunId = null;
    state.chatStreamStartedAt = null;
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      updateChatReadonlyRunFromChat(
        state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
        {
          runId: payload.runId,
          sessionKey: payload.sessionKey,
          state: "final",
          ts: Date.now(),
          text: extractText(payload.message),
        },
      );
    }
  } else if (payload.state === "aborted") {
    state.chatStream = null;
    state.chatRunId = null;
    state.chatStreamStartedAt = null;
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      updateChatReadonlyRunFromChat(
        state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
        {
          runId: payload.runId,
          sessionKey: payload.sessionKey,
          state: "aborted",
          ts: Date.now(),
        },
      );
    }
  } else if (payload.state === "error") {
    state.chatStream = null;
    state.chatRunId = null;
    state.chatStreamStartedAt = null;
    state.lastError = payload.errorMessage ?? "chat error";
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      updateChatReadonlyRunFromChat(
        state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
        {
          runId: payload.runId,
          sessionKey: payload.sessionKey,
          state: "error",
          ts: Date.now(),
          errorMessage: payload.errorMessage ?? "chat error",
        },
      );
    }
  }
  return payload.state;
}

// ── Chat model selector ──

export type ChatModelState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  chatModels: Array<{ id: string; name: string; provider: string; source: string }>;
  chatCurrentModel: string | null;
  // Also sync debugModels for agents tab
  debugModels: unknown[];
};

export async function loadChatModels(state: ChatModelState) {
  if (!state.client || !state.connected) return;
  try {
    const [listRes, defaultRes] = await Promise.all([
      state.client.request<{ models?: Array<Record<string, unknown>> }>("models.list", {}),
      state.client.request<{ model?: string }>("models.default.get", {}),
    ]);
    const rawModels = Array.isArray(listRes?.models) ? listRes.models : [];
    state.chatModels = rawModels.map((m) => ({
      id: String(m.id ?? ""),
      name: String(m.name ?? m.id ?? ""),
      provider: String(m.provider ?? ""),
      source: String(m.source ?? "custom"),
    }));
    state.chatCurrentModel = defaultRes?.model ?? null;
    // Keep debugModels in sync so agents tab doesn't need separate fetch
    if (state.debugModels.length === 0) {
      state.debugModels = rawModels;
    }
  } catch {
    // non-critical — composer will work without model selector
  }
}

export async function setChatModel(state: ChatModelState, model: string) {
  if (!state.client || !state.connected) return;
  state.chatCurrentModel = model; // optimistic update
  try {
    await state.client.request("models.default.set", { model });
  } catch {
    // revert on failure — reload from server
    void loadChatModels(state);
  }
}
