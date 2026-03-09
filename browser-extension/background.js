// background.js — CrabClaw Chrome Extension Service Worker
// Manages CDP debugging sessions and relay connection.
//
// Connection strategy (same pattern as Claude-in-Chrome / 1Password):
//   1. Primary: Native Messaging (connectNative) → strong keepalive, SW never suspends
//   2. Fallback: WebSocket + heartbeat ping → resets 30s idle timer (Chrome 116+)

// ---- Constants ----
const NATIVE_HOST_NAME = 'com.acosmi.crabclaw';
const DEFAULT_RELAY_URL = 'ws://127.0.0.1:19004/ws';
const RECONNECT_BASE_MS = 2000;
const RECONNECT_MAX_MS = 60000;
const WS_HEARTBEAT_MS = 20000; // 20s ping keeps SW alive (must be < 30s)

// ---- State ----
let nativePort = null;
let relayWs = null;
let relayToken = '';
let relayUrl = DEFAULT_RELAY_URL;
let attachedTabs = new Map(); // tabId -> { debuggee, attached }
let reconnectAttempts = 0;
let reconnectTimer = null;
let heartbeatTimer = null;
let lastConnectOpened = false;
let connectionMode = 'none'; // 'native' | 'websocket' | 'none'

// ---- Badge & Status ----
const STATUS = {
  OFF: { text: '', color: '#888888' },
  CONNECTING: { text: '...', color: '#FFA500' },
  ON: { text: 'ON', color: '#00AA00' },
  NATIVE: { text: 'N', color: '#0066CC' }, // native messaging connected
  ERROR: { text: '!', color: '#FF0000' },
};

function setBadge(status) {
  chrome.action.setBadgeText({ text: status.text });
  chrome.action.setBadgeBackgroundColor({ color: status.color });
}

function updateBadge() {
  const connected = connectionMode === 'native' ||
    (relayWs && relayWs.readyState === WebSocket.OPEN);

  if (attachedTabs.size === 0) {
    if (connectionMode === 'native') {
      setBadge(STATUS.NATIVE);
    } else if (relayWs && relayWs.readyState === WebSocket.OPEN) {
      setBadge(STATUS.OFF);
    } else if (relayWs && relayWs.readyState === WebSocket.CONNECTING) {
      setBadge(STATUS.CONNECTING);
    } else {
      setBadge(STATUS.OFF);
    }
    return;
  }

  if (!connected) {
    setBadge(STATUS.ERROR);
    return;
  }

  setBadge(connectionMode === 'native' ? STATUS.NATIVE : STATUS.ON);
}

