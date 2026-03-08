package infra

// fs_safe.go — 文件系统安全操作
// 对应 TS: src/infra/fs-safe.ts (105L)
//
// 原子写入、安全目录创建等文件操作。

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureDir 确保目录存在（递归创建）。
// 对应 TS: ensureDir(dirPath)
func EnsureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0o755)
}

// WriteFileAtomic 原子写入文件（先写临时文件，再 rename）。
// 对应 TS: writeFileAtomic(filePath, content)
//
// 确保不会出现半写文件——要么完整写入，要么完全不改。
func WriteFileAtomic(filePath string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(filePath)
	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(filePath)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	defer func() {
		// 清理失败时的临时文件
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err = os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err = os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// ReadFileSafe 安全读取文件（文件不存在返回 nil 而非错误）。
// 对应 TS: readFileSafe(filePath)
func ReadFileSafe(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// RemoveFileSafe 安全删除文件（文件不存在不视为错误）。
func RemoveFileSafe(filePath string) error {
	err := os.Remove(filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// FileExists 检查文件是否存在。
func FileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}

// DirExists 检查目录是否存在。
func DirExists(dirPath string) bool {
	info, err := os.Stat(dirPath)
	return err == nil && info.IsDir()
}
