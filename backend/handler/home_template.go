package handler

import (
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
	"regexp"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const (
	homeTemplateSchemaVersion = 1
	pluginSourceCacheTTL      = 5 * time.Minute
	sourceManifestMaxBytes    = 2 << 20
	homeTemplateMaxBytes      = 2 << 20
)

type remoteHomeTemplate struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Version       string `json:"version"`
	PreviewURL    string `json:"previewUrl"`
	SchemaVersion int    `json:"schemaVersion"`
	SHA256        string `json:"sha256"`
	TemplateURL   string `json:"templateUrl"`
	TemplatePath  string `json:"templatePath"`
}

type homeTemplateDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	Hero          struct {
		Title string `json:"title"`
	} `json:"hero"`
	Scripts json.RawMessage `json:"scripts"`
}

type cachedPluginSource struct {
	SourceType string
	Manifest   []byte
	ExpiresAt  time.Time
}

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func ensureHomeTemplateStorage(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_source_cache (
			source_id BIGINT NOT NULL PRIMARY KEY,
			source_type VARCHAR(20) NOT NULL DEFAULT 'json' COMMENT 'json/git',
			manifest_json MEDIUMTEXT NOT NULL,
			fetched_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			last_error VARCHAR(500) NOT NULL DEFAULT ''
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='软件源清单缓存'
	`); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS home_templates (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			template_key VARCHAR(60) NOT NULL COMMENT '仓库内模板标识',
			source_id BIGINT NOT NULL COMMENT '软件源ID',
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500) NOT NULL DEFAULT '',
			version VARCHAR(40) NOT NULL,
			source_url VARCHAR(500) NOT NULL,
			source_type VARCHAR(20) NOT NULL DEFAULT 'json',
			preview_url VARCHAR(500) NOT NULL DEFAULT '',
			template_url VARCHAR(500) NOT NULL DEFAULT '',
			template_path VARCHAR(500) NOT NULL DEFAULT '',
			sha256 CHAR(64) NOT NULL,
			schema_version INT NOT NULL DEFAULT 1,
			available TINYINT(1) NOT NULL DEFAULT 1,
			installed_path VARCHAR(1000) NOT NULL DEFAULT '',
			installed_at DATETIME DEFAULT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_source_template (source_id, template_key),
			KEY idx_available (available)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='首页模板'
	`)
	return err
}

func fetchPluginSourceManifest(ctx context.Context, rawURL string) (*remotePluginIndex, []byte, string, error) {
	if _, err := validatePluginSourceURL(rawURL); err != nil {
		return nil, nil, "", err
	}

	preferGit := looksLikeGitRepositoryURL(rawURL)
	if !preferGit {
		if index, payload, err := fetchJSONSourceManifest(ctx, rawURL); err == nil {
			return index, payload, "json", nil
		}
	}
	index, payload, gitErr := fetchGitSourceManifest(ctx, rawURL)
	if gitErr == nil {
		return index, payload, "git", nil
	}
	if preferGit {
		return nil, nil, "", gitErr
	}
	return nil, nil, "", fmt.Errorf("既不是有效 JSON 清单，也无法作为 Git 仓库拉取：%w", gitErr)
}

func looksLikeGitRepositoryURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	pathValue := strings.ToLower(strings.TrimSuffix(parsed.Path, "/"))
	if strings.HasSuffix(pathValue, ".git") {
		return true
	}
	if strings.HasSuffix(pathValue, ".json") || strings.Contains(pathValue, "/raw/") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" || host == "gitlab.com" || host == "gitee.com"
}

func fetchJSONSourceManifest(ctx context.Context, rawURL string) (*remotePluginIndex, []byte, error) {
	payload, err := fetchLimitedHTTP(ctx, rawURL, sourceManifestMaxBytes, 10*time.Second)
	if err != nil {
		return nil, nil, err
	}
	index, err := parsePluginSourceManifest(payload)
	return index, payload, err
}

