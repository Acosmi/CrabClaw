package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// CDPClient provides high-level CDP commands over a WebSocket connection.
// Ported from cdp.ts (454L).
type CDPClient struct {
	mu      sync.Mutex
	wsURL   string
	headers map[string]string
	logger  *slog.Logger
}

// NewCDPClient creates a CDPClient for the given WebSocket URL.
func NewCDPClient(wsURL string, logger *slog.Logger) *CDPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &CDPClient{
		wsURL:  wsURL,
		logger: logger,
	}
}

// NormalizeCdpWsURL normalises a CDP WebSocket URL by aligning its hostname,
// port, protocol, credentials, and query parameters with the given CDP HTTP
// endpoint URL. Aligns with TS cdp.ts normalizeCdpWsUrl().
//
// Rules applied:
//  1. If wsURL host is loopback but cdpURL host is NOT → remap host/port/proto.
//  2. If cdpURL is HTTPS but wsURL is ws → upgrade to wss.
//  3. If wsURL lacks credentials but cdpURL has them → copy over.
//  4. Merge cdpURL query params into wsURL (wsURL params take precedence).
func NormalizeCdpWsURL(wsRaw, cdpRaw string) string {
	wsURL, err := url.Parse(strings.TrimSpace(wsRaw))
	if err != nil {
		return wsRaw
	}
	cdpURL, err := url.Parse(strings.TrimSpace(cdpRaw))
	if err != nil {
		return wsRaw
	}

	wsHost := wsURL.Hostname()
	cdpHost := cdpURL.Hostname()

	// 1. Loopback remapping: if ws is loopback but cdp is remote, adopt cdp's host/port/proto.
	if IsLoopbackHost(wsHost) && !IsLoopbackHost(cdpHost) {
		wsURL.Host = cdpHost
		cdpPort := cdpURL.Port()
		if cdpPort == "" {
			if cdpURL.Scheme == "https" {
				cdpPort = "443"
			} else {
				cdpPort = "80"
			}
		}
		wsURL.Host = cdpHost + ":" + cdpPort

		if cdpURL.Scheme == "https" {
			wsURL.Scheme = "wss"
		} else {
			wsURL.Scheme = "ws"
		}
	}

	// 2. Protocol alignment: https cdp → wss ws.
	if cdpURL.Scheme == "https" && wsURL.Scheme == "ws" {
		wsURL.Scheme = "wss"
	}

	// 3. Credentials inheritance.
	if wsURL.User == nil && cdpURL.User != nil {
		wsURL.User = cdpURL.User
	}

	// 4. Merge query parameters (wsURL params take precedence).
	wsQ := wsURL.Query()
	cdpQ := cdpURL.Query()
	for k, vals := range cdpQ {
		if _, exists := wsQ[k]; !exists {
			wsQ[k] = vals
		}
	}
	wsURL.RawQuery = wsQ.Encode()

	return wsURL.String()
}

// CdpURLForPort returns the HTTP CDP endpoint for a given port on localhost.
func CdpURLForPort(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

// CDPTarget represents a Chrome target (page/tab).
type CDPTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// ListTargets fetches the list of Chrome targets via the /json endpoint.
func (c *CDPClient) ListTargets(ctx context.Context) ([]CDPTarget, error) {
	httpBase := wsToHTTP(c.wsURL)
	endpoint, err := AppendCdpPath(httpBase, "/json")
	if err != nil {
		return nil, err
	}

	var targets []CDPTarget
	if err := FetchJSON(ctx, endpoint, &targets, DefaultFetchTimeoutMs); err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	return targets, nil
}

// Navigate sends a Page.navigate command.
func (c *CDPClient) Navigate(ctx context.Context, targetURL string) error {
	return WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		_, err := send("Page.navigate", map[string]any{"url": targetURL})
		return err
	})
}

