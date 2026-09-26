package handler

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

const commercialOrdinaryPurchaseRejected = "该套餐是本站商业版，不能在授权购买页下单。请到管理后台顶栏打开「升级商业版」。"

type commercialEditionGrant struct {
	LicenseID         int64
	Period            string
	OrderID           int64
	GrantedBy         int64
	Extend            bool
	Stack             bool
	MarkStorePurchase bool
}

type commercialGapOrder struct {
	OrderNo   string `json:"orderNo"`
	LicenseID int64  `json:"licenseId"`
	LicenseNo string `json:"licenseNo"`
	AppName   string `json:"appName"`
	PlanName  string `json:"planName"`
	PaidAt    string `json:"paidAt"`
	Period    string `json:"-"`
}

func appIsCommercialProduct(db *sql.DB, appID int64) (bool, error) {
	if appID <= 0 {
		return false, nil
	}
	if err := ensureCommercialProductColumn(db); err != nil {
		return false, err
	}
	var flag int
	err := db.QueryRow(`SELECT commercial_product FROM apps WHERE id = ?`, appID).Scan(&flag)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return flag == 1, nil
}

func rejectCommercialOrdinaryPurchase(c *gin.Context, db *sql.DB, appID int64) bool {
	on, err := appIsCommercialProduct(db, appID)
	if err != nil {
		c.JSON(200, gin.H{"code": 500, "msg": "读取商业版产品失败"})
		return true
	}
	if on {
		c.JSON(200, gin.H{"code": 400, "msg": commercialOrdinaryPurchaseRejected})
		return true
	}
	return false
}

// grantCommercialEditionTx 把一张授权开通为商业版。Extend 为 false 时，已有未过期的商业版直接返回，不新增行。
func grantCommercialEditionTx(tx *sql.Tx, grant commercialEditionGrant) (bool, error) {
	if grant.LicenseID <= 0 {
		return false, errors.New("授权不存在")
	}
	period := strings.TrimSpace(grant.Period)
	if period == "" {
		period = storePeriodPermanent
	}
	var expires sql.NullTime
	err := tx.QueryRow(`SELECT expires_at FROM main_license_editions
		WHERE license_id = ? AND edition = 'commercial' AND status = 'active'
		ORDER BY id DESC LIMIT 1 FOR UPDATE`, grant.LicenseID).Scan(&expires)
	active := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	unexpired := active && (!expires.Valid || expires.Time.After(time.Now()))
	if unexpired && !grant.Extend {
		return false, nil
	}
	if _, err := tx.Exec(`UPDATE main_license_editions SET status = 'replaced', updated_at = NOW()
		WHERE license_id = ? AND status = 'active'`, grant.LicenseID); err != nil {
		return false, err
	}
	var current *time.Time
	if grant.Stack && expires.Valid {
		t := expires.Time
		current = &t
	}
	expiry := nextEditionExpiry(period, current, time.Now())
	var exp any
	if expiry != nil {
		exp = *expiry
	}
	var orderID any
	if grant.OrderID > 0 {
		orderID = grant.OrderID
	}
	var grantedBy any
	if grant.GrantedBy > 0 {
		grantedBy = grant.GrantedBy
	}
	if _, err := tx.Exec(`INSERT INTO main_license_editions
		(license_id, edition, period, started_at, expires_at, status, order_id, granted_by)
		VALUES (?, 'commercial', ?, NOW(), ?, 'active', ?, ?)`,
		grant.LicenseID, period, exp, orderID, grantedBy); err != nil {
		return false, err
	}
	if grant.MarkStorePurchase {
		if _, err := tx.Exec(`UPDATE licenses SET source = 'store_purchase' WHERE id = ?`, grant.LicenseID); err != nil {
			return false, err
		}
	}
	return true, nil
}

