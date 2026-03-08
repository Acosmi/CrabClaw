package infra

// TS 对照: src/infra/exec-safety.ts
// 命令执行安全检查 — 防注入、防路径穿越、防恶意参数。

import (
	"regexp"
	"strings"
)

// shellMetachars 匹配 Shell 元字符（注入风险）。
// TS 对照: exec-safety.ts L1 SHELL_METACHARS
var shellMetachars = regexp.MustCompile(`[;&|` + "`" + `$<>]`)

// controlChars 匹配控制字符（换行/回车注入）。
// TS 对照: exec-safety.ts L2 CONTROL_CHARS
var controlChars = regexp.MustCompile(`[\r\n]`)

// quoteChars 匹配引号字符。
// TS 对照: exec-safety.ts L3 QUOTE_CHARS
var quoteChars = regexp.MustCompile(`["']`)

// bareNamePattern 匹配仅包含安全字符的裸名（命令名）。
// TS 对照: exec-safety.ts L4 BARE_NAME_PATTERN
var bareNamePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// isLikelyPath 判断值是否为路径形式（以 . ~ / \ 开头或包含路径分隔符）。
// TS 对照: exec-safety.ts isLikelyPath (L6-14)
func isLikelyPath(value string) bool {
	if strings.HasPrefix(value, ".") || strings.HasPrefix(value, "~") {
		return true
	}
	if strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return true
	}
	// Windows 绝对路径: C:\ 或 C:/
	if len(value) >= 3 && value[1] == ':' && (value[2] == '/' || value[2] == '\\') {
		ch := value[0]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			return true
		}
	}
	return false
}

// IsSafeExecutableValue 验证命令名或路径参数是否安全。
// 规则（对照 TS 实现 exec-safety.ts isSafeExecutableValue L16-44）：
//  1. 空值或仅含空白字符 -> 不安全
//  2. 包含 NUL 字节 -> 不安全
//  3. 包含控制字符（\r \n）-> 不安全
//  4. 包含 Shell 元字符（; & | ` $ < >）-> 不安全
//  5. 包含引号（" '）-> 不安全
//  6. 路径形式（含 / \ 或以 . ~ 开头）-> 安全
//  7. 以 - 开头（选项注入）-> 不安全
//  8. 符合裸名模式 [A-Za-z0-9._+-]+ -> 安全
//  9. 其他 -> 不安全
func IsSafeExecutableValue(value string) bool {
	if value == "" {
		return false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	// NUL 字节检查
	if strings.ContainsRune(trimmed, 0) {
		return false
	}
	// 控制字符检查
	if controlChars.MatchString(trimmed) {
		return false
	}
	// Shell 元字符检查
	if shellMetachars.MatchString(trimmed) {
		return false
	}
	// 引号检查
	if quoteChars.MatchString(trimmed) {
		return false
	}
	// 路径形式允许通过
	if isLikelyPath(trimmed) {
		return true
	}
	// 以 - 开头视为选项注入，拒绝
	if strings.HasPrefix(trimmed, "-") {
		return false
	}
	// 裸名模式
	return bareNamePattern.MatchString(trimmed)
}
