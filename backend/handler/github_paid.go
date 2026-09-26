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
	githubPackagePrefix                 = "github:"
	githubPaidTokenSettingKey           = "github_paid_token"
	paidLegacyFulfillmentNote           = "需要改成私有仓库来源或上传 zip 才能继续收费出售"
	githubPaidTokenMissingText          = "请先在软件源设置里配置 GitHub 只读令牌"
	githubPaidDeveloperTokenMissingText = "请先在开发者面板配置 GitHub 只读令牌"
	githubPaidTokenInvalidText          = "GitHub 只读令牌无效或已过期，请到软件源设置重新配置"
	githubPaidDeveloperTokenInvalidText = "GitHub 只读令牌无效或已过期，请到开发者面板重新配置"
	githubPaidAssetMissingText          = "私有仓库里找不到该安装包，请核对 Release 标签和文件名"
)

var (
	errGitHubPaidTokenMissing          = errors.New(githubPaidTokenMissingText)
	errDeveloperGitHubPaidTokenMissing = errors.New(githubPaidDeveloperTokenMissingText)
	errGitHubPaidTokenInvalid          = errors.New(githubPaidTokenInvalidText)
	errDeveloperGitHubPaidTokenInvalid = errors.New(githubPaidDeveloperTokenInvalidText)
	errGitHubPaidAssetMissing          = errors.New(githubPaidAssetMissingText)
)

func withDeveloperTokenError(developerID int64, err error) error {
	if developerID <= 0 || err == nil {
		return err
	}
	if errors.Is(err, errGitHubPaidTokenMissing) {
		return errDeveloperGitHubPaidTokenMissing
	}
	if errors.Is(err, errGitHubPaidTokenInvalid) {
		return errDeveloperGitHubPaidTokenInvalid
	}
	return err
}

func rejectCatalogPackageSource(source, location string, price int64) error {
	switch strings.TrimSpace(source) {
	case "public":
		if price > 0 && !isPrivatePackageRef(location) && !isStationHostedPackageURL(location) && !isGitHubPackageRef(location) {
			return errors.New("公开地址只能用于免费条目。收费请改用私有 GitHub 仓库，或上传压缩包由本站托管")
		}
	case "github":
		if price <= 0 {
			return errors.New("私有 GitHub 仓库只用于收费条目，请填写大于 0 的售价")
		}
		if !isGitHubPackageRef(location) && !looksLikeGitHubReleaseAssetURL(location) {
			return errors.New("请粘贴 GitHub Release 资产链接，例如 https://github.com/所有者/仓库/releases/download/标签/文件名.zip")
		}
	}
	return nil
}

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
		return "", errors.New("请填写 GitHub 只读令牌")
	}
	key, err := loadOrCreateStoreFileKey("github-paid.key")
	if err != nil {
		return "", errors.New("保存 GitHub 只读令牌失败")
	}
	blob, err := sealStoreSecret(key, []byte(plain))
	if err != nil {
		return "", errors.New("保存 GitHub 只读令牌失败")
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
		return "", errors.New("读取 GitHub 只读令牌失败")
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
		return "", errors.New("读取 GitHub 只读令牌失败")
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
		return errors.New("保存 GitHub 只读令牌失败")
	}
}

func loadGitHubPaidToken() (string, error) {
	return loadGitHubPaidTokenFor(0)
}

func loadGitHubPaidTokenFor(developerID int64) (string, error) {
	var sealed string
	var err error
	if developerID > 0 {
		sealed, err = readDeveloperGitHubPaidTokenSealed(developerID)
	} else {
		sealed, err = readGitHubPaidTokenSealed()
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sealed) == "" {
		if developerID > 0 {
			return "", errDeveloperGitHubPaidTokenMissing
		}
		return "", errGitHubPaidTokenMissing
	}
	token, err := openGitHubPaidToken(sealed)
	if err != nil {
		return "", withDeveloperTokenError(developerID, err)
	}
	return token, nil
}

func readDeveloperGitHubPaidTokenSealed(developerID int64) (string, error) {
	if developerID <= 0 {
		return "", errDeveloperGitHubPaidTokenMissing
	}
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.developerGitHubTokens == nil {
			return "", nil
		}
		return store.developerGitHubTokens[developerID], nil
	case mysqlSourceStore:
		db, err := config.DB()
		if err != nil {
			return "", err
		}
		var sealed string
		err = db.QueryRow(`SELECT COALESCE(github_paid_token, '') FROM source_developers WHERE id=?`, developerID).Scan(&sealed)
		if errors.Is(err, sql.ErrNoRows) {
			return "", errSourceNotFound
		}
		return sealed, err
	default:
		return "", errors.New("读取 GitHub 只读令牌失败")
	}
}

