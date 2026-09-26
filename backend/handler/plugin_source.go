package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	pluginSourceCacheTTL  = 5 * time.Minute
	pluginManifestMaxSize = 2 << 20
	pluginPackageMaxSize  = 20 << 20
)

type pluginSourceRecord struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	SourceType string    `json:"sourceType"`
	CreatedAt  time.Time `json:"createdAt"`
}

type remotePluginEntry struct {
	ID          string         `json:"id"`
	Category    string         `json:"category"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Icon        string         `json:"icon"`
	Version     string         `json:"version"`
	Author      templateAuthor `json:"author"`
	DownloadURL string         `json:"downloadUrl"`
	SHA256      string         `json:"sha256"`
	PriceCents  int64          `json:"priceCents"`
}

type remotePluginIndex struct {
	Name          string                  `json:"name"`
	HomeTemplates []json.RawMessage       `json:"homeTemplates"`
	Categories    []sourceCatalogCategory `json:"categories"`
	Plugins       []remotePluginEntry     `json:"plugins"`
}

type categoryGroup struct {
	Category string       `json:"category"`
	Title    string       `json:"title"`
	Plugins  []pluginInfo `json:"plugins"`
}

type cachedPluginSource struct {
	Manifest  []byte
	ExpiresAt time.Time
	LastError string
}

func ensurePluginSourceStorage(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS plugin_sources (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(60) NOT NULL DEFAULT '',
		url VARCHAR(500) NOT NULL,
		source_type VARCHAR(20) NOT NULL DEFAULT 'json',
		created_at DATETIME DEFAULT NULL,
		UNIQUE KEY uk_url (url(191))
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权系统插件软件源'`); err != nil {
		return err
	}
	if err := ensureColumn(db, "plugin_sources", "source_type",
		"ALTER TABLE plugin_sources ADD COLUMN source_type VARCHAR(20) NOT NULL DEFAULT 'json'"); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS plugin_source_cache (
		source_id BIGINT NOT NULL PRIMARY KEY,
		source_type VARCHAR(20) NOT NULL DEFAULT 'json',
		manifest_json MEDIUMTEXT NOT NULL,
		fetched_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		last_error VARCHAR(500) NOT NULL DEFAULT ''
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权系统插件清单缓存'`); err != nil {
		return err
	}
	return migrateMisclassifiedJSONPluginSources(db)
}

const (
	pluginSourceTypeJSON        = "json"
	pluginSourceTypeGit         = "git"
	errPluginSourceNotGitRepo   = "该地址不是 Git 仓库，可能是 JSON 目录，请修改源类型"
	pluginSourceCorrectedToJSON = "该地址是 JSON 目录，已自动改为 JSON 类型"
	pluginSourceCorrectedToGit  = "该地址不是 JSON 目录，已自动改为 Git 仓库"
	pluginSourceJSONFetchFailed = "JSON 目录拉取失败"
	pluginSourceJSONInvalid     = "该地址不是有效的 JSON 目录"
)

type resolvedPluginSource struct {
	index      *remotePluginIndex
	manifest   []byte
	sourceType string
	notice     string
}

func fetchPluginSourceManifest(ctx context.Context, rawURL string) (*remotePluginIndex, []byte, string, error) {
	resolved, err := resolvePluginSource(ctx, rawURL, "")
	if err != nil || resolved == nil {
		return nil, nil, "", err
	}
	return resolved.index, resolved.manifest, resolved.sourceType, nil
}

func resolvePluginSource(ctx context.Context, rawURL, requestedType string) (*resolvedPluginSource, error) {
	normalized, err := validatePluginSourceURL(rawURL)
	if err != nil {
		return nil, err
	}
	rawURL = normalized
	requested, err := normalizePluginSourceType(requestedType)
	if err != nil {
		return nil, err
	}
	if payload, index, ok := lookupLocalSoftwareSourceIndex(rawURL); ok {
		return &resolvedPluginSource{
			index: index, manifest: payload, sourceType: pluginSourceTypeJSON,
			notice: pluginSourceTypeNotice(requested, pluginSourceTypeJSON),
		}, nil
	}
	jsonURL := pluginSourceURLLooksLikeJSON(rawURL)
	tryHTTPFirst := jsonURL || requested == pluginSourceTypeJSON || !looksLikeGitRepositoryURL(rawURL)
	if tryHTTPFirst {
		resolved, handled, err := resolvePluginSourceFromHTTP(ctx, rawURL, requested, jsonURL)
		if handled || err != nil {
			return resolved, err
		}
	}
	if jsonURL {
		return nil, errors.New(pluginSourceJSONInvalid)
	}
	resolved, err := resolvePluginSourceFromGit(ctx, rawURL, requested)
	if err == nil {
		return resolved, nil
	}
	if !tryHTTPFirst {
		if httpResolved, ok := pluginSourceJSONOverHTTP(ctx, rawURL, requested); ok {
			return httpResolved, nil
		}
	}
	return nil, err
}