func signedSnapshotForLicense(db *sql.DB, licenseID int64) (storeSnapshot, bool, error) {
	var bindingID, domain, licenseNo string
	err := db.QueryRow(`SELECT b.binding_id, b.domain_snapshot, l.license_no
		FROM store_bindings b
		JOIN licenses l ON l.id = b.license_id
		WHERE b.license_id = ? AND b.status = 'active'
		ORDER BY b.id DESC LIMIT 1`, licenseID).Scan(&bindingID, &domain, &licenseNo)
	if errors.Is(err, sql.ErrNoRows) {
		return storeSnapshot{}, false, nil
	}
	if err != nil {
		return storeSnapshot{}, false, err
	}
	settings, err := loadEffectiveStoreSettings(db)
	if err != nil {
		return storeSnapshot{}, false, err
	}
	snapshot, err := buildStoreSnapshot(db, bindingID, licenseID, licenseNo, domain, settings)
	if err != nil {
		return storeSnapshot{}, true, err
	}
	return snapshot, true, nil
}

func listCommercialPurchaseGaps(db *sql.DB) ([]commercialGapOrder, error) {
	if err := ensureCommercialProductColumn(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT o.order_no, o.license_id, COALESCE(o.license_no, ''),
		       COALESCE(NULLIF(o.app_name_snapshot, ''), a.app_name, ''),
		       COALESCE(NULLIF(o.plan_name_snapshot, ''), ''),
		       COALESCE(o.paid_at, o.created_at),
		       COALESCE(l.duration_days, 0)
		FROM license_purchase_orders o
		JOIN apps a ON a.id = o.app_id AND a.commercial_product = 1
		JOIN licenses l ON l.id = o.license_id
		WHERE o.status = 'paid' AND o.owner_type = 'user'
		  AND NOT EXISTS (
		    SELECT 1 FROM main_license_editions e
		    WHERE e.license_id = o.license_id AND e.edition = 'commercial' AND e.status = 'active'
		      AND (e.expires_at IS NULL OR e.expires_at > NOW())
		  )
		ORDER BY o.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]commercialGapOrder, 0)
	for rows.Next() {
		var item commercialGapOrder
		var paid time.Time
		var days int
		if err := rows.Scan(&item.OrderNo, &item.LicenseID, &item.LicenseNo, &item.AppName, &item.PlanName, &paid, &days); err != nil {
			return nil, err
		}
		item.PaidAt = paid.Format("2006-01-02 15:04")
		item.Period = salePeriodFromDuration(days)
		list = append(list, item)
	}
	return list, rows.Err()
}

func reissueCommercialPurchaseGaps(db *sql.DB) (granted, already int, err error) {
	gaps, err := listCommercialPurchaseGaps(db)
	if err != nil {
		return 0, 0, err
	}
	for _, gap := range gaps {
		tx, txErr := db.Begin()
		if txErr != nil {
			return granted, already, txErr
		}
		created, grantErr := grantCommercialEditionTx(tx, commercialEditionGrant{
			LicenseID: gap.LicenseID,
			Period:    gap.Period,
		})
		if grantErr != nil {
			_ = tx.Rollback()
			return granted, already, grantErr
		}
		if err := tx.Commit(); err != nil {
			return granted, already, err
		}
		if created {
			granted++
			_, _, _ = signedSnapshotForLicense(db, gap.LicenseID)
		} else {
			already++
		}
	}
	return granted, already, nil
}

func AdminCommercialPurchaseGaps(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	gaps, err := listCommercialPurchaseGaps(db)
	if err != nil {
		storeFail(c, 500, "读取未开通的商业版订单失败")
		return
	}
	storeData(c, gin.H{"count": len(gaps), "orders": gaps})
}

func AdminCommercialPurchaseReissue(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	granted, already, err := reissueCommercialPurchaseGaps(db)
	if err != nil {
		storeFail(c, 500, "补发商业版授权失败")
		return
	}
	msg := "已补发商业版授权。已经开通过的不会重复发放。"
	if granted == 0 && already == 0 {
		msg = "没有需要补发的商业版订单。"
	} else if granted == 0 {
		msg = "这些订单已经开通过商业版，没有重复发放。"
	}
	storeData(c, gin.H{"granted": granted, "already": already, "msg": msg})
}

func registerCommercialReissueRoutes(admin *gin.RouterGroup) {
	group := admin.Group("/store", middleware.RequireMenu(middleware.MenuLicenseList, middleware.MenuOrderList))
	group.GET("/commercial-gaps", AdminCommercialPurchaseGaps)
	group.POST("/commercial-reissue", AdminCommercialPurchaseReissue)
}
