// 从已保存的令牌读取私有仓库 Release，并把选中的安装包暂存到本站。
// 应用发布、插件发布和模板发布共用这一套，避免各自再连一次仓库。
// 响应里只给版本、标题、说明、大小和校验值，不回仓库页面地址。

package handler

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	releaseImportDefaultPaidRepo = "maizll/auth-pro-paid"
	releaseImportStageTTL        = 2 * time.Hour
)

var releaseStageIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type releaseStageMeta struct {
	FileName  string `json:"fileName"`
	Size      int64  `json:"size"`
	MD5       string `json:"md5"`
	SHA256    string `json:"sha256"`
	Version   string `json:"version"`
	Title     string `json:"title"`
	Changelog string `json:"changelog"`
}

type releaseImportView struct {
	StagingID     string `json:"stagingId"`
	Version       string `json:"version"`
	Title         string `json:"title"`
	Changelog     string `json:"changelog"`
	FileName      string `json:"fileName"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	FileMd5       string `json:"fileMd5"`
	FileSha256    string `json:"fileSha256"`
	AssetName     string `json:"assetName"`
}

// RegisterReleaseImportRoutes 挂上站长侧的导入接口。调用方自己负责登录和菜单权限。
// 开发者路由不挂这组接口。开发者上传 ZIP 仍由存储管理用站长令牌写入收费仓库。
func RegisterReleaseImportRoutes(group *gin.RouterGroup) {
	group.POST("/release-import/releases", ReleaseImportList)
	group.POST("/release-import/fetch", ReleaseImportFetch)
	group.POST("/release-import/probe-url", ReleaseImportProbeURL)
	group.POST("/release-import/materialize", ReleaseImportMaterialize)
}

// ReleaseImportList 列出仓库里的 Release，供后台选择。
// purpose 决定默认仓库：app 用客户交付仓库，plugin 和 template 用付费仓库。请求里的 repo 可以改。
// 令牌用系统里已经保存的那一枚。连不上时返回 400，不回显令牌和仓库页面。
func ReleaseImportList(c *gin.Context) {
	var req struct {
		Purpose string `json:"purpose"`
		Repo    string `json:"repo"`
	}
	_ = c.ShouldBindJSON(&req)
	owner, repo, err := releaseImportRepo(req.Purpose, req.Repo)
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	releases, err := listReleaseImportReleases(c.Request.Context(), owner, repo)
	if err != nil {
		apiError(c, 400, "无法读取仓库发布列表，请检查已保存的令牌")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"repo": owner + "/" + repo, "releases": releases,
	}})
}

// ReleaseImportFetch 下载选中的 Release 附件并暂存。
// 返回版本号、标题、说明、大小、MD5 和 SHA256，发布前仍可改。暂存编号在保存版本时交给对应的发布接口。
func ReleaseImportFetch(c *gin.Context) {
	var req struct {
		Purpose   string `json:"purpose"`
		Repo      string `json:"repo"`
		Tag       string `json:"tag"`
		AssetName string `json:"assetName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Tag) == "" {
		apiError(c, 400, "请选择要导入的发布")
		return
	}
	owner, repo, err := releaseImportRepo(req.Purpose, req.Repo)
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	view, err := fetchReleaseImportAsset(c.Request.Context(), owner, repo, strings.TrimSpace(req.Tag), strings.TrimSpace(req.AssetName))
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": view})
}

// ReleaseImportProbeURL 按填写的 https 地址下载安装包，算出大小和校验值。
// 拒绝内网和元数据地址。结果先暂存，保存发布时再入库，避免只填了数字却没有文件。
func ReleaseImportProbeURL(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, 400, "请填写下载地址")
		return
	}
	rawURL := strings.TrimSpace(req.URL)
	if err := validateDownloadURL(rawURL); err != nil {
		apiError(c, 400, err.Error())
		return
	}
	payload, err := safeHTTPGet(c.Request.Context(), rawURL, safeFetchOptions{
		RequireHTTPS: true,
		MaxBytes:     appReleaseMaxBytes,
		Timeout:      2 * time.Minute,
		UserAgent:    "auth-pro-source",
	})
	if err != nil {
		apiError(c, 400, releaseImportFetchError(err))
		return
	}
	name := packageNameFromURL(rawURL)
	if name == "" {
		name = "package.bin"
	}
	stageID, meta, err := stageReleaseBytes(name, payload, "", "", "")
	if err != nil {
		apiError(c, 500, "保存安装包失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": releaseImportView{
		StagingID: stageID, Version: "", Title: "", Changelog: "",
		FileName: meta.FileName, FileSizeBytes: meta.Size, FileMd5: meta.MD5, FileSha256: meta.SHA256,
		AssetName: meta.FileName,
	}})
}

