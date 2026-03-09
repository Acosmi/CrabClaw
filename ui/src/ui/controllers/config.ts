import type { GatewayBrowserClient } from "../gateway.ts";
import type { ConfigSchemaResponse, ConfigSnapshot, ConfigUiHints } from "../types.ts";
import {
  cloneConfigObject,
  removePathValue,
  serializeConfigForm,
  setPathValue,
} from "./config/form-utils.ts";

export type DesktopUpdateStatus = {
  action?: string;
  currentVersion?: string;
  candidateVersion?: string;
  channel?: string;
  installKind?: string;
  updateManager?: string;
  managedBySystem?: boolean;
  state?: string;
  publishedAt?: string;
  readyToInstall?: boolean;
  rollbackAvailable?: boolean;
  rollbackVersion?: string;
  lastCheckedAt?: string;
  lastError?: string;
  updateAvailable?: boolean;
};

export type ConfigState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  applySessionKey: string;
  configLoading: boolean;
  configRaw: string;
  configRawOriginal: string;
  configValid: boolean | null;
  configIssues: unknown[];
  configSaving: boolean;
  configApplying: boolean;
  updateRunning: boolean;
  updateRollbackRunning: boolean;
  configSnapshot: ConfigSnapshot | null;
  configSchema: unknown;
  configSchemaVersion: string | null;
  configSchemaLoading: boolean;
  configUiHints: ConfigUiHints;
  configForm: Record<string, unknown> | null;
  configFormOriginal: Record<string, unknown> | null;
  configFormDirty: boolean;
  configFormMode: "form" | "raw";
  configSearchQuery: string;
  configActiveSection: string | null;
  configActiveSubsection: string | null;
  desktopUpdateStatus: DesktopUpdateStatus | null;
  lastError: string | null;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasNonEmptyConfigValue(value: unknown): boolean {
  if (value == null) {
    return false;
  }
  if (typeof value === "string") {
    return value.trim().length > 0;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return true;
  }
  if (Array.isArray(value)) {
    return value.some(hasNonEmptyConfigValue);
  }
  if (isRecord(value)) {
    return Object.values(value).some(hasNonEmptyConfigValue);
  }
  return true;
}

export function needsInitialSetup(snapshot: ConfigSnapshot | null | undefined): boolean {
  if (!snapshot) {
    return false;
  }
  if (snapshot.exists === false) {
    return true;
  }
  if (snapshot.valid === false) {
    return false;
  }
  if (!isRecord(snapshot.config)) {
    return false;
  }

  const wizard = isRecord(snapshot.config.wizard) ? snapshot.config.wizard : null;
  if (typeof wizard?.lastRunAt === "string" && wizard.lastRunAt.trim().length > 0) {
    return false;
  }

  const meaningfulSections = [
    "models",
    "agents",
    "channels",
    "memory",
    "skills",
    "tools",
    "gateway",
    "subAgents",
    "stt",
    "docConv",
    "imageUnderstanding",
    "browser",
    "plugins",
    "cron",
    "hooks",
    "discovery",
    "canvasHost",
    "talk",
  ];

  return !meaningfulSections.some((key) => hasNonEmptyConfigValue(snapshot.config?.[key]));
}

export async function loadConfig(state: ConfigState) {
  if (!state.client || !state.connected) {
    return;
  }
  state.configLoading = true;
  state.lastError = null;
  try {
    const res = await state.client.request<ConfigSnapshot>("config.get", {});
    applyConfigSnapshot(state, res);
    await loadDesktopUpdateStatus(state, { silent: true });
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.configLoading = false;
  }
}

export async function loadConfigSchema(state: ConfigState) {
  if (!state.client || !state.connected) {
    return;
  }
  if (state.configSchemaLoading) {
    return;
  }
  state.configSchemaLoading = true;
  try {
    const res = await state.client.request<ConfigSchemaResponse>("config.schema", {});
    applyConfigSchema(state, res);
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.configSchemaLoading = false;
  }
}

export function applyConfigSchema(state: ConfigState, res: ConfigSchemaResponse) {
  state.configSchema = res.schema ?? null;
  state.configUiHints = res.uiHints ?? {};
  state.configSchemaVersion = res.version ?? null;
}

export function applyConfigSnapshot(state: ConfigState, snapshot: ConfigSnapshot) {
  state.configSnapshot = snapshot;
  const rawFromSnapshot =
    typeof snapshot.raw === "string"
      ? snapshot.raw
      : snapshot.config && typeof snapshot.config === "object"
        ? serializeConfigForm(snapshot.config)
        : state.configRaw;
  if (!state.configFormDirty || state.configFormMode === "raw") {
    state.configRaw = rawFromSnapshot;
  } else if (state.configForm) {
    state.configRaw = serializeConfigForm(state.configForm);
  } else {
    state.configRaw = rawFromSnapshot;
  }
  state.configValid = typeof snapshot.valid === "boolean" ? snapshot.valid : null;
  state.configIssues = Array.isArray(snapshot.issues) ? snapshot.issues : [];

  if (!state.configFormDirty) {
    state.configForm = cloneConfigObject(snapshot.config ?? {});
    state.configFormOriginal = cloneConfigObject(snapshot.config ?? {});
    state.configRawOriginal = rawFromSnapshot;
  }
}

