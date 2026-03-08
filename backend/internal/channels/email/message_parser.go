package email

// message_parser.go — Phase 4: MIME 解析器
// 输入: RawEmailMessage.Body ([]byte, 完整 RFC822 原文)
// 输出: ParsedEmail (结构化正文 + 附件 + 元数据)
// 处理: RFC 2047 编码标题、quoted-printable/base64、multipart 递归、
//       HTML→安全纯文本、附件白名单/限制、message/rfc822 摘要(F-03)、CID 内嵌图片

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ParsedEmail MIME 解析后的结构化邮件
type ParsedEmail struct {
	MessageID  string
	InReplyTo  string
	References []string
	From       string
	To         []string
	Cc         []string
	Subject    string
	Date       time.Time

	TextBody string // 纯文本正文（优先 text/plain，回退 HTML→text）
	HTMLBody string // 原始 HTML（仅内部用，不暴露给 agent）

	Attachments  []EmailAttachment
	InlineImages []EmailAttachment // CID 内嵌图片

	// 解析元数据
	HasHTML       bool
	ContentType   string
	ParseWarnings []string // 非致命解析警告
}

// EmailAttachment 邮件附件
type EmailAttachment struct {
	Filename    string
	ContentType string
	ContentID   string // CID (内嵌图片用)
	Data        []byte
	Size        int
	Inline      bool // true=内嵌, false=附件
}

// ParseLimits 解析限制参数
type ParseLimits struct {
	MaxAttachmentBytes          int64
	MaxAttachments              int
	AllowAttachmentMimePrefixes []string
	HTMLMode                    types.EmailHTMLMode
}

// DefaultParseLimits 默认解析限制
func DefaultParseLimits() ParseLimits {
	return ParseLimits{
		MaxAttachmentBytes:          10 * 1024 * 1024, // 10MB
		MaxAttachments:              5,
		AllowAttachmentMimePrefixes: []string{"image/", "text/", "application/pdf"},
		HTMLMode:                    types.EmailHTMLSafeText,
	}
}

// ParseEmail 解析完整 RFC822 邮件原文
// recover 守卫防止异常 MIME 导致 panic
func ParseEmail(rawBody []byte, limits ParseLimits) (result *ParsedEmail, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("MIME parse panic: %v", r)
			slog.Warn("email: MIME parse recovered from panic", "error", r)
		}
	}()

	msg, parseErr := mail.ReadMessage(bytes.NewReader(rawBody))
	if parseErr != nil {
		return nil, fmt.Errorf("parse RFC822: %w", parseErr)
	}

	parsed := &ParsedEmail{}

	// 解析 header（RFC 2047 解码由 mail.Header 自动处理）
	parsed.Subject = decodeHeader(msg.Header.Get("Subject"))
	parsed.MessageID = msg.Header.Get("Message-Id")
	parsed.InReplyTo = msg.Header.Get("In-Reply-To")
	parsed.From = decodeHeader(msg.Header.Get("From"))
	parsed.ContentType = msg.Header.Get("Content-Type")

	// References: 空格分隔的 Message-ID 列表
	if refs := msg.Header.Get("References"); refs != "" {
		parsed.References = parseReferences(refs)
	}

	// To / Cc
	if to := msg.Header.Get("To"); to != "" {
		parsed.To = parseAddressList(to)
	}
	if cc := msg.Header.Get("Cc"); cc != "" {
		parsed.Cc = parseAddressList(cc)
	}

	// Date
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if t, err := mail.ParseDate(dateStr); err == nil {
			parsed.Date = t
		}
	}

	// 解析 body
	contentType := msg.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	mediaType, params, _ := mime.ParseMediaType(contentType)
	body, readErr := io.ReadAll(msg.Body)
	if readErr != nil {
		return nil, fmt.Errorf("read body: %w", readErr)
	}

	// 顶层 Content-Transfer-Encoding 解码（非 multipart 时需手动处理）
	if !strings.HasPrefix(mediaType, "multipart/") {
		cte := strings.ToLower(msg.Header.Get("Content-Transfer-Encoding"))
		if decoded, err := decodeTransferEncoding(body, cte); err == nil {
			body = decoded
		}
	}

	parseBody(parsed, mediaType, params, msg.Header, body, &limits)

	// 如果没有 text body 但有 HTML，转换 HTML → text
	if parsed.TextBody == "" && parsed.HTMLBody != "" {
		parsed.TextBody = htmlToSafeText(parsed.HTMLBody, limits.HTMLMode)
	}

	return parsed, nil
}

