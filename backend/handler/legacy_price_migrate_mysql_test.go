package handler

import (
	"os"
	"strconv"
	"testing"
)

// 从 1.5.8 直接升级：按应用出售的迁移先跑，apps.commercial_product 后补。
// 补列时要按默认应用补上标记，旧的商业版价格才能搬进套餐。
func TestLegacyEditionPricesMigrateAfterLateFlagColumn(t *testing.T) {
	control := openAppUpdateControlDB(t)
	// 关连接要排在删库之后（Cleanup 后进先出），否则删库时连接已关、测试库会留下
	t.Cleanup(func() { control.Close() })
	databaseName := "authpro_legacyprice_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE apps (
			id BIGINT UNSIGNED PRIMARY KEY,
			app_key VARCHAR(64) NOT NULL,
			enabled TINYINT NOT NULL DEFAULT 1
		);
		CREATE TABLE app_commercial_settings (
			app_id BIGINT UNSIGNED PRIMARY KEY,
			mode VARCHAR(16) NOT NULL,
			legacy_default TINYINT NOT NULL DEFAULT 0
		);
		CREATE TABLE store_edition_plans (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(100) NOT NULL,
			period VARCHAR(20) NOT NULL,
			price_cents BIGINT NOT NULL,
			enabled TINYINT NOT NULL DEFAULT 1,
			sort INT NOT NULL DEFAULT 0
		);
		CREATE TABLE license_plans (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			app_id BIGINT UNSIGNED NOT NULL,
			name VARCHAR(100) NOT NULL,
			license_type VARCHAR(20) NOT NULL DEFAULT '',
			duration_days INT NOT NULL,
			price DECIMAL(10,2) NOT NULL,
			max_sites INT NOT NULL DEFAULT 0,
			sort INT NOT NULL DEFAULT 0,
			enabled TINYINT NOT NULL DEFAULT 1,
			remark VARCHAR(255) NOT NULL DEFAULT ''
		);
		INSERT INTO apps (id, app_key) VALUES (1, 'other'), (2, 'shop');
		INSERT INTO app_commercial_settings (app_id, mode, legacy_default) VALUES (2, 'selling', 1);
		INSERT INTO store_edition_plans (name, period, price_cents) VALUES ('旧永久商业版', 'permanent', 9900);
	`); err != nil {
		t.Fatal(err)
	}
	commercialProductColumnOK = false
	t.Cleanup(func() { commercialProductColumnOK = false })
	if err := prepareCommercialProduct(db); err != nil {
		t.Fatal(err)
	}
	var flagged int64
	if err := db.QueryRow(`SELECT id FROM apps WHERE commercial_product = 1`).Scan(&flagged); err != nil || flagged != 2 {
		t.Fatalf("默认应用没有补上 commercial_product: id=%d err=%v", flagged, err)
	}
	var appID int64
	var price string
	if err := db.QueryRow(`SELECT app_id, CAST(price AS CHAR) FROM license_plans WHERE name = '旧永久商业版' AND duration_days = 0`).Scan(&appID, &price); err != nil {
		t.Fatalf("旧价格没有迁进套餐: %v", err)
	}
	if appID != 2 || price != "99.00" {
		t.Fatalf("迁移的套餐不对: app=%d price=%s", appID, price)
	}
}