export async function saveConfig(state: ConfigState) {
  if (!state.client || !state.connected) {
    return;
  }
  state.configSaving = true;
  state.lastError = null;
  try {
    const raw =
      state.configFormMode === "form" && state.configForm
        ? serializeConfigForm(state.configForm)
        : state.configRaw;
    const baseHash = state.configSnapshot?.hash;
    if (!baseHash) {
      state.lastError = "Config hash missing; reload and retry.";
      return;
    }
    await state.client.request("config.set", { raw, baseHash });
    state.configFormDirty = false;
    await loadConfig(state);
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.configSaving = false;
  }
}

export async function applyConfig(state: ConfigState) {
  if (!state.client || !state.connected) {
    return;
  }
  state.configApplying = true;
  state.lastError = null;
  try {
    const raw =
      state.configFormMode === "form" && state.configForm
        ? serializeConfigForm(state.configForm)
        : state.configRaw;
    const baseHash = state.configSnapshot?.hash;
    if (!baseHash) {
      state.lastError = "Config hash missing; reload and retry.";
      return;
    }
    await state.client.request("config.apply", {
      raw,
      baseHash,
      sessionKey: state.applySessionKey,
    });
    state.configFormDirty = false;
    await loadConfig(state);
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.configApplying = false;
  }
}

export async function runUpdate(state: ConfigState) {
  if (!state.client || !state.connected || state.updateRollbackRunning || state.updateRunning) {
    return;
  }
  state.updateRunning = true;
  state.lastError = null;
  try {
    const status = await loadDesktopUpdateStatus(state, { check: true });
    if (!status) {
      return;
    }
    if (isPackagedDesktopInstall(status.installKind)) {
      if (status.managedBySystem || status.updateManager === "package-manager") {
        return;
      }
      if (status.readyToInstall) {
        state.desktopUpdateStatus = await state.client.request<DesktopUpdateStatus>(
          "desktop.update.apply",
          {},
        );
        return;
      }
      if (status.updateAvailable) {
        state.desktopUpdateStatus = await state.client.request<DesktopUpdateStatus>(
          "desktop.update.download",
          {},
        );
      }
      return;
    }
    await state.client.request("update.run", {
      sessionKey: state.applySessionKey,
    });
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.updateRunning = false;
  }
}

export async function rollbackUpdate(state: ConfigState) {
  if (!state.client || !state.connected || state.updateRunning || state.updateRollbackRunning) {
    return;
  }
  state.updateRollbackRunning = true;
  state.lastError = null;
  try {
    state.desktopUpdateStatus = await state.client.request<DesktopUpdateStatus>(
      "desktop.update.rollback",
      {},
    );
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.updateRollbackRunning = false;
  }
}

export function updateConfigFormValue(
  state: ConfigState,
  path: Array<string | number>,
  value: unknown,
) {
  const base = cloneConfigObject(state.configForm ?? state.configSnapshot?.config ?? {});
  setPathValue(base, path, value);
  state.configForm = base;
  state.configFormDirty = true;
  if (state.configFormMode === "form") {
    state.configRaw = serializeConfigForm(base);
  }
}

export function removeConfigFormValue(state: ConfigState, path: Array<string | number>) {
  const base = cloneConfigObject(state.configForm ?? state.configSnapshot?.config ?? {});
  removePathValue(base, path);
  state.configForm = base;
  state.configFormDirty = true;
  if (state.configFormMode === "form") {
    state.configRaw = serializeConfigForm(base);
  }
}

type LoadDesktopUpdateStatusOptions = {
  check?: boolean;
  silent?: boolean;
};

async function loadDesktopUpdateStatus(
  state: ConfigState,
  options: LoadDesktopUpdateStatusOptions = {},
): Promise<DesktopUpdateStatus | null> {
  if (!state.client || !state.connected) {
    state.desktopUpdateStatus = null;
    return null;
  }

  try {
    const res = await state.client.request<DesktopUpdateStatus>(
      options.check ? "desktop.update.check" : "desktop.update.status",
      {},
    );
    state.desktopUpdateStatus = res ?? null;
    return res ?? null;
  } catch (err) {
    state.desktopUpdateStatus = null;
    if (!options.silent) {
      state.lastError = String(err);
    }
    return null;
  }
}

function isPackagedDesktopInstall(installKind: string | null | undefined): boolean {
  return (
    installKind === "macos-wails" ||
    installKind === "windows-msix" ||
    installKind === "windows-nsis" ||
    installKind === "linux-appimage" ||
    installKind === "linux-system-package"
  );
}