// ---- Token Auto-Discovery ----
async function fetchRelayToken(baseUrl) {
  try {
    const httpUrl = baseUrl
      .replace(/^ws:\/\//, 'http://')
      .replace(/^wss:\/\//, 'https://')
      .replace(/\/ws\/?$/, '/json/version');
    const resp = await fetch(httpUrl, { signal: AbortSignal.timeout(3000) });
    if (!resp.ok) return '';
    const info = await resp.json();
    const wsDebugUrl = info.webSocketDebuggerUrl || '';
    const match = wsDebugUrl.match(/[?&]token=([^&]+)/);
    return match ? match[1] : '';
  } catch {
    return '';
  }
}

async function isRelayReachable(baseUrl) {
  try {
    const httpUrl = baseUrl
      .replace(/^ws:\/\//, 'http://')
      .replace(/^wss:\/\//, 'https://')
      .replace(/\/ws\/?$/, '/health');
    const resp = await fetch(httpUrl, { signal: AbortSignal.timeout(1500) });
    return resp.ok;
  } catch {
    return false;
  }
}

// ---- Native Messaging (Primary Path) ----

function connectNative() {
  try {
    nativePort = chrome.runtime.connectNative(NATIVE_HOST_NAME);
  } catch (e) {
    console.log('[CrabClaw] connectNative() threw:', e.message);
    return false;
  }

  nativePort.onMessage.addListener((msg) => {
    // Native messaging delivers parsed JSON objects directly.
    handleRelayMessage(msg);
  });

  nativePort.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError;
    console.log('[CrabClaw] Native port disconnected:', err?.message || 'unknown');
    nativePort = null;
    connectionMode = 'none';
    updateBadge();
    // Fall back to WebSocket.
    console.log('[CrabClaw] Falling back to WebSocket connection');
    connectWebSocket();
  });

  connectionMode = 'native';
  reconnectAttempts = 0;
  updateBadge();
  sendTabList();
  console.log('[CrabClaw] Connected via native messaging (strong keepalive)');
  return true;
}

// ---- WebSocket Connection (Fallback Path) ----

async function connectWebSocket() {
  if (connectionMode === 'native') return; // native is active, skip
  if (relayWs && (relayWs.readyState === WebSocket.OPEN || relayWs.readyState === WebSocket.CONNECTING)) {
    return;
  }

  setBadge(STATUS.CONNECTING);

  // Auto-discover token if needed.
  if (!relayToken || !lastConnectOpened) {
    if (relayToken && !lastConnectOpened) {
      console.log('[CrabClaw] Previous WS connection failed before open — clearing stale token');
      relayToken = '';
      chrome.storage.local.remove('relayToken');
    }
    const discovered = await fetchRelayToken(relayUrl);
    if (discovered) {
      relayToken = discovered;
      chrome.storage.local.set({ relayToken });
      console.log('[CrabClaw] Auto-discovered relay token');
    }
  }

  if (!(await isRelayReachable(relayUrl))) {
    relayWs = null;
    updateBadge();
    scheduleReconnect();
    return;
  }

  lastConnectOpened = false;
  const url = relayToken ? `${relayUrl}?token=${relayToken}` : relayUrl;
  relayWs = new WebSocket(url);

  relayWs.onopen = () => {
    console.log('[CrabClaw] Relay connected (WebSocket fallback)');
    lastConnectOpened = true;
    reconnectAttempts = 0;
    connectionMode = 'websocket';
    updateBadge();
    sendTabList();

    // Start heartbeat to keep Service Worker alive (must be < 30s).
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      if (relayWs && relayWs.readyState === WebSocket.OPEN) {
        relayWs.send(JSON.stringify({ type: 'ping' }));
      } else {
        stopHeartbeat();
      }
    }, WS_HEARTBEAT_MS);
  };

  relayWs.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleRelayMessage(msg);
    } catch {
      console.warn('[CrabClaw] Non-JSON relay message:', event.data);
    }
  };

  relayWs.onclose = (event) => {
    console.log('[CrabClaw] Relay disconnected', event.code, event.reason);
    relayWs = null;
    connectionMode = 'none';
    stopHeartbeat();
    updateBadge();
    scheduleReconnect();
  };

  relayWs.onerror = () => {
    console.warn('[CrabClaw] Relay connection failed, will retry');
    updateBadge();
  };
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
}

function scheduleReconnect() {
  if (connectionMode === 'native') return; // native is handling it
  if (reconnectTimer) return;

  reconnectAttempts++;
  const base = Math.min(RECONNECT_BASE_MS * Math.pow(2, reconnectAttempts - 1), RECONNECT_MAX_MS);
  const jitter = Math.random() * base * 0.3;
  const delay = Math.round(base + jitter);
  console.log(`[CrabClaw] Reconnect #${reconnectAttempts} in ${delay}ms`);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connectRelay();
  }, delay);
}

// ---- Unified Connection Entry Point ----

async function connectRelay() {
  // Already connected?
  if (connectionMode === 'native' && nativePort) return;
  if (connectionMode === 'websocket' && relayWs && relayWs.readyState === WebSocket.OPEN) return;

  // Try native messaging first.
  if (!nativePort) {
    if (connectNative()) return;
  }

  // Fall back to WebSocket.
  await connectWebSocket();
}

