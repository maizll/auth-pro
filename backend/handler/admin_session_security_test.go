package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func TestImpersonateRequiresSuperAdmin(t *testing.T) {
	state := newAdminSessionState(t)
	state.admins[2] = sessionAdmin{id: 2, username: "ops", roleID: 6, roleCode: "R_ADMIN", enabled: true}
	router := useAdminSessionRouter(t, state)

	for _, path := range []string{"/api/user/8/impersonate", "/api/agent/3/impersonate"} {
		recorder := performJSON(router, http.MethodPost, path, signAdminToken(t, 2, "R_ADMIN"), "203.0.113.8:1234", nil)
		if recorder.Code != http.StatusOK || responseCode(t, recorder) != 403 {
			t.Fatalf("%s R_ADMIN = %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
	if len(state.logs) != 0 {
		t.Fatalf("rejected impersonation wrote logs: %+v", state.logs)
	}
}

func TestSuperImpersonateWritesAuditAndKeepsLastLogin(t *testing.T) {
	state := newAdminSessionState(t)
	router := useAdminSessionRouter(t, state)
	token := signAdminToken(t, 1, "R_SUPER")

	for _, target := range []struct {
		path       string
		action     string
		targetType string
		targetID   int64
	}{
		{path: "/api/user/8/impersonate", action: "impersonate_user", targetType: "user", targetID: 8},
		{path: "/api/agent/3/impersonate", action: "impersonate_agent", targetType: "agent", targetID: 3},
	} {
		recorder := performJSON(router, http.MethodPost, target.path, token, "203.0.113.8:1234", nil)
		if recorder.Code != http.StatusOK || responseCode(t, recorder) != 200 {
			t.Fatalf("%s = %d %s", target.path, recorder.Code, recorder.Body.String())
		}
		accessToken := responseDataString(t, recorder, "accessToken")
		claims := parseTokenClaims(t, accessToken)
		if claims.Act != middleware.TokenActImpersonation || claims.OperatorID != 1 {
			t.Fatalf("claims act=%q operator=%d", claims.Act, claims.OperatorID)
		}
		if claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != impersonateTokenTTL {
			t.Fatalf("impersonation ttl = %v, want %v", claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time), impersonateTokenTTL)
		}
		if len(state.logs) == 0 {
			t.Fatal("missing operation log")
		}
		logRow := state.logs[len(state.logs)-1]
		if logRow.operatorID != 1 || logRow.action != target.action || logRow.targetType != target.targetType || logRow.targetID != target.targetID || logRow.ip != "203.0.113.8" {
			t.Fatalf("operation log = %+v", logRow)
		}
		if !strings.Contains(logRow.query, "created_at") || !strings.Contains(logRow.query, "NOW()") {
			t.Fatalf("operation log must record time: %s", logRow.query)
		}
	}
	if state.user.lastLoginIP != "198.51.100.9" || state.agent.lastLoginIP != "198.51.100.10" {
		t.Fatalf("last login changed: user=%q agent=%q", state.user.lastLoginIP, state.agent.lastLoginIP)
	}
	for _, query := range state.execs {
		if strings.Contains(query, "last_login") && (strings.Contains(query, "UPDATE users") || strings.Contains(query, "UPDATE agents")) {
			t.Fatalf("impersonation updated last login: %s", query)
		}
	}

	issued := performJSON(router, http.MethodPost, "/api/user/8/impersonate", token, "203.0.113.8:1234", nil)
	audit := performJSON(router, http.MethodGet, "/api/user-panel/audit-operator", responseDataString(t, issued, "accessToken"), "198.51.100.20:9", nil)
	if audit.Code != http.StatusOK || responseCode(t, audit) != 200 {
		t.Fatalf("audit route = %d %s", audit.Code, audit.Body.String())
	}
	if got := responseDataUint(t, audit, "operatorId"); got != 1 {
		t.Fatalf("operator id = %d, want 1", got)
	}
}

func TestRefreshTokenRejectedAsAccessToken(t *testing.T) {
	state := newAdminSessionState(t)
	router := useAdminSessionRouter(t, state)
	recorder := performJSON(router, http.MethodPost, "/api/auth/login", "", "192.0.2.8:80", bytes.NewReader([]byte(`{"userName":"root","password":"secret-pass"}`)))
	if recorder.Code != http.StatusOK || responseCode(t, recorder) != 200 {
		t.Fatalf("login = %d %s", recorder.Code, recorder.Body.String())
	}
	accessToken := responseDataString(t, recorder, "token")
	refreshToken := responseDataString(t, recorder, "refreshToken")
	accessClaims := parseTokenClaims(t, accessToken)
	refreshClaims := parseTokenClaims(t, refreshToken)
	if accessClaims.Typ != "" || accessClaims.Role != "admin" {
		t.Fatalf("access claims = %+v", accessClaims)
	}
	if refreshClaims.Typ != middleware.TokenTypeRefresh || refreshClaims.UserID != accessClaims.UserID {
		t.Fatalf("refresh claims = %+v", refreshClaims)
	}

	refreshed := performJSON(router, http.MethodGet, "/api/user/info", refreshToken, "192.0.2.8:80", nil)
	if refreshed.Code != http.StatusUnauthorized || responseCode(t, refreshed) != 401 {
		t.Fatalf("refresh token on /api/user/info = %d %s", refreshed.Code, refreshed.Body.String())
	}
	info := performJSON(router, http.MethodGet, "/api/user/info", accessToken, "192.0.2.8:80", nil)
	if info.Code != http.StatusOK || responseCode(t, info) != 200 {
		t.Fatalf("access token on /api/user/info = %d %s", info.Code, info.Body.String())
	}
}

func TestDisabledAdminTokenRejected(t *testing.T) {
	state := newAdminSessionState(t)
	router := useAdminSessionRouter(t, state)
	token := signAdminToken(t, 1, "R_SUPER")
	if recorder := performJSON(router, http.MethodGet, "/api/user/info", token, "192.0.2.8:80", nil); recorder.Code != http.StatusOK || responseCode(t, recorder) != 200 {
		t.Fatalf("enabled admin = %d %s", recorder.Code, recorder.Body.String())
	}
	state.setEnabled(1, false)
	recorder := performJSON(router, http.MethodGet, "/api/user/info", token, "192.0.2.8:80", nil)
	if recorder.Code != http.StatusUnauthorized || responseCode(t, recorder) != 401 || !strings.Contains(recorder.Body.String(), "禁用") {
		t.Fatalf("disabled admin = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDemotedSuperAdminCannotCallSuperRoute(t *testing.T) {
	state := newAdminSessionState(t)
	router := useAdminSessionRouter(t, state)
	token := signAdminToken(t, 1, "R_SUPER")
	if recorder := performJSON(router, http.MethodGet, "/api/system/config", token, "192.0.2.8:80", nil); recorder.Code != http.StatusOK || responseCode(t, recorder) != 200 {
		t.Fatalf("super route before demotion = %d %s", recorder.Code, recorder.Body.String())
	}
	state.setRoleCode(1, "R_ADMIN")
	recorder := performJSON(router, http.MethodGet, "/api/system/config", token, "192.0.2.8:80", nil)
	if recorder.Code != http.StatusUnauthorized || responseCode(t, recorder) != 401 {
		t.Fatalf("super route after demotion = %d %s", recorder.Code, recorder.Body.String())
	}
}

func newAdminSessionRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	api.POST("/auth/login", Login)

	secured := api.Group("/")
	secured.Use(middleware.JWTAuth(), middleware.RequireAdmin(), middleware.RequireFreshPassword("admins"))
	superSecured := secured.Group("")
	superSecured.Use(middleware.RequireSuperAdmin())
	secured.GET("/user/info", GetUserInfo)
	superSecured.POST("/user/:id/impersonate", AdminImpersonateUser)
	superSecured.POST("/agent/:id/impersonate", AdminImpersonateAgent)
	superSecured.GET("/system/config", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok"})
	})

	userSecured := api.Group("/user-panel")
	userSecured.Use(middleware.JWTAuth(), middleware.RequireActiveUser())
	userSecured.GET("/audit-operator", func(c *gin.Context) {
		operatorID, ok := middleware.ImpersonationOperatorID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "缺少代登录操作者"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"operatorId": operatorID}})
	})
	return router
}

