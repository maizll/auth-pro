package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const adOrderPrefix = "AD"

// 状态机（无 refunded）：待付款 → 已付款·检查中 → 排期中 | 待人工复核 | 待修改 → 投放中 ⇄ 已暂停 → 已结束 | 已作废
const (
	adOrderPendingPay   = "pending_pay"
	adOrderChecking     = "checking"
	adOrderScheduled    = "scheduled"
	adOrderManualReview = "manual_review"
	adOrderNeedEdit     = "need_edit"
	adOrderRunning      = "running"
	adOrderPaused       = "paused"
	adOrderEnded        = "ended"
	adOrderVoided       = "voided"
	adOrderExpiredLock  = "expired_lock"
)

type adOrder struct {
	ID              int64
	OrderNo         string
	BuyerType       string // admin / agent / developer
	BuyerID         int64
	BuyerName       string
	SlotID          string
	Title           string
	Description     string
	ImageURL        string
	LinkURL         string
	Days            int
	DayStart        string // YYYY-MM-DD
	DayEnd          string
	AmountCents     int64
	Status          string
	PayChannel      string
	PayMethod       string
	GatewayTradeNo  string
	ReturnURL       string
	CheckResult     string // JSON
	ReviewNote      string
	ReviewedBy      string
	AdvertisementID string
	PostponeDays    int
	VoidReason      string
	LockUntil       time.Time
	PaidAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func ensureAdOrderSchema(db *sql.DB) error {
	if err := ensureAdSlotDaySchema(db); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS source_ad_orders (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		order_no VARCHAR(40) NOT NULL,
		buyer_type VARCHAR(20) NOT NULL DEFAULT 'admin',
		buyer_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
		buyer_name VARCHAR(80) NOT NULL DEFAULT '',
		slot_id VARCHAR(40) NOT NULL,
		title VARCHAR(40) NOT NULL DEFAULT '',
		description VARCHAR(120) NOT NULL DEFAULT '',
		image_url VARCHAR(500) NOT NULL DEFAULT '',
		link_url VARCHAR(500) NOT NULL DEFAULT '',
		days INT NOT NULL DEFAULT 0,
		day_start DATE NOT NULL,
		day_end DATE NOT NULL,
		amount_cents BIGINT NOT NULL DEFAULT 0,
		status VARCHAR(30) NOT NULL DEFAULT 'pending_pay',
		pay_channel VARCHAR(30) NOT NULL DEFAULT '',
		pay_method VARCHAR(30) NOT NULL DEFAULT '',
		gateway_trade_no VARCHAR(100) NOT NULL DEFAULT '',
		return_url VARCHAR(500) NOT NULL DEFAULT '',
		notify_payload MEDIUMTEXT NULL,
		check_result TEXT NULL,
		review_note VARCHAR(500) NOT NULL DEFAULT '',
		reviewed_by VARCHAR(50) NOT NULL DEFAULT '',
		advertisement_id VARCHAR(60) NOT NULL DEFAULT '',
		postpone_days INT NOT NULL DEFAULT 0,
		void_reason VARCHAR(200) NOT NULL DEFAULT '',
		lock_until DATETIME NULL,
		paid_at DATETIME NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_ad_order_no (order_no),
		KEY idx_ad_order_status (status),
		KEY idx_ad_order_buyer (buyer_type, buyer_id),
		KEY idx_ad_order_slot (slot_id, day_start, day_end)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告自助购买订单（无退款）'`)
	return err
}

func generateAdOrderNo() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s%s", adOrderPrefix, time.Now().Format("20060102150405"), strings.ToUpper(hex.EncodeToString(b[:]))), nil
}

type adOrderCreateRequest struct {
	SlotID      string   `json:"slotId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ImageURL    string   `json:"imageUrl"`
	LinkURL     string   `json:"linkUrl"`
	Days        []string `json:"days"` // YYYY-MM-DD 列表
	Agree       bool     `json:"agree"`
}

// ClientAdOrderCreate 锁名额 15 分钟并创建待付款订单。
func ClientAdOrderCreate(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	if err := ensureAdOrderSchema(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化订单表失败"})
		return
	}
	loadAdSlotSwitchesFromStore()

	var req adOrderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if !req.Agree {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请勾选同意虚拟产品不退款规则"})
		return
	}
	slotID := canonicalizeAdSlot(req.SlotID)
	def, ok := adSlotDefOf(slotID)
	if !ok || !def.Sellable || !isAdSlotEnabled(slotID) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告位不可售或已停用"})
		return
	}
	title := strings.TrimSpace(req.Title)
	desc := strings.TrimSpace(req.Description)
	link := strings.TrimSpace(req.LinkURL)
	image := strings.TrimSpace(req.ImageURL)
	if title == "" || utf8.RuneCountInString(title) > 20 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "标题必填且不超过 20 字"})
		return
	}
	if utf8.RuneCountInString(desc) > 60 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "简介不超过 60 字"})
		return
	}
	if err := validateAdLinkURL(link); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	days := normalizeAdDays(req.Days)
	if len(days) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择投放日期"})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "开启事务失败"})
		return
	}
	defer tx.Rollback()

	for _, day := range days {
		left, err := lockAdSlotDay(tx, slotID, day, def.Capacity)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		if left < 0 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": day.Format("2006-01-02") + " 名额已满"})
			return
		}
	}

	orderNo, err := generateAdOrderNo()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成订单号失败"})
		return
	}
	amount := def.PriceCents * int64(len(days))
	buyerName := strings.TrimSpace(c.GetString("username"))
	buyerID := int64(0)
	if v, ok := c.Get("userID"); ok {
		switch n := v.(type) {
		case uint:
			buyerID = int64(n)
		case int64:
			buyerID = n
		case int:
			buyerID = int64(n)
		}
	}
	lockUntil := time.Now().Add(15 * time.Minute)
	dayStart, dayEnd := days[0], days[len(days)-1]
	_, err = tx.Exec(`INSERT INTO source_ad_orders
		(order_no, buyer_type, buyer_id, buyer_name, slot_id, title, description, image_url, link_url,
		 days, day_start, day_end, amount_cents, status, lock_until)
		VALUES (?, 'admin', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orderNo, buyerID, buyerName, slotID, title, desc, image, link,
		len(days), dayStart.Format("2006-01-02"), dayEnd.Format("2006-01-02"), amount, adOrderPendingPay, lockUntil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建订单失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "订单已创建，请在 15 分钟内付款", "data": gin.H{
		"orderNo":        orderNo,
		"amountCents":    amount,
		"amount":         formatCents(amount),
		"days":           len(days),
		"slotId":         slotID,
		"slotName":       def.Name,
		"lockUntil":      lockUntil.Format(time.RFC3339),
		"noRefundNotice": adNoRefundNotice,
		"status":         adOrderPendingPay,
	}})
}

