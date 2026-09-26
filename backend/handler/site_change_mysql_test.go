package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestSiteChangeQuotaMariaDB(t *testing.T) {
	control := openAppUpdateControlDB(t)
	defer control.Close()
	databaseName := "authpro_sitechg_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE apps (
			id BIGINT PRIMARY KEY,
			app_name VARCHAR(100) NOT NULL,
			app_key VARCHAR(64) NOT NULL,
			enabled TINYINT NOT NULL DEFAULT 1
		);
		CREATE TABLE license_plans (
			id BIGINT PRIMARY KEY,
			app_id BIGINT NOT NULL,
			name VARCHAR(100) NOT NULL,
			max_sites INT NOT NULL DEFAULT 0
		);
		CREATE TABLE licenses (
			id BIGINT PRIMARY KEY,
			license_no VARCHAR(64) NOT NULL,
			app_id BIGINT NOT NULL,
			plan_id BIGINT NULL,
			type VARCHAR(20) NOT NULL,
			status VARCHAR(20) NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT NOT NULL,
			expired_at DATETIME NULL,
			remark VARCHAR(255) NOT NULL DEFAULT ''
		);
		CREATE TABLE license_domains (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			license_id BIGINT NOT NULL,
			domain VARCHAR(255) NOT NULL,
			is_wildcard TINYINT NOT NULL DEFAULT 0,
			target_type VARCHAR(20) NOT NULL DEFAULT 'domain',
			server_ip VARCHAR(45) NOT NULL DEFAULT ''
		);
		CREATE TABLE users (
			id BIGINT PRIMARY KEY,
			balance DECIMAL(12,2) NOT NULL DEFAULT 0
		);
		CREATE TABLE transactions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			tx_no VARCHAR(64) NOT NULL,
			subject_type VARCHAR(20) NOT NULL,
			subject_id BIGINT NOT NULL,
			type VARCHAR(20) NOT NULL,
			amount DECIMAL(12,2) NOT NULL,
			balance_after DECIMAL(12,2) NOT NULL,
			ref_type VARCHAR(30) NOT NULL,
			ref_id BIGINT NOT NULL,
			remark VARCHAR(255) NOT NULL DEFAULT ''
		);
		INSERT INTO apps (id, app_name, app_key) VALUES (1, '演示应用', 'app-key');
		INSERT INTO users (id, balance) VALUES (7, 100);
		INSERT INTO licenses (id, license_no, app_id, type, status, owner_type, owner_id)
		VALUES (1, 'OLD-1', 1, 'domain', 'active', 'user', 7);
		INSERT INTO license_domains (license_id, domain) VALUES (1, 'old.example.com');
	`); err != nil {
		t.Fatal(err)
	}
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	siteChangeSchemaMu.Lock()
	delete(siteChangeSchemaDB, databaseName)
	siteChangeSchemaMu.Unlock()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	asUser := func(c *gin.Context) {
		c.Set("role", "user")
		c.Set("user_id", uint(7))
	}
	router.PUT("/licenses/:id/target", func(c *gin.Context) {
		asUser(c)
		UserLicenseUpdateTarget(c)
	})
	router.POST("/licenses/:id/site-change/pay", func(c *gin.Context) {
		asUser(c)
		UserLicenseSiteChangePay(c)
	})
	router.DELETE("/license/:id/sites/:siteId", func(c *gin.Context) {
		c.Set("role", "admin")
		c.Set("user_id", uint(1))
		AdminLicenseSiteUnbind(c)
	})
	router.PUT("/license/:id", func(c *gin.Context) {
		c.Set("role", "admin")
		c.Set("user_id", uint(1))
		LicenseUpdate(c)
	})

	call := func(method, path, body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var parsed map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("响应不是 JSON: %s", rec.Body.String())
		}
		return parsed
	}
	codeOf := func(body map[string]any) int {
		t.Helper()
		value, ok := body["code"].(float64)
		if !ok {
			t.Fatalf("缺少 code: %#v", body)
		}
		return int(value)
	}
	msgOf := func(body map[string]any) string {
		text, _ := body["msg"].(string)
		return text
	}
	domainOf := func(id int64) string {
		t.Helper()
		var domain string
		if err := db.QueryRow(`SELECT COALESCE(domain, '') FROM license_domains WHERE license_id = ? ORDER BY id LIMIT 1`, id).Scan(&domain); err != nil && err != sql.ErrNoRows {
			t.Fatal(err)
		}
		return domain
	}
	leftOf := func(id int64) int {
		t.Helper()
		var left int
		if err := db.QueryRow(`SELECT free_site_changes FROM licenses WHERE id = ?`, id).Scan(&left); err != nil {
			t.Fatal(err)
		}
		return left
	}

	migrated := call(http.MethodPut, "/licenses/1/target", `{"target":"moved.example.com"}`)
	if codeOf(migrated) != 200 || msgOf(migrated) != "更新成功" {
		t.Fatalf("旧授权更换 = %#v", migrated)
	}
	if leftOf(1) != -1 || domainOf(1) != "moved.example.com" {
		t.Fatalf("迁移后应不限次数且域名已更换 left=%d domain=%s", leftOf(1), domainOf(1))
	}

	if _, err := db.Exec(`INSERT INTO license_plans (id, app_id, name, max_sites) VALUES (9, 1, '两次', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE license_plans SET free_site_changes = 2, site_change_price = 3.50 WHERE id = 9`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO licenses (id, license_no, app_id, plan_id, type, status, owner_type, owner_id)
		VALUES (2, 'SNAP-2', 1, 9, 'domain', 'active', 'user', 7)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotLicenseSiteChange(tx, 2, 9); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var snapPrice sql.NullFloat64
	if err := db.QueryRow(`SELECT free_site_changes, site_change_price FROM licenses WHERE id = 2`).Scan(new(int), &snapPrice); err != nil {
		t.Fatal(err)
	}
	if leftOf(2) != 2 || !snapPrice.Valid || snapPrice.Float64 != 3.5 {
		t.Fatalf("快照 left=%d price=%v", leftOf(2), snapPrice)
	}

	if _, err := db.Exec(`UPDATE licenses SET free_site_changes = 1, site_change_price = 8 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE license_domains SET domain = 'a.example.com' WHERE license_id = 1`); err != nil {
		t.Fatal(err)
	}
	free := call(http.MethodPut, "/licenses/1/target", `{"target":"b.example.com"}`)
	if codeOf(free) != 200 || leftOf(1) != 0 || domainOf(1) != "b.example.com" {
		t.Fatalf("免费更换 left=%d domain=%s body=%#v", leftOf(1), domainOf(1), free)
	}
	blocked := call(http.MethodPut, "/licenses/1/target", `{"target":"c.example.com"}`)
	if codeOf(blocked) != 402 || msgOf(blocked) != msgSiteChangeNeedPay || domainOf(1) != "b.example.com" {
		t.Fatalf("应付费更换 = %#v domain=%s", blocked, domainOf(1))
	}
	if _, err := db.Exec(`UPDATE licenses SET site_change_price = NULL WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	denied := call(http.MethodPut, "/licenses/1/target", `{"target":"d.example.com"}`)
	if codeOf(denied) != 400 || msgOf(denied) != msgSiteChangeDenied || domainOf(1) != "b.example.com" {
		t.Fatalf("无价格拒绝 = %#v", denied)
	}

	if _, err := db.Exec(`UPDATE licenses SET site_change_price = 8 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	paid := call(http.MethodPost, "/licenses/1/site-change/pay", `{"action":"replace","target":"e.example.com","payMethod":"balance"}`)
	if codeOf(paid) != 200 || domainOf(1) != "e.example.com" || leftOf(1) != 0 {
		t.Fatalf("余额更换 left=%d domain=%s body=%#v", leftOf(1), domainOf(1), paid)
	}
	data, _ := paid["data"].(map[string]any)
	orderNo, _ := data["orderNo"].(string)
	if orderNo == "" {
		t.Fatalf("缺少订单号: %#v", paid)
	}
	if err := settleSiteChangeOrder(db, orderNo, 800, "balance", "balance", "trade-again", `{}`); err != nil {
		t.Fatalf("重复结算: %v", err)
	}
	if domainOf(1) != "e.example.com" || leftOf(1) != 0 {
		t.Fatalf("重复结算改变了结果 domain=%s left=%d", domainOf(1), leftOf(1))
	}
	var txCount int
	var balance float64
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE ref_type = 'site_change'`).Scan(&txCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT balance FROM users WHERE id = 7`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if txCount != 1 || balance != 92 {
		t.Fatalf("流水=%d 余额=%v", txCount, balance)
	}

	if _, err := db.Exec(`INSERT INTO licenses (id, license_no, app_id, type, status, owner_type, owner_id)
		VALUES (3, 'FIRST-3', 1, 'domain', 'active', 'user', 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE licenses SET free_site_changes = 1 WHERE id = 3`); err != nil {
		t.Fatal(err)
	}
	first := call(http.MethodPut, "/licenses/3/target", `{"target":"first.example.com"}`)
	if codeOf(first) != 200 || leftOf(3) != 1 || domainOf(3) != "first.example.com" {
		t.Fatalf("首次绑定不应扣次 left=%d domain=%s body=%#v", leftOf(3), domainOf(3), first)
	}

	if _, err := db.Exec(`UPDATE licenses SET free_site_changes = 2 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	adminEdit := call(http.MethodPut, "/license/1", `{"appId":1,"type":"domain","domain":"admin.example.com","remark":""}`)
	if codeOf(adminEdit) != 200 || leftOf(1) != 2 || domainOf(1) != "admin.example.com" {
		t.Fatalf("管理员更换不应扣次 left=%d domain=%s body=%#v", leftOf(1), domainOf(1), adminEdit)
	}

	if _, err := db.Exec(`INSERT INTO licenses (id, license_no, app_id, type, status, owner_type, owner_id)
		VALUES (4, 'KEY-4', 1, 'key', 'active', 'user', 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE licenses SET free_site_changes = 3 WHERE id = 4`); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO license_domains (license_id, domain, target_type) VALUES (4, 'key.example.com', 'domain')`)
	if err != nil {
		t.Fatal(err)
	}
	siteID, _ := res.LastInsertId()
	adminUnbind := call(http.MethodDelete, "/license/4/sites/"+strconv.FormatInt(siteID, 10), "")
	if codeOf(adminUnbind) != 200 || leftOf(4) != 3 {
		t.Fatalf("管理员解绑不应扣次 left=%d body=%#v", leftOf(4), adminUnbind)
	}
	var adminLogs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM license_site_change_logs WHERE license_id = 4 AND actor_type = 'admin' AND action = 'license_site_unbind'`).Scan(&adminLogs); err != nil {
		t.Fatal(err)
	}
	if adminLogs != 1 {
		t.Fatalf("管理员解绑日志 = %d", adminLogs)
	}

	if _, err := db.Exec(`INSERT INTO licenses (id, license_no, app_id, type, status, owner_type, owner_id)
		VALUES (5, 'RACE-5', 1, 'domain', 'active', 'user', 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE licenses SET free_site_changes = 1, site_change_price = 8 WHERE id = 5`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO license_domains (license_id, domain) VALUES (5, 'race.example.com')`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]map[string]any, 2)
	start := make(chan struct{})
	for i, target := range []string{"race-a.example.com", "race-b.example.com"} {
		wg.Add(1)
		go func(index int, next string) {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodPut, "/licenses/5/target", bytes.NewBufferString(`{"target":"`+next+`"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			var parsed map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
			results[index] = parsed
		}(i, target)
	}
	close(start)
	wg.Wait()
	successes, payments := 0, 0
	for _, body := range results {
		switch codeOf(body) {
		case 200:
			successes++
		case 402:
			payments++
		default:
			t.Fatalf("并发结果异常: %#v", results)
		}
	}
	if successes != 1 || payments != 1 || leftOf(5) != 0 {
		t.Fatalf("并发应一次成功一次付款 successes=%d payments=%d left=%d results=%#v", successes, payments, leftOf(5), results)
	}
}