func resolvePluginSourceFromHTTP(ctx context.Context, rawURL, requested string, jsonURL bool) (*resolvedPluginSource, bool, error) {
	payload, err := fetchPluginHTTP(ctx, rawURL, pluginManifestMaxSize, 10*time.Second, pluginSourceAllowsPrivate(rawURL))
	if err == nil && payloadLooksLikeJSON(payload) {
		index, parseErr := parsePluginSourceManifest(payload)
		if parseErr != nil {
			return nil, true, parseErr
		}
		return &resolvedPluginSource{
			index: index, manifest: payload, sourceType: pluginSourceTypeJSON,
			notice: pluginSourceTypeNotice(requested, pluginSourceTypeJSON),
		}, true, nil
	}
	if jsonURL {
		if err != nil {
			if err.Error() == softwareSourceAppGoneMessage {
				return nil, true, err
			}
			return nil, true, fmt.Errorf("%s：%s", pluginSourceJSONFetchFailed, err.Error())
		}
		return nil, true, errors.New(pluginSourceJSONInvalid)
	}
	return nil, false, nil
}

func pluginSourceJSONOverHTTP(ctx context.Context, rawURL, requested string) (*resolvedPluginSource, bool) {
	payload, err := fetchPluginHTTP(ctx, rawURL, pluginManifestMaxSize, 10*time.Second, pluginSourceAllowsPrivate(rawURL))
	if err != nil || !payloadLooksLikeJSON(payload) {
		return nil, false
	}
	index, parseErr := parsePluginSourceManifest(payload)
	if parseErr != nil {
		return nil, false
	}
	return &resolvedPluginSource{
		index: index, manifest: payload, sourceType: pluginSourceTypeJSON,
		notice: pluginSourceTypeNotice(requested, pluginSourceTypeJSON),
	}, true
}

func resolvePluginSourceFromGit(ctx context.Context, rawURL, requested string) (*resolvedPluginSource, error) {
	if pluginSourceURLLooksLikeJSON(rawURL) {
		return nil, errors.New(errPluginSourceNotGitRepo)
	}
	var payload []byte
	err := withPluginRepository(ctx, rawURL, func(repositoryDir string) error {
		var readErr error
		payload, readErr = readPluginFile(filepath.Join(repositoryDir, "index.json"), pluginManifestMaxSize)
		return readErr
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("Git 仓库根目录没有 index.json")
		}
		return nil, err
	}
	index, err := parsePluginSourceManifest(payload)
	if err != nil {
		return nil, err
	}
	return &resolvedPluginSource{
		index: index, manifest: payload, sourceType: pluginSourceTypeGit,
		notice: pluginSourceTypeNotice(requested, pluginSourceTypeGit),
	}, nil
}

func normalizePluginSourceType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return "", nil
	case pluginSourceTypeJSON:
		return pluginSourceTypeJSON, nil
	case pluginSourceTypeGit:
		return pluginSourceTypeGit, nil
	default:
		return "", errors.New("源类型只能是 JSON 目录或 Git 仓库")
	}
}

func pluginSourceTypeNotice(requested, detected string) string {
	if requested == pluginSourceTypeGit && detected == pluginSourceTypeJSON {
		return pluginSourceCorrectedToJSON
	}
	if requested == pluginSourceTypeJSON && detected == pluginSourceTypeGit {
		return pluginSourceCorrectedToGit
	}
	return ""
}

func pluginSourceURLPath(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(parsed.Path), "/")
}

func pluginSourceURLLooksLikeJSON(rawURL string) bool {
	return strings.HasSuffix(pluginSourceURLPath(rawURL), ".json")
}

