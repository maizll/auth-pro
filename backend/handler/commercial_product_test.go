package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"auto_pro/config"
)

func TestSalePeriodAndEditionExpiry(t *testing.T) {
	if salePeriodFromDuration(0) != storePeriodPermanent || salePeriodFromDuration(365) != storePeriodYearly || salePeriodFromDuration(30) != "d30" {
		t.Fatalf("periods %s %s %s", salePeriodFromDuration(0), salePeriodFromDuration(365), salePeriodFromDuration(30))
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if nextEditionExpiry(storePeriodPermanent, nil, now) != nil {
		t.Fatal("permanent expiry")
	}
	yearly := nextEditionExpiry(storePeriodYearly, nil, now)
	if yearly == nil || !yearly.Equal(now.AddDate(0, 0, 365)) {
		t.Fatalf("yearly=%v", yearly)
	}
	days := nextEditionExpiry("d30", nil, now)
	if days == nil || !days.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("d30=%v", days)
	}
}

type commercialFlowApp struct {
	id         int64
	key        string
	enabled    int64
	commercial int64
}

type commercialFlowPlan struct {
	id      int64
	appID   int64
	name    string
	days    int64
	price   string
	enabled int64
}

type commercialFlowState struct {
	mu            sync.Mutex
	apps          []commercialFlowApp
	plans         []commercialFlowPlan
	configKey     string
	migrationDone bool
	editionTable  bool
	editionName   string
	editionCents  int64
}

