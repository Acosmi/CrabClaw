package infra

// retry_policy.go — 预定义重试策略集合
// 对应 TS: src/infra/retry-policy.ts (101L)
//
// 为不同场景提供标准化的重试配置。

// ─── 预定义重试配置 ───

// RetryPolicyFast 快速重试（短暂抖动恢复）。
// 适用于：本地 IPC、内存缓存查询。
var RetryPolicyFast = RetryOptions{
	RetryConfig: RetryConfig{Attempts: 3, MinDelay: 50, MaxDelay: 500},
	Label:       "fast",
}

// RetryPolicyStandard 标准重试（HTTP API 调用）。
// 适用于：外部 API、数据库连接。
var RetryPolicyStandard = RetryOptions{
	RetryConfig: RetryConfig{Attempts: 3, MinDelay: 300, MaxDelay: 10_000},
	Label:       "standard",
}

// RetryPolicyPatient 耐心重试（长耗时操作）。
// 适用于：大文件上传、CI/CD 流水线。
var RetryPolicyPatient = RetryOptions{
	RetryConfig: RetryConfig{Attempts: 5, MinDelay: 1000, MaxDelay: 30_000},
	Label:       "patient",
}

// RetryPolicyLLM LLM API 重试策略。
// 适用于：OpenAI / Gemini API 调用（注意 429 Retry-After）。
var RetryPolicyLLM = RetryOptions{
	RetryConfig: RetryConfig{Attempts: 3, MinDelay: 1000, MaxDelay: 60_000},
	Label:       "llm-api",
}

// RetryPolicyWebhook Webhook 投递重试。
// 适用于：消息投递、事件通知。
var RetryPolicyWebhook = RetryOptions{
	RetryConfig: RetryConfig{Attempts: 5, MinDelay: 500, MaxDelay: 30_000},
	Label:       "webhook",
}

// ─── 预定义退避策略 ───

// BackoffPolicyDefault 默认退避策略。
var BackoffPolicyDefault = BackoffPolicy{
	InitialMs: 300,
	MaxMs:     30_000,
	Factor:    2,
	Jitter:    0.1,
}

// BackoffPolicyAggressive 激进退避（快速增长）。
var BackoffPolicyAggressive = BackoffPolicy{
	InitialMs: 100,
	MaxMs:     60_000,
	Factor:    3,
	Jitter:    0.2,
}

// BackoffPolicyGentle 温和退避（缓慢增长）。
var BackoffPolicyGentle = BackoffPolicy{
	InitialMs: 500,
	MaxMs:     10_000,
	Factor:    1.5,
	Jitter:    0.1,
}

// GetRetryPolicy 根据名称获取预定义重试策略。
func GetRetryPolicy(name string) *RetryOptions {
	switch name {
	case "fast":
		p := RetryPolicyFast
		return &p
	case "standard":
		p := RetryPolicyStandard
		return &p
	case "patient":
		p := RetryPolicyPatient
		return &p
	case "llm", "llm-api":
		p := RetryPolicyLLM
		return &p
	case "webhook":
		p := RetryPolicyWebhook
		return &p
	default:
		return nil
	}
}