// ---- Send to Relay (unified) ----

function sendToRelay(data) {
  const payload = typeof data === 'object' ? data : JSON.parse(data);

  // Prefer native messaging.
  if (connectionMode === 'native' && nativePort) {
    try {
      nativePort.postMessage(payload);
      return true;
    } catch (e) {
      console.warn('[CrabClaw] Native send failed:', e.message);
      nativePort = null;
      connectionMode = 'none';
    }
  }

  // WebSocket fallback.
  if (relayWs && relayWs.readyState === WebSocket.OPEN) {
    relayWs.send(JSON.stringify(payload));
    return true;
  }

  return false;
}

// ---- Relay Message Handling ----
function handleRelayMessage(msg) {
  // msg is already a parsed object (from native messaging or JSON.parse).
  const { type, tabId, method, params, id } = msg;

  switch (type) {
    case 'cdp':
      forwardCdpToTab(tabId, method, params, id);
      break;
    case 'list_tabs':
      sendTabList();
      break;
    case 'attach':
      attachTab(tabId);
      break;
    case 'detach':
      detachTab(tabId);
      break;
    case 'navigate':
      if (tabId && msg.url) {
        chrome.tabs.update(tabId, { url: msg.url });
      }
      break;
    case 'create_tab':
      chrome.tabs.create({ url: msg.url || 'about:blank' }, (tab) => {
        sendToRelay({ type: 'tab_created', tabId: tab.id, url: tab.url });
      });
      break;
    case 'close_tab':
      if (tabId) {
        detachTab(tabId);
        chrome.tabs.remove(tabId);
      }
      break;
    case 'switch_tab':
      if (tabId) {
        chrome.tabs.update(tabId, { active: true });
      }
      break;
    case 'config':
      if (msg.relayUrl) relayUrl = msg.relayUrl;
      if (msg.token) relayToken = msg.token;
      break;
    case 'pong':
      // Heartbeat response from relay — no action needed.
      break;
    default:
      console.warn('[CrabClaw] Unknown relay message type:', type);
  }
}

// ---- CDP Debugger ----
async function attachTab(tabId) {
  if (attachedTabs.has(tabId)) {
    console.log('[CrabClaw] Tab already attached:', tabId);
    return true;
  }

  const debuggee = { tabId };
  try {
    await chrome.debugger.attach(debuggee, '1.3');
    attachedTabs.set(tabId, { debuggee, attached: true });
    console.log('[CrabClaw] Attached to tab:', tabId);

    await chrome.debugger.sendCommand(debuggee, 'Page.enable');
    await chrome.debugger.sendCommand(debuggee, 'Runtime.enable');
    await chrome.debugger.sendCommand(debuggee, 'DOM.enable');
    await chrome.debugger.sendCommand(debuggee, 'Accessibility.enable');

    updateBadge();
    sendToRelay({ type: 'tab_attached', tabId });
    return true;
  } catch (err) {
    console.error('[CrabClaw] Attach failed:', tabId, err.message);
    sendToRelay({ type: 'error', tabId, error: err.message });
    return false;
  }
}

async function detachTab(tabId) {
  const info = attachedTabs.get(tabId);
  if (!info) return;

  try {
    await chrome.debugger.detach(info.debuggee);
  } catch {
    // Tab may already be closed.
  }
  attachedTabs.delete(tabId);
  updateBadge();
  sendToRelay({ type: 'tab_detached', tabId });
  console.log('[CrabClaw] Detached from tab:', tabId);
}