func useAdminSessionRouter(t *testing.T, state *adminSessionState) *gin.Engine {
	t.Helper()
	useAdminSessionDB(t, state)
	return newAdminSessionRouter()
}

func signAdminToken(t *testing.T, id uint, roleCode string) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: id, Username: "admin", Role: "admin", RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func parseTokenClaims(t *testing.T, raw string) *middleware.Claims {
	t.Helper()
	token, err := jwt.ParseWithClaims(raw, &middleware.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return middleware.JWTSecret(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, ok := token.Claims.(*middleware.Claims)
	if !ok {
		t.Fatal("claims type")
	}
	return claims
}

func performJSON(router http.Handler, method, path, token, remoteAddr string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.RemoteAddr = remoteAddr
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func responseCode(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %s", recorder.Body.String())
	}
	return body.Code
}

func responseDataString(t *testing.T, recorder *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %s", recorder.Body.String())
	}
	value, _ := body.Data[key].(string)
	if value == "" {
		t.Fatalf("data.%s missing in %s", key, recorder.Body.String())
	}
	return value
}

func responseDataUint(t *testing.T, recorder *httptest.ResponseRecorder, key string) uint {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %s", recorder.Body.String())
	}
	value, ok := body.Data[key].(float64)
	if !ok {
		t.Fatalf("data.%s = %#v", key, body.Data[key])
	}
	return uint(value)
}

