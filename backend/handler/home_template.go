package handler

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"auto_pro/config"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

const (
	homeTemplateSchemaVersion = 1
	homeTemplateMaxBytes      = 2 << 20
)

type templateAuthor struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

type homeTemplateDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	Hero          struct {
		Title string `json:"title"`
	} `json:"hero"`
	Scripts json.RawMessage `json:"scripts"`
}

func ensureHomeTemplateStorage(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS home_templates (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			catalog_id CHAR(36) DEFAULT NULL COMMENT '独立软件源模板ID',
			catalog_snapshot JSON DEFAULT NULL COMMENT '最后成功目录元数据快照',
			template_key VARCHAR(60) NOT NULL COMMENT '目录模板标识',
			source_id BIGINT NOT NULL COMMENT '本地兼容来源标识',
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500) NOT NULL DEFAULT '',
			version VARCHAR(40) NOT NULL,
			source_url VARCHAR(500) NOT NULL DEFAULT '',
			source_type VARCHAR(20) NOT NULL DEFAULT 'remote',
			preview_url VARCHAR(500) NOT NULL DEFAULT '',
			author_name VARCHAR(100) NOT NULL DEFAULT '',
			author_url VARCHAR(300) NOT NULL DEFAULT '',
			author_email VARCHAR(200) NOT NULL DEFAULT '',
			template_url VARCHAR(500) NOT NULL DEFAULT '',
			template_path VARCHAR(500) NOT NULL DEFAULT '',
			sha256 CHAR(64) NOT NULL,
			schema_version INT NOT NULL DEFAULT 1,
			available TINYINT(1) NOT NULL DEFAULT 1,
			installed_path VARCHAR(1000) NOT NULL DEFAULT '',
			installed_at DATETIME DEFAULT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_source_template (source_id, template_key),
			UNIQUE KEY uk_home_template_catalog_id (catalog_id),
			KEY idx_available (available)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权实例首页模板安装状态'
	`); err != nil {
		return err
	}
	columns := []struct{ name, ddl string }{
		{"author_name", "ALTER TABLE home_templates ADD COLUMN author_name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '作者名称' AFTER preview_url"},
		{"author_url", "ALTER TABLE home_templates ADD COLUMN author_url VARCHAR(300) NOT NULL DEFAULT '' COMMENT '作者主页' AFTER author_name"},
		{"author_email", "ALTER TABLE home_templates ADD COLUMN author_email VARCHAR(200) NOT NULL DEFAULT '' COMMENT '作者邮箱' AFTER author_url"},
		{"catalog_id", "ALTER TABLE home_templates ADD COLUMN catalog_id CHAR(36) DEFAULT NULL COMMENT '独立软件源模板ID' AFTER id"},
		{"catalog_snapshot", "ALTER TABLE home_templates ADD COLUMN catalog_snapshot JSON DEFAULT NULL COMMENT '最后成功目录元数据快照' AFTER catalog_id"},
	}
	for _, column := range columns {
		if err := ensureColumn(db, "home_templates", column.name, column.ddl); err != nil {
			return err
		}
	}
	return ensureIndex(db, "home_templates", "uk_home_template_catalog_id", []string{"catalog_id"}, true)
}

func AdminHomeTemplateList(c *gin.Context) {
	warning := ""
	if c.Query("refresh") == "1" {
		warning = homeTemplateCatalogRefreshWarning(c.Request.Context())
	}
	items, err := listHomeTemplates(c.Request.Context())
	if err != nil {
		writeAppStoreError(c, err)
		return
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": legacyHomeTemplateItems(items), "warning": warning}})
}

// homeTemplateCatalogRefreshWarning 刷新与插件共用的软件源目录。
// 未配置远程软件源（自托管 / 空 URL）时静默跳过：本站上传与源站模板足够使用。
func homeTemplateCatalogRefreshWarning(ctx context.Context) string {
	client, err := softwaresource.Default()
	if err != nil {
		if isUnconfiguredRemoteSoftwareSource(err) {
			return ""
		}
		return remoteSoftwareSourceRefreshWarning(err)
	}
	if _, err = client.Refresh(ctx); err != nil {
		return remoteSoftwareSourceRefreshWarning(err)
	}
	return ""
}

func isUnconfiguredRemoteSoftwareSource(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, softwaresource.ErrUnconfigured) {
		return true
	}
	return strings.TrimSpace(config.GetSoftwareSourceURL()) == ""
}

func remoteSoftwareSourceRefreshWarning(err error) string {
	return "刷新远程软件源模板目录失败，本站上传与源站模板仍可使用：" + err.Error()
}

// listHomeTemplates is replaced in tests so refresh warning behavior can be
// asserted without a MySQL-backed catalog.
var listHomeTemplates = listAppStoreTemplates

func AdminHomeTemplateEnable(c *gin.Context) {
	if rejectPaidTemplateEnable(c, strings.TrimSpace(c.Param("id"))) {
		return
	}
	if err := enableAppStoreTemplate(c.Request.Context(), c.Param("id")); err != nil {
		writeAppStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已启用"})
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
	if err := ensurePluginStorage(db); err != nil {
		writeDefaultHomeTemplate(c)
		return
	}
	activeID := loadActiveHomeTemplateID(db)
	if activeID == 0 {
		writeDefaultHomeTemplate(c)
		return
	}
	var templateID, name, version, installedPath, sourceType, checksum string
	if err := db.QueryRow("SELECT template_key, name, version, installed_path, source_type, sha256 FROM home_templates WHERE id = ?", activeID).
		Scan(&templateID, &name, &version, &installedPath, &sourceType, &checksum); err != nil || installedPath == "" {
		writeDefaultHomeTemplate(c)
		return
	}
	assetBaseURL := ""
	if sourceType == "upload" || strings.HasPrefix(filepath.Base(filepath.Dir(installedPath)), "upload-") {
		if sourceType != "upload" {
			installation, err := readCatalogTemplateInstallation(installedPath)
			if err != nil {
				writeDefaultHomeTemplate(c)
				return
			}
			// A catalog refresh must not disable the last successfully installed version.
			checksum = installation.EntrySHA256
			if installation.Version != "" {
				version = installation.Version
			}
		}
		if !installedUploadedTemplateMatches(installedPath, checksum) {
			writeDefaultHomeTemplate(c)
			return
		}
		assetBaseURL = homeTemplateAssetBaseURL(activeID, installedPath)
		if filepath.Ext(installedPath) == ".html" {
			writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
				"id": activeID, "templateId": templateID, "name": name, "version": version, "isDefault": false,
				"format": "static", "entryUrl": assetBaseURL + "index.html",
			}})
			return
		}
	}
	payload, err := readLimitedFile(installedPath, homeTemplateMaxBytes)
	if err != nil || validateHomeTemplateDocument(payload) != nil {
		writeDefaultHomeTemplate(c)
		return
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"id": activeID, "templateId": templateID, "name": name, "version": version,
		"isDefault": false, "format": "json", "assetBaseUrl": assetBaseURL, "schemaVersion": homeTemplateSchemaVersion, "document": json.RawMessage(payload),
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

func readLimitedFile(pathValue string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(pathValue)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxBytes {
		return nil, errors.New("文件超过大小限制")
	}
	return payload, nil
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
