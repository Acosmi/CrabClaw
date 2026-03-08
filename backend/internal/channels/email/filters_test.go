package email

import (
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// --- 入站过滤测试 ---

func TestFilterInbound_SystemSent(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{
			"From":                 {"user@test.com"},
			"X-OpenAcosmi-Channel": {"email"},
		}},
	}
	fc := DefaultFilterConfig()
	passed, filtered := FilterInbound(msgs, fc)
	if filtered != 1 {
		t.Errorf("filtered = %d, want 1", filtered)
	}
	if len(passed) != 0 {
		t.Errorf("passed = %d, want 0", len(passed))
	}
}

func TestFilterInbound_AutoSubmitted(t *testing.T) {
	tests := []struct {
		value    string
		filtered bool
	}{
		{"auto-generated", true},
		{"auto-replied", true},
		{"auto-notified", true},
		{"Auto-Generated", true},
		{"no", false},
		{"", false},
	}
	for _, tt := range tests {
		header := map[string][]string{"From": {"user@test.com"}}
		if tt.value != "" {
			header["Auto-Submitted"] = []string{tt.value}
		}
		msgs := []RawEmailMessage{{UID: 1, Header: header}}
		fc := DefaultFilterConfig()
		_, filtered := FilterInbound(msgs, fc)
		want := 0
		if tt.filtered {
			want = 1
		}
		if filtered != want {
			t.Errorf("Auto-Submitted=%q: filtered=%d, want %d", tt.value, filtered, want)
		}
	}
}

func TestFilterInbound_AutoSubmitted_Disabled(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{
			"From":           {"user@test.com"},
			"Auto-Submitted": {"auto-generated"},
		}},
	}
	fc := DefaultFilterConfig()
	fc.IgnoreAutoSubmitted = false
	passed, _ := FilterInbound(msgs, fc)
	if len(passed) != 1 {
		t.Errorf("should pass when IgnoreAutoSubmitted=false, got %d", len(passed))
	}
}

func TestFilterInbound_BounceMailerDaemon(t *testing.T) {
	tests := []struct {
		from     string
		filtered bool
	}{
		{"MAILER-DAEMON@mail.example.com", true},
		{"postmaster@example.com", true},
		{"bounce+abc@example.com", true},
		{"mail-daemon@example.com", true},
		{"normal@example.com", false},
	}
	for _, tt := range tests {
		msgs := []RawEmailMessage{
			{UID: 1, Header: map[string][]string{"From": {tt.from}}},
		}
		fc := DefaultFilterConfig()
		_, filtered := FilterInbound(msgs, fc)
		want := 0
		if tt.filtered {
			want = 1
		}
		if filtered != want {
			t.Errorf("From=%q: filtered=%d, want %d", tt.from, filtered, want)
		}
	}
}

func TestFilterInbound_ReturnPathBounce(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{
			"From":        {"system@example.com"},
			"Return-Path": {"<>"},
		}},
	}
	fc := DefaultFilterConfig()
	_, filtered := FilterInbound(msgs, fc)
	if filtered != 1 {
		t.Errorf("Return-Path:<> should be filtered, got %d", filtered)
	}
}

func TestFilterInbound_NoReply(t *testing.T) {
	tests := []struct {
		from     string
		filtered bool
	}{
		{"noreply@company.com", true},
		{"no-reply@company.com", true},
		{"no_reply@company.com", true},
		{"donotreply@company.com", true},
		{"do-not-reply@company.com", true},
		{"notification@company.com", true},
		{"notifications@company.com", true},
		{"alert@company.com", true},
		{"alerts@company.com", true},
		{"support@company.com", false},
		{"user@company.com", false},
	}
	for _, tt := range tests {
		msgs := []RawEmailMessage{
			{UID: 1, Header: map[string][]string{"From": {tt.from}}},
		}
		fc := DefaultFilterConfig()
		_, filtered := FilterInbound(msgs, fc)
		want := 0
		if tt.filtered {
			want = 1
		}
		if filtered != want {
			t.Errorf("From=%q: filtered=%d, want %d", tt.from, filtered, want)
		}
	}
}

func TestFilterInbound_MailingList(t *testing.T) {
	tests := []struct {
		name     string
		header   map[string][]string
		filtered bool
	}{
		{
			"List-Id",
			map[string][]string{"From": {"list@example.com"}, "List-Id": {"<dev.example.com>"}},
			true,
		},
		{
			"List-Unsubscribe",
			map[string][]string{"From": {"list@example.com"}, "List-Unsubscribe": {"<mailto:unsub@example.com>"}},
			true,
		},
		{
			"Precedence bulk",
			map[string][]string{"From": {"list@example.com"}, "Precedence": {"bulk"}},
			true,
		},
		{
			"Precedence list",
			map[string][]string{"From": {"list@example.com"}, "Precedence": {"list"}},
			true,
		},
		{
			"Precedence junk",
			map[string][]string{"From": {"list@example.com"}, "Precedence": {"junk"}},
			true,
		},
		{
			"no list headers",
			map[string][]string{"From": {"user@example.com"}},
			false,
		},
	}
	for _, tt := range tests {
		msgs := []RawEmailMessage{{UID: 1, Header: tt.header}}
		fc := DefaultFilterConfig()
		_, filtered := FilterInbound(msgs, fc)
		want := 0
		if tt.filtered {
			want = 1
		}
		if filtered != want {
			t.Errorf("%s: filtered=%d, want %d", tt.name, filtered, want)
		}
	}
}

