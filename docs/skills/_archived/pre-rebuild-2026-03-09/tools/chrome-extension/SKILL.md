---
name: chrome-extension
description: "Chrome extension: let Crab Claw（蟹爪） drive your existing Chrome tab"
---

# Chrome extension (browser relay)

The Crab Claw（蟹爪） Chrome extension lets the agent control your **existing Chrome tabs** (your normal Chrome window) instead of launching a separate openacosmi-managed Chrome profile.

Attach/detach happens via a **single Chrome toolbar button**.

## What it is (concept)

There are three parts:

- **Browser control service** (Gateway or node): the API the agent/tool calls (via the Gateway)
- **Local relay server** (loopback CDP): bridges between the control server and the extension (`http://127.0.0.1:18792` by default)
- **Chrome MV3 extension**: attaches to the active tab using `chrome.debugger` and pipes CDP messages to the relay

Crab Claw（蟹爪） then controls the attached tab through the normal `browser` tool surface (selecting the right profile).

## Install / load (unpacked)

1. 安装扩展到本地稳定路径（Rust CLI）: 参见 `docs/cli/browser.md`
2. Chrome → `chrome://extensions` → Enable "Developer mode" → "Load unpacked" → 选择安装目录
3. Pin the extension.

升级 Crab Claw（蟹爪） 后在 `chrome://extensions` 中 Reload 扩展即可。

## Use it

系统内置 `chrome` profile 指向扩展中继的默认端口。Agent 工具中使用 `browser` 工具 + `profile="chrome"` 参数。

自定义 profile 配置参见 `docs/cli/browser.md`。

## Attach / detach (toolbar button)

- Open the tab you want Crab Claw（蟹爪） to control.
- Click the extension icon. Badge shows `ON` when attached.
- Click again to detach.

## Which tab does it control?

- It does **not** automatically control "whatever tab you're looking at".
- It controls **only the tab(s) you explicitly attached** by clicking the toolbar button.
- To switch: open the other tab and click the extension icon there.

## Badge + common errors

| Badge | 含义 |
|-------|------|
| `ON` | 已附加，agent 可驱动 |
| `…` | 正在连接本地 relay |
| `!` | relay 不可达（常见: relay 服务未运行） |

If you see `!`: Make sure the Gateway is running locally, or run a node host on this machine if the Gateway runs elsewhere.

## Sandboxing (tool containers)

如果 agent session 是沙箱模式，browser 工具默认目标为沙箱。允许 host 控制:

```json5
{
  agents: {
    defaults: {
      sandbox: {
        browser: {
          allowHostControl: true,
        },
      },
    },
  },
}
```

## Security implications (read this)

This is powerful and risky. Treat it like giving the model "hands on your browser".

- The extension uses Chrome's debugger API (`chrome.debugger`). When attached, the model can:
  - click/type/navigate in that tab
  - read page content
  - access whatever the tab's logged-in session can access
- **This is not isolated** like the dedicated openacosmi-managed profile.

Recommendations:

- Prefer a dedicated Chrome profile for extension relay usage.
- Keep the Gateway and any node hosts tailnet-only.
- Avoid exposing relay ports over LAN or public Internet.

## CLI 参考

完整 CLI 命令列表参见 `docs/cli/browser.md`（Rust CLI）。
