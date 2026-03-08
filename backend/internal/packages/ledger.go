package packages

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// PackageLedger 统一安装账本。
// 持久化到 VFS: <configDir>/packages/installs.json
type PackageLedger struct {
	filePath string
	mu       sync.RWMutex
	records  []types.PackageInstallRecord
}

// NewPackageLedger 创建安装账本实例，自动从磁盘加载已有记录。
func NewPackageLedger(filePath string) *PackageLedger {
	l := &PackageLedger{filePath: filePath}
	l.load()
	return l
}

// Add 添加安装记录。
func (l *PackageLedger) Add(record types.PackageInstallRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, r := range l.records {
		if r.ID == record.ID {
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			l.records[i] = record
			return l.persist()
		}
	}

	l.records = append(l.records, record)
	return l.persist()
}

// Remove 移除安装记录。
func (l *PackageLedger) Remove(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, r := range l.records {
		if r.ID == id {
			l.records = append(l.records[:i], l.records[i+1:]...)
			return l.persist()
		}
	}
	return nil
}

// List 列出安装记录，可按 kind 过滤（空字符串返回全部）。
func (l *PackageLedger) List(kind types.PackageKind) []types.PackageInstallRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if kind == "" {
		result := make([]types.PackageInstallRecord, len(l.records))
		copy(result, l.records)
		return result
	}

	var result []types.PackageInstallRecord
	for _, r := range l.records {
		if r.Kind == kind {
			result = append(result, r)
		}
	}
	return result
}

// Has 检查指定 key 是否已安装。
func (l *PackageLedger) Has(key string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, r := range l.records {
		if r.Key == key {
			return true
		}
	}
	return false
}

// [FIX P3-M01: 非 NotExist 错误和 JSON 解析错误记录日志，防止静默数据丢失]
func (l *PackageLedger) load() {
	data, err := os.ReadFile(l.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("packages: ledger load failed", "path", l.filePath, "error", err)
		}
		return
	}
	var records []types.PackageInstallRecord
	if err := json.Unmarshal(data, &records); err != nil {
		slog.Warn("packages: ledger unmarshal failed", "path", l.filePath, "error", err)
		return
	}
	l.records = records
}

func (l *PackageLedger) persist() error {
	data, err := json.MarshalIndent(l.records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ledger: %w", err)
	}

	dir := filepath.Dir(l.filePath)
	// [FIX P3-L02: 敏感数据目录使用 0o700 owner-only 权限]
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}

	tmpFile := l.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return fmt.Errorf("write ledger tmp: %w", err)
	}
	if err := os.Rename(tmpFile, l.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename ledger: %w", err)
	}
	return nil
}
