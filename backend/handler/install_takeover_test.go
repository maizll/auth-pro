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
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

const installProbeDriverName = "install-probe-test"

var registerInstallProbeDriver sync.Once
var installProbeStates sync.Map

type installProbeState struct {
	mu      sync.Mutex
	counts  map[string]int
	queries []string
	execs   []string
	fail    error
}

type installProbeDriver struct{}
type installProbeConn struct{ state *installProbeState }
type installProbeRows struct {
	values [][]driver.Value
	index  int
}
type installProbeResult struct{}

func openInstallProbeDB(t *testing.T, state *installProbeState) *sql.DB {
	t.Helper()
	registerInstallProbeDriver.Do(func() {
		sql.Register(installProbeDriverName, installProbeDriver{})
	})
	name := strings.ReplaceAll(t.Name(), "/", "-")
	installProbeStates.Store(name, state)
	t.Cleanup(func() { installProbeStates.Delete(name) })
	db, err := sql.Open(installProbeDriverName, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func (installProbeDriver) Open(name string) (driver.Conn, error) {
	value, ok := installProbeStates.Load(name)
	if !ok {
		return nil, errors.New("install probe state not found")
	}
	return &installProbeConn{state: value.(*installProbeState)}, nil
}

func (c *installProbeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *installProbeConn) Close() error { return nil }
func (c *installProbeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not used by install probes")
}

func (c *installProbeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.queries = append(c.state.queries, query)
	if c.state.fail != nil {
		return nil, c.state.fail
	}
	if !strings.Contains(query, "COUNT(*)") {
		return nil, errors.New("unexpected install query: " + query)
	}
	for table, count := range c.state.counts {
		if strings.Contains(query, "`"+table+"`") {
			return &installProbeRows{values: [][]driver.Value{{int64(count)}}}, nil
		}
	}
	return nil, &mysql.MySQLError{Number: 1146, Message: "Table doesn't exist"}
}

func (c *installProbeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.execs = append(c.state.execs, query)
	return installProbeResult{}, nil
}

func (installProbeRows) Columns() []string { return []string{"count"} }
func (installProbeRows) Close() error      { return nil }
func (r *installProbeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func (installProbeResult) LastInsertId() (int64, error) { return 1, nil }
func (installProbeResult) RowsAffected() (int64, error) { return 1, nil }

func useInstallProbeDB(t *testing.T, state *installProbeState) {
	t.Helper()
	db := openInstallProbeDB(t, state)
	previous := openInstallDatabase
	openInstallDatabase = func(string) (*sql.DB, error) { return db, nil }
	t.Cleanup(func() { openInstallDatabase = previous })
}

func installJSONRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func decodeInstallCode(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var response struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	return response.Code
}

func assertNoInstallExec(t *testing.T, state *installProbeState, fragment string) {
	t.Helper()
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, query := range state.execs {
		if strings.Contains(strings.ToUpper(query), strings.ToUpper(fragment)) {
			t.Fatalf("unexpected statement containing %q: %s", fragment, query)
		}
	}
}

func TestInstallCreateAdminRefusesExistingAdmins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv("AUTO_PRO_DB_HOST", "127.0.0.1")
	t.Setenv("AUTO_PRO_DB_NAME", "authpro")
	t.Setenv("AUTO_PRO_DB_USER", "root")
	t.Setenv("AUTO_PRO_DB_PASSWORD", "secret")
	state := &installProbeState{counts: map[string]int{"admins": 1}}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"attacker","adminPassword":"secret-pass"}`)
	InstallCreateAdmin(ctx)

	if recorder.Code != http.StatusForbidden || decodeInstallCode(t, recorder) != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoInstallExec(t, state, "INSERT INTO admins")
	if config.IsInstalled() {
		t.Fatal("refused create-admin still wrote install.lock")
	}
}

func TestInstallCreateAdminRefusesWhenBusinessRowsExist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv("AUTO_PRO_DB_HOST", "127.0.0.1")
	t.Setenv("AUTO_PRO_DB_NAME", "authpro")
	t.Setenv("AUTO_PRO_DB_USER", "root")
	t.Setenv("AUTO_PRO_DB_PASSWORD", "secret")
	state := &installProbeState{counts: map[string]int{"licenses": 2}}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"attacker","adminPassword":"secret-pass"}`)
	InstallCreateAdmin(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoInstallExec(t, state, "INSERT INTO admins")
}

func TestInstallInitTablesRefusesDropWhenDataExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	state := &installProbeState{counts: map[string]int{"admins": 1, "licenses": 4}}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/init-tables", `{"host":"127.0.0.1","port":"3306","database":"authpro","username":"root","password":"secret"}`)
	InstallInitTables(ctx)

	if recorder.Code != http.StatusForbidden || decodeInstallCode(t, recorder) != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoInstallExec(t, state, "DROP TABLE")
}

func TestInstallInitTablesDoesNotDropWhenProbeFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	state := &installProbeState{fail: errors.New("information schema unavailable")}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/init-tables", `{"host":"127.0.0.1","port":"3306","database":"authpro","username":"root","password":"secret"}`)
	InstallInitTables(ctx)

	if decodeInstallCode(t, recorder) == http.StatusOK {
		t.Fatalf("probe failure was treated as success: %s", recorder.Body.String())
	}
	assertNoInstallExec(t, state, "DROP TABLE")
}

func TestInstallInitTablesStillBuildsEmptyDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Cleanup(config.ClearCachedDBConfig)
	state := &installProbeState{counts: map[string]int{}}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/init-tables", `{"host":"127.0.0.1","port":"3306","database":"authpro","username":"root","password":"secret"}`)
	InstallInitTables(ctx)

	if recorder.Code != http.StatusOK || decodeInstallCode(t, recorder) != http.StatusOK {
		t.Fatalf("empty install status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	foundDrop := false
	for _, query := range state.execs {
		if strings.Contains(query, "DROP TABLE") {
			foundDrop = true
		}
	}
	if !foundDrop {
		t.Fatal("empty database did not run the installer schema")
	}
}

func TestInstallCreateAdminStillCreatesFirstAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	t.Setenv("AUTO_PRO_DB_HOST", "127.0.0.1")
	t.Setenv("AUTO_PRO_DB_NAME", "authpro")
	t.Setenv("AUTO_PRO_DB_USER", "root")
	t.Setenv("AUTO_PRO_DB_PASSWORD", "secret")
	state := &installProbeState{counts: map[string]int{"admins": 0}}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"root","adminPassword":"secret-pass"}`)
	InstallCreateAdmin(ctx)

	if recorder.Code != http.StatusOK || decodeInstallCode(t, recorder) != http.StatusOK {
		t.Fatalf("fresh admin status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	foundInsert := false
	for _, query := range state.execs {
		if strings.Contains(query, "INSERT INTO admins") {
			foundInsert = true
		}
	}
	if !foundInsert {
		t.Fatal("fresh install did not insert the first admin")
	}
	if !config.IsInstalled() {
		t.Fatal("fresh install did not write install.lock")
	}
}