func writeDeveloperGitHubPaidTokenSealed(developerID int64, sealed string) error {
	if developerID <= 0 {
		return errors.New("保存 GitHub 只读令牌失败")
	}
	switch store := currentSourceStationStore().(type) {
	case *memorySourceStore:
		store.mu.Lock()
		if store.developerGitHubTokens == nil {
			store.developerGitHubTokens = map[int64]string{}
		}
		store.developerGitHubTokens[developerID] = sealed
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
		result, err := db.Exec(`UPDATE source_developers SET github_paid_token=? WHERE id=?`, sealed, developerID)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return errSourceNotFound
		}
		return nil
	default:
		return errors.New("保存 GitHub 只读令牌失败")
	}
}

func developerGitHubPaidTokenConfigured(developerID int64) bool {
	token, err := loadGitHubPaidTokenFor(developerID)
	return err == nil && token != ""
}

func githubPaidTokenConfigured() bool {
	token, err := loadGitHubPaidToken()
	return err == nil && token != ""
}

func AdminGitHubPaidToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"configured": githubPaidTokenConfigured()}})
}

func AdminGitHubPaidTokenSave(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" || isMaskedSourceToken(token) {
		if githubPaidTokenConfigured() {
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保留现有 GitHub 只读令牌", "data": gin.H{"configured": true}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写 GitHub 只读令牌"})
		return
	}
	sealed, err := sealGitHubPaidToken(token)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if err := writeGitHubPaidTokenSealed(sealed); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存 GitHub 只读令牌失败"})
		return
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: c.GetString("username"), Action: "settings",
		TargetType: "github_paid", TargetID: "token", Detail: "已更新 GitHub 只读令牌",
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存 GitHub 只读令牌（页面不回显明文）", "data": gin.H{"configured": true}})
}

func AdminGitHubPaidTokenTest(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
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
	if err := probeGitHubPaidToken(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "令牌可用", "data": gin.H{"configured": true}})
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
		return errors.New("GitHub 只读令牌无法使用")
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

func downloadGitHubPaidZip(ctx context.Context, token string, ref gitHubAssetRef) ([]byte, error) {
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return nil, err
	}
	asset, err := githubAssetByName(release, ref.Asset)
	if err != nil {
		return nil, err
	}
	tempURL, err := githubAssetTemporaryURL(ctx, token, ref, asset.ID, false)
	if err != nil {
		return nil, err
	}
	payload, err := safeHTTPGet(ctx, tempURL, safeFetchOptions{
		AllowPrivate: false,
		RequireHTTPS: true,
		MaxBytes:     pluginPackageMaxSize,
		Timeout:      paidOriginFetchTimeout,
		MaxRedirects: defaultSafeRedirects,
		UserAgent:    "auth-pro-paid-import",
		Accept:       "application/zip,*/*",
		BaseClient:   externalPackageClient,
	})
	if err != nil {
		return nil, paidOriginFetchError(err)
	}
	if !isZipPayload(payload) {
		return nil, errors.New("私有仓库里的文件不是 ZIP")
	}
	return payload, nil
}

func importGitHubPaidMetadata(ctx context.Context, developerID int64, kind, category, rawURL string) (paidImportResult, error) {
	ref, err := parseGitHubReleaseAssetURL(rawURL)
	if err != nil {
		return paidImportResult{}, err
	}
	token, err := loadGitHubPaidTokenFor(developerID)
	if err != nil {
		return paidImportResult{}, err
	}
	payload, err := downloadGitHubPaidZip(ctx, token, ref)
	if err != nil {
		return paidImportResult{}, withDeveloperTokenError(developerID, err)
	}
	manifest, err := parseSourcePackageBytes(ref.Asset, payload, kind, category)
	payload = nil
	if err != nil {
		return paidImportResult{}, err
	}
	return paidImportResult{
		Ref: formatGitHubPackageRef(ref), SHA256: manifest.SHA256, Version: manifest.Version, Health: paidOriginHealthOK,
	}, nil
}