func fetchGitSourceManifest(ctx context.Context, rawURL string) (*remotePluginIndex, []byte, error) {
	var payload []byte
	err := withClonedRepository(ctx, rawURL, func(repositoryDir string) error {
		manifestPath := filepath.Join(repositoryDir, "index.json")
		var err error
		payload, err = readLimitedFile(manifestPath, sourceManifestMaxBytes)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	index, err := parsePluginSourceManifest(payload)
	return index, payload, err
}

func parsePluginSourceManifest(payload []byte) (*remotePluginIndex, error) {
	var index remotePluginIndex
	if err := json.Unmarshal(payload, &index); err != nil {
		return nil, errors.New("仓库清单不是有效的 JSON")
	}
	if err := validateRemoteHomeTemplates(index.HomeTemplates); err != nil {
		return nil, err
	}
	return &index, nil
}

func validateRemoteHomeTemplates(templates []remoteHomeTemplate) error {
	seen := make(map[string]struct{}, len(templates))
	for _, item := range templates {
		item.ID = strings.TrimSpace(item.ID)
		if !pluginIDPattern.MatchString(item.ID) {
			return fmt.Errorf("模板标识 %q 不合法", item.ID)
		}
		if _, exists := seen[item.ID]; exists {
			return fmt.Errorf("模板标识 %q 重复", item.ID)
		}
		seen[item.ID] = struct{}{}
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Version) == "" {
			return fmt.Errorf("模板 %q 缺少名称或版本", item.ID)
		}
		if item.SchemaVersion != homeTemplateSchemaVersion {
			return fmt.Errorf("模板 %q 的 schemaVersion 必须为 %d", item.ID, homeTemplateSchemaVersion)
		}
		if !sha256Pattern.MatchString(strings.TrimSpace(item.SHA256)) {
			return fmt.Errorf("模板 %q 缺少有效 SHA256", item.ID)
		}
		if strings.TrimSpace(item.TemplateURL) == "" && strings.TrimSpace(item.TemplatePath) == "" {
			return fmt.Errorf("模板 %q 缺少 templateUrl 或 templatePath", item.ID)
		}
	}
	return nil
}

func withClonedRepository(ctx context.Context, rawURL string, action func(string) error) error {
	cloneCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "auth-pro-template-source-")
	if err != nil {
		return fmt.Errorf("创建 Git 临时目录失败：%w", err)
	}
	defer os.RemoveAll(tempDir)
	repositoryDir := filepath.Join(tempDir, "repository")
	command := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--single-branch", rawURL, repositoryDir)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		return errors.New("Git 仓库拉取失败，请检查地址、访问权限和仓库内容")
	}
	return action(repositoryDir)
}

func fetchLimitedHTTP(ctx context.Context, rawURL string, maxBytes int64, timeout time.Duration) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("请求地址格式不正确")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("地址返回状态码 %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, errors.New("读取远程内容失败")
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("远程内容超过 %d 字节限制", maxBytes)
	}
	return payload, nil
}

func readLimitedFile(pathValue string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(pathValue)
	if err != nil {
		return nil, fmt.Errorf("读取仓库文件失败：%w", err)
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("仓库文件超过 %d 字节限制", maxBytes)
	}
	return payload, nil
}

func cachePluginSourceManifest(db *sql.DB, sourceID int64, sourceType string, manifest []byte, lastError string) error {
	_, err := db.Exec(`
		INSERT INTO plugin_source_cache (source_id, source_type, manifest_json, fetched_at, expires_at, last_error)
		VALUES (?, ?, ?, NOW(), DATE_ADD(NOW(), INTERVAL ? SECOND), ?)
		ON DUPLICATE KEY UPDATE source_type = VALUES(source_type), manifest_json = VALUES(manifest_json),
			fetched_at = NOW(), expires_at = VALUES(expires_at), last_error = VALUES(last_error)
	`, sourceID, sourceType, string(manifest), int(pluginSourceCacheTTL.Seconds()), lastError)
	return err
}

