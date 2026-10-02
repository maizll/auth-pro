// 商业版套餐的读取和出售缺项检查：没套餐、没支付或签名密钥不可用。每个应用各自判断。

package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

const storeEditionPlansMigration = "store_edition_plans_to_license_plans_v1"

type commercialSaleGap struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Path  string `json:"path,omitempty"`
}

var commercialProductColumnOK bool

func ensureCommercialProductColumn(db *sql.DB) error {
	if commercialProductColumnOK {
		return nil
	}
	var exists int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apps'
		  AND COLUMN_NAME = 'commercial_product'
	`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		_, err := db.Exec(`
			ALTER TABLE apps
			ADD COLUMN commercial_product TINYINT(1) NOT NULL DEFAULT 0
			COMMENT '是否作为本站商业版出售，全站最多一个' AFTER enabled
		`)
		if err != nil {
			var mysqlErr *mysql.MySQLError
			if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
				return err
			}
		}
	}
	commercialProductColumnOK = true
	return nil
}

// prepareCommercialProduct 补 apps.commercial_product 列，并把旧的商业版价格迁进套餐。只在启动时执行。
// 这一列只用来兼容回滚：新程序按 app_commercial_settings 判断，列里只同步「接收老客户端」的那个应用。
func prepareCommercialProduct(db *sql.DB) error {
	if hotPathSchemaSkipped() {
		return nil
	}
	if err := ensureCommercialProductColumn(db); err != nil {
		return err
	}
	return migrateStoreEditionPlans(db)
}

func lookupCommercialProduct(db *sql.DB) (id int64, appKey string, enabled int, err error) {
	err = db.QueryRow(`SELECT id, app_key, enabled FROM apps WHERE commercial_product = 1 ORDER BY id ASC LIMIT 1`).
		Scan(&id, &appKey, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", 0, nil
	}
	if err != nil {
		return 0, "", 0, err
	}
	appKey = strings.TrimSpace(appKey)
	return id, appKey, enabled, nil
}

func migrateStoreEditionPlans(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name VARCHAR(100) NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	pending, err := sourceStationMigrationPending(db, storeEditionPlansMigration)
	if err != nil {
		return err
	}
	if !pending {
		return nil
	}
	var tableCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'store_edition_plans'
	`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount == 0 {
		return nil
	}
	appID, _, _, err := lookupCommercialProduct(db)
	if err != nil {
		return err
	}
	if appID == 0 {
		return nil
	}
	rows, err := db.Query(`SELECT name, period, price_cents, enabled, sort FROM store_edition_plans ORDER BY sort, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, period string
		var priceCents int64
		var enabled, sort int
		if err := rows.Scan(&name, &period, &priceCents, &enabled, &sort); err != nil {
			return err
		}
		days := 365
		if period == storePeriodPermanent {
			days = 0
		}
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM license_plans WHERE app_id = ? AND name = ? AND duration_days = ? AND remark = ?`,
			appID, name, days, "由商业版价格迁移").Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		price := float64(priceCents) / 100
		if _, err := db.Exec(`INSERT INTO license_plans
			(app_id, name, license_type, duration_days, price, max_sites, sort, enabled, remark)
			VALUES (?, ?, '', ?, ?, 0, ?, ?, '由商业版价格迁移')`,
			appID, name, days, price, sort, enabled); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return markSourceStationMigration(db, storeEditionPlansMigration)
}

func salePeriodFromDuration(days int) string {
	switch {
	case days <= 0:
		return storePeriodPermanent
	case days == 365:
		return storePeriodYearly
	default:
		return fmt.Sprintf("d%d", days)
	}
}

func yuanTextToCents(raw string) (int64, error) {
	yuan, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(yuan * 100)), nil
}

// listCommercialSalePlans 列出一个应用正在出售的商业版套餐：已启用、价格大于 0。
func listCommercialSalePlans(db *sql.DB, appID int64) ([]map[string]any, error) {
	if appID <= 0 {
		return []map[string]any{}, nil
	}
	rows, err := db.Query(`SELECT id, name, duration_days, CAST(price AS CHAR),
		COALESCE(free_site_changes, -1), CAST(site_change_price AS CHAR)
		FROM license_plans
		WHERE app_id = ? AND enabled = 1 AND price > 0
		ORDER BY sort, id`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var name, priceText string
		var days, freeChanges int
		var changePrice sql.NullString
		if err := rows.Scan(&id, &name, &days, &priceText, &freeChanges, &changePrice); err != nil {
			continue
		}
		cents, err := yuanTextToCents(priceText)
		if err != nil || cents <= 0 {
			continue
		}
		list = append(list, commercialSalePlanItem(id, name, days, cents, freeChanges, changePrice))
	}
	return list, rows.Err()
}

