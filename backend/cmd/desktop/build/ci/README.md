# 桌面 Release 资料目录

此目录用于存放桌面壳的 CI 模板、已启用的发布运行手册、激活检查项与签名约定。

当前状态：

- Windows / Linux 手动发布 workflow 已启用：`.github/workflows/desktop-release.yml`
- macOS 自动发布仍暂缓
- 其余模板与约定文档继续放在这里集中维护

在继续触发 workflow 或扩大发布矩阵前，至少复核以下事项：

- 桌面二进制、安装器与发布产物命名是否已统一为 `Crab Claw`
- Wails CLI、Go、Node 版本是否仍与 workflow 一致
- 签名与发布 Secret 是否完整
- 平台 runner 依赖是否齐备
- UI staging 路径是否仍稳定
- 产物命名约定：`artifact-conventions.md`
- manifest 生成命令：`wails3 task release:manifest`
- release bundle 校验命令：`wails3 task release:validate`
- 发布执行步骤：`release-runbook.md`
- 发布验收清单：`release-acceptance-checklist.md`
- 签名 Secret 说明：`signing-secrets.md`
- 平台更新归属：
  - `windows-nsis-*`、`macos-wails-*`、`linux-appimage-*` 属于应用自管更新
  - `windows-msix-*` 与 `linux-system-package-*` 仍属于系统或包管理器托管，不应写入 `update.json`

当前发布链的使用方式：

- 将桌面产物输出到 `backend/cmd/desktop/bin`
- 生成 `release/<channel>/update.json` 与 `SHA256SUMS`
- 将 manifest、checksums 与实际安装包发布到同一目录
- 如自动探测无法识别某个产物，可通过 `EXTRA_ARGS` 显式补映射，例如：
  - `-artifact macos-wails-arm64=./bin/CrabClaw-macos-arm64.zip`

当前仅 Windows / Linux 发布组装已激活。  
macOS 自动发布仍待桌面宿主主线完全收口后再接入。
