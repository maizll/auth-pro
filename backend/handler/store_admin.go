// 管理端仍保留的商店套餐、订单、主授权和收入接口。页面已收进授权和订单菜单，接口给升级后的数据用。

package handler

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

func registerStoreAdminRoutes(admin *gin.RouterGroup) {
	read := admin.Group("/store")
	edition := admin.Group("/store", middleware.RequireMenu(middleware.MenuLicensePlans))
	orders := admin.Group("/store", middleware.RequireMenu(middleware.MenuOrderList))
	licenses := admin.Group("/store", middleware.RequireMenu(middleware.MenuLicenseList))
	revenue := admin.Group("/store", middleware.RequireMenu(middleware.MenuOrderList))

	read.GET("/plans", AdminStorePlans)
	edition.POST("/plans", AdminStorePlanSave)
	edition.PUT("/plans/:id", AdminStorePlanSave)
	read.GET("/orders", AdminStoreOrders)
	orders.POST("/orders/:orderNo/refund", AdminStoreOrderRefund)
	read.GET("/licenses", AdminStoreLicenses)
	licenses.POST("/licenses/:id/grant", AdminStoreLicenseGrant)
	licenses.POST("/licenses/:id/revoke", AdminStoreLicenseRevoke)
	licenses.POST("/licenses/:id/transfer", AdminStoreLicenseTransfer)
	registerLicenseEntitlementRoutes(licenses)
	licenses.POST("/bindings/:bindingId/revoke", AdminStoreBindingRevoke)
	read.GET("/revenue", AdminStoreRevenue)
	revenue.POST("/revenue/:id/note", AdminStoreRevenueNote)
}

// AdminStorePlans 返回全部商业版套餐，含已下架的，供管理端编辑。读库失败返回 500。
func AdminStorePlans(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	rows, err := db.Query(`SELECT id, name, period, price_cents, enabled, sort FROM store_edition_plans ORDER BY sort, id`)
	if err != nil {
		storeFail(c, 500, "读取套餐失败")
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id, price int64
		var sort int
		var name, period string
		var enabled bool
		if err := rows.Scan(&id, &name, &period, &price, &enabled, &sort); err != nil {
			continue
		}
		list = append(list, gin.H{"id": id, "name": name, "period": period, "priceCents": price, "enabled": enabled, "sort": sort})
	}
	storeData(c, gin.H{"list": list})
}

// AdminStorePlanSave 新建或更新一条商业版套餐。
// 有路径 id 时更新，否则新建。名称空、周期不是永久或按年、价格不是正数时返回 400。写库失败返回 500。
func AdminStorePlanSave(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		Period     string `json:"period"`
		PriceCents int64  `json:"priceCents"`
		Enabled    bool   `json:"enabled"`
		Sort       int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		storeFail(c, 400, "参数错误")
		return
	}
	req.Period = strings.TrimSpace(req.Period)
	if !validEditionPeriod(req.Period) {
		storeFail(c, 400, "计费周期不合法")
		return
	}
	if req.PriceCents <= 0 {
		storeFail(c, 400, "价格不正确")
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	if id := strings.TrimSpace(c.Param("id")); id != "" {
		if _, err := db.Exec(`UPDATE store_edition_plans SET name=?, period=?, price_cents=?, enabled=?, sort=? WHERE id=?`,
			req.Name, req.Period, req.PriceCents, enabled, req.Sort, id); err != nil {
			storeFail(c, 500, "保存失败")
			return
		}
		storeData(c, gin.H{"id": id})
		return
	}
	res, err := db.Exec(`INSERT INTO store_edition_plans (name, period, price_cents, enabled, sort) VALUES (?, ?, ?, ?, ?)`,
		req.Name, req.Period, req.PriceCents, enabled, req.Sort)
	if err != nil {
		storeFail(c, 500, "保存失败")
		return
	}
	id, _ := res.LastInsertId()
	storeData(c, gin.H{"id": id})
}

