package handler

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
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentLevelSelectListResponseIsOptionArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/agent-level/select-list", nil)

	writeAgentLevelSelectList(c, []agentLevelSelectOption{
		{Code: "gold", Name: "金牌代理", Discount: 7},
		{Code: "bronze", Name: "铜牌代理", Discount: 9},
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200", recorder.Code)
	}

	var body struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, recorder.Body.String())
	}
	if body.Code != 200 {
		t.Fatalf("code = %d, want 200", body.Code)
	}
	if len(body.Data) == 0 || body.Data[0] != '[' {
		t.Fatalf("data must be a JSON array for ElSelect, got %s", body.Data)
	}

	var options []agentLevelSelectOption
	if err := json.Unmarshal(body.Data, &options); err != nil {
		t.Fatalf("data is not []{code,name,discount}: %v body=%s", err, body.Data)
	}
	if len(options) != 2 {
		t.Fatalf("options = %#v, want 2 items", options)
	}
	if options[0].Code != "gold" || options[0].Name != "金牌代理" || options[0].Discount != 7 {
		t.Fatalf("first option = %#v", options[0])
	}
	if options[1].Code != "bronze" || options[1].Name != "铜牌代理" || options[1].Discount != 9 {
		t.Fatalf("second option = %#v", options[1])
	}
}

func TestAgentLevelSelectListResponseEmptyArrayNotNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeAgentLevelSelectList(c, nil)

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, recorder.Body.String())
	}
	if string(body.Data) != "[]" {
		t.Fatalf("empty select-list must serialize as [], got %s", body.Data)
	}
}

func TestQueryAgentLevelSelectOptionsReturnsEnabledLevels(t *testing.T) {
	db := openAgentLevelSelectTestDB(t, []agentLevelSelectOption{
		{Code: "gold", Name: "金牌代理", Discount: 7},
		{Code: "silver", Name: "银牌代理", Discount: 8},
	})

	list, err := queryAgentLevelSelectOptions(db)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d options, want 2: %#v", len(list), list)
	}
	if list[0].Code != "gold" || list[1].Code != "silver" {
		t.Fatalf("codes = %q,%q", list[0].Code, list[1].Code)
	}
}

func TestQueryAgentLevelSelectOptionsRequiresEnabledFilter(t *testing.T) {
	db, script := openAgentLevelSelectTestDBWithScript(t, []agentLevelSelectOption{
		{Code: "gold", Name: "金牌代理", Discount: 7},
	})
	if _, err := queryAgentLevelSelectOptions(db); err != nil {
		t.Fatalf("query: %v", err)
	}
	query := script.lastQuery
	if !strings.Contains(query, "enabled = 1") {
		t.Fatalf("select-list SQL must filter enabled=1, got %q", query)
	}
	if !strings.Contains(query, "code") || !strings.Contains(query, "name") || !strings.Contains(query, "discount") {
		t.Fatalf("select-list SQL must select code,name,discount, got %q", query)
	}
}

type agentLevelSelectTestScript struct {
	rows      []agentLevelSelectOption
	lastQuery string
}

var agentLevelSelectTestDriverID atomic.Uint64

type agentLevelSelectTestDriver struct {
	script *agentLevelSelectTestScript
}

type agentLevelSelectTestConn struct {
	script *agentLevelSelectTestScript
}

type agentLevelSelectTestRows struct {
	rows  []agentLevelSelectOption
	index int
}

func openAgentLevelSelectTestDB(t *testing.T, rows []agentLevelSelectOption) *sql.DB {
	t.Helper()
	db, _ := openAgentLevelSelectTestDBWithScript(t, rows)
	return db
}

func openAgentLevelSelectTestDBWithScript(t *testing.T, rows []agentLevelSelectOption) (*sql.DB, *agentLevelSelectTestScript) {
	t.Helper()
	script := &agentLevelSelectTestScript{rows: rows}
	name := fmt.Sprintf("agent-level-select-test-%d", agentLevelSelectTestDriverID.Add(1))
	sql.Register(name, &agentLevelSelectTestDriver{script: script})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, script
}

func (d *agentLevelSelectTestDriver) Open(string) (driver.Conn, error) {
	return &agentLevelSelectTestConn{script: d.script}, nil
}

func (c *agentLevelSelectTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *agentLevelSelectTestConn) Close() error { return nil }
func (c *agentLevelSelectTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}

func (c *agentLevelSelectTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.script.lastQuery = query
	return &agentLevelSelectTestRows{rows: c.script.rows}, nil
}

func (r *agentLevelSelectTestRows) Columns() []string { return []string{"code", "name", "discount"} }
func (r *agentLevelSelectTestRows) Close() error      { return nil }
func (r *agentLevelSelectTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	item := r.rows[r.index]
	r.index++
	dest[0], dest[1], dest[2] = item.Code, item.Name, item.Discount
	return nil
}