func readCachedPluginSource(db *sql.DB, sourceID int64) (*cachedPluginSource, error) {
	var cache cachedPluginSource
	var manifest string
	err := db.QueryRow(`
		SELECT source_type, manifest_json, expires_at
		FROM plugin_source_cache WHERE source_id = ?
	`, sourceID).Scan(&cache.SourceType, &manifest, &cache.ExpiresAt)
	if err != nil {
		return nil, err
	}
	cache.Manifest = []byte(manifest)
	return &cache, nil
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
		if err := syncHomeTemplates(db, source.ID, source.URL, sourceType, index.HomeTemplates); err != nil {
			return nil, err
		}
		return index, nil
	}
	if cacheErr == nil {
		_, _ = db.Exec("UPDATE plugin_source_cache SET last_error = ? WHERE source_id = ?", truncateText(fetchErr.Error(), 500), source.ID)
		staleIndex, parseErr := parsePluginSourceManifest(cache.Manifest)
		if parseErr == nil {
			return staleIndex, fetchErr
		}
	}
	return nil, fetchErr
}

func syncHomeTemplates(db *sql.DB, sourceID int64, sourceURL string, sourceType string, templates []remoteHomeTemplate) error {
	if err := validateRemoteHomeTemplates(templates); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE home_templates SET available = 0, updated_at = NOW() WHERE source_id = ?", sourceID); err != nil {
		return err
	}
	for _, item := range templates {
		_, err := tx.Exec(`
			INSERT INTO home_templates (template_key, source_id, name, description, version, source_url, source_type,
				preview_url, template_url, template_path, sha256, schema_version, available, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, NOW())
			ON DUPLICATE KEY UPDATE name = VALUES(name), description = VALUES(description), version = VALUES(version),
				source_url = VALUES(source_url), source_type = VALUES(source_type), preview_url = VALUES(preview_url),
				template_url = VALUES(template_url), template_path = VALUES(template_path), sha256 = VALUES(sha256),
				schema_version = VALUES(schema_version), available = 1, updated_at = NOW()
		`, item.ID, sourceID, truncateText(item.Name, 100), truncateText(item.Description, 500), truncateText(item.Version, 40),
			sourceURL, sourceType, item.PreviewURL, item.TemplateURL, item.TemplatePath, strings.ToLower(item.SHA256), item.SchemaVersion)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func AdminPluginSourceRefresh(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "软件源标识不合法"})
		return
	}
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	defer db.Close()
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化模板存储失败"})
		return
	}
	var source pluginSourceRecord
	if err := db.QueryRow("SELECT id, name, url FROM plugin_sources WHERE id = ?", id).Scan(&source.ID, &source.Name, &source.URL); err != nil {
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

func AdminHomeTemplateList(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	defer db.Close()
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化模板存储失败"})
		return
	}
	activeID := loadActiveHomeTemplateID(db)
	items := []gin.H{{
		"id": "default", "templateId": "default", "name": "默认首页模板", "description": "系统内置首页模板",
		"version": "builtin", "source": "builtin", "enabled": activeID == 0, "installed": true, "available": true,
	}}
	rows, err := db.Query(`
		SELECT h.id, h.template_key, h.name, h.description, h.version, h.source_url, h.source_type,
			h.preview_url, h.sha256, h.schema_version, h.available, h.installed_path, h.updated_at,
			COALESCE(s.name, '')
		FROM home_templates h LEFT JOIN plugin_sources s ON s.id = h.source_id
		ORDER BY h.updated_at DESC, h.id DESC
	`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取首页模板失败"})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var templateID, name, description, version, sourceURL, sourceType, previewURL, checksum, installedPath, sourceName string
		var updatedAt time.Time
		var schemaVersion, available int
		if err := rows.Scan(&id, &templateID, &name, &description, &version, &sourceURL, &sourceType, &previewURL,
			&checksum, &schemaVersion, &available, &installedPath, &updatedAt, &sourceName); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取首页模板失败"})
			return
		}
		if sourceName == "" {
			sourceName = sourceURL
		}
		items = append(items, gin.H{
			"id": id, "templateId": templateID, "name": name, "description": description, "version": version,
			"source": sourceName, "sourceUrl": sourceURL, "sourceType": sourceType, "previewUrl": previewURL,
			"sha256": checksum, "schemaVersion": schemaVersion, "enabled": activeID == id,
			"installed": installedPath != "", "available": available == 1, "updatedAt": updatedAt.Format(time.RFC3339),
		})
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": items}})
}