type sessionAdmin struct {
	id           uint64
	username     string
	passwordHash string
	roleID       int64
	roleCode     string
	enabled      bool
	email        string
	avatar       string
	nickname     string
}

type sessionSubject struct {
	email       string
	name        string
	nickname    string
	enabled     bool
	balance     float64
	lastLoginIP string
}

type capturedOperationLog struct {
	query      string
	operatorID int64
	action     string
	targetType string
	targetID   int64
	username   string
	ip         string
}

type adminSessionState struct {
	mu     sync.Mutex
	admins map[uint64]sessionAdmin
	user   sessionSubject
	agent  sessionSubject
	logs   []capturedOperationLog
	execs  []string
}

func newAdminSessionState(t *testing.T) *adminSessionState {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &adminSessionState{
		admins: map[uint64]sessionAdmin{
			1: {
				id: 1, username: "root", passwordHash: string(hash), roleID: 1, roleCode: "R_SUPER",
				enabled: true, email: "root@example.com", nickname: "超级管理员",
			},
		},
		user:  sessionSubject{email: "user@example.com", nickname: "用户甲", enabled: true, lastLoginIP: "198.51.100.9"},
		agent: sessionSubject{email: "agent@example.com", name: "代理甲", enabled: true, balance: 12.5, lastLoginIP: "198.51.100.10"},
	}
}

func (state *adminSessionState) setEnabled(id uint64, enabled bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	admin := state.admins[id]
	admin.enabled = enabled
	state.admins[id] = admin
}

func (state *adminSessionState) setRoleCode(id uint64, roleCode string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	admin := state.admins[id]
	admin.roleCode = roleCode
	state.admins[id] = admin
}

func (state *adminSessionState) adminByID(id int64) (sessionAdmin, bool) {
	admin, ok := state.admins[uint64(id)]
	return admin, ok
}

func (state *adminSessionState) adminByUsername(username string) (sessionAdmin, bool) {
	for _, admin := range state.admins {
		if admin.username == username && admin.enabled {
			return admin, true
		}
	}
	return sessionAdmin{}, false
}

var adminSessionDriverSeq atomic.Uint64

type adminSessionDriver struct{ state *adminSessionState }
type adminSessionConn struct{ state *adminSessionState }
type adminSessionRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func installStaticAdminSessionDB(t *testing.T) {
	t.Helper()
	state := newAdminSessionState(t)
	useAdminSessionDB(t, state)
}

