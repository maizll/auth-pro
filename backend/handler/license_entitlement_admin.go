// 授权详情里的商业版记录、已购插件和操作日志。
// 赠送、撤销和开通都写进同一张日志，避免商业版和插件各记一处。

package handler

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const storeMigrationLicenseOps = "license_operation_logs_v1"

func validEditionPeriod(period string) bool {
	switch strings.TrimSpace(period) {
	case storePeriodPermanent, storePeriodYearly, storePeriodMonthly:
		return true
	default:
		return false
	}
}

func editionPeriodLabel(period string) string {
	switch period {
	case storePeriodYearly:
		return "按年"
	case storePeriodMonthly:
		return "按月"
	default:
		return "永久"
	}
}

func migrateLicenseOperationLogs(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS license_operation_logs (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		license_id BIGINT UNSIGNED NOT NULL,
		action VARCHAR(40) NOT NULL,
		target VARCHAR(80) NOT NULL DEFAULT '',
		detail VARCHAR(500) NOT NULL DEFAULT '',
		actor VARCHAR(80) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_license_operation (license_id, id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权开通、赠送、撤销和转移记录'`)
	if err != nil {
		return err
	}
	return nil
}

func writeLicenseOperation(db *sql.DB, licenseID int64, action, target, detail, actor string) {
	if db == nil || licenseID <= 0 || strings.TrimSpace(action) == "" {
		return
	}
	_, _ = db.Exec(`INSERT INTO license_operation_logs (license_id, action, target, detail, actor) VALUES (?, ?, ?, ?, ?)`,
		licenseID, trimStoreText(action, 40), trimStoreText(target, 80), trimStoreText(detail, 500), trimStoreText(actor, 80))
}

func registerLicenseEntitlementRoutes(licenses *gin.RouterGroup) {
	licenses.GET("/licenses/:id/detail", AdminLicenseCommercialDetail)
	licenses.POST("/licenses/:id/plugins/gift", AdminLicensePluginGift)
	licenses.POST("/licenses/:id/plugins/:entitlementId/revoke", AdminLicensePluginRevoke)
}

// AdminLicenseCommercialDetail 返回一条授权的商业版记录、已购插件和操作日志。
// 授权不存在返回 400。读库失败返回 500。到期时间空表示永久。
func AdminLicenseCommercialDetail(c *gin.Context) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || licenseID <= 0 {
		storeFail(c, 400, "授权不正确")
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	var licenseNo string
	if err := db.QueryRow(`SELECT license_no FROM licenses WHERE id = ?`, licenseID).Scan(&licenseNo); err != nil {
		storeFail(c, 400, "授权不正确")
		return
	}
	editions, err := listLicenseEditions(db, licenseID)
	if err != nil {
		storeFail(c, 500, "读取商业版记录失败")
		return
	}
	plugins, err := listLicensePluginEntitlements(db, licenseID)
	if err != nil {
		storeFail(c, 500, "读取已购插件失败")
		return
	}
	logs, err := listLicenseOperations(db, licenseID)
	if err != nil {
		storeFail(c, 500, "读取操作记录失败")
		return
	}
	catalog, _ := listGiftableCatalog(db)
	storeData(c, gin.H{
		"licenseNo": licenseNo, "editions": editions, "plugins": plugins, "logs": logs, "catalog": catalog,
	})
}

// AdminLicensePluginGift 给这条授权单独赠送一个插件或模板。
// period 为 permanent、yearly 或 monthly。按年、按月会写入到期时间，到期后不再有效。
func AdminLicensePluginGift(c *gin.Context) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || licenseID <= 0 {
		storeFail(c, 400, "授权不正确")
		return
	}
	var req struct {
		ItemKind string `json:"itemKind"`
		ItemID   string `json:"itemId"`
		Period   string `json:"period"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	req.ItemKind = strings.TrimSpace(req.ItemKind)
	req.ItemID = strings.TrimSpace(req.ItemID)
	if req.Period == "" {
		req.Period = storePeriodPermanent
	}
	if (req.ItemKind != "plugin" && req.ItemKind != "template") || req.ItemID == "" || !validEditionPeriod(req.Period) {
		storeFail(c, 400, "请选择要赠送的插件和期限")
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	var ownerType string
	var ownerID int64
	if err := db.QueryRow(`SELECT owner_type, owner_id FROM licenses WHERE id = ?`, licenseID).Scan(&ownerType, &ownerID); err != nil {
		storeFail(c, 400, "授权不正确")
		return
	}
	expiry := nextEditionExpiry(req.Period, nil, time.Now())
	var expires any
	if expiry != nil {
		expires = *expiry
	}
	if _, err := db.Exec(`INSERT INTO plugin_entitlements
		(license_id, owner_type, owner_id, item_kind, item_id, period, expires_at, source, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'gift', 'active')`,
		licenseID, ownerType, ownerID, req.ItemKind, req.ItemID, req.Period, expires); err != nil {
		storeFail(c, 500, "赠送失败")
		return
	}
	writeLicenseOperation(db, licenseID, "gift_plugin", req.ItemKind+":"+req.ItemID, "赠送"+editionPeriodLabel(req.Period), c.GetString("username"))
	storeData(c, gin.H{"ok": true})
}

// AdminLicensePluginRevoke 撤销这一条已购或赠送的插件，不影响同一授权上的其他插件和商业版。
func AdminLicensePluginRevoke(c *gin.Context) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	entitlementID, idErr := strconv.ParseInt(strings.TrimSpace(c.Param("entitlementId")), 10, 64)
	if err != nil || idErr != nil || licenseID <= 0 || entitlementID <= 0 {
		storeFail(c, 400, "参数错误")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	reason := trimStoreText(req.Reason, 200)
	res, err := db.Exec(`UPDATE plugin_entitlements SET status = 'revoked', revoked_at = NOW(), revoke_reason = ?
		WHERE id = ? AND license_id = ? AND status = 'active'`, reason, entitlementID, licenseID)
	if err != nil {
		storeFail(c, 500, "撤销失败")
		return
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		storeFail(c, 400, "这条插件权益不存在或已经撤销")
		return
	}
	writeLicenseOperation(db, licenseID, "revoke_plugin", strconv.FormatInt(entitlementID, 10), reason, c.GetString("username"))
	storeData(c, gin.H{"ok": true})
}

func listLicenseEditions(db *sql.DB, licenseID int64) ([]gin.H, error) {
	rows, err := db.Query(`SELECT id, period, started_at, expires_at, status, order_id, granted_by
		FROM main_license_editions WHERE license_id = ? ORDER BY id DESC LIMIT 50`, licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var period, status string
		var started time.Time
		var expires sql.NullTime
		var orderID, grantedBy sql.NullInt64
		if err := rows.Scan(&id, &period, &started, &expires, &status, &orderID, &grantedBy); err != nil {
			return nil, err
		}
		item := gin.H{
			"id": id, "period": period, "periodLabel": editionPeriodLabel(period),
			"status": status, "statusLabel": editionStatusLabel(status),
			"startedAt": started.Format("2006-01-02 15:04"),
			"source":    editionSourceLabel(orderID, grantedBy),
			"expireAt":  "永久",
		}
		if expires.Valid {
			item["expireAt"] = expires.Time.Format("2006-01-02 15:04")
			item["expired"] = !expires.Time.After(time.Now())
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func listLicensePluginEntitlements(db *sql.DB, licenseID int64) ([]gin.H, error) {
	rows, err := db.Query(`SELECT id, item_kind, item_id, period, expires_at, source, status, granted_at, revoke_reason
		FROM plugin_entitlements WHERE license_id = ? ORDER BY id DESC LIMIT 100`, licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	now := time.Now()
	for rows.Next() {
		var id int64
		var kind, itemID, period, source, status, reason string
		var expires sql.NullTime
		var granted time.Time
		if err := rows.Scan(&id, &kind, &itemID, &period, &expires, &source, &status, &granted, &reason); err != nil {
			return nil, err
		}
		active := status == "active" && (!expires.Valid || expires.Time.After(now))
		item := gin.H{
			"id": id, "itemKind": kind, "itemKindLabel": itemKindLabel(kind), "itemId": itemID,
			"period": period, "periodLabel": editionPeriodLabel(period),
			"source": source, "sourceLabel": entitlementSourceLabel(source),
			"status": status, "active": active, "reason": reason,
			"grantedAt": granted.Format("2006-01-02 15:04"),
			"expireAt":  "永久",
		}
		if expires.Valid {
			item["expireAt"] = expires.Time.Format("2006-01-02 15:04")
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func listLicenseOperations(db *sql.DB, licenseID int64) ([]gin.H, error) {
	if err := migrateLicenseOperationLogs(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT action, target, detail, actor, created_at FROM license_operation_logs
		WHERE license_id = ? ORDER BY id DESC LIMIT 50`, licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var action, target, detail, actor string
		var created time.Time
		if err := rows.Scan(&action, &target, &detail, &actor, &created); err != nil {
			return nil, err
		}
		list = append(list, gin.H{
			"action": action, "actionLabel": licenseActionLabel(action),
			"target": target, "detail": detail, "actor": actor,
			"createdAt": created.Format("2006-01-02 15:04"),
		})
	}
	return list, rows.Err()
}

func listGiftableCatalog(db *sql.DB) ([]gin.H, error) {
	list := make([]gin.H, 0)
	appendRows := func(kind, query string) error {
		rows, err := db.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			list = append(list, gin.H{"itemKind": kind, "itemId": id, "name": name, "label": itemKindLabel(kind) + " " + name})
		}
		return rows.Err()
	}
	if err := appendRows("plugin", `SELECT id, name FROM source_catalog_plugins ORDER BY name LIMIT 100`); err != nil {
		return list, nil
	}
	_ = appendRows("template", `SELECT id, name FROM source_catalog_templates ORDER BY name LIMIT 100`)
	return list, nil
}

