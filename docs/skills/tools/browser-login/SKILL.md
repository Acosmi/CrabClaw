---
name: browser-login
description: "Manual logins for browser automation + X/Twitter posting"
---

# Browser login + X/Twitter posting

## Manual login (recommended)

When a site requires login, **sign in manually** in the **host** browser profile (the managed Crab Claw browser profile, compatibility name `openacosmi`).

Do **not** give the model your credentials. Automated logins often trigger anti-bot defenses and can lock the account.

Primary Rust CLI name: `crabclaw`. The dedicated Chrome profile is still named
`openacosmi` for compatibility until browser profile/storage migration is done.

Back to the main browser docs: [Browser](/tools/browser).

## Which Chrome profile is used?

Crab Claw controls a **dedicated Chrome profile** (named `openacosmi`, orange-tinted UI for compatibility). This is separate from your daily browser profile.

Two easy ways to access it:

1. **Ask the agent to open the browser** — agent 使用 `browser` 工具的 `navigate` action 打开 URL。
2. **手动打开** — 通过 Rust CLI: 参见 `docs/cli/browser.md` 中的 `crabclaw browser start` / `open` 命令。

## X/Twitter: recommended flow

- **Read/search/threads:** use the **host** browser (manual login).
- **Post updates:** use the **host** browser (manual login).

## Sandboxing + host browser access

Sandboxed browser sessions are **more likely** to trigger bot detection. For X/Twitter (and other strict sites), prefer the **host** browser.

If the agent is sandboxed, the browser tool defaults to the sandbox. To allow host control:

```json5
{
  agents: {
    defaults: {
      sandbox: {
        mode: "non-main",
        browser: {
          allowHostControl: true,
        },
      },
    },
  },
}
```

Then use browser tool with `target="host"` parameter. Or disable sandboxing for the agent that posts updates.
