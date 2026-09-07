package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

func TestTicketStatusText(t *testing.T) {
	cases := map[string]string{
		ticketStatusPending: "待处理",
		ticketStatusReplied: "已回复",
		ticketStatusClosed:  "已关闭",
		"unknown":           "unknown",
	}
	for status, want := range cases {
		if got := ticketStatusText(status); got != want {
			t.Fatalf("ticketStatusText(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestTicketCategoryText(t *testing.T) {
	if got := ticketCategoryText("authorization"); got != "授权问题" {
		t.Fatalf("ticketCategoryText(authorization) = %q", got)
	}
	if got := ticketCategoryText("anything-else"); got != "其他" {
		t.Fatalf("ticketCategoryText fallback = %q", got)
	}
}

func TestTicketCreatorScopeSQL(t *testing.T) {
	if scope := ticketCreatorScopeSQL("admin"); scope != "" {
		t.Fatalf("admin scope must be empty, got %q", scope)
	}
	if scope := ticketCreatorScopeSQL("user"); !strings.Contains(scope, "creator_type") {
		t.Fatalf("user scope must restrict creator, got %q", scope)
	}
}

func TestUnreadQueryArgs(t *testing.T) {
	adminArgs := unreadQueryArgs("admin", 0)
	if len(adminArgs) != 3 || adminArgs[0] != "admin" || adminArgs[2] != int64(0) {
		t.Fatalf("admin args = %#v", adminArgs)
	}
	userArgs := unreadQueryArgs("user", 42)
	if len(userArgs) != 5 || userArgs[3] != "user" || userArgs[4] != int64(42) {
		t.Fatalf("user args = %#v", userArgs)
	}
}

// TestTicketIntegration 需要真实 MySQL：
// AUTO_PRO_RUN_TICKET_INTEGRATION=1 AUTO_PRO_TICKET_TEST_DSN="user:pass@tcp(127.0.0.1:3306)/"
func TestTicketIntegration(t *testing.T) {
	if os.Getenv("AUTO_PRO_RUN_TICKET_INTEGRATION") != "1" {
		t.Skip("set AUTO_PRO_RUN_TICKET_INTEGRATION=1")
	}
	baseDSN := os.Getenv("AUTO_PRO_TICKET_TEST_DSN")
	if baseDSN == "" {
		t.Fatal("AUTO_PRO_TICKET_TEST_DSN is required")
	}
	baseConfig, err := mysql.ParseDSN(baseDSN)
	if err != nil {
		t.Fatal(err)
	}
	controlConfig := *baseConfig
	controlConfig.DBName = ""
	control, err := sql.Open("mysql", controlConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	databaseName := "auth_pro_ticket_" + randomP4Suffix(t)
	if _, err := control.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.Exec("DROP DATABASE `" + databaseName + "`") })
	t.Setenv("AUTO_PRO_DB_HOST", baseConfig.Addr[:strings.LastIndex(baseConfig.Addr, ":")])
	t.Setenv("AUTO_PRO_DB_PORT", baseConfig.Addr[strings.LastIndex(baseConfig.Addr, ":")+1:])
	t.Setenv("AUTO_PRO_DB_NAME", databaseName)
	t.Setenv("AUTO_PRO_DB_USER", baseConfig.User)
	t.Setenv("AUTO_PRO_DB_PASSWORD", baseConfig.Passwd)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	db, err := openSystemConfigDB()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureTicketStorage(db); err != nil {
		t.Fatal(err)
	}
	// 造一个用户和一个代理作为工单创建人
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(100) NOT NULL DEFAULT '', nickname VARCHAR(50) DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS admins (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(50) NOT NULL DEFAULT '', email VARCHAR(100) NOT NULL DEFAULT '', enabled TINYINT(1) DEFAULT 1
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (email, nickname) VALUES ('u1@test.com', '测试用户')"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	asUser := func(c *gin.Context) { c.Set("role", "user"); c.Set("user_id", uint(1)) }
	asAdmin := func(c *gin.Context) { c.Set("role", "admin"); c.Set("user_id", uint(1)); c.Set("username", "admin") }

	invoke := func(method, path, body string, identity func(*gin.Context), h gin.HandlerFunc) *httptest.ResponseRecorder {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		handler := h
		router.Handle(method, path, func(c *gin.Context) {
			if identity != nil {
				identity(c)
			}
			handler(c)
		})
		var reader *strings.Reader
		if body != "" {
			reader = strings.NewReader(body)
		} else {
			reader = strings.NewReader("")
		}
		request := httptest.NewRequest(method, path, reader)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	// 1. 用户创建工单
	createResp := invoke(http.MethodPost, "/tickets", `{"category":"authorization","title":"授权无法验证","内容":"x","content":"站点提示未授权"}`, asUser, PanelTicketCreate)
	if !strings.Contains(createResp.Body.String(), `"code":200`) || !strings.Contains(createResp.Body.String(), "WO") {
		t.Fatalf("create response=%s", createResp.Body.String())
	}

	// 2. 创建后管理端未读 = 1，创建人未读 = 0
	adminUnread := invoke(http.MethodGet, "/tickets/unread-count", "", asAdmin, AdminTicketUnreadCount)
	if !strings.Contains(adminUnread.Body.String(), `"count":1`) {
		t.Fatalf("admin unread=%s", adminUnread.Body.String())
	}
	userUnread := invoke(http.MethodGet, "/tickets/unread-count", "", asUser, PanelTicketUnreadCount)
	if !strings.Contains(userUnread.Body.String(), `"count":0`) {
		t.Fatalf("user unread=%s", userUnread.Body.String())
	}

	// 3. 管理员回复后创建人未读 = 1，管理端未读清零
	replyResp := invoke(http.MethodPost, "/tickets/1/reply", `{"content":"请提供授权编号"}`, asAdmin, AdminTicketReply)
	if !strings.Contains(replyResp.Body.String(), `"code":200`) {
		t.Fatalf("admin reply=%s", replyResp.Body.String())
	}
	userUnread = invoke(http.MethodGet, "/tickets/unread-count", "", asUser, PanelTicketUnreadCount)
	if !strings.Contains(userUnread.Body.String(), `"count":1`) {
		t.Fatalf("user unread after reply=%s", userUnread.Body.String())
	}
	adminUnread = invoke(http.MethodGet, "/tickets/unread-count", "", asAdmin, AdminTicketUnreadCount)
	if !strings.Contains(adminUnread.Body.String(), `"count":0`) {
		t.Fatalf("admin unread after reply=%s", adminUnread.Body.String())
	}

	// 4. 详情接口会推进已读游标，且状态为已回复
	detail := invoke(http.MethodGet, "/tickets/1", "", asUser, PanelTicketDetail)
	if !strings.Contains(detail.Body.String(), `"status":"replied"`) || !strings.Contains(detail.Body.String(), "请提供授权编号") {
		t.Fatalf("detail=%s", detail.Body.String())
	}
	userUnread = invoke(http.MethodGet, "/tickets/unread-count", "", asUser, PanelTicketUnreadCount)
	if !strings.Contains(userUnread.Body.String(), `"count":0`) {
		t.Fatalf("user unread after read=%s", userUnread.Body.String())
	}

	// 5. 权限边界：代理身份读不到用户的工单
	asAgent := func(c *gin.Context) { c.Set("role", "agent"); c.Set("user_id", uint(1)) }
	notFound := invoke(http.MethodGet, "/tickets/1", "", asAgent, PanelTicketDetail)
	if !strings.Contains(notFound.Body.String(), `"code":404`) {
		t.Fatalf("agent read others ticket=%s", notFound.Body.String())
	}

	// 6. 关闭后双方都不能再回复
	closeResp := invoke(http.MethodPut, "/tickets/1/close", "", asUser, PanelTicketClose)
	if !strings.Contains(closeResp.Body.String(), `"code":200`) {
		t.Fatalf("close=%s", closeResp.Body.String())
	}
	closedReply := invoke(http.MethodPost, "/tickets/1/replies", `{"content":"再问一句"}`, asUser, PanelTicketReply)
	if !strings.Contains(closedReply.Body.String(), `"code":400`) {
		t.Fatalf("reply on closed=%s", closedReply.Body.String())
	}

	fmt.Println("ticket integration ok")
}
