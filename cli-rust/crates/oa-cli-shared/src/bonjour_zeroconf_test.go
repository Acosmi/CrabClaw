package infra

import (
	"log/slog"
	"os"
	"testing"
)

func TestZeroconfRegistrarInterface(t *testing.T) {
	// 验证 ZeroconfRegistrar 满足 BonjourRegistrar 接口
	var _ BonjourRegistrar = (*ZeroconfRegistrar)(nil)
}

func TestZeroconfRegistrarInvalidPort(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := NewZeroconfRegistrar(logger)
	defer func() { _ = reg.Shutdown() }()

	err := reg.Register(BonjourServiceDef{
		Name: "test",
		Type: "_openacosmi-gw._tcp",
		Port: 0, // 无效端口
	})
	if err == nil {
		t.Error("expected error for invalid port 0")
	}

	err = reg.Register(BonjourServiceDef{
		Name: "test",
		Type: "_openacosmi-gw._tcp",
		Port: -1, // 负数端口
	})
	if err == nil {
		t.Error("expected error for negative port")
	}
}

func TestZeroconfRegistrarShutdownEmpty(t *testing.T) {
	reg := NewZeroconfRegistrar(nil)
	// 空注册器关闭不应 panic
	if err := reg.Shutdown(); err != nil {
		t.Errorf("shutdown empty registrar: %v", err)
	}
}

func TestZeroconfRegistrarShutdownIdempotent(t *testing.T) {
	reg := NewZeroconfRegistrar(nil)
	if err := reg.Shutdown(); err != nil {
		t.Errorf("first shutdown: %v", err)
	}
	if err := reg.Shutdown(); err != nil {
		t.Errorf("second shutdown: %v", err)
	}
}

// TestZeroconfRegistrarRegisterIntegration 实际 mDNS 注册测试。
// 需要网络环境，短模式跳过。
func TestZeroconfRegistrarRegisterIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	reg := NewZeroconfRegistrar(logger)
	defer func() { _ = reg.Shutdown() }()

	err := reg.Register(BonjourServiceDef{
		Name:     "Crab Claw Test",
		Type:     "_openacosmi-gw._tcp",
		Domain:   "local",
		Port:     19001,
		Hostname: "test-host",
		TXT: map[string]string{
			"role":        "gateway",
			"gatewayPort": "19001",
			"displayName": "Test Gateway",
		},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 验证注册成功后可以正常关闭
	if err := reg.Shutdown(); err != nil {
		t.Errorf("shutdown after register: %v", err)
	}
}