// parseBody 递归解析 MIME body
func parseBody(parsed *ParsedEmail, mediaType string, params map[string]string, header mail.Header, body []byte, limits *ParseLimits) {
	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		parseMultipart(parsed, mediaType, params, body, limits)

	case mediaType == "text/plain":
		charset := params["charset"]
		decoded := decodeCharset(body, charset)
		if parsed.TextBody == "" {
			parsed.TextBody = decoded
		}

	case mediaType == "text/html":
		charset := params["charset"]
		decoded := decodeCharset(body, charset)
		parsed.HasHTML = true
		if parsed.HTMLBody == "" {
			parsed.HTMLBody = decoded
		}

	case mediaType == "message/rfc822":
		// F-03: 仅生成摘要，不递归解析
		subject := extractRFC822Subject(body)
		summary := fmt.Sprintf("[转发邮件: %s]", subject)
		if parsed.TextBody == "" {
			parsed.TextBody = summary
		} else {
			parsed.TextBody += "\n\n" + summary
		}

	default:
		// 附件或内嵌资源
		handleAttachment(parsed, mediaType, params, header, body, limits)
	}
}

// parseMultipart 解析 multipart MIME 部分
func parseMultipart(parsed *ParsedEmail, mediaType string, params map[string]string, body []byte, limits *ParseLimits) {
	boundary := params["boundary"]
	if boundary == "" {
		parsed.ParseWarnings = append(parsed.ParseWarnings, "multipart without boundary")
		return
	}

	parts := splitMultipart(body, boundary)

	for _, part := range parts {
		partMsg, err := mail.ReadMessage(bytes.NewReader(part))
		if err != nil {
			// 可能是简单 part 没有完整 header
			parsed.ParseWarnings = append(parsed.ParseWarnings, fmt.Sprintf("skip malformed part: %v", err))
			continue
		}

		partCT := partMsg.Header.Get("Content-Type")
		if partCT == "" {
			partCT = "text/plain"
		}
		partMedia, partParams, _ := mime.ParseMediaType(partCT)

		// 读取 part body，处理 transfer encoding
		partBody, err := readPartBody(partMsg)
		if err != nil {
			parsed.ParseWarnings = append(parsed.ParseWarnings, fmt.Sprintf("read part body: %v", err))
			continue
		}

		parseBody(parsed, partMedia, partParams, partMsg.Header, partBody, limits)
	}

	_ = mediaType // used for context
}

// splitMultipart 按 boundary 切分 multipart body
func splitMultipart(body []byte, boundary string) [][]byte {
	delim := []byte("--" + boundary)
	endDelim := []byte("--" + boundary + "--")

	var parts [][]byte
	rest := body

	// 找到第一个 boundary
	idx := bytes.Index(rest, delim)
	if idx < 0 {
		return nil
	}
	rest = rest[idx+len(delim):]

	for {
		// 跳过 boundary 行后的 CRLF/LF
		if len(rest) > 0 && rest[0] == '\r' {
			rest = rest[1:]
		}
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}

		// 是否到达结束 boundary
		if bytes.HasPrefix(rest, []byte("--")) {
			break
		}

		// 找下一个 boundary
		nextIdx := bytes.Index(rest, delim)
		if nextIdx < 0 {
			// 没有更多 boundary，剩余作为最后一个 part
			trimmed := bytes.TrimSuffix(rest, endDelim)
			if len(trimmed) > 0 {
				parts = append(parts, trimmed)
			}
			break
		}

		part := rest[:nextIdx]
		// 去掉 part 末尾的 CRLF
		part = bytes.TrimRight(part, "\r\n")
		parts = append(parts, part)

		rest = rest[nextIdx+len(delim):]
	}

	return parts
}

// readPartBody 读取 MIME part 的 body，处理 Content-Transfer-Encoding
func readPartBody(msg *mail.Message) ([]byte, error) {
	raw, err := io.ReadAll(msg.Body)
	if err != nil {
		return nil, err
	}

	encoding := strings.ToLower(msg.Header.Get("Content-Transfer-Encoding"))
	return decodeTransferEncoding(raw, encoding)
}

