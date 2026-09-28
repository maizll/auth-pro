// 源站把发布清单和安装包转给客户站。
// 客户站从 1.7.1 起只访问 https://auth.maizll.com，不再直连代码托管站，
// 所以响应、错误信息和日志里都不能带仓库地址或令牌。
// 令牌复用收费仓库或 Release 设置里已经保存的那一枚，不另建一套。
// 仓库还公开时，没有令牌也能拉。仓库名只从服务器环境变量读取，不写进程序。

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	productUpdateManifestTTL  = 3 * time.Minute
	productUpdateJSONLimit    = 30
	productUpdatePackageLimit = 6
	productUpdateRateWindow   = time.Minute
	productUpdateRepoEnv      = "AUTO_PRO_UPDATE_REPOSITORY"
	// 官网从客户交付仓库的 Release 取安装包。仓库已是私有的，必须用源站已保存的令牌。
	// 环境变量可以改成别的 owner/repo；不设时用这个默认值。
	productUpdateDefaultRepository = "maizll/auth-pro-client"
	productUpdateUnavailable       = "暂时无法获取更新"
	productUpdateRepoMissing       = "更新仓库未配置"
	productUpdateRateLimited       = "请求过于频繁，请稍后再试"
	productUpdatePackagePrefix     = "https://auth.maizll.com/api/v1/update/package/"
	productUpdateReleasesURL       = "https://auth.maizll.com/api/v1/update/releases.json"
)

var (
	errProductUpdateUnavailable = errors.New(productUpdateUnavailable)
	errProductUpdateRepoMissing = errors.New(productUpdateRepoMissing)

	// productUpdateGitHubAPI 正式程序是代码托管站的 API 根。测试换成 httptest。
	productUpdateGitHubAPI  = "https://api.github.com"
	productUpdateHTTPClient = &http.Client{Timeout: 2 * time.Minute}
)

type productUpdateRateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newProductUpdateRateLimiter(limit int, window time.Duration) *productUpdateRateLimiter {
	return &productUpdateRateLimiter{hits: make(map[string][]time.Time), limit: limit, window: window}
}

func (limiter *productUpdateRateLimiter) allow(key string, now time.Time) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	cutoff := now.Add(-limiter.window)
	kept := make([]time.Time, 0, limiter.limit)
	for _, hit := range limiter.hits[key] {
		if hit.After(cutoff) {
			kept = append(kept, hit)
		}
	}
	if len(kept) >= limiter.limit {
		limiter.hits[key] = kept
		return false
	}
	limiter.hits[key] = append(kept, now)
	return true
}

var (
	productUpdateJSONLimiter    = newProductUpdateRateLimiter(productUpdateJSONLimit, productUpdateRateWindow)
	productUpdatePackageLimiter = newProductUpdateRateLimiter(productUpdatePackageLimit, productUpdateRateWindow)
	productUpdateCacheMu        sync.Mutex
	productUpdateLatestCache    struct {
		body      []byte
		expiresAt time.Time
	}
	productUpdateReleasesCache struct {
		body      []byte
		expiresAt time.Time
	}
	productUpdatePackageMu   sync.Mutex
	productUpdatePackageLock = map[string]*sync.Mutex{}
)

// RegisterProductUpdateRoutes 注册不登录也能用的更新分发接口。
// 免费版客户也要能更新，所以这里不查授权；用限流挡住批量拉取。
func RegisterProductUpdateRoutes(api *gin.RouterGroup) {
	api.GET("/v1/update/latest.json", ProductUpdateLatest)
	api.GET("/v1/update/releases.json", ProductUpdateReleases)
	api.GET("/v1/update/package/:version", ProductUpdatePackage)
}

func ProductUpdateLatest(c *gin.Context) {
	if !productUpdateAllow(c, productUpdateJSONLimiter) {
		return
	}
	body, err := productUpdateLatestBody(c.Request.Context(), false)
	if err != nil {
		productUpdateFail(c, err)
		return
	}
	productUpdateJSON(c, body)
}

func ProductUpdateReleases(c *gin.Context) {
	if !productUpdateAllow(c, productUpdateJSONLimiter) {
		return
	}
	body, err := productUpdateReleasesBody(c.Request.Context(), false)
	if err != nil {
		productUpdateFail(c, err)
		return
	}
	productUpdateJSON(c, body)
}

