import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatUxMode } from "./chat/readonly-run-state.ts";
import type { Tab } from "./navigation.ts";
import { applySettings, applySettingsFromUrl, setTabFromRoute } from "./app-settings.ts";

type SettingsHost = Parameters<typeof setTabFromRoute>[0] & {
  logsPollInterval: number | null;
  debugPollInterval: number | null;
  chatUxMode?: ChatUxMode;
  pendingGatewayUrl?: string | null;
};

const createHost = (tab: Tab): SettingsHost => ({
  settings: {
    gatewayUrl: "",
    token: "",
    sessionKey: "main",
    lastActiveSessionKey: "main",
    theme: "system",
    chatFocusMode: false,
    chatShowThinking: true,
    chatUxMode: "classic",
    splitRatio: 0.6,
    navCollapsed: false,
    navGroupsCollapsed: {},
    locale: "zh",
  },
  theme: "system",
  themeResolved: "dark",
  applySessionKey: "main",
  sessionKey: "main",
  tab,
  connected: false,
  chatHasAutoScrolled: false,
  logsAtBottom: false,
  eventLog: [],
  eventLogBuffer: [],
  basePath: "",
  themeMedia: null,
  themeMediaHandler: null,
  logsPollInterval: null,
  debugPollInterval: null,
});

describe("setTabFromRoute", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal("window", globalThis);
    vi.stubGlobal("localStorage", {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("starts and stops log polling based on the tab", () => {
    const host = createHost("chat");

    setTabFromRoute(host, "logs");
    expect(host.logsPollInterval).not.toBeNull();
    expect(host.debugPollInterval).toBeNull();

    setTabFromRoute(host, "chat");
    expect(host.logsPollInterval).toBeNull();
  });

  it("starts and stops debug polling based on the tab", () => {
    const host = createHost("chat");

    setTabFromRoute(host, "debug");
    expect(host.debugPollInterval).not.toBeNull();
    expect(host.logsPollInterval).toBeNull();

    setTabFromRoute(host, "chat");
    expect(host.debugPollInterval).toBeNull();
  });

  it("syncs chat ux mode onto the live host state", () => {
    const host = createHost("chat");
    host.chatUxMode = "classic";

    applySettings(host, {
      ...host.settings,
      chatUxMode: "codex-readonly",
    });

    expect(host.settings.chatUxMode).toBe("codex-readonly");
    expect(host.chatUxMode).toBe("codex-readonly");
  });

  it("reads chat ux mode from the url and strips the query param", () => {
    const host = createHost("chat");
    host.chatUxMode = "classic";
    const replaceState = vi.fn();
    const location = new URL("https://example.test/chat?chatUx=codex");
    vi.stubGlobal("window", {
      ...globalThis,
      location,
      history: {
        replaceState,
      },
    });

    applySettingsFromUrl(host);

    expect(host.settings.chatUxMode).toBe("codex-readonly");
    expect(host.chatUxMode).toBe("codex-readonly");
    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(String(replaceState.mock.calls[0]?.[2] ?? "")).not.toContain("chatUx");
  });
});