// decodeTransferEncoding 解码 content transfer encoding
func decodeTransferEncoding(data []byte, encoding string) ([]byte, error) {
	switch encoding {
	case "base64":
		return decodeBase64(data), nil
	case "quoted-printable":
		return decodeQuotedPrintable(data), nil
	default:
		// 7bit, 8bit, binary — 无需转换
		return data, nil
	}
}

// handleAttachment 处理附件/内嵌资源
func handleAttachment(parsed *ParsedEmail, mediaType string, params map[string]string, header mail.Header, body []byte, limits *ParseLimits) {
	// 检查附件数量限制
	totalAttachments := len(parsed.Attachments) + len(parsed.InlineImages)
	if totalAttachments >= limits.MaxAttachments {
		parsed.ParseWarnings = append(parsed.ParseWarnings,
			fmt.Sprintf("attachment limit reached (%d), skipping %s", limits.MaxAttachments, mediaType))
		return
	}

	// 检查 MIME 白名单
	if !isMimeAllowed(mediaType, limits.AllowAttachmentMimePrefixes) {
		parsed.ParseWarnings = append(parsed.ParseWarnings,
			fmt.Sprintf("attachment MIME %q not in whitelist, skipping", mediaType))
		return
	}

	// 检查大小限制
	if limits.MaxAttachmentBytes > 0 && int64(len(body)) > limits.MaxAttachmentBytes {
		parsed.ParseWarnings = append(parsed.ParseWarnings,
			fmt.Sprintf("attachment too large (%d bytes > %d), skipping", len(body), limits.MaxAttachmentBytes))
		return
	}

	// 解析文件名
	filename := resolveFilename(params, header)

	// 解析 Content-ID (CID)
	contentID := header.Get("Content-Id")
	contentID = strings.Trim(contentID, "<>")

	// 判断是否内嵌
	disposition := header.Get("Content-Disposition")
	isInline := strings.HasPrefix(strings.ToLower(disposition), "inline") || contentID != ""

	att := EmailAttachment{
		Filename:    filename,
		ContentType: mediaType,
		ContentID:   contentID,
		Data:        body,
		Size:        len(body),
		Inline:      isInline,
	}

	if isInline && strings.HasPrefix(mediaType, "image/") {
		parsed.InlineImages = append(parsed.InlineImages, att)
	} else {
		parsed.Attachments = append(parsed.Attachments, att)
	}
}

// resolveFilename 从 Content-Disposition 和 Content-Type 参数解析文件名
func resolveFilename(params map[string]string, header mail.Header) string {
	// 优先 Content-Disposition 的 filename
	disposition := header.Get("Content-Disposition")
	if disposition != "" {
		_, dParams, err := mime.ParseMediaType(disposition)
		if err == nil {
			if name := dParams["filename"]; name != "" {
				return decodeRFC2047(name)
			}
		}
	}

	// 回退到 Content-Type 的 name 参数
	if name := params["name"]; name != "" {
		return decodeRFC2047(name)
	}

	return ""
}

// isMimeAllowed 检查 MIME 类型是否在白名单内
func isMimeAllowed(mediaType string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true // 无白名单 = 全部允许
	}
	lower := strings.ToLower(mediaType)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

// --- RFC 2047 / 字符集解码 ---

// mime.WordDecoder 处理 RFC 2047 编码（=?charset?encoding?text?=）
var wordDecoder = &mime.WordDecoder{
	CharsetReader: charsetReader,
}

// decodeHeader 解码 RFC 2047 编码的 header 值
func decodeHeader(s string) string {
	if s == "" {
		return s
	}
	decoded, err := wordDecoder.DecodeHeader(s)
	if err != nil {
		return s // 解码失败返回原文
	}
	return decoded
}

// decodeRFC2047 解码可能含 RFC 2047 的字符串（如附件名）
func decodeRFC2047(s string) string {
	if !strings.Contains(s, "=?") {
		return s
	}
	return decodeHeader(s)
}

