package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	maxOnlineUpdatePackageSize  = int64(512 << 20)
	githubUpdateRepositoryPath  = "/maizll/auth-pro/releases/"
	githubUpdateAPIReleasesPath = "/repos/maizll/auth-pro/releases"
)

var onlineUpdateVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
var onlineUpdateDigestPattern = regexp.MustCompile(`(?i)^sha256:[a-f0-9]{64}$`)

type onlineUpdatePackage struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	FileName  string `json:"fileName"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

type onlineUpdateActions struct {
	UpdateFrontend bool `json:"updateFrontend"`
	UpdateBackend  bool `json:"updateBackend"`
	RestartBackend bool `json:"restartBackend"`
	BackupDatabase bool `json:"backupDatabase"`
}

type onlineUpdateManifest struct {
	Version     string              `json:"version"`
	Channel     string              `json:"channel"`
	MinVersion  string              `json:"minVersion"`
	Force       bool                `json:"force"`
	ReleasedAt  string              `json:"releasedAt"`
	ReleasesURL string              `json:"releasesUrl"`
	Package     onlineUpdatePackage `json:"package"`
	Actions     onlineUpdateActions `json:"actions"`
	Notes       []string            `json:"notes"`

	// 兼容极简版 latest.json。
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type onlineUpdateRelease struct {
	Version    string   `json:"version"`
	Channel    string   `json:"channel"`
	ReleasedAt string   `json:"releasedAt"`
	Notes      []string `json:"notes"`
}

type onlineUpdateReleases struct {
	Releases []onlineUpdateRelease `json:"releases"`
}

type cachedOnlineUpdateReleases struct {
	URL       string
	Releases  []onlineUpdateRelease
	ExpiresAt time.Time
}

type onlineUpdateJob struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Progress  int       `json:"progress"`
	Version   string    `json:"version"`
	Logs      []string  `json:"logs"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type onlineUpdateStore struct {
	mu        sync.Mutex
	jobs      map[string]*onlineUpdateJob
	runningID string
	latest    *onlineUpdateManifest
}

var updateStore = &onlineUpdateStore{jobs: make(map[string]*onlineUpdateJob)}

// 应用阶段的下载和摘要查询可以在测试里替换。默认仍走真实下载，
// 并用钉死的 GitHub Release API 核对附件 digest。
var (
	downloadOnlineUpdatePackageForApply = downloadOnlineUpdatePackage
	fetchOnlineUpdateAssetDigest        = fetchGitHubReleaseAssetDigest
)

var updateReleasesCache struct {
	mu    sync.Mutex
	value cachedOnlineUpdateReleases
}

// SystemVersion 返回当前系统整体版本。健康检查脚本也会访问它。
func SystemVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "",
		"data": gin.H{
			"version":   config.AppVersion,
			"buildTime": config.BuildTime,
			"os":        runtime.GOOS,
			"arch":      runtime.GOARCH,
		},
	})
}

// AdminOnlineUpdateStatus 返回当前版本、更新地址、最近一次清单和任务状态。
func AdminOnlineUpdateStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "",
		"data": gin.H{
			"currentVersion": config.AppVersion,
			"buildTime":      config.BuildTime,
			"updateUrl":      config.GetUpdateManifestURL(),
			"frontendDir":    config.GetFrontendDir(),
			"serviceName":    config.GetServiceName(),
			"latest":         cachedOnlineUpdateManifest(),
			"runningJob":     runningOnlineUpdateJob(),
		},
	})
}

// AdminOnlineUpdateCheck 拉取 latest.json 并检查版本和更新包元数据。
func AdminOnlineUpdateCheck(c *gin.Context) {
	manifest, err := fetchOnlineUpdateManifest()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "检查更新失败：" + err.Error()})
		return
	}

	setCachedOnlineUpdateManifest(manifest)
	available, versionErr, packageErr, canApply := evaluateOnlineUpdateCheck(config.AppVersion, manifest)

	data := gin.H{
		"currentVersion": config.AppVersion,
		"latest":         manifest,
		"updateUrl":      config.GetUpdateManifestURL(),
		"canApply":       canApply,
		"packageValid":   packageErr == nil,
		"packageError":   errorText(packageErr),
		"versionError":   versionErr,
	}
	if available {
		data["updateAvailable"] = true
	} else {
		data["updateAvailable"] = false
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": data})
}

// AdminOnlineUpdateHistory 返回只读历史版本和每版更新日志。
func AdminOnlineUpdateHistory(c *gin.Context) {
	manifest := cachedOnlineUpdateManifest()
	if manifest == nil {
		var err error
		manifest, err = fetchOnlineUpdateManifest()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取历史版本失败：" + err.Error()})
			return
		}
		setCachedOnlineUpdateManifest(manifest)
	}

	releases, releasesURL, err := fetchOnlineUpdateReleases(manifest, c.Query("refresh") == "1")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取历史版本失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "",
		"data": gin.H{
			"currentVersion": config.AppVersion,
			"releasesUrl":    releasesURL,
			"releases":       releases,
		},
	})
}

// AdminOnlineUpdateApply 创建一键整包更新任务。
func AdminOnlineUpdateApply(c *gin.Context) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "一键整包更新仅支持 Linux amd64 宝塔部署环境"})
		return
	}

	manifest, err := fetchOnlineUpdateManifest()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取更新清单失败：" + err.Error()})
		return
	}
	if err := validateOnlineUpdateManifest(manifest); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "更新包信息不完整：" + err.Error()})
		return
	}
	available, versionErr := onlineUpdateAvailable(config.AppVersion, manifest)
	if versionErr != "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": versionErr})
		return
	}
	if !available {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "当前已经是最新版本"})
		return
	}
	if !reserveOnlineUpdateJob() {
		c.JSON(http.StatusOK, gin.H{"code": 409, "msg": "已有更新任务正在执行"})
		return
	}

	job := createOnlineUpdateJob(manifest.Version)
	appendOnlineUpdateLog(job.ID, "更新任务已创建")
	go runOnlineUpdateJob(job.ID, *manifest)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "更新任务已启动，服务即将重启，请稍后刷新页面",
		"data": snapshotOnlineUpdateJob(job.ID),
	})
}

// AdminOnlineUpdateJob 查询更新任务状态。
func AdminOnlineUpdateJob(c *gin.Context) {
	job := snapshotOnlineUpdateJob(c.Param("id"))
	if job == nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "更新任务不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": job})
}

func parseOnlineUpdateURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" || parsed.User != nil {
		return nil, errors.New("更新地址必须是有效的 HTTPS 地址")
	}
	if isGitHubUpdateHost(parsed.Hostname()) {
		if parsed.Port() != "" && parsed.Port() != "443" {
			return nil, errors.New("GitHub 更新地址只能使用 HTTPS 默认端口")
		}
		if !isGitHubRepositoryReleaseURL(parsed) {
			return nil, githubTrustError(parsed)
		}
	}
	if strings.EqualFold(parsed.Hostname(), "gitee.com") {
		if parsed.Port() != "" && parsed.Port() != "443" {
			return nil, errors.New("Gitee 更新地址只能使用 HTTPS 默认端口")
		}
		if !isGiteeRepositoryUpdateURL(parsed) {
			return nil, errors.New("Gitee 更新地址不属于受信任的发布仓库")
		}
	}
	return parsed, nil
}

func isGitHubUpdateHost(hostname string) bool {
	return strings.EqualFold(hostname, "github.com") || strings.EqualFold(hostname, "api.github.com")
}

