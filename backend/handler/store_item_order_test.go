package handler

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestBuyerStoreOrderBody(t *testing.T) {
	body, err := buyerStoreOrderBody(0, "plugin", " epay ", "easypay:alipay")
	if err != nil || body["itemKind"] != "plugin" || body["itemId"] != "epay" || body["payMethod"] != "easypay:alipay" {
		t.Fatalf("插件单品 = %#v err=%v", body, err)
	}
	body, err = buyerStoreOrderBody(0, "template", "gold", "")
	if err != nil || body["itemKind"] != "template" || body["itemId"] != "gold" || body["payMethod"] != nil {
		t.Fatalf("模板单品 = %#v err=%v", body, err)
	}
	if _, err := buyerStoreOrderBody(0, "plugin", " ", ""); err == nil {
		t.Fatal("空插件编号应拒绝")
	}
	body, err = buyerStoreOrderBody(8, "", "", "easypay:wxpay")
	if err != nil || body["itemKind"] != "edition" || body["planId"] != int64(8) || body["payMethod"] != "easypay:wxpay" {
		t.Fatalf("商业版 = %#v err=%v", body, err)
	}
	if _, err := buyerStoreOrderBody(0, "edition", "", ""); err == nil {
		t.Fatal("未选套餐应拒绝")
	}
}

func TestCatalogEntryPurchaseOnly(t *testing.T) {
	paid := sourcePublicPluginEntry(sourcePlugin{
		ID: "dev-pay", DeveloperID: 4, Name: "开发者插件", Version: "1.0.0",
		PriceCents: 9900, Billing: sourceBillingOneTime, Status: sourceItemPublished,
	})
	if paid["purchaseOnly"] != true {
		t.Fatalf("开发者付费条目应仅单买: %#v", paid)
	}
	official := sourcePublicPluginEntry(sourcePlugin{
		ID: "epay", Name: "易支付", Version: "1.0.0",
		PriceCents: 9900, Billing: sourceBillingOneTime, Status: sourceItemPublished,
	})
	if official["purchaseOnly"] != false {
		t.Fatalf("官方付费条目默认可被商业版包含: %#v", official)
	}
	free := sourcePublicPluginEntry(sourcePlugin{ID: "free", Name: "免费", PriceCents: 0})
	if free["purchaseOnly"] != false {
		t.Fatalf("免费条目不是仅单买: %#v", free)
	}
}

