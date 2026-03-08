import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  t,
  getLocale,
  setLocale,
  initLocale,
  onLocaleChange,
  detectBrowserLocale,
} from "./i18n.js";

describe("i18n", () => {
  beforeEach(() => {
    // 重置为默认中文
    initLocale("zh");
  });

  describe("t()", () => {
    it("returns Chinese translation for known key", () => {
      initLocale("zh");
      expect(t("nav.tab.chat")).toBe("开始对话");
    });

    it("returns English translation for known key", () => {
      setLocale("en");
      expect(t("nav.tab.chat")).toBe("Start Chat");
    });

    it("returns key itself when a locale entry no longer exists", () => {
      initLocale("zh");
      expect(t("topbar.brand")).toBe("topbar.brand");
    });

    it("returns key itself when not found in any locale", () => {
      expect(t("non.existent.key")).toBe("non.existent.key");
    });

    it("supports {{var}} interpolation", () => {
      setLocale("en");
      const result = t("devices.confirmRevoke", {
        deviceId: "device-123",
        role: "admin",
      });
      expect(result).toBe("Revoke token for device-123 (admin)?");
    });

    it("supports {{var}} interpolation in Chinese", () => {
      initLocale("zh");
      const result = t("devices.confirmRevoke", {
        deviceId: "设备A",
        role: "管理员",
      });
      expect(result).toBe("撤销 设备A (管理员) 的令牌？");
    });

    it("supports {var} interpolation", () => {
      setLocale("en");
      const result = t("chat.readonly.activity.tool.running", {
        tool: "read_files",
      });
      expect(result).toBe("Running read_files");
    });

    it("supports multiple interpolation params", () => {
      setLocale("en");
      const result = t("sessions.confirmDelete", { key: "test-session" });
      expect(result).toContain("test-session");
    });

    it("leaves {{var}} unchanged if param not provided", () => {
      setLocale("en");
      const result = t("devices.confirmRevoke");
      expect(result).toContain("{{deviceId}}");
    });
  });

  describe("getLocale() / setLocale()", () => {
    it("defaults to zh after initLocale", () => {
      initLocale("zh");
      expect(getLocale()).toBe("zh");
    });

    it("switches locale", () => {
      setLocale("en");
      expect(getLocale()).toBe("en");
    });

    it("ignores invalid locale", () => {
      setLocale("en");
      setLocale("fr" as "en");
      expect(getLocale()).toBe("en");
    });

    it("does nothing when setting the same locale", () => {
      const cb = vi.fn();
      onLocaleChange(cb);
      setLocale("zh"); // already zh
      expect(cb).not.toHaveBeenCalled();
      setLocale("en");
      expect(cb).toHaveBeenCalledOnce();
    });
  });

  describe("initLocale()", () => {
    it("initializes with valid locale", () => {
      initLocale("en");
      expect(getLocale()).toBe("en");
    });

    it("falls back to browser detection for undefined", () => {
      initLocale(undefined);
      // 结果取决于 navigator.language，但不应抛出错误
      expect(["zh", "en"]).toContain(getLocale());
    });
  });

  describe("onLocaleChange()", () => {
    it("calls listener on locale change", () => {
      const cb = vi.fn();
      onLocaleChange(cb);
      setLocale("en");
      expect(cb).toHaveBeenCalledWith("en");
    });

    it("returns unsubscribe function", () => {
      const cb = vi.fn();
      const unsub = onLocaleChange(cb);
      unsub();
      setLocale("en");
      expect(cb).not.toHaveBeenCalled();
    });

    it("handles multiple listeners", () => {
      const cb1 = vi.fn();
      const cb2 = vi.fn();
      onLocaleChange(cb1);
      onLocaleChange(cb2);
      setLocale("en");
      expect(cb1).toHaveBeenCalledWith("en");
      expect(cb2).toHaveBeenCalledWith("en");
    });
  });

  describe("detectBrowserLocale()", () => {
    it("returns a valid locale", () => {
      const result = detectBrowserLocale();
      expect(["zh", "en"]).toContain(result);
    });
  });
});
