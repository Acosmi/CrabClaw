package tools

import (
	"testing"
)

func TestHtmlToReadableMarkdown_ExtractsContent(t *testing.T) {
	html := `<html><head><title>Test Article</title></head><body>
		<nav>Navigation links</nav>
		<article>
			<h1>Main Title</h1>
			<p>This is the main content of the article. It contains enough text for readability to identify it as the main content block of the page.</p>
			<p>Second paragraph with more details about the topic being discussed in this article.</p>
		</article>
		<footer>Footer content</footer>
	</body></html>`

	result := htmlToReadableMarkdown(html, "https://example.com/article")

	if result == "" {
		t.Fatal("expected non-empty result")
	}
	// Should contain the article content (either via readability or fallback)
	if !containsAny(result, "main content", "Main Title") {
		t.Errorf("expected article content in result, got: %s", result[:min(200, len(result))])
	}
}

func TestHtmlToReadableMarkdown_FallbackOnInvalidHTML(t *testing.T) {
	// Empty/invalid HTML should fallback to htmlToSimpleMarkdown
	result := htmlToReadableMarkdown("", "https://example.com")
	// Should not panic, may return empty string
	_ = result

	result2 := htmlToReadableMarkdown("<p>Simple paragraph</p>", "https://example.com")
	if result2 == "" {
		t.Error("expected non-empty result for simple HTML")
	}
}

func TestHtmlToReadableMarkdown_FallbackOnBadURL(t *testing.T) {
	html := "<html><body><p>Content here</p></body></html>"
	// Bad URL should fallback gracefully
	result := htmlToReadableMarkdown(html, "://invalid")
	// url.Parse("://invalid") actually doesn't error in Go, but test the path anyway
	if result == "" {
		t.Error("expected non-empty result even with unusual URL")
	}
}

func TestDefaultWebFetchOptionsUsesCrabClawUserAgent(t *testing.T) {
	opts := DefaultWebFetchOptions()
	if opts.UserAgent != "CrabClaw/1.0 (Web Fetch Tool)" {
		t.Fatalf("user agent = %q, want %q", opts.UserAgent, "CrabClaw/1.0 (Web Fetch Tool)")
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
