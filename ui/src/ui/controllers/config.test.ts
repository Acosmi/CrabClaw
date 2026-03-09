import { describe, expect, it, vi } from "vitest";
import {
  applyConfigSnapshot,
  applyConfig,
  rollbackUpdate,
  runUpdate,
  updateConfigFormValue,
  type ConfigState,
} from "./config.ts";

function createState(): ConfigState {
  return {
    applySessionKey: "main",
    client: null,
    configActiveSection: null,
    configActiveSubsection: null,
    configApplying: false,
    configForm: null,
    configFormDirty: false,
    configFormMode: "form",
    configFormOriginal: null,
    configIssues: [],
    configLoading: false,
    configRaw: "",
    configRawOriginal: "",
    configSaving: false,
    configSchema: null,
    configSchemaLoading: false,
    configSchemaVersion: null,
    configSearchQuery: "",
    configSnapshot: null,
    configUiHints: {},
    configValid: null,
    connected: false,
    desktopUpdateStatus: null,
    lastError: null,
    updateRunning: false,
    updateRollbackRunning: false,
  };
}

describe("applyConfigSnapshot", () => {
  it("does not clobber form edits while dirty", () => {
    const state = createState();
    state.configFormMode = "form";
    state.configFormDirty = true;
    state.configForm = { gateway: { mode: "local", port: 18789 } };
    state.configRaw = "{\n}\n";

    applyConfigSnapshot(state, {
      config: { gateway: { mode: "remote", port: 9999 } },
      valid: true,
      issues: [],
      raw: '{\n  "gateway": { "mode": "remote", "port": 9999 }\n}\n',
    });

    expect(state.configRaw).toBe(
      '{\n  "gateway": {\n    "mode": "local",\n    "port": 18789\n  }\n}\n',
    );
  });

  it("updates config form when clean", () => {
    const state = createState();
    applyConfigSnapshot(state, {
      config: { gateway: { mode: "local" } },
      valid: true,
      issues: [],
      raw: "{}",
    });

    expect(state.configForm).toEqual({ gateway: { mode: "local" } });
  });

  it("sets configRawOriginal when clean for change detection", () => {
    const state = createState();
    applyConfigSnapshot(state, {
      config: { gateway: { mode: "local" } },
      valid: true,
      issues: [],
      raw: '{ "gateway": { "mode": "local" } }',
    });

    expect(state.configRawOriginal).toBe('{ "gateway": { "mode": "local" } }');
    expect(state.configFormOriginal).toEqual({ gateway: { mode: "local" } });
  });

  it("preserves configRawOriginal when dirty", () => {
    const state = createState();
    state.configFormDirty = true;
    state.configRawOriginal = '{ "original": true }';
    state.configFormOriginal = { original: true };

    applyConfigSnapshot(state, {
      config: { gateway: { mode: "local" } },
      valid: true,
      issues: [],
      raw: '{ "gateway": { "mode": "local" } }',
    });

    // Original values should be preserved when dirty
    expect(state.configRawOriginal).toBe('{ "original": true }');
    expect(state.configFormOriginal).toEqual({ original: true });
  });
});

describe("updateConfigFormValue", () => {
  it("seeds from snapshot when form is null", () => {
    const state = createState();
    state.configSnapshot = {
      config: { channels: { telegram: { botToken: "t" } }, gateway: { mode: "local" } },
      valid: true,
      issues: [],
      raw: "{}",
    };

    updateConfigFormValue(state, ["gateway", "port"], 18789);

    expect(state.configFormDirty).toBe(true);
    expect(state.configForm).toEqual({
      channels: { telegram: { botToken: "t" } },
      gateway: { mode: "local", port: 18789 },
    });
  });

  it("keeps raw in sync while editing the form", () => {
    const state = createState();
    state.configSnapshot = {
      config: { gateway: { mode: "local" } },
      valid: true,
      issues: [],
      raw: "{\n}\n",
    };

    updateConfigFormValue(state, ["gateway", "port"], 18789);

    expect(state.configRaw).toBe(
      '{\n  "gateway": {\n    "mode": "local",\n    "port": 18789\n  }\n}\n',
    );
  });
});

describe("applyConfig", () => {
  it("sends config.apply with raw and session key", async () => {
    const request = vi.fn().mockResolvedValue({});
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];
    state.applySessionKey = "agent:main:whatsapp:dm:+15555550123";
    state.configFormMode = "raw";
    state.configRaw = '{\n  agent: { workspace: "~/openacosmi" }\n}\n';
    state.configSnapshot = {
      hash: "hash-123",
    };

    await applyConfig(state);

    expect(request).toHaveBeenCalledWith("config.apply", {
      raw: '{\n  agent: { workspace: "~/openacosmi" }\n}\n',
      baseHash: "hash-123",
      sessionKey: "agent:main:whatsapp:dm:+15555550123",
    });
  });
});

