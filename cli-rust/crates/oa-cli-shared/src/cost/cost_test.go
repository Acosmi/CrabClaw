package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Cost 单元测试 — JSONL 解析、统计计算、格式化输出、HTTP mock
// ============================================================================

// ---------- 测试辅助 ----------

// writeJSONLFile 创建一个临时 JSONL 文件。
func writeJSONLFile(t *testing.T, dir, name string, entries []map[string]interface{}) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.WriteString("\n")
	}
	return path
}

// ---------- scanTranscriptFile ----------

func TestScanTranscriptFile(t *testing.T) {
	dir := t.TempDir()
	entries := []map[string]interface{}{
		{"type": "api_response", "timestampMs": 1000, "role": "assistant",
			"usage": map[string]interface{}{"inputTokens": 100, "outputTokens": 50}},
		{"type": "tool_use", "timestampMs": 2000, "tool": "Read"},
		{"type": "user", "timestampMs": 3000, "role": "user"},
	}
	path := writeJSONLFile(t, dir, "transcript.jsonl", entries)

	var count int
	err := scanTranscriptFile(path, func(entry transcriptEntry) {
		count++
	})
	if err != nil {
		t.Fatalf("scanTranscriptFile failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 entries, got %d", count)
	}
}

func TestScanTranscriptFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte{}, 0o600)

	var count int
	err := scanTranscriptFile(path, func(entry transcriptEntry) {
		count++
	})
	if err != nil {
		t.Fatalf("expected no error for empty file, got: %v", err)
	}
	if count != 0 {
		t.Error("expected 0 entries for empty file")
	}
}

func TestScanTranscriptFile_NonExistent(t *testing.T) {
	err := scanTranscriptFile("/nonexistent/path", func(entry transcriptEntry) {})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestScanTranscriptFile_Corrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jsonl")
	os.WriteFile(path, []byte("not json\n{\"type\":\"valid\"}\nbroken{{\n"), 0o600)

	var validCount int
	err := scanTranscriptFile(path, func(entry transcriptEntry) {
		validCount++
	})
	if err != nil {
		t.Fatalf("corrupted lines should be skipped, got error: %v", err)
	}
	if validCount != 1 {
		t.Errorf("expected 1 valid entry, got %d", validCount)
	}
}

// ---------- computeLatencyStats ----------

func TestComputeLatencyStats(t *testing.T) {
	latencies := []float64{100, 200, 300, 400, 500}
	stats := computeLatencyStats(latencies)

	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.Count != 5 {
		t.Errorf("expected count=5, got %d", stats.Count)
	}
	if stats.Min != 100 {
		t.Errorf("expected min=100, got %f", stats.Min)
	}
	if stats.Max != 500 {
		t.Errorf("expected max=500, got %f", stats.Max)
	}
	if stats.Avg != 300 {
		t.Errorf("expected avg=300, got %f", stats.Avg)
	}
	if stats.Median != 300 {
		t.Errorf("expected median=300, got %f", stats.Median)
	}
}

func TestComputeLatencyStats_Empty(t *testing.T) {
	stats := computeLatencyStats(nil)
	if stats != nil {
		t.Error("expected nil for empty latencies")
	}
}

func TestComputeLatencyStats_Single(t *testing.T) {
	stats := computeLatencyStats([]float64{42})
	if stats == nil {
		t.Fatal("expected non-nil")
	}
	if stats.Min != 42 || stats.Max != 42 || stats.Avg != 42 {
		t.Errorf("single value stats: min=%f max=%f avg=%f", stats.Min, stats.Max, stats.Avg)
	}
}

// ---------- percentile ----------

func TestPercentile(t *testing.T) {
	sorted := []float64{10, 20, 30, 40, 50}

	// p50 (50th percentile) → index = 50/100 * 4 = 2.0 → sorted[2] = 30
	p50 := percentile(sorted, 50)
	if p50 != 30 {
		t.Errorf("p50 expected 30, got %f", p50)
	}

	// p0 → index = 0 → sorted[0] = 10
	p0 := percentile(sorted, 0)
	if p0 != 10 {
		t.Errorf("p0 expected 10, got %f", p0)
	}

	// p100 → index = 100/100 * 4 = 4.0 → sorted[4] = 50
	p100 := percentile(sorted, 100)
	if p100 != 50 {
		t.Errorf("p100 expected 50, got %f", p100)
	}

	// p25 → index = 25/100 * 4 = 1.0 → sorted[1] = 20
	p25 := percentile(sorted, 25)
	if p25 != 20 {
		t.Errorf("p25 expected 20, got %f", p25)
	}
}