async function forwardCdpToTab(tabId, method, params, requestId) {
  let targetTabId = tabId;
  if (!targetTabId && attachedTabs.size > 0) {
    targetTabId = attachedTabs.keys().next().value;
  }

  if (!targetTabId || !attachedTabs.has(targetTabId)) {
    sendToRelay({
      type: 'cdp_response',
      id: requestId,
      error: 'No attached tab for CDP command',
    });
    return;
  }

  const debuggee = attachedTabs.get(targetTabId).debuggee;

  try {
    const result = await chrome.debugger.sendCommand(debuggee, method, params || {});
    sendToRelay({
      type: 'cdp_response',
      id: requestId,
      tabId: targetTabId,
      result,
    });
  } catch (err) {
    sendToRelay({
      type: 'cdp_response',
      id: requestId,
      tabId: targetTabId,
      error: err.message,
    });
  }
}

// ---- Tab Management ----
async function sendTabList() {
  try {
    const tabs = await chrome.tabs.query({});
    const tabList = tabs.map((t) => ({
      id: t.id,
      url: t.url,
      title: t.title,
      active: t.active,
      attached: attachedTabs.has(t.id),
    }));
    sendToRelay({ type: 'tab_list', tabs: tabList });
  } catch (err) {
    console.error('[CrabClaw] Failed to query tabs:', err);
  }
}

// ---- CDP Event Forwarding ----
chrome.debugger.onEvent.addListener((source, method, params) => {
  if (!source.tabId || !attachedTabs.has(source.tabId)) return;

  sendToRelay({
    type: 'cdp_event',
    tabId: source.tabId,
    method,
    params,
  });
});

chrome.debugger.onDetach.addListener((source, reason) => {
  if (source.tabId && attachedTabs.has(source.tabId)) {
    attachedTabs.delete(source.tabId);
    updateBadge();
    sendToRelay({ type: 'tab_detached', tabId: source.tabId, reason });
    console.log('[CrabClaw] Debugger detached:', source.tabId, reason);
  }
});

chrome.tabs.onRemoved.addListener((tabId) => {
  if (attachedTabs.has(tabId)) {
    attachedTabs.delete(tabId);
    updateBadge();
    sendToRelay({ type: 'tab_closed', tabId });
  }
});

// ---- Popup Communication ----
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  switch (msg.action) {
    case 'getStatus':
      sendResponse({
        connected: connectionMode !== 'none',
        connectionMode,
        attachedTabs: Array.from(attachedTabs.keys()),
        relayUrl,
        hasToken: !!relayToken,
      });
      return true;

    case 'toggleTab': {
      const tabId = msg.tabId;
      if (attachedTabs.has(tabId)) {
        detachTab(tabId).then(() => sendResponse({ attached: false }));
      } else {
        attachTab(tabId).then((ok) => sendResponse({ attached: ok }));
      }
      return true;
    }

    case 'connect':
      if (msg.relayUrl) relayUrl = msg.relayUrl;
      relayToken = msg.token || '';
      chrome.storage.local.set({ relayUrl, relayToken });
      reconnectAttempts = 0;
      // Disconnect existing connections.
      if (nativePort) {
        nativePort.disconnect();
        nativePort = null;
      }
      if (relayWs) {
        relayWs.close();
        relayWs = null;
      }
      connectionMode = 'none';
      stopHeartbeat();
      connectRelay();
      sendResponse({ ok: true });
      return true;

    case 'disconnect':
      if (nativePort) {
        nativePort.disconnect();
        nativePort = null;
      }
      if (relayWs) {
        relayWs.close();
        relayWs = null;
      }
      connectionMode = 'none';
      stopHeartbeat();
      for (const tabId of attachedTabs.keys()) {
        detachTab(tabId);
      }
      setBadge(STATUS.OFF);
      sendResponse({ ok: true });
      return true;

    default:
      sendResponse({ error: 'Unknown action' });
      return true;
  }
});

// ---- Startup ----
chrome.storage.local.get(['relayUrl', 'relayToken'], (items) => {
  if (items.relayUrl) relayUrl = items.relayUrl;
  if (items.relayToken) relayToken = items.relayToken;

  // Connect: tries native messaging first, falls back to WebSocket.
  connectRelay();
});

console.log('[CrabClaw] Service worker initialized (native messaging + WebSocket fallback)');
