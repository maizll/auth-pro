// 源站侧的商业版和单品订单。支付渠道验签通过后，由这里按订单号决定入账到升级、商店、换站还是普通购买。

package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"auto_pro/payment"

	"github.com/gin-gonic/gin"
)

// dispatchVerifiedOnlinePayment 在支付渠道已经验签、金额也核对过后入账。
// orderNo 决定去向：代理升级、商店购买、换站，其余才是充值或普通授权购买。
// 商店订单必须先于充值匹配。订单号前缀撞车时，充值入账会把商店订单当成不存在而失败，权益就丢了。
// 金额与订单不一致、订单已关闭时返回错误，调用方应让渠道重试或展示失败。
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
		if _, err := openCommercialEdition(tx, commercialEditionGrant{
			LicenseID: licenseID, Period: period, OrderID: id, Extend: true, Stack: true, MarkStorePurchase: true,
		}, nil); err != nil {
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
	ledgerSource := "edition"
	if itemKind == "plugin" || itemKind == "template" {
		ledgerSource = itemKind
	}
	if _, err := tx.Exec(`INSERT INTO store_revenue_ledger
		(order_id, source_type, developer_id, gross_cents, fee_bps, net_cents, status, settled_at)
		VALUES (?, ?, NULL, ?, 10000, ?, 'settled', NOW())`, id, ledgerSource, amount, amount); err != nil {
		return err
	}
	return tx.Commit()
}

// StoreEditionPlans 列出源站正在出售的商业版套餐。
// 数据库读失败返回 500。不返回已下架套餐。
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
	storeData(c, gin.H{"list": list, "payOptions": configuredOnlinePayOptions(db)})
}

// StoreOrderCreate 为已签名的买家创建商业版或单品订单。
// 请求必须带绑定签名。超过频率、域名不合法、套餐不存在或条目未上架时返回 400。
// 绑定已吊销时返回 400，并带 revoked 和 rebind，不能只靠中文判断。
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
		ItemKind  string `json:"itemKind"`
		ItemID    string `json:"itemId"`
		PlanID    int64  `json:"planId"`
		PayMethod string `json:"payMethod"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	if req.ItemKind == "" {
		req.ItemKind = "edition"
	}
	// 空字符串表示沿用源站排在第一位的收款方式，兼容还没传支付方式的旧买家。
	c.Set("storePayMethod", strings.TrimSpace(req.PayMethod))
	db := c.MustGet("storeDB").(*sql.DB)
	orderNo := storeOrderPrefix + strconv.FormatInt(time.Now().Unix(), 10) + randomHex(4)
	returnURL := buildRequestURL(c, "/store/pay-complete")
	switch req.ItemKind {
	case "edition":
		name, period, price, err := loadCommercialSalePlan(db, req.PlanID)
		if err != nil {
			storeFail(c, 400, err.Error())
			return
		}
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
	case "plugin", "template":
		quote, err := loadCatalogSaleQuote(db, req.ItemKind, req.ItemID)
		if err != nil {
			storeFail(c, 400, err.Error())
			return
		}
		payURL, channel, method, err := createStorePayment(c, db, orderNo, quote.PriceCents, quote.Name)
		if err != nil {
			storeFail(c, 400, err.Error())
			return
		}
		if !acceptablePayURL(payURL) {
			storeFail(c, 400, "收款地址协议不受支持")
			return
		}
		var developer any
		if quote.DeveloperID > 0 {
			developer = quote.DeveloperID
		}
		_, err = db.Exec(`INSERT INTO store_purchase_orders
			(order_no, owner_type, owner_id, license_id, binding_id, item_kind, item_id, period, developer_id, amount_cents, price_cents_snapshot, title_snapshot, pay_channel, pay_method, status, return_url, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
			orderNo, row.OwnerType, row.OwnerID, row.LicenseID, row.BindingID, quote.Kind, quote.ID, quote.Period, developer,
			quote.PriceCents, quote.PriceCents, quote.Name, channel, method, returnURL, time.Now().Add(storeOrderTTL))
		if err != nil {
			storeFail(c, 500, "创建订单失败")
			return
		}
		storeData(c, gin.H{
			"orderNo": orderNo, "payUrl": payURL, "amountCents": quote.PriceCents, "title": quote.Name,
			"itemKind": quote.Kind, "itemId": quote.ID, "period": quote.Period, "purchaseOnly": quote.PurchaseOnly,
		})
	default:
		storeFail(c, 400, "不支持的购买类型")
	}
}

