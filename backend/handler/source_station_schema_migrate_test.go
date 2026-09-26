package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestEnsureSourceStationSchemaCopiesFilePathAndDoesNotDropTwice(t *testing.T) {
	state := newLegacySourceSchemaState()
	state.plugins = []sourceSchemaPluginRow{{
		id: "p1", filePath: "plugins/demo.zip", downloadURL: "",
	}}
	state.templates = []sourceSchemaTemplateRow{{
		id: "t1", filePath: "templates/home.json", templateURL: "", previewPath: "templates/home-preview.json",
	}}
	db := openSourceSchemaMigrateDB(t, state)

	if err := ensureSourceStationStorage(db); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if got := state.pluginDownloadURL("p1"); got != "plugins/demo.zip" {
		t.Fatalf("download_url = %q, want plugins/demo.zip", got)
	}
	if got := state.templateURL("t1"); got != "templates/home.json" {
		t.Fatalf("template_url = %q, want templates/home.json", got)
	}
	if state.hasColumn("source_catalog_plugins", "file_path") {
		t.Fatal("file_path column still present after a successful migration")
	}
	for _, name := range sourceStationMigrationNames {
		if !state.migrationApplied(name) {
			t.Fatalf("migration %s was not recorded", name)
		}
	}

	// 列被重新加回来时，已记录的迁移仍不得再次 DROP。
	state.setColumn("source_catalog_plugins", "file_path", true)
	state.setColumn("source_catalog_templates", "preview_path", true)
	before := state.execCount()
	if err := ensureSourceStationStorage(db); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	for _, query := range state.execsAfter(before) {
		if strings.Contains(strings.ToUpper(query), "DROP COLUMN") {
			t.Fatalf("second ensure emitted DROP COLUMN: %s", query)
		}
	}
}

func TestEnsureSourceStationSchemaRefusesDropWhenFilePathWouldBeLost(t *testing.T) {
	state := newLegacySourceSchemaState()
	state.refuseFilePathBackfill = true
	state.plugins = []sourceSchemaPluginRow{{
		id: "p1", filePath: "plugins/only-on-disk.zip", downloadURL: "",
	}}
	db := openSourceSchemaMigrateDB(t, state)

	err := ensureSourceStationStorage(db)
	if err == nil {
		t.Fatal("expected migration to fail when file_path would not land on download_url")
	}
	if !strings.Contains(err.Error(), "file_path") {
		t.Fatalf("error = %q, want file_path", err)
	}
	if !state.hasColumn("source_catalog_plugins", "file_path") {
		t.Fatal("file_path was dropped even though download_url stayed empty")
	}
	if state.migrationApplied("source_catalog_plugin_file_path_v1") {
		t.Fatal("failed file_path migration was recorded")
	}
	for _, query := range state.allExecs() {
		if strings.Contains(strings.ToUpper(query), "DROP COLUMN") {
			t.Fatalf("refusing migration still emitted DROP COLUMN: %s", query)
		}
	}
}

func TestSourceCatalogPriceMigrationOnExistingDB(t *testing.T) {
	state := newLegacySourceSchemaState()
	for _, table := range []string{"source_catalog_plugins", "source_catalog_templates"} {
		if state.hasColumn(table, "price_cents") || state.hasColumn(table, "billing") || state.hasColumn(table, "delivery") {
			t.Fatalf("legacy %s already has price columns", table)
		}
	}
	db := openSourceSchemaMigrateDB(t, state)
	if err := ensureSourceStationStorage(db); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !state.migrationApplied("source_catalog_price_v1") {
		t.Fatal("source_catalog_price_v1 was not recorded")
	}
	for _, table := range []string{"source_catalog_plugins", "source_catalog_templates"} {
		for _, column := range []string{"price_cents", "billing", "delivery"} {
			if !state.hasColumn(table, column) {
				t.Fatalf("%s.%s missing after migration", table, column)
			}
		}
	}
	before := state.execCount()
	if err := ensureSourceStationStorage(db); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	for _, query := range state.execsAfter(before) {
		upper := strings.ToUpper(query)
		if strings.Contains(upper, "ADD COLUMN") && (strings.Contains(query, "price_cents") || strings.Contains(query, " billing") || strings.Contains(query, " delivery")) {
			t.Fatalf("second ensure re-added a price column: %s", query)
		}
	}
}

func TestEnsureSourceStationSchemaReturnsMigrationExecError(t *testing.T) {
	state := newLegacySourceSchemaState()
	state.failExecContaining = "DROP COLUMN file_path"
	state.plugins = []sourceSchemaPluginRow{{
		id: "p1", filePath: "plugins/demo.zip", downloadURL: "",
	}}
	db := openSourceSchemaMigrateDB(t, state)

	err := ensureSourceStationStorage(db)
	if err == nil {
		t.Fatal("expected DROP COLUMN failure to fail Ensure")
	}
	if state.migrationApplied("source_catalog_plugin_file_path_v1") {
		t.Fatal("migration was recorded after DROP COLUMN failed")
	}
}