// charsetReader 返回指定字符集的 reader（支持 UTF-8/GBK/GB18030/GB2312/ISO-8859-1/Shift_JIS 等）
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	lower := strings.ToLower(strings.TrimSpace(charset))
	lower = strings.ReplaceAll(lower, "-", "")
	lower = strings.ReplaceAll(lower, "_", "")

	switch lower {
	case "utf8", "utf-8": // noop, already handled above
		return input, nil
	case "gbk", "gb2312", "gb231280", "cp936":
		return simplifiedchinese.GBK.NewDecoder().Reader(input), nil
	case "gb18030":
		return simplifiedchinese.GB18030.NewDecoder().Reader(input), nil
	case "iso88591", "latin1":
		return charmap.ISO8859_1.NewDecoder().Reader(input), nil
	case "iso88592":
		return charmap.ISO8859_2.NewDecoder().Reader(input), nil
	case "iso885915":
		return charmap.ISO8859_15.NewDecoder().Reader(input), nil
	case "windows1252", "cp1252":
		return charmap.Windows1252.NewDecoder().Reader(input), nil
	case "shiftjis", "sjis", "cp932":
		return japanese.ShiftJIS.NewDecoder().Reader(input), nil
	case "big5":
		// big5 暂不单独导入，用 UTF-8 降级
		return input, nil
	case "utf16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder().Reader(input), nil
	case "utf16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Reader(input), nil
	default:
		// 未知字符集，返回原始数据
		return input, nil
	}
}

// decodeCharset 将 bytes 按指定字符集解码为 UTF-8 字符串
func decodeCharset(data []byte, charset string) string {
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
		if utf8.Valid(data) {
			return string(data)
		}
		// 无效 UTF-8，尝试 GBK（中文邮件常见）
		charset = "gbk"
	}

	reader, err := charsetReader(charset, bytes.NewReader(data))
	if err != nil {
		return string(data)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return string(data)
	}
	return string(decoded)
}

// --- base64 / quoted-printable 解码 ---

// decodeBase64 解码 base64 (忽略空白行)
func decodeBase64(data []byte) []byte {
	// 去掉换行和空格
	cleaned := make([]byte, 0, len(data))
	for _, b := range data {
		if b != '\r' && b != '\n' && b != ' ' && b != '\t' {
			cleaned = append(cleaned, b)
		}
	}

	// 标准 base64 解码
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	table := [256]byte{}
	for i := range table {
		table[i] = 0xFF
	}
	for i, c := range base64Chars[:64] {
		table[byte(c)] = byte(i)
	}

	result := make([]byte, 0, len(cleaned)*3/4)
	for i := 0; i+4 <= len(cleaned); i += 4 {
		a, b, c, d := table[cleaned[i]], table[cleaned[i+1]], table[cleaned[i+2]], table[cleaned[i+3]]
		if a == 0xFF || b == 0xFF {
			break
		}
		result = append(result, (a<<2)|(b>>4))
		if cleaned[i+2] != '=' {
			if c == 0xFF {
				break
			}
			result = append(result, (b<<4)|(c>>2))
		}
		if cleaned[i+3] != '=' {
			if d == 0xFF {
				break
			}
			result = append(result, (c<<6)|d)
		}
	}
	return result
}

// decodeQuotedPrintable 解码 quoted-printable
func decodeQuotedPrintable(data []byte) []byte {
	var result []byte
	i := 0
	for i < len(data) {
		if data[i] == '=' {
			if i+2 < len(data) {
				if data[i+1] == '\r' && i+2 < len(data) && data[i+2] == '\n' {
					// soft line break (=\r\n)
					i += 3
					continue
				}
				if data[i+1] == '\n' {
					// soft line break (=\n)
					i += 2
					continue
				}
				// hex encoded byte
				hi := unhex(data[i+1])
				lo := unhex(data[i+2])
				if hi >= 0 && lo >= 0 {
					result = append(result, byte(hi<<4|lo))
					i += 3
					continue
				}
			}
		}
		result = append(result, data[i])
		i++
	}
	return result
}

func unhex(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'A' <= c && c <= 'F':
		return int(c - 'A' + 10)
	case 'a' <= c && c <= 'f':
		return int(c - 'a' + 10)
	default:
		return -1
	}
}

// --- HTML → 安全纯文本 ---

