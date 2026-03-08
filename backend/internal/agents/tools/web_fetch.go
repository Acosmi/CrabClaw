// tools/web_fetch.go — 网页抓取 + 搜索工具。
// TS 参考：src/agents/tools/web-fetch.ts (688L) + web-search.ts (690L) +
//
//	web-fetch-utils.ts (122L) + web-shared.ts (95L)
package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
)

// ---------- C-4：内存缓存层 ----------
// 对应 TS 的 FETCH_CACHE / SEARCH_CACHE 模块级 Map 单例。

type webCacheEntry struct {
	result   string
	cachedAt time.Time
	ttl      time.Duration
}

var (
	// webFetchCache 对应 TS 的 FETCH_CACHE
	webFetchCache   = make(map[string]webCacheEntry)
	webFetchCacheMu sync.RWMutex

	// webSearchCache 对应 TS 的 SEARCH_CACHE
	webSearchCache   = make(map[string]webCacheEntry)
	webSearchCacheMu sync.RWMutex
)

// getWebFetchCached 从 web_fetch 缓存中读取条目。
func getWebFetchCached(key string) (string, bool) {
	webFetchCacheMu.RLock()
	entry, ok := webFetchCache[key]
	webFetchCacheMu.RUnlock()
	if !ok {
		return "", false
	}
	if entry.ttl > 0 && time.Since(entry.cachedAt) > entry.ttl {
		webFetchCacheMu.Lock()
		delete(webFetchCache, key)
		webFetchCacheMu.Unlock()
		return "", false
	}
	return entry.result, true
}

// setWebFetchCache 写入 web_fetch 缓存条目。
func setWebFetchCache(key, value string, ttl time.Duration) {
	webFetchCacheMu.Lock()
	webFetchCache[key] = webCacheEntry{result: value, cachedAt: time.Now(), ttl: ttl}
	webFetchCacheMu.Unlock()
}

// getWebSearchCached 从 web_search 缓存中读取条目。
func getWebSearchCached(key string) (string, bool) {
	webSearchCacheMu.RLock()
	entry, ok := webSearchCache[key]
	webSearchCacheMu.RUnlock()
	if !ok {
		return "", false
	}
	if entry.ttl > 0 && time.Since(entry.cachedAt) > entry.ttl {
		webSearchCacheMu.Lock()
		delete(webSearchCache, key)
		webSearchCacheMu.Unlock()
		return "", false
	}
	return entry.result, true
}

// setWebSearchCache 写入 web_search 缓存条目。
func setWebSearchCache(key, value string, ttl time.Duration) {
	webSearchCacheMu.Lock()
	webSearchCache[key] = webCacheEntry{result: value, cachedAt: time.Now(), ttl: ttl}
	webSearchCacheMu.Unlock()
}

// webFetchCacheKey 构造 web_fetch 缓存键。
func webFetchCacheKey(rawURL, extractMode string, maxBytes int64) string {
	return fmt.Sprintf("fetch:%s:%s:%d", rawURL, extractMode, maxBytes)
}

// webSearchCacheKey 构造 web_search 缓存键。
func webSearchCacheKey(query, country, searchLang, uiLang, freshness string, count int) string {
	return fmt.Sprintf("search:%s:%s:%s:%s:%s:%d", query, country, searchLang, uiLang, freshness, count)
}

// ---------- SSRF 保护 ----------

// isPrivateIP 检查 IP 是否为私有地址（SSRF 防护）。
// TS 参考: web-fetch.ts SSRF protection
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		start net.IP
		end   net.IP
	}{
		{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")},
		{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")},
		{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")},
		{net.ParseIP("127.0.0.0"), net.ParseIP("127.255.255.255")},
		{net.ParseIP("169.254.0.0"), net.ParseIP("169.254.255.255")},
	}

	for _, r := range privateRanges {
		if bytesInRange(ip.To4(), r.start.To4(), r.end.To4()) {
			return true
		}
	}
	return false
}

func bytesInRange(ip, start, end net.IP) bool {
	if ip == nil || start == nil || end == nil {
		return false
	}
	for i := 0; i < len(ip) && i < len(start) && i < len(end); i++ {
		if ip[i] < start[i] {
			return false
		}
		if ip[i] > end[i] {
			return false
		}
	}
	return true
}

// validateURL 验证 URL 是否安全。
func validateURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// 仅允许 http 和 https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}

	// DNS 解析检查 SSRF
	host := parsed.Hostname()
	addrs, err := net.LookupHost(host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", host, err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && isPrivateIP(ip) {
			return nil, fmt.Errorf("SSRF blocked: %s resolves to private IP %s", host, addr)
		}
	}

	return parsed, nil
}