var sourceStationMigrationNames = []string{
	"source_catalog_additive_v1",
	"source_catalog_plugin_file_path_v1",
	"source_catalog_plugin_published_v1",
	"source_catalog_template_legacy_v1",
	"source_catalog_version_backfill_v1",
	"source_catalog_app_id_backfill_v1",
	"source_developer_agent_backfill_v1",
	"source_catalog_price_v1",
	"source_catalog_origin_v1",
	"store_bindings_v1",
	"store_editions_v1",
	"store_purchase_orders_v1",
	"plugin_entitlements_v1",
	"store_revenue_ledger_v1",
	"licenses_source_store_bind_v1",
	"licenses_source_store_purchase_v1",
	"license_domain_changes_v1",
}

type sourceSchemaPluginRow struct {
	id          string
	filePath    string
	downloadURL string
}

type sourceSchemaTemplateRow struct {
	id          string
	filePath    string
	templateURL string
	previewPath string
}

type sourceSchemaMigrateState struct {
	mu sync.Mutex

	columns                map[string]map[string]bool
	indexes                map[string]map[string]bool
	migrations             map[string]bool
	plugins                []sourceSchemaPluginRow
	templates              []sourceSchemaTemplateRow
	execs                  []string
	refuseFilePathBackfill bool
	failExecContaining     string
}

func newLegacySourceSchemaState() *sourceSchemaMigrateState {
	return &sourceSchemaMigrateState{
		columns: map[string]map[string]bool{
			"source_catalog_plugins": {
				"id": true, "sha256": true, "version": true, "file_path": true, "published": true,
			},
			"source_catalog_templates": {
				"id": true, "sha256": true, "version": true, "file_path": true,
				"preview_path": true, "preview_content_type": true, "format": true, "published": true,
			},
			"source_developer_applications": {"id": true, "username": true},
			"source_developers":             {"id": true, "username": true, "application_id": true},
		},
		indexes:    map[string]map[string]bool{},
		migrations: map[string]bool{},
	}
}

func (s *sourceSchemaMigrateState) hasColumn(table, column string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.columns[table][column]
}

func (s *sourceSchemaMigrateState) setColumn(table, column string, present bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureColumnMap(table)
	if present {
		s.columns[table][column] = true
		return
	}
	delete(s.columns[table], column)
}

func (s *sourceSchemaMigrateState) ensureColumnMap(table string) {
	if s.columns[table] == nil {
		s.columns[table] = map[string]bool{}
	}
}

func (s *sourceSchemaMigrateState) migrationApplied(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.migrations[name]
}

func (s *sourceSchemaMigrateState) pluginDownloadURL(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.plugins {
		if row.id == id {
			return row.downloadURL
		}
	}
	return ""
}

func (s *sourceSchemaMigrateState) templateURL(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.templates {
		if row.id == id {
			return row.templateURL
		}
	}
	return ""
}

func (s *sourceSchemaMigrateState) execCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.execs)
}

func (s *sourceSchemaMigrateState) execsAfter(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.execs) {
		return nil
	}
	out := make([]string, len(s.execs)-n)
	copy(out, s.execs[n:])
	return out
}

func (s *sourceSchemaMigrateState) allExecs() []string {
	return s.execsAfter(0)
}

