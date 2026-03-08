const KEY = "openacosmi.control.settings.v1";

import type { Locale } from "./i18n.js";
import type { ChatUxMode } from "./chat/readonly-run-state.ts";
import type { ThemeMode } from "./theme.js";

export type UiSettings = {
  gatewayUrl: string;
  token: string;
  sessionKey: string;
  lastActiveSessionKey: string;
  theme: ThemeMode;
  locale: Locale; // UI language: 'zh' | 'en', default 'zh'
  chatFocusMode: boolean;
  chatShowThinking: boolean;
  chatUxMode: ChatUxMode;
  splitRatio: number; // Sidebar split ratio (0.4 to 0.7, default 0.6)
  navCollapsed: boolean; // Collapsible sidebar state
  navGroupsCollapsed: Record<string, boolean>; // Which nav groups are collapsed
  lastSessionByChannel?: Record<string, string>; // History for cross-channel navigation
};

export function loadSettings(): UiSettings {
  const defaultUrl = (() => {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    // In dev mode (Vite on 26222), the Vite proxy handles /ws → ws://localhost:19001/ws
    // so we just use the same origin. In production, the gateway serves everything.
    return `${proto}://${location.host}`;
  })();

  const defaults: UiSettings = {
    gatewayUrl: defaultUrl,
    token: "",
    sessionKey: "main",
    lastActiveSessionKey: "main",
    theme: "system",
    locale: "zh",
    chatFocusMode: false,
    chatShowThinking: true,
    chatUxMode: "classic",
    splitRatio: 0.6,
    navCollapsed: false,
    navGroupsCollapsed: {},
  };

  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) {
      return defaults;
    }
    const parsed = JSON.parse(raw) as Partial<UiSettings>;
    return {
      gatewayUrl:
        typeof parsed.gatewayUrl === "string" && parsed.gatewayUrl.trim()
          ? parsed.gatewayUrl.trim()
          : defaults.gatewayUrl,
      token: typeof parsed.token === "string" ? parsed.token : defaults.token,
      sessionKey:
        typeof parsed.sessionKey === "string" && parsed.sessionKey.trim()
          ? parsed.sessionKey.trim()
          : defaults.sessionKey,
      lastActiveSessionKey:
        typeof parsed.lastActiveSessionKey === "string" && parsed.lastActiveSessionKey.trim()
          ? parsed.lastActiveSessionKey.trim()
          : (typeof parsed.sessionKey === "string" && parsed.sessionKey.trim()) ||
          defaults.lastActiveSessionKey,
      theme:
        parsed.theme === "light" || parsed.theme === "dark" || parsed.theme === "system"
          ? parsed.theme
          : defaults.theme,
      locale:
        parsed.locale === "zh" || parsed.locale === "en"
          ? parsed.locale
          : defaults.locale,
      chatFocusMode:
        typeof parsed.chatFocusMode === "boolean" ? parsed.chatFocusMode : defaults.chatFocusMode,
      chatShowThinking:
        typeof parsed.chatShowThinking === "boolean"
          ? parsed.chatShowThinking
          : defaults.chatShowThinking,
      chatUxMode:
        parsed.chatUxMode === "classic" || parsed.chatUxMode === "codex-readonly"
          ? parsed.chatUxMode
          : defaults.chatUxMode,
      splitRatio:
        typeof parsed.splitRatio === "number" &&
          parsed.splitRatio >= 0.4 &&
          parsed.splitRatio <= 0.7
          ? parsed.splitRatio
          : defaults.splitRatio,
      navCollapsed:
        typeof parsed.navCollapsed === "boolean" ? parsed.navCollapsed : defaults.navCollapsed,
      navGroupsCollapsed:
        typeof parsed.navGroupsCollapsed === "object" && parsed.navGroupsCollapsed !== null
          ? parsed.navGroupsCollapsed
          : defaults.navGroupsCollapsed,
      lastSessionByChannel:
        typeof parsed.lastSessionByChannel === "object" && parsed.lastSessionByChannel !== null
          ? parsed.lastSessionByChannel
          : {},
    };
  } catch {
    return defaults;
  }
}

export function saveSettings(next: UiSettings) {
  localStorage.setItem(KEY, JSON.stringify(next));
}