func githubTrustError(parsed *url.URL) error {
	if parsed != nil && strings.EqualFold(parsed.Hostname(), "api.github.com") {
		return errors.New("GitHub API 更新地址不属于受信任的发布仓库")
	}
	return errors.New("GitHub 更新地址不属于受信任的发布仓库")
}

func isGitHubRepositoryReleaseURL(parsed *url.URL) bool {
	return isGitHubWebsiteReleaseURL(parsed) || isGitHubAPIReleaseURL(parsed)
}

func isGitHubWebsiteReleaseURL(parsed *url.URL) bool {
	return parsed != nil && strings.EqualFold(parsed.Hostname(), "github.com") &&
		strings.HasPrefix(strings.ToLower(parsed.EscapedPath()), githubUpdateRepositoryPath)
}

func isGitHubAPIReleaseURL(parsed *url.URL) bool {
	if parsed == nil || !strings.EqualFold(parsed.Hostname(), "api.github.com") {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	return path == githubUpdateAPIReleasesPath || strings.HasPrefix(path, githubUpdateAPIReleasesPath+"/")
}

func isGitHubLatestReleaseAPIURL(parsed *url.URL) bool {
	return isGitHubAPIReleaseURL(parsed) &&
		strings.TrimSuffix(strings.ToLower(parsed.EscapedPath()), "/") == githubUpdateAPIReleasesPath+"/latest"
}

func isGitHubReleaseAssetHost(hostname string) bool {
	return strings.EqualFold(hostname, "release-assets.githubusercontent.com")
}

// Gitee 不是默认更新源。只有 AUTO_PRO_UPDATE_URL 显式指向某个 Gitee 仓库的
// Release、附件或 API 地址时，才信任同一 owner/repo 的 HTTPS 地址。
type giteeRepository struct {
	owner string
	repo  string
}

func (repo giteeRepository) matches(other giteeRepository) bool {
	return strings.EqualFold(repo.owner, other.owner) && strings.EqualFold(repo.repo, other.repo)
}

func configuredGiteeRepository() (giteeRepository, bool) {
	parsed, err := url.Parse(strings.TrimSpace(config.GetUpdateManifestURL()))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" {
		return giteeRepository{}, false
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return giteeRepository{}, false
	}
	return giteeRepositoryFromURL(parsed)
}

func giteeUpdateSegments(parsed *url.URL) []string {
	if parsed == nil {
		return nil
	}
	raw := parsed.EscapedPath()
	if decoded, err := url.PathUnescape(raw); err == nil {
		raw = decoded
	}
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil
		}
	}
	return parts
}

func giteeRepositoryFromURL(parsed *url.URL) (giteeRepository, bool) {
	if parsed == nil || !strings.EqualFold(parsed.Hostname(), "gitee.com") {
		return giteeRepository{}, false
	}
	parts := giteeUpdateSegments(parsed)
	if len(parts) >= 6 &&
		strings.EqualFold(parts[0], "api") &&
		strings.EqualFold(parts[1], "v5") &&
		strings.EqualFold(parts[2], "repos") &&
		strings.EqualFold(parts[5], "releases") {
		return giteeRepository{owner: parts[3], repo: parts[4]}, true
	}
	if len(parts) >= 3 && (strings.EqualFold(parts[2], "releases") || strings.EqualFold(parts[2], "attach_files")) {
		return giteeRepository{owner: parts[0], repo: parts[1]}, true
	}
	return giteeRepository{}, false
}

func matchingConfiguredGitee(parsed *url.URL) ([]string, bool) {
	repo, ok := giteeRepositoryFromURL(parsed)
	if !ok {
		return nil, false
	}
	configured, allowed := configuredGiteeRepository()
	if !allowed || !repo.matches(configured) {
		return nil, false
	}
	return giteeUpdateSegments(parsed), true
}

func isGiteeRepositoryReleaseURL(parsed *url.URL) bool {
	parts, ok := matchingConfiguredGitee(parsed)
	return ok && len(parts) >= 3 && strings.EqualFold(parts[2], "releases")
}

func isGiteeRepositoryAttachmentURL(parsed *url.URL) bool {
	parts, ok := matchingConfiguredGitee(parsed)
	return ok && len(parts) >= 3 && strings.EqualFold(parts[2], "attach_files")
}

func isGiteeRepositoryAPIURL(parsed *url.URL) bool {
	parts, ok := matchingConfiguredGitee(parsed)
	return ok && len(parts) >= 6 &&
		strings.EqualFold(parts[0], "api") &&
		strings.EqualFold(parts[1], "v5") &&
		strings.EqualFold(parts[2], "repos") &&
		strings.EqualFold(parts[5], "releases")
}

func isGiteeLatestReleaseAPIURL(parsed *url.URL) bool {
	if !isGiteeRepositoryAPIURL(parsed) {
		return false
	}
	parts := giteeUpdateSegments(parsed)
	return len(parts) == 7 && strings.EqualFold(parts[6], "latest")
}

func isGiteeRepositoryUpdateURL(parsed *url.URL) bool {
	return isGiteeRepositoryReleaseURL(parsed) || isGiteeRepositoryAttachmentURL(parsed) || isGiteeRepositoryAPIURL(parsed)
}

func isGiteeReleaseAssetHost(hostname string) bool {
	return strings.EqualFold(hostname, "foruda.gitee.com")
}

func newOnlineUpdateHTTPClient(rawURL string, timeout time.Duration) (*http.Client, error) {
	initialURL, err := parseOnlineUpdateURL(rawURL)
	if err != nil {
		return nil, err
	}
	githubRelease := isGitHubRepositoryReleaseURL(initialURL)
	giteeRelease := isGiteeRepositoryUpdateURL(initialURL)
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("更新地址重定向次数过多")
			}
			if req.URL.Scheme != "https" || req.URL.User != nil {
				return errors.New("更新地址重定向到非 HTTPS 地址")
			}
			if githubRelease {
				if req.URL.Port() != "" && req.URL.Port() != "443" {
					return errors.New("GitHub 更新地址重定向到非 HTTPS 默认端口")
				}
				if isGitHubRepositoryReleaseURL(req.URL) || isGitHubReleaseAssetHost(req.URL.Hostname()) {
					return nil
				}
				return errors.New("GitHub 更新地址重定向到非受信任域名")
			}
			if giteeRelease {
				if req.URL.Port() != "" && req.URL.Port() != "443" {
					return errors.New("Gitee 更新地址重定向到非 HTTPS 默认端口")
				}
				if isGiteeRepositoryUpdateURL(req.URL) || isGiteeReleaseAssetHost(req.URL.Hostname()) {
					return nil
				}
				return errors.New("Gitee 更新地址重定向到非受信任域名")
			}
			if !strings.EqualFold(req.URL.Host, initialURL.Host) {
				return errors.New("更新地址重定向到非同源地址")
			}
			return nil
		},
	}
	return client, nil
}

func fetchOnlineUpdateManifest() (*onlineUpdateManifest, error) {
	manifestURL := strings.TrimSpace(config.GetUpdateManifestURL())
	parsedManifestURL, err := parseOnlineUpdateURL(manifestURL)
	if err != nil {
		return nil, errors.New("更新清单地址格式不正确：" + err.Error())
	}
	if isGiteeLatestReleaseAPIURL(parsedManifestURL) {
		manifestURL, err = fetchGiteeLatestManifestURL(manifestURL)
		if err != nil {
			return nil, err
		}
	}
	if isGitHubLatestReleaseAPIURL(parsedManifestURL) {
		manifestURL, err = fetchGitHubLatestManifestURL(manifestURL)
		if err != nil {
			return nil, err
		}
	}

	client, err := newOnlineUpdateHTTPClient(manifestURL, 15*time.Second)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接更新服务器失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新服务器返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, errors.New("读取更新清单失败")
	}
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, errors.New("更新清单不是有效的 JSON")
	}
	normalizeOnlineUpdateManifest(&manifest)
	return &manifest, nil
}

