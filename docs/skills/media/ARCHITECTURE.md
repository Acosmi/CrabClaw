---
document_type: Architecture
status: Updated
created: 2026-03-02
last_updated: 2026-03-07
audit_report: tracking-media-subagent-audit-fixes-2026-03-07
skill5_verified: true
---

# oa-media 子智能体架构文档

## 1. 概述

oa-media 是 OpenAcosmi 三级指挥体系中的**功能子智能体**，专职媒体运营。
通过委托合约从主智能体接收任务，自主完成热点追踪、内容创作、多平台发布和社交互动。

**定位**: 主智能体 → `spawn_media_agent` → oa-media（独立 LLM 会话 + 专属工具集）

**支撑平台**:
- 微信公众号（API 模式）
- 小红书（RPA 浏览器自动化模式）
- 自有网站（CMS API 模式）

---

## 2. 系统拓扑

```
┌──────────────────────────────────────────────────────────────────────┐
│                         主智能体 (Main Agent)                        │
│                                                                      │
│  spawn_media_agent { task_brief, scope, constraints, timeout_ms }    │
│           ↓                                                          │
│   DelegationContract (合约) + MonotonicDecay (权限衰减)               │
└──────────┬───────────────────────────────────────────────────────────┘
           │ SpawnSubagent(contract, systemPrompt, agentType="media")
           ↓
┌──────────────────────────────────────────────────────────────────────┐
│                       oa-media 子智能体                               │
│                                                                      │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────┐  ┌────────────┐│
│  │ trending     │  │ content      │  │ media       │  │ social     ││
│  │ _topics      │  │ _compose     │  │ _publish    │  │ _interact  ││
│  └──────┬──────┘  └──────┬───────┘  └──────┬──────┘  └─────┬──────┘│
│         │                │                  │                │       │
│  ┌──────┴──────┐  ┌──────┴───────┐  ┌──────┴──────┐  ┌─────┴──────┐│
│  │ Trending    │  │ DraftStore   │  │ Publishers  │  │ Interactor ││
│  │ Aggregator  │  │ (File-based) │  │ (per-plat)  │  │ (RPA)      ││
│  └──────┬──────┘  └──────────────┘  └──────┬──────┘  └────────────┘│
│         │                                   │                       │
│  ┌──────┴──────────────────┐         ┌──────┴──────────────────┐    │
│  │ weibo │ baidu │ zhihu  │         │ wechat_mp │ xiaohongshu │    │
│  │ source│ source│ source │         │ (API)     │ (RPA/CDP)   │    │
│  └────────────────────────┘         └─────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘
           │
           ↓ Gateway RPC
┌──────────────────────────────────────────────────────────────────────┐
│                         Web UI / 前端                                │
│                                                                      │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐        │
│  │ 热点面板      │  │ 草稿面板      │  │ 发布状态面板      │        │
│  │ (Trending)    │  │ (Drafts)      │  │ (Publish)         │        │
│  └───────────────┘  └───────────────┘  └───────────────────┘        │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. 核心包结构

### 3.1 `backend/internal/media/` — 媒体核心包（44 源文件 + 15 understanding 子包）

| 分类 | 文件 | 职责 |
|------|------|------|
| **引导** | `bootstrap.go` | `NewMediaSubsystem()` — 子系统初始化入口 |
| **类型与常量** | `types.go`, `constants.go` | Platform/Style/Status 枚举 + 核心数据结构 + 常量 |
| **工具框架** | `media_tool.go` | `MediaTool` 定义（镜像 `tools.AgentTool`） |
| **工具注册** | `media_registry.go` | 工具名常量 + `DefaultMediaToolDefs()` |
| **热点** | `trending.go` | `TrendingAggregator` + `TrendingSource` 接口 |
| **热点源** | `trending_source_weibo.go` | 微博热搜 API |
| | `trending_source_baidu.go` | 百度热搜 API（支持分类） |
| | `trending_source_zhihu.go` | 知乎热榜 API |
| **热点工具** | `trending_tool.go` | `trending_topics` 工具（fetch/analyze/list_sources） |
| **草稿** | `draft_store.go` | `DraftStore` 接口 + `FileDraftStore` 文件存储 |
| **创作工具** | `content_compose_tool.go` | `content_compose` 工具（draft/preview/revise/list） |
| **发布** | `publish_tool.go` | `media_publish` 工具 |
| | `publish_history.go` | `PublishHistoryStore` 发布记录持久化 |
| **互动工具** | `social_interact_tool.go` | `social_interact` 工具 + context-aware 限速 |
| **系统提示词** | `system_prompt.go` | 12-section `BuildMediaSystemPrompt()` |
| **图像处理** | `image_ops.go` | 图片缩放/裁切/格式转换 |
| | `image_describe.go` | 图片描述接口 |
| | `image_describe_anthropic.go` | Anthropic 视觉描述 |
| | `image_describe_openai.go` | OpenAI 视觉描述 |
| | `image_exif.go` | EXIF 元数据提取 |
| | `image_orient.go` | 方向校正（EXIF orientation） |
| **音频** | `audio.go` | 音频处理基础 |
| | `audio_tags.go` | 音频标签/元数据 |
| **语音转文字 (STT)** | `stt.go` | STT 接口定义 |
| | `stt_openai.go` | OpenAI Whisper 实现 |
| | `stt_dashscope.go` | 阿里 DashScope 实现 |
| | `stt_local.go` | 本地 STT 实现 |
| **文档转换** | `docconv.go` | 文档转换接口 |
| | `docconv_builtin.go` | 内置转换器 |
| | `docconv_mcp.go` | MCP 远程转换器 |
| **文件输入** | `input_files.go` | 多模态文件输入处理 |
| | `input_files_pdf.go` | PDF 文件处理 |
| | `mime.go` | MIME 类型检测与映射 |
| | `parse.go` | 内容解析 |
| **状态与存储** | `state_store.go` | `MediaStateStore` 状态持久化 |
| | `store.go` | 通用存储抽象 |
| | `store_io.go` | 存储 I/O 操作 |
| **调度与事件** | `media_cron.go` | 定时任务调度 |
| | `event_trigger.go` | 事件触发器 |
| | `opportunity_evaluator.go` | 发布时机评估 |
| **网络** | `fetch.go` | HTTP 抓取工具 |
| **服务** | `server.go` | 媒体服务器入口 |
| | `host.go` | 宿主环境抽象 |

### 3.1.1 `backend/internal/media/understanding/` — 多模态理解子包（15 源文件）

| 文件 | 职责 |
|------|------|
| `types.go` | 理解任务/结果类型 |
| `registry.go` | Provider 注册表 |
| `resolve.go` | Provider 自动解析 |
| `runner.go` | 理解任务执行器 |
| `scope.go` | 能力范围定义 |
| `concurrency.go` | 并发控制 |
| `defaults.go` | 默认配置 |
| `video.go` | 视频理解处理 |
| `provider_anthropic.go` | Anthropic Claude 视觉 |
| `provider_openai.go` | OpenAI GPT-4V |
| `provider_google.go` | Google Gemini |
| `provider_groq.go` | Groq 实现 |
| `provider_minimax.go` | MiniMax 实现 |
| `provider_deepgram.go` | Deepgram 语音 |
| `provider_image.go` | 通用图片 Provider |

### 3.2 频道实现

| 频道包 | 文件 | 模式 |
|--------|------|------|
| `channels/xiaohongshu/` | `config.go` | Cookie 路径 + 限速配置 |
| | `rpa_client.go` | 10 步 RPA 发布（CDP 浏览器驱动） |
| | `interactions.go` | 评论/私信 RPA 操作 + LRU 去重 |
| | `browser_adapter.go` | `BrowserDriver` 接口 + `CDPBrowserAdapter` |
| | `playwright_adapter.go` | Playwright 四层桥接适配器 |
| | `plugin.go` | 频道插件注册 + `AllClients()` |
| `channels/wechat_mp/` | `config.go` | AppID + AppSecret |
| | `client.go` | Token 管理 + 图片上传 + 限速 |
| | `publish.go` | CreateDraft → SubmitPublish → GetPublishStatus |
| | `plugin.go` | 频道插件注册 |

### 3.3 集成层

| 包 | 文件 | 职责 |
|----|------|------|
| `agents/runner/` | `spawn_media_agent.go` | 工具定义 + 合约创建 + 子智能体生成 |
| `gateway/` | `server_methods_media.go` | 15 个 media.* RPC 方法 |
| `gateway/` | `server.go` | MediaSubsystem 注入 + RPC 注册 |

---

## 4. 工具架构

### 4.1 工具定义模型

```
MediaTool {
    ToolName    string               // 工具标识（LLM 调用名）
    ToolLabel   string               // 显示名
    ToolDesc    string               // LLM 描述
    ToolParams  any                  // JSON Schema (map[string]any)
    ToolExecute func(ctx, id, args)  // 执行函数
}
```

与主系统 `tools.AgentTool` **结构对齐**但**包级隔离**，避免循环依赖。
集成时由 `MediaSubsystem.GetToolDef()` / `ExecuteTool()` 统一桥接。

### 4.2 四工具矩阵

| 工具 | 常量 | Actions | 依赖 |
|------|------|---------|------|
| `trending_topics` | `ToolTrendingTopics` | fetch, analyze, list_sources | TrendingAggregator |
| `content_compose` | `ToolContentCompose` | draft, preview, revise, list | DraftStore |
| `media_publish` | `ToolMediaPublish` | publish, status, approve | DraftStore + Publishers |
| `social_interact` | `ToolSocialInteract` | list_comments, reply_comment, list_dms, reply_dm | SocialInteractor (RPA) |

### 4.3 工具启用控制

```go
// bootstrap.go
tools := []*MediaTool{
    CreateTrendingTool(agg),       // 始终启用
    CreateContentComposeTool(store), // 始终启用
}
if cfg.EnablePublish {
    tools = append(tools, CreateMediaPublishTool(store, publishers))
}
if cfg.EnableInteract {
    tools = append(tools, CreateSocialInteractTool(nil))
}
```

---

## 5. 热点数据源架构

### 5.1 TrendingSource 接口

```go
type TrendingSource interface {
    Name() string
    Fetch(ctx context.Context, category string, limit int) ([]TrendingTopic, error)
}
```

### 5.2 三源实现

| 源 | API 端点 | 特点 |
|---|---------|------|
| **weibo** | `weibo.com/ajax/side/hotSearch` | 免 Key，`data.realtime[]`，热度 `raw_hot` |
| **baidu** | `top.baidu.com/api/board?tab=realtime` | 支持分类 tab（tech/finance/entertainment），热度 `hotScore` |
| **zhihu** | `zhihu.com/api/v3/feed/topstory/hot-lists/total` | 需 User-Agent，热度从 `detail_text` 正则提取（"万热度" 格式） |

### 5.3 聚合器

```
TrendingAggregator
  ├── AddSource(TrendingSource)
  ├── FetchAll(ctx, category, limit)    → 并发拉取全部源
  ├── FetchBySource(ctx, name, cat, lim) → 定向单源
  └── SourceNames()                     → 已注册源名列表
