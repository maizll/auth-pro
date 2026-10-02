package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 1.8.3 的老数据：应用 1 是唯一的商业版应用，客户站已经绑定并开通了商业版。
// 迁移跑两遍，老客户的商业版不能掉，默认应用就是原来那个。
func TestPerAppCommercialMigrationMariaDB(t *testing.T) {
	db := openPerAppCommercialDB(t, "authpro_pac_mig_")
	now := time.Now().Add(-time.Hour)
	mustExecPAC(t, db, `INSERT INTO apps (id, app_name, app_key, enabled, commercial_product) VALUES
		(1, '云盘系统', 'cloud-app', 1, 1), (2, '博客系统', 'blog-app', 1, 0)`)
	mustExecPAC(t, db, `INSERT INTO license_plans (id, app_id, name, duration_days, price, enabled) VALUES (11, 1, '永久商业版', 0, 199, 1)`)
	mustExecPAC(t, db, `INSERT INTO system_configs (`+"`group`, `key`"+`, value) VALUES ('store', 'store_grace_days', '9'), ('store', 'store_revoke_on_password_change', '0')`)
	mustExecPAC(t, db, `INSERT INTO licenses (id, license_no, app_id, type, status, source, owner_type, owner_id, started_at)
		VALUES (100, 'LIC-OLD', 1, 'domain', 'active', 'store_bind', 'user', 7, ?)`, now)
	mustExecPAC(t, db, `INSERT INTO license_domains (license_id, domain) VALUES (100, 'old.example.com')`)
	mustExecPAC(t, db, `INSERT INTO main_license_editions (license_id, edition, period, started_at, status) VALUES (100, 'commercial', 'permanent', ?, 'active')`, now)
	mustExecPAC(t, db, `INSERT INTO store_bindings (binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, status)
		VALUES ('sb_old', ?, 'user', 7, 100, 'old.example.com', 'active')`, []byte("salt-old"))
	// 授权已经删掉的订单：按套餐归到应用 1。
	mustExecPAC(t, db, `INSERT INTO store_purchase_orders (order_no, owner_type, owner_id, license_id, item_kind, item_id, period, amount_cents, price_cents_snapshot, status, expires_at)
		VALUES ('PP-OLD', 'user', 7, 100, 'edition', '11', 'permanent', 19900, 19900, 'paid', ?),
		       ('PP-GONE', 'user', 8, 999, 'edition', '11', 'permanent', 19900, 19900, 'paid', ?)`, now, now)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(useStoreSnapshotKeysForTest(publicKey, privateKey))
	for round := 1; round <= 2; round++ {
		if err := migratePerAppCommercial(db); err != nil {
			t.Fatalf("第 %d 次迁移失败: %v", round, err)
		}
	}
	var rows, legacyRows int
	mustScanPAC(t, db, `SELECT COUNT(*), COALESCE(SUM(legacy_default), 0) FROM app_commercial_settings`, &rows, &legacyRows)
	if rows != 1 || legacyRows != 1 {
		t.Fatalf("迁移后应只有原商业版应用一行设置 rows=%d legacy=%d", rows, legacyRows)
	}
	legacy, err := resolveStoreProductApp(db, "")
	if err != nil || legacy.AppID != 1 || legacy.Mode != appCommercialModeSelling || legacy.GraceDays != 9 || legacy.RevokeOnPasswordChange {
		t.Fatalf("老客户端应落到原商业版应用，并保留宽限 9 天、改密不解绑: %+v err=%v", legacy, err)
	}
	var bindingApp, editionApp, orderApp, goneApp sql.NullInt64
	mustScanPAC(t, db, `SELECT app_id FROM store_bindings WHERE binding_id = 'sb_old'`, &bindingApp)
	mustScanPAC(t, db, `SELECT app_id FROM main_license_editions WHERE license_id = 100`, &editionApp)
	mustScanPAC(t, db, `SELECT app_id FROM store_purchase_orders WHERE order_no = 'PP-OLD'`, &orderApp)
	mustScanPAC(t, db, `SELECT app_id FROM store_purchase_orders WHERE order_no = 'PP-GONE'`, &goneApp)
	if bindingApp.Int64 != 1 || editionApp.Int64 != 1 || orderApp.Int64 != 1 || goneApp.Int64 != 1 {
		t.Fatalf("app_id 回填不对 binding=%v edition=%v order=%v gone=%v", bindingApp, editionApp, orderApp, goneApp)
	}
	snapshot, bound, err := signedSnapshotForLicense(db, 100)
	if err != nil || !bound || snapshot.Edition != storeEditionCommercial || snapshot.GraceDays != 9 {
		t.Fatalf("升级后老客户的商业版掉了: %+v bound=%v err=%v", snapshot, bound, err)
	}
	var flag1, flag2 int
	mustScanPAC(t, db, `SELECT commercial_product FROM apps WHERE id = 1`, &flag1)
	mustScanPAC(t, db, `SELECT commercial_product FROM apps WHERE id = 2`, &flag2)
	if flag1 != 1 || flag2 != 0 {
		t.Fatalf("回滚用的 commercial_product 应只标记默认应用 got %d %d", flag1, flag2)
	}
}