func ProductUpdatePackage(c *gin.Context) {
	if !productUpdateAllow(c, productUpdatePackageLimiter) {
		return
	}
	version := strings.TrimPrefix(strings.TrimSpace(c.Param("version")), "v")
	if version == "latest" {
		manifest, err := productUpdateCachedManifest(c.Request.Context())
		if err != nil {
			productUpdateFail(c, err)
			return
		}
		version = strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
	}
	if _, ok := parseOnlineUpdateVersion(version); !ok {
		c.String(http.StatusBadRequest, "版本号不正确")
		return
	}
	path, fileName, err := productUpdatePackageFile(c.Request.Context(), version)
	if err != nil {
		productUpdateFail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(path, fileName)
}

func productUpdateAllow(c *gin.Context, limiter *productUpdateRateLimiter) bool {
	if limiter.allow(c.ClientIP(), time.Now()) {
		return true
	}
	c.String(http.StatusTooManyRequests, productUpdateRateLimited)
	return false
}

func productUpdateFail(c *gin.Context, err error) {
	status := http.StatusBadGateway
	msg := productUpdateUnavailable
	if errors.Is(err, errProductUpdateRepoMissing) {
		status = http.StatusServiceUnavailable
		msg = productUpdateRepoMissing
	}
	// 上游错误里可能带着仓库地址或跳转链接，这里只回固定句子。
	c.String(status, msg)
}

func productUpdateJSON(c *gin.Context, body []byte) {
	c.Header("Cache-Control", "public, max-age=120")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func productUpdateLatestBody(ctx context.Context, force bool) ([]byte, error) {
	productUpdateCacheMu.Lock()
	if !force && len(productUpdateLatestCache.body) > 0 && time.Now().Before(productUpdateLatestCache.expiresAt) {
		body := append([]byte(nil), productUpdateLatestCache.body...)
		productUpdateCacheMu.Unlock()
		return body, nil
	}
	productUpdateCacheMu.Unlock()

	raw, err := productUpdateFetchReleaseAsset(ctx, "latest", "latest.json")
	if err != nil {
		return nil, err
	}
	body, err := rewriteProductUpdateManifest(raw)
	if err != nil {
		return nil, errProductUpdateUnavailable
	}
	productUpdateCacheMu.Lock()
	productUpdateLatestCache.body = append([]byte(nil), body...)
	productUpdateLatestCache.expiresAt = time.Now().Add(productUpdateManifestTTL)
	productUpdateCacheMu.Unlock()
	return body, nil
}

func productUpdateReleasesBody(ctx context.Context, force bool) ([]byte, error) {
	productUpdateCacheMu.Lock()
	if !force && len(productUpdateReleasesCache.body) > 0 && time.Now().Before(productUpdateReleasesCache.expiresAt) {
		body := append([]byte(nil), productUpdateReleasesCache.body...)
		productUpdateCacheMu.Unlock()
		return body, nil
	}
	productUpdateCacheMu.Unlock()

	raw, err := productUpdateFetchReleaseAsset(ctx, "latest", "releases.json")
	if err != nil {
		return nil, err
	}
	body, err := rewriteProductUpdateReleases(raw)
	if err != nil {
		return nil, errProductUpdateUnavailable
	}
	productUpdateCacheMu.Lock()
	productUpdateReleasesCache.body = append([]byte(nil), body...)
	productUpdateReleasesCache.expiresAt = time.Now().Add(productUpdateManifestTTL)
	productUpdateCacheMu.Unlock()
	return body, nil
}

func productUpdateCachedManifest(ctx context.Context) (*onlineUpdateManifest, error) {
	body, err := productUpdateLatestBody(ctx, false)
	if err != nil {
		return nil, err
	}
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, errProductUpdateUnavailable
	}
	return &manifest, nil
}

func productUpdatePackageFile(ctx context.Context, version string) (string, string, error) {
	fileName, err := onlineUpdatePackageFileName(version)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(productUpdatePackageCacheDir(version), fileName)
	if info, statErr := os.Stat(target); statErr == nil && info.Size() > 0 {
		return target, fileName, nil
	}

	lock := productUpdateLockFor(version)
	lock.Lock()
	defer lock.Unlock()
	if info, statErr := os.Stat(target); statErr == nil && info.Size() > 0 {
		return target, fileName, nil
	}
	payload, err := productUpdateFetchReleaseAsset(ctx, "tags/v"+version, fileName)
	if err != nil {
		return "", "", err
	}
	if int64(len(payload)) > maxOnlineUpdatePackageSize {
		return "", "", errProductUpdateUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", "", errProductUpdateUnavailable
	}
	temp := target + ".partial"
	if err := os.WriteFile(temp, payload, 0644); err != nil {
		return "", "", errProductUpdateUnavailable
	}
	if err := os.Rename(temp, target); err != nil {
		return "", "", errProductUpdateUnavailable
	}
	return target, fileName, nil
}

func productUpdatePackageCacheDir(version string) string {
	return filepath.Join(config.GetDataDir(), "update-cache", version)
}

func productUpdateLockFor(version string) *sync.Mutex {
	productUpdatePackageMu.Lock()
	defer productUpdatePackageMu.Unlock()
	if productUpdatePackageLock[version] == nil {
		productUpdatePackageLock[version] = &sync.Mutex{}
	}
	return productUpdatePackageLock[version]
}

// rewriteProductUpdateManifest 把下载地址改成源站，摘要和签名原样留下。
// 客户站仍用这两项核对安装包，但请求不会再发到仓库。
func rewriteProductUpdateManifest(raw []byte) ([]byte, error) {
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	normalizeOnlineUpdateManifest(&manifest)
	version := strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
	if _, ok := parseOnlineUpdateVersion(version); !ok {
		return nil, errProductUpdateUnavailable
	}
	manifest.Package.URL = productUpdatePackagePrefix + version
	manifest.URL = manifest.Package.URL
	manifest.ReleasesURL = productUpdateReleasesURL
	manifest.Notes = filterProductUpdateNotes(manifest.Notes)
	body, err := json.Marshal(manifest)
	if err != nil || productUpdateBodyLeaks(body) {
		return nil, errProductUpdateUnavailable
	}
	return body, nil
}

func rewriteProductUpdateReleases(raw []byte) ([]byte, error) {
	var payload onlineUpdateReleases
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	for index := range payload.Releases {
		payload.Releases[index].Notes = filterProductUpdateNotes(payload.Releases[index].Notes)
	}
	body, err := json.Marshal(payload)
	if err != nil || productUpdateBodyLeaks(body) {
		return nil, errProductUpdateUnavailable
	}
	return body, nil
}

func filterProductUpdateNotes(notes []string) []string {
	kept := make([]string, 0, len(notes))
	for _, note := range notes {
		if productUpdateNoteVisible(note) {
			kept = append(kept, note)
		}
	}
	return kept
}

// productUpdateNoteVisible 丢掉提到托管站点的句子。
// 改写后的清单只给客户站，不能把仓库地址再写回去。
func productUpdateNoteVisible(note string) bool {
	lower := strings.ToLower(note)
	if strings.Contains(lower, "github.com") || strings.Contains(lower, "githubusercontent") {
		return false
	}
	if strings.Contains(lower, "github") && strings.Contains(lower, "auth-pro") {
		return false
	}
	return true
}

func productUpdateBodyLeaks(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "github.com") || strings.Contains(lower, "githubusercontent")
}