```

`FetchAll` 使用 `sync.WaitGroup` 并发拉取，按 `HeatScore` 降序合并。

---

## 6. 草稿存储

### 6.1 DraftStore 接口

```go
type DraftStore interface {
    Save(draft *ContentDraft) error
    Get(id string) (*ContentDraft, error)
    List(platformFilter string) ([]*ContentDraft, error)
    Delete(id string) error
    UpdateStatus(id string, status DraftStatus) error
}
```

### 6.2 FileDraftStore 实现

- 存储路径: `{workspace}/_media/drafts/{id}.json`
- 每条草稿独立 JSON 文件
- 读写加 `sync.Mutex` 保护
- `List()` 支持按 `platform` 过滤

### 6.3 草稿生命周期

```
draft → pending_review → approved → published
                ↓
            (rejected → 可重新修改)
```

审批门控在 system prompt 中强制要求，发布前必须通过 `status` action 确认 `approved`。

---

## 7. 发布架构

### 7.1 MediaPublisher 接口

```go
type MediaPublisher interface {
    Publish(ctx context.Context, draft *ContentDraft) (*PublishResult, error)
}
```

由各平台频道包实现，通过 `MediaSubsystem.RegisterPublisher()` 注入。

### 7.2 微信公众号 — API 模式

```
access_token 管理               发布流水线
┌───────────────────┐          ┌──────────────────────────────┐
│ 2h TTL            │          │ 1. UploadImage() → media_id  │
│ 5min 提前刷新     │  ──→     │ 2. CreateDraft() → draft_id  │
│ 50ms 请求间隔     │          │ 3. SubmitPublish() → pub_id  │
│ 持久化缓存(可选)  │          │ 4. GetPublishStatus() 轮询   │
└───────────────────┘          │    → 0=成功 / 1=进行中 / 2+=失败│
                               └──────────────────────────────┘
