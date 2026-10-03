package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

// TestAppUpdateMariaDB 覆盖「编辑已有应用，字段没变或只开商业版开关」仍能保存。
// 本机没有 MariaDB 时跳过。
func TestAppUpdateMariaDB(t *testing.T) {
	control := openAppUpdateControlDB(t)
	// 关连接要排在删库之后（Cleanup 后进先出），否则删库时连接已关、测试库会留下
	t.Cleanup(func() { control.Close() })

	databaseName := "authpro_app_update_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })

	db := openAppUpdateDatabase(t, databaseName)
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE apps (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			app_name VARCHAR(100) NOT NULL,
			app_key VARCHAR(64) NOT NULL,
			app_secret VARCHAR(128) NOT NULL,
			description VARCHAR(255) DEFAULT '',
			enabled TINYINT(1) DEFAULT 1,
			commercial_product TINYINT(1) NOT NULL DEFAULT 0,
			license_required TINYINT(1) NOT NULL DEFAULT 1,
			purchase_license_type_mask TINYINT UNSIGNED NOT NULL DEFAULT 15,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_app_key (app_key)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE licenses (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			app_id BIGINT UNSIGNED NOT NULL,
			PRIMARY KEY (id),
			KEY idx_licenses_app (app_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE system_configs (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			` + "`group`" + ` VARCHAR(50) NOT NULL,
			` + "`key`" + ` VARCHAR(100) NOT NULL,
			value LONGTEXT NOT NULL,
			description VARCHAR(255) DEFAULT '',
			PRIMARY KEY (id),
			UNIQUE KEY uk_group_key (` + "`group`" + `, ` + "`key`" + `)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`); err != nil {
		t.Fatal(err)
	}

	prevPurchase := appPurchaseLicenseTypesOK
	prevVersions := appVersionTableOK
	appPurchaseLicenseTypesOK = false
	appVersionTableOK = false
	t.Cleanup(func() {
		appPurchaseLicenseTypesOK = prevPurchase
		appVersionTableOK = prevVersions
	})

	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })

	gin.SetMode(gin.TestMode)

	// 新建 → 字段不变再保存 → 改名称和备注 → 从列表读出。商业版设置见 app_commercial_mysql_test.go。
	createCode, createBody := postAppCreate(t, appUpdateBody("授权系统", "", true))
	if jsonCode(createBody) != 200 {
		t.Fatalf("新建应用失败 http=%d body=%s", createCode, createBody)
	}
	createdID := int64(createBody["data"].(map[string]any)["id"].(float64))
	same := appUpdateBody("授权系统", "", true)
	code, body := putAppUpdate(t, createdID, same)
	if code != 200 || jsonCode(body) != 200 {
		t.Fatalf("字段不变保存应成功，http=%d body=%s", code, body)
	}
	assertAppRow(t, db, createdID, "授权系统", "", 1)

	// 老数据直接插入：没有 owner 字段，提交值和原值相同。
	res, err := db.Exec(`
		INSERT INTO apps (app_name, app_key, app_secret, description, enabled, purchase_license_type_mask)
		VALUES ('旧应用', 'app_legacy', 'sk_live_legacy', '', 1, 15)
	`)
	if err != nil {
		t.Fatal(err)
	}
	legacyID, _ := res.LastInsertId()
	code, body = putAppUpdate(t, legacyID, appUpdateBody("旧应用", "", true))
	if jsonCode(body) != 200 {
		t.Fatalf("老数据原样保存应成功，http=%d body=%s", code, body)
	}
	assertAppRow(t, db, legacyID, "旧应用", "", 1)
	renamed := appUpdateBody("买家主应用-改", "备注一", true)
	code, body = putAppUpdate(t, createdID, renamed)
	if jsonCode(body) != 200 {
		t.Fatalf("编辑保存失败 http=%d body=%s", code, body)
	}
	reopened := appFromList(t, createdID)
	if reopened["name"] != "买家主应用-改" || reopened["remark"] != "备注一" {
		t.Fatalf("再次打开字段不对: %#v", reopened)
	}
	if _, ok := reopened["commercial"]; ok {
		t.Fatalf("客户站的应用列表不应带商业版设置: %#v", reopened)
	}

	code, body = putAppUpdate(t, createdID, renamed)
	if jsonCode(body) != 200 {
		t.Fatalf("字段不变再次保存失败 http=%d body=%s", code, body)
	}
	assertAppRow(t, db, createdID, "买家主应用-改", "备注一", 1)

	code, body = putAppUpdate(t, 999999, same)
	if jsonCode(body) != 404 {
		t.Fatalf("不存在的应用应返回 404，http=%d body=%s", code, body)
	}
}

func openAppUpdateControlDB(t *testing.T) *sql.DB {
	t.Helper()
	candidates := []string{}
	if dsn := os.Getenv("AUTO_PRO_APP_UPDATE_TEST_DSN"); dsn != "" {
		candidates = append(candidates, dsn)
	}
	candidates = append(candidates, "authpro_e2e:authpro_e2e_pass@tcp(127.0.0.1:3306)/?parseTime=true&charset=utf8mb4&multiStatements=true")
	var last error
	for _, dsn := range candidates {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			last = err
			continue
		}
		if err := db.Ping(); err != nil {
			_ = db.Close()
			last = err
			continue
		}
		return db
	}
	t.Skipf("MariaDB 不可用，跳过应用编辑保存: %v", last)
	return nil
}

func openAppUpdateDatabase(t *testing.T, databaseName string) *sql.DB {
	t.Helper()
	// control 的 DSN 没有库名。用同一套账号连到刚建的库。
	raw := os.Getenv("AUTO_PRO_APP_UPDATE_TEST_DSN")
	if raw == "" {
		raw = "authpro_e2e:authpro_e2e_pass@tcp(127.0.0.1:3306)/?parseTime=true&charset=utf8mb4&multiStatements=true"
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = databaseName
	cfg.ParseTime = true
	cfg.MultiStatements = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}

func appUpdateBody(name, remark string, enabled bool) map[string]any {
	return map[string]any{
		"name":                 name,
		"enabled":              enabled,
		"remark":               remark,
		"purchaseLicenseTypes": []string{"domain", "wildcard", "ip", "key"},
	}
}

func putAppUpdate(t *testing.T, id int64, body map[string]any) (int, map[string]any) {
	t.Helper()
	return callAppJSON(t, http.MethodPut, fmt.Sprintf("/api/app/%d", id), gin.Params{{Key: "id", Value: strconv.FormatInt(id, 10)}}, body, AppUpdate)
}

func postAppCreate(t *testing.T, body map[string]any) (int, map[string]any) {
	t.Helper()
	return callAppJSON(t, http.MethodPost, "/api/app/create", nil, body, AppCreate)
}

func callAppJSON(t *testing.T, method, target string, params gin.Params, body map[string]any, handler gin.HandlerFunc) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	handler(ctx)
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("响应不是 JSON: %s", recorder.Body.String())
	}
	return recorder.Code, decoded
}

func jsonCode(body map[string]any) int {
	switch value := body["code"].(type) {
	case float64:
		return int(value)
	default:
		return 0
	}
}

func assertAppRow(t *testing.T, db *sql.DB, id int64, name, remark string, enabled int) {
	t.Helper()
	var gotName, gotRemark string
	var gotEnabled int
	err := db.QueryRow(`
		SELECT app_name, description, enabled FROM apps WHERE id = ?
	`, id).Scan(&gotName, &gotRemark, &gotEnabled)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != name || gotRemark != remark || gotEnabled != enabled {
		t.Fatalf("apps 行 id=%d got name=%q remark=%q enabled=%d", id, gotName, gotRemark, gotEnabled)
	}
}

func appFromList(t *testing.T, id int64) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/app/list", nil)
	AppManageList(ctx)
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if jsonCode(decoded) != 200 {
		t.Fatalf("应用列表失败: %s", recorder.Body.String())
	}
	list, _ := decoded["data"].([]any)
	for _, item := range list {
		row := item.(map[string]any)
		if int64(row["id"].(float64)) == id {
			return row
		}
	}
	t.Fatalf("列表里没有应用 %d: %s", id, recorder.Body.String())
	return nil
}
