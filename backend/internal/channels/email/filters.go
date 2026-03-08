package email

// filters.go — Phase 8: 入站过滤 + 出站频控 + 防环
// 入站: 自动邮件/退信/列表邮件/noreply/自发自收/系统追踪头 → 跳过不回复
// 出站: 每账号每小时/每日频率限制 → 超频降级为只收不发

import (
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// FilterResult 过滤结果
type FilterResult struct {
	Filtered bool   // true = 应跳过（不回复）
	Reason   string // 过滤原因（用于日志/审计）
	Rule     string // 命中的规则名
}

// FilterConfig 过滤配置（从 EmailRoutingConfig 提取）
type FilterConfig struct {
	IgnoreAutoSubmitted bool
	IgnoreMailingList   bool
	IgnoreNoReply       bool
	IgnoreSelfSent      bool
	SelfAddress         string // 本账号邮箱地址，用于自发自收检测
}

// DefaultFilterConfig 返回默认过滤配置（全部启用）
func DefaultFilterConfig() FilterConfig {
	return FilterConfig{
		IgnoreAutoSubmitted: true,
		IgnoreMailingList:   true,
		IgnoreNoReply:       true,
		IgnoreSelfSent:      true,
	}
}

// NewFilterConfigFromAccount 从账号配置构造过滤配置
func NewFilterConfigFromAccount(cfg *types.EmailAccountConfig) FilterConfig {
	fc := DefaultFilterConfig()
	fc.SelfAddress = cfg.Address
	if cfg.Routing != nil {
		fc.IgnoreAutoSubmitted = cfg.Routing.IgnoreAutoSubmitted
		fc.IgnoreMailingList = cfg.Routing.IgnoreMailingList
		fc.IgnoreNoReply = cfg.Routing.IgnoreNoReply
		fc.IgnoreSelfSent = cfg.Routing.IgnoreSelfSent
	}
	return fc
}

// FilterInbound 对入站邮件应用过滤规则。
// 返回通过过滤的消息列表 + 被过滤的数量。
// 在 MIME 解析之前执行（仅依赖 RawEmailMessage.Header），降低无效邮件的解析开销。
func FilterInbound(msgs []RawEmailMessage, fc FilterConfig) (passed []RawEmailMessage, filteredCount int) {
	for _, msg := range msgs {
		result := checkInboundFilters(msg.Header, fc)
		if result.Filtered {
			slog.Debug("email: inbound message filtered",
				"uid", msg.UID,
				"rule", result.Rule,
				"reason", result.Reason,
			)
			filteredCount++
			continue
		}
		passed = append(passed, msg)
	}
	return
}

// checkInboundFilters 对单条消息依次检查所有过滤规则
func checkInboundFilters(header map[string][]string, fc FilterConfig) FilterResult {
	// 规则 1: 系统已发送追踪头（最高优先级，防环核心）
	if isSystemSent(header) {
		return FilterResult{Filtered: true, Rule: "system_sent", Reason: "X-OpenAcosmi-Channel header detected"}
	}

	// 规则 2: Auto-Submitted（RFC 3834）
	if fc.IgnoreAutoSubmitted && isAutoSubmitted(header) {
		return FilterResult{Filtered: true, Rule: "auto_submitted", Reason: "Auto-Submitted header is not 'no'"}
	}

	// 规则 3: Bounce / mailer-daemon / postmaster
	if isBounceOrDaemon(header) {
		return FilterResult{Filtered: true, Rule: "bounce_daemon", Reason: "sender is bounce/mailer-daemon/postmaster"}
	}

	// 规则 4: noreply / no-reply
	if fc.IgnoreNoReply && isNoReply(header) {
		return FilterResult{Filtered: true, Rule: "noreply", Reason: "sender is noreply address"}
	}

	// 规则 5: 列表邮件 (List-Id / List-Unsubscribe / Precedence: bulk|list)
	if fc.IgnoreMailingList && isMailingList(header) {
		return FilterResult{Filtered: true, Rule: "mailing_list", Reason: "mailing list headers detected"}
	}

	// 规则 6: 自发自收
	if fc.IgnoreSelfSent && fc.SelfAddress != "" && isSelfSent(header, fc.SelfAddress) {
		return FilterResult{Filtered: true, Rule: "self_sent", Reason: "sender matches own address"}
	}

	return FilterResult{Filtered: false}
}

// --- 规则实现 ---

// isSystemSent 检测系统已发送追踪头（防环核心）
func isSystemSent(header map[string][]string) bool {
	return headerHas(header, "X-Openacosmi-Channel") || headerHas(header, "X-OpenAcosmi-Channel")
}

// isAutoSubmitted 检测自动提交邮件 (RFC 3834)
// Auto-Submitted: auto-generated / auto-replied / auto-notified → 过滤
// Auto-Submitted: no → 不过滤
func isAutoSubmitted(header map[string][]string) bool {
	val := headerGet(header, "Auto-Submitted")
	if val == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(val))
	return lower != "no"
}