```

**约束**: JPG/PNG ≤1MB | 标题 ≤64 字 | HTML 正文 | 订阅号每天 1 次群发

### 7.3 小红书 — RPA 模式

```
BrowserDriver 接口                    RPA 发布 10 步
┌───────────────────────────┐        ┌──────────────────────────────┐
│ Navigate(url)             │        │ 1. 检查 browser + cookies    │
│ SetCookies(cookies)       │  ──→   │ 2. Navigate → creator.xhs   │
│ FillBySelector(sel, val)  │        │ 3. 注入 Cookie → 刷新       │
│ ClickBySelector(sel)      │        │ 4. 等待 .ql-editor 加载     │
│ UploadFile(sel, path)     │        │ 5. 上传图片 input[type=file] │
│ WaitForElement(sel, ms)   │        │ 6. 填写标题 #title           │
│ GetPageText()             │        │ 7. 填写正文 .ql-editor       │
│ Screenshot()              │        │ 8. 添加标签 #话题#           │
│ EvaluateJS(expr)          │        │ 9. 点击发布 .publish-btn     │
└───────────────────────────┘        │10. 等待确认 .publish-success │
       ↑                             └──────────────────────────────┘
CDPBrowserAdapter
(委托 browser.PlaywrightTools)
```

**约束**: Cookie 鉴权 | 标题 ≤20 字 | 正文 ≤1000 字 | 图片 ≤9 张 | 操作间隔 ≥5 秒（最低 3 秒）+ 随机延迟

---

## 8. 社交互动架构

### 8.1 SocialInteractor 接口

```go
type SocialInteractor interface {
    ListComments(ctx, noteID string) ([]InteractionItem, error)
    ReplyComment(ctx, noteID, commentID, reply string) error
    ListDMs(ctx context.Context) ([]InteractionItem, error)
    ReplyDM(ctx, userID, message string) error
}
```

### 8.2 RPA 互动实现

当前仅支持小红书（硬编码 `RPAInteractionManager`）：

- **ListComments**: 导航笔记页 → JS 提取评论 DOM → 映射 `InteractionItem`
- **ReplyComment**: 定位评论 → 点击回复按钮 → 输入内容 → 发送 → markProcessed
- **ListDMs**: 导航消息中心 → 解析私信列表
- **ReplyDM**: 打开对话 → 输入内容 → 发送 → markProcessed

所有操作在 `browser == nil` 时返回 `ErrNotImplemented` 作为降级。

### 8.3 去重

`processed map[string]struct{}` + `processedOrder []string` 记录已处理的 comment/DM ID，避免重复回复。
容量上限 10000 条，超限时淘汰最早一半（LRU 风格），防止长期运行内存无限增长。

---

## 9. 系统提示词架构

### 9.1 12-Section 构建

```go
func BuildMediaSystemPrompt(p MediaPromptParams) string {
    writeIdentity(&b, task)        // 1. 身份与角色
    writeCapabilities(&b)          // 2. 能力（7 个工具表）
    writeContentGuidelines(&b)     // 3. 内容创作指南（选题/文风/结构）
    writePlatformSpecs(&b)         // 4. 平台规范（微信/小红书/网站硬限制）
    writeHITLWorkflow(&b)          // 5. HITL 审批流程（强制门控）
    writeSocialRules(&b)           // 6. 社交互动规则（频率/去重/上报）
    writeToolUsage(&b)             // 7. 工具使用（工具链模式）
    writeQualityStandards(&b)      // 8. 质量标准（审查清单）
    writeTaskExecution(&b)         // 9. 任务执行（自主推进）
    writeOutputFormat(&b)          // 10. 输出格式（ThoughtResult JSON）
    writeBoundaries(&b)            // 11. 能力边界（禁止清单）
    writeSessionContext(&b, p)     // 12. 会话上下文（合约注入）
}
```

### 9.2 合约注入

`spawn_media_agent.go` → `buildMediaSystemPrompt()` → `MediaSubsystem.BuildSystemPrompt()`:

1. 合约 `FormatForSystemPrompt()` 生成格式化文本
2. `contractPromptAdapter` 适配为 `ContractFormatter` 接口
3. 注入到 Section 12 尾部

### 9.3 工具集

提示词中声明 7 个工具：`trending_topics` / `content_compose` / `media_publish` / `social_interact` / `web_search` / `web_fetch` / `image`

---

## 10. Gateway RPC 层

### 10.1 方法注册

```go
// server_methods_media.go
func MediaHandlers() map[string]GatewayMethodHandler {
    "media.trending.fetch"   → handleMediaTrendingFetch
    "media.trending.sources" → handleMediaTrendingSources
    "media.trending.health"  → handleMediaTrendingHealth
    "media.drafts.list"      → handleMediaDraftsList
    "media.drafts.get"       → handleMediaDraftsGet
    "media.drafts.delete"    → handleMediaDraftsDelete
    "media.drafts.update"    → handleMediaDraftsUpdate
    "media.drafts.approve"   → handleMediaDraftsApprove
    "media.publish.list"     → handleMediaPublishList
    "media.publish.get"      → handleMediaPublishGet
    "media.config.get"       → handleMediaConfigGet
    "media.config.update"    → handleMediaConfigUpdate
    "media.tools.list"       → handleMediaToolsList
    "media.tools.toggle"     → handleMediaToolsToggle
    "media.sources.toggle"   → handleMediaSourcesToggle
}
```

### 10.2 权限分级

| 方法 | 分类 | 说明 |
|------|------|------|
| `media.trending.fetch` | readMethods | 只读热点数据 |
| `media.trending.sources` | readMethods | 只读源列表 |
| `media.trending.health` | default (admin) | 热搜源健康检查 |
| `media.drafts.list` | readMethods | 列出草稿 |
| `media.drafts.get` | readMethods | 读取草稿 |
| `media.drafts.delete` | **writeMethods** | 删除草稿（破坏性写操作） |
| `media.drafts.update` | default (admin) | 更新草稿内容 |
| `media.drafts.approve` | default (admin) | 审批草稿 |
| `media.publish.list` | default (admin) | 列出发布记录 |
| `media.publish.get` | default (admin) | 查询发布详情 |
| `media.config.get` | default (admin) | 获取媒体配置 |
| `media.config.update` | default (admin) | 更新媒体配置 |
| `media.tools.list` | default (admin) | 列出工具启用状态 |
| `media.tools.toggle` | default (admin) | 切换工具启用 |
| `media.sources.toggle` | default (admin) | 切换热搜源启用 |

### 10.3 注入链路

```
server.go
  → NewMediaSubsystem(cfg)                    // 初始化
  → WsServerConfig { MediaSubsystem: sub }    // 传递到 WS 层
  → GatewayMethodContext { MediaSubsystem }   // 注入到 RPC 上下文
  → handler 通过 ctx.Context.MediaSubsystem 访问
