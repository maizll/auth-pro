package handler

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 强制更新提醒和自动更新。
// 后台每小时读一次更新清单（和「检查更新」同一个来源），结果放进内存缓存，后台横幅和弹框据此提示超级管理员。
// 开了「自动更新」时，只有强制更新会在服务器时间凌晨 3–5 点自动安装，安装走和「立即更新」完全相同的任务：
// 下载、核对签名、备份数据库、切换、健康检查不过自动回退。同一版本自动安装失败过一次就不再自动重试，等人处理。

const (
	onlineUpdateAutoGroup      = "online_update"
	onlineUpdateAutoEnabledKey = "auto_update"
	onlineUpdateAutoPendingKey = "auto_pending_job"
	onlineUpdateAutoFailedKey  = "auto_failed_version"
	onlineUpdateAutoStartHour  = 3
	onlineUpdateAutoEndHour    = 5
	onlineUpdateAutoWindowText = "凌晨 3:00–5:00"
)

var (
	onlineUpdateWatcherOnce sync.Once
	// 以下几项测试里替换
	onlineUpdateWatchNow          = time.Now
	fetchOnlineUpdateManifestAuto = fetchOnlineUpdateManifest
	launchOnlineUpdateJobAuto     = launchOnlineUpdateJob
	onlineUpdateAutoRuntimeOK     = func() bool { return runtime.GOOS == "linux" && runtime.GOARCH == "amd64" }
)

// StartOnlineUpdateWatcher 启动后台更新检查：启动 1 分钟后先查一次，之后每小时一次。
func StartOnlineUpdateWatcher() {
	onlineUpdateWatcherOnce.Do(func() {
		go func() {
			time.Sleep(time.Minute)
			runOnlineUpdateWatch()
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				runOnlineUpdateWatch()
			}
		}()
	})
}

// runOnlineUpdateWatch 先补记上一次自动更新的结果，再读清单；满足条件时自动安装强制更新。
func runOnlineUpdateWatch() {
	db, err := openSystemConfigDB()
	if err != nil {
		return
	}
	if err := ensureSystemConfigStorage(db); err != nil {
		return
	}
	settleOnlineUpdateAutoResult(db)
	manifest, err := fetchOnlineUpdateManifestAuto()
	if err != nil {
		return
	}
	setCachedOnlineUpdateManifest(manifest)
	maybeAutoApplyOnlineUpdate(db, manifest, onlineUpdateWatchNow())
}

// maybeAutoApplyOnlineUpdate 判断这次是否自动安装，返回启动的任务（没启动为 nil）。
func maybeAutoApplyOnlineUpdate(db *sql.DB, manifest *onlineUpdateManifest, now time.Time) *onlineUpdateJob {
	if manifest == nil || !manifest.Force || !onlineUpdateAutoEnabled(db) || !onlineUpdateAutoRuntimeOK() {
		return nil
	}
	if hour := now.Hour(); hour < onlineUpdateAutoStartHour || hour >= onlineUpdateAutoEndHour {
		return nil
	}
	if _, _, _, canApply := evaluateOnlineUpdateCheck(config.AppVersion, manifest); !canApply {
		return nil
	}
	if sameProductVersion(onlineUpdateAutoValue(db, onlineUpdateAutoFailedKey), manifest.Version) {
		return nil
	}
	job := launchOnlineUpdateJobAuto(manifest, func(jobID string) {
		writeOnlineUpdateSystemLog(db, "online_update_auto_apply", map[string]any{
			"jobId": jobID, "toVersion": manifest.Version, "sha256": manifest.Package.SHA256,
		})
	})
	if job != nil {
		_ = saveOnlineUpdateAutoValue(db, onlineUpdateAutoPendingKey, job.ID)
	}
	return job
}

