---
name: email
description: "通过邮件通道收发邮件，支持多账号、线程回复"
tools: send_email
---

## 何时使用

| 场景 | 工具 | 说明 |
|------|------|------|
| 主动发送新邮件 | send_email | to + subject + body |
| 回复已收邮件 | send_email | reply_to_session 自动带线程头 |
| 发送带抄送的邮件 | send_email | cc 字段 |
| 用指定账号发送 | send_email | account 字段 |

## 不适用场景

- 需要发送附件（V1 不支持出站附件，仅支持入站附件解析）
- 需要 HTML 富文本邮件（V1 仅发送纯文本）
- 需要操作草稿箱、日历、联系人

## 工具参数

- `to`（必填）: 收件人邮箱
- `subject`（必填）: 主题
- `body`（必填）: 纯文本正文
- `account`（可选）: 发送账号 ID，省略则用 defaultAccount
- `reply_to_session`（可选）: 回复的 sessionKey，自动恢复线程头
- `cc`（可选）: 抄送地址，逗号分隔

## 推荐工作流

1. 用户说"给 xxx@company.com 发邮件"
2. 智能体确认主题和正文
3. 调用 send_email 工具
4. 返回发送结果（成功 / 失败原因）

## 安全约束

- 发送受每账号每小时/每日频率限制
- NoNetwork 委托合约下 send_email 工具不可用
- 默认添加 Auto-Submitted 头防止邮件环
