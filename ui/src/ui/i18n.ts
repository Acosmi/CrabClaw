/**
 * 零依赖轻量 i18n 模块
 *
 * 设计：
 * - 支持 'zh' | 'en' 两种语言
 * - 默认语言为中文 ('zh')
 * - 支持 {{var}} / {var} 插值
 * - 语言选择持久化到 UiSettings (localStorage)
 * - 自动检测 navigator.language 初始化
 */

import { zhMessages } from "./locales/zh.js";
import { enMessages } from "./locales/en.js";

export type Locale = "zh" | "en";

export const SUPPORTED_LOCALES: readonly Locale[] = ["zh", "en"] as const;

export const LOCALE_LABELS: Record<Locale, string> = {
  zh: "简体中文",
  en: "English",
};

type Messages = Record<string, string>;

const bundles: Record<Locale, Messages> = {
  zh: zhMessages,
  en: enMessages,
};

let currentLocale: Locale = "zh";

const listeners: Set<(locale: Locale) => void> = new Set();

/** 检测浏览器语言，返回最匹配的 Locale */
export function detectBrowserLocale(): Locale {
  if (typeof navigator === "undefined") {
    return "zh";
  }
  const lang = (navigator.language || "").toLowerCase();
  if (lang.startsWith("zh")) {
    return "zh";
  }
  return "en";
}

/** 获取当前语言 */
export function getLocale(): Locale {
  return currentLocale;
}

/** 设置当前语言，触发所有监听器 */
export function setLocale(locale: Locale): void {
  if (!SUPPORTED_LOCALES.includes(locale)) {
    return;
  }
  if (currentLocale === locale) {
    return;
  }
  currentLocale = locale;
  for (const fn of listeners) {
    try {
      fn(locale);
    } catch {
      // 忽略回调错误
    }
  }
}

/** 初始化语言，通常从 UiSettings 调用 */
export function initLocale(locale: Locale | undefined): void {
  if (locale && SUPPORTED_LOCALES.includes(locale)) {
    currentLocale = locale;
  } else {
    currentLocale = detectBrowserLocale();
  }
}

/** 注册语言切换监听器，返回取消注册函数 */
export function onLocaleChange(callback: (locale: Locale) => void): () => void {
  listeners.add(callback);
  return () => listeners.delete(callback);
}

/**
 * 翻译函数
 *
 * @param key - 翻译键，如 'nav.tab.chat'
 * @param params - 可选插值参数，替换 {{key}} / {key} 占位符
 * @returns 翻译后的字符串，找不到时返回 key 本身
 */
export function t(key: string, params?: Record<string, string | number>): string {
  const messages = bundles[currentLocale];
  let text = messages[key];
  if (text === undefined) {
    // 回退到英文
    text = bundles.en[key];
  }
  if (text === undefined) {
    // 最终回退：返回 key
    return key;
  }
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      const value = String(v);
      text = text.replaceAll(`{{${k}}}`, value);
      text = text.replaceAll(`{${k}}`, value);
    }
  }
  return text;
}
