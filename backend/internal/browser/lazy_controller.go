// lazy_controller.go — Lazy-init BrowserController wrapper.
// Defers Chrome discovery/launch to first browser tool call instead of gateway startup.
package browser

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// LazyBrowserController implements BrowserController with lazy initialization.
// Chrome is discovered/launched only when the first browser action is invoked,
// avoiding unnecessary Chrome processes when browser tools are not used.
type LazyBrowserController struct {
	mu       sync.Mutex
	inner    *PlaywrightBrowserController
	instance *ChromeInstance

	resolve    func(ctx context.Context) (PlaywrightTools, string, *ChromeInstance, error)
	onLaunched func(*ChromeInstance)
	logger     *slog.Logger
}

// LazyBrowserControllerConfig configures lazy browser controller initialization.
type LazyBrowserControllerConfig struct {
	// Resolve is called on first use to discover/launch Chrome and create tools.
	Resolve    func(ctx context.Context) (tools PlaywrightTools, cdpURL string, instance *ChromeInstance, err error)
	OnLaunched func(instance *ChromeInstance) // called when a new Chrome instance is launched
	Logger     *slog.Logger
}

// NewLazyBrowserController creates a BrowserController that defers Chrome
// discovery/launch to first use.
func NewLazyBrowserController(cfg LazyBrowserControllerConfig) *LazyBrowserController {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &LazyBrowserController{
		resolve:    cfg.Resolve,
		onLaunched: cfg.OnLaunched,
		logger:     cfg.Logger,
	}
}

// getOrInit returns the inner controller, initializing it on first call.
func (l *LazyBrowserController) getOrInit(ctx context.Context) (*PlaywrightBrowserController, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.inner != nil {
		return l.inner, nil
	}

	l.logger.Info("browser: lazy init — discovering/launching Chrome on first use")
	tools, cdpURL, instance, err := l.resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser not available: %w", err)
	}

	if instance != nil {
		l.instance = instance
		if l.onLaunched != nil {
			l.onLaunched(instance)
		}
	}

	l.inner = NewPlaywrightBrowserController(tools, cdpURL)
	l.logger.Info("browser: lazy init complete", "cdpURL", cdpURL)
	return l.inner, nil
}

// resetOnConnErr clears the cached controller if the error indicates a broken connection,
// allowing re-discovery/re-launch on the next call.
func (l *LazyBrowserController) resetOnConnErr(err error) {
	if err == nil {
		return
	}
	s := err.Error()
	if strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "i/o timeout") {
		l.mu.Lock()
		l.inner = nil
		l.mu.Unlock()
		l.logger.Info("browser: connection lost, will re-init on next use")
	}
}

// ---------- BrowserController interface ----------

func (l *LazyBrowserController) Navigate(ctx context.Context, url string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.Navigate(ctx, url)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) GetContent(ctx context.Context) (string, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return "", err
	}
	r, err := bc.GetContent(ctx)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) Click(ctx context.Context, selector string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.Click(ctx, selector)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) Type(ctx context.Context, selector, text string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.Type(ctx, selector, text)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) Screenshot(ctx context.Context) ([]byte, string, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return nil, "", err
	}
	data, mime, err := bc.Screenshot(ctx)
	l.resetOnConnErr(err)
	return data, mime, err
}

func (l *LazyBrowserController) Evaluate(ctx context.Context, script string) (any, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return nil, err
	}
	r, err := bc.Evaluate(ctx, script)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) WaitForSelector(ctx context.Context, selector string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.WaitForSelector(ctx, selector)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) GoBack(ctx context.Context) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.GoBack(ctx)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) GoForward(ctx context.Context) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.GoForward(ctx)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) GetURL(ctx context.Context) (string, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return "", err
	}
	r, err := bc.GetURL(ctx)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) SnapshotAI(ctx context.Context) (map[string]any, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return nil, err
	}
	r, err := bc.SnapshotAI(ctx)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) ClickRef(ctx context.Context, ref string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.ClickRef(ctx, ref)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) FillRef(ctx context.Context, ref, text string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.FillRef(ctx, ref, text)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) AIBrowse(ctx context.Context, goal string) (string, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return "", err
	}
	r, err := bc.AIBrowse(ctx, goal)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) AnnotateSOM(ctx context.Context) ([]byte, string, []SOMAnnotation, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return nil, "", nil, err
	}
	data, mime, ann, err := bc.AnnotateSOM(ctx)
	l.resetOnConnErr(err)
	return data, mime, ann, err
}

func (l *LazyBrowserController) StartGIFRecording() {
	l.mu.Lock()
	bc := l.inner
	l.mu.Unlock()
	if bc != nil {
		bc.StartGIFRecording()
	}
}

func (l *LazyBrowserController) StopGIFRecording() ([]byte, int, error) {
	l.mu.Lock()
	bc := l.inner
	l.mu.Unlock()
	if bc == nil {
		return nil, 0, fmt.Errorf("browser not initialized")
	}
	return bc.StopGIFRecording()
}

func (l *LazyBrowserController) IsGIFRecording() bool {
	l.mu.Lock()
	bc := l.inner
	l.mu.Unlock()
	if bc == nil {
		return false
	}
	return bc.IsGIFRecording()
}

func (l *LazyBrowserController) ListTabs(ctx context.Context) ([]TabInfo, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return nil, err
	}
	r, err := bc.ListTabs(ctx)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) CreateTab(ctx context.Context, url string) (*TabInfo, error) {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return nil, err
	}
	r, err := bc.CreateTab(ctx, url)
	l.resetOnConnErr(err)
	return r, err
}

func (l *LazyBrowserController) CloseTab(ctx context.Context, targetID string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.CloseTab(ctx, targetID)
	l.resetOnConnErr(err)
	return err
}

func (l *LazyBrowserController) SwitchTab(ctx context.Context, targetID string) error {
	bc, err := l.getOrInit(ctx)
	if err != nil {
		return err
	}
	err = bc.SwitchTab(ctx, targetID)
	l.resetOnConnErr(err)
	return err
}
