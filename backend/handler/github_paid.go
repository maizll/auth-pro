package handler

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	githubPackagePrefix        = "github:"
	githubPaidTokenSettingKey  = "github_paid_token"
	githubPaidOwnerSettingKey  = "github_paid_owner"
	githubPaidRepoSettingKey   = "github_paid_repo"
	paidLegacyFulfillmentNote  = "需要重新上传 zip 或填写公开地址，才能继续收费出售"
	githubPaidTokenMissingText = "请先在软件源设置里配置收费仓库和 GitHub 令牌"
	githubPaidTokenInvalidText = "GitHub 令牌无效或已过期，请到软件源设置重新配置。令牌需要 Contents 读写权限"
	githubPaidAssetMissingText = "收费仓库里找不到该安装包，请重新上传"
	paidLocalFallbackText      = "尚未配置收费仓库，安装包暂存在本站。请到软件源设置填写私有 GitHub 仓库和 Contents 读写令牌。"
)

var (
	errGitHubPaidTokenMissing = errors.New(githubPaidTokenMissingText)
	errGitHubPaidTokenInvalid = errors.New(githubPaidTokenInvalidText)
	errGitHubPaidAssetMissing = errors.New(githubPaidAssetMissingText)
)

type gitHubAssetRef struct {
	Owner string
	Repo  string
	Tag   string
	Asset string
}

func paidExternalNeedsRestore(price int64, status, location string) bool {
	if price <= 0 {
		return false
	}
	status = strings.TrimSpace(status)
	if status != sourceItemHidden && status != sourceItemDeprecated {
		return false
	}
	location = strings.TrimSpace(location)
	if !strings.HasPrefix(strings.ToLower(location), "https://") {
		return false
	}
	if isPrivatePackageRef(location) || isGitHubPackageRef(location) {
		return false
	}
	return true
}

func appendPaidLegacyNote(note string) string {
	note = strings.TrimSpace(note)
	if strings.Contains(note, paidLegacyFulfillmentNote) {
		return note
	}
	if note == "" {
		return paidLegacyFulfillmentNote
	}
	combined := note + "；" + paidLegacyFulfillmentNote
	if len([]rune(combined)) > 500 {
		combined = string([]rune(combined)[:500])
	}
	return combined
}

func paidLegacyFulfillmentHint(price int64, location string) string {
	location = strings.TrimSpace(location)
	if price <= 0 || location == "" {
		return ""
	}
	if isPrivatePackageRef(location) || isGitHubPackageRef(location) || isStationHostedPackageURL(location) {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(location), "https://") {
		return ""
	}
	return paidLegacyFulfillmentNote
}

func migratePaidExternalVisible(db *sql.DB) error {
	note := paidLegacyFulfillmentNote
	for _, statement := range []string{
		`UPDATE source_catalog_plugins
			SET status='draft',
				review_note = IF(review_note LIKE ?, review_note, IF(TRIM(review_note)='', ?, CONCAT(LEFT(review_note, 420), '；', ?)))
			WHERE price_cents > 0 AND status IN ('hidden','deprecated') AND download_url LIKE 'https://%'
			/* paid external restore */`,
		`UPDATE source_catalog_templates
			SET status='draft',
				review_note = IF(review_note LIKE ?, review_note, IF(TRIM(review_note)='', ?, CONCAT(LEFT(review_note, 420), '；', ?)))
			WHERE price_cents > 0 AND status IN ('hidden','deprecated') AND template_url LIKE 'https://%'
			/* paid external restore */`,
	} {
		like := "%" + note + "%"
		if _, err := db.Exec(statement, like, note, note); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unknown column") || strings.Contains(strings.ToLower(err.Error()), "doesn't exist") {
				continue
			}
			return fmt.Errorf("restore paid external catalog rows: %w", err)
		}
	}
	return nil
}

func isGitHubPackageRef(raw string) bool {
	_, ok := parseGitHubPackageRef(raw)
	return ok
}

func parseGitHubPackageRef(raw string) (gitHubAssetRef, bool) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, githubPackagePrefix) {
		return gitHubAssetRef{}, false
	}
	parts := strings.Split(strings.TrimPrefix(value, githubPackagePrefix), "/")
	if len(parts) != 4 {
		return gitHubAssetRef{}, false
	}
	ref := gitHubAssetRef{Owner: parts[0], Repo: parts[1], Tag: parts[2], Asset: parts[3]}
	if !validGitHubAssetRef(ref) {
		return gitHubAssetRef{}, false
	}
	return ref, true
}

