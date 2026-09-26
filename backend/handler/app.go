package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// AppList 应用列表（下拉选择用）
func AppList(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	if err := ensureAppDeletedAt(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用归档字段失败"})
		return
	}
	rows, err := db.Query("SELECT id, app_name FROM apps WHERE enabled = 1 AND deleted_at IS NULL ORDER BY id ASC")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询失败"})
		return
	}
	defer rows.Close()

	type appItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	var list []appItem
	for rows.Next() {
		var item appItem
		if err := rows.Scan(&item.ID, &item.Name); err == nil {
			list = append(list, item)
		}
	}
	if list == nil {
		list = []appItem{}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "",
		"data": list,
	})
}

// AppManageList 应用管理列表（含详细信息）
func AppManageList(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := EnsureAppPurchaseLicenseTypesColumn(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用授权方式失败"})
		return
	}
	if err := ensureAppLicenseRequiredColumn(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用授权开关失败"})
		return
	}
	if err := EnsureAppVersionsTable(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化版本数据失败"})
		return
	}
	if err := ensureAppDeletedAt(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用归档字段失败"})
		return
	}
	if err := prepareCommercialProduct(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化商业版产品失败"})
		return
	}
	storeSettings, settingsErr := loadSourceStoreSettings(db)
	if settingsErr == nil {
		if normalized, err := normalizeStoreSettings(storeSettings); err == nil {
			storeSettings = normalized
		}
	}

	rows, err := db.Query(`
		SELECT a.id, a.app_name, a.app_key, a.app_secret, a.description, a.enabled, a.commercial_product,
		       a.license_required, a.purchase_license_type_mask, a.created_at, a.deleted_at,
		       (SELECT COUNT(*) FROM licenses l WHERE l.app_id = a.id) AS license_count,
		       (SELECT COUNT(*) FROM app_versions v WHERE v.app_id = a.id) AS version_count,
		       COALESCE((
		           SELECT v.version FROM app_versions v WHERE v.app_id = a.id
		           ORDER BY v.published_at DESC, v.id DESC LIMIT 1
		       ), '') AS recent_version
		FROM apps a ORDER BY a.id ASC
	`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询失败"})
		return
	}
	defer rows.Close()

	type appManageItem struct {
		ID                     int64               `json:"id"`
		Name                   string              `json:"name"`
		AppKey                 string              `json:"appKey"`
		AppSecret              string              `json:"appSecret"`
		Remark                 string              `json:"remark"`
		Enabled                bool                `json:"enabled"`
		Archived               bool                `json:"archived"`
		LicenseRequired        bool                `json:"licenseRequired"`
		PurchaseLicenseTypes   []string            `json:"purchaseLicenseTypes"`
		CommercialProduct      bool                `json:"commercialProduct"`
		SaleGaps               []commercialSaleGap `json:"saleGaps,omitempty"`
		GraceDays              int                 `json:"graceDays,omitempty"`
		RevokeOnPasswordChange *bool               `json:"revokeOnPasswordChange,omitempty"`
		CommercialFeatures     []string            `json:"commercialFeatures,omitempty"`
		CreatedAt              string              `json:"createdAt"`
		LicenseCount           int64               `json:"licenseCount"`
		VersionCount           int64               `json:"versionCount"`
		RecentVersion          string              `json:"recentVersion"`
	}

	var list []appManageItem
	for rows.Next() {
		var item appManageItem
		var createdAt time.Time
		var deletedAt sql.NullTime
		var desc string
		var purchaseLicenseTypeMask uint8
		var commercial int
		if err := rows.Scan(&item.ID, &item.Name, &item.AppKey, &item.AppSecret,
			&desc, &item.Enabled, &commercial, &item.LicenseRequired, &purchaseLicenseTypeMask, &createdAt, &deletedAt, &item.LicenseCount, &item.VersionCount,
			&item.RecentVersion); err == nil {
			item.Archived = deletedAt.Valid
			item.Remark = desc
			item.CommercialProduct = commercial == 1
			item.PurchaseLicenseTypes = purchaseLicenseTypesFromMask(purchaseLicenseTypeMask)
			item.CreatedAt = createdAt.Format("2006-01-02 15:04")
			if item.CommercialProduct {
				item.SaleGaps = commercialSaleGaps(db, item.ID, item.Enabled)
				item.GraceDays = storeSettings.GraceDays
				item.RevokeOnPasswordChange = storeSettings.RevokeOnPasswordChange
				item.CommercialFeatures = storeSettings.CommercialFeatures
			}
			list = append(list, item)
		}
	}
	if list == nil {
		list = []appManageItem{}
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": list})
}

// AppCreate 新增应用
func AppCreate(c *gin.Context) {
	var req struct {
		Name                   string    `json:"name" binding:"required"`
		Enabled                bool      `json:"enabled"`
		Remark                 string    `json:"remark"`
		PurchaseLicenseTypes   []string  `json:"purchaseLicenseTypes"`
		CommercialProduct      *bool     `json:"commercialProduct"`
		GraceDays              *int      `json:"graceDays"`
		RevokeOnPasswordChange *bool     `json:"revokeOnPasswordChange"`
		CommercialFeatures     *[]string `json:"commercialFeatures"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "应用名称不能为空"})
		return
	}
	if req.CommercialProduct != nil && *req.CommercialProduct {
		if err := validateCommercialAppSettings(req.GraceDays, req.CommercialFeatures); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	}
	purchaseLicenseTypeMask, err := purchaseLicenseTypeMaskForCreate(req.PurchaseLicenseTypes)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	var appCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apps`).Scan(&appCount); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取应用数量失败"})
		return
	}
	commercial := false
	if appCount >= 1 {
		commercial = buyerFeatureEnabled(c, storeFeatureMultiApp)
	}
	if !appCreateDecision(appCount, commercial) {
		writeEditionRequired(c, storeFeatureMultiApp)
		return
	}
	if err := EnsureAppPurchaseLicenseTypesColumn(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用授权方式失败"})
		return
	}

	appKey := fmt.Sprintf("app_%s_%d", randomHex(6), time.Now().Unix()%10000)
	appSecret := "sk_live_" + randomHex(16)

	result, err := db.Exec(`
		INSERT INTO apps (app_name, app_key, app_secret, description, enabled, purchase_license_type_mask)
		VALUES (?, ?, ?, ?, ?, ?)
	`, req.Name, appKey, appSecret, req.Remark, req.Enabled, purchaseLicenseTypeMask)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建失败: " + err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	switched, msg, err := applyCommercialProductChoice(db, id, req.CommercialProduct, req.GraceDays, req.RevokeOnPasswordChange, req.CommercialFeatures)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if msg == "" {
		msg = "创建成功"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": gin.H{"id": id, "switched": switched}})
}