func fetchGitHubLatestManifestURL(releaseURL string) (string, error) {
	client, err := newOnlineUpdateHTTPClient(releaseURL, 15*time.Second)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("连接 GitHub 更新服务器失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub 最新发行版接口返回状态码 %d", response.StatusCode)
	}

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return "", errors.New("GitHub 最新发行版数据格式不正确")
	}
	for _, asset := range release.Assets {
		if asset.Name != "latest.json" {
			continue
		}
		parsedAssetURL, parseErr := parseOnlineUpdateURL(asset.BrowserDownloadURL)
		if parseErr != nil || !isGitHubWebsiteReleaseURL(parsedAssetURL) {
			return "", errors.New("GitHub latest.json 附件地址不受信任")
		}
		return asset.BrowserDownloadURL, nil
	}
	return "", errors.New("GitHub 最新发行版缺少 latest.json 附件")
}

func fetchGiteeLatestManifestURL(releaseURL string) (string, error) {
	client, err := newOnlineUpdateHTTPClient(releaseURL, 15*time.Second)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("连接 Gitee 更新服务器失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gitee 最新发行版接口返回状态码 %d", response.StatusCode)
	}
	var release struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil || release.ID <= 0 {
		return "", errors.New("Gitee 最新发行版数据格式不正确")
	}

	parsedReleaseURL, err := url.Parse(releaseURL)
	if err != nil {
		return "", errors.New("Gitee 更新地址格式不正确")
	}
	repo, ok := giteeRepositoryFromURL(parsedReleaseURL)
	if !ok || parsedReleaseURL.Scheme == "" || parsedReleaseURL.Host == "" {
		return "", errors.New("Gitee 更新地址不属于受信任的发布仓库")
	}
	attachmentsURL := fmt.Sprintf("%s://%s/api/v5/repos/%s/%s/releases/%d/attach_files",
		parsedReleaseURL.Scheme, parsedReleaseURL.Host,
		url.PathEscape(repo.owner), url.PathEscape(repo.repo), release.ID)
	attachmentsClient, err := newOnlineUpdateHTTPClient(attachmentsURL, 15*time.Second)
	if err != nil {
		return "", err
	}
	attachmentsRequest, err := http.NewRequest(http.MethodGet, attachmentsURL, nil)
	if err != nil {
		return "", err
	}
	attachmentsRequest.Header.Set("Cache-Control", "no-cache")
	attachmentsRequest.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)
	attachmentsResponse, err := attachmentsClient.Do(attachmentsRequest)
	if err != nil {
		return "", fmt.Errorf("读取 Gitee 发行版附件失败：%w", err)
	}
	defer attachmentsResponse.Body.Close()
	if attachmentsResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gitee 发行版附件接口返回状态码 %d", attachmentsResponse.StatusCode)
	}
	var attachments []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}
	if err := json.NewDecoder(io.LimitReader(attachmentsResponse.Body, 1<<20)).Decode(&attachments); err != nil {
		return "", errors.New("Gitee 发行版附件数据格式不正确")
	}
	for _, attachment := range attachments {
		if attachment.Name != "latest.json" {
			continue
		}
		parsedAttachmentURL, err := parseOnlineUpdateURL(attachment.BrowserDownloadURL)
		if err != nil || !isGiteeRepositoryReleaseURL(parsedAttachmentURL) {
			return "", errors.New("Gitee latest.json 附件地址不受信任")
		}
		return attachment.BrowserDownloadURL, nil
	}
	return "", errors.New("Gitee 最新发行版缺少 latest.json 附件")
}

func fetchOnlineUpdateReleases(manifest *onlineUpdateManifest, forceRefresh bool) ([]onlineUpdateRelease, string, error) {
	releasesURL, err := resolveOnlineUpdateReleasesURL(manifest)
	if err != nil {
		return nil, "", err
	}

	updateReleasesCache.mu.Lock()
	cached := updateReleasesCache.value
	if !forceRefresh && cached.URL == releasesURL && time.Now().Before(cached.ExpiresAt) {
		releases := cloneOnlineUpdateReleases(cached.Releases)
		updateReleasesCache.mu.Unlock()
		return releases, releasesURL, nil
	}
	updateReleasesCache.mu.Unlock()

	client, err := newOnlineUpdateHTTPClient(releasesURL, 10*time.Second)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest(http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("连接更新服务器失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("更新服务器返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, "", errors.New("读取历史版本清单失败")
	}
	if len(body) > 1<<20 {
		return nil, "", errors.New("历史版本清单超过 1MB 限制")
	}
	var payload onlineUpdateReleases
	if err := decodeOnlineUpdateReleases(body, releasesURL, &payload); err != nil {
		return nil, "", err
	}
	if err := normalizeOnlineUpdateReleases(&payload); err != nil {
		return nil, "", err
	}

	updateReleasesCache.mu.Lock()
	updateReleasesCache.value = cachedOnlineUpdateReleases{
		URL:       releasesURL,
		Releases:  cloneOnlineUpdateReleases(payload.Releases),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	updateReleasesCache.mu.Unlock()
	return payload.Releases, releasesURL, nil
}

func resolveOnlineUpdateReleasesURL(manifest *onlineUpdateManifest) (string, error) {
	manifestURL, err := parseOnlineUpdateURL(config.GetUpdateManifestURL())
	if err != nil {
		return "", errors.New("更新清单地址格式不正确：" + err.Error())
	}

	releasesRef := strings.TrimSpace(manifest.ReleasesURL)
	if releasesRef == "" {
		if isGitHubAPIReleaseURL(manifestURL) {
			releasesRef = githubAPIReleasesListURL(manifestURL)
		} else {
			releasesRef = "releases.json"
		}
	}
	parsedRef, err := url.Parse(releasesRef)
	if err != nil {
		return "", errors.New("历史版本清单地址格式不正确")
	}
	releasesURL := manifestURL.ResolveReference(parsedRef)
	if _, err := parseOnlineUpdateURL(releasesURL.String()); err != nil {
		return "", errors.New("历史版本清单地址格式不正确：" + err.Error())
	}
	if !onlineUpdateURLsSameTrustOrigin(manifestURL, releasesURL) {
		return "", errors.New("历史版本清单必须与更新清单同源")
	}
	return releasesURL.String(), nil
}

func githubAPIReleasesListURL(manifestURL *url.URL) string {
	return manifestURL.Scheme + "://" + manifestURL.Host + githubUpdateAPIReleasesPath
}

func onlineUpdateURLsSameTrustOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || left.Scheme != right.Scheme {
		return false
	}
	if strings.EqualFold(left.Host, right.Host) {
		return true
	}
	return isGitHubRepositoryReleaseURL(left) && isGitHubRepositoryReleaseURL(right)
}

func decodeOnlineUpdateReleases(body []byte, releasesURL string, payload *onlineUpdateReleases) error {
	trimmed := bytes.TrimSpace(body)
	parsedReleasesURL, _ := url.Parse(releasesURL)
	if isGitHubAPIReleaseURL(parsedReleasesURL) && !isAppReleasesJSON(trimmed) {
		mapped, err := mapGitHubAPIReleases(trimmed)
		if err != nil {
			return err
		}
		*payload = *mapped
		return nil
	}
	if err := json.Unmarshal(trimmed, payload); err != nil {
		return errors.New("历史版本清单不是有效的 JSON")
	}
	return nil
}

func isAppReleasesJSON(body []byte) bool {
	var probe struct {
		Releases json.RawMessage `json:"releases"`
	}
	return json.Unmarshal(body, &probe) == nil && len(bytes.TrimSpace(probe.Releases)) > 0
}

type githubReleaseAPIItem struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

func mapGitHubAPIReleases(body []byte) (*onlineUpdateReleases, error) {
	var items []githubReleaseAPIItem
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var item githubReleaseAPIItem
		if err := json.Unmarshal(trimmed, &item); err != nil {
			return nil, errors.New("GitHub 历史版本数据格式不正确")
		}
		items = []githubReleaseAPIItem{item}
	} else if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, errors.New("GitHub 历史版本数据格式不正确")
	}

	payload := &onlineUpdateReleases{Releases: make([]onlineUpdateRelease, 0, len(items))}
	for _, item := range items {
		if item.Draft {
			continue
		}
		version := normalizeGitHubReleaseVersion(item.TagName)
		if _, ok := parseOnlineUpdateVersion(version); !ok {
			continue
		}
		channel := "stable"
		if item.Prerelease {
			channel = "beta"
		}
		payload.Releases = append(payload.Releases, onlineUpdateRelease{
			Version:    version,
			Channel:    channel,
			ReleasedAt: strings.TrimSpace(item.PublishedAt),
			Notes:      notesFromGitHubReleaseBody(item.Body),
		})
	}
	return payload, nil
}

