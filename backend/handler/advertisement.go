package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 广告位白名单。代理只转发这几个固定位置，避免变成可被任意调用的开放代理。
var advertisementPositions = []string{"home-banner", "sidebar", "popup"}

const advertisementMaxBytes = int64(256 << 10)

// advertisementRecord 的字段与上游、前端完全一致，代理层原样透传，前端不需要再做映射。
type advertisementRecord struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	ImageURL       string `json:"imageUrl"`
	DestinationURL string `json:"destinationUrl"`
	Position       string `json:"position"`
	Weight         int    `json:"weight"`
	StartAt        string `json:"startAt"`
	EndAt          string `json:"endAt"`
	Description    string `json:"description"`
}

type advertisementUpstreamResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Records []advertisementRecord `json:"records"`
	} `json:"data"`
}

type advertisementCacheEntry struct {
	fetchedAt time.Time
	records   []advertisementRecord
}

var (
	// 每个广告位一把锁：冷缓存且上游卡死时，三个位置各自等待，不会串成 3 倍超时。
	advertisementLocks  = map[string]*sync.Mutex{}
	advertisementCache  = map[string]advertisementCacheEntry{}
	advertisementCacheL sync.RWMutex
)

func init() {
	for _, position := range advertisementPositions {
		advertisementLocks[position] = &sync.Mutex{}
	}
}

// PublicAdvertisements 默认返回本站广告；仅当 AUTO_PRO_ADVERTISEMENT_URL 指向 http(s) 时才代理外部投放。
// 前端直连上游会被 CORS 拦截，且共用的 axios 实例会附带后台 JWT，所以统一从这里转发。
func PublicAdvertisements(c *gin.Context) {
	position := strings.TrimSpace(c.Query("position"))
	if advertisementLocks[position] == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告位标识不合法"})
		return
	}
	records := advertisementsForPosition(c.Request.Context(), position)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok", "data": gin.H{"records": records}})
}

// advertisementsForPosition 永远返回可渲染的结果：上游异常时退回旧缓存，再不行就是空列表。
// 广告不是业务功能，不该因为第三方抖动把错误抛给后台界面。
func advertisementsForPosition(ctx context.Context, position string) []advertisementRecord {
	lock := advertisementLocks[position]
	lock.Lock()
	defer lock.Unlock()

	cached, cachedOK := readAdvertisementCache(position)
	if cachedOK && time.Since(cached.fetchedAt) < config.GetAdvertisementCacheTTL() {
		return cached.records
	}

	fresh, err := fetchAdvertisementsUpstream(ctx, position)
	if err == nil {
		writeAdvertisementCache(position, fresh)
		return fresh
	}
	if cachedOK && time.Since(cached.fetchedAt) <= config.GetAdvertisementStaleTTL() {
		return cached.records
	}
	return []advertisementRecord{}
}

func readAdvertisementCache(position string) (advertisementCacheEntry, bool) {
	advertisementCacheL.RLock()
	defer advertisementCacheL.RUnlock()
	entry, ok := advertisementCache[position]
	return entry, ok
}

// 空结果同样入缓存：没有投放的广告位不该每次刷新都去打上游。
func writeAdvertisementCache(position string, records []advertisementRecord) {
	advertisementCacheL.Lock()
	defer advertisementCacheL.Unlock()
	advertisementCache[position] = advertisementCacheEntry{fetchedAt: time.Now(), records: records}
}

func fetchAdvertisementsUpstream(ctx context.Context, position string) ([]advertisementRecord, error) {
	if !config.AdvertisementURLIsRemote() {
		return localAdvertisements(position), nil
	}
	endpoint, err := url.Parse(config.GetAdvertisementURL())
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("广告接口地址不合法")
	}
	query := endpoint.Query()
	query.Set("position", position)
	endpoint.RawQuery = query.Encode()

	timeout := config.GetAdvertisementTimeout()
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("广告接口返回异常状态")
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, advertisementMaxBytes))
	if err != nil {
		return nil, err
	}
	var upstream advertisementUpstreamResponse
	if err := json.Unmarshal(payload, &upstream); err != nil {
		return nil, err
	}
	if upstream.Code != 200 {
		return nil, errors.New("广告接口返回业务失败")
	}
	return normalizeAdvertisements(upstream.Data.Records, position, time.Now()), nil
}

// normalizeAdvertisements 丢弃不属于该位置和已过投放窗口的记录，并按权重倒序排列。
// records 为 null 时同样返回空切片，保证前端拿到的永远是数组。
func normalizeAdvertisements(records []advertisementRecord, position string, now time.Time) []advertisementRecord {
	normalized := make([]advertisementRecord, 0, len(records))
	for _, record := range records {
		if record.Position != "" && record.Position != position {
			continue
		}
		if !advertisementInWindow(record, now) {
			continue
		}
		normalized = append(normalized, record)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Weight > normalized[j].Weight
	})
	return normalized
}

// 时间字段缺失或无法解析时视为该侧无边界：宁可多投，也不要因为格式问题把有效广告误杀。
func advertisementInWindow(record advertisementRecord, now time.Time) bool {
	if startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(record.StartAt)); err == nil && now.Before(startAt) {
		return false
	}
	if endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(record.EndAt)); err == nil && now.After(endAt) {
		return false
	}
	return true
}

// localAdvertisements 是本站自托管投放。未配置远程广告源时读本站广告表，不访问外网。
func localAdvertisements(position string) []advertisementRecord {
	return localSourceAdvertisements(position)
}

// PublicLocalAdvertisements 是与上游协议兼容的本站广告接口。
func PublicLocalAdvertisements(c *gin.Context) {
	position := strings.TrimSpace(c.Query("position"))
	if advertisementLocks[position] == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告位标识不合法"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok", "data": gin.H{"records": localAdvertisements(position)}})
}