func openSourceSchemaMigrateDB(t *testing.T, state *sourceSchemaMigrateState) *sql.DB {
	t.Helper()
	db := sql.OpenDB(sourceSchemaMigrateConnector{state: state})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type sourceSchemaMigrateConnector struct{ state *sourceSchemaMigrateState }

func (c sourceSchemaMigrateConnector) Connect(context.Context) (driver.Conn, error) {
	return &sourceSchemaMigrateConn{state: c.state}, nil
}

func (c sourceSchemaMigrateConnector) Driver() driver.Driver { return sourceSchemaMigrateDriver{} }

type sourceSchemaMigrateDriver struct{}

func (sourceSchemaMigrateDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("source schema migrate test driver requires a connector")
}

type sourceSchemaMigrateConn struct{ state *sourceSchemaMigrateState }

func (c *sourceSchemaMigrateConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not used")
}
func (c *sourceSchemaMigrateConn) Close() error { return nil }
func (c *sourceSchemaMigrateConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not used")
}

func (c *sourceSchemaMigrateConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	state := c.state
	state.mu.Lock()
	defer state.mu.Unlock()
	state.execs = append(state.execs, query)
	if state.failExecContaining != "" && strings.Contains(query, state.failExecContaining) {
		return nil, errors.New("injected exec failure")
	}
	upper := strings.ToUpper(query)
	switch {
	case strings.Contains(upper, "INSERT") && strings.Contains(query, "schema_migrations"):
		name := sourceSchemaArgString(args, 0)
		if name == "" {
			return nil, errors.New("schema_migrations insert missing name")
		}
		state.migrations[name] = true
	case strings.Contains(upper, "ADD COLUMN"):
		table, column := sourceSchemaAlterTarget(query, "ADD COLUMN")
		state.ensureColumnMap(table)
		state.columns[table][column] = true
	case strings.Contains(upper, "DROP COLUMN"):
		table, column := sourceSchemaAlterTarget(query, "DROP COLUMN")
		state.ensureColumnMap(table)
		delete(state.columns[table], column)
	case strings.Contains(upper, "ADD UNIQUE KEY") || strings.Contains(upper, "ADD KEY"):
		table, index := sourceSchemaAlterTarget(query, "KEY")
		if state.indexes[table] == nil {
			state.indexes[table] = map[string]bool{}
		}
		state.indexes[table][index] = true
	case strings.Contains(upper, "UPDATE SOURCE_CATALOG_PLUGINS") && strings.Contains(query, "download_url") && strings.Contains(query, "file_path"):
		if !state.refuseFilePathBackfill {
			for i := range state.plugins {
				if strings.TrimSpace(state.plugins[i].downloadURL) == "" && strings.TrimSpace(state.plugins[i].filePath) != "" {
					state.plugins[i].downloadURL = state.plugins[i].filePath
				}
			}
		}
	case strings.Contains(upper, "UPDATE SOURCE_CATALOG_TEMPLATES") && strings.Contains(query, "template_url") && strings.Contains(query, "file_path") && !strings.Contains(query, "preview_path"):
		for i := range state.templates {
			if strings.TrimSpace(state.templates[i].templateURL) == "" && strings.TrimSpace(state.templates[i].filePath) != "" {
				state.templates[i].templateURL = state.templates[i].filePath
			}
		}
	case strings.Contains(upper, "UPDATE SOURCE_CATALOG_TEMPLATES") && strings.Contains(query, "preview_path") && strings.Contains(query, "template_url"):
		for i := range state.templates {
			if strings.TrimSpace(state.templates[i].templateURL) == "" && strings.TrimSpace(state.templates[i].previewPath) != "" {
				state.templates[i].templateURL = state.templates[i].previewPath
			}
		}
	}
	return sourceSchemaMigrateResult{}, nil
}

func (c *sourceSchemaMigrateConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	state := c.state
	state.mu.Lock()
	defer state.mu.Unlock()
	var count int64
	switch {
	case strings.Contains(query, "schema_migrations"):
		if state.migrations[sourceSchemaArgString(args, 0)] {
			count = 1
		}
	case strings.Contains(query, "information_schema.COLUMNS"):
		table := sourceSchemaArgString(args, 0)
		column := sourceSchemaArgString(args, 1)
		if state.columns[table][column] {
			count = 1
		}
	case strings.Contains(query, "information_schema.STATISTICS"):
		table := sourceSchemaArgString(args, 0)
		index := sourceSchemaArgString(args, 1)
		if state.indexes[table][index] {
			count = 1
		}
	case strings.Contains(query, "COUNT(*)") && strings.Contains(query, "source_catalog_plugins") && strings.Contains(query, "file_path") && strings.Contains(query, "download_url"):
		for _, row := range state.plugins {
			if strings.TrimSpace(row.filePath) != "" && strings.TrimSpace(row.downloadURL) == "" {
				count++
			}
		}
	case strings.Contains(query, "COUNT(*)") && strings.Contains(query, "source_catalog_templates") && strings.Contains(query, "preview_path") && strings.Contains(query, "template_url"):
		for _, row := range state.templates {
			if strings.TrimSpace(row.previewPath) != "" && strings.TrimSpace(row.templateURL) == "" {
				count++
			}
		}
	case strings.Contains(query, "COUNT(*)") && strings.Contains(query, "source_catalog_templates") && strings.Contains(query, "file_path") && strings.Contains(query, "template_url"):
		for _, row := range state.templates {
			if strings.TrimSpace(row.filePath) != "" && strings.TrimSpace(row.templateURL) == "" {
				count++
			}
		}
	default:
		return nil, errors.New("unexpected query: " + query)
	}
	return &sourceSchemaMigrateRows{values: []driver.Value{count}}, nil
}

var sourceSchemaAlterPattern = regexp.MustCompile(`(?i)ALTER TABLE\s+(\w+)\s+(?:ADD\s+(?:UNIQUE\s+)?(?:COLUMN|KEY)|DROP\s+COLUMN)\s+(\w+)`)

func sourceSchemaAlterTarget(query, _ string) (string, string) {
	match := sourceSchemaAlterPattern.FindStringSubmatch(query)
	if len(match) != 3 {
		return "", ""
	}
	return match[1], match[2]
}

func sourceSchemaArgString(args []driver.NamedValue, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	switch value := args[index].Value.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return ""
	}
}

type sourceSchemaMigrateRows struct {
	values []driver.Value
	done   bool
}

func (r *sourceSchemaMigrateRows) Columns() []string { return []string{"count"} }
func (r *sourceSchemaMigrateRows) Close() error      { return nil }
func (r *sourceSchemaMigrateRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.values[0]
	return nil
}

type sourceSchemaMigrateResult struct{}

func (sourceSchemaMigrateResult) LastInsertId() (int64, error) { return 0, nil }
func (sourceSchemaMigrateResult) RowsAffected() (int64, error) { return 1, nil }
