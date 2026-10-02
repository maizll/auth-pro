// 每个应用各自出售商业版：出售状态、接收老客户端、宽限天数等设置，以及停售和作废。
// 迁移 per_app_commercial_v1 只执行 SQL，不调用任何会再触发迁移的业务函数。

package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

const (
	appCommercialModeOff     = "off"
	appCommercialModeSelling = "selling"
	appCommercialModeStopped = "stopped"

	perAppCommercialMigration  = "per_app_commercial_v1"
	appCommercialBackfillBatch = 5000
	// 回填循环的上限，防止数据异常时一直跑下去。5000 × 20000 行足够覆盖任何站点。
	appCommercialBackfillMaxRounds = 20000
)

var (
	errAppCommercialMissing       = errors.New("应用不存在")
	errAppCommercialNoLegacy      = errors.New("源站暂未开放该产品的商业版，请升级到最新版本后再试")
	errAppCommercialUnknown       = errors.New("源站没有这个产品，请确认程序来自官网")
	errAppCommercialLegacyRace    = errors.New("另一位管理员刚刚修改了默认应用，请刷新后重试")
	errAppCommercialCustomerSite  = errors.New("客户站不出售商业版，商业版只在官网管理")
	errAppCommercialNotForSale    = errors.New("该产品暂未开放商业版购买")
	errAppCommercialStoppedSale   = errors.New("该应用已停止出售商业版，已购买的不受影响")
	errAppCommercialSuperRequired = errors.New("只有超级管理员可以作废商业版权益")
)

// appCommercial 是一个应用的商业版设置。app_commercial_settings 没有这一行时等同普通应用。
type appCommercial struct {
	AppID                  int64
	AppKey                 string
	AppName                string
	Enabled                bool
	Mode                   string
	LegacyDefault          bool
	GraceDays              int
	RevokeOnPasswordChange bool
	Features               []string
}

// onSale 表示还在维护商业版：出售中或已停售。已停售仍可绑定、刷新和续期提醒，只是不能新下单。
func (a appCommercial) onSale() bool {
	return a.Mode == appCommercialModeSelling || a.Mode == appCommercialModeStopped
}

// saleProblem 返回不能新下单的原因。可以下单时返回 nil。
func (a appCommercial) saleProblem() error {
	switch {
	case a.Mode == appCommercialModeStopped:
		return errAppCommercialStoppedSale
	case a.Mode != appCommercialModeSelling || !a.Enabled:
		return errAppCommercialNotForSale
	default:
		return nil
	}
}

// storeSettings 把本应用的设置转成签发快照用的结构。
func (a appCommercial) storeSettings() sourceStoreSettings {
	revoke := a.RevokeOnPasswordChange
	grace := a.GraceDays
	if grace < 1 || grace > 30 {
		grace = storeGraceDefaultDays
	}
	features := a.Features
	if features == nil {
		features = []string{}
	}
	return sourceStoreSettings{GraceDays: grace, RevokeOnPasswordChange: &revoke, CommercialFeatures: features}
}

// ---------- 迁移（纯 SQL，可重复执行） ----------