func normalizeAdDays(raw []string) []time.Time {
	seen := map[string]bool{}
	out := make([]time.Time, 0, len(raw))
	today := time.Now().In(time.Local)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	for _, s := range raw {
		s = strings.TrimSpace(s)
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil || seen[s] {
			continue
		}
		if t.Before(today) {
			continue
		}
		seen[s] = true
		out = append(out, t)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Before(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func lockAdSlotDay(tx *sql.Tx, slotID string, day time.Time, capacity int) (int, error) {
	slotID = canonicalizeAdSlot(slotID)
	dayStr := day.Format("2006-01-02")
	var reserved, locked int
	err := tx.QueryRow(`SELECT reserved, locked FROM source_ad_slot_days WHERE slot_id=? AND day_date=? FOR UPDATE`, slotID, dayStr).
		Scan(&reserved, &locked)
	if err == sql.ErrNoRows {
		_, err = tx.Exec(`INSERT INTO source_ad_slot_days (slot_id, day_date, reserved, locked) VALUES (?, ?, 0, 1)`, slotID, dayStr)
		if err != nil {
			return -1, err
		}
		return capacity - 1, nil
	}
	if err != nil {
		return -1, err
	}
	if reserved+locked >= capacity {
		return -1, fmt.Errorf("%s 名额已满", dayStr)
	}
	_, err = tx.Exec(`UPDATE source_ad_slot_days SET locked = locked + 1 WHERE slot_id=? AND day_date=?`, slotID, dayStr)
	if err != nil {
		return -1, err
	}
	return capacity - reserved - locked - 1, nil
}

func validateAdLinkURL(link string) error {
	if link == "" {
		return errors.New("请填写推广链接")
	}
	if !strings.HasPrefix(strings.ToLower(link), "https://") {
		return errors.New("推广链接须为 https://")
	}
	if len(link) > 500 {
		return errors.New("链接过长")
	}
	return nil
}

type adOrderPayRequest struct {
	OrderNo string `json:"orderNo"`
	PayCode string `json:"payCode"` // alipay / wxpay 等，与线上支付选项一致
	Agree   bool   `json:"agree"`
}

// ClientAdOrderPay 对已创建的 AD 订单发起官网收款（复用当面付/易支付）。
func ClientAdOrderPay(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	if err := ensureAdOrderSchema(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化失败"})
		return
	}
	var req adOrderPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if !req.Agree {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请勾选同意虚拟产品不退款规则"})
		return
	}
	orderNo := strings.TrimSpace(req.OrderNo)
	order, err := loadAdOrder(db, orderNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if order.Status != adOrderPendingPay {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "订单状态不可付款"})
		return
	}
	if !order.LockUntil.IsZero() && time.Now().After(order.LockUntil) {
		_ = releaseAdOrderLock(db, order)
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "名额锁定已过期，请重新下单"})
		return
	}
	selection, ok := parseOnlinePaySelection(req.PayCode)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不支持的支付方式"})
		return
	}
	orderName := "广告投放 " + order.Title
	returnPath := "/ads-promote"
	frontendReturnURL := buildFrontendReturnURL(c, orderNo, returnPath)
	_, _ = db.Exec(`UPDATE source_ad_orders SET return_url=?, pay_channel=?, pay_method=? WHERE order_no=? AND status=?`,
		frontendReturnURL, selection.Channel, selection.PayType, orderNo, adOrderPendingPay)

	if isEpayPayChannel(selection.Channel) && selection.Channel == payChannelEpayV1 {
		payConfig, err := loadEpayConfig(db)
		if err != nil || payConfig.validateForPay() != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "支付通道未配置"})
			return
		}
		payURL, _, err := buildEpaySubmitURL(c, payConfig, orderNo, order.AmountCents, selection.PayType, orderName, returnPath)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建支付失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "支付订单已创建，正在跳转收银台", "data": gin.H{
			"orderNo": orderNo, "amount": formatCents(order.AmountCents), "payType": selection.PayType, "payUrl": payURL,
			"noRefundNotice": adNoRefundNotice,
		}})
		return
	}
	if selection.Channel == payChannelEpayV2 {
		payConfigV2, err := loadEpayV2Config(db)
		if err != nil || payConfigV2.validateForPay() != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "支付通道未配置"})
			return
		}
		payURL, _, err := buildEpayV2Payment(c, payConfigV2, orderNo, order.AmountCents, selection.PayType, orderName, returnPath)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建支付失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "支付订单已创建，正在跳转收银台", "data": gin.H{
			"orderNo": orderNo, "amount": formatCents(order.AmountCents), "payType": selection.PayType, "payUrl": payURL,
			"noRefundNotice": adNoRefundNotice,
		}})
		return
	}
	result, _, err := createRegisteredChannelPayment(c, db, selection, orderNo, order.AmountCents, orderName, returnPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "支付网关下单失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "支付订单已创建，请扫码完成支付", "data": checkoutData(orderNo, formatCents(order.AmountCents), selection.PayType, result)})
}