```

---

## 11. 子智能体生成链路

### 11.1 完整调用链

```
主智能体 LLM 调用
  → spawn_media_agent { task_brief, scope, constraints }
    → createMediaContract()                    // 合约创建 + MonotonicDecay 验证
    → buildMediaSystemPrompt()                 // 12-section prompt + 合约注入
    → params.SpawnSubagent(SpawnSubagentParams) // 委托 Gateway
      → Gateway.SpawnSubagent()
        → 新 LLM 会话 (agentType="media")
        → 工具集: MediaSubsystem.GetToolDef() + 主系统工具子集
        → 执行循环: LLM → tool_use → ExecuteTool() → 结果 → LLM
      → ThoughtResult JSON 解析
    → formatMediaSpawnResult()                 // 结果格式化返回主智能体
```

### 11.2 合约安全

- **MonotonicDecay**: 子智能体权限不能超过父智能体（`ValidateMonotonicDecay()`）
- **NoNetwork**: `constraints.no_network=true` → 阻断网络访问
- **Timeout**: 默认 120 秒（比 coder 的 60 秒更长，适应媒体操作延迟）
- **Scope**: 文件路径 + 读写执行权限的显式声明

---

## 12. 技能系统

### 12.1 技能目录

```
docs/skills/media/
  ├── ARCHITECTURE.md              ← 本文档
  ├── media-trending/SKILL.md      ← 热点追踪技能（跨平台）
  ├── xiaohongshu-ops/SKILL.md     ← 小红书全流程技能
  └── wechat-mp-ops/SKILL.md       ← 微信公众号全流程技能