func normalizeGitHubReleaseVersion(tag string) string {
	version := strings.TrimSpace(tag)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return version
}

func notesFromGitHubReleaseBody(body string) []string {
	var bullets []string
	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if note, ok := stripMarkdownListMarker(line); ok {
			if note != "" {
				bullets = append(bullets, note)
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if len(bullets) > 0 {
		return bullets
	}
	return lines
}

func stripMarkdownListMarker(line string) (string, bool) {
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(line[len(prefix):]), true
		}
	}
	return line, false
}

func normalizeOnlineUpdateReleases(payload *onlineUpdateReleases) error {
	if len(payload.Releases) > 200 {
		return errors.New("历史版本数量超过 200 条限制")
	}

	seen := make(map[string]struct{}, len(payload.Releases))
	normalized := make([]onlineUpdateRelease, 0, len(payload.Releases))
	for index := range payload.Releases {
		release := payload.Releases[index]
		release.Version = strings.TrimSpace(release.Version)
		release.Channel = strings.TrimSpace(release.Channel)
		release.ReleasedAt = strings.TrimSpace(release.ReleasedAt)
		if _, ok := parseOnlineUpdateVersion(release.Version); !ok {
			return fmt.Errorf("历史版本第 %d 条版本号格式不正确", index+1)
		}
		if release.ReleasedAt != "" {
			if _, err := time.Parse(time.RFC3339, release.ReleasedAt); err != nil {
				return fmt.Errorf("历史版本 %s 发布时间格式不正确", release.Version)
			}
		}
		if release.Channel == "" {
			release.Channel = "stable"
		}
		if _, exists := seen[release.Version]; exists {
			continue
		}
		seen[release.Version] = struct{}{}
		notes := make([]string, 0, len(release.Notes))
		for _, note := range release.Notes {
			note = strings.TrimSpace(note)
			if note != "" {
				notes = append(notes, note)
			}
		}
		release.Notes = notes
		normalized = append(normalized, release)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		comparison, ok := compareOnlineUpdateVersions(normalized[i].Version, normalized[j].Version)
		return ok && comparison > 0
	})
	payload.Releases = normalized
	return nil
}

func cloneOnlineUpdateReleases(releases []onlineUpdateRelease) []onlineUpdateRelease {
	cloned := make([]onlineUpdateRelease, len(releases))
	for index, release := range releases {
		cloned[index] = release
		cloned[index].Notes = append([]string(nil), release.Notes...)
	}
	return cloned
}

func normalizeOnlineUpdateManifest(manifest *onlineUpdateManifest) {
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.Channel = strings.TrimSpace(manifest.Channel)
	manifest.MinVersion = strings.TrimSpace(manifest.MinVersion)
	manifest.ReleasesURL = strings.TrimSpace(manifest.ReleasesURL)
	manifest.Package.OS = strings.ToLower(strings.TrimSpace(manifest.Package.OS))
	manifest.Package.Arch = strings.ToLower(strings.TrimSpace(manifest.Package.Arch))
	manifest.Package.FileName = strings.TrimSpace(manifest.Package.FileName)
	manifest.Package.URL = strings.TrimSpace(manifest.Package.URL)
	manifest.Package.SHA256 = strings.ToLower(strings.TrimSpace(manifest.Package.SHA256))
	manifest.Package.Signature = strings.TrimSpace(manifest.Package.Signature)

	if manifest.Package.URL == "" {
		manifest.Package.URL = strings.TrimSpace(manifest.URL)
	}
	if manifest.Package.SHA256 == "" {
		manifest.Package.SHA256 = strings.ToLower(strings.TrimSpace(manifest.SHA256))
	}
	if manifest.Package.Size == 0 {
		manifest.Package.Size = manifest.Size
	}
	if manifest.Channel == "" {
		manifest.Channel = "stable"
	}
	if !manifest.Actions.UpdateFrontend && !manifest.Actions.UpdateBackend &&
		!manifest.Actions.RestartBackend && !manifest.Actions.BackupDatabase {
		manifest.Actions = onlineUpdateActions{
			UpdateFrontend: true,
			UpdateBackend:  true,
			RestartBackend: true,
			BackupDatabase: true,
		}
	}
}

func validateOnlineUpdateManifest(manifest *onlineUpdateManifest) error {
	if _, ok := parseOnlineUpdateVersion(manifest.Version); !ok {
		return errors.New("版本号格式不正确")
	}
	parsed, err := parseOnlineUpdateURL(manifest.Package.URL)
	if err != nil {
		return errors.New("更新包下载地址不正确：" + err.Error())
	}
	manifestSource, sourceErr := parseOnlineUpdateURL(config.GetUpdateManifestURL())
	if sourceErr == nil && isGitHubRepositoryReleaseURL(manifestSource) && !isGitHubRepositoryReleaseURL(parsed) {
		return errors.New("GitHub 更新包地址不属于受信任的发布仓库")
	}
	if sourceErr == nil && isGiteeRepositoryUpdateURL(manifestSource) && !isGiteeRepositoryReleaseURL(parsed) {
		return errors.New("Gitee 更新包地址不属于受信任的发布仓库")
	}
	if strings.Contains(strings.ToLower(parsed.Host), "your-domain.com") {
		return errors.New("更新包下载地址仍是示例地址")
	}
	if !isHexSHA256(manifest.Package.SHA256) {
		return errors.New("更新包 SHA256 未配置或格式不正确")
	}
	if manifest.Package.Size <= 0 {
		return errors.New("更新包大小未配置")
	}
	if manifest.Package.Size > maxOnlineUpdatePackageSize {
		return errors.New("更新包超过 512MB 限制")
	}
	if err := validateOnlineUpdateSignature(manifest); err != nil {
		return err
	}
	if err := requireOnlineUpdatePackageFileName(manifest.Version, manifest.Package.FileName); err != nil {
		return err
	}
	return nil
}