func payloadLooksLikeJSON(payload []byte) bool {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return false
	}
	return json.Valid(trimmed)
}

func pluginSourceJSONURLPredicate(column string) string {
	switch column {
	case "url", "s.url":
	default:
		column = "url"
	}
	return "LOWER(TRIM(TRAILING '/' FROM SUBSTRING_INDEX(SUBSTRING_INDEX(" + column + ", '?', 1), '#', 1))) LIKE '%.json'"
}

func migrateMisclassifiedJSONPluginSources(db *sql.DB) error {
	jsonURL := pluginSourceJSONURLPredicate("s.url")
	if _, err := db.Exec(`UPDATE plugin_sources s
		JOIN plugin_source_cache c ON c.source_id = s.id
		SET s.source_type = 'git'
		WHERE c.source_type = 'git' AND s.source_type <> 'git' AND NOT (` + jsonURL + `)`); err != nil {
		return err
	}
	jsonURL = pluginSourceJSONURLPredicate("s.url")
	if _, err := db.Exec(`UPDATE plugin_source_cache c
		JOIN plugin_sources s ON s.id = c.source_id
		SET c.source_type = 'json'
		WHERE c.source_type = 'git' AND ` + jsonURL); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE plugin_sources
		SET source_type = 'json'
		WHERE source_type = 'git' AND ` + pluginSourceJSONURLPredicate("url")); err != nil {
		return err
	}
	return nil
}

func pluginSourceFailureMessage(prefix string, err error) string {
	if err == nil {
		return prefix
	}
	if err.Error() == errPluginSourceNotGitRepo || err.Error() == softwareSourceAppGoneMessage || strings.HasPrefix(err.Error(), pluginSourceJSONFetchFailed) || err.Error() == pluginSourceJSONInvalid {
		return err.Error()
	}
	return prefix + err.Error()
}

func pluginSourceAddedMessage(notice string, pluginCount int) string {
	msg := fmt.Sprintf("软件源已添加，发现 %d 个插件", pluginCount)
	if notice == "" {
		return msg
	}
	return notice + "。" + msg
}

func parsePluginSourceManifest(payload []byte) (*remotePluginIndex, error) {
	var index remotePluginIndex
	if err := json.Unmarshal(payload, &index); err != nil {
		return nil, errors.New("仓库清单不是有效的 JSON")
	}
	seen := make(map[string]struct{}, len(index.Plugins))
	for _, plugin := range index.Plugins {
		if !pluginIDPattern.MatchString(strings.TrimSpace(plugin.ID)) {
			return nil, fmt.Errorf("插件标识 %q 不合法", plugin.ID)
		}
		if _, exists := seen[plugin.ID]; exists {
			return nil, fmt.Errorf("插件标识 %q 重复", plugin.ID)
		}
		seen[plugin.ID] = struct{}{}
		if strings.TrimSpace(plugin.Name) == "" || strings.TrimSpace(plugin.Version) == "" {
			return nil, fmt.Errorf("插件 %q 缺少名称或版本", plugin.ID)
		}
	}
	return &index, nil
}

func loadPluginSourceIndex(ctx context.Context, db *sql.DB, source pluginSourceRecord, force bool) (*remotePluginIndex, string, error) {
	if payload, index, ok := lookupLocalSoftwareSourceIndex(source.URL); ok {
		_ = cachePluginSourceManifest(db, source.ID, pluginSourceTypeJSON, payload, "")
		_ = persistPluginSourceType(db, source.ID, pluginSourceTypeJSON)
		notice := pluginSourceTypeNotice(source.SourceType, pluginSourceTypeJSON)
		return index, notice, nil
	}
	cache, cacheErr := readCachedPluginSource(db, source.ID)
	if !force && cacheErr == nil && time.Now().Before(cache.ExpiresAt) {
		index, err := parsePluginSourceManifest(cache.Manifest)
		if err == nil && cache.LastError == softwareSourceAppGoneMessage {
			return index, "", errors.New(softwareSourceAppGoneMessage)
		}
		return index, "", err
	}
	resolved, fetchErr := resolvePluginSource(ctx, source.URL, source.SourceType)
	if fetchErr == nil && resolved != nil {
		if err := cachePluginSourceManifest(db, source.ID, resolved.sourceType, resolved.manifest, ""); err != nil {
			return nil, "", err
		}
		if err := persistPluginSourceType(db, source.ID, resolved.sourceType); err != nil {
			return nil, "", err
		}
		return resolved.index, resolved.notice, nil
	}
	if cacheErr == nil && fetchErr != nil {
		_, _ = db.Exec("UPDATE plugin_source_cache SET last_error=? WHERE source_id=?", truncateText(fetchErr.Error(), 500), source.ID)
		if stale, err := parsePluginSourceManifest(cache.Manifest); err == nil {
			return stale, "", fetchErr
		}
	}
	return nil, "", fetchErr
}

func persistPluginSourceType(db *sql.DB, sourceID int64, sourceType string) error {
	if db == nil || sourceID == 0 || sourceType == "" {
		return nil
	}
	_, err := db.Exec("UPDATE plugin_sources SET source_type=? WHERE id=?", sourceType, sourceID)
	return err
}

func cachePluginSourceManifest(db *sql.DB, sourceID int64, sourceType string, manifest []byte, lastError string) error {
	_, err := db.Exec(`INSERT INTO plugin_source_cache (source_id, source_type, manifest_json, fetched_at, expires_at, last_error)
		VALUES (?, ?, ?, NOW(), DATE_ADD(NOW(), INTERVAL 5 MINUTE), ?)
		ON DUPLICATE KEY UPDATE source_type=VALUES(source_type), manifest_json=VALUES(manifest_json), fetched_at=VALUES(fetched_at), expires_at=VALUES(expires_at), last_error=VALUES(last_error)`,
		sourceID, sourceType, manifest, truncateText(lastError, 500))
	return err
}

func readCachedPluginSource(db *sql.DB, sourceID int64) (cachedPluginSource, error) {
	var cache cachedPluginSource
	err := db.QueryRow("SELECT manifest_json, expires_at, last_error FROM plugin_source_cache WHERE source_id=?", sourceID).
		Scan(&cache.Manifest, &cache.ExpiresAt, &cache.LastError)
	return cache, err
}

func listPluginSources(db *sql.DB) ([]pluginSourceRecord, error) {
	rows, err := db.Query("SELECT id, name, url, source_type, created_at FROM plugin_sources ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]pluginSourceRecord, 0)
	for rows.Next() {
		var item pluginSourceRecord
		var createdAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.URL, &item.SourceType, &createdAt); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			item.CreatedAt = createdAt.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func AdminPluginList(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 500, "msg": "初始化插件存储失败"})
		return
	}
	enabledMap, err := loadPluginEnabledMap(db)
	if err != nil {
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 500, "msg": "读取插件状态失败"})
		return
	}
	localPlugins, err := loadLocalPlugins()
	if err != nil {
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 500, "msg": "读取本地插件失败"})
		return
	}
	localIDs := make(map[string]bool)
	for _, plugin := range pluginCatalog {
		localIDs[plugin.ID] = true
	}
	for _, plugin := range localPlugins {
		localIDs[plugin.ID] = true
	}
	sources, err := listPluginSources(db)
	if err != nil {
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 500, "msg": "读取软件源失败"})
		return
	}
	sourceFilter := strings.TrimSpace(c.Query("source"))
	keyword := strings.ToLower(strings.TrimSpace(c.Query("q")))
	local := make([]pluginInfo, 0, len(pluginCatalog))
	if sourceFilter == "" || sourceFilter == "local" {
		for _, plugin := range localPlugins {
			plugin.Enabled = enabledMap[plugin.ID]
			plugin.Configured = pluginConfigured(db, plugin.ID)
			plugin.Local = true
			local = append(local, plugin)
		}
	}
	remote := make([]pluginInfo, 0)
	sourceOK := make(map[int64]bool)
	sourceErrors := make(map[int64]string)
	indexes := make([]*remotePluginIndex, 0)
	prices := map[string]int64{}
	if sourceFilter != "local" {
		for _, source := range sources {
			if sourceFilter != "" && sourceFilter != fmt.Sprintf("%d", source.ID) {
				continue
			}
			index, _, loadErr := loadPluginSourceIndex(c.Request.Context(), db, source, false)
			if loadErr != nil {
				sourceErrors[source.ID] = loadErr.Error()
			}
			if index == nil {
				sourceOK[source.ID] = false
				continue
			}
			sourceOK[source.ID] = loadErr == nil
			indexes = append(indexes, index)
			sourceName := source.Name
			if sourceName == "" {
				sourceName = index.Name
			}
			for _, item := range index.Plugins {
				if item.PriceCents > 0 && item.ID != "" {
					prices[item.ID] = item.PriceCents
				}
				if item.ID == "" || localIDs[item.ID] {
					continue
				}
				icon := item.Icon
				if icon == "" {
					icon = "ri:puzzle-line"
				}
				remote = append(remote, pluginInfo{
					ID: item.ID, Category: displayPluginCategory(item.Category), Name: item.Name,
					Description: item.Description, Icon: icon, Version: item.Version, PriceCents: item.PriceCents,
					Author: item.Author, Local: false, Remote: true, Source: sourceName, DownloadURL: item.DownloadURL,
				})
			}
			rememberPaidCatalogFromIndex(index)
		}
	}
	groups := buildPluginStoreGroups(local, remote, indexes, keyword)
	view := currentBuyerAccess(c)
	for i := range groups {
		for j := range groups[i].Plugins {
			if groups[i].Plugins[j].PriceCents == 0 {
				groups[i].Plugins[j].PriceCents = prices[groups[i].Plugins[j].ID]
			}
			groups[i].Plugins[j].Ownership = ownershipForPrice(groups[i].Plugins[j].PriceCents, view.Edition == storeEditionCommercial, buyerItemEntitled(view, "plugin", groups[i].Plugins[j].ID))
		}
	}
	sourceStates := make([]gin.H, 0, len(sources))
	for _, source := range sources {
		state := "unknown"
		if ok, checked := sourceOK[source.ID]; checked {
			if ok {
				state = "ok"
			} else {
				state = "error"
			}
		}
		lastError := ""
		if loadErr, failed := sourceErrors[source.ID]; failed {
			lastError = loadErr
		}
		sourceStates = append(sourceStates, pluginSourceStateView(source, state, lastError))
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"categories": groups, "sources": sourceStates}})
}

func AdminPluginSourceAdd(c *gin.Context) {
	var request struct {
		Name       string `json:"name"`
		URL        string `json:"url"`
		SourceType string `json:"sourceType"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	repositoryURL, err := validatePluginSourceURL(request.URL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	resolved, err := resolvePluginSource(c.Request.Context(), repositoryURL, request.SourceType)
	if err != nil || resolved == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": pluginSourceFailureMessage("仓库校验失败：", err)})
		return
	}
	index, manifest, sourceType := resolved.index, resolved.manifest, resolved.sourceType
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		request.Name = index.Name
	}
	if request.Name == "" {
		request.Name = "未命名仓库"
	}
	request.Name = truncateText(request.Name, 60)
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化插件存储失败"})
		return
	}
	result, err := db.Exec("INSERT INTO plugin_sources (name, url, source_type, created_at) VALUES (?, ?, ?, NOW()) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), name=VALUES(name), source_type=VALUES(source_type)", request.Name, repositoryURL, sourceType)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存软件源失败"})
		return
	}
	sourceID, _ := result.LastInsertId()
	if err := cachePluginSourceManifest(db, sourceID, sourceType, manifest, ""); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "缓存软件源失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  pluginSourceAddedMessage(resolved.notice, len(index.Plugins)),
		"data": gin.H{"sourceType": sourceType, "corrected": resolved.notice != ""},
	})
}

