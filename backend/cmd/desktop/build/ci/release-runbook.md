# 桌面 Release 运行手册

本文档说明如何准备、触发并验收当前已启用的桌面发布 workflow：`.github/workflows/desktop-release.yml`。

当前适用范围：

- Windows `NSIS` amd64 安装器
- Linux `AppImage` amd64 产物
- 汇总生成一份 `update.json`
- 汇总生成一份 `SHA256SUMS`
- 可选：
  - Windows 安装器签名
  - `SHA256SUMS` 的 GPG detached signature

暂不纳入：

- macOS 自动发布
- Windows `MSIX`
- Linux `deb/rpm` 正式发布

## 1. 触发前检查

触发 workflow 前，至少确认：

- 桌面更新客户端仍使用以下 platform key：
  - `windows-nsis-amd64`
  - `linux-appimage-amd64`
- 当前产物命名仍符合 `artifact-conventions.md`
- 目标 `version` 尚未发布
- `base_url` 已经指向最终发布地址
- 本地 UI 可构建：
  - 在 `ui` 下执行 `npm run build`
- 桌面发布工具链通过：
  - 在 `backend` 下执行 `go test ./cmd/desktop/...`

## 2. Secret 准备

可选 Secret 见 `signing-secrets.md`。

最小 unsigned 发布：

- 无需额外 Secret

Windows 安装器签名：

- `DESKTOP_WINDOWS_CERT_BASE64`
- `DESKTOP_WINDOWS_CERT_PASSWORD`
- 可选 `DESKTOP_WINDOWS_SIGNTOOL_TIMESTAMP_URL`

校验文件签名：

- `DESKTOP_RELEASE_GPG_PRIVATE_KEY`
- `DESKTOP_RELEASE_GPG_PASSPHRASE`

## 3. 本地预跑

触发 GitHub Actions 前，先执行：

```bash
cd /Users/fushihua/Desktop/CrabClaw/ui
npm run build
```

```bash
cd /Users/fushihua/Desktop/CrabClaw/backend
go test ./cmd/desktop/...
```

如果本地已有 staged release bundle，可继续执行：

```bash
cd /Users/fushihua/Desktop/CrabClaw/backend/cmd/desktop
wails3 task release:validate VERSION=<version> CHANNEL=<channel> ARTIFACTS_DIR=<artifacts-dir> OUTPUT_DIR=<release-dir>
```

## 4. 手动触发参数

手动触发 `.github/workflows/desktop-release.yml` 时需要传入：

- `version`
  - 例如：`1.2.3`
- `channel`
  - 可选：`stable`、`beta`、`dev`
- `base_url`
  - 例如：`https://downloads.example.com/releases/1.2.3`

预期 job 顺序：

1. `windows-amd64`
2. `linux-amd64`
3. `publish`

## 5. 预期输出

平台分组产物：

- `desktop-release-windows-amd64`
- `desktop-release-linux-amd64`

汇总 release bundle：

- `CrabClaw-windows-amd64-installer.exe`
- `CrabClaw-linux-amd64.AppImage`
- `update.json`
- `SHA256SUMS`
- 可选 `SHA256SUMS.asc`
- `RELEASE_SCOPE.txt`
- 签名状态文本

## 6. 发布后复核

workflow 成功后，至少执行以下检查：

1. 下载汇总后的 release bundle
2. 确认 `update.json` 同时包含两个平台 key
3. 确认 `SHA256SUMS` 覆盖两个平台产物
4. 确认 `SIGNING_STATUS.txt` 与本次发布模式一致
5. 如果启用了 Windows 签名：
   - 检查 `WINDOWS_SIGNING_STATUS.txt`
   - 确认其中显示有效的 Authenticode 签名
6. 如有必要，再次本地校验：

```bash
cd /Users/fushihua/Desktop/CrabClaw/backend/cmd/desktop
go run ./build/tools/validate_release \
  -manifest ./release/<channel>/update.json \
  -checksums ./release/<channel>/SHA256SUMS \
  -artifacts-dir <downloaded-artifacts-dir> \
  -version <version> \
  -channel <channel>
```

## 7. 手动发布步骤

当前 workflow 只负责在 GitHub Actions 中组装并上传 release bundle，不会自动发布到真实下载源。

仍需手动完成：

- 将 release 文件上传到 `base_url` 对应的真实目录
- 保持文件名与 workflow 产出完全一致
- 将 `update.json` 与 `SHA256SUMS` 一并上传
- 如果启用了 checksum 签名，也上传 `SHA256SUMS.asc`

## 8. 故障排查

如果 `windows-amd64` 失败：

- 检查 runner 上是否成功安装 `NSIS`
- 检查是否找到 `signtool.exe`
- 检查 PFX 证书 base64 与密码是否正确

如果 `linux-amd64` 失败：

- 检查 Linux 打包依赖
- 检查 `backend/cmd/desktop/bin` 中是否真的产出了 `.AppImage`

如果 `publish` 失败：

- 检查 `update.json`
- 检查 `SHA256SUMS`
- 将下载后的产物重新执行一次 `validate_release`

## 9. 完成标准

只有满足以下条件，才可视为 release bundle 可发布：

- workflow 成功
- release bundle 校验通过
- 上传文件名与 manifest 完全一致
- 签名状态符合本次发布策略
- 文件已真正发布到 `base_url`
