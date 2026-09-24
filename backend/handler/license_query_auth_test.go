package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
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
)

func TestUserOwnLicenseQueryRequiresLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	secured := router.Group("/api/user-panel")
	secured.Use(middleware.JWTAuth(), middleware.RequireActiveUser(), middleware.RequireFreshPassword("users"))
	secured.GET("/license-query", UserOwnLicenseQuery)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/user-panel/license-query?account=other@example.com", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != 401 {
		t.Fatalf("anonymous body = %s", recorder.Body.String())
	}
}

func TestUserOwnLicenseQueryIgnoresForeignAccount(t *testing.T) {
	script := &ownLicenseScript{}
	name := ownLicenseDriverSeq.Add(1)
	sql.Register("own-license-"+jsonNumber(name), &ownLicenseDriver{script: script})
	db, err := sql.Open("own-license-"+jsonNumber(name), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	secured := router.Group("/api/user-panel")
	secured.Use(middleware.JWTAuth(), middleware.RequireActiveUser(), middleware.RequireFreshPassword("users"))
	secured.GET("/license-query", UserOwnLicenseQuery)

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID:   11,
		Username: "owner",
		Role:     "user",
	}).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/user-panel/license-query?account=other@example.com&email=other@example.com", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				AppName string `json:"appName"`
			} `json:"list"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v body=%s", err, recorder.Body.String())
	}
	if body.Code != 200 || body.Data.Total != 1 || len(body.Data.List) != 1 || body.Data.List[0].AppName != "Own App" {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if script.leaked {
		t.Fatal("query used the foreign account or email")
	}
	if len(script.licenseArgs) == 0 {
		t.Fatal("license query was not issued")
	}
	for _, args := range script.licenseArgs {
		owner, ok := args[0].(int64)
		if !ok || owner != 11 {
			t.Fatalf("owner arg = %#v", args)
		}
	}
}

var ownLicenseDriverSeq atomic.Uint64

type ownLicenseScript struct {
	mu          sync.Mutex
	licenseArgs [][]driver.Value
	leaked      bool
}

type ownLicenseDriver struct {
	script *ownLicenseScript
}

type ownLicenseConn struct {
	script *ownLicenseScript
}

type ownLicenseRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

func (d *ownLicenseDriver) Open(string) (driver.Conn, error) {
	return &ownLicenseConn{script: d.script}, nil
}

func (c *ownLicenseConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *ownLicenseConn) Close() error { return nil }
func (c *ownLicenseConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}

func (c *ownLicenseConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
		if text, ok := arg.Value.(string); ok && strings.Contains(text, "other@example.com") {
			c.script.mu.Lock()
			c.script.leaked = true
			c.script.mu.Unlock()
		}
	}
	if strings.Contains(query, "nickname") || strings.Contains(query, "u.email") || strings.Contains(query, "other@example.com") {
		c.script.mu.Lock()
		c.script.leaked = true
		c.script.mu.Unlock()
	}
	if strings.Contains(query, "FROM users") {
		return &ownLicenseRows{
			cols: []string{"enabled", "account_status", "converted_agent_id"},
			data: [][]driver.Value{{true, "active", nil}},
		}, nil
	}
	if strings.Contains(query, "FROM licenses") {
		c.script.mu.Lock()
		c.script.licenseArgs = append(c.script.licenseArgs, values)
		c.script.mu.Unlock()
		owner, _ := values[0].(int64)
		if strings.Contains(query, "COUNT(*)") {
			total := int64(0)
			if owner == 11 {
				total = 1
			}
			return &ownLicenseRows{cols: []string{"count"}, data: [][]driver.Value{{total}}}, nil
		}
		if owner != 11 {
			return &ownLicenseRows{cols: []string{"app_name", "plan_name", "type", "status", "started_at", "expired_at"}}, nil
		}
		opened := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		return &ownLicenseRows{
			cols: []string{"app_name", "plan_name", "type", "status", "started_at", "expired_at"},
			data: [][]driver.Value{{"Own App", "年度套餐", "domain", "active", opened, nil}},
		}, nil
	}
	return &ownLicenseRows{cols: []string{"value"}}, nil
}

func (r *ownLicenseRows) Columns() []string { return r.cols }
func (r *ownLicenseRows) Close() error      { return nil }
func (r *ownLicenseRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.idx])
	r.idx++
	return nil
}
