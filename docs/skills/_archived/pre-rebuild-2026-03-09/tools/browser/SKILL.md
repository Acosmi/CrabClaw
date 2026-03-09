---
name: browser
description: "Web automation via CDP: navigate, observe, click_ref, fill_ref, screenshot, ai_browse. ARIA refs for robust element targeting. Prefer over Argus for web"
tools: browser
---

## When to Use browser (vs Argus vs web_search)

| 场景 | 工具 | 原因 |
|------|------|------|
| 网页导航、点击、填表 | browser | ARIA ref 精准定位，比 CSS 选择器更健壮 |
| 读取静态网页内容 | web_fetch | 更轻量，无需浏览器 |
| 桌面原生应用操作 | spawn_argus_agent | 原生 UI 无 ARIA 树 |
| 网页截图验证 | browser (screenshot) | 页内截图 |
| 全桌面截图 | argus_capture_screen | 全屏截取 |
| 意图级多步浏览任务 | browser (ai_browse) | 自动 observe→plan→act 循环 |

**关键区别**: browser 通过 ARIA 无障碍树 + ref 标识符定位元素（语义化、跨 DOM 变更健壮），Argus 用屏幕坐标（适合原生应用）。

**规则**: 有 URL 的用 browser，原生桌面窗口用 Argus。

**推荐工作流**: 先 `observe` 获取页面 ARIA 结构和 ref 标注 → 用 `click_ref` / `fill_ref` 交互（比 CSS selector 更可靠）。

---

## Agent tool: `browser`

Agent 获得**一个工具** (`browser`)，包含 **14 种 action**:

### Actions

| Action | Parameters | Description |
|--------|-----------|-------------|
| `navigate` | `url` | 导航到 URL。返回截图用于验证。 |
| `get_content` | — | 获取页面内容。返回 ARIA 无障碍树或 innerText。 |
| `observe` | — | **推荐首步。** 返回 ARIA 树 + ref 标注 (e1, e2...) + 截图。 |
| `click` | `selector` | 通过 CSS 选择器点击元素。 |
| `click_ref` | `ref` | **推荐。** 通过 ARIA ref (如 "e1") 点击元素。比 CSS 更健壮。 |
| `type` | `selector`, `text` | 通过 CSS 选择器输入文本。 |
| `fill_ref` | `ref`, `text` | **推荐。** 通过 ARIA ref 填入文本。 |
| `screenshot` | — | 截取页面截图 (JPEG, 优化)。 |
| `evaluate` | `script` | 执行 JavaScript（需 `browser.evaluateEnabled=true`）。 |
| `wait_for` | `selector` | 等待 CSS 选择器出现 (10s 超时, 100ms 轮询)。 |
| `go_back` | — | 浏览器后退。 |
| `go_forward` | — | 浏览器前进。 |
| `get_url` | — | 获取当前页面 URL。 |
| `ai_browse` | `goal` | **意图级浏览。** 自动 observe→plan→act 循环 (最多 20 步)。 |

### 推荐工作流

1. **`observe`** — 获取 ARIA 无障碍树 + 截图。响应中包含 ref 标识符 (e1, e2...) 用于交互元素。
2. **`click_ref` / `fill_ref`** — 用 observe 返回的 ref 交互。比 CSS 选择器更可靠（基于 ARIA 角色）。
3. **`screenshot`** — 验证结果。所有改变页面状态的操作都自动包含验证截图。

### 错误分类

- **Transient** — 元素可能还在加载。先用 `wait_for` 或 `observe`。
- **Structural** — 该 ref/selector 处没有元素。用 `observe` 检查页面状态。
- **Fatal** — CDP 连接断开。浏览器可能崩溃或断连。

### AI Browse (意图级)

`ai_browse` action 接受自然语言目标（如 "在京东搜索 MacBook Pro 并截图第一个结果"），自动执行多步 observe→plan→act 循环。大幅减少主对话回合数和 token 消耗。

需要 Gateway 中配置 AI planner。不可用时，使用手动 `observe` + `click_ref`/`fill_ref` 工作流替代。

### 截图优化

- 格式: JPEG (quality 75) + `optimizeForSpeed` — 比 PNG 小 3-5 倍。
- 每个改变状态的 action (navigate, click, type, click_ref, fill_ref) 自动返回验证截图。
- 截图通过 `__MULTIMODAL__` 协议传入 LLM 视觉通道。

### 选项

- `profile` — 选择浏览器 profile (openacosmi, chrome, 或 remote CDP)。
- `target` — (`sandbox` | `host` | `node`) 选择浏览器运行位置。
- 沙箱 session 中，`target: "host"` 需要 `agents.defaults.sandbox.browser.allowHostControl=true`。
- 省略 `target` 时: 沙箱 session 默认 `sandbox`，非沙箱默认 `host`。

主 Rust CLI 命令为 `crabclaw`；这里的 `openacosmi` 仅指兼容保留的浏览器 profile 名，不是主 CLI 名。

---

## Profiles

系统支持两种主要 profile:

- **`openacosmi`**: 专属隔离浏览器（独立 user data dir，与个人浏览器完全分开）。
- **`chrome`**: Chrome 扩展中继模式（控制已有 Chrome tab，需安装扩展并手动附加）。

设置 `browser.defaultProfile: "openacosmi"` 使用托管浏览器模式。

## 配置

浏览器设置在 `~/.openacosmi/openacosmi.json`:

```json5
{
  browser: {
    enabled: true,
    defaultProfile: "chrome",
    headless: false,
    noSandbox: false,
    attachOnly: false,
    executablePath: "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
    profiles: {
      openacosmi: { cdpPort: 18800 },
      work: { cdpPort: 18801 },
      remote: { cdpUrl: "http://10.0.0.42:9222" },
    },
  },
}
```

- 浏览器控制服务绑定在 loopback 上，端口 = `gateway.port` + 2（默认 18791）。
- Relay 端口 = `gateway.port` + 3（默认 18792）。
- 本地 CDP 端口: 18800-18899。
- `attachOnly: true` = 不启动浏览器，仅附加已运行的。
- 浏览器自动检测顺序: 系统默认 → Chrome → Brave → Edge → Chromium。

## 安全

- `browser.evaluateEnabled=false` 可禁用 JS 执行（防 prompt injection）。
- 远程 CDP URL/token 视为 secret，通过环境变量管理。
- 扩展中继模式下附加到个人 tab = 授予该账户完整访问权限。

## 故障排除

Linux 上 snap Chromium 问题，参见 [Browser troubleshooting](/tools/browser-linux-troubleshooting)。

## CLI 参考

完整 CLI 命令列表参见 `docs/cli/browser.md`（Rust CLI，主命令 `crabclaw`；profile `openacosmi` 继续保留兼容）。
