package handler

import (
	"database/sql"
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
	licenses.POST("/bindings/:bindingId/revoke", AdminStoreBindingRevoke)
	read.GET("/revenue", AdminStoreRevenue)
	revenue.POST("/revenue/:id/note", AdminStoreRevenueNote)
}

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
	if req.Period != storePeriodPermanent && req.Period != storePeriodYearly {
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

func AdminStoreLicenseGrant(c *gin.Context) {
	var req struct {
		Period string `json:"period"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Period == "" {
		req.Period = storePeriodPermanent
	}
	if req.Period != storePeriodPermanent && req.Period != storePeriodYearly {
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
	tx, err := db.Begin()
	if err != nil {
		storeFail(c, 500, "授予失败")
		return
	}
	defer tx.Rollback()
	if _, err := grantCommercialEditionTx(tx, commercialEditionGrant{
		LicenseID: licenseID, Period: req.Period, GrantedBy: currentAdminID(c), Extend: true,
	}); err != nil {
		storeFail(c, 500, "授予失败")
		return
	}
	if err := tx.Commit(); err != nil {
		storeFail(c, 500, "授予失败")
		return
	}
	storeData(c, gin.H{"ok": true})
}

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
	if _, err := db.Exec(`UPDATE main_license_editions SET status = 'revoked', updated_at = NOW() WHERE license_id = ? AND status = 'active'`, licenseID); err != nil {
		storeFail(c, 500, "吊销失败")
		return
	}
	_, _ = db.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = ? WHERE license_id = ? AND status = 'active'`, trimStoreText(req.Reason, 200), licenseID)
	storeData(c, gin.H{"ok": true})
}

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
	_, _ = db.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = 'transfer' WHERE license_id = ? AND status = 'active'`, licenseID)
	storeData(c, gin.H{"ok": true})
}

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
