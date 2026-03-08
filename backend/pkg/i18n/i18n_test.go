package i18n

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDefaultLanguageIsChinese(t *testing.T) {
	Init(LangZhCN)
	if GetLang() != LangZhCN {
		t.Errorf("默认语言应为中文，实际为 %s", GetLang())
	}
}

func TestTranslationWithParams(t *testing.T) {
	Init(LangZhCN)
	result := T("app.starting", map[string]string{"version": "1.0.0"})
	if !strings.Contains(result, "1.0.0") {
		t.Errorf("参数替换失败，结果: %s", result)
	}
	if !strings.Contains(result, "Crab Claw（蟹爪）") {
		t.Errorf("启动文案应包含新品牌，结果: %s", result)
	}
	if !strings.Contains(result, "启动") {
		t.Errorf("中文翻译应包含'启动'，结果: %s", result)
	}
}

func TestLanguageSwitch(t *testing.T) {
	Init(LangZhCN)

	zhResult := T("app.shutdown", nil)
	if !strings.Contains(zhResult, "Crab Claw（蟹爪）") {
		t.Errorf("中文翻译应包含新品牌，结果: %s", zhResult)
	}
	if !strings.Contains(zhResult, "关闭") {
		t.Errorf("中文翻译应包含'关闭'，结果: %s", zhResult)
	}

	SetLang(LangEnUS)
	enResult := T("app.shutdown", nil)
	if !strings.Contains(enResult, "Crab Claw（蟹爪）") {
		t.Errorf("英文翻译应包含新品牌，结果: %s", enResult)
	}
	if !strings.Contains(enResult, "shut down") {
		t.Errorf("英文翻译应包含'shut down'，结果: %s", enResult)
	}

	// 切回中文
	SetLang(LangZhCN)
}

func TestMissingKey(t *testing.T) {
	Init(LangZhCN)
	result := T("nonexistent.key", nil)
	if !strings.Contains(result, "[missing:") {
		t.Errorf("缺失键应返回 [missing: ...] 格式，结果: %s", result)
	}
}

func TestRegisterBundle(t *testing.T) {
	Init(LangZhCN)
	RegisterBundle(LangZhCN, map[string]string{
		"test.custom": "自定义消息: {value}",
	})
	RegisterBundle(LangEnUS, map[string]string{
		"test.custom": "Custom message: {value}",
	})

	result := T("test.custom", map[string]string{"value": "测试"})
	if result != "自定义消息: 测试" {
		t.Errorf("自定义消息注册失败，结果: %s", result)
	}
}

func TestUnsupportedLanguageFallback(t *testing.T) {
	Init(LangZhCN)
	SetLang(Lang("fr-FR")) // 不支持的语言
	if GetLang() != LangZhCN {
		t.Errorf("不支持的语言应保持原语言，实际为 %s", GetLang())
	}
}

// ---------- W1 扩展测试 ----------

// TestTp 验证无参数翻译快捷函数
func TestTp(t *testing.T) {
	Init(LangZhCN)
	result := Tp("app.shutdown")
	if result != T("app.shutdown", nil) {
		t.Errorf("Tp 与 T(key, nil) 结果不一致: %q vs %q", result, T("app.shutdown", nil))
	}
}

// TestTf 验证 fmt 格式化翻译快捷函数
func TestTf(t *testing.T) {
	Init(LangZhCN)

	// 使用 onboarding 中的 %s 模板
	result := Tf("onboard.completion.prompt", "zsh")
	if !strings.Contains(result, "zsh") {
		t.Errorf("Tf 应插入 fmt 参数，结果: %s", result)
	}

	// 无参数回退
	noArgs := Tf("app.shutdown")
	if noArgs != Tp("app.shutdown") {
		t.Errorf("Tf 无参数时应等价 Tp，结果: %q", noArgs)
	}
}

// TestInitFromEnv_ZhCN 验证中文环境变量检测
func TestInitFromEnv_ZhCN(t *testing.T) {
	// 保存并恢复环境变量
	for _, k := range []string{"CRABCLAW_LANG", "OPENACOSMI_LANG", "LC_ALL", "LANG"} {
		old := os.Getenv(k)
		defer os.Setenv(k, old)
		os.Unsetenv(k)
	}

	os.Setenv("LANG", "zh_CN.UTF-8")
	InitFromEnv()
	if GetLang() != LangZhCN {
		t.Errorf("LANG=zh_CN.UTF-8 应识别为中文，实际: %s", GetLang())
	}
}

