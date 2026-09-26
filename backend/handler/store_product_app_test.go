package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

const storeProductAppMissingMsg = "找不到该应用标识，请从 应用管理 复制 app_key"
const storeFreePlanMismatchMsg = "该套餐不属于所选产品应用，请到 套餐管理 选择"

type storeProductAppState struct {
	mu           sync.Mutex
	enabled      map[string]int64
	plans        map[string]int64
	queried      []string
	queriedPlans []string
	savedKey     string
	savedPlan    string
	saved        bool
}

type storeProductAppDriver struct{ state *storeProductAppState }
type storeProductAppConn struct{ state *storeProductAppState }
type storeProductAppResult struct{}
type storeProductAppRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

var storeProductAppDriverSeq atomic.Uint64

func openStoreProductAppDB(t *testing.T, state *storeProductAppState) {
	t.Helper()
	name := fmt.Sprintf("store-product-app-%d", storeProductAppDriverSeq.Add(1))
	sql.Register(name, storeProductAppDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
}

func (driver storeProductAppDriver) Open(string) (driver.Conn, error) {
	return &storeProductAppConn{state: driver.state}, nil
}

func (driver storeProductAppDriver) OpenConnector(string) (driver.Connector, error) {
	return storeProductAppConnector{state: driver.state}, nil
}

type storeProductAppConnector struct{ state *storeProductAppState }

func (connector storeProductAppConnector) Connect(context.Context) (driver.Conn, error) {
	return &storeProductAppConn{state: connector.state}, nil
}

func (connector storeProductAppConnector) Driver() driver.Driver {
	return storeProductAppDriver{state: connector.state}
}

func (*storeProductAppConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare is not supported")
}
func (*storeProductAppConn) Close() error { return nil }
func (*storeProductAppConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("begin is not supported")
}

func (conn *storeProductAppConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()
	if strings.Contains(query, "FROM license_plans") {
		planID, _ := namedString(args, 0)
		conn.state.queriedPlans = append(conn.state.queriedPlans, planID)
		appID, ok := conn.state.plans[planID]
		if !ok {
			return &storeProductAppRows{columns: []string{"app_id"}}, nil
		}
		return &storeProductAppRows{
			columns: []string{"app_id"},
			values:  [][]driver.Value{{appID}},
		}, nil
	}
	if !strings.Contains(query, "FROM apps") || !strings.Contains(query, "app_key") {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	key, _ := namedString(args, 0)
	conn.state.queried = append(conn.state.queried, key)
	enabled, ok := conn.state.enabled[key]
	if !ok {
		return &storeProductAppRows{columns: []string{"enabled"}}, nil
	}
	return &storeProductAppRows{
		columns: []string{"enabled"},
		values:  [][]driver.Value{{enabled}},
	}, nil
}

func (conn *storeProductAppConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	conn.state.mu.Lock()
	defer conn.state.mu.Unlock()
	if !strings.Contains(query, "system_configs") {
		return nil, fmt.Errorf("unexpected exec: %s", query)
	}
	key, _ := namedString(args, 2)
	plan, _ := namedString(args, 5)
	conn.state.savedKey = key
	conn.state.savedPlan = plan
	conn.state.saved = true
	return storeProductAppResult{}, nil
}

func (storeProductAppResult) LastInsertId() (int64, error) { return 0, nil }
func (storeProductAppResult) RowsAffected() (int64, error) { return 1, nil }
func (rows *storeProductAppRows) Columns() []string        { return rows.columns }
func (*storeProductAppRows) Close() error                  { return nil }
func (rows *storeProductAppRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}

func postStoreSettings(t *testing.T, body string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/source-station/store-settings", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	AdminSourceStoreSettingsSave(ctx)
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return response.Code, response.Msg
}

func TestAdminSourceStoreSettingsSaveChecksEnabledProductApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousReady := systemConfigStorageReady
	systemConfigStorageReady = true
	t.Cleanup(func() { systemConfigStorageReady = previousReady })

	t.Run("unknown key", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"missing-app","graceDays":7}`)
		if code == 200 || msg != storeProductAppMissingMsg || state.saved {
			t.Fatalf("code=%d msg=%q saved=%v", code, msg, state.saved)
		}
	})

	t.Run("disabled key", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{"old-app": 0}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"old-app","graceDays":7}`)
		if code == 200 || msg != storeProductAppMissingMsg || state.saved {
			t.Fatalf("code=%d msg=%q saved=%v", code, msg, state.saved)
		}
	})

	t.Run("trimmed enabled key", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{"good-app": 1}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":" good-app ","graceDays":7}`)
		if code != 200 || !state.saved || state.savedKey != "good-app" {
			t.Fatalf("code=%d msg=%q saved=%v key=%q queried=%v", code, msg, state.saved, state.savedKey, state.queried)
		}
		if len(state.queried) != 1 || state.queried[0] != "good-app" {
			t.Fatalf("queried=%v", state.queried)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"  ","graceDays":7}`)
		if code != 200 || !state.saved || state.savedKey != "" || len(state.queried) != 0 {
			t.Fatalf("code=%d msg=%q saved=%v key=%q queried=%v", code, msg, state.saved, state.savedKey, state.queried)
		}
	})
}

func TestLookupEnabledStoreProductAppIDTrimsKey(t *testing.T) {
	state := &storeProductAppState{enabled: map[string]int64{"good-app": 1}}
	openStoreProductAppDB(t, state)
	db, err := config.DB()
	if err != nil {
		t.Fatal(err)
	}
	id, err := lookupEnabledStoreProductAppID(db, " good-app ")
	if err != nil || id == 0 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if len(state.queried) != 1 || state.queried[0] != "good-app" {
		t.Fatalf("queried=%v", state.queried)
	}

	_, err = lookupEnabledStoreProductAppID(db, "   ")
	if err == nil || err.Error() != "源站未配置产品应用" {
		t.Fatalf("empty err=%v", err)
	}
	_, err = lookupEnabledStoreProductAppID(db, "missing")
	if err == nil || err.Error() != "产品应用不存在或未启用" {
		t.Fatalf("missing err=%v", err)
	}
}

func TestAdminSourceStoreSettingsSaveChecksFreePlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousReady := systemConfigStorageReady
	systemConfigStorageReady = true
	t.Cleanup(func() { systemConfigStorageReady = previousReady })

	t.Run("plan of another app", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{"good-app": 1}, plans: map[string]int64{"99": 8}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"good-app","freePlanId":"99","graceDays":7}`)
		if code == 200 || msg != storeFreePlanMismatchMsg || state.saved {
			t.Fatalf("code=%d msg=%q saved=%v", code, msg, state.saved)
		}
	})

	t.Run("missing plan", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{"good-app": 1}, plans: map[string]int64{}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"good-app","freePlanId":"1","graceDays":7}`)
		if code == 200 || msg != storeFreePlanMismatchMsg || state.saved {
			t.Fatalf("code=%d msg=%q saved=%v", code, msg, state.saved)
		}
	})

	t.Run("plan without product app", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{}, plans: map[string]int64{"12": 1}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"","freePlanId":"12","graceDays":7}`)
		if code == 200 || msg != "请先选择产品应用，再选择免费套餐" || state.saved {
			t.Fatalf("code=%d msg=%q saved=%v", code, msg, state.saved)
		}
	})

	t.Run("trimmed plan of this app", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{"good-app": 1}, plans: map[string]int64{"12": 1}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"good-app","freePlanId":" 12 ","graceDays":7}`)
		if code != 200 || !state.saved || state.savedKey != "good-app" || state.savedPlan != "12" {
			t.Fatalf("code=%d msg=%q saved=%v key=%q plan=%q", code, msg, state.saved, state.savedKey, state.savedPlan)
		}
	})

	t.Run("empty plan", func(t *testing.T) {
		state := &storeProductAppState{enabled: map[string]int64{"good-app": 1}, plans: map[string]int64{"12": 1}}
		openStoreProductAppDB(t, state)
		code, msg := postStoreSettings(t, `{"productAppKey":"good-app","freePlanId":"  ","graceDays":7}`)
		if code != 200 || !state.saved || state.savedPlan != "" || len(state.queriedPlans) != 0 {
			t.Fatalf("code=%d msg=%q saved=%v plan=%q queried=%v", code, msg, state.saved, state.savedPlan, state.queriedPlans)
		}
	})
}