func onlineUpdatePackageFileName(version string) (string, error) {
	parts, ok := parseOnlineUpdateVersion(version)
	if !ok || len(parts) != 3 {
		return "", errors.New("更新包版本号无法用于核对 GitHub 发布摘要")
	}
	return fmt.Sprintf("auth_pro-full-v%d.%d.%d.tar.gz", parts[0], parts[1], parts[2]), nil
}

func requireOnlineUpdatePackageFileName(version, fileName string) error {
	expected, err := onlineUpdatePackageFileName(version)
	if err != nil {
		return err
	}
	if strings.TrimSpace(fileName) != expected {
		return errors.New("更新包文件名与版本不一致")
	}
	return nil
}

// validateOnlineUpdateSignature 要求清单签名等于 sha256:<包哈希>。
// 这个字段和 SHA256 在同一份 JSON 里，不能单独当作防篡改；应用时还要对照
// maizll/auth-pro 发行版附件上由 GitHub 计算的 digest。
func validateOnlineUpdateSignature(manifest *onlineUpdateManifest) error {
	if manifest == nil {
		return errors.New("更新包缺少独立签名")
	}
	signature := strings.TrimSpace(manifest.Package.Signature)
	if signature == "" {
		return errors.New("更新包缺少独立签名")
	}
	expected := onlineUpdateSignatureForSHA256(manifest.Package.SHA256)
	if !onlineUpdateDigestPattern.MatchString(signature) || !strings.EqualFold(signature, expected) {
		return errors.New("更新包签名与 SHA256 不一致")
	}
	return nil
}

func onlineUpdateSignatureForSHA256(sum string) string {
	return "sha256:" + strings.ToLower(strings.TrimSpace(sum))
}

func evaluateOnlineUpdateCheck(currentVersion string, manifest *onlineUpdateManifest) (bool, string, error, bool) {
	return evaluateOnlineUpdateCheckForRuntime(currentVersion, manifest, runtime.GOOS, runtime.GOARCH)
}

func evaluateOnlineUpdateCheckForRuntime(currentVersion string, manifest *onlineUpdateManifest, currentOS, currentArch string) (bool, string, error, bool) {
	available, versionErr := onlineUpdateAvailable(currentVersion, manifest)
	packageErr := validateOnlineUpdateManifest(manifest)
	if packageErr == nil {
		packageErr = onlineUpdateRuntimeCompatibility(currentOS, currentArch, manifest)
	}
	canApply := packageErr == nil && available && versionErr == ""
	return available, versionErr, packageErr, canApply
}

func onlineUpdateRuntimeCompatibility(currentOS, currentArch string, manifest *onlineUpdateManifest) error {
	if currentOS != "linux" || currentArch != "amd64" {
		return errors.New("在线整包更新仅支持 Linux amd64（宝塔）环境。Windows 本地预览可以检查版本和更新说明，但不能安装")
	}
	if manifest != nil && manifest.Package.OS != "" && manifest.Package.OS != currentOS {
		return fmt.Errorf("更新包系统 %s 与当前系统 %s 不兼容", manifest.Package.OS, currentOS)
	}
	if manifest != nil && manifest.Package.Arch != "" && manifest.Package.Arch != currentArch {
		return fmt.Errorf("更新包架构 %s 与当前架构 %s 不兼容", manifest.Package.Arch, currentArch)
	}
	return nil
}

func onlineUpdateAvailable(currentVersion string, manifest *onlineUpdateManifest) (bool, string) {
	comparison, ok := compareOnlineUpdateVersions(currentVersion, manifest.Version)
	if !ok {
		return false, "当前版本或最新版本格式不正确"
	}
	if manifest.MinVersion != "" {
		minComparison, minOK := compareOnlineUpdateVersions(currentVersion, manifest.MinVersion)
		if minOK && minComparison < 0 {
			return false, "当前版本过低，不能直接升级到该版本"
		}
	}
	return comparison < 0, ""
}

func compareOnlineUpdateVersions(left, right string) (int, bool) {
	leftParts, leftOK := parseOnlineUpdateVersion(left)
	rightParts, rightOK := parseOnlineUpdateVersion(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	length := len(leftParts)
	if len(rightParts) > length {
		length = len(rightParts)
	}
	for i := 0; i < length; i++ {
		var l, r int
		if i < len(leftParts) {
			l = leftParts[i]
		}
		if i < len(rightParts) {
			r = rightParts[i]
		}
		if l < r {
			return -1, true
		}
		if l > r {
			return 1, true
		}
	}
	return 0, true
}

func parseOnlineUpdateVersion(value string) ([]int, bool) {
	matches := onlineUpdateVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 4 {
		return nil, false
	}
	parts := make([]int, 0, 3)
	for _, item := range matches[1:] {
		part, err := strconv.Atoi(item)
		if err != nil {
			return nil, false
		}
		parts = append(parts, part)
	}
	return parts, true
}

func isHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func reserveOnlineUpdateJob() bool {
	updateStore.mu.Lock()
	defer updateStore.mu.Unlock()
	if updateStore.runningID != "" {
		return false
	}
	updateStore.runningID = "reserved"
	return true
}

func createOnlineUpdateJob(version string) *onlineUpdateJob {
	now := time.Now()
	job := &onlineUpdateJob{
		ID:        fmt.Sprintf("U%d", now.UnixNano()),
		Status:    "running",
		Message:   "更新任务已启动",
		Progress:  0,
		Version:   version,
		Logs:      []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	updateStore.mu.Lock()
	updateStore.jobs[job.ID] = job
	updateStore.runningID = job.ID
	updateStore.mu.Unlock()
	persistOnlineUpdateJob(job)
	return job
}

func finishOnlineUpdateJob(id string, status string, message string, err error) {
	updateStore.mu.Lock()
	job, ok := updateStore.jobs[id]
	if !ok {
		updateStore.mu.Unlock()
		return
	}
	job.Status = status
	job.Message = message
	job.UpdatedAt = time.Now()
	if status == "restarting" && job.Progress < 95 {
		job.Progress = 95
	}
	if status == "success" {
		job.Progress = 100
	}
	if err != nil {
		job.Error = err.Error()
	}
	if status != "running" && status != "restarting" && updateStore.runningID == id {
		updateStore.runningID = ""
	}
	copyJob := cloneOnlineUpdateJob(job)
	updateStore.mu.Unlock()
	persistOnlineUpdateJob(copyJob)
}

func appendOnlineUpdateLog(id string, message string) {
	updateStore.mu.Lock()
	job, ok := updateStore.jobs[id]
	if !ok {
		updateStore.mu.Unlock()
		return
	}
	job.Logs = append(job.Logs, fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), message))
	job.Message = message
	job.UpdatedAt = time.Now()
	if len(job.Logs) > 200 {
		job.Logs = job.Logs[len(job.Logs)-200:]
	}
	copyJob := cloneOnlineUpdateJob(job)
	updateStore.mu.Unlock()
	persistOnlineUpdateJob(copyJob)
}

func updateOnlineUpdateProgress(id string, progress int, message string) {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	updateStore.mu.Lock()
	job, ok := updateStore.jobs[id]
	if !ok || progress < job.Progress {
		updateStore.mu.Unlock()
		return
	}
	if progress == job.Progress && (message == "" || message == job.Message) {
		updateStore.mu.Unlock()
		return
	}
	job.Progress = progress
	if message != "" {
		job.Message = message
	}
	job.UpdatedAt = time.Now()
	copyJob := cloneOnlineUpdateJob(job)
	updateStore.mu.Unlock()
	persistOnlineUpdateJob(copyJob)
}