```

### 12.2 加载链路

```
LoadSkillEntries()
  → ResolveDocsSkillsDir()         // 定位 docs/skills/
  → 递归扫描 docs/skills/media/    // 发现 3 个 SKILL.md
  → ResolveSkillCategory()         // 路径提取 category = "media"
  → 分发到 agent_type = "media" 的子智能体
```

### 12.3 技能-工具映射

| 技能 | frontmatter tools | 对应工具常量 |
|------|-------------------|-------------|
| media-trending | `trending_topics` | `ToolTrendingTopics` |
| xiaohongshu-ops | `content_compose, media_publish, social_interact` | 三个工具 |
| wechat-mp-ops | `content_compose, media_publish` | 两个工具（无 social_interact） |

---

## 13. 前端 UI

### 13.1 媒体仪表盘

Tab: `"media"` → 路由 `/media` → `renderMediaDashboard(state)`

三面板布局:
1. **热点面板**: 源选择器（weibo/baidu/zhihu/all）+ 热点列表 + 热度分
2. **草稿面板**: 平台过滤（all/wechat/xiaohongshu）+ 草稿列表 + 状态标签 + 删除
3. **发布面板**: 发布历史 + 状态（published/publishing/failed）

### 13.2 RPC 调用

```typescript
// controllers/media-dashboard.ts
loadTrendingTopics(source?, category?, limit?)
  → state.client?.call("media.trending.fetch", params)

