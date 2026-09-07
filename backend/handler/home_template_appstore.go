package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"auto_pro/appstore"
	"auto_pro/config"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

type appStoreTemplateRepository struct{}

func NewAppStoreTemplateRepository() appstore.TemplateRepository {
	return appStoreTemplateRepository{}
}

func (appStoreTemplateRepository) List(ctx context.Context) ([]appstore.Template, error) {
	return listAppStoreTemplates(ctx)
}

func (appStoreTemplateRepository) Enable(ctx context.Context, id string) error {
	return enableAppStoreTemplate(ctx, id)
}

func (appStoreTemplateRepository) Disable(ctx context.Context, id string) error {
	return disableAppStoreTemplate(ctx, id)
}

func listAppStoreTemplates(ctx context.Context) ([]appstore.Template, error) {
	client, err := softwaresource.Default()
	if err != nil {
		return nil, appstore.UnavailableError(err.Error(), err)
	}
	remoteCatalog, err := client.Catalog(ctx)
	if err != nil {
		return nil, appstore.UnavailableError(err.Error(), err)
	}
	db, err := openSystemConfigDB()
	if err != nil {
		return nil, appstore.ServerError("数据库连接失败", err)
	}
	if err := ensurePluginStorage(db); err != nil {
		return nil, appstore.ServerError("初始化模板存储失败", err)
	}
	activeID := loadActiveHomeTemplateID(db)
	items := []appstore.Template{{
		ID: "default", TemplateID: "default", Name: "默认首页模板", Description: "授权管理系统内置首页模板",
		PreviewImage: builtinDefaultPreviewURL(), Version: "1.0.0",
		Author:  appstore.Author{Name: builtinAuthor.Name, URL: builtinAuthor.URL, Email: builtinAuthor.Email},
		Enabled: activeID == 0, Source: "授权系统本地", SourceType: "builtin", Available: true, Installed: true,
	}}
	for _, remote := range remoteCatalog.Templates {
		mapping, err := ensureRemoteTemplateMapping(ctx, db, remote)
		if err != nil {
			return nil, appstore.ServerError("保存模板本地映射失败", err)
		}
		previewURL := ""
		if remote.PreviewURL != "" {
			previewURL = "/api/software-source/templates/" + remote.ID + "/preview?v=" + strconv.FormatInt(remote.UpdatedAt.UnixMilli(), 10)
		}
		item := appstore.Template{
			UpdateAvailable: mapping.InstalledPath != "" && !installedTemplateMatches(mapping.InstalledPath, remote.SHA256),
			ID:              strconv.FormatInt(mapping.ID, 10), CatalogID: remote.ID, TemplateID: remote.TemplateKey,
			Name: remote.Name, Description: remote.Description, PreviewImage: previewURL, Version: remote.Version,
			Author:  appstore.Author{Name: remote.Author.Name, URL: remote.Author.URL, Email: remote.Author.Email},
			Enabled: activeID == mapping.ID, Source: remote.Source.Name, SourceURL: config.GetSoftwareSourceURL(),
			SourceType: remote.Source.Type, Available: remote.Available, Installed: mapping.InstalledPath != "",
			SchemaVersion: remote.SchemaVersion, SHA256: remote.SHA256, UpdatedAt: remote.UpdatedAt.Format(time.RFC3339),
		}
		normalizeAppStoreTemplateMetadata(&item)
		items = append(items, item)
	}
	return items, nil
}

type localTemplateMapping struct {
	ID            int64
	InstalledPath string
}