// AppUpdate 编辑应用
func AppUpdate(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name                   string    `json:"name" binding:"required"`
		Enabled                bool      `json:"enabled"`
		Remark                 string    `json:"remark"`
		PurchaseLicenseTypes   []string  `json:"purchaseLicenseTypes"`
		CommercialProduct      *bool     `json:"commercialProduct"`
		GraceDays              *int      `json:"graceDays"`
		RevokeOnPasswordChange *bool     `json:"revokeOnPasswordChange"`
		CommercialFeatures     *[]string `json:"commercialFeatures"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if req.CommercialProduct != nil && *req.CommercialProduct {
		if err := validateCommercialAppSettings(req.GraceDays, req.CommercialFeatures); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	}
	var purchaseLicenseTypeMask uint8
	if req.PurchaseLicenseTypes != nil {
		var err error
		purchaseLicenseTypeMask, err = parsePurchaseLicenseTypes(req.PurchaseLicenseTypes)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := EnsureAppPurchaseLicenseTypesColumn(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用授权方式失败"})
		return
	}

	var result sql.Result
	if req.PurchaseLicenseTypes == nil {
		result, err = db.Exec("UPDATE apps SET app_name = ?, description = ?, enabled = ? WHERE id = ?",
			req.Name, req.Remark, req.Enabled, id)
	} else {
		result, err = db.Exec(`
			UPDATE apps
			SET app_name = ?, description = ?, enabled = ?, purchase_license_type_mask = ?
			WHERE id = ?
		`, req.Name, req.Remark, req.Enabled, purchaseLicenseTypeMask, id)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败"})
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败"})
		return
	}
	// MariaDB/MySQL 在赋值和原值相同时 RowsAffected 为 0，updated_at 也不会动。
	// 不能据此报「应用不存在」。商业版开关不在这条 UPDATE 里，确认行还在之后还要继续写。
	if rowsAffected == 0 {
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM apps WHERE id = ?", id).Scan(&exists); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "检查应用状态失败"})
			return
		}
		if exists == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "应用不存在"})
			return
		}
	}
	appID, err := positiveInt64(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "应用ID不正确"})
		return
	}
	switched, msg, err := applyCommercialProductChoice(db, appID, req.CommercialProduct, req.GraceDays, req.RevokeOnPasswordChange, req.CommercialFeatures)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if msg == "" {
		msg = "更新成功"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": gin.H{"switched": switched}})
}

// AppEnsureStoreSnapshotKey 在应用列表里补生成商店签名私钥，不另开页面。
func AppEnsureStoreSnapshotKey(c *gin.Context) {
	if _, err := loadStoreSnapshotPrivateKey(); err == nil {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "签名密钥已存在"})
		return
	}
	if _, err := os.Stat(storeSnapshotPrivateKeyPath()); err == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "签名密钥不可用，请检查本机私钥是否与发行包公钥一致"})
		return
	}
	if _, err := generateStoreSnapshotKey(false); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成签名密钥失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已生成签名密钥"})
}

// AppLicenseRequiredUpdate 更新应用是否要求许可证校验。
func AppLicenseRequiredUpdate(c *gin.Context) {
	id, err := positiveInt64(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "应用ID不正确"})
		return
	}

	var req struct {
		LicenseRequired *bool `json:"licenseRequired"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.LicenseRequired == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "授权校验状态不正确"})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureAppLicenseRequiredColumn(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用授权开关失败"})
		return
	}

	result, err := db.Exec("UPDATE apps SET license_required = ? WHERE id = ?", *req.LicenseRequired, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新授权校验状态失败"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM apps WHERE id = ?", id).Scan(&exists); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "检查应用状态失败"})
			return
		}
		if exists == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "应用不存在"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "授权校验状态已更新",
		"data": gin.H{"licenseRequired": *req.LicenseRequired},
	})
}