func TestCommercialProductSwitchAdoptAndSalePlan(t *testing.T) {
	previous := commercialProductColumnOK
	commercialProductColumnOK = true
	t.Cleanup(func() { commercialProductColumnOK = previous })

	state := &commercialFlowState{
		apps: []commercialFlowApp{
			{id: 1, key: "shop", enabled: 1},
			{id: 2, key: "other", enabled: 1},
		},
		configKey:    " shop ",
		editionTable: true,
		editionName:  "永久商业版",
		editionCents: 19900,
	}
	db := openCommercialFlowDB(t, state)

	switched, err := setCommercialProduct(db, 2, true)
	if err != nil || switched {
		t.Fatalf("first switch switched=%v err=%v", switched, err)
	}
	switched, err = setCommercialProduct(db, 1, true)
	if err != nil || !switched {
		t.Fatalf("second switch switched=%v err=%v", switched, err)
	}
	state.mu.Lock()
	if state.apps[0].commercial != 1 || state.apps[1].commercial != 0 {
		state.mu.Unlock()
		t.Fatalf("flags=%d %d", state.apps[0].commercial, state.apps[1].commercial)
	}
	state.apps[0].commercial = 0
	state.mu.Unlock()

	list, err := listCommercialSalePlans(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["name"] != "永久商业版" || list[0]["period"] != storePeriodPermanent || list[0]["priceCents"] != int64(19900) {
		t.Fatalf("list=%v", list)
	}
	state.mu.Lock()
	adopted := state.apps[0].commercial == 1 && state.migrationDone && len(state.plans) == 1
	state.mu.Unlock()
	if !adopted {
		t.Fatal("legacy key was not adopted or edition plan was not migrated")
	}

	name, period, cents, err := loadCommercialSalePlan(db, state.plans[0].id)
	if err != nil || name != "永久商业版" || period != storePeriodPermanent || cents != 19900 {
		t.Fatalf("plan name=%s period=%s cents=%d err=%v", name, period, cents, err)
	}
	if _, _, _, err := loadCommercialSalePlan(db, 404); err == nil || err.Error() != "套餐不存在或未启用" {
		t.Fatalf("missing plan err=%v", err)
	}
}

var commercialFlowSeq atomic.Uint64

func openCommercialFlowDB(t *testing.T, state *commercialFlowState) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("commercial-flow-%d", commercialFlowSeq.Add(1))
	sql.Register(name, commercialFlowDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	config.SetDBOverrideForTest(db)
	t.Cleanup(func() { config.SetDBOverrideForTest(nil) })
	return db
}

type commercialFlowDriver struct{ state *commercialFlowState }
type commercialFlowConn struct{ state *commercialFlowState }

func (d commercialFlowDriver) Open(string) (driver.Conn, error) {
	return &commercialFlowConn{state: d.state}, nil
}

func (*commercialFlowConn) Prepare(string) (driver.Stmt, error) { return nil, fmt.Errorf("prepare") }
func (*commercialFlowConn) Close() error                        { return nil }
func (*commercialFlowConn) Begin() (driver.Tx, error)           { return nil, fmt.Errorf("begin") }

func (c *commercialFlowConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	switch {
	case strings.Contains(query, "app_key = ? AND enabled"):
		key, _ := namedString(args, 0)
		for _, app := range c.state.apps {
			if app.key == key && app.enabled == 1 {
				return oneRow("id", app.id), nil
			}
		}
		return &commercialFlowRows{columns: []string{"id"}}, nil
	case strings.Contains(query, "commercial_product = 1 AND id"):
		id := argInt(args, 0)
		n := int64(0)
		for _, app := range c.state.apps {
			if app.commercial == 1 && app.id != id {
				n++
			}
		}
		return oneRow("n", n), nil
	case strings.Contains(query, "FROM apps WHERE commercial_product = 1"):
		if strings.Contains(query, "app_key") {
			for _, app := range c.state.apps {
				if app.commercial == 1 {
					return &commercialFlowRows{columns: []string{"id", "app_key", "enabled"}, values: [][]driver.Value{{app.id, app.key, app.enabled}}}, nil
				}
			}
			return &commercialFlowRows{columns: []string{"id", "app_key", "enabled"}}, nil
		}
		n := int64(0)
		for _, app := range c.state.apps {
			if app.commercial == 1 {
				n++
			}
		}
		return oneRow("n", n), nil
	case strings.Contains(query, "FROM apps WHERE id"):
		id := argInt(args, 0)
		n := int64(0)
		for _, app := range c.state.apps {
			if app.id == id {
				n = 1
			}
		}
		return oneRow("n", n), nil
	case strings.Contains(query, "system_configs") && strings.Contains(query, "AND"):
		return oneRow("value", c.state.configKey), nil
	case strings.Contains(query, "system_configs"):
		return &commercialFlowRows{
			columns: []string{"key", "value"},
			values: [][]driver.Value{
				{storeConfigProductAppKey, c.state.configKey},
				{storeConfigGraceDays, "7"},
			},
		}, nil
	case strings.Contains(query, "schema_migrations"):
		n := int64(0)
		if c.state.migrationDone {
			n = 1
		}
		return oneRow("n", n), nil
	case strings.Contains(query, "information_schema.TABLES"):
		n := int64(0)
		if c.state.editionTable {
			n = 1
		}
		return oneRow("n", n), nil
	case strings.Contains(query, "FROM store_edition_plans"):
		return &commercialFlowRows{
			columns: []string{"name", "period", "price_cents", "enabled", "sort"},
			values:  [][]driver.Value{{c.state.editionName, storePeriodPermanent, c.state.editionCents, int64(1), int64(1)}},
		}, nil
	case strings.Contains(query, "COUNT(*) FROM license_plans"):
		return oneRow("n", int64(0)), nil
	case strings.Contains(query, "FROM license_plans WHERE id"):
		id := argInt(args, 0)
		appID := argInt(args, 1)
		for _, plan := range c.state.plans {
			if plan.id == id && plan.appID == appID {
				return &commercialFlowRows{
					columns: []string{"name", "duration_days", "price", "enabled"},
					values:  [][]driver.Value{{plan.name, plan.days, plan.price, plan.enabled}},
				}, nil
			}
		}
		return &commercialFlowRows{columns: []string{"name", "duration_days", "price", "enabled"}}, nil
	case strings.Contains(query, "FROM license_plans"):
		appID := argInt(args, 0)
		values := [][]driver.Value{}
		for _, plan := range c.state.plans {
			if plan.appID == appID && plan.enabled == 1 {
				values = append(values, []driver.Value{plan.id, plan.name, plan.days, plan.price})
			}
		}
		return &commercialFlowRows{columns: []string{"id", "name", "duration_days", "price"}, values: values}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func (c *commercialFlowConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	switch {
	case strings.Contains(query, "CASE WHEN"):
		id := argInt(args, 0)
		for i := range c.state.apps {
			if c.state.apps[i].id == id {
				c.state.apps[i].commercial = 1
			} else {
				c.state.apps[i].commercial = 0
			}
		}
	case strings.Contains(query, "WHERE app_key"):
		key, _ := namedString(args, 0)
		for i := range c.state.apps {
			if c.state.apps[i].key == key {
				c.state.apps[i].commercial = 1
			}
		}
	case strings.Contains(query, "INSERT INTO license_plans"):
		price := "0"
		if len(args) > 3 {
			switch v := args[3].Value.(type) {
			case float64:
				price = strconv.FormatFloat(v, 'f', 2, 64)
			case string:
				price = v
			}
		}
		c.state.plans = append(c.state.plans, commercialFlowPlan{
			id: 9, appID: argInt(args, 0), name: mustNamedString(args, 1), days: argInt(args, 2),
			price: price, enabled: argInt(args, 5),
		})
	case strings.Contains(query, "INSERT") && strings.Contains(query, "schema_migrations"):
		c.state.migrationDone = true
	}
	return commercialFlowResult(1), nil
}

func oneRow(column string, value driver.Value) driver.Rows {
	return &commercialFlowRows{columns: []string{column}, values: [][]driver.Value{{value}}}
}

func argInt(args []driver.NamedValue, index int) int64 {
	if index >= len(args) {
		return 0
	}
	switch v := args[index].Value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func mustNamedString(args []driver.NamedValue, index int) string {
	value, _ := namedString(args, index)
	return value
}

type commercialFlowRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (rows *commercialFlowRows) Columns() []string { return rows.columns }
func (*commercialFlowRows) Close() error           { return nil }
func (rows *commercialFlowRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}

type commercialFlowResult int64

func (r commercialFlowResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r commercialFlowResult) RowsAffected() (int64, error) { return 1, nil }