func loadAdOrder(db *sql.DB, orderNo string) (adOrder, error) {
	var o adOrder
	var lockUntil, paidAt sql.NullTime
	err := db.QueryRow(`SELECT id, order_no, buyer_type, buyer_id, buyer_name, slot_id, title, description, image_url, link_url,
		days, day_start, day_end, amount_cents, status, pay_channel, pay_method, gateway_trade_no, return_url,
		IFNULL(check_result,''), review_note, reviewed_by, advertisement_id, postpone_days, void_reason, lock_until, paid_at, created_at, updated_at
		FROM source_ad_orders WHERE order_no=?`, orderNo).Scan(
		&o.ID, &o.OrderNo, &o.BuyerType, &o.BuyerID, &o.BuyerName, &o.SlotID, &o.Title, &o.Description, &o.ImageURL, &o.LinkURL,
		&o.Days, &o.DayStart, &o.DayEnd, &o.AmountCents, &o.Status, &o.PayChannel, &o.PayMethod, &o.GatewayTradeNo, &o.ReturnURL,
		&o.CheckResult, &o.ReviewNote, &o.ReviewedBy, &o.AdvertisementID, &o.PostponeDays, &o.VoidReason, &lockUntil, &paidAt, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return o, err
	}
	if lockUntil.Valid {
		o.LockUntil = lockUntil.Time
	}
	if paidAt.Valid {
		t := paidAt.Time
		o.PaidAt = &t
	}
	return o, nil
}

