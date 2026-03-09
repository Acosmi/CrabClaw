---
name: browser
description: "Web automation via CDP: navigate, observe, click/fill by ARIA ref, screenshot, JS evaluate"
tools: browser
metadata:
  tree_id: "ui/browser"
  tree_group: "ui"
  min_tier: "task_light"
  approval_type: "none"
---

# Browser — Web Automation via CDP

## Actions

| Action | Description | Returns |
|--------|-------------|---------|
| `navigate` | Go to URL | Screenshot |
| `observe` | **Start here** — ARIA tree + ref annotations (e1, e2...) + screenshot | ARIA + screenshot |
| `get_content` | ARIA tree or innerText | Text |
| `click_ref` | Click element by ARIA ref (preferred over CSS) | Screenshot |
| `fill_ref` | Type into input by ARIA ref | Screenshot |
| `click` | Click by CSS selector (fallback) | Screenshot |
| `type` | Type by CSS selector (fallback) | Screenshot |
| `screenshot` | Capture page (JPEG quality 75) | Image |
| `evaluate` | Execute JavaScript (requires `browser.evaluateEnabled`) | Result |
| `wait_for` | Wait for CSS selector (10s timeout) | — |
| `go_back` / `go_forward` | Navigation history | Screenshot |
| `get_url` | Current page URL | URL |
| `ai_browse` | Intent-level goal, auto observe→plan→act (max 20 steps) | Result |

## Recommended Workflow

1. `observe` → get ARIA tree with ref annotations
2. `click_ref` / `fill_ref` → interact using refs (more robust than CSS selectors)
3. `screenshot` → verify results

## Profiles

| Profile | Description | Use Case |
|---------|-------------|----------|
| `openacosmi` | Dedicated isolated browser (separate user data dir) | Default, safe |
| `chrome` | Extension relay (control existing Chrome tab) | Access logged-in sessions |

## Extension Relay (Chrome Profile)

- Extension attaches to active tab via `chrome.debugger`
- Bridges via loopback CDP relay (default `127.0.0.1:18792`)
- Badge: `ON` = attached, `…` = connecting, `!` = relay unreachable
- **Powerful and risky** — equivalent to "hands on your browser"

## Security

- `browser.evaluateEnabled=false` (default) disables JS execution
- Manual login preferred — automated logins trigger anti-bot defenses
- Extension relay = full access to logged-in sessions; keep Gateway on tailnet
- Screenshots: JPEG quality 75 + `optimizeForSpeed` — 3-5x smaller than PNG
