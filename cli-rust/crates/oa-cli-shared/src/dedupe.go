package infra

// dedupe.go — TTL 去重缓存
// 对应 TS: src/infra/dedupe.ts (63L)
//
// 提供线程安全的去重检查——给定 key 在 TTL 窗口内首次出现返回 false，
// 后续出现返回 true。支持 maxSize 淘汰策略。

import (
	"sync"
	"time"
)

// DedupeCache TTL 去重缓存。
type DedupeCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	maxSize int
	// 使用 slice 维护插入顺序，map 做快速查找
	cache map[string]time.Time
	order []string
}

// DedupeCacheOptions 去重缓存配置。
type DedupeCacheOptions struct {
	TTLMs   int `json:"ttlMs"`
	MaxSize int `json:"maxSize"`
}

// NewDedupeCache 创建去重缓存。
// 对应 TS: createDedupeCache(options)
func NewDedupeCache(opts DedupeCacheOptions) *DedupeCache {
	ttl := time.Duration(intMax(0, opts.TTLMs)) * time.Millisecond
	maxSize := intMax(0, opts.MaxSize)
	return &DedupeCache{
		ttl:     ttl,
		maxSize: maxSize,
		cache:   make(map[string]time.Time),
		order:   make([]string, 0),
	}
}

// Check 检查 key 是否在 TTL 内已出现过。
// 返回 true 表示重复（已存在），false 表示首次出现。
// 对应 TS: dedupeCache.check(key, now)
func (d *DedupeCache) Check(key string) bool {
	return d.CheckAt(key, time.Now())
}

// CheckAt 同 Check，但允许指定当前时间（便于测试）。
func (d *DedupeCache) CheckAt(key string, now time.Time) bool {
	if key == "" {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// 检查是否存在且未过期
	if ts, ok := d.cache[key]; ok {
		if d.ttl <= 0 || now.Sub(ts) < d.ttl {
			// 刷新时间
			d.touch(key, now)
			return true
		}
	}

	// 首次出现 → 记录并清理
	d.touch(key, now)
	d.prune(now)
	return false
}

// Clear 清空缓存。
func (d *DedupeCache) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string]time.Time)
	d.order = d.order[:0]
}

// Size 返回缓存大小。
func (d *DedupeCache) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.cache)
}

// ─── 内部方法 ───

func (d *DedupeCache) touch(key string, now time.Time) {
	if _, ok := d.cache[key]; ok {
		// 移除旧位置
		d.removeFromOrder(key)
	}
	d.cache[key] = now
	d.order = append(d.order, key)
}

func (d *DedupeCache) prune(now time.Time) {
	// TTL 淘汰
	if d.ttl > 0 {
		cutoff := now.Add(-d.ttl)
		for len(d.order) > 0 {
			oldest := d.order[0]
			if ts, ok := d.cache[oldest]; ok && ts.Before(cutoff) {
				delete(d.cache, oldest)
				d.order = d.order[1:]
			} else {
				break
			}
		}
	}

	// maxSize 淘汰
	if d.maxSize <= 0 {
		d.cache = make(map[string]time.Time)
		d.order = d.order[:0]
		return
	}
	for len(d.cache) > d.maxSize && len(d.order) > 0 {
		oldest := d.order[0]
		delete(d.cache, oldest)
		d.order = d.order[1:]
	}
}

func (d *DedupeCache) removeFromOrder(key string) {
	for i, k := range d.order {
		if k == key {
			d.order = append(d.order[:i], d.order[i+1:]...)
			return
		}
	}
}