// 只配过旧的 store_product_app_key、没有 commercial_product 标记的站点，也要认出原商业版应用。
func TestPerAppCommercialMigrationFromConfigKeyMariaDB(t *testing.T) {
	db := openPerAppCommercialDB(t, "authpro_pac_key_")
	mustExecPAC(t, db, `INSERT INTO apps (id, app_name, app_key, enabled, commercial_product) VALUES (1, '云盘系统', 'cloud-app', 1, 0), (2, '博客系统', 'blog-app', 1, 0)`)
	mustExecPAC(t, db, `INSERT INTO system_configs (`+"`group`, `key`"+`, value) VALUES ('store', 'store_product_app_key', ' blog-app ')`)
	if err := migratePerAppCommercial(db); err != nil {
		t.Fatal(err)
	}
	legacy, err := resolveStoreProductApp(db, "")
	if err != nil || legacy.AppID != 2 {
		t.Fatalf("应按旧配置认出应用 2: %+v err=%v", legacy, err)
	}
	// 全新站点：没有任何商业版线索时不建设置行，老客户端拿到明确的提示。
	fresh := openPerAppCommercialDB(t, "authpro_pac_new_")
	mustExecPAC(t, fresh, `INSERT INTO apps (id, app_name, app_key, enabled) VALUES (1, '云盘系统', 'cloud-app', 1)`)
	if err := migratePerAppCommercial(fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveStoreProductApp(fresh, ""); !errors.Is(err, errAppCommercialNoLegacy) {
		t.Fatalf("没有默认应用时应提示 err=%v", err)
	}
}

// 官网上两个应用各自出售商业版：套餐、统计、停售和作废互不影响。
func TestTwoAppsCommercialMariaDB(t *testing.T) {
	db := openPerAppCommercialDB(t, "authpro_pac_two_")
	if err := migratePerAppCommercial(db); err != nil {
		t.Fatal(err)
	}
	useOfficialSiteForTest(t)
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	gin.SetMode(gin.TestMode)

	mustExecPAC(t, db, `INSERT INTO apps (id, app_name, app_key, app_secret, enabled) VALUES (1, '云盘系统', 'cloud-app', 's1', 1), (2, '博客系统', 'blog-app', 's2', 1)`)
	mustExecPAC(t, db, `INSERT INTO license_plans (id, app_id, name, duration_days, price, enabled) VALUES
		(11, 1, '云盘永久', 0, 199, 1), (21, 2, '博客年付', 365, 99, 1)`)

	first := putCommercial(t, 1, map[string]any{"mode": "selling"})
	if jsonCode(first) != 200 {
		t.Fatalf("应用 1 开启出售失败: %v", first)
	}
	second := putCommercial(t, 2, map[string]any{"mode": "selling", "graceDays": 3})
	if jsonCode(second) != 200 {
		t.Fatalf("应用 2 开启出售失败（不应再有全站只能一个的限制）: %v", second)
	}
	cloud, _ := loadAppCommercial(db, 1)
	blog, _ := loadAppCommercial(db, 2)
	if cloud.Mode != appCommercialModeSelling || !cloud.LegacyDefault || blog.Mode != appCommercialModeSelling || blog.LegacyDefault || blog.GraceDays != 3 {
		t.Fatalf("两个应用的设置不对 cloud=%+v blog=%+v", cloud, blog)
	}

	cloudPlans, err := listCommercialSalePlans(db, 1)
	if err != nil || len(cloudPlans) != 1 || cloudPlans[0]["name"] != "云盘永久" {
		t.Fatalf("应用 1 只应看到自己的套餐: %v err=%v", cloudPlans, err)
	}
	blogPlans, err := listCommercialSalePlans(db, 2)
	if err != nil || len(blogPlans) != 1 || blogPlans[0]["name"] != "博客年付" {
		t.Fatalf("应用 2 只应看到自己的套餐: %v err=%v", blogPlans, err)
	}
	if _, _, _, err := loadCommercialSalePlan(db, 1, 21); !errors.Is(err, errCommercialPlanAppMismatch) {
		t.Fatalf("用应用 2 的套餐给应用 1 下单应被拒绝 err=%v", err)
	}
	if byKey, err := resolveStoreProductApp(db, "blog-app"); err != nil || byKey.AppID != 2 {
		t.Fatalf("带应用标识的新客户端应落到应用 2: %+v err=%v", byKey, err)
	}

	now := time.Now().Add(-time.Hour)
	mustExecPAC(t, db, `INSERT INTO licenses (id, license_no, app_id, type, status, source, owner_type, owner_id, started_at) VALUES
		(100, 'LIC-CLOUD', 1, 'domain', 'active', 'store_bind', 'user', 7, ?),
		(200, 'LIC-BLOG', 2, 'domain', 'active', 'store_bind', 'user', 7, ?)`, now, now)
	mustExecPAC(t, db, `INSERT INTO license_domains (license_id, domain) VALUES (100, 'shop.example.com'), (200, 'shop.example.com')`)
	for _, licenseID := range []int64{100, 200} {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := grantCommercialEditionTx(tx, commercialEditionGrant{LicenseID: licenseID, Period: storePeriodPermanent}); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	mustExecPAC(t, db, `INSERT INTO store_bindings (binding_id, secret_salt, owner_type, owner_id, license_id, app_id, domain_snapshot, status) VALUES
		('sb_cloud', 'a', 'user', 7, 100, 1, 'shop.example.com', 'active'),
		('sb_blog', 'b', 'user', 7, 200, 2, 'shop.example.com', 'active')`)
	cloudStats, _ := loadAppCommercialStats(db, 1)
	blogStats, _ := loadAppCommercialStats(db, 2)
	if cloudStats.ActiveLicenses != 1 || cloudStats.BoundSites != 1 || blogStats.ActiveLicenses != 1 || blogStats.BoundSites != 1 {
		t.Fatalf("统计数字串了 cloud=%+v blog=%+v", cloudStats, blogStats)
	}

	// 有有效授权时不能直接转为普通应用。
	off := putCommercial(t, 2, map[string]any{"mode": "off"})
	if jsonCode(off) == 200 {
		t.Fatalf("有有效授权时转为普通应用应被拒绝: %v", off)
	}

	// 停售博客：已售权益照常，新单被拒绝；云盘不受影响。
	stop := closeCommercial(t, 2, map[string]any{"action": "stop"}, "R_ADMIN")
	if jsonCode(stop) != 200 {
		t.Fatalf("停售失败: %v", stop)
	}
	blog, _ = loadAppCommercial(db, 2)
	if blog.Mode != appCommercialModeStopped || !errors.Is(blog.saleProblem(), errAppCommercialStoppedSale) {
		t.Fatalf("停售后状态不对: %+v", blog)
	}
	if snap, _, err := signedSnapshotForLicense(db, 200); err != nil || snap.Edition != storeEditionCommercial {
		t.Fatalf("停售后已售权益应照常: %+v err=%v", snap, err)
	}
	cloud, _ = loadAppCommercial(db, 1)
	if cloud.saleProblem() != nil {
		t.Fatalf("停售博客不应影响云盘: %+v", cloud)
	}

	// 作废：普通管理员不行，名字输错不行，超级管理员输对才行。
	if body := closeCommercial(t, 2, map[string]any{"action": "revoke", "confirmName": "博客系统"}, "R_ADMIN"); jsonCode(body) != 403 {
		t.Fatalf("普通管理员作废应返回 403: %v", body)
	}
	if body := closeCommercial(t, 2, map[string]any{"action": "revoke", "confirmName": "博客"}, "R_SUPER"); jsonCode(body) != 400 {
		t.Fatalf("应用名输错应返回 400: %v", body)
	}
	revoke := closeCommercial(t, 2, map[string]any{"action": "revoke", "confirmName": "博客系统"}, "R_SUPER")
	if jsonCode(revoke) != 200 {
		t.Fatalf("超级管理员作废失败: %v", revoke)
	}
	if snap, _, err := signedSnapshotForLicense(db, 200); err != nil || snap.Edition != storeEditionFree {
		t.Fatalf("作废后博客客户站应回到免费版: %+v err=%v", snap, err)
	}
	var bindingStatus string
	mustScanPAC(t, db, `SELECT status FROM store_bindings WHERE binding_id = 'sb_blog'`, &bindingStatus)
	if bindingStatus != "active" {
		t.Fatalf("作废不应吊销绑定 status=%s", bindingStatus)
	}
	if snap, _, err := signedSnapshotForLicense(db, 100); err != nil || snap.Edition != storeEditionCommercial {
		t.Fatalf("作废博客不应影响云盘的商业版: %+v err=%v", snap, err)
	}

	// 默认应用换到博客要先恢复出售；关闭云盘时把老客户端改到博客。
	if body := putCommercial(t, 2, map[string]any{"mode": "selling"}); jsonCode(body) != 200 {
		t.Fatalf("恢复出售失败: %v", body)
	}
	legacyTo := int64(2)
	if _, err := closeAppCommercial(db, 1, appCommercialCloseRequest{Action: "stop", LegacyTo: &legacyTo}, 1, false); err != nil {
		t.Fatal(err)
	}
	if legacy, err := resolveStoreProductApp(db, ""); err != nil || legacy.AppID != 2 {
		t.Fatalf("老客户端应改到博客: %+v err=%v", legacy, err)
	}
	var flag int
	mustScanPAC(t, db, `SELECT commercial_product FROM apps WHERE id = 2`, &flag)
	if flag != 1 {
		t.Fatal("commercial_product 应跟着默认应用走")
	}

	// 应用列表在官网返回每个应用自己的商业版设置。
	list := callCommercialJSON(t, http.MethodGet, "/api/app/list", nil, AppManageList)
	if jsonCode(list) != 200 {
		t.Fatalf("应用列表失败: %v", list)
	}
	items, _ := list["data"].([]any)
	modes := map[string]string{}
	for _, raw := range items {
		row := raw.(map[string]any)
		commercial, _ := row["commercial"].(map[string]any)
		mode, _ := commercial["mode"].(string)
		modes[row["name"].(string)] = mode
	}
	if modes["云盘系统"] != appCommercialModeStopped || modes["博客系统"] != appCommercialModeSelling {
		t.Fatalf("列表里的商业版状态不对: %v", modes)
	}
}

// 客户站不出售商业版：带商业版字段的保存和关闭都被拒绝。
func TestCustomerSiteRejectsCommercialMariaDB(t *testing.T) {
	db := openPerAppCommercialDB(t, "authpro_pac_cust_")
	if err := migratePerAppCommercial(db); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if officialSite() {
		t.Fatal("没有签名私钥时应是客户站")
	}
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	gin.SetMode(gin.TestMode)
	mustExecPAC(t, db, `INSERT INTO apps (id, app_name, app_key, app_secret, enabled) VALUES (1, '云盘系统', 'cloud-app', 's1', 1)`)
	if body := putCommercial(t, 1, map[string]any{"mode": "selling"}); jsonCode(body) != 400 {
		t.Fatalf("客户站保存商业版应被拒绝: %v", body)
	}
	if body := closeCommercial(t, 1, map[string]any{"action": "stop"}, "R_SUPER"); jsonCode(body) != 403 {
		t.Fatalf("客户站关闭商业版应返回 403: %v", body)
	}
	var rows int
	mustScanPAC(t, db, `SELECT COUNT(*) FROM app_commercial_settings`, &rows)
	if rows != 0 {
		t.Fatalf("客户站不应写商业版设置 rows=%d", rows)
	}
}

func putCommercial(t *testing.T, appID int64, commercial map[string]any) map[string]any {
	t.Helper()
	var name string
	db, _ := config.DB()
	_ = db.QueryRow(`SELECT app_name FROM apps WHERE id = ?`, appID).Scan(&name)
	_, body := callAppJSON(t, http.MethodPut, fmt.Sprintf("/api/app/%d", appID), gin.Params{{Key: "id", Value: strconv.FormatInt(appID, 10)}},
		map[string]any{"name": name, "enabled": true, "commercial": commercial}, func(c *gin.Context) {
			c.Set("user_id", uint(1))
			AppUpdate(c)
		})
	return body
}

func closeCommercial(t *testing.T, appID int64, req map[string]any, role string) map[string]any {
	t.Helper()
	_, body := callAppJSON(t, http.MethodPost, fmt.Sprintf("/api/app/%d/commercial/close", appID), gin.Params{{Key: "id", Value: strconv.FormatInt(appID, 10)}},
		req, func(c *gin.Context) {
			c.Set("user_id", uint(1))
			c.Set("username", "tester")
			c.Set("role_code", role)
			AppCommercialCloseHandler(c)
		})
	return body
}

func useOfficialSiteForTest(t *testing.T) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previous })
	writeSnapshotKey(t, privateKey)
	t.Cleanup(useStoreSnapshotKeysForTest(publicKey, privateKey))
	t.Cleanup(SetSourceStationStoreForTest(newMemorySourceStore()))
	if !officialSite() {
		t.Fatal("测试环境应是官网")
	}
}

