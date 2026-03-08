const WINDOW_NAME = "openacosmi-media-manage";
const WINDOW_FEATURES = "popup=yes,width=1440,height=960,top=60,left=80,noopener,noreferrer";
const NATIVE_HANDLER_NAME = "openacosmiMediaManageWindow";
const DEEP_LINK_TIMEOUT_MS = 1200;
const PREFERRED_DEEP_LINK_SCHEME = "openacosmi";

const VALID_SUB_TABS = new Set([
  "overview",
  "llm",
  "sources",
  "tools",
  "drafts",
  "publish",
  "patrol",
  "strategy",
]);

type WebkitMessageHandler = {
  postMessage: (message: unknown) => void;
};

type NativeMediaManageBridge = {
  open?: (payload: { mediaSubTab?: string } | string) => void;
};

type HostTransport = "bridge" | "webkit" | "deeplink";

export type MediaManageOpenSource = "menu" | "agents" | "subagents" | "media-page";

export type MediaManageOpenRequest = {
  targetUrl: string;
  mediaSubTab: string;
  source?: MediaManageOpenSource;
};

export type MediaManageOpenResult =
  | { mode: "native-bridge" }
  | { mode: "native-webkit" }
  | { mode: "native-deeplink" }
  | { mode: "browser-window" }
  | { mode: "same-tab-fallback"; reason: string }
  | { mode: "failed"; reason: string };

export type OpenAcosmiHostCapabilities = {
  mediaManageWindow?: {
    open: boolean;
    transport: HostTransport;
  };
};

export type MediaManageOpenEnvironment = {
  getHostCapabilities?: () => OpenAcosmiHostCapabilities | undefined;
  getNativeBridge?: () => NativeMediaManageBridge | undefined;
  getWebkitMessageHandler?: () => WebkitMessageHandler | undefined;
  openDeepLink?: (
    deepLinkUrl: string,
    request: MediaManageOpenRequest & { mediaSubTab: string },
  ) => Promise<boolean>;
  openBrowserWindow?: (targetUrl: string) => Window | null;
  focusBrowserWindow?: (handle: Window) => void;
  closeBrowserWindow?: (handle: Window) => void;
  navigateSameTab?: (targetUrl: string) => void;
};

export function normalizeMediaManageSubTab(rawSubTab: string): string {
  const normalized = rawSubTab.trim().toLowerCase();
  return VALID_SUB_TABS.has(normalized) ? normalized : "overview";
}

function resolveNativeTransport(env: MediaManageOpenEnvironment): HostTransport | null {
  const capabilities = env.getHostCapabilities?.();
  const declared = capabilities?.mediaManageWindow;

  if (declared?.open) {
    if (declared.transport === "bridge" && env.getNativeBridge?.()?.open) {
      return "bridge";
    }
    if (declared.transport === "webkit" && env.getWebkitMessageHandler?.()) {
      return "webkit";
    }
    if (declared.transport === "deeplink") {
      return "deeplink";
    }
  }

  if (env.getNativeBridge?.()?.open) {
    return "bridge";
  }
  if (env.getWebkitMessageHandler?.()) {
    return "webkit";
  }
  return null;
}

function buildMediaManageDeepLink(subTab: string): string {
  const params = new URLSearchParams({ tab: "media" });
  if (subTab !== "overview") {
    params.set("mediaSubTab", subTab);
  }
  return `${PREFERRED_DEEP_LINK_SCHEME}://dashboard?${params.toString()}`;
}

function closeBrowserWindow(handle: Window) {
  handle.close?.();
}

async function triggerDashboardDeepLink(deepLinkUrl: string): Promise<boolean> {
  if (typeof window === "undefined" || typeof document === "undefined") {
    return false;
  }

  return new Promise<boolean>((resolve) => {
    let settled = false;
    const cleanups: Array<() => void> = [];

    const finish = (opened: boolean) => {
      if (settled) {
        return;
      }
      settled = true;
      while (cleanups.length > 0) {
        cleanups.pop()?.();
      }
      resolve(opened);
    };

    const onBlur = () => finish(true);
    const onVisibilityChange = () => {
      if (document.visibilityState === "hidden") {
        finish(true);
      }
    };

    window.addEventListener("blur", onBlur, true);
    cleanups.push(() => window.removeEventListener("blur", onBlur, true));

    document.addEventListener("visibilitychange", onVisibilityChange, true);
    cleanups.push(() => document.removeEventListener("visibilitychange", onVisibilityChange, true));

    const timeout = window.setTimeout(() => finish(false), DEEP_LINK_TIMEOUT_MS);
    cleanups.push(() => window.clearTimeout(timeout));

    try {
      const iframe = document.createElement("iframe");
      iframe.setAttribute("aria-hidden", "true");
      iframe.tabIndex = -1;
      iframe.style.position = "absolute";
      iframe.style.width = "0";
      iframe.style.height = "0";
      iframe.style.opacity = "0";
      iframe.style.pointerEvents = "none";
      iframe.src = deepLinkUrl;
      (document.body ?? document.documentElement).appendChild(iframe);
      cleanups.push(() => iframe.remove());
    } catch {
      finish(false);
    }
  });
}