// AdminStoreOrders 返回最近 200 笔商店订单。读库失败返回 500。
func AdminStoreOrders(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	rows, err := db.Query(`SELECT order_no, owner_type, owner_id, item_kind, title_snapshot, amount_cents, status, pay_channel, created_at, paid_at
		FROM store_purchase_orders ORDER BY id DESC LIMIT 200`)
	if err != nil {
		storeFail(c, 500, "读取订单失败")
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var ownerID, amount int64
		var orderNo, ownerType, kind, title, status, channel string
		var created, paid sql.NullTime
		if err := rows.Scan(&orderNo, &ownerType, &ownerID, &kind, &title, &amount, &status, &channel, &created, &paid); err != nil {
			continue
		}
		item := gin.H{"orderNo": orderNo, "ownerType": ownerType, "ownerId": ownerID, "itemKind": kind, "title": title, "amountCents": amount, "status": status, "payChannel": channel}
		if created.Valid {
			item["createdAt"] = created.Time.Format("2006-01-02 15:04:05")
		}
		if paid.Valid {
			item["paidAt"] = paid.Time.Format("2006-01-02 15:04:05")
		}
		list = append(list, item)
	}
	storeData(c, gin.H{"list": list})
}

// AdminStoreOrderRefund 按订单号退款。原因可空。订单不存在或状态不允许时返回 400。
func AdminStoreOrderRefund(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if err := refundStoreOrder(db, strings.TrimSpace(c.Param("orderNo")), req.Reason); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	storeData(c, gin.H{"ok": true})
}

// AdminStoreLicenses 列出挂在商业版产品应用上的主授权及其商业版状态。读库失败返回 500。
func AdminStoreLicenses(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	settings, _ := loadEffectiveStoreSettings(db)
	rows, err := db.Query(`SELECT l.id, l.license_no, l.owner_type, l.owner_id, l.status,
		COALESCE(e.edition, 'free'), COALESCE(e.period, ''), e.expires_at, e.status
		FROM licenses l
		JOIN apps a ON a.id = l.app_id AND a.app_key = ?
		LEFT JOIN main_license_editions e ON e.id = (
			SELECT id FROM main_license_editions WHERE license_id = l.id ORDER BY id DESC LIMIT 1
		)
		ORDER BY l.id DESC LIMIT 200`, settings.ProductAppKey)
	if err != nil {
		storeFail(c, 500, "读取主授权失败")
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id, ownerID int64
		var licenseNo, ownerType, licenseStatus, edition, period string
		var editionStatus sql.NullString
		var expires sql.NullTime
		if err := rows.Scan(&id, &licenseNo, &ownerType, &ownerID, &licenseStatus, &edition, &period, &expires, &editionStatus); err != nil {
			continue
		}
		item := gin.H{"id": id, "licenseNo": licenseNo, "ownerType": ownerType, "ownerId": ownerID, "licenseStatus": licenseStatus, "edition": edition, "period": period}
		if expires.Valid {
			item["editionExpireAt"] = expires.Time.Format("2006-01-02 15:04:05")
		}
		if editionStatus.Valid {
			item["editionStatus"] = editionStatus.String
		}
		list = append(list, item)
	}
	storeData(c, gin.H{"list": list})
}