func migratePerAppCommercial(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS app_commercial_settings (
		app_id BIGINT UNSIGNED NOT NULL,
		mode VARCHAR(16) NOT NULL DEFAULT 'off',
		legacy_default TINYINT(1) NOT NULL DEFAULT 0,
		legacy_flag TINYINT AS (IF(legacy_default = 1, 1, NULL)) STORED,
		grace_days INT NOT NULL DEFAULT 7,
		revoke_on_password_change TINYINT(1) NOT NULL DEFAULT 1,
		features VARCHAR(500) NOT NULL DEFAULT 'multi_app',
		stopped_at DATETIME DEFAULT NULL,
		stopped_by BIGINT UNSIGNED DEFAULT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		updated_by BIGINT UNSIGNED DEFAULT NULL,
		PRIMARY KEY (app_id),
		UNIQUE KEY uk_app_commercial_legacy (legacy_flag),
		KEY idx_app_commercial_mode (mode)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每个应用的商业版设置'`); err != nil {
		return fmt.Errorf("create app_commercial_settings: %w", err)
	}
	columns := []struct {
		table, column, addColumn, index, addIndex string
	}{
		{"store_bindings", "app_id", "ALTER TABLE store_bindings ADD COLUMN app_id BIGINT UNSIGNED DEFAULT NULL AFTER license_id",
			"idx_store_binding_app", "ALTER TABLE store_bindings ADD KEY idx_store_binding_app (app_id, status)"},
		{"store_bind_challenges", "app_id", "ALTER TABLE store_bind_challenges ADD COLUMN app_id BIGINT UNSIGNED DEFAULT NULL AFTER owner_id", "", ""},
		{"store_purchase_orders", "app_id", "ALTER TABLE store_purchase_orders ADD COLUMN app_id BIGINT UNSIGNED DEFAULT NULL AFTER license_id",
			"idx_store_purchase_app", "ALTER TABLE store_purchase_orders ADD KEY idx_store_purchase_app (app_id, status)"},
		{"main_license_editions", "app_id", "ALTER TABLE main_license_editions ADD COLUMN app_id BIGINT UNSIGNED DEFAULT NULL AFTER license_id",
			"idx_main_license_edition_app", "ALTER TABLE main_license_editions ADD KEY idx_main_license_edition_app (app_id, status)"},
	}
	for _, item := range columns {
		ok, err := catalogTableExists(db, item.table)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := ignoreDuplicateDDL(ensureSourceStationColumn(db, item.table, item.column, item.addColumn)); err != nil {
			return err
		}
		if item.index != "" {
			if err := ignoreDuplicateDDL(ensureSourceStationIndex(db, item.table, item.index, item.addIndex)); err != nil {
				return err
			}
		}
	}
	legacyID, err := perAppCommercialLegacyApp(db)
	if err != nil {
		return err
	}
	if legacyID > 0 {
		if err := insertLegacyCommercialSettings(db, legacyID); err != nil {
			return err
		}
	}
	if err := backfillCommercialAppIDs(db, legacyID); err != nil {
		return err
	}
	return syncLegacyCommercialFlag(db)
}

// ignoreDuplicateDDL 忽略两个进程同时补列、补索引时的「已存在」错误。
func ignoreDuplicateDDL(err error) error {
	if err == nil {
		return nil
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && (mysqlErr.Number == 1060 || mysqlErr.Number == 1061) {
		return nil
	}
	return err
}

// perAppCommercialLegacyApp 找出迁移前的那个商业版应用。顺序：已有默认应用、apps.commercial_product、旧的手填 app_key。
func perAppCommercialLegacyApp(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT app_id FROM app_commercial_settings WHERE legacy_default = 1 LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("read legacy commercial app: %w", err)
	}
	hasFlag, err := sourceStationColumnExists(db, "apps", "commercial_product")
	if err != nil {
		return 0, err
	}
	if hasFlag {
		err = db.QueryRow(`SELECT id FROM apps WHERE commercial_product = 1 ORDER BY id ASC LIMIT 1`).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("read commercial_product: %w", err)
		}
	}
	hasConfigs, err := catalogTableExists(db, "system_configs")
	if err != nil || !hasConfigs {
		return 0, err
	}
	err = db.QueryRow("SELECT a.id FROM apps a JOIN system_configs s ON s.`group` = 'store' AND s.`key` = 'store_product_app_key' " +
		"AND TRIM(s.value) <> '' AND a.app_key = TRIM(s.value) ORDER BY a.id ASC LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read store_product_app_key: %w", err)
	}
	return id, nil
}

// insertLegacyCommercialSettings 把原商业版应用写成「出售中、接收老客户端」，宽限天数等从旧的全站设置拷贝。
// 已经有这一行，或者别的应用已经是默认应用时 INSERT IGNORE 不做任何事。
func insertLegacyCommercialSettings(db *sql.DB, appID int64) error {
	hasConfigs, err := catalogTableExists(db, "system_configs")
	if err != nil {
		return err
	}
	if !hasConfigs {
		_, err := db.Exec(`INSERT IGNORE INTO app_commercial_settings (app_id, mode, legacy_default) VALUES (?, 'selling', 1)`, appID)
		return err
	}
	_, err = db.Exec("INSERT IGNORE INTO app_commercial_settings (app_id, mode, legacy_default, grace_days, revoke_on_password_change, features) "+
		"SELECT ?, 'selling', 1, "+
		"LEAST(30, GREATEST(1, COALESCE((SELECT CAST(value AS SIGNED) FROM system_configs WHERE `group` = 'store' AND `key` = 'store_grace_days' AND TRIM(value) REGEXP '^[0-9]+$' LIMIT 1), 7))), "+
		"COALESCE((SELECT IF(TRIM(value) = '0', 0, 1) FROM system_configs WHERE `group` = 'store' AND `key` = 'store_revoke_on_password_change' LIMIT 1), 1), "+
		"COALESCE((SELECT LEFT(TRIM(value), 500) FROM system_configs WHERE `group` = 'store' AND `key` = 'store_commercial_features' LIMIT 1), 'multi_app')",
		appID)
	if err != nil {
		return fmt.Errorf("insert legacy commercial settings: %w", err)
	}
	return nil
}

