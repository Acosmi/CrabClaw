package types

// PackageKind 包类型
type PackageKind string

const (
	PackageKindSkill  PackageKind = "skill"
	PackageKindPlugin PackageKind = "plugin"
	PackageKindBundle PackageKind = "bundle"
)

// PackageCatalogItem 统一目录条目（聚合 skill + plugin + bundle）
type PackageCatalogItem struct {
	ID             string      `json:"id"`
	Kind           PackageKind `json:"kind"`
	Key            string      `json:"key"`
	Name           string      `json:"name"`
	Description    string      `json:"description,omitempty"`
	Icon           string      `json:"icon,omitempty"`
	Version        string      `json:"version"`
	Author         string      `json:"author,omitempty"`
	Tags           []string    `json:"tags,omitempty"`
	CapabilityTags []string    `json:"capabilityTags,omitempty"`
	SecurityLevel  string      `json:"securityLevel,omitempty"`
	SecurityScore  int         `json:"securityScore,omitempty"`
	DownloadCount  int64       `json:"downloadCount,omitempty"`
	Source         string      `json:"source"` // "local" | "remote" | "builtin"
	IsInstalled    bool        `json:"isInstalled"`
	InstalledAt    string      `json:"installedAt,omitempty"`
	// Plugin 特有
	ExecutionMode string `json:"executionMode,omitempty"` // "builtin" | "bridge" | "wasm" | "external"
	// Bundle 特有
	BundleItems []BundleRef `json:"bundleItems,omitempty"`
	BundleType  string      `json:"bundleType,omitempty"` // "membership" | "industry" | "enterprise" | "curated"
}

// BundleRef bundle 中引用的子项
type BundleRef struct {
	Kind PackageKind `json:"kind"`
	ID   string      `json:"id"`
	Key  string      `json:"key"`
}

// PackageInstallRecord 统一安装账本条目
type PackageInstallRecord struct {
	ID          string      `json:"id"`
	Kind        PackageKind `json:"kind"`
	Key         string      `json:"key"`
	Version     string      `json:"version"`
	Source      string      `json:"source"` // "remote" | "local" | "builtin"
	InstalledAt string      `json:"installedAt"`
	UpdatedAt   string      `json:"updatedAt,omitempty"`
}

// Entitlement 权益条目
type Entitlement struct {
	Type      string `json:"type"` // "managed_models" | "skill_store" | "bundle_xxx"
	Granted   bool   `json:"granted"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// EntitlementCache 本地缓存结构
type EntitlementCache struct {
	UserID       string        `json:"userId"`
	Entitlements []Entitlement `json:"entitlements"`
	FetchedAt    string        `json:"fetchedAt"`
	TTLSeconds   int           `json:"ttlSeconds"` // 默认 3600
}
