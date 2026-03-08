import { describe, expect, it, vi } from "vitest";
import {
  createMediaManageOpenCoordinator,
  normalizeMediaManageSubTab,
  type MediaManageOpenEnvironment,
} from "./media-manage-open.ts";

function createEnv(overrides: Partial<MediaManageOpenEnvironment> = {}): MediaManageOpenEnvironment {
  return {
    getHostCapabilities: () => undefined,
    getNativeBridge: () => undefined,
    getWebkitMessageHandler: () => undefined,
    openBrowserWindow: () => null,
    focusBrowserWindow: () => {},
    navigateSameTab: () => {},
    ...overrides,
  };
}

describe("media manage open coordinator", () => {
  it("prefers the native bridge when available", async () => {
    const open = vi.fn();
    const openBrowserWindow = vi.fn();
    const coordinator = createMediaManageOpenCoordinator(createEnv({
      getNativeBridge: () => ({ open }),
      openBrowserWindow,
    }));

    const result = await coordinator.open({
      targetUrl: "https://example.com/media?mediaSubTab=drafts",
      mediaSubTab: "drafts",
      source: "agents",
    });

    expect(result).toEqual({ mode: "native-bridge" });
    expect(open).toHaveBeenCalledWith({ mediaSubTab: "drafts" });
    expect(openBrowserWindow).not.toHaveBeenCalled();
  });

  it("uses the webkit handler when declared by host capabilities", async () => {
    const postMessage = vi.fn();
    const coordinator = createMediaManageOpenCoordinator(createEnv({
      getHostCapabilities: () => ({
        mediaManageWindow: {
          open: true,
          transport: "webkit",
        },
      }),
      getWebkitMessageHandler: () => ({ postMessage }),
    }));

    const result = await coordinator.open({
      targetUrl: "https://example.com/media",
      mediaSubTab: "tools",
      source: "media-page",
    });

    expect(result).toEqual({ mode: "native-webkit" });
    expect(postMessage).toHaveBeenCalledWith({
      kind: "open",
      tab: "media",
      mediaSubTab: "tools",
    });
  });

  it("opens a browser window when no native transport is available", async () => {
    const focusBrowserWindow = vi.fn();
    const openBrowserWindow = vi.fn(() => ({}) as unknown as Window);
    const coordinator = createMediaManageOpenCoordinator(createEnv({
      openBrowserWindow,
      focusBrowserWindow,
    }));

    const result = await coordinator.open({
      targetUrl: "https://example.com/media",
      mediaSubTab: "publish",
      source: "subagents",
    });

    expect(result).toEqual({ mode: "browser-window" });
    expect(openBrowserWindow).toHaveBeenCalledWith("https://example.com/media");
    expect(focusBrowserWindow).toHaveBeenCalledTimes(1);
  });

  it("uses deep links and closes the browser fallback window after native handoff", async () => {
    const closeBrowserWindow = vi.fn();
    const openBrowserWindow = vi.fn(() => ({}) as unknown as Window);
    const openDeepLink = vi.fn(async () => true);
    const coordinator = createMediaManageOpenCoordinator(createEnv({
      getHostCapabilities: () => ({
        mediaManageWindow: {
          open: true,
          transport: "deeplink",
        },
      }),
      openBrowserWindow,
      openDeepLink,
      closeBrowserWindow,
    }));

    const result = await coordinator.open({
      targetUrl: "https://example.com/media?mediaSubTab=publish",
      mediaSubTab: "publish",
      source: "menu",
    });

    expect(result).toEqual({ mode: "native-deeplink" });
    expect(openBrowserWindow).toHaveBeenCalledWith("https://example.com/media?mediaSubTab=publish");
    expect(openDeepLink).toHaveBeenCalledWith(
      "openacosmi://dashboard?tab=media&mediaSubTab=publish",
      expect.objectContaining({
        mediaSubTab: "publish",
        source: "menu",
      }),
    );
    expect(closeBrowserWindow).toHaveBeenCalledTimes(1);
  });

  it("falls back to the opened browser window when deeplink handoff fails", async () => {
    const focusBrowserWindow = vi.fn();
    const openBrowserWindow = vi.fn(() => ({}) as unknown as Window);
    const coordinator = createMediaManageOpenCoordinator(createEnv({
      getHostCapabilities: () => ({
        mediaManageWindow: {
          open: true,
          transport: "deeplink",
        },
      }),
      openBrowserWindow,
      openDeepLink: async () => false,
      focusBrowserWindow,
    }));

    const result = await coordinator.open({
      targetUrl: "https://example.com/media",
      mediaSubTab: "overview",
      source: "subagents",
    });

    expect(result).toEqual({ mode: "browser-window" });
    expect(openBrowserWindow).toHaveBeenCalledTimes(1);
    expect(focusBrowserWindow).toHaveBeenCalledTimes(1);
  });

  it("falls back to same-tab navigation when the popup is blocked", async () => {
    const navigateSameTab = vi.fn();
    const coordinator = createMediaManageOpenCoordinator(createEnv({
      openBrowserWindow: () => null,
      navigateSameTab,
    }));

    const result = await coordinator.open({
      targetUrl: "https://example.com/media?mediaSubTab=overview",
      mediaSubTab: "unknown",
      source: "media-page",
    });

    expect(result).toEqual({ mode: "same-tab-fallback", reason: "popup-blocked" });
    expect(navigateSameTab).toHaveBeenCalledWith("https://example.com/media?mediaSubTab=overview");
  });
});

describe("normalizeMediaManageSubTab", () => {
  it("keeps supported subtabs and falls back for unknown values", () => {
    expect(normalizeMediaManageSubTab(" drafts ")).toBe("drafts");
    expect(normalizeMediaManageSubTab("unknown")).toBe("overview");
  });
});
