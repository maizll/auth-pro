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
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestCommercialPurchaseMariaDB(t *testing.T) {
	control := openAppUpdateControlDB(t)
	defer control.Close()
	databaseName := "authpro_cg_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()
	if err := createCommercialGapSchema(db); err != nil {
		t.Fatal(err)
	}
	markCommercialGapMigrations(t, db)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restoreKeys := useStoreSnapshotKeysForTest(pub, priv)
	t.Cleanup(restoreKeys)
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })

	if _, err := db.Exec(`INSERT INTO apps (id, app_name, app_key, enabled, commercial_product, purchase_license_type_mask) VALUES
		(1, '普通应用', 'plain-app', 1, 0, 15),
		(2, '商业版', 'commercial-app', 1, 1, 15)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO license_plans (id, app_id, name, license_type, duration_days, price, enabled) VALUES
		(11, 1, '普通年付', '', 365, 10, 1),
		(22, 2, '永久商业版', '', 0, 199, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, balance, enabled) VALUES (7, 'buyer@example.com', 'x', 500, 1)`); err != nil {
		t.Fatal(err)
	}

	usePurchaseLicenseTypeTestHooks(t, db)
	gin.SetMode(gin.TestMode)

	listBody := callCommercialJSON(t, http.MethodGet, "/apps/purchase", nil, func(c *gin.Context) {
		c.Set("role", "user")
		c.Set("user_id", uint(7))
		UserAppListForPurchase(c)
	})
	if jsonCode(listBody) != 200 {
		t.Fatalf("购买页失败: %s", listBody)
	}
	rawApps, _ := json.Marshal(listBody["data"])
	if bytes.Contains(rawApps, []byte("商业版")) || bytes.Contains(rawApps, []byte("永久商业版")) {
		t.Fatalf("用户端购买页列出了商业版套餐: %s", rawApps)
	}
	if !bytes.Contains(rawApps, []byte("普通应用")) {
		t.Fatalf("用户端购买页缺少普通应用: %s", rawApps)
	}

	rejectBody := callCommercialJSON(t, http.MethodPost, "/purchase", map[string]any{
		"appId": 2, "planId": 22, "type": "domain", "domain": "shop.example.com", "payMethod": "balance",
	}, func(c *gin.Context) {
		c.Set("role", "user")
		c.Set("user_id", uint(7))
		UserPurchase(c)
	})
	msg, _ := rejectBody["msg"].(string)
	if jsonCode(rejectBody) != 400 || msg != commercialOrdinaryPurchaseRejected {
		t.Fatalf("下单拒绝不符合预期: %v", rejectBody)
	}

	now := time.Now().Add(-time.Hour)
	if _, err := db.Exec(`INSERT INTO licenses
		(id, license_no, app_id, plan_id, type, status, source, owner_type, owner_id, duration_days, started_at, max_domains)
		VALUES (100, 'LIC-GAP', 2, 22, 'domain', 'active', 'user_purchase', 'user', 7, 0, ?, 0)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO license_domains (license_id, domain, is_wildcard) VALUES (100, 'shop.example.com', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO license_purchase_orders
		(order_no, agent_id, user_id, app_id, plan_id, owner_type, owner_id, type, target, amount,
		 app_name_snapshot, plan_name_snapshot, duration_days_snapshot, status, license_id, license_no, paid_at)
		VALUES ('U-GAP', 0, 7, 2, 22, 'user', 7, 'domain', 'shop.example.com', 199,
		 '商业版', '永久商业版', 0, 'paid', 100, 'LIC-GAP', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO store_bindings
		(binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, status)
		VALUES ('sb_gap', ?, 'user', 7, 100, 'shop.example.com', 'active')`, []byte("salt-gap-32-bytes-padding-ok!!")); err != nil {
		t.Fatal(err)
	}

	before, bound, err := signedSnapshotForLicense(db, 100)
	if err != nil || !bound || before.Edition != storeEditionFree {
		t.Fatalf("补发前买家快照应是免费版, edition=%s bound=%v err=%v", before.Edition, bound, err)
	}

	gaps, err := listCommercialPurchaseGaps(db)
	if err != nil || len(gaps) != 1 || gaps[0].OrderNo != "U-GAP" {
		t.Fatalf("缺口订单 = %#v err=%v", gaps, err)
	}
	granted, already, err := reissueCommercialPurchaseGaps(db)
	if err != nil || granted != 1 || already != 0 {
		t.Fatalf("首次补发 granted=%d already=%d err=%v", granted, already, err)
	}
	after, bound, err := signedSnapshotForLicense(db, 100)
	if err != nil || !bound || after.Edition != storeEditionCommercial || !after.AllPaidItems || !verifyStoreSnapshot(after) {
		t.Fatalf("补发后快照未解锁商业版: %+v bound=%v err=%v", after, bound, err)
	}
	if !snapshotHasFeature(after, storeFeatureMultiApp) || !appCreateDecision(1, true) {
		t.Fatal("买家站点仍不能创建第二个应用")
	}
	var editionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM main_license_editions WHERE license_id = 100 AND status = 'active'`).Scan(&editionCount); err != nil {
		t.Fatal(err)
	}
	granted, already, err = reissueCommercialPurchaseGaps(db)
	if err != nil || granted != 0 || already != 0 {
		t.Fatalf("重复补发应为空缺口 granted=%d already=%d err=%v", granted, already, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	created, err := grantCommercialEditionTx(tx, commercialEditionGrant{LicenseID: 100, Period: storePeriodPermanent})
	if err != nil || created {
		_ = tx.Rollback()
		t.Fatalf("直接再开通应跳过 created=%v err=%v", created, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var editionCountAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM main_license_editions WHERE license_id = 100`).Scan(&editionCountAfter); err != nil {
		t.Fatal(err)
	}
	if editionCountAfter != editionCount {
		t.Fatalf("重复补发新增了权益行 %d -> %d", editionCount, editionCountAfter)
	}

	if _, err := db.Exec(`INSERT INTO licenses
		(id, license_no, app_id, plan_id, type, status, source, owner_type, owner_id, duration_days, started_at, max_domains)
		VALUES (200, 'LIC-STORE', 2, 22, 'domain', 'active', 'store_bind', 'user', 7, 0, ?, 0)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO store_bindings
		(binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, status)
		VALUES ('sb_store', ?, 'user', 7, 200, 'buyer.example.com', 'active')`, []byte("salt-store-32-bytes-padding!!")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO store_purchase_orders
		(order_no, owner_type, owner_id, license_id, binding_id, item_kind, item_id, period, amount_cents, price_cents_snapshot, title_snapshot, status, expires_at)
		VALUES ('PP-STORE', 'user', 7, 200, 'sb_store', 'edition', '22', 'permanent', 19900, 19900, '永久商业版', 'pending', ?)`,
		time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := settleStorePurchaseOrder(db, "PP-STORE", 19900, "epay_v1", "alipay", "trade-store", "notify"); err != nil {
		t.Fatal(err)
	}
	storeSnap, bound, err := signedSnapshotForLicense(db, 200)
	if err != nil || !bound || storeSnap.Edition != storeEditionCommercial || !storeSnap.AllPaidItems || !verifyStoreSnapshot(storeSnap) {
		t.Fatalf("顶栏订单付款后买家未解锁: %+v bound=%v err=%v", storeSnap, bound, err)
	}
	if !snapshotHasFeature(storeSnap, storeFeatureMultiApp) {
		t.Fatalf("顶栏订单快照缺少多应用权益: %+v", storeSnap.Features)
	}
}

func snapshotHasFeature(snapshot storeSnapshot, feature string) bool {
	for _, item := range snapshot.Features {
		if item == feature {
			return true
		}
	}
	return snapshot.AllPaidItems
}

func callCommercialJSON(t *testing.T, method, target string, payload any, handler func(*gin.Context)) map[string]any {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("响应不是 JSON: %s", recorder.Body.String())
	}
	return decoded
}

func markCommercialGapMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	names := []string{
		sourceMigrationCatalogAdditive,
		sourceMigrationPluginFilePath,
		sourceMigrationPluginPublished,
		sourceMigrationTemplateLegacy,
		sourceMigrationVersionBackfill,
		sourceMigrationAppIDBackfill,
		sourceMigrationDeveloperAgent,
		sourceMigrationCatalogPrice,
		sourceMigrationCatalogOrigin,
		sourceMigrationPaidExternal,
		sourceMigrationVersionStorage,
		storeMigrationBindings,
		storeMigrationEditions,
		storeMigrationOrders,
		storeMigrationEntitlements,
		storeMigrationRevenue,
		storeMigrationLicenseSource,
		storeMigrationLicenseSourcePurchase,
		storeMigrationDomainChanges,
		storeEditionPlansMigration,
	}
	for _, name := range names {
		if _, err := db.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			t.Fatal(err)
		}
	}
}

func createCommercialGapSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE schema_migrations (
			name VARCHAR(100) NOT NULL PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE apps (
			id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
			app_name VARCHAR(100) NOT NULL,
			app_key VARCHAR(64) NOT NULL,
			enabled TINYINT(1) NOT NULL DEFAULT 1,
			commercial_product TINYINT(1) NOT NULL DEFAULT 0,
			purchase_license_type_mask TINYINT UNSIGNED NOT NULL DEFAULT 15,
			description VARCHAR(255) NOT NULL DEFAULT '',
			icon VARCHAR(255) NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE license_plans (
			id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
			app_id BIGINT UNSIGNED NOT NULL,
			name VARCHAR(100) NOT NULL,
			license_type VARCHAR(20) NOT NULL DEFAULT '',
			duration_days INT NOT NULL DEFAULT 0,
			price DECIMAL(12,2) NOT NULL DEFAULT 0,
			max_sites INT NOT NULL DEFAULT 0,
			sort INT NOT NULL DEFAULT 0,
			enabled TINYINT(1) NOT NULL DEFAULT 1,
			remark VARCHAR(255) NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_app_plan_name (app_id, name, license_type)
		)`,
		`CREATE TABLE users (
			id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
			email VARCHAR(100) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			nickname VARCHAR(100) NOT NULL DEFAULT '',
			balance DECIMAL(12,2) NOT NULL DEFAULT 0,
			enabled TINYINT(1) NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE licenses (
			id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
			license_no VARCHAR(64) NOT NULL,
			app_id BIGINT UNSIGNED NOT NULL,
			plan_id BIGINT UNSIGNED DEFAULT NULL,
			type VARCHAR(20) NOT NULL,
			status VARCHAR(20) NOT NULL,
			source VARCHAR(40) NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT UNSIGNED NOT NULL,
			duration_days INT NOT NULL DEFAULT 0,
			started_at DATETIME NOT NULL,
			expired_at DATETIME DEFAULT NULL,
			license_key VARCHAR(128) NOT NULL DEFAULT '',
			max_domains INT NOT NULL DEFAULT 0,
			remark VARCHAR(255) NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE license_domains (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			license_id BIGINT UNSIGNED NOT NULL,
			domain VARCHAR(255) NOT NULL,
			is_wildcard TINYINT(1) NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE license_purchase_orders (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			order_no VARCHAR(64) NOT NULL,
			agent_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			user_id BIGINT UNSIGNED DEFAULT NULL,
			app_id BIGINT UNSIGNED NOT NULL,
			plan_id BIGINT UNSIGNED NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT UNSIGNED NOT NULL,
			type VARCHAR(20) NOT NULL,
			target VARCHAR(255) NOT NULL DEFAULT '',
			amount DECIMAL(12,2) NOT NULL,
			app_name_snapshot VARCHAR(100) NOT NULL DEFAULT '',
			plan_name_snapshot VARCHAR(100) NOT NULL DEFAULT '',
			duration_days_snapshot INT DEFAULT NULL,
			status VARCHAR(20) NOT NULL,
			license_id BIGINT UNSIGNED DEFAULT NULL,
			license_no VARCHAR(64) NOT NULL DEFAULT '',
			paid_at DATETIME DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE main_license_editions (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			license_id BIGINT UNSIGNED NOT NULL,
			edition VARCHAR(20) NOT NULL,
			period VARCHAR(20) NOT NULL,
			started_at DATETIME NOT NULL,
			expires_at DATETIME DEFAULT NULL,
			status VARCHAR(20) NOT NULL,
			order_id BIGINT UNSIGNED DEFAULT NULL,
			granted_by BIGINT UNSIGNED DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT NULL,
			KEY idx_main_license_edition (license_id, status)
		)`,
		`CREATE TABLE store_bindings (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			binding_id VARCHAR(64) NOT NULL,
			secret_salt VARBINARY(64) NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT UNSIGNED NOT NULL,
			license_id BIGINT UNSIGNED NOT NULL,
			domain_snapshot VARCHAR(255) NOT NULL,
			install_id VARCHAR(64) NOT NULL DEFAULT '',
			app_version VARCHAR(40) NOT NULL DEFAULT '',
			last_ip VARCHAR(64) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at DATETIME DEFAULT NULL,
			revoked_at DATETIME DEFAULT NULL,
			revoke_reason VARCHAR(200) NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE store_purchase_orders (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			order_no VARCHAR(64) NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT UNSIGNED NOT NULL,
			license_id BIGINT UNSIGNED NOT NULL,
			binding_id VARCHAR(64) NOT NULL DEFAULT '',
			item_kind VARCHAR(20) NOT NULL,
			item_id VARCHAR(64) NOT NULL,
			item_version VARCHAR(40) NOT NULL DEFAULT '',
			period VARCHAR(20) NOT NULL,
			developer_id BIGINT UNSIGNED DEFAULT NULL,
			amount_cents BIGINT NOT NULL,
			price_cents_snapshot BIGINT NOT NULL,
			title_snapshot VARCHAR(200) NOT NULL DEFAULT '',
			pay_channel VARCHAR(40) NOT NULL DEFAULT '',
			pay_method VARCHAR(40) NOT NULL DEFAULT '',
			gateway_trade_no VARCHAR(80) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL,
			return_url VARCHAR(500) NOT NULL DEFAULT '',
			needs_review TINYINT(1) NOT NULL DEFAULT 0,
			expires_at DATETIME NOT NULL,
			paid_at DATETIME DEFAULT NULL,
			result_ref VARCHAR(64) NOT NULL DEFAULT '',
			notify_payload TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE store_revenue_ledger (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			order_id BIGINT UNSIGNED NOT NULL,
			source_type VARCHAR(20) NOT NULL,
			developer_id BIGINT UNSIGNED DEFAULT NULL,
			gross_cents BIGINT NOT NULL,
			fee_bps INT NOT NULL,
			net_cents BIGINT NOT NULL,
			status VARCHAR(20) NOT NULL,
			settled_at DATETIME DEFAULT NULL,
			payout_note VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE system_configs (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			` + "`group`" + ` VARCHAR(50) NOT NULL,
			` + "`key`" + ` VARCHAR(100) NOT NULL,
			value LONGTEXT NOT NULL,
			description VARCHAR(255) DEFAULT '',
			UNIQUE KEY uk_group_key (` + "`group`" + `, ` + "`key`" + `)
		)`,
		`CREATE TABLE plugin_entitlements (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			license_id BIGINT UNSIGNED NOT NULL,
			item_kind VARCHAR(20) NOT NULL,
			item_id VARCHAR(64) NOT NULL,
			period VARCHAR(20) NOT NULL,
			expires_at DATETIME DEFAULT NULL,
			status VARCHAR(20) NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}
