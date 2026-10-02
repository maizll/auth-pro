// 把已支付但还没开通的商业版订单补到授权上，并签出快照。普通授权购买页不能下商业版套餐。

package handler

import (
	"database/sql"
	"errors"
	"strconv"
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
	AppID     int64  `json:"appId"`
	LicenseID int64  `json:"licenseId"`
	LicenseNo string `json:"licenseNo"`
	AppName   string `json:"appName"`
	PlanName  string `json:"planName"`
	PaidAt    string `json:"paidAt"`
	Period    string `json:"-"`
}

// appIsCommercialProduct 判断应用是不是在维护商业版（出售中或已停售）。
// 已停售的应用仍然算：它的套餐不能当普通授权卖，已付款的订单照常开通。
func appIsCommercialProduct(db *sql.DB, appID int64) (bool, error) {
	if appID <= 0 {
		return false, nil
	}
	var mode string
	err := db.QueryRow(`SELECT mode FROM app_commercial_settings WHERE app_id = ?`, appID).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return mode == appCommercialModeSelling || mode == appCommercialModeStopped, nil
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
		(license_id, app_id, edition, period, started_at, expires_at, status, order_id, granted_by)
		VALUES (?, (SELECT l.app_id FROM licenses l WHERE l.id = ?), 'commercial', ?, NOW(), ?, 'active', ?, ?)`,
		grant.LicenseID, grant.LicenseID, period, exp, orderID, grantedBy); err != nil {
		return false, err
	}
	if grant.MarkStorePurchase {
		if _, err := tx.Exec(`UPDATE licenses SET source = 'store_purchase' WHERE id = ?`, grant.LicenseID); err != nil {
			return false, err
		}
	}
	return true, nil
}

// mergeCommercialGrantTargets 把后台点中的授权和客户站当前绑定的授权合成一份。
// 购买时两者是同一条，只保留一次。后台如果开在另一条同域名授权上，绑定的那条也要开通，
// 否则客户站刷新读到的仍是免费版。
func mergeCommercialGrantTargets(requested int64, bound []int64) []int64 {
	seen := make(map[int64]struct{}, 1+len(bound))
	out := make([]int64, 0, 1+len(bound))
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(requested)
	for _, id := range bound {
		add(id)
	}
	return out
}

// openCommercialEdition 是购买开通和后台开通共用的入口，里面只调用 grantCommercialEditionTx。
// extra 是同域名上客户站正在使用的授权。和本次授权不是同一条时，不把订单号写过去，也不叠加时长。
// 返回值表示本次点名的那条授权是否新写了一行权益。已经开通过且没有要求续期时为 false。
func openCommercialEdition(tx *sql.Tx, grant commercialEditionGrant, extra []int64) (bool, error) {
	if grant.LicenseID <= 0 {
		return false, errors.New("授权不存在")
	}
	created := false
	for _, id := range mergeCommercialGrantTargets(grant.LicenseID, extra) {
		item := grant
		item.LicenseID = id
		if id != grant.LicenseID {
			item.OrderID = 0
			item.MarkStorePurchase = false
			item.Stack = false
			item.Extend = true
		}
		wrote, err := grantCommercialEditionTx(tx, item)
		if err != nil {
			return false, err
		}
		if id == grant.LicenseID {
			created = wrote
		}
	}
	return created, nil
}

type editionTargetQuery interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// boundLicensesForSameDomain 找出和这条授权同应用、同域名、仍在使用的站点绑定。
// 客户站顶栏看的是绑定上的授权，不是后台后来另建的那一条。
func boundLicensesForSameDomain(q editionTargetQuery, licenseID int64) ([]int64, error) {
	var appID int64
	var domain string
	err := q.QueryRow(`SELECT l.app_id, COALESCE((SELECT domain FROM license_domains WHERE license_id = l.id ORDER BY id LIMIT 1), '')
		FROM licenses l WHERE l.id = ?`, licenseID).Scan(&appID, &domain)
	if err != nil {
		return nil, err
	}
	domain = normalizeLicenseDomain(domain)
	if domain == "" || appID <= 0 {
		return nil, nil
	}
	rows, err := q.Query(`SELECT b.license_id, b.domain_snapshot
		FROM store_bindings b
		JOIN licenses l ON l.id = b.license_id
		WHERE b.status = 'active' AND l.app_id = ? AND l.status = 'active'`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		var snapshot string
		if err := rows.Scan(&id, &snapshot); err != nil {
			return nil, err
		}
		if normalizeLicenseDomain(snapshot) != domain {
			continue
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// storeStatusLicenseID 决定刷新快照时用哪条授权。
// 当前绑定仍覆盖这个域名时，继续用它，不要因为后台又加了一条同域名授权就要求重新绑定。
// 绑定已经对不上域名时，不能悄悄改挂到另一条授权上。
func storeStatusLicenseID(boundID, matchedID int64, boundCovers bool) (int64, string) {
	if boundCovers && boundID > 0 {
		return boundID, ""
	}
	if matchedID <= 0 || matchedID != boundID {
		return 0, "license_not_found"
	}
	return matchedID, ""
}

// boundLicenseCoversDomain 判断客户站绑定的那条授权是否仍对这个域名有效。
// 只看授权是否还在、是否过期、域名是否对得上，不看商业版权益。权益由快照另算。
func boundLicenseCoversDomain(db *sql.DB, licenseID, appID int64, domain string) bool {
	domain = normalizeLicenseDomain(domain)
	if db == nil || licenseID <= 0 || appID <= 0 || domain == "" {
		return false
	}
	var status, licType string
	var licApp int64
	var expired sql.NullTime
	err := db.QueryRow(`SELECT app_id, status, type, expired_at FROM licenses WHERE id = ?`, licenseID).
		Scan(&licApp, &status, &licType, &expired)
	if err != nil || licApp != appID || status != "active" {
		return false
	}
	if expired.Valid && !expired.Time.After(time.Now()) {
		return false
	}
	rows, err := db.Query(`SELECT domain, is_wildcard FROM license_domains WHERE license_id = ?`, licenseID)
	if err != nil {
		return false
	}
	defer rows.Close()
	req := licenseVerifyRequest{Domain: domain}
	for rows.Next() {
		var target string
		var wildcard int
		if err := rows.Scan(&target, &wildcard); err != nil {
			continue
		}
		if licenseRowMatchesRequest(licType, normalizeLicenseTarget(target), wildcard == 1, req) {
			return true
		}
	}
	return false
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
	product, err := loadAppCommercialForLicense(db, licenseID)
	if err != nil {
		return storeSnapshot{}, false, err
	}
	snapshot, err := buildStoreSnapshot(db, bindingID, licenseID, licenseNo, domain, product.storeSettings())
	if err != nil {
		return storeSnapshot{}, true, err
	}
	return snapshot, true, nil
}

// listCommercialPurchaseGaps 列出在授权购买页付了款、却还没开通商业版的订单。appID 为 0 时列出全部商业版应用。
// 已停售的应用也算，它们的历史订单同样要能补发。
func listCommercialPurchaseGaps(db *sql.DB, appID int64) ([]commercialGapOrder, error) {
	query := `
		SELECT o.order_no, o.app_id, o.license_id, COALESCE(o.license_no, ''),
		       COALESCE(NULLIF(o.app_name_snapshot, ''), a.app_name, ''),
		       COALESCE(NULLIF(o.plan_name_snapshot, ''), ''),
		       COALESCE(o.paid_at, o.created_at),
		       COALESCE(l.duration_days, 0)
		FROM license_purchase_orders o
		JOIN apps a ON a.id = o.app_id
		JOIN app_commercial_settings s ON s.app_id = o.app_id AND s.mode <> 'off'
		JOIN licenses l ON l.id = o.license_id
		WHERE o.status = 'paid' AND o.owner_type = 'user'
		  AND (? = 0 OR o.app_id = ?)
		  AND NOT EXISTS (
		    SELECT 1 FROM main_license_editions e
		    WHERE e.license_id = o.license_id AND e.edition = 'commercial' AND e.status = 'active'
		      AND (e.expires_at IS NULL OR e.expires_at > NOW())
		  )
		ORDER BY o.id ASC`
	rows, err := db.Query(query, appID, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]commercialGapOrder, 0)
	for rows.Next() {
		var item commercialGapOrder
		var paid time.Time
		var days int
		if err := rows.Scan(&item.OrderNo, &item.AppID, &item.LicenseID, &item.LicenseNo, &item.AppName, &item.PlanName, &paid, &days); err != nil {
			return nil, err
		}
		item.PaidAt = paid.Format("2006-01-02 15:04")
		item.Period = salePeriodFromDuration(days)
		list = append(list, item)
	}
	return list, rows.Err()
}

func reissueCommercialPurchaseGaps(db *sql.DB, appID int64) (granted, already int, err error) {
	gaps, err := listCommercialPurchaseGaps(db, appID)
	if err != nil {
		return 0, 0, err
	}
	for _, gap := range gaps {
		tx, txErr := db.Begin()
		if txErr != nil {
			return granted, already, txErr
		}
		created, grantErr := openCommercialEdition(tx, commercialEditionGrant{
			LicenseID: gap.LicenseID,
			Period:    gap.Period,
		}, nil)
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

// AdminCommercialPurchaseGaps 列出已支付但还没有商业版权益的订单。读库失败返回 500。
func AdminCommercialPurchaseGaps(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	appID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("appId")), 10, 64)
	gaps, err := listCommercialPurchaseGaps(db, appID)
	if err != nil {
		storeFail(c, 500, "读取未开通的商业版订单失败")
		return
	}
	storeData(c, gin.H{"count": len(gaps), "orders": gaps})
}

// AdminCommercialPurchaseReissue 给这些订单补发商业版。请求体 appId 指定只补发哪个应用，不传或为 0 时补发全部。
// 已经开通过的不重复发放。返回补发数量和原本就已经开通的数量。中途写库失败返回 500，已提交的不会回滚。
func AdminCommercialPurchaseReissue(c *gin.Context) {
	var req struct {
		AppID int64 `json:"appId"`
	}
	_ = c.ShouldBindJSON(&req)
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	granted, already, err := reissueCommercialPurchaseGaps(db, req.AppID)
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