// ---------- msToDateString ----------

func TestMsToDateString(t *testing.T) {
	// 2026-01-15 00:00:00 UTC = 1768435200000
	ms := int64(1768435200000)
	got := msToDateString(ms)
	if got != "2026-01-15" {
		t.Errorf("expected 2026-01-15, got %q", got)
	}
}

func TestMsToDateString_Zero(t *testing.T) {
	got := msToDateString(0)
	if got != "1970-01-01" {
		t.Errorf("expected 1970-01-01, got %q", got)
	}
}

// ---------- ClampPercent ----------

func TestClampPercent(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{50, 50},
		{0, 0},
		{100, 100},
		{-10, 0},
		{150, 100},
		{math.NaN(), 0},
		{math.Inf(1), 0},
		{math.Inf(-1), 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%f", tt.input), func(t *testing.T) {
			got := ClampPercent(tt.input)
			if got != tt.want {
				t.Errorf("ClampPercent(%f) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- parseISO ----------

func TestParseISO(t *testing.T) {
	ms := parseISO("2026-01-15T00:00:00Z")
	if ms == 0 {
		t.Error("expected non-zero for valid ISO string")
	}

	ms2 := parseISO("not-a-date")
	if ms2 != 0 {
		t.Error("expected 0 for invalid ISO string")
	}
}

// ---------- FormatResetRemaining ----------

func TestFormatResetRemaining(t *testing.T) {
	now := int64(1000000)

	// nil → ""
	if got := FormatResetRemaining(nil, now); got != "" {
		t.Errorf("nil target should return empty, got %q", got)
	}

	// past → "now"
	past := now - 1000
	if got := FormatResetRemaining(&past, now); got != "now" {
		t.Errorf("past target should return 'now', got %q", got)
	}

	// 30 min ahead
	m30 := now + 30*60000
	got := FormatResetRemaining(&m30, now)
	if !strings.Contains(got, "m") {
		t.Errorf("30m should contain 'm', got %q", got)
	}

	// 3 hours ahead
	h3 := now + 3*3600000
	got = FormatResetRemaining(&h3, now)
	if !strings.Contains(got, "h") {
		t.Errorf("3h should contain 'h', got %q", got)
	}
}

// ---------- FormatUsageReportLines ----------

func TestFormatUsageReportLines_Empty(t *testing.T) {
	summary := &ProviderUsageSummary{}
	lines := FormatUsageReportLines(summary, 0)
	if len(lines) != 1 || !strings.Contains(lines[0], "no provider") {
		t.Errorf("expected 'no provider' message, got %v", lines)
	}
}

func TestFormatUsageReportLines_WithData(t *testing.T) {
	summary := &ProviderUsageSummary{
		Providers: []ProviderUsageSnapshot{
			{
				Provider:    ProviderAnthropic,
				DisplayName: "Claude",
				Windows: []ProviderUsageWindow{
					{Label: "Daily", UsedPercent: 30},
				},
			},
		},
	}
	lines := FormatUsageReportLines(summary, 0)
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines, got %d", len(lines))
	}
	// Should contain provider name
	foundClaude := false
	for _, l := range lines {
		if strings.Contains(l, "Claude") {
			foundClaude = true
		}
	}
	if !foundClaude {
		t.Error("expected 'Claude' in output lines")
	}
}

func TestFormatUsageSummaryLine(t *testing.T) {
	summary := &ProviderUsageSummary{
		Providers: []ProviderUsageSnapshot{
			{
				Provider:    ProviderAnthropic,
				DisplayName: "Claude",
				Windows: []ProviderUsageWindow{
					{Label: "Daily", UsedPercent: 40},
				},
			},
		},
	}
	line := FormatUsageSummaryLine(summary, 0)
	if line == "" {
		t.Fatal("expected non-empty summary line")
	}
	if !strings.Contains(line, "Claude") {
		t.Error("expected 'Claude' in summary line")
	}
	if !strings.Contains(line, "60%") { // 100-40
		t.Errorf("expected '60%%' remaining, got %q", line)
	}
}

// ---------- FetchProviderUsage with HTTP mock ----------

func TestFetchProviderUsage_UnsupportedProvider(t *testing.T) {
	ctx := context.Background()
	snap, err := FetchProviderUsage(ctx, ProviderAuth{
		Provider: UsageProviderId("unknown-provider"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.Error != "Unsupported provider" {
		t.Errorf("expected 'Unsupported provider', got %q", snap.Error)
	}
}

func TestFetchProviderUsage_Xiaomi(t *testing.T) {
	ctx := context.Background()
	snap, err := FetchProviderUsage(ctx, ProviderAuth{
		Provider: ProviderXiaomi,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Provider != ProviderXiaomi {
		t.Errorf("expected xiaomi provider, got %v", snap.Provider)
	}
}

// ---------- HTTP helpers ----------

func TestReadJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "value"})
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]string
	if err := readJSONBody(resp, &result); err != nil {
		t.Fatal(err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result)
	}
}

func TestErrSnapshot(t *testing.T) {
	snap := errSnapshot(ProviderAnthropic, "test error")
	if snap.Provider != ProviderAnthropic {
		t.Error("wrong provider")
	}
	if snap.Error != "test error" {
		t.Errorf("expected 'test error', got %q", snap.Error)
	}
	if snap.DisplayName != "Claude" {
		t.Errorf("expected 'Claude', got %q", snap.DisplayName)
	}
}

func TestOkSnapshot(t *testing.T) {
	windows := []ProviderUsageWindow{
		{Label: "Daily", UsedPercent: 50},
	}
	snap := okSnapshot(ProviderCopilot, windows, "free")
	if snap.Provider != ProviderCopilot {
		t.Error("wrong provider")
	}
	if snap.Plan != "free" {
		t.Errorf("expected plan=free, got %q", snap.Plan)
	}
	if len(snap.Windows) != 1 {
		t.Error("expected 1 window")
	}
}

func TestHttpErr(t *testing.T) {
	got := httpErr(404)
	if got != "HTTP 404" {
		t.Errorf("expected 'HTTP 404', got %q", got)
	}
}

// ---------- LoadProviderUsageSummary ----------

func TestLoadProviderUsageSummary_Empty(t *testing.T) {
	ctx := context.Background()
	summary := LoadProviderUsageSummary(ctx, nil)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if len(summary.Providers) != 0 {
		t.Error("expected 0 providers for nil input")
	}
}

// ---------- downsamplePoints ----------

func TestDownsamplePoints(t *testing.T) {
	// Create 100 points
	points := make([]SessionUsageTimePoint, 100)
	for i := 0; i < 100; i++ {
		points[i] = SessionUsageTimePoint{
			TimestampMs:    int64(i * 1000),
			CumulativeCost: float64(i) * 0.01,
		}
	}

	// Downsample to 10
	result := downsamplePoints(points, 10)
	if len(result) > 10 {
		t.Errorf("expected at most 10 points, got %d", len(result))
	}

	// Should preserve first and last
	if result[0].TimestampMs != 0 {
		t.Error("should preserve first point")
	}
	if result[len(result)-1].TimestampMs != 99000 {
		t.Error("should preserve last point")
	}
}

func TestDownsamplePoints_NoOp(t *testing.T) {
	points := []SessionUsageTimePoint{
		{TimestampMs: 1000},
		{TimestampMs: 2000},
	}
	result := downsamplePoints(points, 100)
	if len(result) != 2 {
		t.Errorf("expected passthrough for small input, got %d", len(result))
	}
}

// ---------- Type JSON Tests ----------

func TestProviderUsageSnapshotJSON(t *testing.T) {
	snap := ProviderUsageSnapshot{
		Provider:    ProviderAnthropic,
		DisplayName: "Claude",
		Plan:        "free",
		Windows: []ProviderUsageWindow{
			{Label: "Daily", UsedPercent: 30.5, ResetAt: nil},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ProviderUsageSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Provider != ProviderAnthropic {
		t.Errorf("expected anthropic, got %v", decoded.Provider)
	}
	if len(decoded.Windows) != 1 {
		t.Error("expected 1 window")
	}
}

func TestSessionCostSummaryJSON(t *testing.T) {
	summary := SessionCostSummary{
		SessionID: "test-session",
		TotalCost: CostBreakdown{
			InputTokens:  1000,
			OutputTokens: 500,
			TotalCostUSD: 0.05,
		},
		MessageTotal: SessionMessageCounts{
			User:      3,
			Assistant: 3,
		},
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test-session") {
		t.Error("expected sessionId in JSON")
	}
}