func AdminHomeTemplateEnable(c *gin.Context) {
	rawID := strings.TrimSpace(c.Param("id"))
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	defer db.Close()
	if err := ensurePluginStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化模板存储失败"})
		return
	}
	if rawID == "default" {
		_, err := db.Exec("DELETE FROM system_configs WHERE `group` = 'home_template' AND `key` = 'active_template_id'")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "启用默认模板失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "默认首页模板已启用"})
		return
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "模板标识不合法"})
		return
	}
	installedPath, err := installHomeTemplate(c.Request.Context(), db, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "模板启用失败：" + err.Error()})
		return
	}
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "模板启用失败"})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE home_templates SET installed_path = ?, installed_at = NOW(), updated_at = NOW() WHERE id = ?", installedPath, id); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存模板安装状态失败"})
		return
	}
	if _, err := tx.Exec(`
		INSERT INTO system_configs (`+"`group`"+`, `+"`key`"+`, value, description)
		VALUES ('home_template', 'active_template_id', ?, '当前启用的首页模板ID')
		ON DUPLICATE KEY UPDATE value = VALUES(value)
	`, strconv.FormatInt(id, 10)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存模板启用状态失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "模板启用失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已启用"})
}

func installHomeTemplate(ctx context.Context, db *sql.DB, id int64) (string, error) {
	var sourceURL, templateURL, templatePath, checksum, existingInstalledPath string
	var available int
	err := db.QueryRow(`
		SELECT source_url, template_url, template_path, sha256, available, installed_path
		FROM home_templates WHERE id = ?
	`, id).Scan(&sourceURL, &templateURL, &templatePath, &checksum, &available, &existingInstalledPath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("模板不存在")
	}
	if err != nil {
		return "", err
	}
	if existingInstalledPath != "" {
		if existingPayload, readErr := readLimitedFile(existingInstalledPath, homeTemplateMaxBytes); readErr == nil {
			existingChecksum := sha256.Sum256(existingPayload)
			if strings.EqualFold(hex.EncodeToString(existingChecksum[:]), checksum) && validateHomeTemplateDocument(existingPayload) == nil {
				return existingInstalledPath, nil
			}
		}
	}
	if available != 1 {
		return "", errors.New("模板已不在软件源中，且本地安装文件不可用")
	}
	var payload []byte
	if strings.TrimSpace(templatePath) != "" {
		err = withClonedRepository(ctx, sourceURL, func(repositoryDir string) error {
			pathValue, pathErr := safeRepositoryPath(repositoryDir, templatePath)
			if pathErr != nil {
				return pathErr
			}
			payload, pathErr = readLimitedFile(pathValue, homeTemplateMaxBytes)
			return pathErr
		})
	} else {
		resolvedURL, resolveErr := resolveTemplateURL(sourceURL, templateURL)
		if resolveErr != nil {
			return "", resolveErr
		}
		payload, err = fetchLimitedHTTP(ctx, resolvedURL, homeTemplateMaxBytes, 15*time.Second)
	}
	if err != nil {
		return "", err
	}
	actualChecksum := sha256.Sum256(payload)
	if !strings.EqualFold(hex.EncodeToString(actualChecksum[:]), checksum) {
		return "", errors.New("模板 SHA256 校验失败")
	}
	if err := validateHomeTemplateDocument(payload); err != nil {
		return "", err
	}
	installDir := filepath.Join(config.GetHomeTemplateDir(), strconv.FormatInt(id, 10), actualChecksumString(actualChecksum)[:16])
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", err
	}
	installedPath := filepath.Join(installDir, "template.json")
	if existingPayload, readErr := readLimitedFile(installedPath, homeTemplateMaxBytes); readErr == nil {
		existingChecksum := sha256.Sum256(existingPayload)
		if existingChecksum == actualChecksum && validateHomeTemplateDocument(existingPayload) == nil {
			return installedPath, nil
		}
	}
	tempFile, err := os.CreateTemp(installDir, ".template-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(0644); err != nil {
		_ = tempFile.Close()
		return "", err
	}
	if _, err := tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		return "", err
	}
	if err := tempFile.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, installedPath); err != nil {
		existingPayload, readErr := readLimitedFile(installedPath, homeTemplateMaxBytes)
		if readErr == nil && sha256.Sum256(existingPayload) == actualChecksum && validateHomeTemplateDocument(existingPayload) == nil {
			return installedPath, nil
		}
		return "", err
	}
	return installedPath, nil
}