describe("runUpdate", () => {
  it("checks desktop status before running legacy source update", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce({ installKind: "source", state: "idle" })
      .mockResolvedValueOnce({});
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];
    state.applySessionKey = "agent:main:whatsapp:dm:+15555550123";

    await runUpdate(state);

    expect(request).toHaveBeenNthCalledWith(1, "desktop.update.check", {});
    expect(state.desktopUpdateStatus).toEqual({ installKind: "source", state: "idle" });
    expect(request).toHaveBeenNthCalledWith(2, "update.run", {
      sessionKey: "agent:main:whatsapp:dm:+15555550123",
    });
  });

  it("stops after desktop.update.check for packaged installs", async () => {
    const request = vi.fn().mockResolvedValue({
      installKind: "macos-wails",
      state: "idle",
      updateAvailable: false,
    });
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await runUpdate(state);

    expect(request).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledWith("desktop.update.check", {});
    expect(state.desktopUpdateStatus).toEqual({
      installKind: "macos-wails",
      state: "idle",
      updateAvailable: false,
    });
  });

  it("downloads packaged updates after desktop.update.check", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce({
        installKind: "macos-wails",
        state: "available",
        updateAvailable: true,
        candidateVersion: "1.2.0",
      })
      .mockResolvedValueOnce({
        installKind: "macos-wails",
        state: "ready-to-install",
        readyToInstall: true,
        action: "downloaded",
        candidateVersion: "1.2.0",
      });
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await runUpdate(state);

    expect(request).toHaveBeenNthCalledWith(1, "desktop.update.check", {});
    expect(request).toHaveBeenNthCalledWith(2, "desktop.update.download", {});
    expect(state.desktopUpdateStatus).toEqual({
      installKind: "macos-wails",
      state: "ready-to-install",
      readyToInstall: true,
      action: "downloaded",
      candidateVersion: "1.2.0",
    });
  });

  it("applies packaged updates when readyToInstall", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce({
        installKind: "macos-wails",
        state: "ready-to-install",
        readyToInstall: true,
        candidateVersion: "1.2.0",
      })
      .mockResolvedValueOnce({
        installKind: "macos-wails",
        state: "ready-to-install",
        readyToInstall: true,
        action: "artifact-opened",
        candidateVersion: "1.2.0",
      });
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await runUpdate(state);

    expect(request).toHaveBeenNthCalledWith(1, "desktop.update.check", {});
    expect(request).toHaveBeenNthCalledWith(2, "desktop.update.apply", {});
    expect(state.desktopUpdateStatus).toEqual({
      installKind: "macos-wails",
      state: "ready-to-install",
      readyToInstall: true,
      action: "artifact-opened",
      candidateVersion: "1.2.0",
    });
  });

  it("records desktop.update.check errors", async () => {
    const request = vi.fn().mockRejectedValue(new Error("boom"));
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await runUpdate(state);

    expect(request).toHaveBeenCalledWith("desktop.update.check", {});
    expect(state.lastError).toContain("boom");
  });

  it("stops when update is managed by package manager", async () => {
    const request = vi.fn().mockResolvedValue({
      installKind: "linux-system-package",
      updateManager: "package-manager",
      state: "idle",
      updateAvailable: false,
    });
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await runUpdate(state);

    expect(request).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledWith("desktop.update.check", {});
  });
});

describe("rollbackUpdate", () => {
  it("calls desktop.update.rollback directly", async () => {
    const request = vi.fn().mockResolvedValue({
      installKind: "linux-appimage",
      state: "rolled-back",
      action: "rollback-completed",
    });
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await rollbackUpdate(state);

    expect(request).toHaveBeenCalledWith("desktop.update.rollback", {});
    expect(state.desktopUpdateStatus).toEqual({
      installKind: "linux-appimage",
      state: "rolled-back",
      action: "rollback-completed",
    });
    expect(state.updateRollbackRunning).toBe(false);
  });

  it("records desktop.update.rollback errors", async () => {
    const request = vi.fn().mockRejectedValue(new Error("rollback boom"));
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    await rollbackUpdate(state);

    expect(request).toHaveBeenCalledWith("desktop.update.rollback", {});
    expect(state.lastError).toContain("rollback boom");
    expect(state.updateRollbackRunning).toBe(false);
  });
});

describe("loadConfig", () => {
  it("refreshes desktop update status after config snapshot load", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce({
        config: { update: { channel: "stable" } },
        valid: true,
        issues: [],
        raw: "{\n}\n",
      })
      .mockResolvedValueOnce({
        installKind: "macos-wails",
        state: "idle",
        lastCheckedAt: "2026-03-09T12:00:00Z",
      });
    const state = createState();
    state.connected = true;
    state.client = { request } as unknown as ConfigState["client"];

    const { loadConfig } = await import("./config.ts");
    await loadConfig(state);

    expect(request).toHaveBeenNthCalledWith(1, "config.get", {});
    expect(request).toHaveBeenNthCalledWith(2, "desktop.update.status", {});
    expect(state.desktopUpdateStatus).toEqual({
      installKind: "macos-wails",
      state: "idle",
      lastCheckedAt: "2026-03-09T12:00:00Z",
    });
  });
});