// commercialSalePlanItem 组装公开套餐。free_site_changes 为 -1 表示不限；site_change_price 为空表示不能付费更换。
func commercialSalePlanItem(id int64, name string, days int, cents int64, freeChanges int, changePrice sql.NullString) map[string]any {
	item := map[string]any{
		"id": id, "name": name, "period": salePeriodFromDuration(days), "priceCents": cents,
		"free_site_changes": freeChanges, "site_change_price": nil,
	}
	if changePrice.Valid {
		if price, err := strconv.ParseFloat(strings.TrimSpace(changePrice.String), 64); err == nil {
			item["site_change_price"] = price
		}
	}
	return item
}

// errCommercialPlanAppMismatch 表示套餐不属于这个站点绑定的应用，多半是老客户端缓存了别的应用的套餐。
var errCommercialPlanAppMismatch = errors.New("套餐已更新，请刷新后重新选择")

// loadCommercialSalePlan 读取下单用的套餐。套餐必须属于站点绑定的应用。
func loadCommercialSalePlan(db *sql.DB, appID, planID int64) (name, period string, priceCents int64, err error) {
	var days, enabled int
	var planApp int64
	var priceText string
	queryErr := db.QueryRow(`SELECT app_id, name, duration_days, CAST(price AS CHAR), enabled
		FROM license_plans WHERE id = ?`, planID).Scan(&planApp, &name, &days, &priceText, &enabled)
	if queryErr != nil || enabled != 1 {
		return "", "", 0, errors.New("套餐不存在或未启用")
	}
	if planApp != appID {
		return "", "", 0, errCommercialPlanAppMismatch
	}
	priceCents, err = yuanTextToCents(priceText)
	if err != nil || priceCents <= 0 {
		return "", "", 0, errors.New("套餐价格不正确")
	}
	return name, salePeriodFromDuration(days), priceCents, nil
}

// commercialSaleShared 是和具体应用无关的出售条件，列表里只算一次。
type commercialSaleShared struct {
	paymentMissing bool
	signingLabel   string
}

func loadCommercialSaleShared(db *sql.DB) commercialSaleShared {
	shared := commercialSaleShared{}
	if len(configuredOnlinePayOptions(db)) == 0 {
		shared.paymentMissing = true
	}
	if _, err := os.Stat(storeSnapshotPrivateKeyPath()); err != nil {
		shared.signingLabel = "签名密钥未生成"
	} else if _, err := loadStoreSnapshotPrivateKey(); err != nil {
		shared.signingLabel = "签名密钥不可用"
	}
	return shared
}

func loadPricedPlanCounts(db *sql.DB) map[int64]int {
	counts := map[int64]int{}
	rows, err := db.Query(`SELECT app_id, COUNT(*) FROM license_plans WHERE enabled = 1 AND price > 0 GROUP BY app_id`)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var appID int64
		var count int
		if err := rows.Scan(&appID, &count); err == nil {
			counts[appID] = count
		}
	}
	return counts
}

func commercialSaleGapsWith(enabled bool, priced int, shared commercialSaleShared) []commercialSaleGap {
	gaps := make([]commercialSaleGap, 0)
	if !enabled {
		gaps = append(gaps, commercialSaleGap{Code: "disabled", Label: "应用未启用"})
	}
	if priced == 0 {
		gaps = append(gaps, commercialSaleGap{Code: "plan", Label: "没有有价格的套餐", Path: "/license/plans"})
	}
	if shared.paymentMissing {
		gaps = append(gaps, commercialSaleGap{Code: "payment", Label: "未配支付", Path: "/system/epay-config"})
	}
	if shared.signingLabel != "" {
		gaps = append(gaps, commercialSaleGap{Code: "signing_key", Label: shared.signingLabel})
	}
	return gaps
}