// openPerAppCommercialDB 建一个 1.8.3 结构的临时库，测试结束删除。
func openPerAppCommercialDB(t *testing.T, prefix string) *sql.DB {
	t.Helper()
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	name := prefix + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + name + "`") })
	db := openAppUpdateDatabase(t, name)
	t.Cleanup(func() { db.Close() })
	if err := createCommercialGapSchema(db); err != nil {
		t.Fatal(err)
	}
	markCommercialGapMigrations(t, db)
	mustExecPAC(t, db, `ALTER TABLE apps
		ADD COLUMN app_secret VARCHAR(128) NOT NULL DEFAULT '',
		ADD COLUMN license_required TINYINT(1) NOT NULL DEFAULT 1,
		ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP`)
	mustExecPAC(t, db, `ALTER TABLE license_plans
		ADD COLUMN free_site_changes INT DEFAULT NULL,
		ADD COLUMN site_change_price DECIMAL(12,2) DEFAULT NULL`)
	return db
}

func mustExecPAC(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("%v\n%s", err, query)
	}
}

func mustScanPAC(t *testing.T, db *sql.DB, query string, dest ...any) {
	t.Helper()
	if err := db.QueryRow(query).Scan(dest...); err != nil {
		t.Fatalf("%v\n%s", err, query)
	}
}