func ensureRemoteTemplateMapping(ctx context.Context, db *sql.DB, remote softwaresource.Template) (localTemplateMapping, error) {
	snapshot, _ := json.Marshal(remote)
	sourceID := syntheticSoftwareSourceID(remote.Source.ID)
	if remote.Source.LegacyID != nil {
		sourceID = *remote.Source.LegacyID
	}
	var mapping localTemplateMapping
	err := db.QueryRowContext(ctx, "SELECT id, installed_path FROM home_templates WHERE catalog_id = ?", remote.ID).
		Scan(&mapping.ID, &mapping.InstalledPath)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRowContext(ctx, `SELECT id, installed_path FROM home_templates
			WHERE source_id = ? AND template_key = ? ORDER BY id ASC LIMIT 1`, sourceID, remote.TemplateKey).
			Scan(&mapping.ID, &mapping.InstalledPath)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRowContext(ctx, `SELECT id, installed_path FROM home_templates
			WHERE template_key = ? AND (catalog_id IS NULL OR catalog_id = '') ORDER BY id ASC LIMIT 1`, remote.TemplateKey).
			Scan(&mapping.ID, &mapping.InstalledPath)
	}
	if errors.Is(err, sql.ErrNoRows) {
		result, insertErr := db.ExecContext(ctx, `INSERT INTO home_templates
			(catalog_id, catalog_snapshot, template_key, source_id, name, description, version, source_url, source_type,
			 preview_url, author_name, author_url, author_email, template_url, template_path, sha256, schema_version, available, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, 1, NOW())`,
			remote.ID, snapshot, remote.TemplateKey, sourceID, truncateText(remote.Name, 100), truncateText(remote.Description, 500),
			truncateText(remote.Version, 40), config.GetSoftwareSourceURL(), remote.Source.Type, remote.PreviewURL,
			truncateText(remote.Author.Name, 100), truncateText(remote.Author.URL, 300), truncateText(remote.Author.Email, 200),
			remote.ContentURL, strings.ToLower(remote.SHA256), remote.SchemaVersion)
		if insertErr != nil {
			return localTemplateMapping{}, insertErr
		}
		mapping.ID, err = result.LastInsertId()
		if err != nil {
			return localTemplateMapping{}, err
		}
	} else if err != nil {
		return localTemplateMapping{}, err
	}
	_, err = db.ExecContext(ctx, `UPDATE home_templates SET catalog_id=?, catalog_snapshot=?, source_id=?, name=?, description=?, version=?,
		source_url=?, source_type=?, preview_url=?, author_name=?, author_url=?, author_email=?, template_url=?, sha256=?, schema_version=?, available=1, updated_at=NOW()
		WHERE id=?`, remote.ID, snapshot, sourceID, truncateText(remote.Name, 100), truncateText(remote.Description, 500), truncateText(remote.Version, 40),
		config.GetSoftwareSourceURL(), remote.Source.Type, remote.PreviewURL, truncateText(remote.Author.Name, 100), truncateText(remote.Author.URL, 300),
		truncateText(remote.Author.Email, 200), remote.ContentURL, strings.ToLower(remote.SHA256), remote.SchemaVersion, mapping.ID)
	return mapping, err
}

func syntheticSoftwareSourceID(sourceID string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(sourceID))
	value := int64(hasher.Sum64() & math.MaxInt64)
	if value == 0 {
		value = 1
	}
	return -value
}

func normalizeAppStoreTemplateMetadata(item *appstore.Template) {
	if item == nil {
		return
	}
	if strings.TrimSpace(item.Name) == "" {
		item.Name = item.TemplateID
	}
	if strings.TrimSpace(item.Description) == "" {
		item.Description = "暂无模板简介"
	}
	if strings.TrimSpace(item.Version) == "" {
		item.Version = "0.0.0"
	}
	if strings.TrimSpace(item.Author.Name) == "" {
		item.Author.Name = "未提供"
	}
}

func installedTemplateMatches(path, checksum string) bool {
	if path == "" {
		return false
	}
	payload, err := readLimitedFile(path, homeTemplateMaxBytes)
	return err == nil && strings.EqualFold(actualChecksumString(sha256.Sum256(payload)), checksum) && validateHomeTemplateDocument(payload) == nil
}

func installHomeTemplate(ctx context.Context, db *sql.DB, id int64) (string, error) {
	return installSoftwareSourceTemplate(ctx, db, id)
}

func installSoftwareSourceTemplate(ctx context.Context, db *sql.DB, id int64) (string, error) {
	var catalogID, checksum, existingInstalledPath string
	err := db.QueryRowContext(ctx, "SELECT COALESCE(catalog_id, ''), sha256, installed_path FROM home_templates WHERE id = ?", id).
		Scan(&catalogID, &checksum, &existingInstalledPath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("模板不存在")
	}
	if err != nil {
		return "", err
	}
	if installedTemplateMatches(existingInstalledPath, checksum) {
		return existingInstalledPath, nil
	}
	if catalogID == "" {
		return "", errors.New("模板尚未关联独立软件源目录，请刷新应用商店")
	}
	client, err := softwaresource.Default()
	if err != nil {
		return "", err
	}
	remote, err := client.FindTemplate(ctx, catalogID)
	if err != nil {
		return "", err
	}
	payload, err := client.TemplateContent(ctx, remote)
	if err != nil {
		return "", err
	}
	if err := validateHomeTemplateDocument(payload); err != nil {
		return "", err
	}
	actualChecksum := sha256.Sum256(payload)
	checksum = actualChecksumString(actualChecksum)
	installDir := filepath.Join(config.GetHomeTemplateDir(), strconv.FormatInt(id, 10), checksum[:16])
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", err
	}
	installedPath := filepath.Join(installDir, "template.json")
	if existingPayload, readErr := readLimitedFile(installedPath, homeTemplateMaxBytes); readErr == nil &&
		sha256.Sum256(existingPayload) == actualChecksum && validateHomeTemplateDocument(existingPayload) == nil {
		return installedPath, nil
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
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return "", err
	}
	if err := tempFile.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, installedPath); err != nil {
		return "", err
	}
	snapshot, _ := json.Marshal(remote)
	_, _ = db.ExecContext(ctx, "UPDATE home_templates SET sha256=?, catalog_snapshot=?, updated_at=NOW() WHERE id=?", checksum, snapshot, id)
	return installedPath, nil
}

