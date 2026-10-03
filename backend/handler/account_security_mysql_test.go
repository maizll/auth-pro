package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 1.8.8 的两条账户安全修复（B1 冻结代理、B3 保存用户资料回滚余额）用真实 MariaDB/MySQL 复现。本机没有数据库时跳过。
func openAccountSecurityDB(t *testing.T, label string) {
	t.Helper()
	control := openAppUpdateControlDB(t)
	t.Cleanup(func() { control.Close() })
	name := "authpro_acctsec_" + label + "_" + strconv.Itoa(os.Getpid())
	if _, err := control.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + name + "`") })
	db := openAppUpdateDatabase(t, name)
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		`CREATE TABLE agents (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100) NOT NULL DEFAULT '',
			password_hash VARCHAR(255) NOT NULL DEFAULT '',
			password_changed_at DATETIME DEFAULT NULL,
			enabled TINYINT(1) NOT NULL DEFAULT 1,
			balance DECIMAL(12,2) NOT NULL DEFAULT 0
		) ENGINE=InnoDB`,
		`CREATE TABLE users (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			email VARCHAR(100) NOT NULL,
			nickname VARCHAR(100) NOT NULL DEFAULT '',
			password_hash VARCHAR(255) NOT NULL DEFAULT '',
			password_changed_at DATETIME DEFAULT NULL,
			balance DECIMAL(12,2) NOT NULL DEFAULT 0
		) ENGINE=InnoDB`,
		`CREATE TABLE transactions (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			tx_no VARCHAR(64) NOT NULL,
			subject_type ENUM('agent','user') NOT NULL,
			subject_id BIGINT UNSIGNED NOT NULL,
			type ENUM('recharge','consume','refund','purchase','transfer','bonus') NOT NULL,
			amount DECIMAL(12,2) NOT NULL,
			balance_after DECIMAL(12,2) DEFAULT NULL,
			ref_type VARCHAR(50) DEFAULT '',
			ref_id BIGINT UNSIGNED DEFAULT NULL,
			remark VARCHAR(255) DEFAULT '',
			UNIQUE KEY uk_tx_no (tx_no)
		) ENGINE=InnoDB`,
		`CREATE TABLE operation_logs (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			operator_type ENUM('admin','agent','user','system') NOT NULL,
			operator_id BIGINT UNSIGNED DEFAULT NULL,
			action VARCHAR(100) NOT NULL,
			target_type VARCHAR(50) DEFAULT '',
			target_id BIGINT UNSIGNED DEFAULT NULL,
			detail JSON DEFAULT NULL,
			ip VARCHAR(45) DEFAULT ''
		) ENGINE=InnoDB`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
}

func signAgentPanelToken(t *testing.T, agentID uint, issuedAt time.Time) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: agentID, Username: "agent", Role: "agent",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// 复现 B1：代理被冻结后，旧登录仍能改授权、换密钥、解绑站点。中间件链和 main.go 的 /agent-panel 一致。
func TestFrozenAgentSessionRejectedMySQL(t *testing.T) {
	openAccountSecurityDB(t, "agent")
	db, _ := config.DB()
	if _, err := db.Exec("INSERT INTO agents (id, name, enabled) VALUES (7, '代理七', 1)"); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	panel := router.Group("/api/agent-panel")
	panel.Use(middleware.JWTAuth(), middleware.RequireAgent(), middleware.RequireFreshPassword("agents"))
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200}) }
	panel.PUT("/licenses/:id", ok)
	panel.POST("/licenses/:id/refresh-key", ok)
	panel.DELETE("/licenses/:id/sites/:siteId", ok)
	router.PUT("/api/agent/:id/toggle", AgentToggle)

	token := signAgentPanelToken(t, 7, time.Now().Add(-10*time.Second))
	call := func() []int {
		codes := []int{}
		for _, req := range []*http.Request{
			httptest.NewRequest(http.MethodPut, "/api/agent-panel/licenses/1", strings.NewReader("{}")),
			httptest.NewRequest(http.MethodPost, "/api/agent-panel/licenses/1/refresh-key", nil),
			httptest.NewRequest(http.MethodDelete, "/api/agent-panel/licenses/1/sites/2", nil),
		} {
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			codes = append(codes, rec.Code)
		}
		return codes
	}
	toggle := func(status string) {
		req := httptest.NewRequest(http.MethodPut, "/api/agent/7/toggle", strings.NewReader(`{"status":"`+status+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), `"code":200`) {
			t.Fatalf("toggle %s: %s", status, rec.Body.String())
		}
	}

	for _, code := range call() {
		if code != http.StatusOK {
			t.Fatalf("冻结前应能访问，得到 %d", code)
		}
	}
	toggle("frozen")
	for _, code := range call() {
		if code != http.StatusUnauthorized {
			t.Fatalf("冻结后旧登录应 401，得到 %d", code)
		}
	}
	toggle("active")
	for _, code := range call() {
		if code != http.StatusUnauthorized {
			t.Fatalf("解冻后冻结前的旧登录也不应复活，得到 %d", code)
		}
	}
	fresh := signAgentPanelToken(t, 7, time.Now().Add(2*time.Second))
	req := httptest.NewRequest(http.MethodPut, "/api/agent-panel/licenses/1", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+fresh)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("解冻后重新登录应可用，得到 %d %s", rec.Code, rec.Body.String())
	}
}

