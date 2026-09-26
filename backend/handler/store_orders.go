package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"auto_pro/payment"

	"github.com/gin-gonic/gin"
)

func dispatchVerifiedOnlinePayment(db *sql.DB, orderNo string, paidCents int64, channel, payMethod, tradeNo, payload string) error {
	switch onlineSettlementRoute(orderNo) {
	case "upgrade":
		return settleAgentUpgradeOnlinePayment(db, orderNo, paidCents, channel, payMethod, tradeNo, payload)
	case "store":
		return settleStorePurchaseOrder(db, orderNo, paidCents, channel, payMethod, tradeNo, payload)
	case "site_change":
		return settleSiteChangeOrder(db, orderNo, paidCents, channel, payMethod, tradeNo, payload)
	default:
		if err := settleRechargeOrder(db, orderNo, paidCents, tradeNo, payMethod, payload); err == nil {
			return nil
		}
		return settleLicensePurchaseOrder(db, orderNo, paidCents, channel, tradeNo, payMethod, payload)
	}
}

func settleStorePurchaseOrder(db *sql.DB, orderNo string, paidCents int64, channel, payMethod, tradeNo, payload string) error {
	if err := ensurePaidStoreSchema(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id, licenseID, amount int64
	var ownerType, itemKind, itemID, period, status string
	var ownerID int64
	err = tx.QueryRow(`SELECT id, owner_type, owner_id, license_id, item_kind, item_id, period, amount_cents, status
		FROM store_purchase_orders WHERE order_no = ? FOR UPDATE`, orderNo).
		Scan(&id, &ownerType, &ownerID, &licenseID, &itemKind, &itemID, &period, &amount, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errStoreOrderNotFound
		}
		return err
	}
	grant, err := shouldGrantStoreOrder(status, amount, paidCents)
	if err != nil {
		return err
	}
	if !grant {
		return tx.Commit()
	}
	if _, err := tx.Exec(`UPDATE store_purchase_orders
		SET status = 'paid', pay_channel = ?, pay_method = ?, gateway_trade_no = ?, paid_at = NOW(), notify_payload = ?
		WHERE id = ? AND status = 'pending'`, channel, payMethod, tradeNo, payload, id); err != nil {
		return err
	}
	switch itemKind {
	case "edition":
		if _, err := grantCommercialEditionTx(tx, commercialEditionGrant{
			LicenseID: licenseID, Period: period, OrderID: id, Extend: true, Stack: true, MarkStorePurchase: true,
		}); err != nil {
			return err
		}
	case "plugin", "template":
		if _, err := tx.Exec(`INSERT INTO plugin_entitlements
			(order_id, license_id, owner_type, owner_id, item_kind, item_id, period, source, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'purchase', 'active')`, id, licenseID, ownerType, ownerID, itemKind, itemID, period); err != nil {
			return err
		}
	default:
		return errors.New("订单类型不受支持")
	}
	if _, err := tx.Exec(`INSERT INTO store_revenue_ledger
		(order_id, source_type, developer_id, gross_cents, fee_bps, net_cents, status, settled_at)
		VALUES (?, 'edition', NULL, ?, 10000, ?, 'settled', NOW())`, id, amount, amount); err != nil {
		return err
	}
	return tx.Commit()
}

func StoreEditionPlans(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	list, err := listCommercialSalePlans(db)
	if err != nil {
		storeFail(c, 500, "读取套餐失败")
		return
	}
	storeData(c, gin.H{"list": list})
}