// ---------- Web Fetch ----------

// WebFetchOptions 网页抓取选项。
type WebFetchOptions struct {
	MaxResponseBytes int64
	Timeout          time.Duration
	UserAgent        string
	CacheTTL         time.Duration
}

// DefaultWebFetchOptions 默认选项。
func DefaultWebFetchOptions() WebFetchOptions {
	return WebFetchOptions{
		MaxResponseBytes: 5 * 1024 * 1024, // 5MB
		Timeout:          30 * time.Second,
		UserAgent:        "CrabClaw/1.0 (Web Fetch Tool)",
		CacheTTL:         15 * time.Minute,
	}
}

// CreateWebFetchTool 创建网页抓取工具。
// TS 参考: web-fetch.ts
// C-3：新增 extractMode 参数（markdown/text/raw）
// C-4：执行前检查缓存，执行后写入缓存
func CreateWebFetchTool(opts WebFetchOptions) *AgentTool {
	if opts.MaxResponseBytes <= 0 {
		opts = DefaultWebFetchOptions()
	}
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 15 * time.Minute
	}

	return &AgentTool{
		Name:        "web_fetch",
		Label:       "Web Fetch",
		Description: "Fetch and extract readable content from a URL (HTML → markdown/text). Use for lightweight page access without browser automation.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "HTTP or HTTPS URL to fetch.",
				},
				// C-3：extractMode 参数（与 TS WebFetchSchema 对齐）
				"extractMode": map[string]any{
					"type":        "string",
					"enum":        []any{"markdown", "text", "raw"},
					"description": `Extraction mode: "markdown" (default) converts HTML to markdown, "text" strips all tags, "raw" returns the original HTML/content.`,
				},
				"maxChars": map[string]any{
					"type":        "number",
					"description": "Maximum characters to return (truncates when exceeded).",
					"minimum":     100,
				},
				"include_links": map[string]any{
					"type":        "boolean",
					"description": "Whether to include discovered links (default: false)",
				},
			},
			"required": []any{"url"},
		},
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (*AgentToolResult, error) {
			rawURL, err := ReadStringParam(args, "url", &StringParamOptions{Required: true})
			if err != nil {
				return nil, err
			}

			// C-3：读取 extractMode（默认 markdown）
			extractModeRaw, _ := ReadStringParam(args, "extractMode", nil)
			extractMode := "markdown"
			switch extractModeRaw {
			case "text", "raw":
				extractMode = extractModeRaw
			}

			// C-4：检查缓存
			cacheKey := webFetchCacheKey(rawURL, extractMode, opts.MaxResponseBytes)
			if cached, ok := getWebFetchCached(cacheKey); ok {
				return JsonResult(map[string]any{
					"url":         rawURL,
					"cached":      true,
					"extractMode": extractMode,
					"content":     cached,
				}), nil
			}

			_, err = validateURL(rawURL)
			if err != nil {
				return nil, err
			}

			ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
			if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
			}
			req.Header.Set("User-Agent", opts.UserAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,*/*")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("fetch failed: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(io.LimitReader(resp.Body, opts.MaxResponseBytes))
			if err != nil {
				return nil, fmt.Errorf("read response: %w", err)
			}

			rawBody := string(body)
			contentType := resp.Header.Get("Content-Type")

			// C-3：根据 extractMode 选择处理方式
			var content string
			switch extractMode {
			case "raw":
				// 直接返回原始 HTML/内容
				content = rawBody
			case "text":
				// 剥离所有 HTML 标签
				if strings.Contains(contentType, "text/html") {
					content = stripHTMLTags(rawBody)
				} else {
					content = rawBody
				}
			default: // "markdown"
				// 转换为 markdown — 优先 go-readability 正文提取，fallback 简单转换
				if strings.Contains(contentType, "text/html") {
					content = htmlToReadableMarkdown(rawBody, rawURL)
				} else {
					content = rawBody
				}
			}

			truncated := truncateString(content, 50000)

			// C-4：写入缓存
			setWebFetchCache(cacheKey, truncated, opts.CacheTTL)

			return JsonResult(map[string]any{
				"url":         rawURL,
				"status":      resp.StatusCode,
				"contentType": contentType,
				"extractMode": extractMode,
				"content":     truncated,
				"length":      len(body),
				"cached":      false,
			}), nil
		},
	}
}

// ---------- Web Search ----------

// WebSearchProvider 网页搜索 provider 接口。
type WebSearchProvider interface {
	Search(ctx context.Context, query string, maxResults int) ([]WebSearchResult, error)
}

// WebSearchResult 搜索结果。
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// WebSearchOptions web_search 工具选项。
type WebSearchOptions struct {
	CacheTTL time.Duration
}

// CreateWebSearchTool 创建网页搜索工具。
// TS 参考: web-search.ts — WebSearchSchema
// C-2：参数名对齐（max_results→count），新增 country/search_lang/ui_lang/freshness
// C-4：执行前检查缓存，执行后写入缓存
func CreateWebSearchTool(provider WebSearchProvider) *AgentTool {
	return CreateWebSearchToolWithOptions(provider, WebSearchOptions{CacheTTL: 15 * time.Minute})
}

// CreateWebSearchToolWithOptions 带选项的 web_search 工具构造函数。
func CreateWebSearchToolWithOptions(provider WebSearchProvider, opts WebSearchOptions) *AgentTool {
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 15 * time.Minute
	}
	return &AgentTool{
		Name:        "web_search",
		Label:       "Web Search",
		Description: "Search the web using a search engine. Returns a list of relevant results.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				// C-2：参数名与 TS WebSearchSchema 完全对齐
				"query": map[string]any{
					"type":        "string",
					"description": "Search query string.",
				},
				// C-2：max_results → count
				"count": map[string]any{
					"type":        "number",
					"description": "Number of results to return (1-10, default: 5).",
					"minimum":     1,
					"maximum":     10,
				},
				// C-2：新增参数
				"country": map[string]any{
					"type":        "string",
					"description": "2-letter country code for region-specific results (e.g. 'DE', 'US', 'ALL'). Default: 'US'.",
				},
				"search_lang": map[string]any{
					"type":        "string",
					"description": "ISO language code for search results (e.g. 'de', 'en', 'fr').",
				},
				"ui_lang": map[string]any{
					"type":        "string",
					"description": "ISO language code for UI elements.",
				},
				"freshness": map[string]any{
					"type":        "string",
					"description": "Filter results by discovery time. Values: 'pd' (past 24h), 'pw' (past week), 'pm' (past month), 'py' (past year), or date range 'YYYY-MM-DDtoYYYY-MM-DD'.",
				},
			},
			"required": []any{"query"},
		},
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (*AgentToolResult, error) {
			query, err := ReadStringParam(args, "query", &StringParamOptions{Required: true})
			if err != nil {
				return nil, err
			}

			// C-2：读取 count（原 max_results）
			count := 5
			if n, ok, _ := ReadNumberParam(args, "count", &NumberParamOptions{Integer: true}); ok && n > 0 {
				count = int(n)
				if count > 10 {
					count = 10
				}
			}

			// C-2：读取新增参数
			country, _ := ReadStringParam(args, "country", nil)
			searchLang, _ := ReadStringParam(args, "search_lang", nil)
			uiLang, _ := ReadStringParam(args, "ui_lang", nil)
			freshness, _ := ReadStringParam(args, "freshness", nil)

			// C-4：检查缓存
			cacheKey := webSearchCacheKey(query, country, searchLang, uiLang, freshness, count)
			if cached, ok := getWebSearchCached(cacheKey); ok {
				return JsonResult(map[string]any{
					"query":   query,
					"cached":  true,
					"content": cached,
				}), nil
			}

			if provider == nil {
				return nil, fmt.Errorf("web search provider not configured")
			}

			results, err := provider.Search(context.Background(), query, count)
			if err != nil {
				return nil, fmt.Errorf("web search failed: %w", err)
			}

			payload := map[string]any{
				"query":   query,
				"results": results,
				"count":   len(results),
				"cached":  false,
			}

			// 附加过滤参数（供调用方了解本次使用的参数）
			if country != "" {
				payload["country"] = country
			}
			if searchLang != "" {
				payload["search_lang"] = searchLang
			}
			if uiLang != "" {
				payload["ui_lang"] = uiLang
			}
			if freshness != "" {
				payload["freshness"] = freshness
			}

			// C-4：写入缓存（序列化结果摘要）
			summaryLines := make([]string, 0, len(results))
			for i, r := range results {
				summaryLines = append(summaryLines, fmt.Sprintf("%d. %s\n   %s\n   %s", i+1, r.Title, r.URL, r.Snippet))
			}
			setWebSearchCache(cacheKey, strings.Join(summaryLines, "\n\n"), opts.CacheTTL)

			return JsonResult(payload), nil
		},
	}
}

// ---------- 辅助函数 ----------

// stripHTMLTags 简单的 HTML 标签剥离。
func stripHTMLTags(html string) string {
	var b strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			b.WriteByte(' ')
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	// 合并连续空白
	result := b.String()
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return strings.TrimSpace(result)
}

// htmlToReadableMarkdown 使用 go-readability 提取正文并转为简化 Markdown。
// 对应 TS: web-fetch-utils.ts extractReadableContent() —
// @mozilla/readability + linkedom 提取正文 → htmlToMarkdown。
// 失败时 fallback 到 htmlToSimpleMarkdown。
func htmlToReadableMarkdown(rawHTML, pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return htmlToSimpleMarkdown(rawHTML)
	}
	article, err := readability.FromReader(strings.NewReader(rawHTML), u)
	if err != nil || article.Node == nil {
		return htmlToSimpleMarkdown(rawHTML)
	}
	// 将清洗后的 HTML 渲染为字符串，再转 markdown
	var buf strings.Builder
	if err := article.RenderHTML(&buf); err != nil || buf.Len() == 0 {
		return htmlToSimpleMarkdown(rawHTML)
	}
	md := htmlToSimpleMarkdown(buf.String())
	if md == "" {
		return htmlToSimpleMarkdown(rawHTML)
	}
	if title := article.Title(); title != "" {
		md = "# " + title + "\n\n" + md
	}
	return md
}

// htmlToSimpleMarkdown 将 HTML 转换为简化的 Markdown。
// C-3：extractMode=markdown 时使用。
// 作为 htmlToReadableMarkdown 的 fallback。
func htmlToSimpleMarkdown(html string) string {
	// 移除 <script> 和 <style> 块
	content := removeHTMLBlock(html, "script")
	content = removeHTMLBlock(content, "style")

	// 标题转换
	for i := 6; i >= 1; i-- {
		prefix := strings.Repeat("#", i)
		open := fmt.Sprintf("<h%d", i)
		close := fmt.Sprintf("</h%d>", i)
		content = replaceHTMLTag(content, open, close, prefix+" ", "\n\n")
	}

	// 段落和换行
	content = strings.ReplaceAll(content, "</p>", "\n\n")
	content = strings.ReplaceAll(content, "<br>", "\n")
	content = strings.ReplaceAll(content, "<br/>", "\n")
	content = strings.ReplaceAll(content, "<br />", "\n")
	content = strings.ReplaceAll(content, "</div>", "\n")
	content = strings.ReplaceAll(content, "</li>", "\n")
	content = strings.ReplaceAll(content, "<li", "- <li")

	// 粗体和斜体
	content = replaceHTMLTag(content, "<strong", "</strong>", "**", "**")
	content = replaceHTMLTag(content, "<b", "</b>", "**", "**")
	content = replaceHTMLTag(content, "<em", "</em>", "*", "*")
	content = replaceHTMLTag(content, "<i", "</i>", "*", "*")

	// 剥离剩余标签
	content = stripHTMLTags(content)

	// 合并多余空行
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(content)
}

// removeHTMLBlock 移除指定标签的完整块（含内容）。
func removeHTMLBlock(html, tag string) string {
	lower := strings.ToLower(html)
	open := "<" + tag
	close := "</" + tag + ">"
	var result strings.Builder
	pos := 0
	for {
		start := strings.Index(lower[pos:], open)
		if start == -1 {
			result.WriteString(html[pos:])
			break
		}
		result.WriteString(html[pos : pos+start])
		end := strings.Index(lower[pos+start:], close)
		if end == -1 {
			// 未找到关闭标签，剩余全部丢弃
			break
		}
		pos = pos + start + end + len(close)
	}
	return result.String()
}

// replaceHTMLTag 将 HTML 标签替换为 Markdown 标记（简单前缀/后缀模式）。
func replaceHTMLTag(html, openPrefix, closeTag, mdPrefix, mdSuffix string) string {
	lower := strings.ToLower(html)
	var result strings.Builder
	pos := 0
	for {
		start := strings.Index(lower[pos:], strings.ToLower(openPrefix))
		if start == -1 {
			result.WriteString(html[pos:])
			break
		}
		result.WriteString(html[pos : pos+start])
		// 找到开标签结束的 >
		tagEnd := strings.Index(html[pos+start:], ">")
		if tagEnd == -1 {
			result.WriteString(html[pos+start:])
			break
		}
		// 写入 MD 前缀，跳过开标签
		result.WriteString(mdPrefix)
		pos = pos + start + tagEnd + 1

		// 找到关闭标签
		closeStart := strings.Index(strings.ToLower(html[pos:]), strings.ToLower(closeTag))
		if closeStart == -1 {
			result.WriteString(html[pos:])
			break
		}
		result.WriteString(html[pos : pos+closeStart])
		result.WriteString(mdSuffix)
		pos = pos + closeStart + len(closeTag)
	}
	return result.String()
}

// truncateString 截断字符串到最大长度。
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n...[truncated]"
}