func TestStoreItemPurchaseMariaDB(t *testing.T) {
	control := openAppUpdateControlDB(t)
	defer control.Close()
	databaseName := "authpro_item_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()
	if err := createCommercialGapSchema(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE plugin_entitlements
		ADD COLUMN order_id BIGINT UNSIGNED DEFAULT NULL,
		ADD COLUMN owner_type VARCHAR(20) NOT NULL DEFAULT 'user',
		ADD COLUMN owner_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
		ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'purchase'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE source_catalog_plugins (
		id VARCHAR(60) NOT NULL PRIMARY KEY,
		developer_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
		name VARCHAR(100) NOT NULL,
		version VARCHAR(40) NOT NULL DEFAULT '',
		price_cents BIGINT NOT NULL DEFAULT 0,
		billing VARCHAR(20) NOT NULL DEFAULT 'free',
		status VARCHAR(20) NOT NULL DEFAULT 'draft'
	)`); err != nil {
		t.Fatal(err)
	}
	markCommercialGapMigrations(t, db)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restoreKeys := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restoreKeys)
	gin.SetMode(gin.TestMode)

	previousPay := createStorePayment
	createStorePayment = func(_ *gin.Context, _ *sql.DB, _ string, _ int64, _ string) (string, string, string, error) {
		return "https://pay.example.com/item", "stub", "alipay", nil
	}
	t.Cleanup(func() { createStorePayment = previousPay })

	now := time.Now().Add(-time.Hour)
	if _, err := db.Exec(`INSERT INTO apps (id, app_name, app_key, enabled) VALUES (1, '站点', 'site-app', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, nickname, balance, enabled) VALUES (7, 'buyer@example.com', 'x', '买家', 0, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO licenses
		(id, license_no, app_id, type, status, source, owner_type, owner_id, duration_days, started_at, max_domains)
		VALUES (100, 'LIC-ITEM', 1, 'domain', 'active', 'store_bind', 'user', 7, 0, ?, 0)`, now); err != nil {
		t.Fatal(err)
	}
	salt := []byte("salt-item-32-bytes-padding-ok!!")
	const bindingID = "sb_item"
	if _, err := db.Exec(`INSERT INTO store_bindings
		(binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, status)
		VALUES (?, ?, 'user', 7, 100, 'shop.example.com', 'active')`, bindingID, salt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO source_catalog_plugins (id, developer_id, name, version, price_cents, billing, status) VALUES
		('epay', 0, '易支付', '1.0.0', 9900, 'one_time', 'published'),
		('dev-extra', 8, '开发者插件', '1.0.0', 5000, 'one_time', 'published')`); err != nil {
		t.Fatal(err)
	}
	savePaidCatalog([]paidCatalogItem{
		{Kind: "plugin", ID: "epay", Name: "易支付", PriceCents: 9900, Billing: "one_time"},
		{Kind: "plugin", ID: "dev-extra", Name: "开发者插件", PriceCents: 5000, Billing: "one_time", PurchaseOnly: true},
	})

	before, err := buildStoreSnapshot(db, bindingID, 100, "LIC-ITEM", "shop.example.com", sourceStoreSettings{GraceDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveBuyerSnapshot(buyerSnapshotState{
		Snapshot: before, VerifiedAt: time.Now().Unix(), LastRefreshOK: true,
		GraceUntil: time.Now().Add(7 * 24 * time.Hour).Unix(), BindingID: bindingID, LicenseNo: "LIC-ITEM",
	}); err != nil {
		t.Fatal(err)
	}
	denied := callPluginToggle(t, "epay", true)
	if jsonCode(denied) != 402 || denied["msg"] != "该插件需要购买后才能启用" {
		t.Fatalf("未购买应拒绝启用: %#v", denied)
	}
	data, _ := denied["data"].(map[string]any)
	if data["kind"] != "plugin" || data["id"] != "epay" || int64(data["priceCents"].(float64)) != 9900 || data["purchaseOnly"] != false || data["period"] != "permanent" {
		t.Fatalf("402 缺少条目信息: %#v", data)
	}

	created := callSignedStore(t, http.MethodPost, "/api/v1/store/orders", []byte(`{"itemKind":"plugin","itemId":"epay"}`), bindingID, salt, "")
	if jsonCode(created) != 200 {
		t.Fatalf("创建单品订单失败: %#v", created)
	}
	orderData, _ := created["data"].(map[string]any)
	orderNo, _ := orderData["orderNo"].(string)
	if orderNo == "" || orderData["payUrl"] != "https://pay.example.com/item" || int64(orderData["amountCents"].(float64)) != 9900 {
		t.Fatalf("订单响应不正确: %#v", orderData)
	}
	if err := settleStorePurchaseOrder(db, orderNo, 9900, "stub", "alipay", "trade-item", "notify"); err != nil {
		t.Fatal(err)
	}
	var entitled int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_entitlements WHERE license_id = 100 AND item_kind = 'plugin' AND item_id = 'epay' AND status = 'active'`).Scan(&entitled); err != nil || entitled != 1 {
		t.Fatalf("权益行 = %d err=%v", entitled, err)
	}
	refreshed := callSignedStore(t, http.MethodGet, "/api/v1/store/orders/"+orderNo, nil, bindingID, salt, orderNo)
	if jsonCode(refreshed) != 200 {
		t.Fatalf("查单失败: %#v", refreshed)
	}
	refreshData, _ := refreshed["data"].(map[string]any)
	if refreshData["status"] != "paid" {
		t.Fatalf("查单未支付: %#v", refreshData)
	}
	snapRaw, _ := refreshData["snapshot"].(map[string]any)
	if err := saveSnapshotMap(snapRaw, true, false, "买家\nuser"); err != nil {
		t.Fatal(err)
	}
	enabled := callPluginToggle(t, "epay", true)
	if jsonCode(enabled) != 200 {
		t.Fatalf("购买后启用失败: %#v", enabled)
	}

	if _, err := db.Exec(`INSERT INTO main_license_editions (license_id, edition, period, started_at, status) VALUES (100, 'commercial', 'permanent', ?, 'active')`, now); err != nil {
		t.Fatal(err)
	}
	commercialSnap, err := buildStoreSnapshot(db, bindingID, 100, "LIC-ITEM", "shop.example.com", sourceStoreSettings{GraceDays: 7, CommercialFeatures: []string{"multi_app"}})
	if err != nil || !commercialSnap.AllPaidItems {
		t.Fatalf("商业版快照不正确: %+v err=%v", commercialSnap, err)
	}
	if err := saveBuyerSnapshot(buyerSnapshotState{
		Snapshot: commercialSnap, VerifiedAt: time.Now().Unix(), LastRefreshOK: true,
		GraceUntil: time.Now().Add(7 * 24 * time.Hour).Unix(), BindingID: bindingID, LicenseNo: "LIC-ITEM",
	}); err != nil {
		t.Fatal(err)
	}
	blocked := callPluginGate(t, "dev-extra")
	if jsonCode(blocked) != 402 || blocked["msg"] != "该插件需要购买后才能启用" {
		t.Fatalf("商业版仍应拦住仅单买: %#v", blocked)
	}
	blockedData, _ := blocked["data"].(map[string]any)
	if blockedData["purchaseOnly"] != true || blockedData["id"] != "dev-extra" || int64(blockedData["priceCents"].(float64)) != 5000 {
		t.Fatalf("仅单买 402 不正确: %#v", blockedData)
	}
}

func callSignedStore(t *testing.T, method, path string, body []byte, bindingID string, salt []byte, orderNo string) map[string]any {
	t.Helper()
	master, err := loadOrCreateStoreFileKey("binding-master.key")
	if err != nil {
		t.Fatal(err)
	}
	secret := deriveBindingSecret(master, bindingID, salt)
	ts := time.Now().Unix()
	nonce := randomHex(8)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "shop.example.com"
	req.Header.Set("X-Store-Binding", bindingID)
	req.Header.Set("X-Store-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Store-Nonce", nonce)
	req.Header.Set("X-Store-Signature", storeRequestSignature(secret, method, path, ts, nonce, body))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	if orderNo != "" {
		c.Params = gin.Params{{Key: "orderNo", Value: orderNo}}
		StoreOrderQuery(c)
	} else {
		StoreOrderCreate(c)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.Bytes())
	}
	return payload
}

func callPluginToggle(t *testing.T, id string, enabled bool) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"enabled": enabled})
	req := httptest.NewRequest(http.MethodPost, "/api/system/plugins/"+id+"/toggle", bytes.NewReader(body))
	req.Host = "shop.example.com"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: id}}
	AdminPluginToggle(c)
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("启用响应不是 JSON: %s", w.Body.Bytes())
	}
	return payload
}

func callPluginGate(t *testing.T, id string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/plugins/"+id, strings.NewReader(`{"enabled":true}`))
	req.Host = "shop.example.com"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	if !rejectPaidPluginEnable(c, id) {
		t.Fatal("仅单买条目不应被商业版放行")
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("402 不是 JSON: %s", w.Body.Bytes())
	}
	return payload
}
