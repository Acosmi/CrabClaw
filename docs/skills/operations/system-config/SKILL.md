---
name: system-config
description: "系统配置全链路：schema 校验、增量 patch、模型切换、安全变更与回滚"
---

# 系统配置技能

## 配置变更安全流程

```
查看 → 校验 → 增量修改 → 应用 → 验证 → (失败则回滚)
```

### 1. 查看当前配置
```
config.get → 记录当前状态（hash + 关键字段）
```

### 2. Schema 校验
```
config.schema → 获取类型定义和可选值
```

### 3. 增量修改
```
config.patch → 一次只改一个主题块（最小 diff 原则）
```

**规则**:
- 避免同时修改多个高风险块（auth、models、security）
- 敏感项使用最小 diff
- 只有需要完全替换时才用 `config.set`

### 4. 应用并验证
```
config.apply → 立即生效 + 检查重启/结果
```

### 5. 回滚（如需）
```
config.patch → 回退上一次修改
system.backup.restore → 恢复到备份点
```

## 模型配置

### 切换主模型
```json5
{
  models: {
    default: {
      provider: "anthropic",
      model: "claude-sonnet-4-6"
    }
  }
}
```

### 配置备用模型
```json5
{
  models: {
    fallback: {
      provider: "openai",
      model: "gpt-4o"
    }
  }
}
```

### Provider 认证
```json5
{
  providers: {
    anthropic: { apiKey: "sk-..." },
    openai: { apiKey: "sk-..." },
    ollama: { baseUrl: "http://localhost:11434" }
  }
}
```

## 安全配置

### 执行审批策略
```json5
{
  tools: {
    exec: {
      security: "allowlist",  // deny | allowlist | full
      ask: "on-miss",         // off | on-miss | always
      askFallback: "deny"     // deny | allowlist | full
    }
  }
}
```

### 提权控制
```json5
{
  tools: {
    elevated: {
      enabled: true,
      allowFrom: ["discord", "whatsapp"]
    }
  }
}
```

### 远程审批
```json5
{
  security: {
    remoteApproval: {
      provider: "feishu",
      // ... provider-specific config
    }
  }
}
```

## 高风险配置块

| 配置块 | 风险等级 | 注意事项 |
|--------|---------|---------|
| `models.*` | 中 | 切换后需验证 API 连通性 |
| `providers.*` | 中 | API key 变更需立即测试 |
| `security.*` | 高 | 可能锁死审批链路 |
| `tools.exec.*` | 高 | 可能阻断命令执行 |
| `channels.*` | 中 | 可能断开远程频道 |
| `system.reset` | 极高 | 不可逆操作，必须 preview 先 |

## 故障恢复

1. 配置错误导致无法访问 → `system.backup.restore`
2. 审批链路锁死 → 本地修改 `exec-approvals.json`
3. 模型不可用 → `config.patch` 切换到备用 provider
