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
	releaseImportStageTTL         = 2 * time.Hour
	releaseImportActorSite        = "site"
	releaseImportActorDeveloper   = "developer"
	releaseImportRepoAppKey       = "release_import_repo_app"
	releaseImportRepoCatalogKey   = "release_import_repo_catalog"
	releaseImportRepoRequiredText = "请填写仓库，格式为 所有者/名称"
	releaseImportActorContextKey  = "releaseImportActor"
)

// releaseImportCreds 把这一次导入用谁的令牌记在上下文里。
// 开发者端只允许请求里自带的令牌，或匿名访问公开仓库，不能落到站长保存的令牌。
type releaseImportCreds struct {
	actor string
	token string
}

type releaseImportCredsKey struct{}

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
func RegisterReleaseImportRoutes(group *gin.RouterGroup) {
	registerReleaseImportRoutes(group, releaseImportActorSite)
}

// RegisterDeveloperReleaseImportRoutes 挂上开发者端导入。令牌只认本次请求里填写的那一枚。
func RegisterDeveloperReleaseImportRoutes(group *gin.RouterGroup) {
	registerReleaseImportRoutes(group, releaseImportActorDeveloper)
}

func registerReleaseImportRoutes(group *gin.RouterGroup, actor string) {
	group.Use(func(c *gin.Context) {
		c.Set(releaseImportActorContextKey, actor)
		c.Next()
	})
	group.GET("/release-import/preference", ReleaseImportPreference)
	group.POST("/release-import/releases", ReleaseImportList)
	group.POST("/release-import/fetch", ReleaseImportFetch)
	group.POST("/release-import/probe-url", ReleaseImportProbeURL)
	group.POST("/release-import/materialize", ReleaseImportMaterialize)
}

// ReleaseImportPreference 返回这个用途上次保存的仓库，供输入框自动带出。
// 开发者端始终为空，避免把站长的私有仓库名交给开发者。
func ReleaseImportPreference(c *gin.Context) {
	repo := ""
	if releaseImportActorFrom(c) == releaseImportActorSite {
		saved, err := ensureOfficialImportRepo(releaseImportRepoKind(c.Query("purpose")))
		if err == nil {
			repo = saved
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"repo": repo}})
}

// ReleaseImportList 列出仓库里的 Release，供后台选择。
// 仓库留空时用本站按用途保存的地址。没有保存过就要求填写，不回落到编译进程序的默认仓库。
// 站长侧用已经保存的令牌。开发者侧只用本次填写的令牌，公开仓库可以不填。
func ReleaseImportList(c *gin.Context) {
	var req struct {
		Purpose string `json:"purpose"`
		Repo    string `json:"repo"`
		Token   string `json:"token"`
	}
	_ = c.ShouldBindJSON(&req)
	actor := releaseImportActorFrom(c)
	owner, repo, err := releaseImportRepo(actor, req.Purpose, req.Repo)
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	ctx := releaseImportContext(c, req.Token)
	releases, err := listReleaseImportReleases(ctx, owner, repo)
	if err != nil {
		apiError(c, 400, releaseImportConnectError(actor))
		return
	}
	rememberReleaseImportRepo(actor, req.Purpose, owner, repo)
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
		Token     string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Tag) == "" {
		apiError(c, 400, "请选择要导入的发布")
		return
	}
	actor := releaseImportActorFrom(c)
	owner, repo, err := releaseImportRepo(actor, req.Purpose, req.Repo)
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	view, err := fetchReleaseImportAsset(releaseImportContext(c, req.Token), owner, repo, strings.TrimSpace(req.Tag), strings.TrimSpace(req.AssetName))
	if err != nil {
		apiError(c, 400, err.Error())
		return
	}
	rememberReleaseImportRepo(actor, req.Purpose, owner, repo)
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

func releaseImportActorFrom(c *gin.Context) string {
	if c == nil {
		return releaseImportActorSite
	}
	if actor := strings.TrimSpace(c.GetString(releaseImportActorContextKey)); actor != "" {
		return actor
	}
	return releaseImportActorSite
}

func releaseImportContext(c *gin.Context, token string) context.Context {
	creds := releaseImportCreds{actor: releaseImportActorFrom(c), token: strings.TrimSpace(token)}
	return context.WithValue(c.Request.Context(), releaseImportCredsKey{}, creds)
}