func formatGitHubPackageRef(ref gitHubAssetRef) string {
	return githubPackagePrefix + ref.Owner + "/" + ref.Repo + "/" + ref.Tag + "/" + ref.Asset
}

func validGitHubAssetRef(ref gitHubAssetRef) bool {
	if !sourceReleaseRepoPattern.MatchString(ref.Owner) || !sourceReleaseRepoPattern.MatchString(ref.Repo) {
		return false
	}
	if ref.Tag == "" || ref.Asset == "" || strings.Contains(ref.Tag, "/") || strings.Contains(ref.Asset, "/") {
		return false
	}
	if strings.Contains(ref.Tag, "..") || strings.Contains(ref.Asset, "..") {
		return false
	}
	return true
}

func looksLikeGitHubReleaseAssetURL(raw string) bool {
	_, err := parseGitHubReleaseAssetURL(raw)
	return err == nil
}

func parseGitHubReleaseAssetURL(raw string) (gitHubAssetRef, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return gitHubAssetRef{}, errors.New("请粘贴 GitHub Release 资产链接，例如 https://github.com/所有者/仓库/releases/download/标签/文件名.zip")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" {
		return gitHubAssetRef{}, errors.New("请粘贴 GitHub Release 资产链接，例如 https://github.com/所有者/仓库/releases/download/标签/文件名.zip")
	}
	owner, errOwner := url.PathUnescape(parts[0])
	repo, errRepo := url.PathUnescape(parts[1])
	tag, errTag := url.PathUnescape(parts[4])
	asset, errAsset := url.PathUnescape(parts[5])
	if errOwner != nil || errRepo != nil || errTag != nil || errAsset != nil {
		return gitHubAssetRef{}, errors.New("GitHub Release 资产链接无法解析")
	}
	ref := gitHubAssetRef{Owner: owner, Repo: repo, Tag: tag, Asset: asset}
	if !validGitHubAssetRef(ref) {
		return gitHubAssetRef{}, errors.New("GitHub Release 资产链接里的所有者、仓库、标签或文件名不合法")
	}
	return ref, nil
}

func sealGitHubPaidToken(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", errors.New("请填写 GitHub 令牌")
	}
	key, err := loadOrCreateStoreFileKey("github-paid.key")
	if err != nil {
		return "", errors.New("保存 GitHub 令牌失败")
	}
	blob, err := sealStoreSecret(key, []byte(plain))
	if err != nil {
		return "", errors.New("保存 GitHub 令牌失败")
	}
	return base64.StdEncoding.EncodeToString(blob), nil
}

func openGitHubPaidToken(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sealed))
	if err != nil {
		return "", errGitHubPaidTokenInvalid
	}
	key, err := loadOrCreateStoreFileKey("github-paid.key")
	if err != nil {
		return "", errors.New("读取 GitHub 令牌失败")
	}
	plain, err := openStoreSecret(key, raw)
	if err != nil {
		return "", errGitHubPaidTokenInvalid
	}
	token := strings.TrimSpace(string(plain))
	if token == "" {
		return "", errGitHubPaidTokenMissing
	}
	return token, nil
}

func readGitHubPaidTokenSealed() (string, error) {
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.githubPaidToken, nil
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return "", err
		}
		if err := ensureSourceStationStorage(db); err != nil {
			return "", err
		}
		var raw string
		err = db.QueryRow(`SELECT setting_value FROM source_station_settings WHERE setting_key=?`, githubPaidTokenSettingKey).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return raw, err
	default:
		return "", errors.New("读取 GitHub 令牌失败")
	}
}

func writeGitHubPaidTokenSealed(sealed string) error {
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		store.githubPaidToken = sealed
		store.mu.Unlock()
		return nil
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return err
		}
		if err := ensureSourceStationStorage(db); err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO source_station_settings (setting_key, setting_value) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, githubPaidTokenSettingKey, sealed)
		return err
	default:
		return errors.New("保存 GitHub 令牌失败")
	}
}

