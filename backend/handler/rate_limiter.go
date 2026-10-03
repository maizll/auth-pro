package handler

import (
	"hash/fnv"
	"sync"
	"time"
)

// rateLimiterShards 把来源分散到多把锁上，来源再多也不会所有请求挤在一把全局锁里。
const (
	rateLimiterShards      = 64
	rateLimiterShardMaxKey = 4096 // 每片最多记这么多来源，64 片合计约 26 万个，内存有上限
)

// rateLimiter 是分片令牌桶：每个来源一个桶，容量 limit，按 limit/window 的速度回补。
// 每个来源只存「剩余令牌 + 上次时间」两个数，不存每次请求的时间戳；
// 过期的桶（已经回满，等于没记）每过一个窗口按片清一次，不会每个请求扫一遍全部来源。
// 授权校验、更新接口、目录安装包下载的限流都用它。
type rateLimiter struct {
	limit  float64
	window time.Duration
	perSec float64
	shards [rateLimiterShards]rateLimiterShard
}

type rateLimiterShard struct {
	mu        sync.Mutex
	buckets   map[string]rateBucket
	lastSweep time.Time
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	limiter := &rateLimiter{limit: float64(limit), window: window, perSec: float64(limit) / window.Seconds()}
	for index := range limiter.shards {
		limiter.shards[index].buckets = make(map[string]rateBucket)
	}
	return limiter
}

// allow 取一个令牌；没有令牌时拒绝，拒绝本身不消耗令牌。
func (limiter *rateLimiter) allow(key string, now time.Time) bool {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))
	shard := &limiter.shards[hasher.Sum32()%rateLimiterShards]
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if now.Sub(shard.lastSweep) >= limiter.window {
		shard.sweep(now, limiter.window)
	}
	bucket, ok := shard.buckets[key]
	if !ok {
		if len(shard.buckets) >= rateLimiterShardMaxKey {
			shard.sweep(now, limiter.window)
			if len(shard.buckets) >= rateLimiterShardMaxKey {
				// 仍然满了就随便丢一个旧来源（Go 的 map 遍历顺序随机），宁可放宽一个来源也不让内存无限涨。
				for victim := range shard.buckets {
					delete(shard.buckets, victim)
					break
				}
			}
		}
		bucket = rateBucket{tokens: limiter.limit, last: now}
	} else if elapsed := now.Sub(bucket.last); elapsed > 0 {
		bucket.tokens += elapsed.Seconds() * limiter.perSec
		if bucket.tokens > limiter.limit {
			bucket.tokens = limiter.limit
		}
		bucket.last = now
	}
	if bucket.tokens < 1 {
		shard.buckets[key] = bucket
		return false
	}
	bucket.tokens--
	shard.buckets[key] = bucket
	return true
}

// sweep 删掉一个窗口内没再来过的来源：它们的桶已经回满，删掉和保留效果一样。
func (shard *rateLimiterShard) sweep(now time.Time, window time.Duration) {
	for key, bucket := range shard.buckets {
		if now.Sub(bucket.last) >= window {
			delete(shard.buckets, key)
		}
	}
	shard.lastSweep = now
}

// size 返回当前记着的来源数，测试用来确认内存有上限。
func (limiter *rateLimiter) size() int {
	total := 0
	for index := range limiter.shards {
		shard := &limiter.shards[index]
		shard.mu.Lock()
		total += len(shard.buckets)
		shard.mu.Unlock()
	}
	return total
}