// backfillCommercialAppIDs 只处理 app_id 为空的行，每批 5000 行，避免长时间锁表。
func backfillCommercialAppIDs(db *sql.DB, legacyID int64) error {
	steps := []struct {
		table string
		query string
	}{
		{"store_bindings", `UPDATE store_bindings SET app_id = (SELECT l.app_id FROM licenses l WHERE l.id = store_bindings.license_id)
			WHERE app_id IS NULL AND EXISTS (SELECT 1 FROM licenses l WHERE l.id = store_bindings.license_id AND l.app_id IS NOT NULL) LIMIT 5000`},
		{"main_license_editions", `UPDATE main_license_editions SET app_id = (SELECT l.app_id FROM licenses l WHERE l.id = main_license_editions.license_id)
			WHERE app_id IS NULL AND EXISTS (SELECT 1 FROM licenses l WHERE l.id = main_license_editions.license_id AND l.app_id IS NOT NULL) LIMIT 5000`},
		{"store_purchase_orders", `UPDATE store_purchase_orders SET app_id = (SELECT l.app_id FROM licenses l WHERE l.id = store_purchase_orders.license_id)
			WHERE app_id IS NULL AND EXISTS (SELECT 1 FROM licenses l WHERE l.id = store_purchase_orders.license_id AND l.app_id IS NOT NULL) LIMIT 5000`},
		// 授权已经删掉的商业版订单，按套餐所属应用回填。item_id 就是套餐 id。
		{"store_purchase_orders", `UPDATE store_purchase_orders SET app_id = (SELECT p.app_id FROM license_plans p WHERE p.id = CAST(store_purchase_orders.item_id AS UNSIGNED))
			WHERE app_id IS NULL AND item_kind = 'edition' AND item_id REGEXP '^[0-9]+$'
			  AND EXISTS (SELECT 1 FROM license_plans p WHERE p.id = CAST(store_purchase_orders.item_id AS UNSIGNED) AND p.app_id IS NOT NULL) LIMIT 5000`},
	}
	for _, step := range steps {
		ok, err := catalogTableExists(db, step.table)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if _, err := runCommercialBackfill(db, step.query); err != nil {
			return fmt.Errorf("backfill %s.app_id: %w", step.table, err)
		}
	}
	if legacyID <= 0 {
		return nil
	}
	for _, table := range []string{"store_bindings", "main_license_editions", "store_purchase_orders"} {
		ok, err := catalogTableExists(db, table)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		n, err := runCommercialBackfill(db, "UPDATE "+table+" SET app_id = ? WHERE app_id IS NULL LIMIT 5000", legacyID)
		if err != nil {
			return fmt.Errorf("backfill %s.app_id to legacy app: %w", table, err)
		}
		if n > 0 {
			log.Printf("per_app_commercial_v1: %s 有 %d 行找不到所属应用，已归到原商业版应用 %d", table, n, legacyID)
		}
	}
	return nil
}

func runCommercialBackfill(db *sql.DB, query string, args ...any) (int64, error) {
	var total int64
	for round := 0; round < appCommercialBackfillMaxRounds; round++ {
		res, err := db.Exec(query, args...)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < appCommercialBackfillBatch {
			return total, nil
		}
	}
	return total, errors.New("回填行数超过上限")
}

