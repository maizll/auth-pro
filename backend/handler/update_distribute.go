// 源站把「授权系统」应用的发布版本转成客户站认识的更新清单。
// 1.7.1 客户站只访问下面三个官网地址，响应格式不能改，也不能带仓库地址。
// 版本和安装包来自应用 app_f93896d80066_5811 的发布记录；下载地址始终写成官网自己的地址。
// 连接私有仓库只发生在后台「从仓库导入」，不在这个对外接口里。

package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
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
	// 客户站系统更新只认官网这个应用的发布版本。标识写死，避免指到别的应用。
	productUpdateAppKey        = "app_f93896d80066_5811"
	productUpdateUnavailable   = "暂时无法获取更新"
	productUpdateRepoMissing   = "更新仓库未配置"
	productUpdateRateLimited   = "请求过于频繁，请稍后再试"
	productUpdatePackagePrefix = "https://auth.maizll.com/api/v1/update/package/"
	productUpdateReleasesURL   = "https://auth.maizll.com/api/v1/update/releases.json"
)

var (
	errProductUpdateUnavailable = errors.New(productUpdateUnavailable)
	errProductUpdateRepoMissing = errors.New(productUpdateRepoMissing)

	// productUpdateGitHubAPI 正式程序是代码托管站的 API 根。测试换成 httptest。
	productUpdateGitHubAPI  = "https://api.github.com"
	productUpdateHTTPClient = &http.Client{Timeout: 2 * time.Minute}
)

// productUpdateFetchError 保留状态码和网络原因。对外仍显示「暂时无法获取更新」。
// 官网仓库迁移要区分 404 和 401/403/超时，不能把这些都当成同一种失败。
type productUpdateFetchError struct {
	status int
	cause  error
}

func (e *productUpdateFetchError) Error() string {
	return errProductUpdateUnavailable.Error()
}

func (e *productUpdateFetchError) Unwrap() error {
	return errProductUpdateUnavailable
}

func productUpdateHTTPStatus(err error) int {
	var fetchErr *productUpdateFetchError
	if errors.As(err, &fetchErr) {
		return fetchErr.status
	}
	return 0
}

var (
	productUpdateJSONLimiter    = newRateLimiter(productUpdateJSONLimit, productUpdateRateWindow)
	productUpdatePackageLimiter = newRateLimiter(productUpdatePackageLimit, productUpdateRateWindow)
	productUpdateCacheMu        sync.Mutex
	productUpdateLatestCache    struct {
		body []byte
		// forceBelow 是最新版本记录里的「最低版本」（强制更新阈值），按请求方版本单独套用，不写进缓存的清单
		forceBelow string
		expiresAt  time.Time
	}
	productUpdateReleasesCache struct {
		body      []byte
		expiresAt time.Time
	}
)

// productUpdateRecord 是一条已经发布、可以交给客户站的版本。
// SHA256 和大小以安装包文件为准，清单里的签名必须和文件对得上。
type productUpdateRecord struct {
	Version     string
	Title       string
	Changelog   string
	PublishedAt time.Time
	PackagePath string
	FileSize    int64
	SHA256      string
	Force       bool
	MinVersion  string
}

// loadProductUpdateRecords 读取授权系统应用的发布版本。测试可以换成固定数据。
var loadProductUpdateRecords = loadProductUpdateRecordsFromDB

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
	body, forceBelow, err := productUpdateLatestBody(c.Request.Context(), false)
	if err != nil {
		productUpdateFail(c, err)
		return
	}
	productUpdateJSON(c, productUpdateManifestForRequester(body, forceBelow, c.GetHeader("User-Agent")))
}

// productUpdateWritableFixVersion 起，客户站的更新脚本在 /www/backup 进不去时改用本站数据目录存备份。
// 更低版本跑的是它自己程序里的旧脚本，官网改不了，只能提示站长先在服务器上修一次权限。
const productUpdateWritableFixVersion = "1.8.9"

