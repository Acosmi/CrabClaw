---
name: browser-linux-troubleshooting
description: "Fix Chrome/Brave/Edge/Chromium CDP startup issues for Crab Claw（蟹爪） browser control on Linux"
---

# Browser Troubleshooting (Linux)

## Problem: "Failed to start Chrome CDP on port 18800"

Crab Claw（蟹爪）'s browser control server fails to launch Chrome/Brave/Edge/Chromium with the error:

```
{"error":"Error: Failed to start Chrome CDP on port 18800 for profile \"openacosmi\"."}
```

### Root Cause

On Ubuntu (and many Linux distros), the default Chromium installation is a **snap package**. Snap's AppArmor confinement interferes with how Crab Claw（蟹爪） spawns and monitors the browser process.

`apt install chromium` installs a stub package that redirects to snap — NOT a real browser.

### Solution 1: Install Google Chrome (Recommended)

Install the official Google Chrome `.deb` package:

```bash
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo dpkg -i google-chrome-stable_current_amd64.deb
sudo apt --fix-broken install -y
```

Then update config (`~/.openacosmi/openacosmi.json`):

```json
{
  "browser": {
    "enabled": true,
    "executablePath": "/usr/bin/google-chrome-stable",
    "headless": true,
    "noSandbox": true
  }
}
```

### Solution 2: Snap Chromium + Attach-Only Mode

If you must use snap Chromium, configure attach-only mode:

```json
{
  "browser": {
    "enabled": true,
    "attachOnly": true,
    "headless": true,
    "noSandbox": true
  }
}
```

Start Chromium manually:

```bash
chromium-browser --headless --no-sandbox --disable-gpu \
  --remote-debugging-port=18800 \
  --user-data-dir=$HOME/.openacosmi/browser/openacosmi/user-data \
  about:blank &
```

Optional: create systemd user service for auto-start (`~/.config/systemd/user/openacosmi-browser.service`).

### Verifying

```bash
curl -s http://127.0.0.1:18791/ | jq '{running, pid, chosenBrowser}'
curl -s -X POST http://127.0.0.1:18791/start
curl -s http://127.0.0.1:18791/tabs
```

### Config Reference

| Option | Description | Default |
|--------|-------------|---------|
| `browser.enabled` | Enable browser control | `true` |
| `browser.executablePath` | Chromium-based browser binary path | auto-detected |
| `browser.headless` | Run without GUI | `false` |
| `browser.noSandbox` | Add `--no-sandbox` flag | `false` |
| `browser.attachOnly` | Don't launch browser, only attach | `false` |
| `browser.cdpPort` | Chrome DevTools Protocol port | `18800` |

### Problem: "Chrome extension relay is running, but no tab is connected"

Using the `chrome` profile (extension relay) but no tab is attached.

Fix: set `browser.defaultProfile: "openacosmi"` to use managed browser, or install the extension and click the icon to attach a tab.
