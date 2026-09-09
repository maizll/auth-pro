package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"auto_pro/appstore"
	"auto_pro/config"
	"auto_pro/softwaresource"
	"github.com/gin-gonic/gin"
)

// ponytail: filesystem mutations run in one server process; use a database lease
// if multiple auth-pro processes ever share the same template installation root.
var homeTemplateMutationMu sync.Mutex

func AdminHomeTemplateInstall(c *gin.Context) {
	if err := applyHomeTemplate(c.Request.Context(), c.Param("id"), false); err != nil {
		writeAppStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已安装，尚未切换当前首页"})
}

func AdminHomeTemplateDisable(c *gin.Context) {
	if err := disableAppStoreTemplate(c.Request.Context(), c.Param("id")); err != nil {
		writeAppStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已停用，安装文件保留"})
}

func AdminHomeTemplateUninstall(c *gin.Context) {
	if err := uninstallAppStoreTemplate(c.Request.Context(), c.Param("id")); err != nil {
		writeAppStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已卸载；若原来正在使用，已恢复默认首页"})
}

func AdminHomeTemplateDownload(c *gin.Context) {
	payload, filename, contentType, err := downloadHomeTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAppStoreError(c, err)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Checksum-SHA256", actualChecksumString(sha256.Sum256(payload)))
	c.Data(http.StatusOK, contentType, payload)
}

func downloadHomeTemplate(ctx context.Context, rawID string) ([]byte, string, string, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err != nil || id <= 0 {
		return nil, "", "", appstore.ClientError("模板标识不合法，内置默认模板无需下载", err)
	}
	db, err := openSystemConfigDB()
	if err != nil {
		return nil, "", "", appstore.ServerError("数据库连接失败", err)
	}
	var catalogID, key, installedPath string
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(catalog_id, ''), template_key, installed_path FROM home_templates WHERE id=?", id).
		Scan(&catalogID, &key, &installedPath); err != nil {
		return nil, "", "", appstore.ClientError("模板不存在", err)
	}
	if catalogID != "" {
		if client, err := softwaresource.Default(); err == nil {
			if remote, err := client.FindTemplate(ctx, catalogID); err == nil && remote.Available && remote.Published {
				if payload, err := client.TemplateContent(ctx, remote); err == nil {
					extension, contentType := "json", "application/json"
					if remote.Format == "zip" {
						extension, contentType = "zip", "application/zip"
					}
					return payload, remote.TemplateKey + "-" + remote.Version + "." + extension, contentType, nil
				}
			}
		}
	}
	// An installed package stays downloadable when its source is offline or withdrawn.
	directory, err := managedHomeTemplateDirectory(installedPath, id)
	if err != nil {
		return nil, "", "", appstore.ClientError("模板暂不可下载，请刷新软件源后重试", err)
	}
	if marker, err := readCatalogTemplateInstallation(installedPath); err == nil {
		payload, err := readLimitedFile(filepath.Join(directory, catalogTemplatePackageFile), pluginPackageMaxSize)
		if err == nil && strings.EqualFold(actualChecksumString(sha256.Sum256(payload)), marker.SHA256) {
			return payload, key + "-" + marker.Version + ".zip", "application/zip", nil
		}
		return nil, "", "", appstore.ClientError("本地模板压缩包校验失败，请重新安装", err)
	}
	if filepath.Base(installedPath) == "template.json" {
		payload, err := readLimitedFile(installedPath, homeTemplateMaxBytes)
		if err == nil && validateHomeTemplateDocument(payload) == nil {
			return payload, key + "-installed.json", "application/json", nil
		}
	}
	return nil, "", "", appstore.ClientError("没有可下载的模板原包，请从分发中心重新安装", nil)
}

func listStoredCatalogTemplates(ctx context.Context, db *sql.DB, activeID int64) ([]appstore.Template, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(catalog_id, ''), template_key, name, description, version,
        author_name, sha256, schema_version, catalog_snapshot FROM home_templates
        WHERE source_type <> 'upload' AND installed_path <> '' ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []appstore.Template{}
	for rows.Next() {
		var item appstore.Template
		var id int64
		var snapshot []byte
		if err := rows.Scan(&id, &item.CatalogID, &item.TemplateID, &item.Name, &item.Description, &item.Version,
			&item.Author.Name, &item.SHA256, &item.SchemaVersion, &snapshot); err != nil {
			return nil, err
		}
		var remote softwaresource.Template
		_ = json.Unmarshal(snapshot, &remote)
		item.ID, item.Format = strconv.FormatInt(id, 10), remote.Format
		item.Enabled, item.Installed = activeID == id, true
		item.Source, item.SourceType = remote.Source.Name, remote.Source.Type
		if item.Source == "" {
			item.Source = "已安装模板（软件源暂不可用）"
		}
		normalizeAppStoreTemplateMetadata(&item)
		items = append(items, item)
	}
	return items, rows.Err()
}

// Validate the absolute directory against the server's own installation layouts,
// including missing directories, before any rename or recursive removal.
func managedHomeTemplateDirectory(installedPath string, id int64) (string, error) {
	if installedPath == "" || (filepath.Base(installedPath) != "template.json" && filepath.Base(installedPath) != "index.html") {
		return "", errors.New("模板安装路径不合法")
	}
	base, err := filepath.Abs(config.GetHomeTemplateDir())
	if err != nil {
		return "", err
	}
	directory, err := filepath.Abs(filepath.Dir(installedPath))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(base, directory)
	if err != nil || filepath.IsAbs(relative) {
		return "", errors.New("模板安装路径超出数据目录")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	valid := len(parts) == 1 && strings.HasPrefix(parts[0], "upload-") && len(parts[0]) > len("upload-")
	if len(parts) == 2 && parts[0] == strconv.FormatInt(id, 10) && filepath.Base(installedPath) == "template.json" {
		checksum, err := hex.DecodeString(parts[1])
		valid = err == nil && len(checksum) == 8
	}
	if !valid || validatePackagePath(filepath.ToSlash(relative)) != nil {
		return "", errors.New("模板安装路径不属于该模板")
	}
	target := base
	for _, part := range parts {
		target = filepath.Join(target, part)
		info, err := os.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			return directory, nil
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("模板安装目录不能是链接或特殊文件")
		}
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	resolvedRelative, err := filepath.Rel(resolvedBase, resolved)
	if err != nil || !strings.EqualFold(resolvedRelative, relative) {
		return "", errors.New("模板安装目录解析后越界")
	}
	return directory, nil
}

func removeReplacedHomeTemplate(installedPath string, id int64) {
	directory, err := managedHomeTemplateDirectory(installedPath, id)
	if err == nil {
		err = os.RemoveAll(directory)
	}
	if err != nil {
		log.Printf("home template %d: old installation cleanup failed: %v", id, err)
	}
}

func uninstallAppStoreTemplate(ctx context.Context, rawID string) error {
	homeTemplateMutationMu.Lock()
	defer homeTemplateMutationMu.Unlock()
	id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err != nil || id <= 0 {
		return appstore.ClientError("模板标识不合法，内置默认模板不能卸载", err)
	}
	db, err := openSystemConfigDB()
	if err != nil {
		return appstore.ServerError("数据库连接失败", err)
	}
	if err := ensurePluginStorage(db); err != nil {
		return appstore.ServerError("初始化模板存储失败", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return appstore.ServerError("模板卸载失败", err)
	}
	defer tx.Rollback()
	var installedPath, sourceType string
	if err := tx.QueryRowContext(ctx, "SELECT installed_path, source_type FROM home_templates WHERE id=? FOR UPDATE", id).
		Scan(&installedPath, &sourceType); err != nil {
		return appstore.ClientError("模板不存在", err)
	}
	directory, quarantine := "", ""
	committed := false
	defer func() {
		if quarantine == "" {
			return
		}
		if !committed {
			if err := os.Rename(filepath.Join(quarantine, "content"), directory); err != nil {
				log.Printf("home template %d: uninstall rollback failed: %v", id, err)
				return
			}
		}
		if err := os.RemoveAll(quarantine); err != nil {
			log.Printf("home template %d: quarantined files cleanup failed: %v", id, err)
		}
	}()
	if installedPath != "" {
		directory, err = managedHomeTemplateDirectory(installedPath, id)
		if err != nil {
			return appstore.ClientError("拒绝卸载不安全的模板路径", err)
		}
		if _, err := os.Lstat(directory); err == nil {
			quarantine, err = os.MkdirTemp(config.GetHomeTemplateDir(), ".uninstall-")
			if err != nil {
				return appstore.ServerError("创建卸载临时目录失败", err)
			}
			if err := os.Rename(directory, filepath.Join(quarantine, "content")); err != nil {
				_ = os.Remove(quarantine)
				quarantine = ""
				return appstore.ServerError("模板文件占用或移动失败，请稍后重试", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return appstore.ServerError("读取模板安装目录失败", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM system_configs WHERE `group`='home_template' AND `key`='active_template_id' AND value=?", strconv.FormatInt(id, 10)); err != nil {
		return appstore.ServerError("停用待卸载模板失败", err)
	}
	if sourceType == "upload" {
		_, err = tx.ExecContext(ctx, "DELETE FROM home_templates WHERE id=?", id)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE home_templates SET installed_path='', installed_at=NULL, updated_at=NOW() WHERE id=?", id)
	}
	if err != nil {
		return appstore.ServerError("清理模板安装状态失败", err)
	}
	if err := tx.Commit(); err != nil {
		return appstore.ServerError("保存模板卸载状态失败", err)
	}
	committed = true
	return nil
}
