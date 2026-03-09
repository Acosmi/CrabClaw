# 桌面 Release 验收清单

当前桌面 release workflow 执行完成后，使用这份清单逐项验收。

## 1. Bundle 完整性

- 存在 `update.json`
- 存在 `SHA256SUMS`
- `update.json` 中的版本号与预期一致
- `update.json` 中的渠道与预期一致
- `update.json` 中包含 `windows-nsis-amd64`
- `update.json` 中包含 `linux-appimage-amd64`
- `update.json` 中的每个产物都出现在 `SHA256SUMS`
- `SHA256SUMS` 中的每条记录都被 `update.json` 引用
- 对下载后的 bundle 执行本地 `validate_release` 可以通过

## 2. 产物命名

- Windows 产物名为 `CrabClaw-windows-amd64-installer.exe`
- Linux 产物名为 `CrabClaw-linux-amd64.AppImage`
- `update.json` 中的文件名与实际上传文件名完全一致

## 3. 签名状态

- 存在 `WINDOWS_SIGNING_STATUS.txt`
- 如果是 unsigned 发布：
  - 状态文件明确说明未签名
- 如果启用了 Windows 签名：
  - 状态文件显示签名成功
  - 若配置了时间戳服务，状态文件中有记录
- 如果启用了 checksum 签名：
  - 存在 `SHA256SUMS.asc`
  - `SIGNING_STATUS.txt` 明确说明 detached signature 已生成

## 4. 发布结果

- 所有文件都已上传到真实发布源
- 文件发布路径与 `base_url` 完全一致
- `<base_url>/update.json` 可访问
- `update.json` 中的下载链接可访问

## 5. 客户端可用性

- 桌面客户端 `update.sourceURL` 可以指向已发布目录
- 客户端可以拉取该 `update.json`
- 客户端能够将当前安装形态匹配到某个已发布平台

## 6. 延后范围确认

- macOS 自动发布仍不属于当前 workflow
- Windows `MSIX` 仍为系统托管
- Linux `deb/rpm` 仍为包管理器托管