// htmlToSafeText 将 HTML 转为安全纯文本
// safe_text: 去 script/style/tracking pixel，保留链接文本+URL
// strip: 仅去除标签
func htmlToSafeText(htmlStr string, mode types.EmailHTMLMode) string {
	if mode == types.EmailHTMLStrip {
		return stripHTMLTags(htmlStr)
	}
	return safeTextFromHTML(htmlStr)
}

// safeTextFromHTML 安全文本提取（默认模式）
func safeTextFromHTML(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return stripHTMLTags(htmlStr) // 解析失败降级
	}

	var buf strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		// 跳过危险标签
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "object", "embed", "applet", "noscript":
				return
			case "img":
				// tracking pixel 检测：1x1 或无 alt 的隐藏图片 → 跳过
				if isTrackingPixel(n) {
					return
				}
				// 保留 alt 文本
				if alt := getAttr(n, "alt"); alt != "" {
					buf.WriteString("[图片: ")
					buf.WriteString(alt)
					buf.WriteString("] ")
				}
				return
			case "a":
				// 保留链接文本 + URL
				href := getAttr(n, "href")
				text := extractText(n)
				if text != "" {
					buf.WriteString(text)
					if href != "" && href != text && !strings.HasPrefix(href, "cid:") {
						buf.WriteString(" (")
						buf.WriteString(href)
						buf.WriteString(")")
					}
				} else if href != "" && !strings.HasPrefix(href, "cid:") {
					buf.WriteString(href)
				}
				buf.WriteString(" ")
				return
			case "br":
				buf.WriteString("\n")
			case "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
				buf.WriteString("\n")
			case "td", "th":
				buf.WriteString("\t")
			case "hr":
				buf.WriteString("\n---\n")
			}
		}

		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				buf.WriteString(text)
				buf.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}

		// 块级元素结尾加换行
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
				buf.WriteString("\n")
			}
		}
	}

	extract(doc)

	// 清理多余空行
	result := buf.String()
	result = collapseNewlines(result)
	return strings.TrimSpace(result)
}

// isTrackingPixel 检测 1x1 追踪像素
func isTrackingPixel(n *html.Node) bool {
	width := getAttr(n, "width")
	height := getAttr(n, "height")
	if (width == "1" || width == "0") && (height == "1" || height == "0") {
		return true
	}
	style := getAttr(n, "style")
	if strings.Contains(style, "display:none") || strings.Contains(style, "display: none") {
		return true
	}
	if strings.Contains(style, "width:1px") || strings.Contains(style, "width: 1px") ||
		strings.Contains(style, "width:0") || strings.Contains(style, "height:1px") ||
		strings.Contains(style, "height: 1px") || strings.Contains(style, "height:0") {
		return true
	}
	return false
}

// getAttr 获取 HTML 元素属性
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// extractText 递归提取节点文本
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		buf.WriteString(extractText(c))
	}
	return buf.String()
}

// stripHTMLTags 简单去除所有 HTML 标签
func stripHTMLTags(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		// 最终降级: 正则式剥离
		return regexStripTags(s)
	}
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(buf.String())
}

// regexStripTags 简单标签剥离（最终降级）
func regexStripTags(s string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// collapseNewlines 将连续空行折叠为最多两个换行
func collapseNewlines(s string) string {
	var buf strings.Builder
	nlCount := 0
	for _, r := range s {
		if r == '\n' {
			nlCount++
			if nlCount <= 2 {
				buf.WriteRune(r)
			}
		} else {
			nlCount = 0
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// --- 辅助函数 ---

// parseReferences 解析 References header（空格分隔的 Message-ID 列表）
func parseReferences(refs string) []string {
	var result []string
	for _, part := range strings.Fields(refs) {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// parseAddressList 解析邮件地址列表
func parseAddressList(s string) []string {
	addrs, err := mail.ParseAddressList(s)
	if err != nil {
		// 降级: 逗号分隔
		parts := strings.Split(s, ",")
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	result := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a.Name != "" {
			result = append(result, fmt.Sprintf("%s <%s>", a.Name, a.Address))
		} else {
			result = append(result, a.Address)
		}
	}
	return result
}

// extractRFC822Subject 从嵌套 message/rfc822 中提取 Subject（F-03）
func extractRFC822Subject(body []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(body))
	if err != nil {
		return "未知"
	}
	subject := decodeHeader(msg.Header.Get("Subject"))
	if subject == "" {
		return "无主题"
	}
	return subject
}
