package types

// Skills 配置类型 — 继承自 src/config/types.skills.ts (32 行)

// SkillConfig 单个技能配置
type SkillConfig struct {
	Enabled *bool                  `json:"enabled,omitempty"`
	APIKey  string                 `json:"apiKey,omitempty"`
	Env     map[string]string      `json:"env,omitempty"`
	Config  map[string]interface{} `json:"config,omitempty"`
}

// SkillsLoadConfig 技能加载配置
type SkillsLoadConfig struct {
	ExtraDirs       []string `json:"extraDirs,omitempty"`
	Watch           *bool    `json:"watch,omitempty"`
	WatchDebounceMs *int     `json:"watchDebounceMs,omitempty"`
}

// SkillsInstallConfig 技能安装配置
type SkillsInstallConfig struct {
	PreferBrew  *bool  `json:"preferBrew,omitempty"`
	NodeManager string `json:"nodeManager,omitempty"` // "npm"|"pnpm"|"yarn"|"bun"
}

// SkillsStoreOAuthConfig 技能商店 OAuth 配置
type SkillsStoreOAuthConfig struct {
	Enabled   bool   `json:"enabled,omitempty" yaml:"enabled"`
	IssuerURL string `json:"issuerUrl,omitempty" yaml:"issuerUrl"`
	ClientID  string `json:"clientId,omitempty" yaml:"clientId"`
}

// SkillsStoreConfig 技能商店连接配置（nexus-v4 云端）
type SkillsStoreConfig struct {
	URL   string                  `json:"url,omitempty"`                // nexus-v4 基础 URL，如 "https://chat.acosmi.com"
	Token string                  `json:"token,omitempty"`              // JWT Bearer token (P1 REST + P2 OAuth bootstrap)
	MCP   *MCPConnectionConfig    `json:"mcp,omitempty"`                // MCP 远程工具连接配置 (P2)
	OAuth *SkillsStoreOAuthConfig `json:"oauth,omitempty" yaml:"oauth"` // OAuth 2.1 + PKCE 配置 (P2)
}

// AuthState 登录态（供 Gateway 其他模块消费）
type AuthState struct {
	IsAuthenticated bool          `json:"isAuthenticated"`
	UserID          string        `json:"userId,omitempty"`
	Email           string        `json:"email,omitempty"`
	DisplayName     string        `json:"displayName,omitempty"`
	Entitlements    []Entitlement `json:"entitlements,omitempty"`
	TokenExpiresAt  string        `json:"tokenExpiresAt,omitempty"`
}

// MCPConnectionConfig MCP 远程工具执行连接配置。
type MCPConnectionConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`      // 是否启用 MCP 远程工具
	Endpoint     string `json:"endpoint,omitempty"`     // MCP 端点 (默认 {url}/api/v4/mcp)
	OAuthEnabled bool   `json:"oauthEnabled,omitempty"` // 是否用 OAuth 2.1 (否则用 P1 JWT)
}

// SkillsConfig 技能总配置
type SkillsConfig struct {
	AllowBundled []string                `json:"allowBundled,omitempty"`
	Load         *SkillsLoadConfig       `json:"load,omitempty"`
	Install      *SkillsInstallConfig    `json:"install,omitempty"`
	Entries      map[string]*SkillConfig `json:"entries,omitempty"`
	Store        *SkillsStoreConfig      `json:"store,omitempty"` // 远程技能商店
}