func TestFilterInbound_SelfSent(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{"From": {"robot@company.com"}}},
	}
	fc := DefaultFilterConfig()
	fc.SelfAddress = "robot@company.com"
	_, filtered := FilterInbound(msgs, fc)
	if filtered != 1 {
		t.Errorf("self-sent should be filtered, got %d", filtered)
	}
}

func TestFilterInbound_SelfSent_CaseInsensitive(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{"From": {"Robot@Company.COM"}}},
	}
	fc := DefaultFilterConfig()
	fc.SelfAddress = "robot@company.com"
	_, filtered := FilterInbound(msgs, fc)
	if filtered != 1 {
		t.Errorf("self-sent (case insensitive) should be filtered, got %d", filtered)
	}
}

func TestFilterInbound_SelfSent_Disabled(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{"From": {"robot@company.com"}}},
	}
	fc := DefaultFilterConfig()
	fc.SelfAddress = "robot@company.com"
	fc.IgnoreSelfSent = false
	passed, _ := FilterInbound(msgs, fc)
	if len(passed) != 1 {
		t.Errorf("should pass when IgnoreSelfSent=false, got %d", len(passed))
	}
}

func TestFilterInbound_MultipleMessages(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{"From": {"user@test.com"}}},                                      // pass
		{UID: 2, Header: map[string][]string{"From": {"noreply@company.com"}}},                                // filtered
		{UID: 3, Header: map[string][]string{"From": {"user2@test.com"}}},                                     // pass
		{UID: 4, Header: map[string][]string{"From": {"list@example.com"}, "List-Id": {"<dev.example.com>"}}}, // filtered
		{UID: 5, Header: map[string][]string{"From": {"user@test.com"}, "X-OpenAcosmi-Channel": {"email"}}},   // filtered (system)
	}
	fc := DefaultFilterConfig()
	passed, filtered := FilterInbound(msgs, fc)
	if filtered != 3 {
		t.Errorf("filtered = %d, want 3", filtered)
	}
	if len(passed) != 2 {
		t.Errorf("passed = %d, want 2", len(passed))
	}
	if passed[0].UID != 1 || passed[1].UID != 3 {
		t.Errorf("passed UIDs = [%d, %d], want [1, 3]", passed[0].UID, passed[1].UID)
	}
}

func TestFilterInbound_NormalMessage_Passes(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{
			"From":    {"colleague@company.com"},
			"To":      {"me@company.com"},
			"Subject": {"项目进度更新"},
		}},
	}
	fc := DefaultFilterConfig()
	fc.SelfAddress = "me@company.com"
	passed, filtered := FilterInbound(msgs, fc)
	if filtered != 0 {
		t.Errorf("normal message should not be filtered, got %d", filtered)
	}
	if len(passed) != 1 {
		t.Errorf("passed = %d, want 1", len(passed))
	}
}

func TestFilterInbound_FromWithDisplayName(t *testing.T) {
	msgs := []RawEmailMessage{
		{UID: 1, Header: map[string][]string{"From": {"No Reply <noreply@company.com>"}}},
	}
	fc := DefaultFilterConfig()
	_, filtered := FilterInbound(msgs, fc)
	if filtered != 1 {
		t.Errorf("noreply with display name should be filtered, got %d", filtered)
	}
}

func TestNewFilterConfigFromAccount(t *testing.T) {
	cfg := &types.EmailAccountConfig{
		Address: "robot@company.com",
		Routing: &types.EmailRoutingConfig{
			IgnoreAutoSubmitted: true,
			IgnoreMailingList:   false,
			IgnoreNoReply:       true,
			IgnoreSelfSent:      false,
		},
	}
	fc := NewFilterConfigFromAccount(cfg)
	if fc.SelfAddress != "robot@company.com" {
		t.Errorf("SelfAddress = %q", fc.SelfAddress)
	}
	if !fc.IgnoreAutoSubmitted {
		t.Error("IgnoreAutoSubmitted should be true")
	}
	if fc.IgnoreMailingList {
		t.Error("IgnoreMailingList should be false")
	}
	if !fc.IgnoreNoReply {
		t.Error("IgnoreNoReply should be true")
	}
	if fc.IgnoreSelfSent {
		t.Error("IgnoreSelfSent should be false")
	}
}

