# OpenAcosmi 沙箱架构设计与使用说明

本文档详细介绍了 OpenAcosmi 项目中的核心沙箱（Sandbox）架构选型、设计理念，以及 Native Rust CLI 与 Docker 之间的优先级关系和使用说明。

## 1. 架构核心结论

在 OpenAcosmi 的架构设计中，**原生 Rust CLI 沙箱是绝对的第一优先级（Primary），而 Docker 容器仅仅是备用与特定场景的补充（Fallback / Specialized）**。

在网关启动流程（`backend/internal/gateway/boot.go`）中，系统会优先探测并拉起基于 Rust 的原生 `openacosmi` 进程作为沙箱 Worker。只有当原生二进制文件彻底丢失或不可用时，系统才会勉强初始化 Docker 容器池进行兜底。

## 2. 为什么优先使用 Rust 原生沙箱？

在代码 `native_bridge.go` 中，明确指出了放弃 Docker 作为默认选项，全面转向 Rust CLI 的三大核心原因：

1. **极致性能与超低延迟**
   作为持久化的后台子进程存在，Rust Worker 使用标准输入输出（stdin/stdout）进行高效的 JSON-Lines IPC 通信。每次命令调用的响应延迟 **< 1ms**。
   *(对比：即便通过 Docker API 启动常驻容器执行 `exec`，平均延迟也在 **~215ms** 级别，差了整整个两个数量级)*。
2. **操作系统级的原生安全隔离**
   Rust 原生沙箱并不依赖容器技术，而是直接在各操作系统的最底层进行硬隔离：
   - macOS: Seatbelt / App Sandbox
   - Windows: AppContainer
   - Linux: Landlock + Seccomp
3. **零依赖的轻量化部署**
   在桌面端（如 Mac/Windows 端）部署时，用户可以直接运行 AI 系统，**完全不需要安装庞大沉重的 Docker Desktop**。

## 3. Docker 的定位与保留场景

如果在源码中看到诸如 `HybridExecutor`、`DockerRunner` 或多个带 `sandbox` 字样的 `Dockerfile`，这并不代表系统依赖 Docker。它们被保留下来仅用于以下两种情况：

1. **环境缺失兜底 (Fallback 底层保护)**
   当系统运行在特殊的云端裸机服务器上，且没有预埋静态编译好的 `openacosmi` Rust CLI 运行时，系统会激活 Docker 机制防止核心功能大面积宕机。
2. **重型特殊任务专精 (Specialized Data & Browser Isolation)**
   Rust 原生沙箱虽然轻量，但不包含复杂的生态运行环境。如果 AI Agent 决定需要执行：
   - 使用了 `numpy`, `pandas` 甚至深度学习框架的复杂 Python 数据处理代码。
   - 唤起带有 Chromium 无头浏览器的网页截图和分析爬虫。
   遇到这种任务，即使 Rust CLI 可用，代码中的 `executeDataProcessing` 等特定方法依然会强制唤启专门的 Docker 镜像（如 `python:3.12-alpine` 或 `Dockerfile.sandbox-browser`）来处理这些极度复杂的环境依赖。

## 4. 开发者配置与使用说明

如果您正在进行开发调试，或者希望自定义沙箱的运行逻辑，请参考以下配置和环境变量：

### 4.1 核心环境变量

系统在网关启动时（见 `resolveNativeSandboxBinaryPath()`），会按照以下优先级侦听并装载 Rust Sandbox：

1. **`$OA_CLI_BINARY`**: 开发者覆盖配置（最高优先级）
2. **`~/.openacosmi/bin/openacosmi`**: 用户级安装目录
3. **`openacosmi`**: 系统 `PATH` 环境变量全局查找

👉 **如果您想强制指定使用自己编译好的 Rust 调试版本：**

```bash
export OA_CLI_BINARY="/path/to/your/OpenAcosmi-rust+go/cli-rust/target/debug/openacosmi"
npm run dev # 或 go run main.go
```

👉 **如果您想测试 Docker 兜底逻辑：**
您可以故意设定一个不存在的路径来欺骗网关，强制让其判定 Rust 失败从而使用 Docker：

```bash
export OA_CLI_BINARY="/tmp/non-existent-binary"
npm run dev
```

### 4.2 审查与修改代码的关键文件清单

如需了解底层的具体实现或排查问题，请定位到以下核心模块：

- **`backend/internal/gateway/boot.go`**: 沙箱优先级的决策与加载入口 (`resolveNativeSandboxBinaryPath`)。
- **`backend/internal/sandbox/native_bridge.go`**: Rust 原生沙箱进程的 IPC 通信桥、生命周期与高可用崩溃重启拉起机制。
- **`backend/internal/sandbox/docker.go` & `docker_runner.go`**: Docker 兜底环境的容器调度、管理和拉起实现。
