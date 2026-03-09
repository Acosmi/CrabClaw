package config

// 配置验证模块 — 将 Zod schema 验证逻辑转换为 Go 的 go-playground/validator
//
// 原版 TypeScript 使用 Zod 的 z.object().strict().refine().superRefine() 进行运行时验证。
// Go 端使用 go-playground/validator struct tags 处理字段级约束，
// 并通过自定义验证函数处理跨字段规则（如 allow/alsoAllow 互斥）。

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/go-playground/validator/v10"
)

// validate 全局验证器实例（只初始化一次）
var (
	validate     *validator.Validate
	validateOnce sync.Once
)

// getValidator 获取或初始化全局验证器
func getValidator() *validator.Validate {
	validateOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())

		// 注册自定义验证器
		_ = validate.RegisterValidation("hexcolor", validateHexColor)
		_ = validate.RegisterValidation("safe_executable", validateSafeExecutable)
		_ = validate.RegisterValidation("duration_string", validateDurationString)
	})
	return validate
}

// ValidateConfig 验证 OpenAcosmiConfig 配置
// 返回所有验证错误的列表
func ValidateConfig(cfg interface{}) []ValidationError {
	v := getValidator()
	err := v.Struct(cfg)
	if err == nil {
		return nil
	}

	var result []ValidationError
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			result = append(result, ValidationError{
				Field:   e.Namespace(),
				Tag:     e.Tag(),
				Value:   fmt.Sprintf("%v", e.Value()),
				Message: formatValidationMessage(e),
			})
		}
	}
	return result
}

// ValidationError 验证错误详情
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// Error 实现 error 接口
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ----- 自定义验证器 -----

var hexColorRegex = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)

// validateHexColor 验证十六进制颜色值 (#RRGGBB 或 RRGGBB)
func validateHexColor(fl validator.FieldLevel) bool {
	return hexColorRegex.MatchString(fl.Field().String())
}

// safeExecutableRegex 安全可执行路径模式（禁止 shell 注入字符）
var safeExecutableRegex = regexp.MustCompile(`^[a-zA-Z0-9_/.\-]+$`)

// validateSafeExecutable 验证安全可执行文件名或路径
func validateSafeExecutable(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true // 空值由 required 标签处理
	}
	// 禁止 shell 元字符
	return safeExecutableRegex.MatchString(value)
}

// durationSuffixes 合法的时间后缀
var durationSuffixes = []string{"ms", "s", "m", "h"}

