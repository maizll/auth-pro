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
	"os"
	"path/filepath"
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
	// adminsTaken：模拟另一个连接在检查之后抢先插入了管理员，这次插入影响 0 行。
	adminsTaken bool
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
	if c.state.adminsTaken && strings.Contains(query, "INSERT INTO admins") {
		return installProbeNoRows{}, nil
	}
	return installProbeResult{}, nil
}

type installProbeNoRows struct{}

func (installProbeNoRows) LastInsertId() (int64, error) { return 0, nil }
func (installProbeNoRows) RowsAffected() (int64, error) { return 0, nil }

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

func TestInstallCreateAdminRejectsMissingPassword(t *testing.T) {
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
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"admin"}`)
	InstallCreateAdmin(ctx)

	if recorder.Code != http.StatusBadRequest || decodeInstallCode(t, recorder) != http.StatusBadRequest {
		t.Fatalf("missing password status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoInstallExec(t, state, "INSERT INTO admins")
	if config.IsInstalled() {
		t.Fatal("missing password still wrote install.lock")
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

// useInstallProbeDBs 按库名给不同的假库，用来区分「现有配置的库」和「这次请求填的库」。
// 和生产一样每次打开一个新连接池，调用方关掉也不影响下一次。
func useInstallProbeDBs(t *testing.T, states map[string]*installProbeState) {
	t.Helper()
	registerInstallProbeDriver.Do(func() {
		sql.Register(installProbeDriverName, installProbeDriver{})
	})
	prefix := strings.ReplaceAll(t.Name(), "/", "-") + "-"
	for name, state := range states {
		installProbeStates.Store(prefix+name, state)
		key := prefix + name
		t.Cleanup(func() { installProbeStates.Delete(key) })
	}
	previous := openInstallDatabase
	openInstallDatabase = func(dsn string) (*sql.DB, error) {
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, err
		}
		if _, ok := states[cfg.DBName]; !ok {
			return nil, errors.New("unknown database " + cfg.DBName)
		}
		return sql.Open(installProbeDriverName, prefix+cfg.DBName)
	}
	t.Cleanup(func() { openInstallDatabase = previous })
}

// 复现 B2：install.lock 丢了，db.json 指向有数据的库；攻击者调 init-tables 填自己的空库，
// 以前会覆盖 db.json 并建表，再调 create-admin 就成了超级管理员。
func TestInstallLockLostCannotRepointToAnotherDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	config.ClearCachedDBConfig()
	t.Cleanup(config.ClearCachedDBConfig)
	live := &config.DBConfig{Host: "127.0.0.1", Port: "3306", Database: "authpro_live", Username: "auth", Password: "live-pass"}
	if err := config.SaveDBConfig(live); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "db.json"))
	liveState := &installProbeState{counts: map[string]int{"admins": 1, "licenses": 9}}
	attackerState := &installProbeState{counts: map[string]int{}}
	useInstallProbeDBs(t, map[string]*installProbeState{"authpro_live": liveState, "attacker_db": attackerState})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/init-tables", `{"host":"203.0.113.50","port":"3306","database":"attacker_db","username":"evil","password":"x"}`)
	InstallInitTables(ctx)
	if recorder.Code != http.StatusForbidden || decodeInstallCode(t, recorder) != http.StatusForbidden {
		t.Fatalf("改接空库应被拒绝 status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	after, _ := os.ReadFile(filepath.Join(dir, "db.json"))
	if string(before) != string(after) {
		t.Fatalf("db.json 被改写:\n%s", after)
	}
	assertNoInstallExec(t, attackerState, "DROP TABLE")
	assertNoInstallExec(t, attackerState, "CREATE TABLE")

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"attacker","adminPassword":"secret-pass"}`)
	InstallCreateAdmin(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("create-admin 应被拒绝 status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertNoInstallExec(t, liveState, "INSERT INTO admins")
	if config.IsInstalled() {
		t.Fatal("被拒绝后不应写 install.lock")
	}
}

// 现有配置连不上时同样拒绝（没法确认是不是已经装过）；现有库是空的（装到一半）可以换库重装。
func TestInstallExistingConfigUnreachableOrEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	config.ClearCachedDBConfig()
	t.Cleanup(config.ClearCachedDBConfig)
	if err := config.SaveDBConfig(&config.DBConfig{Host: "127.0.0.1", Port: "3306", Database: "half_done", Username: "a", Password: "b"}); err != nil {
		t.Fatal(err)
	}
	halfDone := &installProbeState{fail: errors.New("connection refused")}
	fresh := &installProbeState{counts: map[string]int{}}
	useInstallProbeDBs(t, map[string]*installProbeState{"half_done": halfDone, "fresh_db": fresh})
	body := `{"host":"127.0.0.1","port":"3306","database":"fresh_db","username":"a","password":"b"}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/init-tables", body)
	InstallInitTables(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("现有配置连不上应拒绝 status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	halfDone.mu.Lock()
	halfDone.fail = nil
	halfDone.counts = map[string]int{}
	halfDone.mu.Unlock()
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/init-tables", body)
	InstallInitTables(ctx)
	if recorder.Code != http.StatusOK || decodeInstallCode(t, recorder) != http.StatusOK {
		t.Fatalf("现有库为空时应允许换库 status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

// B5：首次安装时两个请求同时「创建管理员」，只能建出一个超级管理员；后到的请求被拒绝。
func TestInstallCreateAdminConcurrentRequestsCreateOneAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv("AUTO_PRO_DB_HOST", "127.0.0.1")
	t.Setenv("AUTO_PRO_DB_NAME", "authpro")
	t.Setenv("AUTO_PRO_DB_USER", "root")
	t.Setenv("AUTO_PRO_DB_PASSWORD", "secret")
	state := &installProbeState{counts: map[string]int{"admins": 0}}
	useInstallProbeDB(t, state)

	const n = 4
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"root","adminPassword":"secret-pass"}`)
			InstallCreateAdmin(ctx)
			codes[i] = decodeInstallCode(t, recorder)
		}(i)
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusForbidden:
			refused++
		}
	}
	state.mu.Lock()
	inserts := 0
	for _, query := range state.execs {
		if strings.Contains(query, "INSERT INTO admins") {
			inserts++
		}
	}
	state.mu.Unlock()
	if ok != 1 || refused != n-1 || inserts != 1 {
		t.Fatalf("want exactly one admin created, codes=%v inserts=%d", codes, inserts)
	}
}

// 检查之后、插入之前别的连接抢先建了管理员：插入影响 0 行，拒绝且不写锁文件。
func TestInstallCreateAdminRefusesWhenInsertFindsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv("AUTO_PRO_DB_HOST", "127.0.0.1")
	t.Setenv("AUTO_PRO_DB_NAME", "authpro")
	t.Setenv("AUTO_PRO_DB_USER", "root")
	t.Setenv("AUTO_PRO_DB_PASSWORD", "secret")
	state := &installProbeState{counts: map[string]int{"admins": 0}, adminsTaken: true}
	useInstallProbeDB(t, state)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = installJSONRequest(http.MethodPost, "/api/install/create-admin", `{"adminUsername":"root","adminPassword":"secret-pass"}`)
	InstallCreateAdmin(ctx)
	if decodeInstallCode(t, recorder) != http.StatusForbidden {
		t.Fatalf("insert that found an admin must be refused: %s", recorder.Body.String())
	}
	if config.IsInstalled() {
		t.Fatal("refused install must not write install.lock")
	}
	assertNoInstallExec(t, state, "INSERT IGNORE INTO role_menus")
}