// isBounceOrDaemon 检测退信 / 系统邮件
func isBounceOrDaemon(header map[string][]string) bool {
	from := strings.ToLower(headerGet(header, "From"))
	sender := extractEmailAddrFromHeader(from)

	bouncePrefixes := []string{
		"mailer-daemon@", "postmaster@",
		"bounce", "mail-daemon@",
	}
	for _, prefix := range bouncePrefixes {
		if strings.HasPrefix(sender, prefix) {
			return true
		}
	}

	// Return-Path: <> 是标准退信标记（RFC 5321 Section 4.5.5）
	// Return-Path 缺失不触发过滤
	if headerGet(header, "Return-Path") == "<>" {
		return true
	}

	return false
}

// isNoReply 检测 noreply 地址
func isNoReply(header map[string][]string) bool {
	from := strings.ToLower(headerGet(header, "From"))
	sender := extractEmailAddrFromHeader(from)

	noReplyPatterns := []string{
		"noreply@", "no-reply@", "no_reply@",
		"donotreply@", "do-not-reply@", "do_not_reply@",
		"notification@", "notifications@",
		"alert@", "alerts@",
	}
	for _, pattern := range noReplyPatterns {
		if strings.HasPrefix(sender, pattern) {
			return true
		}
	}
	return false
}

// isMailingList 检测列表邮件
func isMailingList(header map[string][]string) bool {
	if headerHas(header, "List-Id") || headerHas(header, "List-Unsubscribe") {
		return true
	}
	precedence := strings.ToLower(headerGet(header, "Precedence"))
	return precedence == "bulk" || precedence == "list" || precedence == "junk"
}

// isSelfSent 检测自发自收
func isSelfSent(header map[string][]string, selfAddr string) bool {
	from := strings.ToLower(headerGet(header, "From"))
	sender := extractEmailAddrFromHeader(from)
	return sender == strings.ToLower(selfAddr)
}

// --- Header 辅助 ---

// headerGet 从 header map 获取值（case-insensitive key lookup）
func headerGet(header map[string][]string, key string) string {
	// 精确匹配
	if vals, ok := header[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	// Case-insensitive 降级
	lowerKey := strings.ToLower(key)
	for k, vals := range header {
		if strings.ToLower(k) == lowerKey && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

// headerHas 检查 header 是否存在（case-insensitive）
func headerHas(header map[string][]string, key string) bool {
	return headerGet(header, key) != ""
}

// extractEmailAddrFromHeader 从 "Name <addr>" 格式提取纯邮箱地址
func extractEmailAddrFromHeader(from string) string {
	addr, err := mail.ParseAddress(from)
	if err == nil {
		return strings.ToLower(addr.Address)
	}
	// 降级: 直接用 extractSenderAddress
	return extractSenderAddress(from)
}

// --- 出站频率限制 ---

// SendRateLimiter 每账号出站频率限制器
type SendRateLimiter struct {
	mu          sync.Mutex
	hourlyLimit int
	dailyLimit  int

	// 滑动窗口记录
	hourlySends []time.Time
	dailySends  []time.Time
}

// NewSendRateLimiter 创建频率限制器
func NewSendRateLimiter(hourlyLimit, dailyLimit int) *SendRateLimiter {
	return &SendRateLimiter{
		hourlyLimit: hourlyLimit,
		dailyLimit:  dailyLimit,
	}
}

// Allow 检查是否允许发送。返回 true 表示允许，false 表示已达频率上限。
func (rl *SendRateLimiter) Allow() bool {
	if rl.hourlyLimit <= 0 && rl.dailyLimit <= 0 {
		return true // 无限制
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rl.cleanup(now)

	if rl.hourlyLimit > 0 && len(rl.hourlySends) >= rl.hourlyLimit {
		return false
	}
	if rl.dailyLimit > 0 && len(rl.dailySends) >= rl.dailyLimit {
		return false
	}
	return true
}

// Record 记录一次发送
func (rl *SendRateLimiter) Record() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rl.hourlySends = append(rl.hourlySends, now)
	rl.dailySends = append(rl.dailySends, now)
}

// Stats 返回当前发送统计
func (rl *SendRateLimiter) Stats() (hourly, daily int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.cleanup(time.Now())
	return len(rl.hourlySends), len(rl.dailySends)
}

// cleanup 清理过期的发送记录
func (rl *SendRateLimiter) cleanup(now time.Time) {
	hourAgo := now.Add(-1 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	// 清理小时窗口
	idx := 0
	for idx < len(rl.hourlySends) && rl.hourlySends[idx].Before(hourAgo) {
		idx++
	}
	if idx > 0 {
		rl.hourlySends = rl.hourlySends[idx:]
	}

	// 清理日窗口
	idx = 0
	for idx < len(rl.dailySends) && rl.dailySends[idx].Before(dayAgo) {
		idx++
	}
	if idx > 0 {
		rl.dailySends = rl.dailySends[idx:]
	}
}