// TestInitFromEnv_EnUS 验证英文环境变量检测
func TestInitFromEnv_EnUS(t *testing.T) {
	for _, k := range []string{"CRABCLAW_LANG", "OPENACOSMI_LANG", "LC_ALL", "LANG"} {
		old := os.Getenv(k)
		defer os.Setenv(k, old)
		os.Unsetenv(k)
	}

	os.Setenv("LANG", "en_US.UTF-8")
	InitFromEnv()
	if GetLang() != LangEnUS {
		t.Errorf("LANG=en_US.UTF-8 应识别为英文，实际: %s", GetLang())
	}
}

// TestInitFromEnv_Priority 验证 CRABCLAW_LANG > OPENACOSMI_LANG > LC_ALL > LANG 优先级
func TestInitFromEnv_Priority(t *testing.T) {
	for _, k := range []string{"CRABCLAW_LANG", "OPENACOSMI_LANG", "LC_ALL", "LANG"} {
		old := os.Getenv(k)
		defer os.Setenv(k, old)
		os.Unsetenv(k)
	}

	os.Setenv("LANG", "en_US.UTF-8")
	os.Setenv("OPENACOSMI_LANG", "zh-CN")
	os.Setenv("CRABCLAW_LANG", "en-US")
	InitFromEnv()
	if GetLang() != LangEnUS {
		t.Errorf("CRABCLAW_LANG 应优先于 OPENACOSMI_LANG 和 LANG，实际: %s", GetLang())
	}
}

// TestInitFromEnv_Default 验证无环境变量时默认中文
func TestInitFromEnv_Default(t *testing.T) {
	for _, k := range []string{"CRABCLAW_LANG", "OPENACOSMI_LANG", "LC_ALL", "LANG"} {
		old := os.Getenv(k)
		defer os.Setenv(k, old)
		os.Unsetenv(k)
	}

	Init(LangEnUS) // 先设为英文
	InitFromEnv()
	if GetLang() != LangZhCN {
		t.Errorf("无环境变量时应默认中文，实际: %s", GetLang())
	}
}