func editionSourceLabel(orderID, grantedBy sql.NullInt64) string {
	if orderID.Valid && orderID.Int64 > 0 {
		return "购买"
	}
	if grantedBy.Valid && grantedBy.Int64 > 0 {
		return "后台开通"
	}
	return "系统"
}

func editionStatusLabel(status string) string {
	switch status {
	case "active":
		return "生效中"
	case "replaced":
		return "已更换"
	case "revoked":
		return "已撤销"
	default:
		return "其他"
	}
}

func entitlementSourceLabel(source string) string {
	switch source {
	case "purchase":
		return "购买"
	case "gift":
		return "赠送"
	case "grandfather":
		return "老用户保留"
	default:
		return "其他"
	}
}

func itemKindLabel(kind string) string {
	if kind == "template" {
		return "模板"
	}
	return "插件"
}

func licenseActionLabel(action string) string {
	switch action {
	case "grant_edition":
		return "开通商业版"
	case "revoke_edition":
		return "撤销商业版"
	case "gift_plugin":
		return "赠送插件"
	case "revoke_plugin":
		return "撤销插件"
	case "transfer":
		return "转移授权"
	default:
		return "其他"
	}
}

func licenseSourceLabel(source string) string {
	switch source {
	case "admin":
		return "管理员开通"
	case "agent":
		return "代理开通"
	case "user_purchase":
		return "自助购买"
	case "card":
		return "卡密兑换"
	case "store_bind":
		return "商店绑定"
	case "store_purchase":
		return "商店购买"
	default:
		return "其他"
	}
}

// loadCommercialEditionSource 给客户站胶囊用，不放进签名快照，避免旧客户端验签失败。
func loadCommercialEditionSource(db *sql.DB, licenseID int64) string {
	var orderID, grantedBy sql.NullInt64
	var status string
	var expires sql.NullTime
	err := db.QueryRow(`SELECT status, expires_at, order_id, granted_by FROM main_license_editions
		WHERE license_id = ? AND edition = 'commercial' ORDER BY id DESC LIMIT 1`, licenseID).Scan(&status, &expires, &orderID, &grantedBy)
	if err != nil || status != "active" {
		return ""
	}
	if expires.Valid && !expires.Time.After(time.Now()) {
		return ""
	}
	return editionSourceLabel(orderID, grantedBy)
}