func productUpdateFetchReleaseAsset(ctx context.Context, releaseRef, assetName string) ([]byte, error) {
	owner, repo, err := productUpdateRepository()
	if err != nil {
		return nil, err
	}
	return fetchGitHubReleaseAsset(ctx, owner, repo, releaseRef, assetName)
}

// fetchGitHubReleaseAsset 用已保存的令牌读取某个 Release 附件。
// 分发客户包和官网自己更新都走这里，避免两套下载。仓库名由调用方传入。
func fetchGitHubReleaseAsset(ctx context.Context, owner, repo, releaseRef, assetName string) ([]byte, error) {
	if owner == "" || repo == "" || !safeOnlineUpdateAssetName(assetName) {
		return nil, errProductUpdateUnavailable
	}
	releaseURL := strings.TrimRight(productUpdateGitHubAPI, "/") + "/repos/" + owner + "/" + repo + "/releases/" + releaseRef
	tokens := productUpdateTokenCandidates()
	var lastErr error
	for _, token := range tokens {
		body, fetchErr := productUpdateFetch(ctx, releaseURL, token, "application/vnd.github+json")
		if fetchErr != nil {
			lastErr = fetchErr
			continue
		}
		assetURL, assetErr := productUpdateAssetAPIURL(body, assetName)
		if assetErr != nil {
			return nil, errProductUpdateUnavailable
		}
		payload, payloadErr := productUpdateFetch(ctx, assetURL, token, "application/octet-stream")
		if payloadErr != nil {
			lastErr = payloadErr
			continue
		}
		return payload, nil
	}
	if lastErr != nil {
		return nil, errProductUpdateUnavailable
	}
	return nil, errProductUpdateUnavailable
}