func loadGitHubPaidToken() (string, error) {
	sealed, err := readGitHubPaidTokenSealed()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sealed) == "" {
		return "", errGitHubPaidTokenMissing
	}
	return openGitHubPaidToken(sealed)
}

func readGitHubPaidSetting(key string) (string, error) {
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		switch key {
		case githubPaidOwnerSettingKey:
			return store.githubPaidOwner, nil
		case githubPaidRepoSettingKey:
			return store.githubPaidRepo, nil
		default:
			return "", nil
		}
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return "", err
		}
		if err := ensureSourceStationStorage(db); err != nil {
			return "", err
		}
		var raw string
		err = db.QueryRow(`SELECT setting_value FROM source_station_settings WHERE setting_key=?`, key).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return raw, err
	default:
		return "", errors.New("读取收费仓库设置失败")
	}
}

func writeGitHubPaidSetting(key, value string) error {
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		switch key {
		case githubPaidOwnerSettingKey:
			store.githubPaidOwner = value
		case githubPaidRepoSettingKey:
			store.githubPaidRepo = value
		}
		store.mu.Unlock()
		return nil
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return err
		}
		if err := ensureSourceStationStorage(db); err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO source_station_settings (setting_key, setting_value) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, key, value)
		return err
	default:
		return errors.New("保存收费仓库设置失败")
	}
}

func githubPaidRepoConfigured() bool {
	owner, repo, token, err := loadGitHubPaidRepo()
	return err == nil && owner != "" && repo != "" && token != ""
}

func loadGitHubPaidRepo() (owner, repo, token string, err error) {
	owner, err = readGitHubPaidSetting(githubPaidOwnerSettingKey)
	if err != nil {
		return "", "", "", err
	}
	repo, err = readGitHubPaidSetting(githubPaidRepoSettingKey)
	if err != nil {
		return "", "", "", err
	}
	token, err = loadGitHubPaidToken()
	if err != nil && !errors.Is(err, errGitHubPaidTokenMissing) {
		return owner, repo, "", err
	}
	if errors.Is(err, errGitHubPaidTokenMissing) {
		err = nil
	}
	return strings.TrimSpace(owner), strings.TrimSpace(repo), token, nil
}

func githubPaidSettingsView() gin.H {
	owner, repo, _, _ := loadGitHubPaidRepo()
	configured := githubPaidRepoConfigured()
	reminder := ""
	if !configured {
		reminder = paidLocalFallbackText
	}
	return gin.H{"configured": configured, "owner": owner, "repo": repo, "reminder": reminder}
}

func AdminGitHubPaidToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": githubPaidSettingsView()})
}

func AdminGitHubPaidTokenSave(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	owner := strings.TrimSpace(body.Owner)
	repo := strings.TrimSpace(body.Repo)
	if owner == "" || repo == "" || !sourceReleaseRepoPattern.MatchString(owner) || !sourceReleaseRepoPattern.MatchString(repo) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写私有仓库的所有者和仓库名"})
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" || isMaskedSourceToken(token) {
		if _, err := loadGitHubPaidToken(); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写 GitHub 令牌，需要 Contents 读写权限"})
			return
		}
	} else {
		sealed, err := sealGitHubPaidToken(token)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
			return
		}
		if err := writeGitHubPaidTokenSealed(sealed); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存 GitHub 令牌失败"})
			return
		}
	}
	if err := writeGitHubPaidSetting(githubPaidOwnerSettingKey, owner); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存收费仓库失败"})
		return
	}
	if err := writeGitHubPaidSetting(githubPaidRepoSettingKey, repo); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存收费仓库失败"})
		return
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: c.GetString("username"), Action: "settings",
		TargetType: "github_paid", TargetID: owner + "/" + repo, Detail: "已更新收费仓库",
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存收费仓库（页面不回显令牌明文）", "data": githubPaidSettingsView()})
}

func AdminGitHubPaidTokenTest(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	_ = c.ShouldBindJSON(&body)
	token := strings.TrimSpace(body.Token)
	if token == "" || isMaskedSourceToken(token) {
		var err error
		token, err = loadGitHubPaidToken()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	}
	owner := strings.TrimSpace(body.Owner)
	repo := strings.TrimSpace(body.Repo)
	if owner == "" || repo == "" {
		savedOwner, savedRepo, _, _ := loadGitHubPaidRepo()
		owner, repo = savedOwner, savedRepo
	}
	if err := probeGitHubPaidRepo(c.Request.Context(), token, owner, repo); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "令牌可用", "data": githubPaidSettingsView()})
}