// Evaluate executes JavaScript in the page context.
func (c *CDPClient) Evaluate(ctx context.Context, expression string) (json.RawMessage, error) {
	var result json.RawMessage
	err := WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    expression,
			"returnByValue": true,
		})
		result = raw
		return err
	})
	return result, err
}

// GetVersion fetches the browser version via /json/version.
func (c *CDPClient) GetVersion(ctx context.Context) (map[string]string, error) {
	httpBase := wsToHTTP(c.wsURL)
	endpoint, err := AppendCdpPath(httpBase, "/json/version")
	if err != nil {
		return nil, err
	}

	var version map[string]string
	if err := FetchJSON(ctx, endpoint, &version, DefaultFetchTimeoutMs); err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	return version, nil
}

// CaptureScreenshot captures a PNG screenshot of the current page.
func (c *CDPClient) CaptureScreenshot(ctx context.Context) ([]byte, error) {
	var screenshot []byte
	err := WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		raw, err := send("Page.captureScreenshot", map[string]any{
			"format": "png",
		})
		if err != nil {
			return err
		}
		var resp struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		// Data is base64 encoded.
		screenshot = []byte(resp.Data)
		return nil
	})
	return screenshot, err
}

// HealthCheck probes the CDP endpoint for availability.
func (c *CDPClient) HealthCheck(ctx context.Context) error {
	httpURL := wsToHTTP(c.wsURL)
	endpoint, err := AppendCdpPath(httpURL, "/json/version")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("health check: HTTP %d", resp.StatusCode)
	}
	return nil
}

// CreateTarget creates a new browser tab/target via CDP Target.createTarget.
// Aligns with TS cdp.ts createTargetViaCdp().
func (c *CDPClient) CreateTarget(ctx context.Context, targetURL string) (string, error) {
	var targetID string
	err := WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		raw, err := send("Target.createTarget", map[string]any{"url": targetURL})
		if err != nil {
			return fmt.Errorf("Target.createTarget: %w", err)
		}
		var resp struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("Target.createTarget unmarshal: %w", err)
		}
		if strings.TrimSpace(resp.TargetID) == "" {
			return fmt.Errorf("Target.createTarget returned empty targetId")
		}
		targetID = resp.TargetID
		return nil
	})
	return targetID, err
}

// ResolveFirstPageWsURL fetches /json from the CDP HTTP endpoint derived from
// a browser-level WebSocket URL, and returns the first "page" target's WS URL.
// Returns empty string on failure. Used to convert browser-level WS URLs
// (which don't support Page/Runtime domains) to page-level WS URLs.
func ResolveFirstPageWsURL(browserWsURL string) string {
	httpBase := wsToHTTP(browserWsURL)
	if idx := strings.Index(httpBase, "/devtools/"); idx >= 0 {
		httpBase = httpBase[:idx]
	}
	endpoint, err := AppendCdpPath(httpBase, "/json")
	if err != nil {
		return ""
	}
	var targets []CDPTarget
	if err := FetchJSON(context.Background(), endpoint, &targets, 3000); err != nil {
		return ""
	}
	for _, t := range targets {
		if t.Type == "page" && t.WebSocketDebuggerURL != "" {
			return t.WebSocketDebuggerURL
		}
	}
	return ""
}

// IsBrowserWsURL returns true if the URL points to a browser-level target
// (/devtools/browser/...) rather than a page-level target.
func IsBrowserWsURL(wsURL string) bool {
	return strings.Contains(wsURL, "/devtools/browser/")
}

// GetWebSocketDebuggerURL fetches the browser-level WebSocket URL from /json/version.
// Aligns with TS cdp.ts getChromeWebSocketUrl() partial logic.
func (c *CDPClient) GetWebSocketDebuggerURL(ctx context.Context) (string, error) {
	version, err := c.GetVersion(ctx)
	if err != nil {
		return "", err
	}
	wsURL := strings.TrimSpace(version["webSocketDebuggerUrl"])
	if wsURL == "" {
		return "", fmt.Errorf("CDP /json/version missing webSocketDebuggerUrl")
	}
	return wsURL, nil
}