// --- 出站频率限制测试 ---

func TestSendRateLimiter_NoLimits(t *testing.T) {
	rl := NewSendRateLimiter(0, 0)
	if !rl.Allow() {
		t.Error("no limits should always allow")
	}
}

func TestSendRateLimiter_HourlyLimit(t *testing.T) {
	rl := NewSendRateLimiter(3, 0)

	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Errorf("send %d should be allowed", i+1)
		}
		rl.Record()
	}

	if rl.Allow() {
		t.Error("4th send should be denied (hourly limit 3)")
	}

	hourly, _ := rl.Stats()
	if hourly != 3 {
		t.Errorf("hourly = %d, want 3", hourly)
	}
}

func TestSendRateLimiter_DailyLimit(t *testing.T) {
	rl := NewSendRateLimiter(0, 5)

	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("send %d should be allowed", i+1)
		}
		rl.Record()
	}

	if rl.Allow() {
		t.Error("6th send should be denied (daily limit 5)")
	}

	_, daily := rl.Stats()
	if daily != 5 {
		t.Errorf("daily = %d, want 5", daily)
	}
}

func TestSendRateLimiter_WindowSliding(t *testing.T) {
	rl := NewSendRateLimiter(2, 0)

	// 手动注入过期记录
	rl.mu.Lock()
	past := time.Now().Add(-2 * time.Hour)
	rl.hourlySends = []time.Time{past, past}
	rl.mu.Unlock()

	// 过期记录不应阻止新发送
	if !rl.Allow() {
		t.Error("expired records should not block")
	}
}

func TestSendRateLimiter_Stats(t *testing.T) {
	rl := NewSendRateLimiter(10, 100)
	rl.Record()
	rl.Record()

	hourly, daily := rl.Stats()
	if hourly != 2 {
		t.Errorf("hourly = %d, want 2", hourly)
	}
	if daily != 2 {
		t.Errorf("daily = %d, want 2", daily)
	}
}

// --- Header 辅助函数测试 ---

func TestHeaderGet_CaseInsensitive(t *testing.T) {
	header := map[string][]string{
		"Content-Type": {"text/plain"},
	}
	if headerGet(header, "content-type") != "text/plain" {
		t.Error("case-insensitive lookup failed")
	}
	if headerGet(header, "Content-Type") != "text/plain" {
		t.Error("exact match failed")
	}
	if headerGet(header, "X-Missing") != "" {
		t.Error("missing key should return empty")
	}
}

func TestHeaderHas(t *testing.T) {
	header := map[string][]string{
		"List-Id": {"<dev.example.com>"},
	}
	if !headerHas(header, "List-Id") {
		t.Error("List-Id should exist")
	}
	if headerHas(header, "X-Missing") {
		t.Error("X-Missing should not exist")
	}
}

func TestIsAutoSubmitted(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"auto-generated", true},
		{"auto-replied", true},
		{"no", false},
		{"", false},
	}
	for _, tt := range tests {
		header := map[string][]string{}
		if tt.val != "" {
			header["Auto-Submitted"] = []string{tt.val}
		}
		got := isAutoSubmitted(header)
		if got != tt.want {
			t.Errorf("isAutoSubmitted(%q) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestIsMailingList(t *testing.T) {
	tests := []struct {
		name   string
		header map[string][]string
		want   bool
	}{
		{"List-Id", map[string][]string{"List-Id": {"<dev>"}}, true},
		{"List-Unsubscribe", map[string][]string{"List-Unsubscribe": {"<mailto:unsub>"}}, true},
		{"Precedence bulk", map[string][]string{"Precedence": {"bulk"}}, true},
		{"Precedence list", map[string][]string{"Precedence": {"list"}}, true},
		{"Precedence junk", map[string][]string{"Precedence": {"junk"}}, true},
		{"normal", map[string][]string{}, false},
	}
	for _, tt := range tests {
		got := isMailingList(tt.header)
		if got != tt.want {
			t.Errorf("%s: isMailingList = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsSelfSent(t *testing.T) {
	header := map[string][]string{"From": {"Robot <robot@company.com>"}}
	if !isSelfSent(header, "robot@company.com") {
		t.Error("should detect self-sent")
	}
	if isSelfSent(header, "other@company.com") {
		t.Error("should not detect different sender")
	}
}

func TestExtractEmailAddrFromHeader(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@test.com", "user@test.com"},
		{"User Name <user@test.com>", "user@test.com"},
		{"\"Display\" <USER@Test.COM>", "user@test.com"},
	}
	for _, tt := range tests {
		got := extractEmailAddrFromHeader(tt.input)
		if got != tt.want {
			t.Errorf("extractEmailAddrFromHeader(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
