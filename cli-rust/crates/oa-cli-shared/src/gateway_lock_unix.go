//go:build !windows

package infra

import (
	"os"
	"syscall"
)

// sendSignalZero 向进程发送信号 0，用于检测进程是否存在（Unix only）。
// 等价于 kill(pid, 0)：若进程存在返回 true，否则返回 false。
func sendSignalZero(proc *os.Process) bool {
	err := proc.Signal(syscall.Signal(0))
	return err == nil
}

// isProcessAliveWindows 仅在 Windows 使用，Unix 下不编译。
func isProcessAliveWindows(_ *os.Process) bool { return false }