func productUpdateAssetAPIURL(body []byte, assetName string) (string, error) {
	var release struct {
		Assets []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	for _, asset := range release.Assets {
		if asset.Name != assetName || asset.ID <= 0 {
			continue
		}
		// 用 API 附件地址，由源站自己跟随跳转。页面上的下载地址不往外传。
		if strings.TrimSpace(asset.URL) != "" {
			return asset.URL, nil
		}
	}
	return "", errProductUpdateUnavailable
}

func productUpdateFetch(ctx context.Context, rawURL, token, accept string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errProductUpdateUnavailable
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	request.Header.Set("User-Agent", "auth-pro-source")
	client := productUpdateHTTPClient
	// 清单请求沿用 2 分钟。官网自己下安装包时上下文是 10 分钟，这里跟着放宽，避免大包被 2 分钟截断。
	if deadline, ok := ctx.Deadline(); ok && productUpdateHTTPClient.Timeout > 0 {
		if remaining := time.Until(deadline); remaining > productUpdateHTTPClient.Timeout {
			client = &http.Client{Timeout: remaining}
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errProductUpdateUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, errProductUpdateUnavailable
	}
	limit := int64(2 << 20)
	if accept == "application/octet-stream" {
		limit = maxOnlineUpdatePackageSize + 1
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil || int64(len(body)) > maxOnlineUpdatePackageSize {
		return nil, errProductUpdateUnavailable
	}
	return body, nil
}

func productUpdateRepository() (string, string, error) {
	raw := strings.Trim(strings.TrimSpace(os.Getenv(productUpdateRepoEnv)), "/")
	if raw == "" {
		// 官网默认从客户交付用的私有仓库取包，不从本仓库或官网仓库取。
		raw = productUpdateDefaultRepository
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || !sourceReleaseRepoPattern.MatchString(parts[0]) || !sourceReleaseRepoPattern.MatchString(parts[1]) {
		return "", "", errProductUpdateRepoMissing
	}
	return parts[0], parts[1], nil
}

// productUpdateTokenCandidates 先用收费仓库令牌，再用 Release 设置里的令牌，最后匿名。
// 默认仓库是私有的，没有令牌会失败。环境变量改到公开仓库时，匿名这一步仍能成功。
// 两处都没有令牌时只试匿名，不新建配置项。
func productUpdateTokenCandidates() []string {
	seen := map[string]struct{}{}
	list := make([]string, 0, 3)
	add := func(token string) {
		token = strings.TrimSpace(token)
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		list = append(list, token)
	}
	if token, err := loadGitHubPaidToken(); err == nil {
		add(token)
	}
	if settings, err := currentSourceStationStore().GetReleaseSettings(); err == nil {
		settings = normalizeReleaseSettings(settings)
		if settings.Provider == "github" {
			add(settings.Token)
		}
	}
	add("")
	return list
}

func resetProductUpdateStateForTest() {
	productUpdateCacheMu.Lock()
	productUpdateLatestCache.body = nil
	productUpdateLatestCache.expiresAt = time.Time{}
	productUpdateReleasesCache.body = nil
	productUpdateReleasesCache.expiresAt = time.Time{}
	productUpdateCacheMu.Unlock()
	productUpdateJSONLimiter = newProductUpdateRateLimiter(productUpdateJSONLimit, productUpdateRateWindow)
	productUpdatePackageLimiter = newProductUpdateRateLimiter(productUpdatePackageLimit, productUpdateRateWindow)
	productUpdateGitHubAPI = "https://api.github.com"
}