// settleOnlineUpdateAutoResult 自动更新的任务结束后（新进程起来或已回退）记一条结果日志。
func settleOnlineUpdateAutoResult(db *sql.DB) {
	jobID := onlineUpdateAutoValue(db, onlineUpdateAutoPendingKey)
	if jobID == "" {
		return
	}
	job := loadOnlineUpdateJob(jobID)
	if job != nil && job.Status != "success" && job.Status != "failed" {
		return
	}
	detail := map[string]any{"jobId": jobID, "status": "missing"}
	if job != nil {
		detail["status"] = job.Status
		detail["toVersion"] = job.Version
		if job.Status == "failed" {
			detail["reason"] = firstNonEmpty(job.Error, job.Message)
			_ = saveOnlineUpdateAutoValue(db, onlineUpdateAutoFailedKey, job.Version)
		}
	}
	writeOnlineUpdateSystemLog(db, "online_update_auto_result", detail)
	_ = saveOnlineUpdateAutoValue(db, onlineUpdateAutoPendingKey, "")
}

func onlineUpdateAutoValue(db *sql.DB, key string) string {
	var value string
	if err := db.QueryRow("SELECT value FROM system_configs WHERE `group` = ? AND `key` = ?", onlineUpdateAutoGroup, key).Scan(&value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func saveOnlineUpdateAutoValue(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT INTO system_configs (`group`, `key`, value, description) VALUES (?, ?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE value = VALUES(value)", onlineUpdateAutoGroup, key, value, "在线更新")
	return err
}

func onlineUpdateAutoEnabled(db *sql.DB) bool {
	return onlineUpdateAutoValue(db, onlineUpdateAutoEnabledKey) == "1"
}

// writeOnlineUpdateSystemLog 记系统自己发起的更新操作（没有操作人）。
func writeOnlineUpdateSystemLog(db *sql.DB, action string, detail map[string]any) {
	detail["fromVersion"] = config.AppVersion
	raw, err := json.Marshal(detail)
	if err != nil {
		return
	}
	if _, err := db.Exec(`INSERT INTO operation_logs
		(operator_type, operator_id, action, target_type, target_id, detail, ip)
		VALUES ('system', 0, ?, 'system_update', NULL, ?, '')`, action, string(raw)); err != nil {
		log.Printf("记录自动更新日志失败: %v", err)
	}
}

// onlineUpdateNotice 是后台横幅和弹框要的强制更新信息，来自最近一次读到的清单。
func onlineUpdateNotice(db *sql.DB) gin.H {
	notice := gin.H{"force": false, "autoUpdate": db != nil && onlineUpdateAutoEnabled(db), "autoWindow": onlineUpdateAutoWindowText}
	manifest := cachedOnlineUpdateManifest()
	if manifest == nil || !manifest.Force {
		return notice
	}
	available, versionErr := onlineUpdateAvailable(config.AppVersion, manifest)
	if !available || versionErr != "" {
		return notice
	}
	notice["force"] = true
	notice["version"] = strings.TrimPrefix(manifest.Version, "v")
	notice["releasedAt"] = manifest.ReleasedAt
	notice["notes"] = manifest.Notes
	if db != nil && sameProductVersion(onlineUpdateAutoValue(db, onlineUpdateAutoFailedKey), manifest.Version) {
		notice["autoFailed"] = true
	}
	return notice
}

// AdminOnlineUpdateNotice 返回强制更新提醒（仅超级管理员）。
func AdminOnlineUpdateNotice(c *gin.Context) {
	db, _ := openSystemConfigDB()
	if db != nil && ensureSystemConfigStorage(db) != nil {
		db = nil
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": onlineUpdateNotice(db)})
}

// AdminOnlineUpdateAutoSave 打开或关闭自动更新，写操作日志。
func AdminOnlineUpdateAutoSave(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数不正确"})
		return
	}
	db, err := openSystemConfigDB()
	if err == nil {
		err = ensureSystemConfigStorage(db)
	}
	value := "0"
	if req.Enabled {
		value = "1"
	}
	if err == nil {
		err = saveOnlineUpdateAutoValue(db, onlineUpdateAutoEnabledKey, value)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存失败，请稍后重试"})
		return
	}
	writeOnlineUpdateAuditLog(c, "online_update_auto_switch", map[string]any{"enabled": req.Enabled})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存", "data": onlineUpdateNotice(db)})
}