func snapshotOnlineUpdateJob(id string) *onlineUpdateJob {
	updateStore.mu.Lock()
	job, ok := updateStore.jobs[id]
	if ok {
		job = cloneOnlineUpdateJob(job)
	}
	updateStore.mu.Unlock()
	if ok {
		return job
	}
	return loadOnlineUpdateJob(id)
}

func cloneOnlineUpdateJob(job *onlineUpdateJob) *onlineUpdateJob {
	if job == nil {
		return nil
	}
	copyJob := *job
	copyJob.Logs = append([]string{}, job.Logs...)
	return &copyJob
}

func onlineUpdateJobStatePath(id string) string {
	return filepath.Join(config.GetUpdateDir(), id+".json")
}

func persistOnlineUpdateJob(job *onlineUpdateJob) {
	if job == nil {
		return
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return
	}
	path := onlineUpdateJobStatePath(job.ID)
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tempPath, path)
}

func loadOnlineUpdateJob(id string) *onlineUpdateJob {
	if id == "" || filepath.Base(id) != id {
		return nil
	}
	data, err := os.ReadFile(onlineUpdateJobStatePath(id))
	if err != nil {
		return nil
	}
	var job onlineUpdateJob
	if err := json.Unmarshal(data, &job); err != nil || job.ID != id {
		return nil
	}
	return reconcileOnlineUpdateJobResult(&job)
}