func releaseImportTokenOverride(ctx context.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	creds, ok := ctx.Value(releaseImportCredsKey{}).(releaseImportCreds)
	if !ok || creds.actor != releaseImportActorDeveloper {
		return nil, false
	}
	token := strings.TrimSpace(creds.token)
	if token == "" {
		return []string{""}, true
	}
	return []string{token}, true
}

func releaseImportConnectError(actor string) string {
	if actor == releaseImportActorDeveloper {
		return "无法读取仓库发布列表，请检查填写的令牌"
	}
	return "无法读取仓库发布列表，请检查已保存的令牌"
}

func releaseImportRepoKind(purpose string) string {
	switch strings.TrimSpace(purpose) {
	case "plugin", "template":
		return "catalog"
	default:
		return "app"
	}
}

func releaseImportRepoSettingKey(kind string) string {
	if kind == "catalog" {
		return releaseImportRepoCatalogKey
	}
	return releaseImportRepoAppKey
}

// releaseImportRepo 解析本次要读的仓库。留空时只用本站保存的地址。
// 开发者端不读取站长保存的仓库。非官网也不会使用编译进程序的默认仓库。
func releaseImportRepo(actor, purpose, raw string) (string, string, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" {
		if actor == releaseImportActorDeveloper {
			return "", "", errors.New(releaseImportRepoRequiredText)
		}
		saved, err := ensureOfficialImportRepo(releaseImportRepoKind(purpose))
		if err != nil || strings.TrimSpace(saved) == "" {
			return "", "", errors.New(releaseImportRepoRequiredText)
		}
		raw = saved
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || !sourceReleaseRepoPattern.MatchString(parts[0]) || !sourceReleaseRepoPattern.MatchString(parts[1]) {
		return "", "", errors.New("仓库格式应为 所有者/名称")
	}
	return parts[0], parts[1], nil
}

func rememberReleaseImportRepo(actor, purpose, owner, repo string) {
	if actor != releaseImportActorSite || owner == "" || repo == "" {
		return
	}
	_ = writeImportRepo(releaseImportRepoKind(purpose), owner+"/"+repo)
}

func readImportRepo(kind string) (string, error) {
	key := releaseImportRepoSettingKey(kind)
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.importRepos == nil {
			return "", nil
		}
		return strings.TrimSpace(store.importRepos[key]), nil
	case mysqlSourceStore:
		return readGitHubPaidSetting(key)
	default:
		return "", errors.New("读取仓库设置失败")
	}
}

func writeImportRepo(kind, repo string) error {
	key := releaseImportRepoSettingKey(kind)
	repo = strings.TrimSpace(repo)
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.importRepos == nil {
			store.importRepos = map[string]string{}
		}
		store.importRepos[key] = repo
		return nil
	case mysqlSourceStore:
		return writeGitHubPaidSetting(key, repo)
	default:
		return errors.New("保存仓库设置失败")
	}
}

// ensureOfficialImportRepo 在官网第一次升级后，把原先写死的两个仓库写入设置。
// 客户站 officialSite 为假时这里返回空，调用方必须让用户自己填写。
func ensureOfficialImportRepo(kind string) (string, error) {
	saved, err := readImportRepo(kind)
	if err != nil || saved != "" {
		return saved, err
	}
	seed := officialImportRepoSeed(kind)
	if seed == "" {
		return "", nil
	}
	if err := writeImportRepo(kind, seed); err != nil {
		return "", err
	}
	return seed, nil
}

func officialImportRepoSeed(kind string) string {
	if !officialSite() {
		return ""
	}
	switch kind {
	case "app":
		return "maizll/auth-pro-client"
	case "catalog":
		return "maizll/auth-pro-paid"
	default:
		return ""
	}
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
		if _, developer := releaseImportTokenOverride(ctx); developer {
			return releaseImportView{}, errors.New("无法下载所选安装包，请检查填写的令牌")
		}
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
	tokens := productUpdateTokenCandidates()
	if override, ok := releaseImportTokenOverride(ctx); ok {
		tokens = override
	}
	for _, token := range tokens {
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