loadDraftsList(platform?)
  → state.client?.call("media.drafts.list", { platform })

deleteDraft(id)
  → state.client?.call("media.drafts.delete", { id })

updateDraft(id, fields)
  → state.client?.call("media.drafts.update", { id, ...fields })

approveDraft(id)
  → state.client?.call("media.drafts.approve", { id })

loadPublishHistory(limit?, offset?)
  → state.client?.call("media.publish.list", params)

loadConfig()
  → state.client?.call("media.config.get", {})

updateConfig(config)
  → state.client?.call("media.config.update", config)

loadTools()
  → state.client?.call("media.tools.list", {})

toggleTool(tool, enabled)
  → state.client?.call("media.tools.toggle", { tool, enabled })
```

---

## 14. 数据流总图

```
[微博/百度/知乎 API]
        ↓ HTTP GET
  TrendingAggregator.FetchAll()
        ↓ []TrendingTopic
  trending_topics 工具
        ↓ LLM 选题
  content_compose 工具 (draft)
        ↓ ContentDraft
  FileDraftStore.Save()
        ↓ draft_id
  回报主智能体 → 用户审批
        ↓ approved
  media_publish 工具 (publish)
        ↓
  ┌─────┴─────┐
  │ wechat_mp │ xiaohongshu
  │ API 发布   │ RPA 发布
  │           │
  │ Upload    │ Navigate
  │ Draft     │ Cookie
  │ Submit    │ Fill
  │ Poll      │ Click
  └─────┬─────┘
        ↓ PublishResult
  回报主智能体
```

---

## 15. 配置

### 15.1 微信公众号

```yaml
channels:
  wechat_mp:
    app_id: "wx..."
    app_secret: "..."
    token_cache_path: ""    # 可选：Token 持久化路径
```

### 15.2 小红书

```yaml
channels:
  xiaohongshu:
    cookie_path: "/path/to/cookies.json"
    rate_limit_seconds: 5    # 最低 3 秒
    error_screenshot_dir: "_media/xhs/errors/"