// AppResetSecret 重置应用密钥
func AppResetSecret(c *gin.Context) {
	id := c.Param("id")

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	newSecret := "sk_live_" + randomHex(16)
	_, err = db.Exec("UPDATE apps SET app_secret = ? WHERE id = ?", newSecret, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "重置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "密钥已重置", "data": gin.H{"appSecret": newSecret}})
}

// AppDelete 删除应用
func AppDelete(c *gin.Context) {
	id := c.Param("id")
	appID, convErr := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if convErr != nil || appID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "应用不存在"})
		return
	}
	migrateAppID, migrateErr := parseMigrateAppID(c)
	if migrateErr != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": migrateErr.Error()})
		return
	}
	archiveInPlace := requestAppArchiveInPlace(c)
	if err := relocateCatalogBeforeAppDelete(appID, migrateAppID, archiveInPlace, c.GetString("username")); err != nil {
		var required appCatalogMigrateRequiredError
		if errors.As(err, &required) {
			c.JSON(http.StatusOK, gin.H{"code": 409, "msg": err.Error(), "data": gin.H{"count": required.Count}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureAppDeletedAt(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用归档字段失败"})
		return
	}

	tx, err := db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "归档应用失败"})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE apps SET deleted_at = UTC_TIMESTAMP(), enabled = 0 WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "归档失败"})
		return
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		var deletedAt sql.NullTime
		scanErr := tx.QueryRow(`SELECT deleted_at FROM apps WHERE id = ?`, id).Scan(&deletedAt)
		if scanErr == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "应用不存在"})
			return
		}
		if scanErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "归档失败"})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "归档应用失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "应用已归档"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "归档应用失败"})
		return
	}
	detail := "归档应用，授权、套餐和版本保留"
	if migrateAppID > 0 {
		detail = fmt.Sprintf("目录条目已迁移到应用 %d 后归档", migrateAppID)
	} else if archiveInPlace {
		detail = "直接归档，目录条目仍绑定本应用"
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: c.GetString("username"), Action: "archive_app",
		TargetType: "app", TargetID: id, Detail: detail,
	})

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "应用已归档"})
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
