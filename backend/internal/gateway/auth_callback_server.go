package gateway

// auth_callback_server.go — 本地 loopback HTTP server，接收 OAuth 授权回调。
// [FIX-01: P2A-X01 AuthCallbackServer 实现]
//
// 符合 RFC 8252 §7.3:
//   - 使用 127.0.0.1（不用 localhost，避免误监听非回环接口）
//   - 随机端口（:0，由 OS 分配临时端口）
//   - 收到 code 后立即关闭 listener

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// AuthCallbackServer 本地 loopback HTTP server，接收 OAuth 授权回调。
type AuthCallbackServer struct {
	listener net.Listener
	codeChan chan string
	errChan  chan error
	port     int
	server   *http.Server
}

// NewAuthCallbackServer 创建 callback server 并绑定随机端口。
// RFC 8252 §7.3: MUST use 127.0.0.1, MUST allow any port.
func NewAuthCallbackServer() (*AuthCallbackServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("auth callback: listen on loopback: %w", err)
	}

	addr := listener.Addr().(*net.TCPAddr)
	return &AuthCallbackServer{
		listener: listener,
		codeChan: make(chan string, 1),
		errChan:  make(chan error, 1),
		port:     addr.Port,
	}, nil
}

// Start 启动 HTTP server（后台 goroutine）。
func (s *AuthCallbackServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Warn("auth callback server error", "error", err)
		}
	}()
}

// handleCallback 处理 /callback?code=xxx&state=yyy 请求。
func (s *AuthCallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error")
		if errMsg == "" {
			errMsg = "no authorization code received"
		}
		s.errChan <- fmt.Errorf("oauth callback error: %s", errMsg)
		http.Error(w, "Authorization failed: "+errMsg, http.StatusBadRequest)
		return
	}

	s.codeChan <- code

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<html><body><h2>Authorization successful</h2><p>You can close this window.</p><script>window.close()</script></body></html>`)
}

// WaitForCode 等待授权码，默认超时 120s。
func (s *AuthCallbackServer) WaitForCode(timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case code := <-s.codeChan:
		return code, nil
	case err := <-s.errChan:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("auth callback: timeout waiting for authorization code (%v)", timeout)
	}
}

// Stop 关闭 callback server。
func (s *AuthCallbackServer) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

// RedirectURI 返回 OAuth redirect_uri。
// RFC 8252 §7.3: http://127.0.0.1:{port}/callback
func (s *AuthCallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", s.port)
}

// Port 返回监听端口。
func (s *AuthCallbackServer) Port() int {
	return s.port
}