// syncLegacyCommercialFlag 让 apps.commercial_product 只标记默认应用。回滚到 1.8.3 时老程序仍认同一个应用。
func syncLegacyCommercialFlag(db *sql.DB) error {
	hasFlag, err := sourceStationColumnExists(db, "apps", "commercial_product")
	if err != nil || !hasFlag {
		return err
	}
	_, err = db.Exec(`UPDATE apps a LEFT JOIN app_commercial_settings s ON s.app_id = a.id
		SET a.commercial_product = IF(COALESCE(s.legacy_default, 0) = 1, 1, 0)`)
	return err
}

// ---------- 读取 ----------

const appCommercialSelect = `SELECT a.id, a.app_key, a.app_name, a.enabled,
	COALESCE(s.mode, 'off'), COALESCE(s.legacy_default, 0), COALESCE(s.grace_days, 7),
	COALESCE(s.revoke_on_password_change, 1), COALESCE(s.features, 'multi_app')
	FROM apps a LEFT JOIN app_commercial_settings s ON s.app_id = a.id`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAppCommercial(row rowScanner) (appCommercial, error) {
	var item appCommercial
	var enabled, legacy, revoke int
	var features string
	if err := row.Scan(&item.AppID, &item.AppKey, &item.AppName, &enabled, &item.Mode, &legacy, &item.GraceDays, &revoke, &features); err != nil {
		return appCommercial{}, err
	}
	item.AppKey = strings.TrimSpace(item.AppKey)
	item.Enabled = enabled == 1
	item.LegacyDefault = legacy == 1
	item.RevokeOnPasswordChange = revoke == 1
	item.Features = splitStoreFeatures(features)
	return item, nil
}

func loadAppCommercial(db *sql.DB, appID int64) (appCommercial, error) {
	if db == nil || appID <= 0 {
		return appCommercial{}, errAppCommercialMissing
	}
	item, err := scanAppCommercial(db.QueryRow(appCommercialSelect+` WHERE a.id = ?`, appID))
	if errors.Is(err, sql.ErrNoRows) {
		return appCommercial{}, errAppCommercialMissing
	}
	return item, err
}

func loadAppCommercialForLicense(db *sql.DB, licenseID int64) (appCommercial, error) {
	var appID int64
	if err := db.QueryRow(`SELECT app_id FROM licenses WHERE id = ?`, licenseID).Scan(&appID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appCommercial{}, errAppCommercialMissing
		}
		return appCommercial{}, err
	}
	return loadAppCommercial(db, appID)
}