func validateHomeTemplateDocument(payload []byte) error {
	var document homeTemplateDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		return errors.New("模板文件不是有效 JSON")
	}
	if document.SchemaVersion != homeTemplateSchemaVersion {
		return fmt.Errorf("模板文件 schemaVersion 必须为 %d", homeTemplateSchemaVersion)
	}
	if strings.TrimSpace(document.Hero.Title) == "" {
		return errors.New("模板文件缺少 hero.title")
	}
	if len(document.Scripts) > 0 && string(document.Scripts) != "null" {
		return errors.New("声明式模板不允许 scripts 字段")
	}
	return nil
}

func PublicActiveHomeTemplate(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		writeDefaultHomeTemplate(c)
		return
	}
	defer db.Close()
	if err := ensurePluginStorage(db); err != nil {
		writeDefaultHomeTemplate(c)
		return
	}
	activeID := loadActiveHomeTemplateID(db)
	if activeID == 0 {
		writeDefaultHomeTemplate(c)
		return
	}
	var templateID, name, version, installedPath string
	if err := db.QueryRow("SELECT template_key, name, version, installed_path FROM home_templates WHERE id = ?", activeID).Scan(&templateID, &name, &version, &installedPath); err != nil || installedPath == "" {
		writeDefaultHomeTemplate(c)
		return
	}
	payload, err := readLimitedFile(installedPath, homeTemplateMaxBytes)
	if err != nil || validateHomeTemplateDocument(payload) != nil {
		writeDefaultHomeTemplate(c)
		return
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"id": activeID, "templateId": templateID, "name": name, "version": version,
		"isDefault": false, "schemaVersion": homeTemplateSchemaVersion, "document": json.RawMessage(payload),
	}})
}

func writeDefaultHomeTemplate(c *gin.Context) {
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"id": "default", "templateId": "default", "name": "默认首页模板", "version": "builtin", "isDefault": true,
	}})
}

func loadActiveHomeTemplateID(db *sql.DB) int64 {
	var value string
	if err := db.QueryRow("SELECT value FROM system_configs WHERE `group` = 'home_template' AND `key` = 'active_template_id'").Scan(&value); err != nil {
		return 0
	}
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

func resolveTemplateURL(sourceURL string, templateURL string) (string, error) {
	base, err := url.Parse(sourceURL)
	if err != nil {
		return "", errors.New("软件源地址无效")
	}
	reference, err := url.Parse(strings.TrimSpace(templateURL))
	if err != nil {
		return "", errors.New("模板地址无效")
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", errors.New("模板地址必须使用 HTTP(S)")
	}
	return resolved.String(), nil
}

func safeRepositoryPath(repositoryDir string, relativePath string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(relativePath))
	if cleanPath == "." || filepath.IsAbs(cleanPath) {
		return "", errors.New("templatePath 不合法")
	}
	target := filepath.Join(repositoryDir, cleanPath)
	evaluatedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", errors.New("templatePath 指向的文件不存在或不可访问")
	}
	evaluatedRepository, err := filepath.EvalSymlinks(repositoryDir)
	if err != nil {
		return "", errors.New("Git 仓库目录不可访问")
	}
	relative, err := filepath.Rel(evaluatedRepository, evaluatedTarget)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("templatePath 超出仓库目录")
	}
	return evaluatedTarget, nil
}

func actualChecksumString(checksum [32]byte) string {
	return hex.EncodeToString(checksum[:])
}

func truncateText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
