package handler

import (
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"
)

// 客户站已经绑定在授权 100。后台又在同域名的另一条授权 200 上开通商业版。
// 旧实现只给 200 写权益，100 仍是免费版，客户站刷新读的是 100，顶栏不会变。
func TestManualGrantAlsoOpensTheBoundLicense(t *testing.T) {
	targets := mergeCommercialGrantTargets(200, []int64{100})
	if !grantTargetHas(targets, 100) || !grantTargetHas(targets, 200) {
		t.Fatalf("开通没有落到客户站当前绑定的授权: %v", targets)
	}
	same := mergeCommercialGrantTargets(100, []int64{100})
	if len(same) != 1 || same[0] != 100 {
		t.Fatalf("购买开通和绑定是同一条授权时不应重复写: %v", same)
	}
}

// 绑定仍覆盖这个域名时，后台再添加一条同域名授权，刷新不能要求重新绑定。
// 重新绑定之前客户站一直用旧快照，所以会一直显示免费版。
func TestRefreshKeepsBoundLicenseWhenAdminAddsAnother(t *testing.T) {
	id, reason := storeStatusLicenseID(100, 200, true)
	if reason != "" || id != 100 {
		t.Fatalf("应继续用当前绑定签发快照 id=%d reason=%s", id, reason)
	}
	id, reason = storeStatusLicenseID(100, 200, false)
	if reason == "" || id != 0 {
		t.Fatalf("绑定已不覆盖该域名时不能改挂到另一条授权 id=%d reason=%s", id, reason)
	}
	id, reason = storeStatusLicenseID(100, 100, false)
	if reason != "" || id != 100 {
		t.Fatalf("域名校验对上绑定授权时应照常刷新 id=%d reason=%s", id, reason)
	}
}

func TestManualCommercialGrantReachesBoundLicense(t *testing.T) {
	db := openCommercialGrantDB(t)
	defer db.Close()
	now := time.Now().Add(-time.Hour)
	if _, err := db.Exec(`INSERT INTO apps (id, app_name, app_key, enabled, commercial_product) VALUES (2, '商业版', 'commercial-app', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO licenses
		(id, license_no, app_id, type, status, source, owner_type, owner_id, started_at)
		VALUES
		(100, 'LIC-BOUND', 2, 'domain', 'active', 'store_bind', 'user', 7, ?),
		(200, 'LIC-ADMIN', 2, 'domain', 'active', 'admin', 'user', 7, ?)`, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO license_domains (license_id, domain, is_wildcard) VALUES (100, 'shop.example.com', 0), (200, 'shop.example.com', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO store_bindings
		(binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, status, last_seen_at)
		VALUES ('sb_shop', ?, 'user', 7, 100, 'shop.example.com', 'active', ?)`, []byte("salt"), now); err != nil {
		t.Fatal(err)
	}

	extra, err := boundLicensesForSameDomain(db, 200)
	if err != nil || !grantTargetHas(extra, 100) {
		t.Fatalf("同域名绑定应指向客户站正在用的授权 extra=%v err=%v", extra, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openCommercialEdition(tx, commercialEditionGrant{
		LicenseID: 200, Period: storePeriodPermanent, GrantedBy: 3, Extend: true,
	}, extra); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	edition, _, _, active := loadCommercialEdition(db, 100)
	if !active || edition != storeEditionCommercial {
		t.Fatalf("客户站绑定的授权仍是免费版 edition=%s active=%v", edition, active)
	}
	edition, _, _, active = loadCommercialEdition(db, 200)
	if !active || edition != storeEditionCommercial {
		t.Fatalf("后台新建的授权也应开通 edition=%s active=%v", edition, active)
	}
	var bindingStatus string
	if err := db.QueryRow(`SELECT status FROM store_bindings WHERE binding_id = 'sb_shop'`).Scan(&bindingStatus); err != nil {
		t.Fatal(err)
	}
	if bindingStatus != "active" {
		t.Fatalf("开通不应吊销绑定 status=%s", bindingStatus)
	}
	if !boundLicenseCoversDomain(db, 100, 2, "shop.example.com") {
		t.Fatal("绑定授权仍覆盖该域名，刷新不应要求重新绑定")
	}
	id, reason := storeStatusLicenseID(100, 200, true)
	if id != 100 || reason != "" {
		t.Fatalf("刷新应签发绑定授权的快照 id=%d reason=%s", id, reason)
	}
}

func openCommercialGrantDB(t *testing.T) *sql.DB {
	t.Helper()
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	databaseName := "authpro_grant_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	if err := createCommercialGapSchema(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func grantTargetHas(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