// DomSnapshotNode represents a single DOM element in the snapshot tree.
type DomSnapshotNode struct {
	Ref       string `json:"ref"`
	ParentRef string `json:"parentRef,omitempty"`
	Depth     int    `json:"depth"`
	Tag       string `json:"tag"`
	ID        string `json:"id,omitempty"`
	ClassName string `json:"className,omitempty"`
	Role      string `json:"role,omitempty"`
	Name      string `json:"name,omitempty"`
	Text      string `json:"text,omitempty"`
	Href      string `json:"href,omitempty"`
	Type      string `json:"type,omitempty"`
	Value     string `json:"value,omitempty"`
}

// SnapshotDom captures the DOM tree structure via JavaScript execution.
// Aligns with TS cdp.ts snapshotDom().
func (c *CDPClient) SnapshotDom(ctx context.Context, limit, maxTextChars int) ([]DomSnapshotNode, error) {
	if limit <= 0 {
		limit = 800
	}
	if limit > 5000 {
		limit = 5000
	}
	if maxTextChars <= 0 {
		maxTextChars = 220
	}
	if maxTextChars > 5000 {
		maxTextChars = 5000
	}

	// Build JS that traverses DOM depth-first, collecting element info.
	expr := fmt.Sprintf(`(function(){
  var maxN=%d, maxT=%d, nodes=[], stack=[{el:document.documentElement,depth:0,parentRef:null}];
  while(stack.length && nodes.length<maxN){
    var item=stack.pop(), el=item.el;
    if(!el||el.nodeType!==1) continue;
    var ref="n"+(nodes.length+1);
    var tag=(el.tagName||"").toLowerCase();
    var node={ref:ref,parentRef:item.parentRef,depth:item.depth,tag:tag};
    if(el.id) node.id=el.id;
    var cn=typeof el.className==="string"?el.className:"";
    if(cn) node.className=cn.slice(0,300);
    var role=el.getAttribute&&el.getAttribute("role");
    if(role) node.role=role;
    var ariaLabel=el.getAttribute&&el.getAttribute("aria-label");
    if(ariaLabel) node.name=ariaLabel;
    try{var t=(el.innerText||"").trim(); if(t){node.text=t.length>maxT?t.slice(0,maxT)+"…":t;}}catch(e){}
    if(el.href) node.href=String(el.href);
    if(el.type) node.type=String(el.type);
    if(el.value!==undefined&&el.value!=="") node.value=String(el.value).slice(0,500);
    nodes.push(node);
    var children=el.children;
    for(var i=children.length-1;i>=0;i--) stack.push({el:children[i],depth:item.depth+1,parentRef:ref});
  }
  return JSON.stringify(nodes);
})()`, limit, maxTextChars)

	var result []DomSnapshotNode
	err := WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			return err
		}
		var resp struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		// The value is a JSON string.
		var s string
		if err := json.Unmarshal(resp.Result.Value, &s); err != nil {
			// Try direct array parse.
			return json.Unmarshal(resp.Result.Value, &result)
		}
		return json.Unmarshal([]byte(s), &result)
	})
	return result, err
}