func reconcileOnlineUpdateJobResult(job *onlineUpdateJob) *onlineUpdateJob {
	data, err := os.ReadFile(onlineUpdateJobStatePath(job.ID) + ".result")
	if err != nil {
		return job
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n"), "\n")
	result := strings.TrimSpace(lines[0])
	reason := ""
	if len(lines) > 1 {
		reason = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	if result != "success" && result != "failed" {
		return job
	}
	if job.Status == result && (result != "failed" || job.Error == reason || reason == "") {
		return job
	}
	job.Status = result
	job.Error = ""
	if result == "success" {
		job.Message = "更新完成"
		job.Progress = 100
		job.Logs = append(job.Logs, fmt.Sprintf("%s 前端和后端已切换到 v%s", time.Now().Format("15:04:05"), job.Version))
	} else {
		job.Message = "更新失败，已尝试回滚"
		if reason == "" {
			reason = "新版本健康检查失败或更新脚本执行异常"
		}
		job.Error = reason
		job.Logs = append(job.Logs, fmt.Sprintf("%s 更新失败，已尝试回滚：%s", time.Now().Format("15:04:05"), reason))
	}
	job.UpdatedAt = time.Now()
	persistOnlineUpdateJob(job)
	return job
}

func runningOnlineUpdateJob() *onlineUpdateJob {
	updateStore.mu.Lock()
	id := updateStore.runningID
	updateStore.mu.Unlock()
	if id == "" || id == "reserved" {
		return nil
	}
	return snapshotOnlineUpdateJob(id)
}

func setCachedOnlineUpdateManifest(manifest *onlineUpdateManifest) {
	updateStore.mu.Lock()
	copyManifest := *manifest
	copyManifest.Notes = append([]string{}, manifest.Notes...)
	updateStore.latest = &copyManifest
	updateStore.mu.Unlock()
}

func cachedOnlineUpdateManifest() *onlineUpdateManifest {
	updateStore.mu.Lock()
	defer updateStore.mu.Unlock()
	if updateStore.latest == nil {
		return nil
	}
	copyManifest := *updateStore.latest
	copyManifest.Notes = append([]string{}, updateStore.latest.Notes...)
	return &copyManifest
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func runOnlineUpdateJob(jobID string, manifest onlineUpdateManifest) {
	if err := executeOnlineUpdate(jobID, &manifest); err != nil {
		appendOnlineUpdateLog(jobID, "更新失败："+err.Error())
		finishOnlineUpdateJob(jobID, "failed", "更新失败", err)
	}
}

func executeOnlineUpdate(jobID string, manifest *onlineUpdateManifest) error {
	if !manifest.Actions.UpdateFrontend || !manifest.Actions.UpdateBackend {
		return errors.New("方案二要求更新包同时包含前端和后端")
	}
	if err := validateOnlineUpdateSignature(manifest); err != nil {
		return err
	}

	updateOnlineUpdateProgress(jobID, 5, "正在准备更新")
	appendOnlineUpdateLog(jobID, "开始下载更新包")
	updateOnlineUpdateProgress(jobID, 10, "开始下载更新包")
	packagePath, err := downloadOnlineUpdatePackageForApply(jobID, manifest)
	if err != nil {
		return err
	}
	if err := verifyDownloadedOnlineUpdatePackage(manifest, packagePath); err != nil {
		return err
	}
	appendOnlineUpdateLog(jobID, "已核对更新包签名与 GitHub 发布资产摘要")
	appendOnlineUpdateLog(jobID, "更新包下载完成")
	updateOnlineUpdateProgress(jobID, 50, "更新包下载完成")

	appendOnlineUpdateLog(jobID, "开始解压更新包")
	updateOnlineUpdateProgress(jobID, 55, "开始解压更新包")
	stagingDir, err := extractOnlineUpdatePackage(jobID, packagePath)
	if err != nil {
		return err
	}
	appendOnlineUpdateLog(jobID, "更新包解压完成")
	updateOnlineUpdateProgress(jobID, 65, "更新包解压完成")

	pkg, err := validateExtractedOnlineUpdatePackage(stagingDir, manifest.Version)
	if err != nil {
		return err
	}
	appendOnlineUpdateLog(jobID, "更新包结构校验通过")
	updateOnlineUpdateProgress(jobID, 72, "更新包结构校验通过")

	if manifest.Actions.BackupDatabase {
		updateOnlineUpdateProgress(jobID, 76, "正在备份数据库")
		if err := backupDatabaseForOnlineUpdate(jobID); err != nil {
			return err
		}
		updateOnlineUpdateProgress(jobID, 82, "数据库备份完成")
	}

	frontendDir := config.GetFrontendDir()
	if info, err := os.Stat(frontendDir); err != nil || !info.IsDir() {
		return fmt.Errorf("未找到前端目录 %s，请设置 AUTO_PRO_FRONTEND_DIR", frontendDir)
	}

	updateOnlineUpdateProgress(jobID, 86, "正在准备新版本文件")
	frontendSource, err := prepareOnlineUpdateFrontendSource(stagingDir, pkg)
	if err != nil {
		return err
	}

	scriptPath, err := writeOnlineUpdateScript(jobID, stagingDir, pkg, frontendSource, manifest.Version, frontendDir)
	if err != nil {
		return err
	}
	mode := resolveOnlineUpdateProcessManager()
	appendOnlineUpdateLog(jobID, "更新脚本已生成，准备重启服务")
	appendOnlineUpdateLog(jobID, describeProcessManager(mode))
	updateOnlineUpdateProgress(jobID, 92, "更新脚本已生成，准备重启服务")

	logPath := filepath.Join(config.GetUpdateDir(), jobID+".log")
	logFile, _ := os.Create(logPath)
	if logFile != nil {
		defer logFile.Close()
	}
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Dir = config.GetDataDir()
	detachOnlineUpdateCommand(cmd)
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()

	finishOnlineUpdateJob(jobID, "restarting", "服务正在切换并重启", nil)
	return nil
}

func downloadOnlineUpdatePackage(jobID string, manifest *onlineUpdateManifest) (string, error) {
	client, err := newOnlineUpdateHTTPClient(manifest.Package.URL, 10*time.Minute)
	if err != nil {
		return "", err
	}
	return downloadOnlineUpdatePackageWithClient(jobID, manifest, client)
}

type onlineUpdateDownloadWriter struct {
	jobID       string
	destination io.Writer
	total       int64
	written     int64
	lastPercent int
}

func (writer *onlineUpdateDownloadWriter) Write(data []byte) (int, error) {
	count, err := writer.destination.Write(data)
	writer.written += int64(count)
	if writer.total <= 0 {
		return count, err
	}

	downloadPercent := int(writer.written * 100 / writer.total)
	if downloadPercent > 100 {
		downloadPercent = 100
	}
	if downloadPercent != writer.lastPercent {
		writer.lastPercent = downloadPercent
		overallProgress := 10 + downloadPercent*40/100
		updateOnlineUpdateProgress(
			writer.jobID,
			overallProgress,
			fmt.Sprintf("正在下载更新包 %d%%", downloadPercent),
		)
	}
	return count, err
}

func downloadOnlineUpdatePackageWithClient(jobID string, manifest *onlineUpdateManifest, client *http.Client) (string, error) {
	fileName := manifest.Package.FileName
	if fileName == "" {
		fileName = fmt.Sprintf("auth_pro-full-v%s.tar.gz", manifest.Version)
	}
	fileName = filepath.Base(fileName)
	target := filepath.Join(config.GetUpdateDir(), jobID+"-"+fileName)

	req, err := http.NewRequest(http.MethodGet, manifest.Package.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载更新包失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("更新包下载地址返回状态码 %d", resp.StatusCode)
	}

	out, err := os.Create(target)
	if err != nil {
		return "", fmt.Errorf("创建更新包文件失败：%w", err)
	}
	defer out.Close()

	hash := sha256.New()
	progressWriter := &onlineUpdateDownloadWriter{
		jobID:       jobID,
		destination: out,
		total:       manifest.Package.Size,
		lastPercent: -1,
	}
	written, err := io.Copy(
		progressWriter,
		io.TeeReader(io.LimitReader(resp.Body, maxOnlineUpdatePackageSize+1), hash),
	)
	if err != nil {
		return "", fmt.Errorf("保存更新包失败：%w", err)
	}
	if written > maxOnlineUpdatePackageSize {
		return "", errors.New("更新包超过 512MB 限制")
	}
	if written != manifest.Package.Size {
		return "", fmt.Errorf("更新包大小不一致，期望 %d 字节，实际 %d 字节", manifest.Package.Size, written)
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualSHA256, manifest.Package.SHA256) {
		return "", fmt.Errorf("更新包 SHA256 不一致，期望 %s，实际 %s", manifest.Package.SHA256, actualSHA256)
	}
	return target, nil
}

func verifyDownloadedOnlineUpdatePackage(manifest *onlineUpdateManifest, packagePath string) error {
	sum, err := hashOnlineUpdateFile(packagePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(sum, manifest.Package.SHA256) {
		return fmt.Errorf("更新包 SHA256 不一致，期望 %s，实际 %s", manifest.Package.SHA256, sum)
	}
	if !strings.EqualFold(strings.TrimSpace(manifest.Package.Signature), onlineUpdateSignatureForSHA256(sum)) {
		return errors.New("更新包签名与 SHA256 不一致")
	}
	return confirmOnlineUpdateTrustedDigest(manifest.Version, manifest.Package.FileName, sum)
}

func hashOnlineUpdateFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取更新包失败：%w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("读取更新包失败：%w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func confirmOnlineUpdateTrustedDigest(version, fileName, actualSHA256 string) error {
	if err := requireOnlineUpdatePackageFileName(version, fileName); err != nil {
		return err
	}
	if allowLocalOnlineUpdateDigest() {
		return nil
	}
	digest, err := fetchOnlineUpdateAssetDigest(version, fileName)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(digest), onlineUpdateSignatureForSHA256(actualSHA256)) {
		return errors.New("更新包签名与 GitHub 发布资产摘要不一致")
	}
	return nil
}

func onlineUpdateGitHubReleaseTagURL(version string) (string, error) {
	parts, ok := parseOnlineUpdateVersion(version)
	if !ok || len(parts) != 3 {
		return "", errors.New("更新包版本号无法用于核对 GitHub 发布摘要")
	}
	return fmt.Sprintf("https://api.github.com/repos/maizll/auth-pro/releases/tags/v%d.%d.%d", parts[0], parts[1], parts[2]), nil
}

func safeOnlineUpdateAssetName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

func fetchGitHubReleaseAssetDigest(version, fileName string) (string, error) {
	if !safeOnlineUpdateAssetName(fileName) {
		return "", errors.New("更新包文件名无法用于核对 GitHub 发布摘要")
	}
	rawURL, err := onlineUpdateGitHubReleaseTagURL(version)
	if err != nil {
		return "", err
	}
	client, err := newOnlineUpdateHTTPClient(rawURL, 15*time.Second)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "auth_pro-updater/"+config.AppVersion)

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("核对 GitHub 发布资产摘要失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub 发布资产摘要接口返回状态码 %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", errors.New("读取 GitHub 发布资产摘要失败")
	}
	return gitHubReleaseAssetDigest(body, fileName)
}

func gitHubReleaseAssetDigest(body []byte, fileName string) (string, error) {
	var release struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", errors.New("GitHub 发布资产摘要格式不正确")
	}
	found := ""
	for _, asset := range release.Assets {
		if asset.Name != fileName {
			continue
		}
		digest := strings.TrimSpace(asset.Digest)
		if !onlineUpdateDigestPattern.MatchString(digest) {
			return "", errors.New("GitHub 发布资产缺少摘要")
		}
		if found == "" {
			found = digest
			continue
		}
		if !strings.EqualFold(found, digest) {
			return "", errors.New("GitHub 发布资产摘要不一致")
		}
	}
	if found == "" {
		return "", errors.New("GitHub 发布资产缺少更新包")
	}
	return found, nil
}

func extractOnlineUpdatePackage(jobID string, packagePath string) (string, error) {
	stagingDir := filepath.Join(config.GetUpdateDir(), jobID, "staging")
	if err := os.RemoveAll(filepath.Dir(stagingDir)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return "", err
	}

	file, err := os.Open(packagePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", errors.New("更新包不是有效的 tar.gz 文件")
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("读取更新包失败")
		}
		name := path.Clean(strings.TrimPrefix(header.Name, "./"))
		if name == "." {
			if header.Typeflag == tar.TypeDir {
				continue
			}
			return "", fmt.Errorf("更新包包含非法路径 %s", header.Name)
		}
		if path.IsAbs(name) || strings.HasPrefix(name, "../") {
			return "", fmt.Errorf("更新包包含非法路径 %s", header.Name)
		}
		target := filepath.Join(stagingDir, filepath.FromSlash(name))
		cleanStaging := filepath.Clean(stagingDir)
		if target != cleanStaging && !strings.HasPrefix(target, cleanStaging+string(os.PathSeparator)) {
			return "", fmt.Errorf("更新包包含越界路径 %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return "", err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return "", err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(out, tarReader)
			closeErr := out.Close()
			if copyErr != nil {
				return "", copyErr
			}
			if closeErr != nil {
				return "", closeErr
			}
		default:
			return "", fmt.Errorf("更新包包含不支持的文件类型 %s", header.Name)
		}
	}
	return stagingDir, nil
}

type extractedOnlineUpdateManifest struct {
	Version       string   `json:"version"`
	FrontendDir   string   `json:"frontendDir"`
	BackendFile   string   `json:"backendFile"`
	RequiredFiles []string `json:"requiredFiles"`
}

func validateExtractedOnlineUpdatePackage(stagingDir string, expectedVersion string) (*extractedOnlineUpdateManifest, error) {
	manifestPath := filepath.Join(stagingDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.New("更新包缺少 manifest.json")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var pkg extractedOnlineUpdateManifest
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, errors.New("更新包 manifest.json 不是有效 JSON")
	}
	if pkg.Version != "" && pkg.Version != expectedVersion {
		return nil, fmt.Errorf("更新包内部版本 %s 与清单版本 %s 不一致", pkg.Version, expectedVersion)
	}
	if pkg.FrontendDir == "" {
		pkg.FrontendDir = "frontend"
	}
	if pkg.BackendFile == "" {
		pkg.BackendFile = "backend/auth_pro"
	}
	if !safeOnlineUpdateRelativePath(pkg.FrontendDir) || !safeOnlineUpdateRelativePath(pkg.BackendFile) {
		return nil, errors.New("更新包 manifest 包含非法路径")
	}
	required := append([]string{
		path.Join(pkg.FrontendDir, "index.html"),
		path.Join(pkg.FrontendDir, "version.json"),
		pkg.BackendFile,
	}, pkg.RequiredFiles...)
	for _, item := range required {
		if !safeOnlineUpdateRelativePath(item) {
			return nil, fmt.Errorf("更新包 manifest 包含非法路径 %s", item)
		}
		if info, err := os.Stat(filepath.Join(stagingDir, filepath.FromSlash(item))); err != nil || info.IsDir() {
			return nil, fmt.Errorf("更新包缺少文件 %s", item)
		}
	}
	return &pkg, nil
}

func safeOnlineUpdateRelativePath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !path.IsAbs(value) && path.Clean(value) == value && !strings.HasPrefix(value, "../")
}