// ReleaseImportMaterialize 把暂存的插件或模板包写入目录存储，并返回可登记的地址和校验值。
// 收费条目走收费包存储，免费条目走本站公开目录。应用发布不走这里，它在保存版本时直接收暂存编号。
func ReleaseImportMaterialize(c *gin.Context) {
	var req struct {
		StagingID  string `json:"stagingId"`
		Kind       string `json:"kind"`
		ItemID     string `json:"itemId"`
		Version    string `json:"version"`
		PriceCents int64  `json:"priceCents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.StagingID) == "" {
		apiError(c, 400, "请先从仓库导入安装包")
		return
	}
	payload, meta, err := readReleaseStage(req.StagingID)
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind != sourceKindPlugin && kind != sourceKindTemplate {
		apiError(c, 400, "请指定插件或模板")
		return
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = meta.Version
	}
	var location, sum string
	if req.PriceCents > 0 {
		location, sum, _, err = settlePaidZipBytes(c.Request.Context(), kind, strings.TrimSpace(req.ItemID), version, payload)
	} else {
		location, sum, err = storeStationPackage(payload)
	}
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"location": location, "sha256": sum, "fileName": meta.FileName,
		"fileSizeBytes": meta.Size, "fileMd5": meta.MD5,
	}})
}

func releaseImportRepo(purpose, raw string) (string, string, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" {
		if purpose == "plugin" || purpose == "template" {
			owner, repo, _, err := loadGitHubPaidRepo()
			if err == nil && owner != "" && repo != "" {
				return owner, repo, nil
			}
			if owner, repo, ok := primaryGitHubStorageRepo(); ok {
				return owner, repo, nil
			}
			raw = releaseImportDefaultPaidRepo
		} else {
			owner, repo, err := productUpdateRepository()
			return owner, repo, err
		}
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || !sourceReleaseRepoPattern.MatchString(parts[0]) || !sourceReleaseRepoPattern.MatchString(parts[1]) {
		return "", "", errors.New("仓库格式应为 所有者/名称")
	}
	return parts[0], parts[1], nil
}

// primaryGitHubStorageRepo 用存储管理里启用的 GitHub 位置作为插件、模板导入的默认仓库。
func primaryGitHubStorageRepo() (string, string, bool) {
	blob, err := loadStorageBlob()
	if err != nil {
		return "", "", false
	}
	for _, loc := range enabledStorageLocations(blob.Locations) {
		if loc.Kind != packageStorageGitHub {
			continue
		}
		owner := strings.TrimSpace(loc.Owner)
		repo := strings.TrimSpace(loc.Repo)
		if owner != "" && repo != "" {
			return owner, repo, true
		}
	}
	return "", "", false
}

type releaseImportListItem struct {
	Tag         string `json:"tag"`
	Title       string `json:"title"`
	Changelog   string `json:"changelog"`
	AssetName   string `json:"assetName"`
	PublishedAt string `json:"publishedAt"`
}

func listReleaseImportReleases(ctx context.Context, owner, repo string) ([]releaseImportListItem, error) {
	rawURL := strings.TrimRight(productUpdateGitHubAPI, "/") + "/repos/" + owner + "/" + repo + "/releases?per_page=20"
	body, err := releaseImportGet(ctx, rawURL, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Assets      []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, err
	}
	list := make([]releaseImportListItem, 0, len(releases))
	for _, release := range releases {
		if release.Draft || strings.TrimSpace(release.TagName) == "" {
			continue
		}
		asset := chooseReleaseAsset(release.Assets)
		title, changelog := releaseImportDescribe(ctx, owner, repo, release.TagName, release.Name, release.Body)
		list = append(list, releaseImportListItem{
			Tag: release.TagName, Title: title,
			Changelog: changelog, AssetName: asset,
			PublishedAt: release.PublishedAt,
		})
	}
	return list, nil
}

func fetchReleaseImportAsset(ctx context.Context, owner, repo, tag, assetName string) (releaseImportView, error) {
	ref := tag
	if !strings.Contains(ref, "/") {
		ref = "tags/" + tag
	}
	if assetName == "" {
		listed, err := listReleaseImportReleases(ctx, owner, repo)
		if err != nil {
			return releaseImportView{}, errors.New("无法读取仓库发布列表，请检查已保存的令牌")
		}
		for _, item := range listed {
			if item.Tag == tag {
				assetName = item.AssetName
				break
			}
		}
	}
	if assetName == "" || !safeOnlineUpdateAssetName(assetName) {
		return releaseImportView{}, errors.New("这个发布里没有可导入的安装包")
	}
	payload, err := fetchGitHubReleaseAsset(ctx, owner, repo, ref, assetName)
	if err != nil {
		return releaseImportView{}, errors.New("无法下载所选安装包，请检查已保存的令牌")
	}
	version := releaseImportVersion(tag)
	title := "v" + version
	notes := ""
	listed, listErr := listReleaseImportReleases(ctx, owner, repo)
	if listErr == nil {
		for _, item := range listed {
			if item.Tag == tag {
				if item.Title != "" {
					title = item.Title
				}
				notes = item.Changelog
				break
			}
		}
	}
	stageID, meta, err := stageReleaseBytes(assetName, payload, version, title, notes)
	if err != nil {
		return releaseImportView{}, errors.New("保存安装包失败")
	}
	return releaseImportView{
		StagingID: stageID, Version: version, Title: title, Changelog: notes,
		FileName: meta.FileName, FileSizeBytes: meta.Size, FileMd5: meta.MD5, FileSha256: meta.SHA256,
		AssetName: assetName,
	}, nil
}

func chooseReleaseAsset(assets []struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}) string {
	bestScore := -1
	best := ""
	for _, asset := range assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" || strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		score := 1
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "auth_pro-full") && strings.HasSuffix(lower, ".tar.gz"):
			score = 5
		case strings.HasSuffix(lower, ".tar.gz"):
			score = 4
		case strings.HasSuffix(lower, ".zip"):
			score = 3
		}
		if score > bestScore || (score == bestScore && asset.Size > 0 && best == "") {
			bestScore = score
			best = name
		}
	}
	return best
}

func releaseImportVersion(tag string) string {
	version := strings.TrimSpace(tag)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	if validVersion(version) {
		return version
	}
	return strings.TrimSpace(tag)
}

// releaseImportDescribe 优先用 Release 附件 latest.json 的 notes 填标题和更新内容。
// 服务端仓库和客户端仓库走同一条路径，私有仓库用已经保存的令牌。
// 没有 notes 时才退回 Release 正文，并丢掉自动生成的变更模板。
func releaseImportDescribe(ctx context.Context, owner, repo, tag, name, body string) (string, string) {
	if title, changelog, ok := releaseImportLatestNotes(ctx, owner, repo, tag); ok {
		return title, changelog
	}
	title := strings.TrimSpace(name)
	if title == "" || releaseImportTitleIsTag(title, tag) {
		title = "v" + releaseImportVersion(tag)
	}
	return title, releaseImportChangelog(body)
}

func releaseImportTitleIsTag(title, tag string) bool {
	title = strings.TrimSpace(title)
	version := releaseImportVersion(tag)
	return strings.EqualFold(title, tag) || strings.EqualFold(title, "v"+version) || title == version
}

func releaseImportLatestNotes(ctx context.Context, owner, repo, tag string) (string, string, bool) {
	ref := strings.TrimSpace(tag)
	if ref == "" {
		return "", "", false
	}
	if !strings.Contains(ref, "/") {
		ref = "tags/" + ref
	}
	payload, err := fetchGitHubReleaseAsset(ctx, owner, repo, ref, "latest.json")
	if err != nil || len(payload) == 0 {
		return "", "", false
	}
	var manifest struct {
		Notes []string `json:"notes"`
	}
	if json.Unmarshal(payload, &manifest) != nil {
		return "", "", false
	}
	return releaseImportTitleAndChangelog(manifest.Notes)
}

func releaseImportTitleAndChangelog(notes []string) (string, string, bool) {
	lines := releaseImportVisibleNotes(notes)
	if len(lines) == 0 {
		return "", "", false
	}
	title := strings.TrimSpace(strings.TrimLeft(lines[0], "-"))
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", false
	}
	body := lines
	// 第一行是短标题、后面还有条目时，更新内容只保留后面的条目。
	if len(lines) > 1 && !strings.HasPrefix(strings.TrimSpace(lines[0]), "-") && len([]rune(title)) <= 40 {
		body = lines[1:]
	}
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	return title, strings.Join(body, "\n"), true
}

func releaseImportVisibleNotes(notes []string) []string {
	lines := make([]string, 0, len(notes))
	for _, note := range notes {
		for _, line := range strings.Split(note, "\n") {
			if !releaseImportLineKept(line) {
				continue
			}
			lines = append(lines, strings.TrimSpace(line))
			if len(lines) >= 8 {
				return lines
			}
		}
	}
	return lines
}

func releaseImportChangelog(body string) string {
	return strings.Join(releaseImportVisibleNotes([]string{body}), "\n")
}

// releaseImportLineKept 丢掉托管站链接，以及自动生成的变更说明模板。
func releaseImportLineKept(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !productUpdateNoteVisible(line) {
		return false
	}
	trimmed := strings.Trim(line, "#*_ \t")
	lower := strings.ToLower(trimmed)
	if lower == "what's changed" || lower == "whats changed" || lower == "what’s changed" {
		return false
	}
	if strings.Contains(lower, "full changelog") || strings.HasPrefix(lower, "new contributors") {
		return false
	}
	return true
}

func releaseImportGet(ctx context.Context, rawURL, accept string) ([]byte, error) {
	var last error
	for _, token := range productUpdateTokenCandidates() {
		body, err := productUpdateFetch(ctx, rawURL, token, accept)
		if err == nil {
			return body, nil
		}
		last = err
		if token == "" {
			break
		}
	}
	if last != nil {
		return nil, last
	}
	return nil, errProductUpdateUnavailable
}

func stageReleaseBytes(name string, payload []byte, version, title, changelog string) (string, releaseStageMeta, error) {
	if len(payload) == 0 || int64(len(payload)) > appReleaseMaxBytes {
		return "", releaseStageMeta{}, errors.New("安装包为空或超过 512MB")
	}
	id, err := randomStageID()
	if err != nil {
		return "", releaseStageMeta{}, err
	}
	dir := filepath.Join(config.GetDataDir(), "release-staging", id)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", releaseStageMeta{}, err
	}
	name = sanitizePackageName(name)
	if err := os.WriteFile(filepath.Join(dir, name), payload, 0640); err != nil {
		return "", releaseStageMeta{}, err
	}
	sumMD5 := md5.Sum(payload)
	sumSHA := sha256.Sum256(payload)
	meta := releaseStageMeta{
		FileName: name, Size: int64(len(payload)),
		MD5: hex.EncodeToString(sumMD5[:]), SHA256: hex.EncodeToString(sumSHA[:]),
		Version: version, Title: title, Changelog: changelog,
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "", releaseStageMeta{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0640); err != nil {
		return "", releaseStageMeta{}, err
	}
	return id, meta, nil
}

func randomStageID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func readReleaseStage(id string) ([]byte, releaseStageMeta, error) {
	id = strings.TrimSpace(id)
	if !releaseStageIDPattern.MatchString(id) {
		return nil, releaseStageMeta{}, errors.New("导入文件已失效，请重新导入")
	}
	dir := filepath.Join(config.GetDataDir(), "release-staging", id)
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, releaseStageMeta{}, errors.New("导入文件已失效，请重新导入")
	}
	var meta releaseStageMeta
	if err := json.Unmarshal(raw, &meta); err != nil || meta.FileName == "" {
		return nil, releaseStageMeta{}, errors.New("导入文件已失效，请重新导入")
	}
	info, err := os.Stat(dir)
	if err != nil || time.Since(info.ModTime()) > releaseImportStageTTL {
		return nil, releaseStageMeta{}, errors.New("导入文件已失效，请重新导入")
	}
	payload, err := os.ReadFile(filepath.Join(dir, meta.FileName))
	if err != nil {
		return nil, releaseStageMeta{}, errors.New("导入文件已失效，请重新导入")
	}
	return payload, meta, nil
}

// consumeReleaseStage 把暂存安装包搬进应用发布目录。
// 输入是暂存编号，输出文件名、相对路径、大小、MD5 和 SHA256。编号不对或文件过期时返回错误。
func consumeReleaseStage(appID int64, stagingID string) (string, string, int64, string, string, error) {
	payload, meta, err := readReleaseStage(stagingID)
	if err != nil {
		return "", "", 0, "", "", err
	}
	appDir := filepath.Join(config.GetAppReleaseDir(), "app-"+strconv.FormatInt(appID, 10))
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", "", 0, "", "", errors.New("创建更新包目录失败")
	}
	target, err := os.CreateTemp(appDir, "release-*")
	if err != nil {
		return "", "", 0, "", "", errors.New("创建更新包文件失败")
	}
	targetPath := target.Name()
	if _, err := target.Write(payload); err != nil {
		_ = target.Close()
		_ = os.Remove(targetPath)
		return "", "", 0, "", "", errors.New("保存更新包失败")
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(targetPath)
		return "", "", 0, "", "", errors.New("保存更新包失败")
	}
	relative, err := filepath.Rel(config.GetAppReleaseDir(), targetPath)
	if err != nil {
		_ = os.Remove(targetPath)
		return "", "", 0, "", "", errors.New("生成更新包路径失败")
	}
	_ = os.RemoveAll(filepath.Join(config.GetDataDir(), "release-staging", stagingID))
	return meta.FileName, filepath.ToSlash(relative), meta.Size, meta.MD5, meta.SHA256, nil
}

func releaseImportFetchError(err error) string {
	if err == nil {
		return "下载失败"
	}
	if isSafeFetchPolicyError(err) || errors.Is(err, errSafeTooLarge) {
		return err.Error()
	}
	return "无法下载该地址，请确认它是可公开访问的 https 链接"
}