type catalogSaleQuote struct {
	Kind         string
	ID           string
	Name         string
	PriceCents   int64
	Billing      string
	Period       string
	DeveloperID  int64
	PurchaseOnly bool
}

func loadCatalogSaleQuote(db *sql.DB, kind, id string) (catalogSaleQuote, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		if kind == "template" {
			return catalogSaleQuote{}, errors.New("请选择要购买的模板")
		}
		return catalogSaleQuote{}, errors.New("请选择要购买的插件")
	}
	var name, billing, status string
	var price, developerID int64
	var err error
	switch kind {
	case "plugin":
		err = db.QueryRow(`SELECT name, price_cents, billing, developer_id, status FROM source_catalog_plugins WHERE id = ?`, id).
			Scan(&name, &price, &billing, &developerID, &status)
	case "template":
		err = db.QueryRow(`SELECT name, price_cents, billing, developer_id, status FROM source_catalog_templates WHERE template_key = ? OR id = ? ORDER BY CASE WHEN template_key = ? THEN 0 ELSE 1 END LIMIT 1`, id, id, id).
			Scan(&name, &price, &billing, &developerID, &status)
	default:
		return catalogSaleQuote{}, errors.New("不支持的购买类型")
	}
	if errors.Is(err, sql.ErrNoRows) {
		return catalogSaleQuote{}, errors.New("该条目不存在或未上架")
	}
	if err != nil {
		return catalogSaleQuote{}, errors.New("读取条目价格失败")
	}
	if status != sourceItemPublished || price <= 0 {
		return catalogSaleQuote{}, errors.New("该条目不存在或未上架")
	}
	if billing == sourceBillingYearly {
		return catalogSaleQuote{}, errSourcePaidYearly
	}
	purchaseOnly := developerID > 0 || catalogItemPurchaseOnly(kind, id)
	return catalogSaleQuote{
		Kind: kind, ID: id, Name: name, PriceCents: price, Billing: billing,
		Period: catalogSalePeriod(billing), DeveloperID: developerID, PurchaseOnly: purchaseOnly,
	}, nil
}

func catalogSalePeriod(billing string) string {
	switch strings.ToLower(strings.TrimSpace(billing)) {
	case sourceBillingYearly:
		return "yearly"
	default:
		return "permanent"
	}
}

// StoreOrderQuery 按订单号返回支付状态。已支付且属于当前绑定时附带新快照。
// 订单不属于这个买家返回 404。绑定已终止时同样要求重新绑定。
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

var createStorePayment = createStorePaymentDefault

// pickConfiguredPayOption 按买家选的 code 取收款方式。没传时用列表第一项，传了但不在列表里就拒绝。
func pickConfiguredPayOption(options []payOption, requested string) (payOption, error) {
	if len(options) == 0 {
		return payOption{}, errors.New("源站未配置收款方式")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return options[0], nil
	}
	for _, option := range options {
		if option.Code == requested {
			return option, nil
		}
	}
	return payOption{}, errors.New("该支付方式未开启")
}

func createStorePaymentDefault(c *gin.Context, db *sql.DB, orderNo string, amountCents int64, title string) (payURL, channel, method string, err error) {
	requested, _ := c.Get("storePayMethod")
	requestedText, _ := requested.(string)
	option, err := pickConfiguredPayOption(configuredOnlinePayOptions(db), requestedText)
	if err != nil {
		return "", "", "", err
	}
	selection, ok := parseOnlinePaySelection(option.Code)
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