func enableAppStoreTemplate(ctx context.Context, rawID string) error {
	rawID = strings.TrimSpace(rawID)
	db, err := openSystemConfigDB()
	if err != nil {
		return appstore.ServerError("数据库连接失败", err)
	}
	if err := ensurePluginStorage(db); err != nil {
		return appstore.ServerError("初始化模板存储失败", err)
	}
	if rawID == "default" {
		if _, err := db.ExecContext(ctx, "DELETE FROM system_configs WHERE `group` = 'home_template' AND `key` = 'active_template_id'"); err != nil {
			return appstore.ServerError("启用默认模板失败", err)
		}
		return nil
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return appstore.ClientError("模板标识不合法", err)
	}
	installedPath, err := installHomeTemplate(ctx, db, id)
	if err != nil {
		return appstore.ClientError("模板启用失败："+err.Error(), err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return appstore.ServerError("模板启用失败", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE home_templates SET installed_path=?, installed_at=NOW(), updated_at=NOW() WHERE id=?", installedPath, id); err != nil {
		return appstore.ServerError("保存模板安装状态失败", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO system_configs (`+"`group`"+`, `+"`key`"+`, value, description)
		VALUES ('home_template', 'active_template_id', ?, '当前启用的首页模板ID') ON DUPLICATE KEY UPDATE value=VALUES(value)`, strconv.FormatInt(id, 10)); err != nil {
		return appstore.ServerError("保存模板启用状态失败", err)
	}
	if err := tx.Commit(); err != nil {
		return appstore.ServerError("模板启用失败", err)
	}
	return nil
}

func disableAppStoreTemplate(ctx context.Context, rawID string) error {
	rawID = strings.TrimSpace(rawID)
	if rawID == "default" {
		return appstore.ClientError("默认模板不能禁用", nil)
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return appstore.ClientError("模板标识不合法", err)
	}
	db, err := openSystemConfigDB()
	if err != nil {
		return appstore.ServerError("数据库连接失败", err)
	}
	if err := ensurePluginStorage(db); err != nil {
		return appstore.ServerError("初始化模板存储失败", err)
	}
	var existingID int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM home_templates WHERE id=?", id).Scan(&existingID); errors.Is(err, sql.ErrNoRows) {
		return appstore.ClientError("模板不存在", err)
	} else if err != nil {
		return appstore.ServerError("读取首页模板失败", err)
	}
	if loadActiveHomeTemplateID(db) != id {
		return nil
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM system_configs WHERE `group`='home_template' AND `key`='active_template_id'"); err != nil {
		return appstore.ServerError("禁用首页模板失败", err)
	}
	return nil
}

func legacyHomeTemplateItems(items []appstore.Template) []gin.H {
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		var id any = item.ID
		version := item.Version
		if item.ID == "default" {
			version = "builtin"
		} else if numericID, err := strconv.ParseInt(item.ID, 10, 64); err == nil {
			id = numericID
		}
		result = append(result, gin.H{
			"id": id, "catalogId": item.CatalogID, "templateId": item.TemplateID, "name": item.Name, "description": item.Description,
			"version": version, "source": item.Source, "sourceUrl": item.SourceURL, "sourceType": item.SourceType,
			"previewUrl": item.PreviewImage, "author": templateAuthor{Name: item.Author.Name, URL: item.Author.URL, Email: item.Author.Email},
			"sha256": item.SHA256, "schemaVersion": item.SchemaVersion, "enabled": item.Enabled,
			"installed": item.Installed, "available": item.Available, "updatedAt": item.UpdatedAt, "updateAvailable": item.UpdateAvailable,
		})
	}
	return result
}

func PublicSoftwareSourceTemplatePreview(c *gin.Context) {
	client, err := softwaresource.Default()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": err.Error()})
		return
	}
	payload, contentType, err := client.TemplatePreview(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "模板示例图片读取失败"})
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=3600")
	if strings.Contains(strings.ToLower(contentType), "svg") {
		c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	c.Data(http.StatusOK, contentType, payload)
}

func writeAppStoreError(c *gin.Context, err error) {
	code, message := appstore.ErrorResponse(err)
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": message})
}