func probeGitHubPaidRepo(ctx context.Context, token, owner, repo string) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return errors.New("请填写私有仓库的所有者和仓库名")
	}
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	status, _, err := githubPaidJSON(ctx, http.MethodGet, rawURL, token, nil)
	if err != nil {
		return errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return errGitHubPaidTokenInvalid
	}
	if status == http.StatusNotFound || status == http.StatusForbidden {
		return errors.New("找不到该私有仓库，或令牌没有访问权限")
	}
	if status < 200 || status >= 300 {
		return errors.New("GitHub 令牌无法使用，请确认 Contents 为读写")
	}
	return nil
}

func probeGitHubPaidToken(ctx context.Context, token string) error {
	status, _, err := githubPaidJSON(ctx, http.MethodGet, strings.TrimRight(sourceGitHubAPIBase, "/")+"/rate_limit", token, nil)
	if err != nil {
		return errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return errGitHubPaidTokenInvalid
	}
	if status < 200 || status >= 300 {
		return errors.New("GitHub 令牌无法使用，请确认 Contents 为读写")
	}
	return nil
}

func githubPaidJSON(ctx context.Context, method, rawURL, token string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := sourceReleaseHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, payload, nil
}

func githubReleaseByTag(ctx context.Context, token string, ref gitHubAssetRef) (gitHubReleaseDTO, error) {
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/tags/" + url.PathEscape(ref.Tag)
	status, payload, err := githubPaidJSON(ctx, http.MethodGet, rawURL, token, nil)
	if err != nil {
		return gitHubReleaseDTO{}, errors.New("无法连接 GitHub，请稍后再试")
	}
	if status == http.StatusUnauthorized {
		return gitHubReleaseDTO{}, errGitHubPaidTokenInvalid
	}
	if status == http.StatusNotFound {
		return gitHubReleaseDTO{}, errGitHubPaidAssetMissing
	}
	if status < 200 || status >= 300 {
		return gitHubReleaseDTO{}, errors.New("读取私有仓库 Release 失败")
	}
	var release gitHubReleaseDTO
	if err := json.Unmarshal(payload, &release); err != nil {
		return gitHubReleaseDTO{}, errors.New("读取私有仓库 Release 失败")
	}
	return release, nil
}

func githubAssetByName(release gitHubReleaseDTO, name string) (gitHubAssetDTO, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return gitHubAssetDTO{}, errGitHubPaidAssetMissing
}

func githubAssetTemporaryURL(ctx context.Context, token string, ref gitHubAssetRef, assetID int64, forBuyer bool) (string, error) {
	rawURL := strings.TrimRight(sourceGitHubAPIBase, "/") + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/assets/" + fmt.Sprintf("%d", assetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", errors.New("无法生成下载地址")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("无法连接 GitHub，请稍后再试")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnauthorized {
		return "", errGitHubPaidTokenInvalid
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", errGitHubPaidAssetMissing
	}
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusMovedPermanently {
		return "", errors.New("暂时无法生成下载地址，请稍后再试")
	}
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if err := validateGitHubBuyerRedirect(location, forBuyer); err != nil {
		return "", err
	}
	if strings.Contains(location, token) {
		return "", errors.New("暂时无法生成下载地址，请稍后再试")
	}
	return location, nil
}

func validateGitHubBuyerRedirect(raw string, strictPort bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return errors.New("暂时无法生成下载地址，请稍后再试")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return errors.New("暂时无法生成下载地址，请稍后再试")
	}
	if strictPort {
		if port := parsed.Port(); port != "" && port != "443" {
			return errors.New("暂时无法生成下载地址，请稍后再试")
		}
	}
	return nil
}

func authorizeGitHubBuyerURL(ctx context.Context, allowed bool, location string) (string, error) {
	if !allowed {
		return "", errStoreDownloadDenied
	}
	return githubBuyerTemporaryURL(ctx, location)
}

