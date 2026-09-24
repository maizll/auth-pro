package middleware

import (
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

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 审计 P1-05：没有「授权列表」菜单的普通管理员，不能调用 DELETE /api/license/:id。
// 拒绝形态与 RequireAdmin 一致：HTTP 200，响应体 code 为 403，前端拦截器才能展示「无权限访问」。
func TestLicenseDeleteForbiddenWithoutLicenseListMenu(t *testing.T) {
	state := &menuGateState{
		enabled:  true,
		roleCode: "R_OPS",
		menus:    map[string]bool{"Dashboard": true},
	}
	var reached atomic.Bool
	router := newMenuGateRouter(t, state, func(secured *gin.RouterGroup) {
		secured.DELETE("/license/:id", RequireMenu(MenuLicenseList), func(c *gin.Context) {
			reached.Store(true)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
		})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/license/1", signMenuGateToken(t, "R_OPS"))
	if recorder.Code != http.StatusOK || menuGateBodyCode(t, recorder) != 403 {
		t.Fatalf("DELETE /api/license/1 = %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "无权限访问") {
		t.Fatalf("missing denial message: %s", recorder.Body.String())
	}
	if reached.Load() {
		t.Fatal("handler ran without LicenseList menu")
	}
	if !state.sawMenuQuery() {
		t.Fatal("menu grant was not queried")
	}
}

func TestLicenseDeleteAllowedWhenRoleHasLicenseListMenu(t *testing.T) {
	state := &menuGateState{
		enabled:  true,
		roleCode: "R_OPS",
		menus:    map[string]bool{MenuLicenseList: true, "Dashboard": true},
	}
	var reached atomic.Bool
	router := newMenuGateRouter(t, state, func(secured *gin.RouterGroup) {
		secured.DELETE("/license/:id", RequireMenu(MenuLicenseList), func(c *gin.Context) {
			reached.Store(true)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
		})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/license/1", signMenuGateToken(t, "R_OPS"))
	if recorder.Code != http.StatusOK || menuGateBodyCode(t, recorder) != 200 {
		t.Fatalf("DELETE /api/license/1 = %d %s", recorder.Code, recorder.Body.String())
	}
	if !reached.Load() {
		t.Fatal("handler did not run")
	}
}

func TestLicenseDeleteSuperAdminBypassesMenuGrant(t *testing.T) {
	state := &menuGateState{enabled: true, roleCode: "R_SUPER"}
	var reached atomic.Bool
	router := newMenuGateRouter(t, state, func(secured *gin.RouterGroup) {
		secured.DELETE("/license/:id", RequireMenu(MenuLicenseList), func(c *gin.Context) {
			reached.Store(true)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
		})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/license/1", signMenuGateToken(t, "R_SUPER"))
	if recorder.Code != http.StatusOK || menuGateBodyCode(t, recorder) != 200 {
		t.Fatalf("super admin DELETE /api/license/1 = %d %s", recorder.Code, recorder.Body.String())
	}
	if !reached.Load() {
		t.Fatal("super admin was rejected by menu middleware")
	}
	if state.sawMenuQuery() {
		t.Fatal("R_SUPER should not consult role_menus")
	}
}

func TestDisabledMenuDoesNotGrantWrite(t *testing.T) {
	state := &menuGateState{
		enabled:  true,
		roleCode: "R_OPS",
		menus:    map[string]bool{MenuLicenseList: false},
	}
	var reached atomic.Bool
	router := newMenuGateRouter(t, state, func(secured *gin.RouterGroup) {
		secured.DELETE("/license/:id", RequireMenu(MenuLicenseList), func(c *gin.Context) {
			reached.Store(true)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
		})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/license/1", signMenuGateToken(t, "R_OPS"))
	if recorder.Code != http.StatusOK || menuGateBodyCode(t, recorder) != 403 || reached.Load() {
		t.Fatalf("disabled menu = %d reached=%v %s", recorder.Code, reached.Load(), recorder.Body.String())
	}
}

func TestRequireMenuAcceptsAnyGrantedName(t *testing.T) {
	state := &menuGateState{
		enabled:  true,
		roleCode: "R_OPS",
		menus:    map[string]bool{MenuAppVersions: true},
	}
	var reached atomic.Bool
	router := newMenuGateRouter(t, state, func(secured *gin.RouterGroup) {
		secured.DELETE("/app/:id/versions/:versionId", RequireMenu(MenuLicenseVersions, MenuAppVersions), func(c *gin.Context) {
			reached.Store(true)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
		})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/app/3/versions/9", signMenuGateToken(t, "R_OPS"))
	if recorder.Code != http.StatusOK || menuGateBodyCode(t, recorder) != 200 || !reached.Load() {
		t.Fatalf("either-menu grant = %d reached=%v %s", recorder.Code, reached.Load(), recorder.Body.String())
	}
}

func TestRequireMenuDatabaseError(t *testing.T) {
	state := &menuGateState{
		enabled:      true,
		roleCode:     "R_OPS",
		menus:        map[string]bool{MenuLicenseList: true},
		menuQueryErr: errors.New("db down"),
	}
	var reached atomic.Bool
	router := newMenuGateRouter(t, state, func(secured *gin.RouterGroup) {
		secured.DELETE("/license/:id", RequireMenu(MenuLicenseList), func(c *gin.Context) {
			reached.Store(true)
			c.JSON(http.StatusOK, gin.H{"code": 200})
		})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/license/1", signMenuGateToken(t, "R_OPS"))
	if recorder.Code != http.StatusInternalServerError || menuGateBodyCode(t, recorder) != 500 || reached.Load() {
		t.Fatalf("db error = %d reached=%v %s", recorder.Code, reached.Load(), recorder.Body.String())
	}
}

func TestRequireMenuRejectsNonAdminEvenWithSuperRoleCode(t *testing.T) {
	state := &menuGateState{enabled: true, roleCode: "R_SUPER"}
	useMenuGateDB(t, state)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/license/:id", func(c *gin.Context) {
		c.Set("role", "user")
		c.Set("role_code", "R_SUPER")
		c.Set("user_id", uint(1))
	}, RequireMenu(MenuLicenseList), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 200})
	})

	recorder := performMenuGate(router, http.MethodDelete, "/api/license/1", "")
	if recorder.Code != http.StatusOK || menuGateBodyCode(t, recorder) != 403 {
		t.Fatalf("non-admin = %d %s", recorder.Code, recorder.Body.String())
	}
	if state.sawMenuQuery() {
		t.Fatal("non-admin should not reach role_menus")
	}
}

type menuGateState struct {
	mu           sync.Mutex
	enabled      bool
	roleCode     string
	menus        map[string]bool
	menuQueryErr error
	queries      []string
}

func (state *menuGateState) sawMenuQuery() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, query := range state.queries {
		if strings.Contains(query, "role_menus") {
			return true
		}
	}
	return false
}

func newMenuGateRouter(t *testing.T, state *menuGateState, register func(secured *gin.RouterGroup)) *gin.Engine {
	t.Helper()
	useMenuGateDB(t, state)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	secured := router.Group("/api")
	secured.Use(JWTAuth(), RequireAdmin())
	register(secured)
	return router
}

func signMenuGateToken(t *testing.T, roleCode string) string {
	t.Helper()
	claims := Claims{
		UserID: 7, Username: "ops", Role: "admin", RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func performMenuGate(router http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func menuGateBodyCode(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %s", recorder.Body.String())
	}
	return body.Code
}

var menuGateDriverSeq atomic.Uint64

func useMenuGateDB(t *testing.T, state *menuGateState) {
	t.Helper()
	name := fmt.Sprintf("menu-gate-%d", menuGateDriverSeq.Add(1))
	sql.Register(name, &menuGateDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
}

type menuGateDriver struct{ state *menuGateState }
type menuGateConn struct{ state *menuGateState }
type menuGateRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (driver *menuGateDriver) Open(string) (driver.Conn, error) {
	return &menuGateConn{state: driver.state}, nil
}
func (*menuGateConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (*menuGateConn) Close() error { return nil }
func (*menuGateConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}

func (conn *menuGateConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()
	conn.state.queries = append(conn.state.queries, query)

	switch {
	case strings.Contains(query, "FROM admins a") && strings.Contains(query, "role_code"):
		return &menuGateRows{
			columns: []string{"enabled", "role_code"},
			values:  [][]driver.Value{{conn.state.enabled, conn.state.roleCode}},
		}, nil
	case strings.Contains(query, "role_menus"):
		if conn.state.menuQueryErr != nil {
			return nil, conn.state.menuQueryErr
		}
		for _, arg := range args {
			name, ok := arg.Value.(string)
			if !ok {
				continue
			}
			if conn.state.menus[name] {
				return &menuGateRows{columns: []string{"granted"}, values: [][]driver.Value{{int64(1)}}}, nil
			}
		}
		return &menuGateRows{columns: []string{"granted"}}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func (rows *menuGateRows) Columns() []string { return rows.columns }
func (*menuGateRows) Close() error           { return nil }
func (rows *menuGateRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}