// validateDurationString 验证持续时间字符串格式 (如 "500ms", "30s", "5m", "1h")
func validateDurationString(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}
	for _, suffix := range durationSuffixes {
		if strings.HasSuffix(value, suffix) {
			numPart := strings.TrimSuffix(value, suffix)
			if numPart == "" {
				return false
			}
			for _, c := range numPart {
				if c < '0' || c > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}

// ----- 跨字段验证辅助 -----

// ValidateAllowAlsoAllowMutex 验证 allow 和 alsoAllow 不能同时设置
// 对应 Zod 的 superRefine: "cannot set both allow and alsoAllow in the same scope"
func ValidateAllowAlsoAllowMutex(allow, alsoAllow []string) *ValidationError {
	if len(allow) > 0 && len(alsoAllow) > 0 {
		return &ValidationError{
			Field:   "allow/alsoAllow",
			Tag:     "mutex",
			Message: "cannot set both allow and alsoAllow in the same scope (merge alsoAllow into allow, or remove allow and use profile + alsoAllow)",
		}
	}
	return nil
}

// ValidateOpenPolicyAllowFrom 验证 open 策略必须有 allowFrom 包含 "*"
// 对应 Zod 的 requireOpenAllowFrom
func ValidateOpenPolicyAllowFrom(policy string, allowFrom []interface{}, fieldPath string) *ValidationError {
	if policy != "open" {
		return nil
	}
	for _, v := range allowFrom {
		if fmt.Sprintf("%v", v) == "*" {
			return nil
		}
	}
	return &ValidationError{
		Field:   fieldPath,
		Tag:     "open_policy",
		Message: fmt.Sprintf("%s: policy is 'open' but allowFrom does not include '*'", fieldPath),
	}
}

// NormalizeAllowFrom 规范化 allowFrom 列表（转字符串，去空白）
// 对应 Zod 的 normalizeAllowFrom
func NormalizeAllowFrom(values []interface{}) []string {
	var result []string
	for _, v := range values {
		s := strings.TrimSpace(fmt.Sprintf("%v", v))
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// ----- 格式化辅助 -----

func formatValidationMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "is required"
	case "min":
		return fmt.Sprintf("must be at least %s", e.Param())
	case "max":
		return fmt.Sprintf("must be at most %s", e.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", e.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", e.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", e.Param())
	case "hexcolor":
		return "must be a valid hex color (RRGGBB)"
	case "safe_executable":
		return "must be a safe executable name or path"
	case "duration_string":
		return "must be a valid duration (e.g., 500ms, 30s, 5m, 1h)"
	default:
		return fmt.Sprintf("failed on tag '%s'", e.Tag())
	}
}

// ----- 深层约束验证 (H7-2) -----

// isValidEnum 检查值是否在允许的枚举列表中
func isValidEnum(value string, allowed []string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// validateDeepConstraints 验证 Zod schema 中的深层嵌套约束
// 对应 zod-schema.ts 中的枚举范围、数值范围等约束
func validateDeepConstraints(cfg *types.OpenAcosmiConfig) []ValidationError {
	var errs []ValidationError

	// logging.level 枚举
	if cfg.Logging != nil && cfg.Logging.Level != "" {
		validLevels := []string{"silent", "fatal", "error", "warn", "info", "debug", "trace"}
		if !isValidEnum(string(cfg.Logging.Level), validLevels) {
			errs = append(errs, ValidationError{
				Field:   "logging.level",
				Tag:     "enum",
				Value:   string(cfg.Logging.Level),
				Message: fmt.Sprintf("logging.level must be one of: %s", strings.Join(validLevels, ", ")),
			})
		}
	}

	// logging.consoleLevel 枚举
	if cfg.Logging != nil && cfg.Logging.ConsoleLevel != "" {
		validLevels := []string{"silent", "fatal", "error", "warn", "info", "debug", "trace"}
		if !isValidEnum(string(cfg.Logging.ConsoleLevel), validLevels) {
			errs = append(errs, ValidationError{
				Field:   "logging.consoleLevel",
				Tag:     "enum",
				Value:   string(cfg.Logging.ConsoleLevel),
				Message: fmt.Sprintf("logging.consoleLevel must be one of: %s", strings.Join(validLevels, ", ")),
			})
		}
	}

	// gateway.mode 枚举
	if cfg.Gateway != nil && cfg.Gateway.Mode != "" {
		validModes := []string{"local", "remote"}
		if !isValidEnum(string(cfg.Gateway.Mode), validModes) {
			errs = append(errs, ValidationError{
				Field:   "gateway.mode",
				Tag:     "enum",
				Value:   string(cfg.Gateway.Mode),
				Message: fmt.Sprintf("gateway.mode must be one of: %s", strings.Join(validModes, ", ")),
			})
		}
	}

	// gateway.bind 枚举
	if cfg.Gateway != nil && cfg.Gateway.Bind != "" {
		validBinds := []string{"auto", "lan", "localhost", "loopback", "0.0.0.0"}
		if !isValidEnum(string(cfg.Gateway.Bind), validBinds) {
			errs = append(errs, ValidationError{
				Field:   "gateway.bind",
				Tag:     "enum",
				Value:   string(cfg.Gateway.Bind),
				Message: fmt.Sprintf("gateway.bind must be one of: %s", strings.Join(validBinds, ", ")),
			})
		}
	}

	// gateway.port 范围
	if cfg.Gateway != nil && cfg.Gateway.Port != nil {
		port := *cfg.Gateway.Port
		if port < 1 || port > 65535 {
			errs = append(errs, ValidationError{
				Field:   "gateway.port",
				Tag:     "range",
				Value:   fmt.Sprintf("%d", port),
				Message: "gateway.port must be between 1 and 65535",
			})
		}
	}

	// session.dmScope 枚举
	if cfg.Session != nil && cfg.Session.DmScope != "" {
		validScopes := []string{"main", "per-peer", "per-channel-peer", "per-account-channel-peer"}
		if !isValidEnum(string(cfg.Session.DmScope), validScopes) {
			errs = append(errs, ValidationError{
				Field:   "session.dmScope",
				Tag:     "enum",
				Value:   string(cfg.Session.DmScope),
				Message: fmt.Sprintf("session.dmScope must be one of: %s", strings.Join(validScopes, ", ")),
			})
		}
	}

	// session.scope 枚举
	if cfg.Session != nil && cfg.Session.Scope != "" {
		validScopes := []string{"per-sender", "global"}
		if !isValidEnum(string(cfg.Session.Scope), validScopes) {
			errs = append(errs, ValidationError{
				Field:   "session.scope",
				Tag:     "enum",
				Value:   string(cfg.Session.Scope),
				Message: fmt.Sprintf("session.scope must be one of: %s", strings.Join(validScopes, ", ")),
			})
		}
	}

	// session.agentToAgent.maxPingPongTurns 范围 (0-5)
	if cfg.Session != nil && cfg.Session.AgentToAgent != nil && cfg.Session.AgentToAgent.MaxPingPongTurns != nil {
		turns := *cfg.Session.AgentToAgent.MaxPingPongTurns
		if turns < 0 || turns > 5 {
			errs = append(errs, ValidationError{
				Field:   "session.agentToAgent.maxPingPongTurns",
				Tag:     "range",
				Value:   fmt.Sprintf("%d", turns),
				Message: "session.agentToAgent.maxPingPongTurns must be between 0 and 5",
			})
		}
	}

	// update.channel 枚举
	if cfg.Update != nil && cfg.Update.Channel != "" {
		validChannels := []string{"stable", "beta", "dev"}
		if !isValidEnum(cfg.Update.Channel, validChannels) {
			errs = append(errs, ValidationError{
				Field:   "update.channel",
				Tag:     "enum",
				Value:   cfg.Update.Channel,
				Message: fmt.Sprintf("update.channel must be one of: %s", strings.Join(validChannels, ", ")),
			})
		}
	}

	// update.installPolicy 枚举
	if cfg.Update != nil && cfg.Update.InstallPolicy != "" {
		validPolicies := []string{"manual", "on-quit", "idle"}
		if !isValidEnum(cfg.Update.InstallPolicy, validPolicies) {
			errs = append(errs, ValidationError{
				Field:   "update.installPolicy",
				Tag:     "enum",
				Value:   cfg.Update.InstallPolicy,
				Message: fmt.Sprintf("update.installPolicy must be one of: %s", strings.Join(validPolicies, ", ")),
			})
		}
	}

	// update.sourceURL URL 基本合法性
	if cfg.Update != nil && strings.TrimSpace(cfg.Update.SourceURL) != "" {
		sourceURL := strings.TrimSpace(cfg.Update.SourceURL)
		parsed, err := url.Parse(sourceURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			errs = append(errs, ValidationError{
				Field:   "update.sourceURL",
				Tag:     "url",
				Value:   cfg.Update.SourceURL,
				Message: "update.sourceURL must be an absolute http(s) URL",
			})
		} else if parsed.Scheme != "http" && parsed.Scheme != "https" {
			errs = append(errs, ValidationError{
				Field:   "update.sourceURL",
				Tag:     "url",
				Value:   cfg.Update.SourceURL,
				Message: "update.sourceURL must use http or https",
			})
		}
	}

	// stt.provider 枚举
	if cfg.STT != nil && cfg.STT.Provider != "" {
		validProviders := []string{"openai", "groq", "azure", "qwen", "ollama", "local-whisper"}
		if !isValidEnum(cfg.STT.Provider, validProviders) {
			errs = append(errs, ValidationError{
				Field:   "stt.provider",
				Tag:     "enum",
				Value:   cfg.STT.Provider,
				Message: fmt.Sprintf("stt.provider must be one of: %s", strings.Join(validProviders, ", ")),
			})
		}
	}

	// docConv.provider 枚举
	if cfg.DocConv != nil && cfg.DocConv.Provider != "" {
		validProviders := []string{"mcp", "builtin"}
		if !isValidEnum(cfg.DocConv.Provider, validProviders) {
			errs = append(errs, ValidationError{
				Field:   "docConv.provider",
				Tag:     "enum",
				Value:   cfg.DocConv.Provider,
				Message: fmt.Sprintf("docConv.provider must be one of: %s", strings.Join(validProviders, ", ")),
			})
		}
	}

	// imageUnderstanding.provider 枚举
	if cfg.ImageUnderstanding != nil && cfg.ImageUnderstanding.Provider != "" {
		validProviders := []string{"qwen-vl", "openai", "ollama", "google", "anthropic"}
		if !isValidEnum(cfg.ImageUnderstanding.Provider, validProviders) {
			errs = append(errs, ValidationError{
				Field:   "imageUnderstanding.provider",
				Tag:     "enum",
				Value:   cfg.ImageUnderstanding.Provider,
				Message: fmt.Sprintf("imageUnderstanding.provider must be one of: %s", strings.Join(validProviders, ", ")),
			})
		}
	}

	return errs
}
