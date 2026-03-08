package gateway

import "os"

// preferredGatewayEnvValue 读取环境变量，优先使用 preferred（CRABCLAW_*），
// 若为空则回退到 fallback（OPENACOSMI_*）。
// 用于 OPENACOSMI → CRABCLAW 品牌迁移期间的双命名兼容。
func preferredGatewayEnvValue(preferred, fallback string) string {
	if v := os.Getenv(preferred); v != "" {
		return v
	}
	return os.Getenv(fallback)
}
