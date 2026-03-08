//go:build windows

package infra

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sendSignalZero 在 Windows 下不可用（Windows 没有 POSIX 信号机制）。
// 始终返回 false，由 isProcessAliveWindows 处理。
func sendSignalZero(_ *os.Process) bool { return false }

// isProcessAliveWindows 在 Windows 下检测进程是否存在。
// 使用 OpenProcess + GetProcessTimes 替代 tasklist 命令：
//   - OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION) 检查句柄是否可获取
//   - 如果失败 → 进程不存在（PID 无效或权限不足）
//   - 如果成功 → 获取 GetExitCodeProcess 确认进程未退出
//
// 对齐 TS: process.kill(pid, 0) 等价行为。
func isProcessAliveWindows(proc *os.Process) bool {
	if proc == nil {
		return false
	}
	pid := proc.Pid
	if pid <= 0 {
		return false
	}

	// PROCESS_QUERY_LIMITED_INFORMATION (0x1000) — 最小权限查询
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		// ERROR_INVALID_PARAMETER 或 ERROR_ACCESS_DENIED → 进程不存在或无权限
		return false
	}
	defer windows.CloseHandle(handle)

	// 获取退出码：STILL_ACTIVE (259) 表示进程仍在运行
	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		// GetExitCodeProcess 失败 — 保守返回 true（避免误抢锁）
		return true
	}

	const STILL_ACTIVE = 259
	return exitCode == STILL_ACTIVE
}

// getProcessStartTime 获取 Windows 进程的创建时间（用于 PID 复用检测）。
// 返回 FILETIME 的 100 纳秒时间戳，失败返回 0。
func getProcessStartTime(pid int) int64 {
	if pid <= 0 {
		return 0
	}

	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(handle)

	var creation, exit, kernel, user windows.Filetime
	err = windows.GetProcessTimes(
		handle,
		&creation,
		&exit,
		&kernel,
		&user,
	)
	if err != nil {
		return 0
	}

	return *(*int64)(unsafe.Pointer(&creation))
}
