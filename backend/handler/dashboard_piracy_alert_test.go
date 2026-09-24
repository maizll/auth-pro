package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDashboardDoesNotCountUnwiredPiracyAlerts(t *testing.T) {
	script := &dashboardCountScript{}
	name := dashboardCountDriverSeq.Add(1)
	driverName := "dashboard-count-" + jsonNumber(name)
	sql.Register(driverName, &dashboardCountDriver{script: script})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	todos := dashboardTodos(db)
	alerts := dashboardRiskAlerts(db)
	for _, item := range append(todos, alerts...) {
		if strings.Contains(item.Title, "盗版告警") {
			t.Fatalf("dashboard still counts piracy alerts: %+v", item)
		}
	}
	for _, query := range script.queries {
		if strings.Contains(query, "piracy_alerts") {
			t.Fatalf("dashboard queried unwired piracy_alerts: %s", query)
		}
	}
	for _, entry := range dashboardQuickEntries() {
		if entry.Title == "盗版告警" && strings.Contains(entry.Desc, "处理风险告警") {
			t.Fatalf("quick entry still implies a live alert queue: %+v", entry)
		}
	}
}

var dashboardCountDriverSeq atomic.Uint64

type dashboardCountScript struct {
	mu      sync.Mutex
	queries []string
}

type dashboardCountDriver struct {
	script *dashboardCountScript
}

type dashboardCountConn struct {
	script *dashboardCountScript
}

type dashboardCountRows struct {
	idx int
}

func (d *dashboardCountDriver) Open(string) (driver.Conn, error) {
	return &dashboardCountConn{script: d.script}, nil
}

func (c *dashboardCountConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *dashboardCountConn) Close() error { return nil }
func (c *dashboardCountConn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin is not supported")
}

func (c *dashboardCountConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.script.mu.Lock()
	c.script.queries = append(c.script.queries, query)
	c.script.mu.Unlock()
	return &dashboardCountRows{}, nil
}

func (r *dashboardCountRows) Columns() []string { return []string{"value"} }
func (r *dashboardCountRows) Close() error      { return nil }
func (r *dashboardCountRows) Next(dest []driver.Value) error {
	if r.idx > 0 {
		return io.EOF
	}
	r.idx++
	dest[0] = int64(9)
	return nil
}
