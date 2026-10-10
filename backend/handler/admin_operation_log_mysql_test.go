package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

// B6：改角色、改角色菜单、删角色、清空校验日志都要留下操作日志；清空校验日志只有超级管理员能做。
func TestRoleChangesAndVerifyLogClearAreAuditedMySQL(t *testing.T) {
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	name := "authpro_oplog_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + name + "`") })
	db := openAppUpdateDatabase(t, name)
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		`CREATE TABLE roles (id INT AUTO_INCREMENT PRIMARY KEY, role_name VARCHAR(50) NOT NULL, role_code VARCHAR(50) NOT NULL UNIQUE,
			description VARCHAR(255) DEFAULT '', discount DECIMAL(4,2) DEFAULT 10.0, enabled TINYINT(1) DEFAULT 1) ENGINE=InnoDB`,
		`CREATE TABLE menus (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(50)) ENGINE=InnoDB`,
		`CREATE TABLE role_menus (role_id INT NOT NULL, menu_id INT NOT NULL, PRIMARY KEY (role_id, menu_id)) ENGINE=InnoDB`,
		`CREATE TABLE admins (id INT AUTO_INCREMENT PRIMARY KEY, role_id INT) ENGINE=InnoDB`,
		`CREATE TABLE verify_logs (id BIGINT AUTO_INCREMENT PRIMARY KEY, result VARCHAR(10)) ENGINE=InnoDB`,
		`CREATE TABLE operation_logs (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			operator_type ENUM('admin','agent','user','system') NOT NULL, operator_id BIGINT UNSIGNED DEFAULT NULL,
			action VARCHAR(100) NOT NULL, target_type VARCHAR(50) DEFAULT '', target_id BIGINT UNSIGNED DEFAULT NULL,
			detail JSON DEFAULT NULL, ip VARCHAR(45) DEFAULT '') ENGINE=InnoDB`,
		`INSERT INTO menus (id, name) VALUES (1, 'A'), (2, 'B'), (3, 'C')`,
		`INSERT INTO verify_logs (result) VALUES ('pass'), ('reject'), ('pass')`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })

	gin.SetMode(gin.TestMode)
	asRole := func(code string) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", uint(7))
			c.Set("role", "admin")
			c.Set("role_code", code)
			c.Next()
		})
		router.POST("/api/role/create", RoleCreate)
		router.PUT("/api/role/:id", RoleUpdate)
		router.PUT("/api/role/:id/menus", RoleUpdateMenus)
		router.DELETE("/api/role/:id", RoleDelete)
		router.DELETE("/api/verify-log/clear", middleware.RequireSuperAdmin(), VerifyLogClear)
		return router
	}
	super := asRole("R_SUPER")

	created := adminJSON(super, http.MethodPost, "/api/role/create", `{"roleName":"客服","roleCode":"R_CS","discount":9,"enabled":true}`)
	if created["code"].(float64) != 200 {
		t.Fatalf("create role: %v", created)
	}
	id := strconv.Itoa(int(created["data"].(map[string]any)["id"].(float64)))
	if resp := adminJSON(super, http.MethodPut, "/api/role/"+id, `{"roleName":"高级客服","discount":8,"enabled":true}`); resp["code"].(float64) != 200 {
		t.Fatalf("update role: %v", resp)
	}
	if resp := adminJSON(super, http.MethodPut, "/api/role/"+id+"/menus", `{"menuIds":[1,3]}`); resp["code"].(float64) != 200 {
		t.Fatalf("update menus: %v", resp)
	}
	if resp := adminJSON(super, http.MethodDelete, "/api/role/"+id, ``); resp["code"].(float64) != 200 {
		t.Fatalf("delete role: %v", resp)
	}

	logged := map[string]string{}
	rows, err := db.Query("SELECT action, CAST(detail AS CHAR), operator_id, target_id FROM operation_logs ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var action, detail string
		var operator, target int64
		if err := rows.Scan(&action, &detail, &operator, &target); err != nil {
			t.Fatal(err)
		}
		if operator != 7 || strconv.FormatInt(target, 10) != id {
			t.Fatalf("%s logged operator=%d target=%d, want 7/%s", action, operator, target, id)
		}
		logged[action] = detail
	}
	rows.Close()
	compactJSON := func(s string) string {
		var buf bytes.Buffer
		if err := json.Compact(&buf, []byte(s)); err != nil {
			return s
		}
		return buf.String()
	}
	for _, want := range []struct{ action, fragment string }{
		{"role_create", `"roleCode":"R_CS"`},
		{"role_update", `"roleName":"高级客服"`},
		{"role_menus_update", `"removed":[2]`},
		{"role_delete", `"roleName":"高级客服"`},
	} {
		detail := compactJSON(logged[want.action])
		if !strings.Contains(detail, want.fragment) {
			t.Fatalf("%s detail %q should contain %q (all: %v)", want.action, detail, want.fragment, logged)
		}
	}
	roleUpdate := compactJSON(logged["role_update"])
	if !strings.Contains(roleUpdate, `"before":{`) || !strings.Contains(roleUpdate, `"roleName":"客服"`) {
		t.Fatalf("role_update should keep the before value: %s", roleUpdate)
	}

	// 普通管理员清空校验日志：403，日志还在，不记清空。
	normal := asRole("R_ADMIN")
	if resp := adminJSON(normal, http.MethodDelete, "/api/verify-log/clear", ``); resp["code"] == float64(200) {
		t.Fatalf("non-super admin must not clear verify logs: %v", resp)
	}
	var left int
	_ = db.QueryRow("SELECT COUNT(*) FROM verify_logs").Scan(&left)
	if left != 3 {
		t.Fatalf("verify logs were cleared by a non-super admin: %d left", left)
	}
	if resp := adminJSON(super, http.MethodDelete, "/api/verify-log/clear", ``); resp["code"].(float64) != 200 {
		t.Fatalf("super admin clear: %v", resp)
	}
	var detail string
	if err := db.QueryRow("SELECT CAST(detail AS CHAR) FROM operation_logs WHERE action='verify_logs_clear'").Scan(&detail); err != nil || !strings.Contains(compactJSON(detail), `"rows":3`) {
		t.Fatalf("clearing verify logs should log the row count: %q %v", detail, err)
	}
	_ = db.QueryRow("SELECT COUNT(*) FROM verify_logs").Scan(&left)
	if left != 0 {
		t.Fatalf("super admin clear left %d rows", left)
	}
}