async function openNativeWindow(
  request: MediaManageOpenRequest & { mediaSubTab: string },
  env: MediaManageOpenEnvironment,
): Promise<MediaManageOpenResult | null> {
  const subTab = request.mediaSubTab;
  const transport = resolveNativeTransport(env);
  if (!transport) {
    return null;
  }

  if (transport === "bridge") {
    const bridge = env.getNativeBridge?.();
    bridge?.open?.({ mediaSubTab: subTab });
    return { mode: "native-bridge" };
  }

  if (transport === "webkit") {
    const handler = env.getWebkitMessageHandler?.();
    handler?.postMessage({ kind: "open", tab: "media", mediaSubTab: subTab });
    return { mode: "native-webkit" };
  }

  const browserWindow = env.openBrowserWindow?.(request.targetUrl) ?? null;
  const deepLinkUrl = buildMediaManageDeepLink(subTab);
  const didOpenNative = await (env.openDeepLink?.(deepLinkUrl, request) ?? triggerDashboardDeepLink(deepLinkUrl));

  if (didOpenNative) {
    if (browserWindow) {
      (env.closeBrowserWindow ?? closeBrowserWindow)(browserWindow);
    }
    return { mode: "native-deeplink" };
  }

  if (browserWindow) {
    if (env.focusBrowserWindow) {
      env.focusBrowserWindow(browserWindow);
    } else {
      browserWindow.focus?.();
    }
    return { mode: "browser-window" };
  }

  if (env.navigateSameTab) {
    env.navigateSameTab(request.targetUrl);
    return { mode: "same-tab-fallback", reason: "popup-blocked" };
  }

  return { mode: "failed", reason: "deeplink-fallback-unavailable" };
}

export function createMediaManageOpenCoordinator(env: MediaManageOpenEnvironment) {
  return {
    async open(request: MediaManageOpenRequest): Promise<MediaManageOpenResult> {
      const subTab = normalizeMediaManageSubTab(request.mediaSubTab);
      const normalizedRequest = {
        ...request,
        mediaSubTab: subTab,
      };
      const nativeResult = await openNativeWindow(normalizedRequest, env);
      if (nativeResult) {
        return nativeResult;
      }

      const browserWindow = env.openBrowserWindow?.(request.targetUrl) ?? null;
      if (browserWindow) {
        if (env.focusBrowserWindow) {
          env.focusBrowserWindow(browserWindow);
        } else {
          browserWindow.focus?.();
        }
        return { mode: "browser-window" };
      }

      if (env.navigateSameTab) {
        env.navigateSameTab(request.targetUrl);
        return { mode: "same-tab-fallback", reason: "popup-blocked" };
      }

      return { mode: "failed", reason: "no-supported-surface" };
    },
  };
}

function readHostCapabilities(): OpenAcosmiHostCapabilities | undefined {
  const host = globalThis as typeof globalThis & {
    openacosmiHostCapabilities?: OpenAcosmiHostCapabilities;
  };
  return host.openacosmiHostCapabilities;
}

function readNativeBridge(): NativeMediaManageBridge | undefined {
  const host = globalThis as typeof globalThis & {
    openacosmiMediaManageWindow?: NativeMediaManageBridge;
  };
  return host.openacosmiMediaManageWindow;
}

function readWebkitMessageHandler(): WebkitMessageHandler | undefined {
  const host = globalThis as typeof globalThis & {
    webkit?: {
      messageHandlers?: Record<string, WebkitMessageHandler | undefined>;
    };
  };
  return host.webkit?.messageHandlers?.[NATIVE_HANDLER_NAME];
}

function openBrowserWindow(targetUrl: string): Window | null {
  if (typeof window === "undefined") {
    return null;
  }
  return window.open(targetUrl, WINDOW_NAME, WINDOW_FEATURES);
}

function focusBrowserWindow(handle: Window) {
  handle.focus?.();
}

function navigateSameTab(targetUrl: string) {
  if (typeof window === "undefined") {
    return;
  }
  window.location.assign(targetUrl);
}

export async function openMediaManageWindow(
  targetUrl: string,
  rawSubTab: string,
  source: MediaManageOpenSource = "media-page",
): Promise<MediaManageOpenResult> {
  const coordinator = createMediaManageOpenCoordinator({
    getHostCapabilities: readHostCapabilities,
    getNativeBridge: readNativeBridge,
    getWebkitMessageHandler: readWebkitMessageHandler,
    openBrowserWindow,
    focusBrowserWindow,
    closeBrowserWindow,
    navigateSameTab,
  });
  return coordinator.open({
    targetUrl,
    mediaSubTab: rawSubTab,
    source,
  });
}
