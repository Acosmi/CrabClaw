package infra

// system_presence.go — 系统存在感检测
// 对应 TS: src/infra/system-presence.ts
//
// 检测本地 openacosmi gateway 是否正在运行，以及系统是否就绪。
// 供 doctor 命令、infra 工具和监控模块使用。

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// SystemPresence gateway 运行状态快照。
type SystemPresence struct {
	// GatewayRunning 表示 gateway 端口当前可达。
	GatewayRunning bool `json:"gatewayRunning"`
	// GatewayPort 检查的端口号。
	GatewayPort int `json:"gatewayPort"`
	// LockFilePID 锁文件中记录的 PID（0 表示无锁或无法解析）。
	LockFilePID int `json:"lockFilePid,omitempty"`
	// LockFileStale 锁文件存在但对应进程已死。
	LockFileStale bool `json:"lockFileStale,omitempty"`
	// StateDir 状态目录路径。
	StateDir string `json:"stateDir"`
	// StateDirReady 状态目录存在且可写。
	StateDirReady bool `json:"stateDirReady"`
}

// CheckSystemPresence 查询本地 openacosmi 系统状态。
// 对应 TS: checkSystemPresence(stateDir, gatewayPort)
func CheckSystemPresence(stateDir string, gatewayPort int) *SystemPresence {
	presence := &SystemPresence{
		StateDir:    stateDir,
		GatewayPort: gatewayPort,
	}

	// 检查状态目录是否可写
	presence.StateDirReady = checkStateDirReady(stateDir)

	// 读取锁文件
	lockPath := filepath.Join(stateDir, "gateway.lock")
	if payload := readLockPayload(lockPath); payload != nil {
		presence.LockFilePID = payload.PID
		// 若 PID 已死，标记为陈旧锁
		if payload.PID > 0 && !isProcessAlive(payload.PID) {
			presence.LockFileStale = true
		}
	}

	// 检查 gateway 端口是否响应
	addr := fmt.Sprintf("127.0.0.1:%d", gatewayPort)
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err == nil {
		conn.Close()
		presence.GatewayRunning = true
	}

	return presence
}

// IsGatewayRunning 快速检查 gateway 是否正在监听指定端口。
// 对应 TS: isGatewayRunning(port)
func IsGatewayRunning(stateDir string, gatewayPort int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", gatewayPort)
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkStateDirReady 检查状态目录是否存在且可写。
func checkStateDirReady(stateDir string) bool {
	info, err := os.Stat(stateDir)
	if err != nil || !info.IsDir() {
		return false
	}
	// 写入测试
	tmp := filepath.Join(stateDir, fmt.Sprintf(".presence_test_%d", time.Now().UnixNano()))
	if writeErr := os.WriteFile(tmp, nil, 0o600); writeErr != nil {
		return false
	}
	os.Remove(tmp)
	return true
}

// WaitForGateway 等待 gateway 端口就绪，超时后返回 false。
// 对应 TS: waitForGateway(port, timeoutMs)
func WaitForGateway(gatewayPort int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", gatewayPort)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
