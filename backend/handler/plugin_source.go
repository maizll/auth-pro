package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
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
}

func ensurePluginSourceStorage(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS plugin_sources (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(60) NOT NULL DEFAULT '',
		url VARCHAR(500) NOT NULL,
		created_at DATETIME DEFAULT NULL,
		UNIQUE KEY uk_url (url(191))
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权系统插件软件源'`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS plugin_source_cache (
		source_id BIGINT NOT NULL PRIMARY KEY,
		source_type VARCHAR(20) NOT NULL DEFAULT 'json',
		manifest_json MEDIUMTEXT NOT NULL,
		fetched_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		last_error VARCHAR(500) NOT NULL DEFAULT ''
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权系统插件清单缓存'`)
	return err
}

func fetchPluginSourceManifest(ctx context.Context, rawURL string) (*remotePluginIndex, []byte, string, error) {
	if _, err := validatePluginSourceURL(rawURL); err != nil {
		return nil, nil, "", err
	}
	preferGit := looksLikeGitRepositoryURL(rawURL)
	if !preferGit {
		payload, err := fetchPluginHTTP(ctx, rawURL, pluginManifestMaxSize, 10*time.Second)
		if err == nil {
			index, parseErr := parsePluginSourceManifest(payload)
			if parseErr == nil {
				return index, payload, "json", nil
			}
		}
	}
	var payload []byte
	err := withPluginRepository(ctx, rawURL, func(repositoryDir string) error {
		var readErr error
		payload, readErr = readPluginFile(filepath.Join(repositoryDir, "index.json"), pluginManifestMaxSize)
		return readErr
	})
	if err != nil {
		return nil, nil, "", err
	}
	index, err := parsePluginSourceManifest(payload)
	return index, payload, "git", err
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

func loadPluginSourceIndex(ctx context.Context, db *sql.DB, source pluginSourceRecord, force bool) (*remotePluginIndex, error) {
	cache, cacheErr := readCachedPluginSource(db, source.ID)
	if !force && cacheErr == nil && time.Now().Before(cache.ExpiresAt) {
		return parsePluginSourceManifest(cache.Manifest)
	}
	index, manifest, sourceType, fetchErr := fetchPluginSourceManifest(ctx, source.URL)
	if fetchErr == nil {
		if err := cachePluginSourceManifest(db, source.ID, sourceType, manifest, ""); err != nil {
			return nil, err
		}
		return index, nil
	}
	if cacheErr == nil {
		_, _ = db.Exec("UPDATE plugin_source_cache SET last_error=? WHERE source_id=?", truncateText(fetchErr.Error(), 500), source.ID)
		if stale, err := parsePluginSourceManifest(cache.Manifest); err == nil {
			return stale, fetchErr
		}
	}
	return nil, fetchErr
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
	err := db.QueryRow("SELECT manifest_json, expires_at FROM plugin_source_cache WHERE source_id=?", sourceID).
		Scan(&cache.Manifest, &cache.ExpiresAt)
	return cache, err
}

func listPluginSources(db *sql.DB) ([]pluginSourceRecord, error) {
	rows, err := db.Query("SELECT id, name, url, created_at FROM plugin_sources ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]pluginSourceRecord, 0)
	for rows.Next() {
		var item pluginSourceRecord
		var createdAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.URL, &createdAt); err != nil {
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
	indexes := make([]*remotePluginIndex, 0)
	if sourceFilter != "local" {
		for _, source := range sources {
			if sourceFilter != "" && sourceFilter != fmt.Sprintf("%d", source.ID) {
				continue
			}
			index, loadErr := loadPluginSourceIndex(c.Request.Context(), db, source, false)
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
				if item.ID == "" || localIDs[item.ID] {
					continue
				}
				icon := item.Icon
				if icon == "" {
					icon = "ri:puzzle-line"
				}
				remote = append(remote, pluginInfo{
					ID: item.ID, Category: displayPluginCategory(item.Category), Name: item.Name,
					Description: item.Description, Icon: icon, Version: item.Version,
					Author: item.Author, Local: false, Remote: true, Source: sourceName, DownloadURL: item.DownloadURL,
				})
			}
		}
	}
	groups := buildPluginStoreGroups(local, remote, indexes, keyword)
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
		sourceStates = append(sourceStates, gin.H{"id": source.ID, "name": source.Name, "url": source.URL, "state": state})
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"categories": groups, "sources": sourceStates}})
}

func AdminPluginSourceAdd(c *gin.Context) {
	var request struct {
		Name string `json:"name"`
		URL  string `json:"url"`
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
	index, manifest, sourceType, err := fetchPluginSourceManifest(c.Request.Context(), repositoryURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "仓库校验失败：" + err.Error()})
		return
	}
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
	result, err := db.Exec("INSERT INTO plugin_sources (name, url, created_at) VALUES (?, ?, NOW()) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), name=VALUES(name)", request.Name, repositoryURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存软件源失败"})
		return
	}
	sourceID, _ := result.LastInsertId()
	if err := cachePluginSourceManifest(db, sourceID, sourceType, manifest, ""); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "缓存软件源失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": fmt.Sprintf("软件源已添加，发现 %d 个插件", len(index.Plugins))})
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
	var metadata pluginInfo
	for _, source := range sources {
		index, _ := loadPluginSourceIndex(c.Request.Context(), db, source, false)
		if index == nil {
			continue
		}
		for _, plugin := range index.Plugins {
			if plugin.ID == pluginID {
				downloadURL = strings.TrimSpace(plugin.DownloadURL)
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
	payload, err := downloadPluginPackage(downloadURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "下载失败：" + err.Error()})
		return
	}
	if err := installPluginZIP(payload, metadata); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "插件安装失败：" + err.Error()})
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
	if err := db.QueryRow("SELECT id, name, url FROM plugin_sources WHERE id=?", id).Scan(&source.ID, &source.Name, &source.URL); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "软件源不存在"})
		return
	}
	index, err := loadPluginSourceIndex(c.Request.Context(), db, source, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "软件源刷新失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "软件源刷新成功", "data": gin.H{
		"plugins": len(index.Plugins), "homeTemplates": len(index.HomeTemplates),
	}})
}

func downloadPluginPackage(rawURL string) ([]byte, error) {
	payload, err := fetchPluginHTTP(context.Background(), rawURL, pluginPackageMaxSize, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, errors.New("插件包为空")
	}
	return payload, nil
}

func fetchPluginHTTP(ctx context.Context, rawURL string, maxBytes int64, timeout time.Duration) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载地址返回状态码 %d", response.StatusCode)
	}
	return readPluginReader(response.Body, maxBytes)
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
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("Git 仓库拉取失败：%s", truncateText(string(output), 500))
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
