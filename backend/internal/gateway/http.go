package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ---------- HTTP 服务器配置 ----------

// ServerConfig HTTP/HTTPS 服务器配置。
type ServerConfig struct {
	Host            string
	Port            int
	TLSCert         string
	TLSKey          string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	MaxHeaderBytes  int
}

// DefaultServerConfig 默认服务器配置。
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Host:            "0.0.0.0",
		Port:            3777,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    0, // SSE 需要无限写超时
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1MB
	}
}

// ---------- 路由 ----------

// Router HTTP 路由器。
type Router struct {
	mu     sync.RWMutex
	mux    *http.ServeMux
	prefix string
}

// NewRouter 创建路由器。
func NewRouter(prefix string) *Router {
	return &Router{mux: http.NewServeMux(), prefix: prefix}
}

// Handle 注册路由。
func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mux.Handle(r.prefix+pattern, handler)
}

// HandleFunc 注册路由函数。
func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.Handle(pattern, handler)
}

// ServeHTTP 实现 http.Handler。
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.mux.ServeHTTP(w, req)
}

// ---------- 中间件 ----------

// Middleware HTTP 中间件类型。
type Middleware func(next http.Handler) http.Handler

// Chain 将多个中间件链接成一个。
func Chain(middlewares ...Middleware) Middleware {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// CORSMiddleware CORS 中间件。
func CORSMiddleware(allowOrigin string) Middleware {
	if allowOrigin == "" {
		allowOrigin = "*"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CrabClaw-Token, X-OpenAcosmi-Token, X-CrabClaw-Agent-Id, X-OpenAcosmi-Agent-Id, X-CrabClaw-Agent, X-OpenAcosmi-Agent")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware Bearer token 认证中间件。
func AuthMiddleware(token string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqToken := GetGatewayToken(r)
			if !SafeEqual(reqToken, token) {
				SendUnauthorized(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------- HTTP 服务器 ----------

// GatewayHTTPServer 网关 HTTP 服务器。
type GatewayHTTPServer struct {
	config ServerConfig
	server *http.Server
}

// NewGatewayHTTPServer 创建 HTTP 服务器。
func NewGatewayHTTPServer(config ServerConfig, handler http.Handler) *GatewayHTTPServer {
	srv := &http.Server{
		Addr:           fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:        handler,
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    config.IdleTimeout,
		MaxHeaderBytes: config.MaxHeaderBytes,
	}
	if config.TLSCert != "" && config.TLSKey != "" {
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &GatewayHTTPServer{config: config, server: srv}
}

// ListenAndServe 启动 HTTP 或 HTTPS 服务器。
func (s *GatewayHTTPServer) ListenAndServe() error {
	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		return s.server.ListenAndServeTLS(s.config.TLSCert, s.config.TLSKey)
	}
	return s.server.ListenAndServe()
}

// Shutdown 优雅关闭服务器。
func (s *GatewayHTTPServer) Shutdown() error {
	timeout := s.config.ShutdownTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// Addr 返回监听地址。
func (s *GatewayHTTPServer) Addr() string {
	return s.server.Addr
}