func loadLegacyCommercialApp(db *sql.DB) (appCommercial, bool, error) {
	item, err := scanAppCommercial(db.QueryRow(appCommercialSelect + ` WHERE s.legacy_default = 1 LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return appCommercial{}, false, nil
	}
	if err != nil {
		return appCommercial{}, false, err
	}
	return item, true, nil
}

// resolveStoreProductApp 按客户端声明的应用标识找应用。没声明（老客户端）时用接收老客户端的默认应用。
func resolveStoreProductApp(db *sql.DB, productKey string) (appCommercial, error) {
	productKey = strings.TrimSpace(productKey)
	if productKey == "" {
		item, ok, err := loadLegacyCommercialApp(db)
		if err != nil {
			return appCommercial{}, err
		}
		if !ok {
			return appCommercial{}, errAppCommercialNoLegacy
		}
		return item, nil
	}
	item, err := scanAppCommercial(db.QueryRow(appCommercialSelect+` WHERE a.app_key = ?`, productKey))
	if errors.Is(err, sql.ErrNoRows) {
		return appCommercial{}, errAppCommercialUnknown
	}
	return item, err
}

func listAppCommercials(db *sql.DB) (map[int64]appCommercial, error) {
	rows, err := db.Query(appCommercialSelect)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]appCommercial{}
	for rows.Next() {
		item, err := scanAppCommercial(rows)
		if err != nil {
			return nil, err
		}
		out[item.AppID] = item
	}
	return out, rows.Err()
}

// appCommercialStats 是关闭确认和编辑弹框里的三个数字。
type appCommercialStats struct {
	ActiveLicenses int `json:"activeLicenses"`
	BoundSites     int `json:"boundSites"`
	PendingOrders  int `json:"pendingOrders"`
}

func loadAppCommercialStats(db *sql.DB, appID int64) (appCommercialStats, error) {
	var stats appCommercialStats
	if err := db.QueryRow(`SELECT COUNT(DISTINCT e.license_id) FROM main_license_editions e
		JOIN licenses l ON l.id = e.license_id
		WHERE l.app_id = ? AND e.edition = 'commercial' AND e.status = 'active'
		  AND (e.expires_at IS NULL OR e.expires_at > NOW())`, appID).Scan(&stats.ActiveLicenses); err != nil {
		return stats, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM store_bindings WHERE app_id = ? AND status = 'active'`, appID).Scan(&stats.BoundSites); err != nil {
		return stats, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM store_purchase_orders
		WHERE app_id = ? AND item_kind = 'edition' AND status = 'pending' AND (expires_at IS NULL OR expires_at > NOW())`, appID).Scan(&stats.PendingOrders); err != nil {
		return stats, err
	}
	return stats, nil
}

// appCommercialPlan 是编辑弹框里只读的本应用商业版套餐。
type appCommercialPlan struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	DurationDays int    `json:"durationDays"`
	Price        string `json:"price"`
	Enabled      bool   `json:"enabled"`
}

func loadAppCommercialPlans(db *sql.DB) (map[int64][]appCommercialPlan, error) {
	rows, err := db.Query(`SELECT id, app_id, name, duration_days, CAST(price AS CHAR), enabled
		FROM license_plans WHERE price > 0 ORDER BY app_id, sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]appCommercialPlan{}
	for rows.Next() {
		var plan appCommercialPlan
		var appID int64
		var enabled int
		if err := rows.Scan(&plan.ID, &appID, &plan.Name, &plan.DurationDays, &plan.Price, &enabled); err != nil {
			return nil, err
		}
		plan.Enabled = enabled == 1
		out[appID] = append(out[appID], plan)
	}
	return out, rows.Err()
}

// ---------- 保存 ----------

// appCommercialInput 是应用弹框里商业版小节提交的字段。字段为空表示不改。
type appCommercialInput struct {
	Mode                   *string   `json:"mode"`
	LegacyDefault          *bool     `json:"legacyDefault"`
	GraceDays              *int      `json:"graceDays"`
	RevokeOnPasswordChange *bool     `json:"revokeOnPasswordChange"`
	Features               *[]string `json:"features"`
}

func (in *appCommercialInput) empty() bool {
	return in == nil || (in.Mode == nil && in.LegacyDefault == nil && in.GraceDays == nil && in.RevokeOnPasswordChange == nil && in.Features == nil)
}

// validate 只检查格式，不读库。宽限 1–30 天，功能键小写字母开头。
func (in *appCommercialInput) validate() error {
	if in.empty() {
		return nil
	}
	if in.Mode != nil {
		mode := strings.TrimSpace(*in.Mode)
		if mode != appCommercialModeSelling && mode != appCommercialModeOff {
			return errors.New("商业版状态只能是出售或普通应用；停售请用「关闭出售」")
		}
	}
	days := storeGraceDefaultDays
	if in.GraceDays != nil {
		days = *in.GraceDays
		if days < 1 || days > 30 {
			return errors.New("离线宽限天数须在 1 到 30 之间")
		}
	}
	var features []string
	if in.Features != nil {
		features = *in.Features
	}
	_, err := normalizeStoreSettings(sourceStoreSettings{GraceDays: days, CommercialFeatures: features})
	return err
}

type appCommercialSaveResult struct {
	Mode          string `json:"mode"`
	LegacyDefault bool   `json:"legacyDefault"`
	Notice        string `json:"notice,omitempty"`
}

// saveAppCommercial 保存一个应用的商业版设置。关闭有效授权的应用要走 closeAppCommercial。
func saveAppCommercial(db *sql.DB, appID int64, in *appCommercialInput, actor int64) (appCommercialSaveResult, error) {
	current, err := loadAppCommercial(db, appID)
	if err != nil {
		return appCommercialSaveResult{}, err
	}
	result := appCommercialSaveResult{Mode: current.Mode, LegacyDefault: current.LegacyDefault}
	if in.empty() {
		return result, nil
	}
	if err := in.validate(); err != nil {
		return result, err
	}
	mode := current.Mode
	if in.Mode != nil {
		mode = strings.TrimSpace(*in.Mode)
	}
	if mode == appCommercialModeOff && current.Mode != appCommercialModeOff {
		stats, err := loadAppCommercialStats(db, appID)
		if err != nil {
			return result, err
		}
		if stats.ActiveLicenses > 0 {
			return result, fmt.Errorf("还有 %d 个有效商业版授权，不能转为普通应用。请先用「关闭出售」选择停售方式", stats.ActiveLicenses)
		}
		if stats.PendingOrders > 0 {
			return result, fmt.Errorf("还有 %d 笔待支付的商业版订单，请等订单完成或过期后再关闭", stats.PendingOrders)
		}
	}
	// 停售的应用保存其他字段时保持停售，只有明确打开出售才恢复。
	if mode == current.Mode && current.Mode == appCommercialModeStopped && (in.Mode == nil || *in.Mode != appCommercialModeSelling) {
		mode = appCommercialModeStopped
	}
	grace := current.GraceDays
	if in.GraceDays != nil {
		grace = *in.GraceDays
	}
	revoke := current.RevokeOnPasswordChange
	if in.RevokeOnPasswordChange != nil {
		revoke = *in.RevokeOnPasswordChange
	}
	features := current.Features
	if in.Features != nil {
		normalized, err := normalizeStoreSettings(sourceStoreSettings{GraceDays: grace, CommercialFeatures: *in.Features})
		if err != nil {
			return result, err
		}
		features = normalized.CommercialFeatures
	}
	legacy := current.LegacyDefault
	if in.LegacyDefault != nil {
		legacy = *in.LegacyDefault
	}
	if mode == appCommercialModeOff {
		legacy = false
	}
	// 全站第一个出售的应用，又还没有默认应用时，自动接收老客户端。
	if mode == appCommercialModeSelling && current.Mode != appCommercialModeSelling && in.LegacyDefault == nil && !legacy {
		if _, ok, err := loadLegacyCommercialApp(db); err != nil {
			return result, err
		} else if !ok {
			legacy = true
			result.Notice = "这是第一个出售商业版的应用，已自动设为接收老客户端"
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if legacy && !current.LegacyDefault {
		if _, err := tx.Exec(`UPDATE app_commercial_settings SET legacy_default = 0, updated_by = ? WHERE legacy_default = 1 AND app_id <> ?`, nullableActor(actor), appID); err != nil {
			return result, err
		}
	}
	var stoppedSQL string
	switch {
	case mode == appCommercialModeSelling:
		stoppedSQL = "stopped_at = NULL, stopped_by = NULL"
	case mode == appCommercialModeOff:
		stoppedSQL = "stopped_at = NULL, stopped_by = NULL"
	default:
		stoppedSQL = "stopped_at = stopped_at"
	}
	_, err = tx.Exec(`INSERT INTO app_commercial_settings
		(app_id, mode, legacy_default, grace_days, revoke_on_password_change, features, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE mode = VALUES(mode), legacy_default = VALUES(legacy_default), grace_days = VALUES(grace_days),
		revoke_on_password_change = VALUES(revoke_on_password_change), features = VALUES(features), updated_by = VALUES(updated_by), `+stoppedSQL,
		appID, mode, boolInt(legacy), grace, boolInt(revoke), strings.Join(features, ","), nullableActor(actor))
	if err != nil {
		if isDuplicateKey(err) {
			return result, errAppCommercialLegacyRace
		}
		return result, err
	}
	if err := syncLegacyCommercialFlagTx(tx); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		if isDuplicateKey(err) {
			return result, errAppCommercialLegacyRace
		}
		return result, err
	}
	result.Mode = mode
	result.LegacyDefault = legacy
	return result, nil
}

func syncLegacyCommercialFlagTx(tx *sql.Tx) error {
	_, err := tx.Exec(`UPDATE apps a LEFT JOIN app_commercial_settings s ON s.app_id = a.id
		SET a.commercial_product = IF(COALESCE(s.legacy_default, 0) = 1, 1, 0)`)
	return err
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableActor(id int64) any {
	if id > 0 {
		return id
	}
	return nil
}

// ---------- 关闭出售：停售或作废 ----------

type appCommercialCloseRequest struct {
	Action      string `json:"action"`
	ConfirmName string `json:"confirmName"`
	// LegacyTo 只在关闭默认应用时有用：不传表示不改，0 表示不再接收老客户端，其他值表示改到这个应用。
	LegacyTo *int64 `json:"legacyTo"`
}

type appCommercialCloseResult struct {
	Mode    string `json:"mode"`
	Revoked int64  `json:"revoked"`
	Msg     string `json:"msg"`
}

// closeAppCommercial 停止出售。stop 只停新售，已售权益照常；revoke 同时把本应用的商业版权益改为 revoked，绑定保留。
func closeAppCommercial(db *sql.DB, appID int64, req appCommercialCloseRequest, actor int64, super bool) (appCommercialCloseResult, error) {
	current, err := loadAppCommercial(db, appID)
	if err != nil {
		return appCommercialCloseResult{}, err
	}
	if current.Mode == appCommercialModeOff {
		return appCommercialCloseResult{}, errors.New("该应用没有出售商业版")
	}
	action := strings.TrimSpace(req.Action)
	if action != "stop" && action != "revoke" {
		return appCommercialCloseResult{}, errors.New("请选择关闭方式")
	}
	if action == "revoke" {
		if !super {
			return appCommercialCloseResult{}, errAppCommercialSuperRequired
		}
		if strings.TrimSpace(req.ConfirmName) != strings.TrimSpace(current.AppName) {
			return appCommercialCloseResult{}, errors.New("输入的应用名称不一致")
		}
	}
	var legacyTarget appCommercial
	changeLegacy := current.LegacyDefault && req.LegacyTo != nil && *req.LegacyTo != appID
	if changeLegacy && *req.LegacyTo > 0 {
		legacyTarget, err = loadAppCommercial(db, *req.LegacyTo)
		if err != nil {
			return appCommercialCloseResult{}, errors.New("要接收老客户端的应用不存在")
		}
		if legacyTarget.Mode != appCommercialModeSelling {
			return appCommercialCloseResult{}, errors.New("只能把老客户端改到正在出售商业版的应用")
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return appCommercialCloseResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE app_commercial_settings SET mode = 'stopped', stopped_at = NOW(), stopped_by = ?, updated_by = ? WHERE app_id = ?`,
		nullableActor(actor), nullableActor(actor), appID); err != nil {
		return appCommercialCloseResult{}, err
	}
	if changeLegacy {
		if _, err := tx.Exec(`UPDATE app_commercial_settings SET legacy_default = 0 WHERE app_id = ?`, appID); err != nil {
			return appCommercialCloseResult{}, err
		}
		if legacyTarget.AppID > 0 {
			if _, err := tx.Exec(`UPDATE app_commercial_settings SET legacy_default = 1 WHERE app_id = ?`, legacyTarget.AppID); err != nil {
				if isDuplicateKey(err) {
					return appCommercialCloseResult{}, errAppCommercialLegacyRace
				}
				return appCommercialCloseResult{}, err
			}
		}
		if err := syncLegacyCommercialFlagTx(tx); err != nil {
			return appCommercialCloseResult{}, err
		}
	}
	var revoked int64
	if action == "revoke" {
		res, err := tx.Exec(`UPDATE main_license_editions e JOIN licenses l ON l.id = e.license_id
			SET e.status = 'revoked', e.updated_at = NOW()
			WHERE l.app_id = ? AND e.edition = 'commercial' AND e.status = 'active'`, appID)
		if err != nil {
			return appCommercialCloseResult{}, err
		}
		revoked, _ = res.RowsAffected()
	}
	if err := tx.Commit(); err != nil {
		return appCommercialCloseResult{}, err
	}
	out := appCommercialCloseResult{Mode: appCommercialModeStopped, Revoked: revoked, Msg: "已停止新售，已售出的商业版照常使用"}
	if action == "revoke" {
		out.Msg = fmt.Sprintf("已停止新售并作废 %d 条商业版权益，客户站下次刷新回到免费版。已付款订单请另行退款", revoked)
	}
	return out, nil
}

// ---------- 接口 ----------

// AppCommercialContext 告诉后台页面：本站是不是官网、当前管理员是不是超级管理员。客户站隐藏商业版设置。
func AppCommercialContext(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"managed": officialSite(),
		"super":   c.GetString("role_code") == "R_SUPER",
	}})
}

// AppCommercialCloseHandler 关闭一个应用的商业版出售。作废只允许超级管理员，并且要输入应用名确认。
func AppCommercialCloseHandler(c *gin.Context) {
	if !officialSite() {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errAppCommercialCustomerSite.Error()})
		return
	}
	appID, err := positiveInt64(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "应用ID不正确"})
		return
	}
	var req appCommercialCloseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	result, err := closeAppCommercial(db, appID, req, currentAdminID(c), c.GetString("role_code") == "R_SUPER")
	if err != nil {
		code := 400
		if errors.Is(err, errAppCommercialSuperRequired) {
			code = 403
		} else if errors.Is(err, errAppCommercialMissing) {
			code = 404
		}
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": err.Error()})
		return
	}
	detail := "停止新售商业版"
	if strings.TrimSpace(req.Action) == "revoke" {
		detail = fmt.Sprintf("停止新售并作废 %d 条商业版权益", result.Revoked)
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: c.GetString("username"), Action: "close_app_commercial",
		TargetType: "app", TargetID: fmt.Sprint(appID), Detail: detail,
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": result.Msg, "data": result})
}

// appCommercialRejectArchive 出售中的应用不能归档，要先停售。
func appCommercialRejectArchive(db *sql.DB, appID int64) error {
	item, err := loadAppCommercial(db, appID)
	if err != nil {
		return nil
	}
	if item.Mode == appCommercialModeSelling {
		return errors.New("请先停止出售商业版")
	}
	return nil
}

// appCommercialView 是应用列表里给编辑弹框用的商业版信息。只在官网返回。
type appCommercialView struct {
	Mode                   string              `json:"mode"`
	LegacyDefault          bool                `json:"legacyDefault"`
	GraceDays              int                 `json:"graceDays"`
	RevokeOnPasswordChange bool                `json:"revokeOnPasswordChange"`
	Features               []string            `json:"features"`
	SaleGaps               []commercialSaleGap `json:"saleGaps"`
	Stats                  appCommercialStats  `json:"stats"`
	Plans                  []appCommercialPlan `json:"plans"`
}

func buildAppCommercialView(db *sql.DB, item appCommercial, enabled bool, priced int, shared commercialSaleShared, plans []appCommercialPlan) *appCommercialView {
	view := &appCommercialView{
		Mode: item.Mode, LegacyDefault: item.LegacyDefault, GraceDays: item.GraceDays,
		RevokeOnPasswordChange: item.RevokeOnPasswordChange, Features: item.Features,
		SaleGaps: []commercialSaleGap{}, Plans: plans,
	}
	if item.AppID == 0 {
		view.Mode = appCommercialModeOff
		view.GraceDays = storeGraceDefaultDays
		view.RevokeOnPasswordChange = true
		view.Features = []string{storeFeatureMultiApp}
	}
	if view.Features == nil {
		view.Features = []string{}
	}
	if view.Plans == nil {
		view.Plans = []appCommercialPlan{}
	}
	if view.Mode == appCommercialModeOff {
		return view
	}
	view.SaleGaps = commercialSaleGapsWith(enabled, priced, shared)
	if stats, err := loadAppCommercialStats(db, item.AppID); err == nil {
		view.Stats = stats
	}
	return view
}

// checkAppCommercialInput 检查应用弹框提交的商业版字段。客户站不接受这些字段。
func checkAppCommercialInput(in *appCommercialInput) error {
	if in.empty() {
		return nil
	}
	if !officialSite() {
		return errAppCommercialCustomerSite
	}
	return in.validate()
}

// storeBindingProduct 取已绑定站点所属的应用，只看绑定记录，不看任何后台开关。
// 新客户端在 X-Store-Product 里声明自己属于哪个应用；和绑定的应用对不上时返回非终止错误，不删除客户站凭据。
func storeBindingProduct(c *gin.Context, db *sql.DB, row storeBindingRecord) (appCommercial, bool) {
	product, err := loadAppCommercial(db, row.AppID)
	if err != nil {
		storeFail(c, 500, "产品应用不存在")
		return appCommercial{}, false
	}
	declared := strings.TrimSpace(c.GetHeader(storeProductHeader))
	if declared != "" && declared != product.AppKey {
		other := declared
		if item, err := resolveStoreProductApp(db, declared); err == nil {
			other = item.AppName
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 409,
			"msg":  fmt.Sprintf("本站绑定的是「%s」，当前程序属于「%s」。请切换绑定", product.AppName, other),
			"data": gin.H{"reason": "app_mismatch"},
		})
		return appCommercial{}, false
	}
	return product, true
}