func pluginSourceStateView(source pluginSourceRecord, state, lastError string) gin.H {
	view := gin.H{
		"id": source.ID, "name": source.Name, "url": source.URL, "sourceType": source.SourceType,
		"state": state, "lastError": lastError, "restoreAppId": 0,
	}
	if lastError != softwareSourceAppGoneMessage {
		return view
	}
	appKey := parsePublicSoftwareSourceAppKey(source.URL)
	view["goneAppKey"] = appKey
	if appKey == "" {
		return view
	}
	app, err := currentSourceStationStore().GetCatalogAppByKey(appKey)
	if err == nil && app.Archived {
		view["restoreAppId"] = app.ID
	}
	return view
}

func AdminPluginSourceRetarget(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var request struct {
		TargetAppID int64  `json:"targetAppId"`
		URL         string `json:"url"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化插件存储失败"})
		return
	}
	var source pluginSourceRecord
	if err := db.QueryRow("SELECT id, name, url, source_type FROM plugin_sources WHERE id=?", id).Scan(&source.ID, &source.Name, &source.URL, &source.SourceType); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "软件源不存在"})
		return
	}
	newURL := strings.TrimSpace(request.URL)
	msg := "软件源地址已更换"
	if request.TargetAppID > 0 {
		appKey := parsePublicSoftwareSourceAppKey(source.URL)
		if appKey == "" {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "这条软件源地址里没有应用标识"})
			return
		}
		if err := saveSoftwareSourceAlias(appKey, request.TargetAppID, true); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		target, err := currentSourceStationStore().GetCatalogAppByID(request.TargetAppID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "目标应用不存在"})
			return
		}
		newURL, err = rewriteSoftwareSourceURL(source.URL, target.AppKey)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		msg = "已更换软件源地址，旧地址也会打开目标应用的目录"
	}
	if newURL == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择目标应用或填写新的软件源地址"})
		return
	}
	newURL, err = validatePluginSourceURL(newURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if _, err := db.Exec("UPDATE plugin_sources SET url=?, source_type=? WHERE id=?", newURL, pluginSourceTypeJSON, source.ID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更换软件源地址失败"})
		return
	}
	source.URL = newURL
	source.SourceType = pluginSourceTypeJSON
	index, notice, err := loadPluginSourceIndex(c.Request.Context(), db, source, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": pluginSourceFailureMessage("软件源地址已更换，但刷新失败：", err)})
		return
	}
	if notice != "" {
		msg = notice
	}
	pluginCount := 0
	if index != nil {
		pluginCount = len(index.Plugins)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": gin.H{
		"url": newURL, "sourceType": pluginSourceTypeJSON, "plugins": pluginCount,
	}})
}

func AdminPluginSourceDelete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化插件存储失败"})
		return
	}
	if _, err := db.Exec("DELETE FROM plugin_sources WHERE id=?", id); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "删除软件源失败"})
		return
	}
	_, _ = db.Exec("DELETE FROM plugin_source_cache WHERE source_id=?", id)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "软件源已删除"})
}

func AdminPluginDownload(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	if !pluginIDPattern.MatchString(pluginID) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "插件标识不合法"})
		return
	}
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化插件存储失败"})
		return
	}
	localIDs, err := loadLocalPluginIDs()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取本地插件失败"})
		return
	}
	if localIDs[pluginID] {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "插件已安装，无需重复下载"})
		return
	}
	sources, err := listPluginSources(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取软件源失败"})
		return
	}
	downloadURL := ""
	sourceURL := ""
	expectedSHA := ""
	var metadata pluginInfo
	for _, source := range sources {
		index, _, _ := loadPluginSourceIndex(c.Request.Context(), db, source, false)
		if index == nil {
			continue
		}
		for _, plugin := range index.Plugins {
			if plugin.ID == pluginID {
				downloadURL = strings.TrimSpace(plugin.DownloadURL)
				expectedSHA = plugin.SHA256
				sourceURL = source.URL
				metadata = pluginInfo{ID: plugin.ID, Category: plugin.Category, Name: plugin.Name,
					Description: plugin.Description, Icon: plugin.Icon, Version: plugin.Version,
					Author: plugin.Author, Source: source.Name, DownloadURL: downloadURL}
				if metadata.Source == "" {
					metadata.Source = index.Name
				}
				break
			}
		}
		if downloadURL != "" {
			break
		}
	}
	if downloadURL == "" {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "未在任何软件源中找到该插件或插件未提供下载地址"})
		return
	}
	if err := downloadAndInstallPluginPackage(c.Request.Context(), downloadURL, expectedSHA, pluginSourceAllowsPrivate(sourceURL), metadata); err != nil {
		code := 500
		if isPluginPackageReject(err) {
			code = 400
		}
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": "下载失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "插件已下载、解压并安装"})
}

func AdminPluginSourceRefresh(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化插件存储失败"})
		return
	}
	var source pluginSourceRecord
	if err := db.QueryRow("SELECT id, name, url, source_type FROM plugin_sources WHERE id=?", id).Scan(&source.ID, &source.Name, &source.URL, &source.SourceType); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "软件源不存在"})
		return
	}
	index, notice, err := loadPluginSourceIndex(c.Request.Context(), db, source, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": pluginSourceFailureMessage("软件源刷新失败：", err)})
		return
	}
	msg := "软件源刷新成功"
	if notice != "" {
		msg = notice
	}
	storedType := source.SourceType
	_ = db.QueryRow("SELECT source_type FROM plugin_sources WHERE id=?", source.ID).Scan(&storedType)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": gin.H{
		"plugins": len(index.Plugins), "homeTemplates": len(index.HomeTemplates),
		"sourceType": storedType, "notice": notice,
	}})
}

func downloadAndInstallPluginPackage(ctx context.Context, rawURL, expectedSHA string, allowPrivate bool, plugin pluginInfo) error {
	payload, err := downloadPluginPackage(ctx, rawURL, expectedSHA, allowPrivate)
	if err != nil {
		return err
	}
	return installPluginZIP(payload, plugin)
}

func isPluginPackageReject(err error) bool {
	return errors.Is(err, errPluginSHAMissing) || errors.Is(err, errPluginSHAMismatch) || errors.Is(err, errSafeTooLarge) || isSafeFetchPolicyError(err)
}

func downloadPluginPackage(ctx context.Context, rawURL, expectedSHA string, allowPrivate bool) ([]byte, error) {
	expectedSHA = strings.ToLower(strings.TrimSpace(expectedSHA))
	if expectedSHA == "" {
		return nil, errPluginSHAMissing
	}
	if err := validateSHA256(expectedSHA); err != nil {
		return nil, err
	}
	payload, err := safeHTTPGet(ctx, rawURL, safeFetchOptions{
		AllowPrivate: allowPrivate,
		RequireHTTPS: !allowPrivate,
		MaxBytes:     pluginPackageMaxSize,
		Timeout:      30 * time.Second,
		MaxRedirects: defaultSafeRedirects,
	})
	if err != nil {
		return nil, wrapPluginNetError(err)
	}
	if len(payload) == 0 {
		return nil, errors.New("插件包为空")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != expectedSHA {
		return nil, errPluginSHAMismatch
	}
	return payload, nil
}

func fetchPluginHTTP(ctx context.Context, rawURL string, maxBytes int64, timeout time.Duration, allowPrivate bool) ([]byte, error) {
	payload, err := safeHTTPGet(ctx, rawURL, safeFetchOptions{
		AllowPrivate: allowPrivate,
		RequireHTTPS: false,
		MaxBytes:     maxBytes,
		Timeout:      timeout,
		MaxRedirects: defaultSafeRedirects,
	})
	if err != nil {
		return nil, wrapPluginNetError(err)
	}
	return payload, nil
}

func wrapPluginNetError(err error) error {
	if err == nil || isPluginPackageReject(err) {
		return err
	}
	var status *safeStatusError
	if errors.As(err, &status) {
		return err
	}
	return fmt.Errorf("连接失败：%w", err)
}

func explainGitCloneFailure(output string) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "is this a git repository") ||
		strings.Contains(lower, "not a git repository") ||
		strings.Contains(lower, "does not appear to be a git repository") ||
		strings.Contains(lower, "repository") && strings.Contains(lower, "not found") ||
		strings.Contains(lower, "bad line length character") {
		return errors.New(errPluginSourceNotGitRepo)
	}
	return errors.New("Git 仓库拉取失败，请确认地址可以匿名克隆")
}

func withPluginRepository(ctx context.Context, rawURL string, action func(string) error) error {
	cloneCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "auth-pro-plugin-source-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	repositoryDir := filepath.Join(tempDir, "repository")
	command := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--single-branch", rawURL, repositoryDir)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		if errors.Is(cloneCtx.Err(), context.DeadlineExceeded) {
			return errors.New("Git 仓库拉取超时，请稍后重试")
		}
		return explainGitCloneFailure(string(output))
	}
	return action(repositoryDir)
}

func readPluginFile(pathValue string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(pathValue)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readPluginReader(file, maxBytes)
}

func readPluginReader(reader io.Reader, maxBytes int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxBytes {
		return nil, errors.New("响应超过大小限制")
	}
	return payload, nil
}
