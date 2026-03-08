package infra

// warning_filter.go — 日志警告去重过滤
// 对应 TS: src/infra/warning-filter.ts (85L)
//
// 防止相同警告重复输出。基于 DedupeCache 实现。

import "sync"

// WarningFilter 日志警告去重过滤器。
type WarningFilter struct {
	mu   sync.Mutex
	seen map[string]bool
	max  int
}

// NewWarningFilter 创建警告过滤器。
// maxUnique: 最多记录多少个唯一警告（0 = 无限制）。
func NewWarningFilter(maxUnique int) *WarningFilter {
	if maxUnique <= 0 {
		maxUnique = 1000
	}
	return &WarningFilter{
		seen: make(map[string]bool),
		max:  maxUnique,
	}
}

// ShouldEmit 检查警告是否应该输出。
// 首次出现返回 true，重复出现返回 false。
// 对应 TS: warningFilter.shouldEmit(key)
func (f *WarningFilter) ShouldEmit(key string) bool {
	if key == "" {
		return true
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.seen[key] {
		return false
	}

	// 达到上限时不再记录新的
	if len(f.seen) >= f.max {
		return true // 放行但不记录
	}

	f.seen[key] = true
	return true
}

// Reset 清空过滤器。
func (f *WarningFilter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = make(map[string]bool)
}

// Size 已记录的唯一警告数。
func (f *WarningFilter) Size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}