func backupDatabaseForOnlineUpdate(jobID string) error {
	mysqldump, err := exec.LookPath("mysqldump")
	if err != nil {
		return errors.New("未找到 mysqldump，无法执行更新前数据库备份")
	}
	cfg, err := config.LoadDBConfig()
	if err != nil {
		return fmt.Errorf("读取数据库配置失败，无法执行更新前备份：%w", err)
	}
	backupDir := filepath.Join(config.GetUpdateDir(), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("创建数据库备份目录失败：%w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("db-%s.sql", time.Now().Format("20060102150405")))
	out, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("创建数据库备份文件失败：%w", err)
	}

	args := []string{"--single-transaction", "--quick"}
	if strings.HasPrefix(cfg.Host, "unix:") {
		args = append(args, "--socket", strings.TrimPrefix(cfg.Host, "unix:"))
	} else {
		args = append(args, "-h", cfg.Host)
		if cfg.Port != "" {
			args = append(args, "-P", cfg.Port)
		}
	}
	args = append(args, "-u", cfg.Username, cfg.Database)

	var stderr bytes.Buffer
	cmd := exec.Command(mysqldump, args...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.Password)
	cmd.Stdout = out
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	closeErr := out.Close()
	if runErr != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("数据库备份失败：%s", strings.TrimSpace(stderr.String()))
	}
	if closeErr != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("保存数据库备份失败：%w", closeErr)
	}
	appendOnlineUpdateLog(jobID, "数据库已备份到 "+backupPath)
	return nil
}

func prepareOnlineUpdateFrontendSource(stagingDir string, pkg *extractedOnlineUpdateManifest) (string, error) {
	if pkg.FrontendDir != "." {
		return filepath.Join(stagingDir, filepath.FromSlash(pkg.FrontendDir)), nil
	}

	// 宝塔手工部署包的前端文件直接放在包根目录。
	// 在线更新时过滤掉后端和 manifest，只把 Web 文件复制到前端 releases 目录。
	filteredDir := filepath.Join(filepath.Dir(stagingDir), "frontend-filtered")
	if err := os.RemoveAll(filteredDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filteredDir, 0755); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "backend" || name == "manifest.json" {
			continue
		}
		if err := copyOnlineUpdatePath(
			filepath.Join(stagingDir, name),
			filepath.Join(filteredDir, name),
		); err != nil {
			return "", err
		}
	}
	return filteredDir, nil
}

func copyOnlineUpdatePath(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}

	if err := os.MkdirAll(target, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyOnlineUpdatePath(
			filepath.Join(source, entry.Name()),
			filepath.Join(target, entry.Name()),
		); err != nil {
			return err
		}
	}
	return nil
}

// allowLocalOnlineUpdateDigest 只给本机演练用。清单地址必须是回环 HTTPS，
// 并且显式设置 AUTO_PRO_UPDATE_LOCAL_DIGEST=1。生产默认的 GitHub 清单不会走这里。
func allowLocalOnlineUpdateDigest() bool {
	if strings.TrimSpace(os.Getenv("AUTO_PRO_UPDATE_LOCAL_DIGEST")) != "1" {
		return false
	}
	parsed, err := parseOnlineUpdateURL(config.GetUpdateManifestURL())
	if err != nil {
		return false
	}
	host := strings.Trim(strings.ToLower(parsed.Hostname()), "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func writeOnlineUpdateScript(jobID string, stagingDir string, pkg *extractedOnlineUpdateManifest, frontendSource string, version string, frontendDir string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}

	dataDir := config.GetDataDir()
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", err
	}
	scriptPath := filepath.Join(config.GetUpdateDir(), jobID+".sh")
	mode := resolveOnlineUpdateProcessManager()
	replacements := []struct{ token, value string }{
		{"__APP_PID__", strconv.Itoa(os.Getpid())},
		{"__APP_BIN__", shellQuoteOnlineUpdate(executable)},
		{"__NEW_BIN__", shellQuoteOnlineUpdate(filepath.Join(stagingDir, filepath.FromSlash(pkg.BackendFile)))},
		{"__FRONTEND_SOURCE__", shellQuoteOnlineUpdate(frontendSource)},
		{"__FRONTEND_CURRENT__", shellQuoteOnlineUpdate(frontendDir)},
		{"__DATA_DIR__", shellQuoteOnlineUpdate(dataDir)},
		{"__SERVICE_NAME__", shellQuoteOnlineUpdate(config.GetServiceName())},
		{"__PORT__", shellQuoteOnlineUpdate(config.GetPort())},
		{"__VERSION__", shellQuoteOnlineUpdate(version)},
		{"__OLD_VERSION__", shellQuoteOnlineUpdate(config.AppVersion)},
		{"__LOG_FILE__", shellQuoteOnlineUpdate(filepath.Join(logDir, jobID+".log"))},
		{"__JOB_RESULT__", shellQuoteOnlineUpdate(onlineUpdateJobStatePath(jobID) + ".result")},
		{"__PROCESS_MANAGER__", shellQuoteOnlineUpdate(mode)},
		{"__SUPERVISOR_PROGRAM__", shellQuoteOnlineUpdate(onlineUpdateSupervisorProgram())},
		{"__SUPERVISOR_CONF__", shellQuoteOnlineUpdate(strings.TrimSpace(os.Getenv("AUTO_PRO_SUPERVISOR_CONF")))},
		{"__HEALTH_TRIES__", strconv.Itoa(onlineUpdateHealthTries())},
	}
	script := onlineUpdateScriptTemplate
	for _, item := range replacements {
		script = strings.ReplaceAll(script, item.token, item.value)
	}
	if strings.Contains(script, "__") {
		return "", errors.New("更新脚本模板存在未替换的占位符")
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func shellQuoteOnlineUpdate(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