func StoreOrderCreate(c *gin.Context) {
	if !storeOrderRate.allow(c.ClientIP(), 30, time.Minute, time.Now()) {
		storeFail(c, 429, "下单过于频繁")
		return
	}
	body, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	var req struct {
		ItemKind string `json:"itemKind"`
		PlanID   int64  `json:"planId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	if req.ItemKind != "edition" {
		storeFail(c, 400, "当前版本只支持购买商业版")
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	name, period, price, err := loadCommercialSalePlan(db, req.PlanID)
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	orderNo := storeOrderPrefix + strconv.FormatInt(time.Now().Unix(), 10) + randomHex(4)
	returnURL := buildRequestURL(c, "/store/pay-complete")
	payURL, channel, method, err := createStorePayment(c, db, orderNo, price, name)
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	if !acceptablePayURL(payURL) {
		storeFail(c, 400, "收款地址协议不受支持")
		return
	}
	_, err = db.Exec(`INSERT INTO store_purchase_orders
		(order_no, owner_type, owner_id, license_id, binding_id, item_kind, item_id, period, amount_cents, price_cents_snapshot, title_snapshot, pay_channel, pay_method, status, return_url, expires_at)
		VALUES (?, ?, ?, ?, ?, 'edition', ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		orderNo, row.OwnerType, row.OwnerID, row.LicenseID, row.BindingID, strconv.FormatInt(req.PlanID, 10), period,
		price, price, name, channel, method, returnURL, time.Now().Add(storeOrderTTL))
	if err != nil {
		storeFail(c, 500, "创建订单失败")
		return
	}
	storeData(c, gin.H{"orderNo": orderNo, "payUrl": payURL, "amountCents": price, "title": name})
}

func StoreOrderQuery(c *gin.Context) {
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	orderNo := strings.TrimSpace(c.Param("orderNo"))
	var status, itemKind string
	var licenseID int64
	var licenseNo, domain string
	err := db.QueryRow(`SELECT o.status, o.item_kind, o.license_id, l.license_no
		FROM store_purchase_orders o JOIN licenses l ON l.id = o.license_id
		WHERE o.order_no = ? AND o.binding_id = ?`, orderNo, row.BindingID).Scan(&status, &itemKind, &licenseID, &licenseNo)
	if err != nil {
		storeFail(c, 404, "订单不存在")
		return
	}
	domain = row.Domain
	data := gin.H{"orderNo": orderNo, "status": status}
	if status == "paid" {
		settings, err := loadEffectiveStoreSettings(db)
		if err != nil {
			storeFail(c, 500, "读取配置失败")
			return
		}
		snapshot, err := buildStoreSnapshot(db, row.BindingID, licenseID, licenseNo, domain, settings)
		if err != nil {
			storeFail(c, 500, err.Error())
			return
		}
		data["snapshot"] = snapshot
	}
	storeData(c, data)
}

func createStorePayment(c *gin.Context, db *sql.DB, orderNo string, amountCents int64, title string) (payURL, channel, method string, err error) {
	options := configuredOnlinePayOptions(db)
	if len(options) == 0 {
		return "", "", "", errors.New("源站未配置收款方式")
	}
	selection, ok := parseOnlinePaySelection(options[0].Code)
	if !ok {
		return "", "", "", errors.New("源站未配置收款方式")
	}
	if selection.Channel == "" {
		selection.Channel = payChannelEpayV1
	}
	complete := buildRequestURL(c, "/store/pay-complete")
	switch selection.Channel {
	case payChannelEpayV1:
		cfg, err := loadEpayConfig(db)
		if err != nil {
			return "", "", "", err
		}
		payURL, _, err = buildEpaySubmitURL(c, cfg, orderNo, amountCents, selection.PayType, title, "/store/pay-complete")
		if err != nil {
			return "", "", "", err
		}
		return payURL, selection.Channel, selection.PayType, nil
	case payChannelEpayV2:
		cfg, err := loadEpayV2Config(db)
		if err != nil {
			return "", "", "", err
		}
		notifyURL := strings.TrimSpace(cfg.NotifyURL)
		if notifyURL == "" {
			notifyURL = buildRequestURL(c, "/api/payment/easypay-v2/notify")
		}
		gatewayReturn := strings.TrimSpace(cfg.ReturnURL)
		if gatewayReturn == "" {
			gatewayReturn = buildRequestURL(c, "/api/payment/easypay-v2/return")
		}
		payURL, err = epayV2CreateOrder(cfg, orderNo, amountCents, selection.PayType, title, notifyURL, gatewayReturn, c.ClientIP())
		if err != nil {
			return "", "", "", err
		}
		return payURL, selection.Channel, selection.PayType, nil
	default:
		ch, ok := payment.Get(selection.Channel)
		if !ok || !isPluginEnabled(db, ch.PluginID()) || !ch.Available(db) {
			return "", "", "", errors.New("该支付方式未开启")
		}
		result, err := ch.CreatePayment(db, payment.CreateRequest{
			OrderNo: orderNo, AmountCents: amountCents, Subject: title, PayType: selection.PayType,
			NotifyURL: buildRequestURL(c, "/api/payment/"+ch.ID()+"/notify"),
			ReturnURL: complete, ClientIP: c.ClientIP(),
		})
		if err != nil {
			return "", "", "", err
		}
		payURL = strings.TrimSpace(result.QRCode)
		if payURL == "" {
			payURL = strings.TrimSpace(result.PayURL)
		}
		return payURL, selection.Channel, selection.PayType, nil
	}
}

func refundStoreOrder(db *sql.DB, orderNo, reason string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, licenseID int64
	var status, itemKind, itemID string
	if err := tx.QueryRow(`SELECT id, license_id, status, item_kind, item_id FROM store_purchase_orders WHERE order_no = ? FOR UPDATE`, orderNo).
		Scan(&id, &licenseID, &status, &itemKind, &itemID); err != nil {
		return errStoreOrderNotFound
	}
	if status != "paid" {
		return errStoreOrderClosed
	}
	if _, err := tx.Exec(`UPDATE store_purchase_orders SET status = 'refunded', needs_review = 0 WHERE id = ?`, id); err != nil {
		return err
	}
	if itemKind == "edition" {
		if _, err := tx.Exec(`UPDATE main_license_editions SET status = 'revoked', updated_at = NOW() WHERE order_id = ? AND status = 'active'`, id); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE plugin_entitlements SET status = 'revoked', revoked_at = NOW(), revoke_reason = ? WHERE order_id = ? AND status = 'active'`, trimStoreText(reason, 200), id); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(`UPDATE store_revenue_ledger SET status = 'refunded' WHERE order_id = ?`, id)
	return tx.Commit()
}

func fmtStoreCents(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func writeStoreList(c *gin.Context, rows *sql.Rows, columns []string) {
	list := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		item := map[string]any{}
		for i, name := range columns {
			switch v := values[i].(type) {
			case []byte:
				item[name] = string(v)
			default:
				item[name] = v
			}
		}
		list = append(list, item)
	}
	storeData(c, gin.H{"list": list})
}

func storeHTTPError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"code":%d,"msg":%q}`, code, msg)))
}
