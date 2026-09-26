package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestUserLicenseDomainBindMariaDB(t *testing.T) {
	control := openAppUpdateControlDB(t)
	defer control.Close()
	databaseName := "authpro_bind_" + strconv.Itoa(os.Getpid())
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
			enabled TINYINT NOT NULL DEFAULT 1,
			commercial_product TINYINT NOT NULL DEFAULT 0
		);
		CREATE TABLE licenses (
			id BIGINT PRIMARY KEY,
			license_no VARCHAR(64) NOT NULL,
			app_id BIGINT NOT NULL,
			type VARCHAR(20) NOT NULL,
			status VARCHAR(20) NOT NULL,
			owner_type VARCHAR(20) NOT NULL,
			owner_id BIGINT NOT NULL
		);
		CREATE TABLE license_domains (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			license_id BIGINT NOT NULL,
			domain VARCHAR(255) NOT NULL,
			is_wildcard TINYINT NOT NULL DEFAULT 0
		);
		CREATE TABLE license_domain_changes (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			license_id BIGINT UNSIGNED NOT NULL,
			old_domain VARCHAR(255) NOT NULL DEFAULT '',
			new_domain VARCHAR(255) NOT NULL DEFAULT '',
			actor VARCHAR(50) NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id)
		);
		CREATE TABLE system_configs (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			` + "`group`" + ` VARCHAR(50) NOT NULL,
			` + "`key`" + ` VARCHAR(50) NOT NULL,
			value TEXT,
			description VARCHAR(255) NOT NULL DEFAULT ''
		);
		INSERT INTO apps (id, app_name, app_key, enabled, commercial_product) VALUES (2, '商业版产品', 'product-key', 1, 1);
		INSERT INTO licenses (id, license_no, app_id, type, status, owner_type, owner_id) VALUES
			(1, 'LIC-1', 2, 'domain', 'active', 'user', 7),
			(2, 'LIC-2', 2, 'domain', 'active', 'user', 8);
		INSERT INTO license_domains (license_id, domain) VALUES (1, 'old.example.com'), (2, 'taken.example.com');
	`); err != nil {
		t.Fatal(err)
	}
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/licenses/:id/target", func(c *gin.Context) {
		c.Set("role", "user")
		c.Set("user_id", uint(7))
		UserLicenseUpdateTarget(c)
	})

	call := func(body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/licenses/1/target", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var parsed map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("响应不是 JSON: %s", rec.Body.String())
		}
		return parsed
	}
	msg := func(body map[string]any) string {
		text, _ := body["msg"].(string)
		return text
	}

	if got := msg(call(`{"target":"bad domain"}`)); got != "单域名格式不正确" {
		t.Fatalf("格式错误 = %s", got)
	}
	if got := msg(call(`{"target":"taken.example.com"}`)); got != licenseDomainOccupied {
		t.Fatalf("占用 = %s", got)
	}
	if got := msg(call(`{"unbind":true}`)); got != "已解绑域名" {
		t.Fatalf("解绑 = %s", got)
	}
	if got := msg(call(`{"unbind":true}`)); got != "已解绑域名" {
		t.Fatalf("重复解绑 = %s", got)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM license_domains WHERE license_id = 1`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("解绑后域名行 = %d err=%v", left, err)
	}
	if got := msg(call(`{"target":"fresh.example.com"}`)); got != "更新成功" {
		t.Fatalf("绑定 = %s", got)
	}
	if got := msg(call(`{"target":"other.example.com"}`)); got != "自助更换域名每 30 天仅一次" {
		t.Fatalf("冷却 = %s", got)
	}
	var domain string
	if err := db.QueryRow(`SELECT domain FROM license_domains WHERE license_id = 1`).Scan(&domain); err != nil || domain != "fresh.example.com" {
		t.Fatalf("冷却后域名 = %s err=%v", domain, err)
	}
	var changes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM license_domain_changes WHERE license_id = 1`).Scan(&changes); err != nil || changes != 1 {
		t.Fatalf("更换记录 = %d err=%v", changes, err)
	}
}