// productUpdateRequesterVersion 从请求头「auth_pro-updater/版本号」取出客户站版本。认不出时返回 false。
func productUpdateRequesterVersion(userAgent string) (string, bool) {
	current, ok := strings.CutPrefix(strings.TrimSpace(userAgent), "auth_pro-updater/")
	if !ok {
		return "", false
	}
	if fields := strings.Fields(current); len(fields) > 0 {
		current = strings.TrimPrefix(fields[0], "v")
	}
	if _, valid := parseOnlineUpdateVersion(current); !valid {
		return "", false
	}
	return current, true
}

// productUpdateWritableHint 是给旧客户站的一句说明，放在更新内容第一行。命令里的地址取官网自己的地址。
func productUpdateWritableHint() string {
	base := "https://auth.maizll.com"
	if parsed, err := url.Parse(productUpdatePackagePrefix); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		base = parsed.Scheme + "://" + parsed.Host
	}
	return "更新前请先看：如果点「立即更新」后提示「无法创建前端备份目录」，请用 root 登录服务器运行 curl -fsSL " +
		base + "/install.sh | bash -s -- --repair-update-perms 你的域名 ，再回来点「立即更新」。只需运行一次。"
}

// productUpdateManifestForRequester 按请求方版本改写共用清单，缓存里的那一份不动。
//   - 版本管理里的「最低版本」是强制更新阈值：请求方低于它时只把 force 标成 true。
//     不能写进 minVersion：客户站把 minVersion 当成「低于则不能直接升级」，会把该升级的老站拦住。
//   - 请求方低于 productUpdateWritableFixVersion 时，在更新内容最前面加一句修复在线更新权限的提示。
//
// 认不出请求方版本时清单原样返回。
func productUpdateManifestForRequester(body []byte, forceBelow, userAgent string) []byte {
	current, ok := productUpdateRequesterVersion(userAgent)
	if !ok {
		return body
	}
	needForce := false
	if forceBelow != "" {
		if comparison, valid := compareOnlineUpdateVersions(current, forceBelow); valid && comparison < 0 {
			needForce = true
		}
	}
	needHint := false
	if comparison, valid := compareOnlineUpdateVersions(current, productUpdateWritableFixVersion); valid && comparison < 0 {
		needHint = true
	}
	if !needForce && !needHint {
		return body
	}
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return body
	}
	changed := false
	if needForce && !manifest.Force {
		manifest.Force = true
		changed = true
	}
	if needHint && manifest.Version != "" {
		manifest.Notes = append([]string{productUpdateWritableHint()}, manifest.Notes...)
		changed = true
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(manifest)
	if err != nil {
		return body
	}
	return rewritten
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

func productUpdateAllow(c *gin.Context, limiter *rateLimiter) bool {
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

// productUpdateLatestBody 返回所有人相同的 latest.json 和最新记录的强制更新阈值。
func productUpdateLatestBody(_ context.Context, force bool) ([]byte, string, error) {
	productUpdateCacheMu.Lock()
	if !force && len(productUpdateLatestCache.body) > 0 && time.Now().Before(productUpdateLatestCache.expiresAt) {
		body := append([]byte(nil), productUpdateLatestCache.body...)
		forceBelow := productUpdateLatestCache.forceBelow
		productUpdateCacheMu.Unlock()
		return body, forceBelow, nil
	}
	productUpdateCacheMu.Unlock()

	records, err := loadProductUpdateRecords()
	if err != nil || len(records) == 0 {
		return nil, "", errProductUpdateUnavailable
	}
	latest := productUpdateHighest(records)
	body, err := buildProductUpdateManifest(latest)
	if err != nil {
		return nil, "", errProductUpdateUnavailable
	}
	forceBelow := strings.TrimPrefix(strings.TrimSpace(latest.MinVersion), "v")
	if _, ok := parseOnlineUpdateVersion(forceBelow); !ok {
		forceBelow = ""
	}
	productUpdateCacheMu.Lock()
	productUpdateLatestCache.body = append([]byte(nil), body...)
	productUpdateLatestCache.forceBelow = forceBelow
	productUpdateLatestCache.expiresAt = time.Now().Add(productUpdateManifestTTL)
	productUpdateCacheMu.Unlock()
	return body, forceBelow, nil
}

func productUpdateReleasesBody(_ context.Context, force bool) ([]byte, error) {
	productUpdateCacheMu.Lock()
	if !force && len(productUpdateReleasesCache.body) > 0 && time.Now().Before(productUpdateReleasesCache.expiresAt) {
		body := append([]byte(nil), productUpdateReleasesCache.body...)
		productUpdateCacheMu.Unlock()
		return body, nil
	}
	productUpdateCacheMu.Unlock()

	records, err := loadProductUpdateRecords()
	if err != nil || len(records) == 0 {
		return nil, errProductUpdateUnavailable
	}
	body, err := buildProductUpdateReleases(records)
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
	body, _, err := productUpdateLatestBody(ctx, false)
	if err != nil {
		return nil, err
	}
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, errProductUpdateUnavailable
	}
	return &manifest, nil
}

func productUpdatePackageFile(_ context.Context, version string) (string, string, error) {
	fileName, err := onlineUpdatePackageFileName(version)
	if err != nil {
		return "", "", err
	}
	records, err := loadProductUpdateRecords()
	if err != nil {
		return "", "", errProductUpdateUnavailable
	}
	record, ok := productUpdateFindVersion(records, version)
	if !ok {
		return "", "", errProductUpdateUnavailable
	}
	path := productUpdateResolvePackage(record.PackagePath)
	info, statErr := os.Stat(path)
	if statErr != nil || info.IsDir() || info.Size() <= 0 {
		return "", "", errProductUpdateUnavailable
	}
	return path, fileName, nil
}

// buildProductUpdateManifest 用发布记录生成 1.7.1 客户站认识的 latest.json。
// 下载地址固定写成官网，签名是 sha256: 加上安装包哈希，文件名带版本号。
// minVersion 不下发：记录里的「最低版本」是强制更新阈值，由 productUpdateManifestForRequester 按请求方版本换成 force。
func buildProductUpdateManifest(record productUpdateRecord) ([]byte, error) {
	version := strings.TrimPrefix(strings.TrimSpace(record.Version), "v")
	fileName, err := onlineUpdatePackageFileName(version)
	if err != nil {
		return nil, err
	}
	sum, size, err := productUpdateFileDigest(record)
	if err != nil {
		return nil, err
	}
	released := record.PublishedAt
	if released.IsZero() {
		released = time.Now()
	}
	manifest := onlineUpdateManifest{
		Version: version, Channel: "stable",
		Force: record.Force, ReleasedAt: released.UTC().Format(time.RFC3339),
		ReleasesURL: productUpdateReleasesURL,
		Package: onlineUpdatePackage{
			OS: "linux", Arch: "amd64", FileName: fileName,
			URL: productUpdatePackagePrefix + version, SHA256: sum, Size: size,
			Signature: "sha256:" + sum,
		},
		Actions: onlineUpdateActions{UpdateFrontend: true, UpdateBackend: true, RestartBackend: true, BackupDatabase: true},
		Notes:   productUpdateNotes(record),
	}
	manifest.URL = manifest.Package.URL
	manifest.SHA256 = sum
	manifest.Size = size
	body, err := json.Marshal(manifest)
	if err != nil || productUpdateBodyLeaks(body) {
		return nil, errProductUpdateUnavailable
	}
	return body, nil
}

func buildProductUpdateReleases(records []productUpdateRecord) ([]byte, error) {
	list := make([]onlineUpdateRelease, 0, len(records))
	for _, record := range records {
		version := strings.TrimPrefix(strings.TrimSpace(record.Version), "v")
		if _, ok := parseOnlineUpdateVersion(version); !ok {
			continue
		}
		released := record.PublishedAt
		if released.IsZero() {
			released = time.Now()
		}
		list = append(list, onlineUpdateRelease{
			Version: version, Channel: "stable",
			ReleasedAt: released.UTC().Format(time.RFC3339),
			Notes:      productUpdateNotes(record),
		})
	}
	if len(list) == 0 {
		return nil, errProductUpdateUnavailable
	}
	body, err := json.Marshal(onlineUpdateReleases{Releases: list})
	if err != nil || productUpdateBodyLeaks(body) {
		return nil, errProductUpdateUnavailable
	}
	return body, nil
}

func productUpdateNotes(record productUpdateRecord) []string {
	lines := make([]string, 0)
	if title := strings.TrimSpace(record.Title); title != "" {
		lines = append(lines, title)
	}
	for _, line := range strings.Split(record.Changelog, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return filterProductUpdateNotes(lines)
}

func productUpdateHighest(records []productUpdateRecord) productUpdateRecord {
	best := records[0]
	for _, record := range records[1:] {
		cmp, ok := compareOnlineUpdateVersions(record.Version, best.Version)
		if ok && cmp > 0 {
			best = record
		}
	}
	return best
}

func productUpdateFindVersion(records []productUpdateRecord, version string) (productUpdateRecord, bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	for _, record := range records {
		if strings.TrimPrefix(strings.TrimSpace(record.Version), "v") == version {
			return record, true
		}
	}
	return productUpdateRecord{}, false
}

func productUpdateResolvePackage(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return ""
	}
	if filepath.IsAbs(stored) {
		return stored
	}
	return filepath.Join(config.GetAppReleaseDir(), filepath.FromSlash(stored))
}

// productUpdateFileDigest 用安装包文件的真实哈希和大小生成清单。
// 后台表单里的 MD5 可以改，客户站核对的是这份 SHA256，所以不能用改过的数字代替文件。
func productUpdateFileDigest(record productUpdateRecord) (string, int64, error) {
	path := productUpdateResolvePackage(record.PackagePath)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > maxOnlineUpdatePackageSize {
		return "", 0, errProductUpdateUnavailable
	}
	if isHexSHA256(record.SHA256) && record.FileSize == info.Size() {
		return strings.ToLower(record.SHA256), info.Size(), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, errProductUpdateUnavailable
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil || written != info.Size() {
		return "", 0, errProductUpdateUnavailable
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

// loadProductUpdateRecordsFromDB 只取「授权系统」应用已发布的版本。
// 没有这个应用、还没发版本或读库失败，对外都说暂时无法获取更新，不暴露内部原因。
func loadProductUpdateRecordsFromDB() ([]productUpdateRecord, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := EnsureAppVersionsTable(db); err != nil {
		return nil, err
	}
	var appID int64
	err = db.QueryRow(`SELECT id FROM apps WHERE app_key = ? LIMIT 1`, productUpdateAppKey).Scan(&appID)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT version, title, changelog, published_at, package_path, file_size_bytes, file_sha256, force_update, min_version
		FROM app_versions WHERE app_id = ? ORDER BY published_at DESC, id DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]productUpdateRecord, 0)
	for rows.Next() {
		var record productUpdateRecord
		var published sql.NullTime
		var force int
		if err := rows.Scan(&record.Version, &record.Title, &record.Changelog, &published, &record.PackagePath, &record.FileSize, &record.SHA256, &force, &record.MinVersion); err != nil {
			return nil, err
		}
		if published.Valid {
			record.PublishedAt = published.Time
		}
		record.Force = force == 1
		list = append(list, record)
	}
	return list, rows.Err()
}

// invalidateProductUpdateCache 发布或修改授权系统版本后清掉清单缓存，客户站不用等缓存过期。
func invalidateProductUpdateCache() {
	productUpdateCacheMu.Lock()
	productUpdateLatestCache.body = nil
	productUpdateLatestCache.expiresAt = time.Time{}
	productUpdateReleasesCache.body = nil
	productUpdateReleasesCache.expiresAt = time.Time{}
	productUpdateCacheMu.Unlock()
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

// fetchGitHubReleaseAsset 用已保存的令牌读取某个 Release 附件。
// 分发客户包和官网自己更新都走 fetchGitHubReleaseAssetWith，避免两套下载。仓库名由调用方传入。
func fetchGitHubReleaseAsset(ctx context.Context, owner, repo, releaseRef, assetName string) ([]byte, error) {
	payload, _, err := fetchGitHubReleaseAssetWith(ctx, owner, repo, releaseRef, assetName, productUpdateTokenCandidates())
	return payload, err
}

// fetchGitHubReleaseAssetWith 按给定顺序逐个令牌尝试，返回成功时用的是第几个令牌。
// 全部失败时返回最后一次的 productUpdateFetchError，调用方可以用 productUpdateHTTPStatus 区分 401、404 和断网。
func fetchGitHubReleaseAssetWith(ctx context.Context, owner, repo, releaseRef, assetName string, tokens []string) ([]byte, int, error) {
	if owner == "" || repo == "" || !safeOnlineUpdateAssetName(assetName) || len(tokens) == 0 {
		return nil, -1, errProductUpdateUnavailable
	}
	releaseURL := strings.TrimRight(productUpdateGitHubAPI, "/") + "/repos/" + owner + "/" + repo + "/releases/" + releaseRef
	var lastErr error = errProductUpdateUnavailable
	for index, token := range tokens {
		body, fetchErr := productUpdateFetch(ctx, releaseURL, token, "application/vnd.github+json")
		if fetchErr != nil {
			lastErr = fetchErr
			continue
		}
		assetURL, assetErr := productUpdateAssetAPIURL(body, assetName)
		if assetErr != nil {
			return nil, index, errProductUpdateUnavailable
		}
		payload, payloadErr := productUpdateFetch(ctx, assetURL, token, "application/octet-stream")
		if payloadErr != nil {
			lastErr = payloadErr
			continue
		}
		return payload, index, nil
	}
	return nil, -1, lastErr
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
		return nil, &productUpdateFetchError{cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, &productUpdateFetchError{status: response.StatusCode}
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
		// 导入和上传不再使用写死的客户仓库。没配置就按未配置处理。
		return "", "", errProductUpdateRepoMissing
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
// productUpdateTokenSource 是读取发布仓库时要依次尝试的一个令牌，Label 是给站长看的来源说明。
type productUpdateTokenSource struct {
	Token string
	Label string
}

// productUpdateTokenSources 按固定顺序列出令牌：软件源设置里的收费仓库令牌、存储管理里的 GitHub 令牌、
// Release 设置里的令牌，最后匿名。官网自己更新、分发客户包、仓库导入都用这一个顺序。
func productUpdateTokenSources() []productUpdateTokenSource {
	seen := map[string]struct{}{}
	list := make([]productUpdateTokenSource, 0, 4)
	add := func(token, label string) {
		token = strings.TrimSpace(token)
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		list = append(list, productUpdateTokenSource{Token: token, Label: label})
	}
	if token, err := loadGitHubPaidToken(); err == nil {
		add(token, "收费仓库设置里的令牌")
	}
	// 存储管理里的 GitHub 令牌。从仓库导入列出 Release 时和旧的收费仓库令牌是同一批凭证。
	if blob, blobErr := loadStorageBlob(); blobErr == nil {
		for _, loc := range enabledStorageLocations(blob.Locations) {
			if loc.Kind != packageStorageGitHub {
				continue
			}
			secret, secretErr := locationSecret(loc)
			if secretErr == nil {
				add(secret, "存储「"+loc.Name+"」的令牌")
			}
		}
	}
	if settings, err := currentSourceStationStore().GetReleaseSettings(); err == nil {
		settings = normalizeReleaseSettings(settings)
		if settings.Provider == "github" {
			add(settings.Token, "Release 设置里的令牌")
		}
	}
	add("", "匿名读取")
	return list
}

func productUpdateTokenCandidates() []string {
	sources := productUpdateTokenSources()
	tokens := make([]string, len(sources))
	for index, source := range sources {
		tokens[index] = source.Token
	}
	return tokens
}

func resetProductUpdateStateForTest() {
	productUpdateCacheMu.Lock()
	productUpdateLatestCache.body = nil
	productUpdateLatestCache.expiresAt = time.Time{}
	productUpdateReleasesCache.body = nil
	productUpdateReleasesCache.expiresAt = time.Time{}
	productUpdateCacheMu.Unlock()
	productUpdateJSONLimiter = newRateLimiter(productUpdateJSONLimit, productUpdateRateWindow)
	productUpdatePackageLimiter = newRateLimiter(productUpdatePackageLimit, productUpdateRateWindow)
	productUpdateGitHubAPI = "https://api.github.com"
	loadProductUpdateRecords = loadProductUpdateRecordsFromDB
}