func releaseAdOrderLock(db *sql.DB, order adOrder) error {
	days := expandAdOrderDays(order)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, day := range days {
		_, _ = tx.Exec(`UPDATE source_ad_slot_days SET locked = GREATEST(locked - 1, 0) WHERE slot_id=? AND day_date=?`,
			order.SlotID, day.Format("2006-01-02"))
	}
	_, err = tx.Exec(`UPDATE source_ad_orders SET status=? WHERE id=? AND status=?`, adOrderExpiredLock, order.ID, adOrderPendingPay)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func expandAdOrderDays(order adOrder) []time.Time {
	start, err1 := time.ParseInLocation("2006-01-02", order.DayStart, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02", order.DayEnd, time.Local)
	if err1 != nil || err2 != nil {
		return nil
	}
	out := make([]time.Time, 0, order.Days)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
		if len(out) >= order.Days && order.Days > 0 {
			break
		}
	}
	if order.Days > 0 && len(out) > order.Days {
		out = out[:order.Days]
	}
	return out
}

// settleAdOrder 支付成功：锁转占用 → 自动检查 → 排期或人工复核。无退款。
func settleAdOrder(db *sql.DB, orderNo string, paidCents int64, channel, payMethod, tradeNo, payload string) error {
	if err := ensureAdOrderSchema(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var o adOrder
	var lockUntil sql.NullTime
	err = tx.QueryRow(`SELECT id, order_no, slot_id, title, description, image_url, link_url, days, day_start, day_end, amount_cents, status
		FROM source_ad_orders WHERE order_no=? FOR UPDATE`, orderNo).
		Scan(&o.ID, &o.OrderNo, &o.SlotID, &o.Title, &o.Description, &o.ImageURL, &o.LinkURL, &o.Days, &o.DayStart, &o.DayEnd, &o.AmountCents, &o.Status)
	if err != nil {
		return err
	}
	_ = lockUntil
	if o.Status == adOrderChecking || o.Status == adOrderScheduled || o.Status == adOrderManualReview ||
		o.Status == adOrderRunning || o.Status == adOrderNeedEdit {
		return tx.Commit()
	}
	if o.Status != adOrderPendingPay {
		return errors.New("广告订单状态不可入账")
	}
	if o.AmountCents != paidCents {
		return errors.New("广告订单金额不一致")
	}
	days := expandAdOrderDays(o)
	for _, day := range days {
		_, err = tx.Exec(`UPDATE source_ad_slot_days SET locked = GREATEST(locked - 1, 0), reserved = reserved + 1 WHERE slot_id=? AND day_date=?`,
			o.SlotID, day.Format("2006-01-02"))
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE source_ad_orders SET status=?, pay_channel=?, pay_method=?, gateway_trade_no=?, notify_payload=?, paid_at=NOW()
		WHERE id=?`, adOrderChecking, channel, payMethod, tradeNo, payload, o.ID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	go runAdOrderAutoCheck(orderNo)
	return nil
}

type adCheckItem struct {
	Key     string `json:"key"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func runAdOrderAutoCheck(orderNo string) {
	db, err := openSystemConfigDB()
	if err != nil {
		return
	}
	order, err := loadAdOrder(db, orderNo)
	if err != nil || order.Status != adOrderChecking {
		return
	}
	items := []adCheckItem{}
	linkOK := validateAdLinkURL(order.LinkURL) == nil
	items = append(items, adCheckItem{Key: "link", OK: linkOK, Message: map[bool]string{true: "https 链接格式正确", false: "链接须为 https://"}[linkOK]})
	titleOK := order.Title != "" && utf8.RuneCountInString(order.Title) <= 20
	items = append(items, adCheckItem{Key: "title", OK: titleOK, Message: map[bool]string{true: "标题长度合格", false: "标题不合格"}[titleOK]})
	descOK := utf8.RuneCountInString(order.Description) <= 60
	items = append(items, adCheckItem{Key: "desc", OK: descOK, Message: map[bool]string{true: "简介长度合格", false: "简介过长"}[descOK]})
	banned := containsAdBannedWords(order.Title + " " + order.Description)
	items = append(items, adCheckItem{Key: "words", OK: !banned, Message: map[bool]string{true: "文案无违禁词", false: "文案含绝对化/违禁用语"}[!banned]})
	quotaOK := order.Days > 0
	items = append(items, adCheckItem{Key: "quota", OK: quotaOK, Message: map[bool]string{true: "名额与天数有效", false: "天数无效"}[quotaOK]})
	allOK := true
	for _, it := range items {
		if !it.OK {
			allOK = false
			break
		}
	}
	raw, _ := json.Marshal(items)
	if allOK {
		_, _ = db.Exec(`UPDATE source_ad_orders SET status=?, check_result=? WHERE order_no=? AND status=?`,
			adOrderScheduled, string(raw), orderNo, adOrderChecking)
		_ = materializeAdOrderAdvertisement(db, orderNo)
	} else {
		_, _ = db.Exec(`UPDATE source_ad_orders SET status=?, check_result=? WHERE order_no=? AND status=?`,
			adOrderManualReview, string(raw), orderNo, adOrderChecking)
	}
}

func containsAdBannedWords(text string) bool {
	lower := strings.ToLower(text)
	for _, w := range []string{"最强", "第一", "绝对", "国家级", "保证赚钱"} {
		if strings.Contains(lower, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

func materializeAdOrderAdvertisement(db *sql.DB, orderNo string) error {
	order, err := loadAdOrder(db, orderNo)
	if err != nil {
		return err
	}
	if order.AdvertisementID != "" {
		return nil
	}
	id, err := generateAdvertisementID()
	if err != nil {
		return err
	}
	startAt := order.DayStart + "T00:00:00+08:00"
	endAt := order.DayEnd + "T23:59:59+08:00"
	record := advertisementRecord{
		ID:             id,
		Title:          order.Title,
		ImageURL:       order.ImageURL,
		DestinationURL: order.LinkURL,
		Positions:      []string{order.SlotID},
		Weight:         10,
		StartAt:        startAt,
		EndAt:          endAt,
		Description:    order.Description,
	}
	if len(prepareAdvertisementForStore(&record)) == 0 {
		return errors.New("广告位不合法")
	}
	if err := currentSourceStationStore().UpsertAdvertisement(record); err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE source_ad_orders SET advertisement_id=?, status=? WHERE order_no=? AND status IN (?, ?)`,
		id, adOrderScheduled, orderNo, adOrderScheduled, adOrderChecking)
	invalidateAllAdvertisementCaches()
	return err
}

// ClientAdOrderList 我的投放。
func ClientAdOrderList(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	if err := ensureAdOrderSchema(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化失败"})
		return
	}
	rows, err := db.Query(`SELECT order_no, slot_id, title, days, day_start, day_end, amount_cents, status, review_note, created_at
		FROM source_ad_orders ORDER BY id DESC LIMIT 100`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询失败"})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var orderNo, slotID, title, dayStart, dayEnd, status, reviewNote string
		var days int
		var amount int64
		var created time.Time
		if err := rows.Scan(&orderNo, &slotID, &title, &days, &dayStart, &dayEnd, &amount, &status, &reviewNote, &created); err != nil {
			continue
		}
		name := slotID
		if def, ok := adSlotDefOf(slotID); ok {
			name = def.Name
		}
		list = append(list, gin.H{
			"orderNo": orderNo, "slotId": slotID, "slotName": name, "title": title, "days": days,
			"dayStart": dayStart, "dayEnd": dayEnd, "amountCents": amount, "amount": formatCents(amount),
			"status": status, "statusLabel": adOrderStatusLabel(status), "reviewNote": reviewNote,
			"createdAt": created.Format(time.RFC3339), "noRefund": true,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list), "noRefundNotice": adNoRefundNotice}})
}

func adOrderStatusLabel(status string) string {
	switch status {
	case adOrderPendingPay:
		return "待付款"
	case adOrderChecking:
		return "已付款·检查中"
	case adOrderScheduled:
		return "排期中"
	case adOrderManualReview:
		return "待人工复核"
	case adOrderNeedEdit:
		return "待修改"
	case adOrderRunning:
		return "投放中"
	case adOrderPaused:
		return "已暂停"
	case adOrderEnded:
		return "已结束"
	case adOrderVoided:
		return "已作废"
	case adOrderExpiredLock:
		return "锁定过期"
	default:
		return status
	}
}

type adOrderEditRequest struct {
	OrderNo     string `json:"orderNo"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"imageUrl"`
	LinkURL     string `json:"linkUrl"`
}

// ClientAdOrderResubmit 审核不过：改完再交，天数保留，不退款。
func ClientAdOrderResubmit(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	var req adOrderEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	order, err := loadAdOrder(db, strings.TrimSpace(req.OrderNo))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if order.Status != adOrderNeedEdit {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "仅待修改订单可重新提交"})
		return
	}
	title := strings.TrimSpace(req.Title)
	desc := strings.TrimSpace(req.Description)
	link := strings.TrimSpace(req.LinkURL)
	image := strings.TrimSpace(req.ImageURL)
	if title == "" || utf8.RuneCountInString(title) > 20 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "标题必填且不超过 20 字"})
		return
	}
	if err := validateAdLinkURL(link); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	_, err = db.Exec(`UPDATE source_ad_orders SET title=?, description=?, image_url=?, link_url=?, status=?, review_note='' WHERE order_no=?`,
		title, desc, image, link, adOrderChecking, order.OrderNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存失败"})
		return
	}
	go runAdOrderAutoCheck(order.OrderNo)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已重新提交检查（不退款，天数保留）", "data": gin.H{"orderNo": order.OrderNo, "status": adOrderChecking}})
}

// AdminAdAuditQueue 官网审核队列。
func AdminAdAuditQueue(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	if err := ensureAdOrderSchema(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化失败"})
		return
	}
	tab := strings.TrimSpace(c.Query("tab"))
	var statusFilter string
	switch tab {
	case "auto", "checking":
		statusFilter = adOrderChecking
	case "manual", "manual_review":
		statusFilter = adOrderManualReview
	case "done":
		statusFilter = ""
	default:
		statusFilter = adOrderManualReview
	}
	q := `SELECT order_no, buyer_name, slot_id, title, description, image_url, link_url, days, day_start, day_end, amount_cents, status, IFNULL(check_result,''), review_note, created_at
		FROM source_ad_orders`
	args := []any{}
	if statusFilter != "" {
		q += ` WHERE status=?`
		args = append(args, statusFilter)
	} else {
		q += ` WHERE status IN (?,?,?,?,?,?)`
		args = append(args, adOrderScheduled, adOrderNeedEdit, adOrderRunning, adOrderPaused, adOrderEnded, adOrderVoided)
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := db.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询失败"})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var orderNo, buyer, slotID, title, desc, image, link, dayStart, dayEnd, status, checkResult, reviewNote string
		var days int
		var amount int64
		var created time.Time
		if err := rows.Scan(&orderNo, &buyer, &slotID, &title, &desc, &image, &link, &days, &dayStart, &dayEnd, &amount, &status, &checkResult, &reviewNote, &created); err != nil {
			continue
		}
		name := slotID
		if def, ok := adSlotDefOf(slotID); ok {
			name = def.Name
		}
		var checks []adCheckItem
		_ = json.Unmarshal([]byte(checkResult), &checks)
		list = append(list, gin.H{
			"orderNo": orderNo, "buyerName": buyer, "slotId": slotID, "slotName": name,
			"title": title, "description": desc, "imageUrl": image, "linkUrl": link,
			"days": days, "dayStart": dayStart, "dayEnd": dayEnd, "amountCents": amount, "amount": formatCents(amount),
			"status": status, "statusLabel": adOrderStatusLabel(status), "checks": checks, "reviewNote": reviewNote,
			"createdAt": created.Format(time.RFC3339), "noRefund": true,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list), "noRefundNotice": adNoRefundNotice}})
}

type adAuditDecisionRequest struct {
	OrderNo string `json:"orderNo"`
	Reason  string `json:"reason"`
	Note    string `json:"note"`
}

// AdminAdAuditApprove 通过（无退款按钮）。
func AdminAdAuditApprove(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	var req adAuditDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	order, err := loadAdOrder(db, strings.TrimSpace(req.OrderNo))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if order.Status != adOrderManualReview && order.Status != adOrderChecking {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "当前状态不可通过"})
		return
	}
	reviewer := c.GetString("username")
	_, err = db.Exec(`UPDATE source_ad_orders SET status=?, reviewed_by=?, review_note=? WHERE order_no=?`,
		adOrderScheduled, reviewer, strings.TrimSpace(req.Note), order.OrderNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败"})
		return
	}
	_ = materializeAdOrderAdvertisement(db, order.OrderNo)
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: reviewer, Action: "ad_audit_approve",
		TargetType: "ad_order", TargetID: order.OrderNo, Detail: "通过投放，无退款",
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已通过并排期", "data": gin.H{"orderNo": order.OrderNo, "status": adOrderScheduled}})
}

// AdminAdAuditReturn 退回修改（不退款，天数保留）。
func AdminAdAuditReturn(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	var req adAuditDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	order, err := loadAdOrder(db, strings.TrimSpace(req.OrderNo))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if order.Status != adOrderManualReview && order.Status != adOrderChecking {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "当前状态不可退回"})
		return
	}
	note := strings.TrimSpace(req.Reason)
	if n := strings.TrimSpace(req.Note); n != "" {
		if note != "" {
			note += "；"
		}
		note += n
	}
	if note == "" {
		note = "请修改后重新提交"
	}
	reviewer := c.GetString("username")
	_, err = db.Exec(`UPDATE source_ad_orders SET status=?, reviewed_by=?, review_note=? WHERE order_no=?`,
		adOrderNeedEdit, reviewer, note, order.OrderNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败"})
		return
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: reviewer, Action: "ad_audit_return",
		TargetType: "ad_order", TargetID: order.OrderNo, Detail: "退回修改，不退款：" + note,
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已退回修改（不退款，天数保留）", "data": gin.H{"orderNo": order.OrderNo, "status": adOrderNeedEdit}})
}

// AdminAdOrderVoid 违规下线：剩余天数作废，不退款。
func AdminAdOrderVoid(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	var req adAuditDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	order, err := loadAdOrder(db, strings.TrimSpace(req.OrderNo))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = strings.TrimSpace(req.Note)
	}
	if reason == "" {
		reason = "违规下线"
	}
	if order.AdvertisementID != "" {
		_ = currentSourceStationStore().DeleteAdvertisement(order.AdvertisementID)
	}
	reviewer := c.GetString("username")
	_, err = db.Exec(`UPDATE source_ad_orders SET status=?, void_reason=?, reviewed_by=? WHERE order_no=?`,
		adOrderVoided, reason, reviewer, order.OrderNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败"})
		return
	}
	invalidateAllAdvertisementCaches()
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: reviewer, Action: "ad_order_void",
		TargetType: "ad_order", TargetID: order.OrderNo, Detail: "违规作废剩余天数，不退款：" + reason,
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已作废剩余天数（不退款）", "data": gin.H{"orderNo": order.OrderNo, "status": adOrderVoided}})
}

// ClientAdCalendar 月历名额。
func ClientAdCalendar(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	loadAdSlotSwitchesFromStore()
	slotID := canonicalizeAdSlot(c.Query("slotId"))
	def, ok := adSlotDefOf(slotID)
	if !ok || !def.Sellable {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告位不可售"})
		return
	}
	month := strings.TrimSpace(c.Query("month")) // YYYY-MM
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", month, time.Local)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "月份格式错误"})
		return
	}
	end := start.AddDate(0, 1, 0)
	days := []gin.H{}
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		left, _ := adSlotRemaining(db, slotID, d)
		days = append(days, gin.H{
			"date": d.Format("2006-01-02"),
			"left": left,
			"full": left <= 0 || !isAdSlotEnabled(slotID),
			"priceCents": def.PriceCents,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"slotId": slotID, "month": month, "capacity": def.Capacity, "days": days}})
}

// ClientAdPayOptions 广告结账可用支付方式（仅线上，无余额）。
func ClientAdPayOptions(c *gin.Context) {
	db, err := openSystemConfigDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库不可用"})
		return
	}
	options := configuredOnlinePayOptions(db)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": options, "noRefundNotice": adNoRefundNotice}})
}
