# 桌面 CI 激活检查清单

这份清单用于定义桌面 CI 模板从“文档方案”进入“真实可执行 workflow”前必须满足的门槛。

当前状态：

- `.github/workflows/desktop-release.yml` 已作为 Windows amd64 / Linux amd64 的手动发布 workflow 启用
- macOS 自动发布仍暂缓

## 1. 命名冻结

- 确认用户可见产品名统一为 `Crab Claw`
- 确认桌面宿主二进制名统一为 `CrabClaw`
- 确认遗留兼容命名不会重新出现在安装器元数据和发布产物中

## 2. 构建输入

- 确认 `dist/control-ui` 仍是权威 UI staging 输出
- 确认 `scripts/desktop/stage_control_ui.sh` 仍是 staging 脚本
- 确认 `backend/cmd/desktop/frontend/dist` 仍是桌面嵌入目录

## 3. 工具链固定

- 固定 Go 版本
- 固定 Node 版本
- 固定 Wails CLI 版本
- 确认 Linux runner 的打包依赖仍完整

## 4. 发布输出

- 明确 Windows 产物标准文件名
- 明确 Linux 产物标准文件名
- 明确上传、保留期与发布策略

## 5. Secret 与签名

- 确认本轮是否要求签名
- 若要求签名，明确 Secret 名称与 runner 前提
- 若不要求签名，明确 unsigned 产物的状态标识

## 6. 运行安全

- 确认 workflow 不会在未经批准的情况下触发实验性桌面路径
- 确认 workflow 失败不会影响现有 Gateway 与 macOS 发布主线

## 7. 当前激活结论

当前范围已完成：

- `.github/workflows/desktop-release.yml` 已启用
- Windows / Linux 的打包命令已接入

未来扩容前仍需完成：

- macOS 仅在宿主归属明确后接入
- 扩 runner 矩阵前，先同步审查命名与签名策略