func githubBuyerTemporaryURL(ctx context.Context, location string) (string, error) {
	ref, ok := parseGitHubPackageRef(location)
	if !ok {
		return "", errors.New("付费包不存在")
	}
	token, err := loadGitHubPaidToken()
	if err != nil {
		return "", err
	}
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return "", err
	}
	asset, err := githubAssetByName(release, ref.Asset)
	if err != nil {
		return "", err
	}
	return githubAssetTemporaryURL(ctx, token, ref, asset.ID, true)
}

func probeGitHubPaidAsset(ctx context.Context, location string) error {
	ref, ok := parseGitHubPackageRef(location)
	if !ok {
		return nil
	}
	token, err := loadGitHubPaidToken()
	if err != nil {
		return err
	}
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return err
	}
	_, err = githubAssetByName(release, ref.Asset)
	return err
}

func uploadPaidZipToStationRepo(ctx context.Context, kind, id, version string, payload []byte) (string, error) {
	owner, repo, token, err := loadGitHubPaidRepo()
	if err != nil {
		return "", err
	}
	if owner == "" || repo == "" || token == "" {
		return "", errGitHubPaidTokenMissing
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = sourceKindPlugin
	}
	manifest := sourcePackageManifest{
		ID: id, Version: version, Kind: kind, Name: id, Description: id + " " + version, Filename: id + "-" + version + ".zip",
	}
	settings := sourceReleaseSettings{
		Provider: "github", Owner: owner, Repo: repo, Token: token, TagStrategy: "paid-{kind}-{id}-{version}",
	}
	if _, err := pushGitHubRelease(ctx, settings, manifest, payload); err != nil {
		if strings.Contains(err.Error(), "401") {
			return "", errGitHubPaidTokenInvalid
		}
		return "", errors.New("上传到收费仓库失败，请检查令牌是否具备 Contents 读写权限")
	}
	ref := gitHubAssetRef{
		Owner: owner, Repo: repo,
		Tag:   renderSourceReleaseTag(settings.TagStrategy, id, version, kind),
		Asset: sourceReleaseAssetName(manifest),
	}
	if !validGitHubAssetRef(ref) {
		return "", errors.New("收费仓库里的标签或文件名不合法")
	}
	return formatGitHubPackageRef(ref), nil
}

func touchGitHubPaidHealth(ctx context.Context, store sourceStationStore, kind, id, name string, price int64, location, current string) {
	if price <= 0 || !isGitHubPackageRef(location) {
		return
	}
	health := paidOriginHealthOK
	probeErr := probeGitHubPaidAsset(ctx, location)
	if probeErr != nil {
		health = paidOriginHealthUnavailable
	}
	if health == current {
		return
	}
	if err := store.SetPaidOriginHealth(kind, id, health); err != nil {
		return
	}
	if health != paidOriginHealthUnavailable {
		return
	}
	title := "收费安装包暂时无法访问"
	body := strings.TrimSpace(name)
	if body == "" {
		body = id
	}
	body += "：" + probeErr.Error() + "。条目仍在目录中，不会自动下架。"
	notifyAllAdmins(notificationTabNotice, title, body, "/source-station/catalog", "github_paid_unavailable", kind, id)
}

func attachGitHubPaidView(view gin.H, price int64, location, health string) {
	if ref, ok := parseGitHubPackageRef(location); ok {
		view["packageSource"] = "github"
		view["githubOwner"] = ref.Owner
		view["githubRepo"] = ref.Repo
		view["githubTag"] = ref.Tag
		view["githubAsset"] = ref.Asset
		if health == paidOriginHealthUnavailable {
			view["originHealth"] = health
			view["originHint"] = "收费仓库里的安装包暂时无法访问，请检查令牌或重新上传。条目仍在目录中，不会自动下架。"
		}
	}
	if hint := paidLegacyFulfillmentHint(price, location); hint != "" {
		view["fulfillmentHint"] = hint
	}
}

func concealDeveloperPaidStorage(view gin.H) {
	delete(view, "githubOwner")
	delete(view, "githubRepo")
	delete(view, "githubTag")
	delete(view, "githubAsset")
	for _, key := range []string{"downloadUrl", "templateUrl"} {
		raw, _ := view[key].(string)
		if isGitHubPackageRef(raw) || isPrivatePackageRef(raw) {
			view[key] = ""
			view["storedBySite"] = true
			view["packageSource"] = "upload"
		}
	}
}