// TestKeyCompleteness_ZhEN 验证所有 zh-CN key 在 en-US 中都有对应翻译
func TestKeyCompleteness_ZhEN(t *testing.T) {
	mu.RLock()
	zhBundle := bundles[LangZhCN]
	enBundle := bundles[LangEnUS]
	mu.RUnlock()

	var missing []string
	for key := range zhBundle {
		if _, ok := enBundle[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("zh-CN 中有 %d 个 key 在 en-US 中缺失:\n%s",
			len(missing), strings.Join(missing, "\n"))
	}
}

// TestKeyCompleteness_ENZh 验证所有 en-US key 在 zh-CN 中都有对应翻译
func TestKeyCompleteness_ENZh(t *testing.T) {
	mu.RLock()
	zhBundle := bundles[LangZhCN]
	enBundle := bundles[LangEnUS]
	mu.RUnlock()

	var missing []string
	for key := range enBundle {
		if _, ok := zhBundle[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("en-US 中有 %d 个 key 在 zh-CN 中缺失:\n%s",
			len(missing), strings.Join(missing, "\n"))
	}
}

// TestKeyNotEmpty 验证所有翻译值非空
func TestKeyNotEmpty(t *testing.T) {
	mu.RLock()
	defer mu.RUnlock()

	for lang, bundle := range bundles {
		for key, val := range bundle {
			if strings.TrimSpace(val) == "" {
				t.Errorf("语言 %s 中 key %q 的值为空", lang, key)
			}
		}
	}
}

// TestOnboardingKeysExist 验证关键 onboarding key 均已注册
func TestOnboardingKeysExist(t *testing.T) {
	Init(LangZhCN)

	criticalKeys := []string{
		// gateway/wizard
		"onboard.daemon.confirm",
		"onboard.daemon.installed",
		"onboard.completion.prompt",
		"onboard.hatch.prompt",
		"onboard.welcome",
		"onboard.provider.select",
		// channels
		"onboard.ch.discord.title",
		"onboard.ch.slack.title",
		"onboard.ch.telegram.title",
		"onboard.ch.whatsapp.title",
		"onboard.ch.signal.title",
		"onboard.ch.imessage.title",
		// setup
		"onboard.ch.title",
		"onboard.skill.title",
		"onboard.hook.title",
		"onboard.remote.title",
		"onboard.auth.provider_select",
		"onboard.auth.cred_select",
	}

	for _, key := range criticalKeys {
		result := Tp(key)
		if strings.HasPrefix(result, "[missing:") {
			t.Errorf("关键 key %q 缺失翻译", key)
		}
	}
}

// TestKeyCount 验证 key 数量符合预期（防止意外丢失）
func TestKeyCount(t *testing.T) {
	mu.RLock()
	zhCount := len(bundles[LangZhCN])
	enCount := len(bundles[LangEnUS])
	mu.RUnlock()

	// init() 中有 17 基础 key + onboarding ~110 key = 至少 120 个
	minExpected := 120
	if zhCount < minExpected {
		t.Errorf("zh-CN key 数量 %d 低于预期 %d", zhCount, minExpected)
	}
	if enCount < minExpected {
		t.Errorf("en-US key 数量 %d 低于预期 %d", enCount, minExpected)
	}
	t.Logf("key 统计: zh-CN=%d, en-US=%d", zhCount, enCount)

	// 两种语言的 key 数量应一致
	if zhCount != enCount {
		t.Errorf("zh-CN (%d) 与 en-US (%d) key 数量不匹配", zhCount, enCount)
	}
}

// TestFmtPlaceholderConsistency 验证含 %s/%d 占位符的 key 在两种语言中格式一致
func TestFmtPlaceholderConsistency(t *testing.T) {
	mu.RLock()
	zhBundle := bundles[LangZhCN]
	enBundle := bundles[LangEnUS]
	mu.RUnlock()

	countPlaceholders := func(s string) int {
		count := 0
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '%' && (s[i+1] == 's' || s[i+1] == 'd' || s[i+1] == 'v') {
				count++
			}
		}
		return count
	}

	for key, zhVal := range zhBundle {
		enVal, ok := enBundle[key]
		if !ok {
			continue // TestKeyCompleteness 会报告
		}
		zhN := countPlaceholders(zhVal)
		enN := countPlaceholders(enVal)
		if zhN != enN {
			t.Errorf("key %q 的 fmt 占位符数量不一致: zh=%d, en=%d\n  zh: %s\n  en: %s",
				key, zhN, enN, zhVal, enVal)
		}
	}
}

// TestBracePlaceholderConsistency 验证含 {param} 占位符的 key 在两种语言中一致
func TestBracePlaceholderConsistency(t *testing.T) {
	mu.RLock()
	zhBundle := bundles[LangZhCN]
	enBundle := bundles[LangEnUS]
	mu.RUnlock()

	extractBracePlaceholders := func(s string) []string {
		var placeholders []string
		for {
			start := strings.Index(s, "{")
			if start == -1 {
				break
			}
			// Skip %s/%d style format verbs
			if start > 0 && s[start-1] == '%' {
				s = s[start+1:]
				continue
			}
			end := strings.Index(s[start:], "}")
			if end == -1 {
				break
			}
			placeholder := s[start : start+end+1]
			placeholders = append(placeholders, placeholder)
			s = s[start+end+1:]
		}
		return placeholders
	}

	for key, zhVal := range zhBundle {
		enVal, ok := enBundle[key]
		if !ok {
			continue
		}
		zhP := extractBracePlaceholders(zhVal)
		enP := extractBracePlaceholders(enVal)
		if len(zhP) != len(enP) {
			t.Errorf("key %q 的 {param} 占位符数量不一致: zh=%v, en=%v", key, zhP, enP)
		}
		// 检查占位符名称一致
		zhSet := make(map[string]bool)
		for _, p := range zhP {
			zhSet[p] = true
		}
		for _, p := range enP {
			if !zhSet[p] {
				t.Errorf("key %q 的占位符 %s 仅在 en-US 中存在", key, p)
			}
		}
	}
}

// suppress unused import warning
var _ = fmt.Sprintf