func readGitHubPaidSource(c *gin.Context, location string) (string, []byte, string, error) {
	cents, err := parseCatalogPriceCents(c.PostForm("priceCents"))
	if err != nil {
		return "", nil, "", rejectSourcePackage("priceCents", "invalid", err.Error())
	}
	if cents <= 0 {
		return "", nil, "", rejectSourcePackage("priceCents", "github_paid", "私有 GitHub 仓库只用于收费条目，请填写大于 0 的售价")
	}
	ref, err := parseGitHubReleaseAssetURL(location)
	if err != nil {
		return "", nil, "", rejectSourcePackage("downloadUrl", "github_url", err.Error())
	}
	token, err := loadGitHubPaidToken()
	if err != nil {
		return "", nil, "", rejectSourcePackage("token", "github_token", err.Error())
	}
	payload, err := downloadGitHubPaidZip(c.Request.Context(), token, ref)
	if err != nil {
		return "", nil, "", rejectSourcePackage("downloadUrl", "github_fetch", err.Error())
	}
	return ref.Asset, payload, formatGitHubPackageRef(ref), nil
}

func shouldReadGitHubPaid(c *gin.Context, location string) bool {
	if c.PostForm("packageSource") == "public" {
		return false
	}
	if c.PostForm("packageSource") == "github" {
		return true
	}
	cents, err := parseCatalogPriceCents(c.PostForm("priceCents"))
	if err != nil || cents <= 0 {
		return false
	}
	return looksLikeGitHubReleaseAssetURL(location)
}

func authorizeGitHubBuyerURL(ctx context.Context, allowed bool, location string, developerID int64) (string, error) {
	if !allowed {
		return "", errStoreDownloadDenied
	}
	return githubBuyerTemporaryURL(ctx, location, developerID)
}

func githubBuyerTemporaryURL(ctx context.Context, location string, developerID int64) (string, error) {
	ref, ok := parseGitHubPackageRef(location)
	if !ok {
		return "", errors.New("付费包不存在")
	}
	token, err := loadGitHubPaidTokenFor(developerID)
	if err != nil {
		return "", err
	}
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return "", withDeveloperTokenError(developerID, err)
	}
	asset, err := githubAssetByName(release, ref.Asset)
	if err != nil {
		return "", err
	}
	tempURL, err := githubAssetTemporaryURL(ctx, token, ref, asset.ID, true)
	if err != nil {
		return "", withDeveloperTokenError(developerID, err)
	}
	return tempURL, nil
}

func probeGitHubPaidAsset(ctx context.Context, location string, developerID int64) error {
	ref, ok := parseGitHubPackageRef(location)
	if !ok {
		return nil
	}
	token, err := loadGitHubPaidTokenFor(developerID)
	if err != nil {
		return err
	}
	release, err := githubReleaseByTag(ctx, token, ref)
	if err != nil {
		return withDeveloperTokenError(developerID, err)
	}
	_, err = githubAssetByName(release, ref.Asset)
	return err
}

func touchGitHubPaidHealth(ctx context.Context, store sourceStationStore, kind, id, name string, price int64, location, current string, developerID int64) {
	if price <= 0 || !isGitHubPackageRef(location) {
		return
	}
	health := paidOriginHealthOK
	probeErr := probeGitHubPaidAsset(ctx, location, developerID)
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
			view["originHint"] = "私有仓库安装包暂时无法访问，请检查只读令牌或 Release 文件。条目仍在目录中，不会自动下架。"
		}
	}
	if hint := paidLegacyFulfillmentHint(price, location); hint != "" {
		view["fulfillmentHint"] = hint
	}
}

func DeveloperGitHubPaidToken(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"configured": developerGitHubPaidTokenConfigured(developer.ID)}})
}

func DeveloperGitHubPaidTokenSave(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" || isMaskedSourceToken(token) {
		if developerGitHubPaidTokenConfigured(developer.ID) {
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保留现有 GitHub 只读令牌", "data": gin.H{"configured": true}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写 GitHub 只读令牌"})
		return
	}
	sealed, err := sealGitHubPaidToken(token)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if err := writeDeveloperGitHubPaidTokenSealed(developer.ID, sealed); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存 GitHub 只读令牌失败"})
		return
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "developer", ActorName: developer.Username, Action: "settings",
		TargetType: "github_paid", TargetID: fmt.Sprintf("%d", developer.ID), Detail: "已更新开发者 GitHub 只读令牌",
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存 GitHub 只读令牌（页面不回显明文）", "data": gin.H{"configured": true}})
}

func DeveloperGitHubPaidTokenTest(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = c.ShouldBindJSON(&body)
	token := strings.TrimSpace(body.Token)
	if token == "" || isMaskedSourceToken(token) {
		token, err = loadGitHubPaidTokenFor(developer.ID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	}
	if err := probeGitHubPaidToken(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": withDeveloperTokenError(developer.ID, err).Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "令牌可用", "data": gin.H{"configured": true}})
}
