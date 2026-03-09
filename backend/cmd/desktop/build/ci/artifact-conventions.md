# 桌面产物命名约定

本文档定义桌面发布产物的标准命名与保留约定。

当前状态：

- 已启用的手动 workflow 当前消费以下平台：
  - `windows-nsis-amd64`
  - `linux-appimage-amd64`
- macOS 仍为手动发布 / 暂缓接入

## 1. 品牌命名

- 用户可见产品名：`Crab Claw`
- 带品牌归属说明的完整名称：`Crab Claw by Acosmi.ai`
- 技术标识、二进制与产物前缀：`CrabClaw`

## 2. Windows 产物

- 标准安装器前缀：`CrabClaw-windows`
- 示例：
  - `CrabClaw-windows-amd64-installer.exe`
  - `CrabClaw-windows-arm64-installer.exe`
- manifest platform key：
  - `windows-nsis-amd64`
  - `windows-nsis-arm64`
- `msix` 仍由系统托管，不写入 `update.json`

## 3. macOS 产物

- 标准归档前缀：`CrabClaw-macos`
- 示例：
  - `CrabClaw-macos-arm64.zip`
  - `CrabClaw-macos-amd64.zip`
- manifest platform key：
  - `macos-wails-arm64`
  - `macos-wails-amd64`
- 不直接把原始 `.app` 写入 `update.json`，应先归档

## 4. Linux 产物

- 标准归档前缀：`CrabClaw-linux`
- 示例：
  - `CrabClaw-linux-amd64.AppImage`
  - `CrabClaw-linux-arm64.AppImage`
  - `CrabClaw-linux-amd64.deb`
- manifest platform key：
  - `linux-appimage-amd64`
  - `linux-appimage-arm64`
- `deb` / `rpm` 仍由包管理器托管，不写入 `update.json`

## 5. Release metadata

- 使用 `wails3 task release:manifest` 基于产物生成 `release/<channel>/update.json`
- 将 `release/<channel>/SHA256SUMS` 与 manifest 一起发布
- 若自动探测无法判断平台 key，可传显式覆盖：
  - `-artifact key=/path/to/file`

## 6. 保留策略基线

- 分支验证产物：短期保留
- 正式发布产物：长期保留
- 未签名产物必须显式标记为 unsigned

## 7. 变更门槛

Windows / Linux 命名已经进入真实 workflow 输入。  
后续不要再单独改文件名或平台 key；若要调整，必须同步更新运行手册与验收清单。
