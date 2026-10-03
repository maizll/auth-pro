package handler

import (
	"sync"
	"time"
)

// replayNonceCache 记住一段时间内用过的随机数，拒绝重放。
// 两代表轮换：当前代满一个窗口就变成上一代，再满一个窗口丢掉。查找和写入都是 O(1)，不用每次扫全表。
// 每代最多 maxPerGen 条，满了就不再记（只放行不记录），宁可少防一次重放也不让内存无限涨。
type replayNonceCache struct {
	mu        sync.Mutex
	window    time.Duration
	maxPerGen int
	rotatedAt time.Time
	current   map[string]struct{}
	previous  map[string]struct{}
}

func newReplayNonceCache(window time.Duration, maxPerGen int) *replayNonceCache {
	return &replayNonceCache{window: window, maxPerGen: maxPerGen, current: map[string]struct{}{}, previous: map[string]struct{}{}}
}

// remember 第一次见到 key 返回 true 并记下；窗口内再次出现返回 false。
func (c *replayNonceCache) remember(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rotatedAt.IsZero() {
		c.rotatedAt = now
	}
	for now.Sub(c.rotatedAt) >= c.window {
		c.previous, c.current = c.current, map[string]struct{}{}
		c.rotatedAt = c.rotatedAt.Add(c.window)
		if now.Sub(c.rotatedAt) >= c.window {
			// 停了很久没有请求：两代都已过期，直接从现在重新计时。
			c.previous = map[string]struct{}{}
			c.rotatedAt = now
		}
	}
	if _, ok := c.current[key]; ok {
		return false
	}
	if _, ok := c.previous[key]; ok {
		return false
	}
	if len(c.current) < c.maxPerGen {
		c.current[key] = struct{}{}
	}
	return true
}

// 商店签名请求和 SDK v3 授权校验共用一份随机数记录，键带前缀区分。
// SDK 时间戳前后各容忍 10 分钟（商店 5 分钟），一个请求最长 20 分钟内有效，所以每代窗口取 20 分钟，保证有效期内一直记得。
var requestNonces = newReplayNonceCache(20*time.Minute, 1_000_000)
