package infra

// gateway_lock.go — 网关实例锁
// 对应 TS: src/infra/gateway-lock.ts
//
// 防止同一配置的网关多实例并发运行。
// 锁文件路径：stateDir/gateway.lock（写入当前进程 PID 和元数据）。
// 若锁文件已存在且 PID 对应进程仍在运行，返回 ErrGatewayAlreadyRunning 错误。
// 获取成功后返回 unlock 函数，调用后删除锁文件。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	gatewayLockFileName = "gateway.lock"
	defaultStaleMs      = 30_000 // 30 秒后视为陈旧锁
)

// ErrGatewayAlreadyRunning 表示网关已在另一个进程中运行。
var ErrGatewayAlreadyRunning = errors.New("gateway already running")

// GatewayLockError 锁操作错误，携带原始错误。
type GatewayLockError struct {
	Message string
	Cause   error
}

func (e *GatewayLockError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *GatewayLockError) Unwrap() error { return e.Cause }

// lockPayload 锁文件的 JSON 内容。
// 对应 TS: LockPayload { pid, createdAt, configPath, startTime? }
type lockPayload struct {
	PID        int    `json:"pid"`
	CreatedAt  string `json:"createdAt"`
	ConfigPath string `json:"configPath"`
}

// GatewayLockOptions 获取锁时的可选参数。
type GatewayLockOptions struct {
	// StaleMs 超过多少毫秒后陈旧锁视为可强占（默认 30000ms）。
	StaleMs int64
}

// AcquireGatewayLock 获取网关实例锁。
// 对应 TS: acquireGatewayLock(opts?)
//
// 成功后返回 unlock 函数，调用后释放锁文件。
// 如果锁已被活跃进程持有，返回 ErrGatewayAlreadyRunning 错误。
// 如果锁文件陈旧（PID 对应进程已死），自动清理后重新获取。
func AcquireGatewayLock(stateDir string, opts *GatewayLockOptions) (unlock func(), err error) {
	// 对应 TS: gateway-lock.ts:182 — env.OPENACOSMI_ALLOW_MULTI_GATEWAY === "1"
	if AllowMultiGateway() {
		return nil, nil
	}

	staleMs := int64(defaultStaleMs)
	if opts != nil && opts.StaleMs > 0 {
		staleMs = opts.StaleMs
	}

	lockPath := filepath.Join(stateDir, gatewayLockFileName)

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, &GatewayLockError{
			Message: fmt.Sprintf("创建锁文件目录失败: %s", filepath.Dir(lockPath)),
			Cause:   err,
		}
	}

	// 尝试原子创建锁文件（O_EXCL 保证独占）
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if !os.IsExist(err) {
			return nil, &GatewayLockError{
				Message: fmt.Sprintf("创建锁文件失败: %s", lockPath),
				Cause:   err,
			}
		}

		// 锁文件已存在 — 检查是否陈旧或进程已死
		if canSteal, stealErr := checkAndStealStaleLock(lockPath, staleMs); canSteal {
			if stealErr != nil {
				return nil, stealErr
			}
			// 重新尝试获取
			f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return nil, &GatewayLockError{
					Message: "清理陈旧锁后仍无法获取锁",
					Cause:   err,
				}
			}
		} else {
			// 进程仍在运行
			payload := readLockPayload(lockPath)
			ownerDesc := ""
			if payload != nil {
				ownerDesc = fmt.Sprintf(" (pid %d)", payload.PID)
			}
			return nil, &GatewayLockError{
				Message: fmt.Sprintf("gateway already running%s", ownerDesc),
				Cause:   ErrGatewayAlreadyRunning,
			}
		}
	}

	// 写入锁内容
	payload := lockPayload{
		PID:        os.Getpid(),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		ConfigPath: stateDir,
	}
	data, _ := json.Marshal(payload)
	_, _ = f.Write(data)
	_ = f.Close()

	// 返回 unlock 函数
	released := false
	unlockFn := func() {
		if released {
			return
		}
		released = true
		// 验证锁文件仍然属于本进程再删除
		if current := readLockPayload(lockPath); current != nil && current.PID == os.Getpid() {
			_ = os.Remove(lockPath)
		}
	}

	return unlockFn, nil
}

// checkAndStealStaleLock 检查现有锁文件，若锁持有者已死或锁陈旧则删除锁文件。
// 返回 (true, nil) 表示已删除可重新获取，(false, nil) 表示锁仍被活跃进程持有。
func checkAndStealStaleLock(lockPath string, staleMs int64) (bool, error) {
	payload := readLockPayload(lockPath)

	// 检查 PID 是否仍在运行
	if payload != nil {
		if isProcessAlive(payload.PID) {
			return false, nil
		}
		// 进程已死 — 删除陈旧锁
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return false, &GatewayLockError{
				Message: "删除死进程的锁文件失败",
				Cause:   err,
			}
		}
		return true, nil
	}

	// 无法解析 payload — 检查文件修改时间是否陈旧
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, nil
	}
	ageMs := time.Since(info.ModTime()).Milliseconds()
	if ageMs > staleMs {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return false, nil
		}
		return true, nil
	}

	return false, nil
}

// readLockPayload 读取并解析锁文件内容，失败返回 nil。
func readLockPayload(lockPath string) *lockPayload {
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		return nil
	}
	var p lockPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		// 兼容纯数字 PID 格式（旧版）
		pidStr := strings.TrimSpace(string(raw))
		pid, parseErr := strconv.Atoi(pidStr)
		if parseErr != nil {
			return nil
		}
		return &lockPayload{PID: pid}
	}
	if p.PID <= 0 {
		return nil
	}
	return &p
}

// isProcessAlive 检查指定 PID 的进程是否仍在运行。
// 对应 TS: isAlive(pid) → process.kill(pid, 0)
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Unix: kill(pid, 0) — 仅检查进程是否存在，不发送信号
	// Windows: FindProcess 总是成功，需要额外检查
	if runtime.GOOS == "windows" {
		return isProcessAliveWindows(proc)
	}
	return sendSignalZero(proc)
}