func userBalance(t *testing.T, id int) string {
	t.Helper()
	db, _ := config.DB()
	var balance string
	if err := db.QueryRow("SELECT balance FROM users WHERE id = ?", id).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func adminJSON(router *gin.Engine, method, path, body string) map[string]any {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// 复现 B3：管理员打开编辑窗口时余额 100，期间用户花掉 30，管理员保存资料把余额写回 100；接口还收负数，也不留记录。
func TestAdminUserSaveCannotRollBackBalanceMySQL(t *testing.T) {
	openAccountSecurityDB(t, "balance")
	db, _ := config.DB()
	if _, err := db.Exec("INSERT INTO users (id, email, nickname, balance) VALUES (8, 'u8@example.com', '用户八', 100)"); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(1)); c.Next() })
	router.PUT("/api/user/:id", AdminUserUpdate)
	router.POST("/api/user/:id/balance", AdminUserBalanceAdjust)

	// 编辑窗口打开时读到 100，用户随后买东西扣 30。
	tx, _ := db.Begin()
	if _, err := deductPurchaseBalance(tx, "users", 8, 30); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	resp := adminJSON(router, http.MethodPut, "/api/user/8", `{"nickname":"用户八改名","email":"u8@example.com","balance":100}`)
	if resp["code"].(float64) != 200 {
		t.Fatalf("保存资料失败: %v", resp)
	}
	if got := userBalance(t, 8); got != "70.00" {
		t.Fatalf("保存资料把余额改成了 %s，应保持 70.00", got)
	}
	if resp := adminJSON(router, http.MethodPut, "/api/user/8", `{"balance":-500}`); resp["code"].(float64) == 200 || userBalance(t, 8) != "70.00" {
		t.Fatalf("编辑接口不应再能改余额: %v", resp)
	}

	// 调整余额：扣成负数被拒，不写流水。
	if resp := adminJSON(router, http.MethodPost, "/api/user/8/balance", `{"amount":-70.01,"remark":"测试"}`); resp["code"].(float64) != 400 {
		t.Fatalf("扣成负数应被拒绝: %v", resp)
	}
	if resp := adminJSON(router, http.MethodPost, "/api/user/8/balance", `{"amount":5}`); resp["code"].(float64) != 400 {
		t.Fatalf("没写原因应被拒绝: %v", resp)
	}

	// 并发：10 笔购买各扣 5、10 次调整各加 3，最后余额必须精确。
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for index := 0; index < 10; index++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			tx, err := db.Begin()
			if err != nil {
				errs <- err
				return
			}
			if _, err := deductPurchaseBalance(tx, "users", 8, 5); err != nil {
				_ = tx.Rollback()
				errs <- err
				return
			}
			errs <- tx.Commit()
		}()
		go func(n int) {
			defer wg.Done()
			resp := adminJSON(router, http.MethodPost, "/api/user/8/balance", `{"amount":3,"remark":"补偿 `+strconv.Itoa(n)+`"}`)
			if resp["code"].(float64) != 200 {
				errs <- &json.UnsupportedValueError{Str: "adjust failed"}
				return
			}
			errs <- nil
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := userBalance(t, 8); got != "50.00" {
		t.Fatalf("并发后余额 %s，应为 70-50+30=50.00", got)
	}
	var txCount, logCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM transactions WHERE subject_type='user' AND subject_id=8 AND ref_type='admin_balance_adjust'").Scan(&txCount)
	_ = db.QueryRow("SELECT COUNT(*) FROM operation_logs WHERE action='user_balance_adjust' AND target_id=8 AND operator_id=1").Scan(&logCount)
	if txCount != 10 || logCount != 10 {
		t.Fatalf("流水 %d 条、操作日志 %d 条，应各 10 条", txCount, logCount)
	}
}