// GetDomText extracts text or HTML content from the page.
// Aligns with TS cdp.ts getDomText().
func (c *CDPClient) GetDomText(ctx context.Context, format string, maxChars int, selector string) (string, error) {
	if format != "html" && format != "text" {
		format = "text"
	}
	if maxChars <= 0 {
		maxChars = 200000
	}
	if maxChars > 5000000 {
		maxChars = 5000000
	}

	// Build the selector part.
	selectorJS := "null"
	if selector != "" {
		selectorJS = fmt.Sprintf("document.querySelector(%s)", jsonStringLiteral(selector))
	}

	var extractJS string
	if format == "text" {
		extractJS = fmt.Sprintf(`(function(){
  var target=%s;
  var el=target||document.body||document.documentElement;
  var out="";
  try{out=String(el.innerText||"");}catch(e){}
  if(%d>0&&out.length>%d) out=out.slice(0,%d)+"\n<!-- …truncated… -->";
  return out;
})()`, selectorJS, maxChars, maxChars, maxChars)
	} else {
		extractJS = fmt.Sprintf(`(function(){
  var target=%s;
  var el=target||document.documentElement;
  var out="";
  try{out=String(el.outerHTML||"");}catch(e){}
  if(%d>0&&out.length>%d) out=out.slice(0,%d)+"\n<!-- …truncated… -->";
  return out;
})()`, selectorJS, maxChars, maxChars, maxChars)
	}

	var text string
	err := WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    extractJS,
			"returnByValue": true,
		})
		if err != nil {
			return err
		}
		var resp struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		if err := json.Unmarshal(resp.Result.Value, &text); err != nil {
			text = string(resp.Result.Value)
		}
		return nil
	})
	return text, err
}

// QueryMatch represents a single CSS selector match result.
type QueryMatch struct {
	Index     int    `json:"index"`
	Tag       string `json:"tag"`
	ID        string `json:"id,omitempty"`
	ClassName string `json:"className,omitempty"`
	Text      string `json:"text,omitempty"`
	Value     string `json:"value,omitempty"`
	Href      string `json:"href,omitempty"`
	OuterHTML string `json:"outerHTML,omitempty"`
}

// QuerySelector finds elements matching a CSS selector via Runtime.evaluate.
// Aligns with TS cdp.ts querySelector().
func (c *CDPClient) QuerySelector(ctx context.Context, selector string, limit, maxTextChars, maxHtmlChars int) ([]QueryMatch, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if maxTextChars <= 0 {
		maxTextChars = 500
	}
	if maxTextChars > 5000 {
		maxTextChars = 5000
	}
	if maxHtmlChars <= 0 {
		maxHtmlChars = 1500
	}
	if maxHtmlChars > 20000 {
		maxHtmlChars = 20000
	}

	expr := fmt.Sprintf(`(function(){
  var lim=%d, maxT=%d, maxH=%d;
  var els=Array.from(document.querySelectorAll(%s)).slice(0,lim);
  return JSON.stringify(els.map(function(el,i){
    var m={index:i+1,tag:(el.tagName||"").toLowerCase()};
    if(el.id) m.id=el.id;
    var cn=typeof el.className==="string"?el.className:"";
    if(cn) m.className=cn.slice(0,300);
    try{var t=(el.innerText||"").trim();if(t){m.text=t.length>maxT?t.slice(0,maxT)+"…":t;}}catch(e){}
    if(el.value!==undefined&&el.value!=="") m.value=String(el.value).slice(0,500);
    if(el.href) m.href=String(el.href);
    try{var h=el.outerHTML||"";if(h){m.outerHTML=h.length>maxH?h.slice(0,maxH)+"…":h;}}catch(e){}
    return m;
  }));
})()`, limit, maxTextChars, maxHtmlChars, jsonStringLiteral(selector))

	var matches []QueryMatch
	err := WithCdpSocket(ctx, c.wsURL, func(send CdpSendFn) error {
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			return err
		}
		var resp struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		var s string
		if err := json.Unmarshal(resp.Result.Value, &s); err != nil {
			return json.Unmarshal(resp.Result.Value, &matches)
		}
		return json.Unmarshal([]byte(s), &matches)
	})
	return matches, err
}

// wsToHTTP converts a ws:// or wss:// URL to http:// or https://.
func wsToHTTP(wsURL string) string {
	if strings.HasPrefix(wsURL, "ws://") {
		return "http://" + wsURL[5:]
	}
	if strings.HasPrefix(wsURL, "wss://") {
		return "https://" + wsURL[6:]
	}
	return wsURL
}