// AdminStoreLicenseGrant 给指定授权手工开通商业版。
// period 为 permanent、yearly 或 monthly，空则按永久。授权编号不合法返回 400，写入失败返回 500。
func AdminStoreLicenseGrant(c *gin.Context) {
	var req struct {
		Period string `json:"period"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Period == "" {
		req.Period = storePeriodPermanent
	}
	if !validEditionPeriod(req.Period) {
		storeFail(c, 400, "计费周期不合法")
		return
	}
	licenseID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || licenseID <= 0 {
		storeFail(c, 400, "授权不正确")
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	extra, err := boundLicensesForSameDomain(db, licenseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			storeFail(c, 400, "授权不正确")
			return
		}
		storeFail(c, 500, "授予失败")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		storeFail(c, 500, "授予失败")
		return
	}
	defer tx.Rollback()
	if _, err := openCommercialEdition(tx, commercialEditionGrant{
		LicenseID: licenseID, Period: req.Period, GrantedBy: currentAdminID(c), Extend: true,
	}, extra); err != nil {
		storeFail(c, 500, "授予失败")
		return
	}
	if err := tx.Commit(); err != nil {
		storeFail(c, 500, "授予失败")
		return
	}
	writeLicenseOperation(db, licenseID, "grant_edition", req.Period, editionPeriodLabel(req.Period), c.GetString("username"))
	storeData(c, gin.H{"ok": true})
}

// AdminStoreLicenseRevoke 吊销这条授权的商业版权益。绑定保持有效，客户站刷新后变为免费版。
// reason 会截断后记入审计。授权不存在或写库失败返回 500。
func AdminStoreLicenseRevoke(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	licenseID := strings.TrimSpace(c.Param("id"))
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if err := revokeCommercialRightsForLicense(db, licenseID, trimStoreText(req.Reason, 200)); err != nil {
		storeFail(c, 500, "吊销失败")
		return
	}
	if id, err := strconv.ParseInt(licenseID, 10, 64); err == nil {
		writeLicenseOperation(db, id, "revoke_edition", "", trimStoreText(req.Reason, 200), c.GetString("username"))
	}
	storeData(c, gin.H{"ok": true})
}

// AdminStoreLicenseTransfer 把主授权改到另一个用户或代理名下。
// 目标账号不存在返回 400。不能转到会拆掉商业版绑定关系的归属。
func AdminStoreLicenseTransfer(c *gin.Context) {
	var req struct {
		OwnerType string `json:"ownerType"`
		OwnerID   int64  `json:"ownerId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.OwnerType != "user" && req.OwnerType != "agent") || req.OwnerID <= 0 {
		storeFail(c, 400, "请指定新的归属账号")
		return
	}
	licenseID := strings.TrimSpace(c.Param("id"))
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if _, err := db.Exec(`UPDATE licenses SET owner_type = ?, owner_id = ? WHERE id = ?`, req.OwnerType, req.OwnerID, licenseID); err != nil {
		storeFail(c, 500, "转移失败")
		return
	}
	// 商业版记在授权上，已购插件记在权益表。归属改了以后两边都要跟着走。
	// 不吊销站点绑定：客户站靠这条绑定刷新，拆掉绑定就会要求重新登录。
	_, _ = db.Exec(`UPDATE plugin_entitlements SET owner_type = ?, owner_id = ? WHERE license_id = ? AND status = 'active'`, req.OwnerType, req.OwnerID, licenseID)
	_, _ = db.Exec(`UPDATE store_bindings SET owner_type = ?, owner_id = ? WHERE license_id = ? AND status = 'active'`, req.OwnerType, req.OwnerID, licenseID)
	if id, err := strconv.ParseInt(licenseID, 10, 64); err == nil {
		writeLicenseOperation(db, id, "transfer", req.OwnerType, "权益已随授权转移到新账号", c.GetString("username"))
	}
	storeData(c, gin.H{"ok": true})
}

// AdminStoreBindingRevoke 只吊销站点绑定，不删除授权记录。
// 买家下次核对会收到 revoked，并被要求重新绑定。
func AdminStoreBindingRevoke(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if _, err := db.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = 'admin_unbind' WHERE binding_id = ? AND status = 'active'`, c.Param("bindingId")); err != nil {
		storeFail(c, 500, "解绑失败")
		return
	}
	storeData(c, gin.H{"ok": true})
}

// AdminStoreRevenue 汇总商店收入。读库失败返回 500。
func AdminStoreRevenue(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	rows, err := db.Query(`SELECT id, order_id, source_type, gross_cents, fee_bps, net_cents, status, payout_note, created_at
		FROM store_revenue_ledger ORDER BY id DESC LIMIT 200`)
	if err != nil {
		storeFail(c, 500, "读取收入失败")
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id, orderID, gross, net int64
		var fee int
		var source, status, note string
		var created sql.NullTime
		if err := rows.Scan(&id, &orderID, &source, &gross, &fee, &net, &status, &note, &created); err != nil {
			continue
		}
		item := gin.H{"id": id, "orderId": orderID, "sourceType": source, "grossCents": gross, "feeBps": fee, "netCents": net, "status": status, "note": note}
		if created.Valid {
			item["createdAt"] = created.Time.Format("2006-01-02 15:04:05")
		}
		list = append(list, item)
	}
	storeData(c, gin.H{"list": list})
}

// AdminStoreRevenueNote 给一笔记账补备注。账目不存在返回 400。
func AdminStoreRevenueNote(c *gin.Context) {
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if _, err := db.Exec(`UPDATE store_revenue_ledger SET payout_note = ? WHERE id = ?`, trimStoreText(req.Note, 500), c.Param("id")); err != nil {
		storeFail(c, 500, "保存失败")
		return
	}
	storeData(c, gin.H{"ok": true})
}

func currentAdminID(c *gin.Context) int64 {
	id, _ := c.Get("user_id")
	switch v := id.(type) {
	case int64:
		return v
	case uint:
		return int64(v)
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}
