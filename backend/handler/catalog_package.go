// 软件目录里的安装包只从官网地址下载。
// 公开清单不写仓库地址，也不写收费仓库的临时链接。
// 官网按目录里保存的位置取包，核对 sha256 后再交给客户端。
// 客户端检查插件更新时，以这份清单里的最新版本为准。

package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	catalogPackagePublicPrefix = "https://auth.maizll.com/api/v1/catalog/package/"
	catalogPackageMissingText  = "安装包不存在"
	catalogPackageFailText     = "暂时无法下载安装包"
	catalogPackageSHAText      = "安装包校验失败"
	catalogPackageRateText     = "请求过于频繁，请稍后再试"
	catalogPackageHostText     = "安装包只能从官网下载"
)

var (
	errCatalogPackageMissing     = errors.New(catalogPackageMissingText)
	errCatalogPackageUnavailable = errors.New(catalogPackageFailText)
	errCatalogPackageSHA         = errors.New(catalogPackageSHAText)
	errPackageHostBlocked        = errors.New(catalogPackageHostText)
	catalogPackageIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,80}$`)
	catalogPackageLimiter        = newProductUpdateRateLimiter(30, time.Minute)
)

// catalogBuyerPackageURL 是写进公开目录的下载地址。客户端只认这个地址。
func catalogBuyerPackageURL(kind, id string) string {
	return catalogPackagePublicPrefix + kind + "/" + url.PathEscape(strings.TrimSpace(id))
}

func catalogPublicPackageFields(kind, id, location, sha string, priceCents int64) (field, rawURL, sum string, ok bool) {
	location = strings.TrimSpace(location)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if priceCents > 0 || location == "" || sha == "" {
		return "", "", "", false
	}
	if err := validateSHA256(sha); err != nil {
		return "", "", "", false
	}
	id = strings.TrimSpace(id)
	if !catalogPackageIDPattern.MatchString(id) {
		return "", "", "", false
	}
	switch kind {
	case "plugin":
		field = "downloadUrl"
	case "template":
		field = "templateUrl"
	default:
		return "", "", "", false
	}
	return field, catalogBuyerPackageURL(kind, id), sha, true
}

// catalogRepoHostBlocked 判断这个地址是不是代码托管站。
// 客户端不能去这些主机取包，响应里也不能带上仓库地址。
func catalogRepoHostBlocked(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	if isRemoteManagedRef(raw) || strings.HasPrefix(lower, "github:") || strings.HasPrefix(lower, "gitee:") || strings.HasPrefix(lower, "s3:") || strings.HasPrefix(lower, "webdav:") {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	return githubHostBlocked(parsed.Hostname())
}

func publicCatalogAuthor(author sourceAuthor) sourceAuthor {
	if catalogRepoHostBlocked(author.URL) {
		author.URL = ""
	}
	return author
}

func githubHostBlocked(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "github.com", "www.github.com", "api.github.com", "codeload.github.com", "gist.github.com", "githubusercontent.com":
		return true
	}
	return strings.HasSuffix(host, ".github.com") || strings.HasSuffix(host, ".githubusercontent.com")
}

// CatalogPackageDownload 按目录条目把安装包发给客户端。
// 只提供已上架的免费包。付费包仍要先换下载票。失败时不回显存储位置。
func CatalogPackageDownload(c *gin.Context) {
	if !catalogPackageLimiter.allow(c.ClientIP(), time.Now()) {
		catalogPackageFail(c, http.StatusTooManyRequests, catalogPackageRateText)
		return
	}
	kind := strings.TrimSpace(c.Param("kind"))
	id := strings.TrimSpace(c.Param("id"))
	if (kind != "plugin" && kind != "template") || !catalogPackageIDPattern.MatchString(id) {
		catalogPackageFail(c, http.StatusNotFound, catalogPackageMissingText)
		return
	}
	location, sha, ok := publishedFreePackage(kind, id)
	if !ok {
		catalogPackageFail(c, http.StatusNotFound, catalogPackageMissingText)
		return
	}
	payload, err := readStoredCatalogPackage(c.Request.Context(), location, sha)
	if err != nil {
		status := http.StatusBadGateway
		msg := catalogPackageFailText
		if errors.Is(err, errCatalogPackageMissing) {
			status = http.StatusNotFound
			msg = catalogPackageMissingText
		}
		if errors.Is(err, errCatalogPackageSHA) {
			status = http.StatusBadGateway
			msg = catalogPackageSHAText
		}
		catalogPackageFail(c, status, msg)
		return
	}
	filename := id + ".bin"
	media := "application/octet-stream"
	if isZipPayload(payload) {
		filename = id + ".zip"
		media = "application/zip"
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, media, payload)
}

func catalogPackageFail(c *gin.Context, status int, msg string) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.String(status, msg)
}

func publishedFreePackage(kind, id string) (location, sha string, ok bool) {
	switch kind {
	case "plugin":
		item, err := currentSourceStationStore().GetPlugin(id)
		if err != nil || item.Status != sourceItemPublished || item.PriceCents > 0 {
			return "", "", false
		}
		if strings.TrimSpace(item.SHA256) == "" || strings.TrimSpace(item.DownloadURL) == "" {
			return "", "", false
		}
		return item.DownloadURL, item.SHA256, true
	case "template":
		item, found := findPublishedTemplate(id)
		if !found || item.PriceCents > 0 {
			return "", "", false
		}
		if strings.TrimSpace(item.SHA256) == "" || strings.TrimSpace(item.TemplateURL) == "" {
			return "", "", false
		}
		return item.TemplateURL, item.SHA256, true
	default:
		return "", "", false
	}
}

func findPublishedTemplate(id string) (sourceTemplate, bool) {
	if item, err := currentSourceStationStore().GetTemplate(id); err == nil {
		if item.Status == sourceItemPublished && (item.ID == id || item.TemplateKey == id) {
			return item, true
		}
	}
	items, err := currentSourceStationStore().ListTemplates(sourceItemPublished)
	if err != nil {
		return sourceTemplate{}, false
	}
	for _, item := range items {
		if item.TemplateKey == id || item.ID == id {
			return item, true
		}
	}
	return sourceTemplate{}, false
}

// readStoredCatalogPackage 在服务端取出安装包并核对清单里的 sha256。
// 仓库地址和令牌留在这一侧，不写进返回值。
func readStoredCatalogPackage(ctx context.Context, location, expectedSHA string) ([]byte, error) {
	expectedSHA = strings.ToLower(strings.TrimSpace(expectedSHA))
	if err := validateSHA256(expectedSHA); err != nil {
		return nil, errCatalogPackageMissing
	}
	payload, err := readStoredPackageBytes(ctx, location)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != expectedSHA {
		return nil, errCatalogPackageSHA
	}
	return payload, nil
}

func readStoredPackageBytes(ctx context.Context, location string) ([]byte, error) {
	return readStoredPackageWithFallback(ctx, location)
}

func readPrivatePackageFile(location string) ([]byte, error) {
	name, ok := privatePackageName(location)
	if !ok || strings.Contains(name, "..") {
		return nil, errCatalogPackageMissing
	}
	path := filepath.Join(stationPaidPackageDir(), name)
	if !pathInsideDir(stationPaidPackageDirPath(), path) {
		return nil, errCatalogPackageMissing
	}
	return readCatalogFile(path)
}

func readCatalogFile(path string) ([]byte, error) {
	payload, err := os.ReadFile(path)
	if err != nil || len(payload) == 0 {
		return nil, errCatalogPackageMissing
	}
	if int64(len(payload)) > pluginPackageMaxSize {
		return nil, errCatalogPackageUnavailable
	}
	return payload, nil
}

// fetchGitHubPackageBytes 用源站已保存的令牌把仓库里的附件读回来。
// 跳转留在服务端，调用方只拿到字节。
func fetchGitHubPackageBytes(ctx context.Context, location string) ([]byte, error) {
	ref, ok := parseGitHubPackageRef(location)
	if !ok {
		return nil, errCatalogPackageMissing
	}
	token, err := tokenForGitHubRepo(ref.Owner, ref.Repo)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, errCatalogPackageUnavailable
	}
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return nil, errCatalogPackageUnavailable
	}
	asset, err := githubAssetByName(release, ref.Asset)
	if err != nil {
		return nil, errCatalogPackageUnavailable
	}
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/assets/" + fmt.Sprintf("%d", asset.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errCatalogPackageUnavailable
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req == nil || req.URL == nil {
				return errCatalogPackageUnavailable
			}
			if err := validateGitHubBuyerRedirect(req.URL.String(), false); err != nil {
				return errCatalogPackageUnavailable
			}
			req.Header.Del("Authorization")
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errCatalogPackageUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, errCatalogPackageUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, storageReadLimit+1))
	if err != nil || int64(len(payload)) > storageReadLimit || len(payload) == 0 {
		return nil, errCatalogPackageUnavailable
	}
	manifest, sharded := parseShardManifest(payload)
	if !sharded {
		if int64(len(payload)) > pluginPackageMaxSize {
			return nil, errCatalogPackageUnavailable
		}
		return payload, nil
	}
	parts := make([][]byte, 0, len(manifest.Parts))
	for _, name := range manifest.Parts {
		partRef := ref
		partRef.Asset = name
		part, partErr := fetchGitHubAssetBytes(ctx, token, partRef)
		if partErr != nil {
			return nil, errCatalogPackageUnavailable
		}
		parts = append(parts, part)
	}
	joined, joinErr := joinShards(parts, manifest.SHA256, manifest.Size)
	if joinErr != nil {
		return nil, errCatalogPackageUnavailable
	}
	return joined, nil
}