```

### 15.3 媒体子系统

```yaml
media:
  workspace: "/path/to/workspace"
  enable_publish: true
  enable_interact: true
```

---

## 16. 安全边界

| 层 | 机制 | 说明 |
|----|------|------|
| 合约层 | MonotonicDecay | 子智能体权限 ≤ 父智能体 |
| 合约层 | NoNetwork | 可禁止网络访问 |
| 合约层 | Timeout | 120 秒默认超时 |
| 审批层 | HITL 门控 | 发布前强制审批（system prompt 硬编码） |
| 平台层 | 内容校验 | `validatePlatformContent()` 字数/图片限制 |
| 平台层 | 频率限制 | XHS ≥5 秒间隔（context-aware timer）/ 微信 50ms API 限流 |
| 工具层 | action 白名单 | 各工具限定 action 枚举 |
| RPC 层 | read/write 分级 | `media.drafts.delete` 在 writeMethods |
| 提示词层 | 能力边界 | 禁止文件操作/bash/直接对话用户 |

---

## 17. 已知限制与延迟项

| 项 | 状态 | 说明 |
|----|------|------|
| Phase 6D 浏览器注入 | ✅ 已实现 | playwright_adapter.go 四层桥接 + server.go 接线 |
| XHS CSS 选择器 | 需验证 | RPA 选择器基于推测，需实际页面确认 |
| `processed` map 增长 | ✅ 已修复 | LRU 淘汰，上限 10000 条 |
| 微信评论管理 | 不支持 | `social_interact` 仅支持小红书 |
| 热搜 API 限流 | 无策略 | 公开 API 可能被限流 |
| noteID/commentID 注入 | 未校验 | RPA 操作中 ID 参数无输入验证 |
| `context.Background()` | ✅ 已修复 | `handleMediaTrendingFetch` 已改用 `MethodHandlerContext.Ctx` |

---

## 18. 文件索引

> 行数统计截至 2026-03-07

### 核心包 (`backend/internal/media/` — 44 源文件，~8400 行)

**工具与框架**

| 文件 | 行数 | 职责 |
|------|------|------|
| `bootstrap.go` | 297 | 子系统初始化 + 工具构建 + RegisterInteractor |
| `types.go` | 132 | Platform/Style/Status 枚举 + 核心数据结构 |
| `constants.go` | 88 | 包级常量 |
| `media_tool.go` | 186 | MediaTool 定义 + 参数读取 + 平台约束校验 |
| `media_registry.go` | 87 | 工具名常量 + DefaultMediaToolDefs() |
| `system_prompt.go` | 313 | 12-section BuildMediaSystemPrompt() |

**四核心工具**

| 文件 | 行数 | 职责 |
|------|------|------|
| `trending.go` | 151 | TrendingAggregator + TrendingSource 接口 |
| `trending_source_weibo.go` | 103 | 微博热搜源 |
| `trending_source_baidu.go` | 150 | 百度热搜源（含分类映射） |
| `trending_source_zhihu.go` | 130 | 知乎热榜源（含热度正则） |
| `trending_tool.go` | 232 | trending_topics 工具 |
| `content_compose_tool.go` | 261 | content_compose 工具 |
| `draft_store.go` | 199 | DraftStore 接口 + FileDraftStore |
| `publish_tool.go` | 247 | media_publish 工具 |
| `publish_history.go` | 180 | PublishHistoryStore 发布记录持久化 |
| `social_interact_tool.go` | 261 | social_interact 工具 + context-aware 限速 |

**图像处理**

| 文件 | 行数 | 职责 |
|------|------|------|
| `image_ops.go` | 290 | 图片缩放/裁切/格式转换 |
| `image_describe.go` | 105 | 视觉描述接口 |
| `image_describe_anthropic.go` | 170 | Anthropic 视觉描述 |
| `image_describe_openai.go` | 173 | OpenAI 视觉描述 |
| `image_exif.go` | 125 | EXIF 元数据提取 |
| `image_orient.go` | 145 | 方向校正（EXIF orientation） |

**音频与 STT**

| 文件 | 行数 | 职责 |
|------|------|------|
| `audio.go` | 44 | 音频处理基础 |
| `audio_tags.go` | 58 | 音频标签/元数据 |
| `stt.go` | 85 | STT 接口定义 |
| `stt_openai.go` | 190 | OpenAI Whisper 实现 |
| `stt_dashscope.go` | 331 | 阿里 DashScope 实现 |
| `stt_local.go` | 115 | 本地 STT 实现 |

**文档转换与文件输入**

| 文件 | 行数 | 职责 |
|------|------|------|
| `docconv.go` | 112 | 文档转换接口 |
| `docconv_builtin.go` | 194 | 内置转换器 |
| `docconv_mcp.go` | 433 | MCP 远程转换器 |
| `input_files.go` | 307 | 多模态文件输入处理 |
| `input_files_pdf.go` | 257 | PDF 文件处理 |
| `mime.go` | 223 | MIME 类型检测与映射 |
| `parse.go` | 212 | 内容解析 |

**状态、存储与调度**

| 文件 | 行数 | 职责 |
|------|------|------|
| `state_store.go` | 279 | MediaStateStore 状态持久化 |
| `store.go` | 181 | 通用存储抽象 |
| `store_io.go` | 189 | 存储 I/O 操作 |
| `media_cron.go` | 146 | 定时任务调度 |
| `event_trigger.go` | 221 | 事件触发器 |
| `opportunity_evaluator.go` | 251 | 发布时机评估 |
| `fetch.go` | 206 | HTTP 抓取工具 |
| `server.go` | 124 | 媒体服务器入口 |
| `host.go` | 243 | 宿主环境抽象 |

### 多模态理解子包 (`media/understanding/` — 15 源文件，~1800 行)

| 文件 | 行数 | 职责 |
|------|------|------|
| `types.go` | — | 理解任务/结果类型 |
| `registry.go` | — | Provider 注册表 |
| `runner.go` | — | 理解任务执行器 |
| `scope.go` | — | 能力范围定义 |
| `concurrency.go` | — | 并发控制 |
| `provider_anthropic.go` | — | Anthropic Claude 视觉 |
| `provider_openai.go` | — | OpenAI GPT-4V |
| `provider_google.go` | — | Google Gemini |
| `provider_groq.go` | — | Groq 实现 |
| `provider_minimax.go` | — | MiniMax 实现 |
| `provider_deepgram.go` | — | Deepgram 语音 |
| `provider_image.go` | — | 通用图片 Provider |
| `video.go` | — | 视频理解处理 |

### 频道包

| 文件 | 行数 | 职责 |
|------|------|------|
| `xiaohongshu/config.go` | 52 | XHS 配置（Cookie/限速） |
| `xiaohongshu/rpa_client.go` | 300 | 10 步 RPA 发布 + 限速 |
| `xiaohongshu/interactions.go` | 342 | 4 个 RPA 互动方法 + LRU 去重 |
| `xiaohongshu/browser_adapter.go` | 179 | BrowserDriver 接口 + CDPBrowserAdapter |
| `xiaohongshu/playwright_adapter.go` | 182 | Playwright 四层桥接适配器 |
| `xiaohongshu/plugin.go` | 138 | 频道插件 + AllClients() |
| `wechat_mp/config.go` | 32 | 微信配置（AppID/Secret） |
| `wechat_mp/client.go` | 283 | Token 管理 + 图片上传 + 限流 |
| `wechat_mp/publish.go` | 225 | 4 步 API 发布流水线 |
| `wechat_mp/plugin.go` | 125 | 频道插件 |

### 集成层

| 文件 | 行数 | 职责 |
|------|------|------|
| `runner/spawn_media_agent.go` | 209 | 子智能体生成工具 |
| `gateway/server_methods_media.go` | 691 | 15 个 RPC 方法 |
| `ui/controllers/media-dashboard.ts` | 356 | 前端控制器 |
| `ui/views/media-dashboard.ts` | 1041 | 前端视图渲染（主面板） |
| `ui/views/media-manage.ts` | 278 | 草稿/发布管理视图 |
| `ui/views/media-config.ts` | 114 | 媒体配置视图 |
