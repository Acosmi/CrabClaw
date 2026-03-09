package types

// 模型配置类型 — 继承自 src/config/types.models.ts

// ModelApi 模型 API 类型
type ModelApi string

const (
	ModelAPIOpenAICompletions  ModelApi = "openai-completions"
	ModelAPIOpenAIResponses    ModelApi = "openai-responses"
	ModelAPIAnthropicMessages  ModelApi = "anthropic-messages"
	ModelAPIGoogleGenerativeAI ModelApi = "google-generative-ai"
	ModelAPIGitHubCopilot      ModelApi = "github-copilot"
	ModelAPIBedrockConverse    ModelApi = "bedrock-converse-stream"
)

// ModelCompatConfig 模型兼容性配置
type ModelCompatConfig struct {
	SupportsStore           *bool  `json:"supportsStore,omitempty"`
	SupportsDeveloperRole   *bool  `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort *bool  `json:"supportsReasoningEffort,omitempty"`
	MaxTokensField          string `json:"maxTokensField,omitempty"` // "max_completion_tokens"|"max_tokens"
}

// ModelProviderAuthMode 模型供应商认证模式
type ModelProviderAuthMode string

const (
	ModelAuthAPIKey ModelProviderAuthMode = "api-key"
	ModelAuthAWS    ModelProviderAuthMode = "aws-sdk"
	ModelAuthOAuth  ModelProviderAuthMode = "oauth"
	ModelAuthToken  ModelProviderAuthMode = "token"
)

// ModelInputType 模型输入类型
type ModelInputType string

const (
	ModelInputText  ModelInputType = "text"
	ModelInputImage ModelInputType = "image"
)

// ModelCostConfig 模型计费配置
type ModelCostConfig struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ModelDefinitionConfig 模型定义配置
// 原版: export type ModelDefinitionConfig
type ModelDefinitionConfig struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	API           ModelApi           `json:"api,omitempty"`
	Reasoning     bool               `json:"reasoning"`
	Input         []ModelInputType   `json:"input"`
	Cost          ModelCostConfig    `json:"cost"`
	ContextWindow int                `json:"contextWindow"`
	MaxTokens     int                `json:"maxTokens"`
	Headers       map[string]string  `json:"headers,omitempty"`
	Compat        *ModelCompatConfig `json:"compat,omitempty"`
}

// ModelProviderConfig 模型供应商配置
// 原版: export type ModelProviderConfig
type ModelProviderConfig struct {
	BaseURL    string                  `json:"baseUrl"`
	APIKey     string                  `json:"apiKey,omitempty"`
	Auth       ModelProviderAuthMode   `json:"auth,omitempty"`
	API        ModelApi                `json:"api,omitempty"`
	Headers    map[string]string       `json:"headers,omitempty"`
	AuthHeader *bool                   `json:"authHeader,omitempty"`
	Models     []ModelDefinitionConfig `json:"models"`
}

// BedrockDiscoveryConfig AWS Bedrock 模型发现配置
type BedrockDiscoveryConfig struct {
	Enabled              *bool    `json:"enabled,omitempty"`
	Region               string   `json:"region,omitempty"`
	ProviderFilter       []string `json:"providerFilter,omitempty"`
	RefreshInterval      *int     `json:"refreshInterval,omitempty"`
	DefaultContextWindow *int     `json:"defaultContextWindow,omitempty"`
	DefaultMaxTokens     *int     `json:"defaultMaxTokens,omitempty"`
}

// ModelsConfigMode 模型配置合并模式
type ModelsConfigMode string

const (
	ModelsConfigMerge   ModelsConfigMode = "merge"
	ModelsConfigReplace ModelsConfigMode = "replace"
)

// ModelSource 模型来源
type ModelSource string

const (
	ModelSourceManaged ModelSource = "managed"
	ModelSourceBuiltin ModelSource = "builtin"
	ModelSourceCustom  ModelSource = "custom"
)

// ManagedModelEntry 托管模型条目（从 nexus-v4 拉取）
type ManagedModelEntry struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Provider      string  `json:"provider"`
	ModelID       string  `json:"modelId"`
	MaxTokens     int     `json:"maxTokens,omitempty"`
	PricePerMTok  float64 `json:"pricePerMTok,omitempty"` // 每百万 token 价格
	IsDefault     bool    `json:"isDefault,omitempty"`
	ContextWindow int     `json:"contextWindow,omitempty"` // [FIX P0-L02: 上下文窗口大小]
}

// ManagedModelsConfig 托管模型配置
type ManagedModelsConfig struct {
	Enabled       bool   `json:"enabled,omitempty" yaml:"enabled"`
	CatalogURL    string `json:"catalogUrl,omitempty" yaml:"catalogUrl"`       // nexus-v4 模型目录 API
	ProxyEndpoint string `json:"proxyEndpoint,omitempty" yaml:"proxyEndpoint"` // 托管模型代理端点
}

// ModelsConfig 模型总配置
type ModelsConfig struct {
	Mode             ModelsConfigMode                `json:"mode,omitempty"`
	Providers        map[string]*ModelProviderConfig `json:"providers,omitempty"`
	BedrockDiscovery *BedrockDiscoveryConfig         `json:"bedrockDiscovery,omitempty"`
	ManagedModels    *ManagedModelsConfig            `json:"managedModels,omitempty" yaml:"managedModels"`
}