func useAdminSessionDB(t *testing.T, state *adminSessionState) {
	t.Helper()
	name := fmt.Sprintf("admin-session-%d", adminSessionDriverSeq.Add(1))
	sql.Register(name, &adminSessionDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
}

func (driver *adminSessionDriver) Open(string) (driver.Conn, error) {
	return &adminSessionConn{state: driver.state}, nil
}
func (*adminSessionConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (*adminSessionConn) Close() error { return nil }
func (*adminSessionConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}

func (conn *adminSessionConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()

	switch {
	case strings.Contains(query, "FROM admins a"):
		id, ok := namedInt(args, 0)
		if !ok {
			return nil, errors.New("admin session id")
		}
		admin, found := conn.state.adminByID(id)
		if !found {
			return &adminSessionRows{columns: []string{"enabled", "role_code"}}, nil
		}
		return &adminSessionRows{columns: []string{"enabled", "role_code"}, values: [][]driver.Value{{admin.enabled, admin.roleCode}}}, nil
	case strings.Contains(query, "password_hash"):
		username, _ := namedString(args, 0)
		admin, found := conn.state.adminByUsername(username)
		if !found {
			return &adminSessionRows{columns: []string{"id", "password_hash", "role_id"}}, nil
		}
		return &adminSessionRows{
			columns: []string{"id", "password_hash", "role_id"},
			values:  [][]driver.Value{{int64(admin.id), admin.passwordHash, admin.roleID}},
		}, nil
	case strings.Contains(query, "email, avatar, nickname"):
		id, ok := namedInt(args, 0)
		if !ok {
			return nil, errors.New("profile id")
		}
		admin, found := conn.state.adminByID(id)
		if !found {
			return &adminSessionRows{columns: []string{"email", "avatar", "nickname", "role_id"}}, nil
		}
		return &adminSessionRows{
			columns: []string{"email", "avatar", "nickname", "role_id"},
			values:  [][]driver.Value{{admin.email, admin.avatar, admin.nickname, admin.roleID}},
		}, nil
	case strings.Contains(query, "role_code FROM roles") || strings.Contains(query, "SELECT role_code FROM roles"):
		id, ok := namedInt(args, 0)
		if !ok {
			return nil, errors.New("role id")
		}
		for _, admin := range conn.state.admins {
			if admin.roleID == id {
				return &adminSessionRows{columns: []string{"role_code"}, values: [][]driver.Value{{admin.roleCode}}}, nil
			}
		}
		return &adminSessionRows{columns: []string{"role_code"}}, nil
	case strings.Contains(query, "account_status"):
		return &adminSessionRows{
			columns: []string{"enabled", "account_status", "converted_agent_id"},
			values:  [][]driver.Value{{conn.state.user.enabled, "active", nil}},
		}, nil
	case strings.Contains(query, "FROM users WHERE id"):
		return &adminSessionRows{
			columns: []string{"email", "nickname", "enabled"},
			values:  [][]driver.Value{{conn.state.user.email, conn.state.user.nickname, conn.state.user.enabled}},
		}, nil
	case strings.Contains(query, "FROM agents WHERE id"):
		return &adminSessionRows{
			columns: []string{"email", "name", "balance", "enabled"},
			values:  [][]driver.Value{{conn.state.agent.email, conn.state.agent.name, conn.state.agent.balance, conn.state.agent.enabled}},
		}, nil
	case strings.Contains(query, "system_configs"):
		return &adminSessionRows{columns: []string{"key", "value"}}, nil
	case strings.Contains(query, "information_schema"):
		return &adminSessionRows{columns: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
	case strings.Contains(query, "password_changed_at"):
		return &adminSessionRows{columns: []string{"password_changed_at"}, values: [][]driver.Value{{nil}}}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func (conn *adminSessionConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()
	conn.state.execs = append(conn.state.execs, query)
	switch {
	case strings.Contains(query, "INSERT INTO operation_logs"):
		if len(args) != 6 {
			return nil, fmt.Errorf("operation log args = %d", len(args))
		}
		operatorID, _ := namedInt(args, 0)
		action, _ := namedString(args, 1)
		targetType, _ := namedString(args, 2)
		targetID, _ := namedInt(args, 3)
		username, _ := namedString(args, 4)
		ip, _ := namedString(args, 5)
		conn.state.logs = append(conn.state.logs, capturedOperationLog{
			query: query, operatorID: operatorID, action: action, targetType: targetType,
			targetID: targetID, username: username, ip: ip,
		})
		return adminSessionResult{affected: 1}, nil
	case strings.Contains(query, "UPDATE admins SET last_login"):
		return adminSessionResult{affected: 1}, nil
	case strings.Contains(query, "UPDATE users SET last_login"):
		if ip, ok := namedString(args, 0); ok {
			conn.state.user.lastLoginIP = ip
		}
		return adminSessionResult{affected: 1}, nil
	case strings.Contains(query, "UPDATE agents SET last_login"):
		if ip, ok := namedString(args, 0); ok {
			conn.state.agent.lastLoginIP = ip
		}
		return adminSessionResult{affected: 1}, nil
	default:
		return nil, fmt.Errorf("unexpected exec: %s", query)
	}
}

type adminSessionResult struct{ affected int64 }

func (result adminSessionResult) LastInsertId() (int64, error) { return 1, nil }
func (result adminSessionResult) RowsAffected() (int64, error) { return result.affected, nil }
func (rows *adminSessionRows) Columns() []string               { return rows.columns }
func (*adminSessionRows) Close() error                         { return nil }
func (rows *adminSessionRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}

func namedInt(args []driver.NamedValue, index int) (int64, bool) {
	if index >= len(args) || args[index].Value == nil {
		return 0, false
	}
	switch value := args[index].Value.(type) {
	case int64:
		return value, true
	case int32:
		return int64(value), true
	default:
		return 0, false
	}
}

func namedString(args []driver.NamedValue, index int) (string, bool) {
	if index >= len(args) {
		return "", false
	}
	value, ok := args[index].Value.(string)
	return value, ok
}
