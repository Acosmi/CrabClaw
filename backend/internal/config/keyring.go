package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/log"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/zalando/go-keyring"
)

// KeyringSentinel 脱敏并存入 Keyring 后的占位符
const KeyringSentinel = "__OPENACOSMI_KEYRING_REF__"

// KeyringServiceName 系统钥匙串中用于新品牌的服务名称。
const KeyringServiceName = "Crab Claw"

// LegacyKeyringServiceName 兼容旧版本遗留的 Keyring 服务名称。
const LegacyKeyringServiceName = "OpenAcosmi"

var keyringLogger = log.New("keyring")

func keyringServiceNames() []string {
	return []string{KeyringServiceName, LegacyKeyringServiceName}
}

// storeSecretInKeyring 双写新旧 service name，保证新品牌和旧版本都能读取同一份凭据。
func storeSecretInKeyring(path, secret string) error {
	var failures []string
	successCount := 0
	for _, serviceName := range keyringServiceNames() {
		if err := keyring.Set(serviceName, path, secret); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", serviceName, err))
			continue
		}
		successCount++
	}
	if successCount == 0 {
		return fmt.Errorf("keyring set failed for all services: %s", strings.Join(failures, "; "))
	}
	if len(failures) > 0 {
		keyringLogger.Warn("Keyring mirror write partial failure for '%s': %s", path, strings.Join(failures, "; "))
	}
	return nil
}

// restoreSecretFromKeyring 优先读取新 service name，读取不到时回退旧 service name。
func restoreSecretFromKeyring(path string) (string, error) {
	var failures []string
	for _, serviceName := range keyringServiceNames() {
		secret, err := keyring.Get(serviceName, path)
		if err == nil && secret != "" {
			return secret, nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", serviceName, err))
			continue
		}
		failures = append(failures, fmt.Sprintf("%s: empty secret", serviceName))
	}
	return "", fmt.Errorf("keyring get failed for all services: %s", strings.Join(failures, "; "))
}

// StoreSensitiveToKeyring 递归遍历配置对象，将敏感字段保存到 OS Keyring，并在原位留存 KeyringSentinel 占位符。
func StoreSensitiveToKeyring(obj interface{}) (interface{}, error) {
	return storeToKeyringRecursively(obj, "")
}

func storeToKeyringRecursively(obj interface{}, pathPrefix string) (interface{}, error) {
	if obj == nil {
		return nil, nil
	}

	switch v := obj.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			currentPath := key
			if pathPrefix != "" {
				currentPath = pathPrefix + "." + key
			}

			if IsSensitiveKey(key) {
				if strVal, ok := value.(string); ok && strVal != "" && strVal != KeyringSentinel && strVal != RedactedSentinel {
					err := storeSecretInKeyring(currentPath, strVal)
					if err != nil {
						keyringLogger.Warn("Keyring set failed for '%s': %v. Storing as plain text.", currentPath, err)
						result[key] = value // Fallback to plain text store
					} else {
						result[key] = KeyringSentinel
					}
					continue
				}
			}

			if sub, ok := value.(map[string]interface{}); ok {
				restored, err := storeToKeyringRecursively(sub, currentPath)
				if err != nil {
					return nil, err
				}
				result[key] = restored
			} else if arr, ok := value.([]interface{}); ok {
				restored, err := storeToKeyringRecursively(arr, currentPath)
				if err != nil {
					return nil, err
				}
				result[key] = restored
			} else {
				result[key] = value
			}
		}
		return result, nil

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			currentPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
			restored, err := storeToKeyringRecursively(item, currentPath)
			if err != nil {
				return nil, err
			}
			result[i] = restored
		}
		return result, nil

	default:
		return obj, nil
	}
}

// RestoreFromKeyring 递归查找对象内的 KeyringSentinel 占位符，并从 OS Keyring 中取出真实密钥覆盖。
func RestoreFromKeyring(obj interface{}) error {
	_, err := restoreFromKeyringRecursively(obj, "")
	return err
}

func restoreFromKeyringRecursively(obj interface{}, pathPrefix string) (interface{}, error) {
	if obj == nil {
		return nil, nil
	}

	switch v := obj.(type) {
	case map[string]interface{}:
		for key, value := range v {
			currentPath := key
			if pathPrefix != "" {
				currentPath = pathPrefix + "." + key
			}

			if IsSensitiveKey(key) {
				if strVal, ok := value.(string); ok && strVal == KeyringSentinel {
					secret, err := restoreSecretFromKeyring(currentPath)
					if err == nil && secret != "" {
						v[key] = secret
					} else {
						keyringLogger.Error("Keyring restore FAILED for '%s': %v. Setting to empty — reconfiguration required.", currentPath, err)
						v[key] = "" // 清空，让 config 校验能捕获问题，而不是静默传递 sentinel
					}
					continue
				}
			}

			if sub, ok := value.(map[string]interface{}); ok {
				_, err := restoreFromKeyringRecursively(sub, currentPath)
				if err != nil {
					return nil, err
				}
			} else if arr, ok := value.([]interface{}); ok {
				_, err := restoreFromKeyringRecursively(arr, currentPath)
				if err != nil {
					return nil, err
				}
			}
		}
		return v, nil

	case []interface{}:
		for i, item := range v {
			currentPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
			_, err := restoreFromKeyringRecursively(item, currentPath)
			if err != nil {
				return nil, err
			}
		}
		return v, nil

	default:
		return obj, nil
	}
}

// MapStructToMapForKeyring 辅助函数，结构体转为 Map 供 Keyring 脱敏使用
func MapStructToMapForKeyring(cfg *types.OpenAcosmiConfig) (interface{}, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	return m, err
}
