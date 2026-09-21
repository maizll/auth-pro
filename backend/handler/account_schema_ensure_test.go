package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

type schemaLockTestState struct {
	mu sync.Mutex

	deadlocksRemaining int
	lockNumber         uint16
	deadlockOnOverlap  bool
	stickyErr          error
	pause              time.Duration

	inFlight                 int
	maxInFlight              int
	execs                    int
	queries                  int
	deadlocksReturned        int
	otherErrors              int
	agentLevelCreateAttempts int
}

func (s *schemaLockTestState) operations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execs + s.queries
}

func (s *schemaLockTestState) snapshot() (creates, maxInFlight, deadlocks, others int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentLevelCreateAttempts, s.maxInFlight, s.deadlocksReturned, s.otherErrors
}

func (s *schemaLockTestState) begin(query string) (func(), error) {
	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.maxInFlight {
		s.maxInFlight = s.inFlight
	}
	overlap := s.inFlight > 1
	if strings.Contains(query, "CREATE TABLE IF NOT EXISTS agent_levels") {
		s.agentLevelCreateAttempts++
	}
	var err error
	var pause time.Duration
	switch {
	case overlap && s.deadlockOnOverlap:
		s.deadlocksReturned++
		err = &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock; try restarting transaction"}
	case s.deadlocksRemaining > 0:
		s.deadlocksRemaining--
		s.deadlocksReturned++
		number := s.lockNumber
		if number == 0 {
			number = 1213
		}
		message := "Deadlock found when trying to get lock; try restarting transaction"
		if number == 1205 {
			message = "Lock wait timeout exceeded; try restarting transaction"
		}
		err = &mysql.MySQLError{Number: number, Message: message}
	case s.stickyErr != nil:
		s.otherErrors++
		err = s.stickyErr
	case s.pause > 0 && strings.Contains(query, "CREATE TABLE IF NOT EXISTS agent_levels"):
		pause = s.pause
	}
	s.mu.Unlock()

	finish := func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}
	if err != nil {
		finish()
		return nil, err
	}
	if pause > 0 {
		time.Sleep(pause)
	}
	return finish, nil
}

type schemaLockTestConnector struct {
	state *schemaLockTestState
}

func (c schemaLockTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &schemaLockTestConn{state: c.state}, nil
}

func (c schemaLockTestConnector) Driver() driver.Driver { return schemaLockTestDriver{} }

type schemaLockTestDriver struct{}

func (schemaLockTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("schema lock test driver requires a connector")
}

type schemaLockTestConn struct {
	state *schemaLockTestState
}

func (c *schemaLockTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not used")
}

func (c *schemaLockTestConn) Close() error { return nil }

func (c *schemaLockTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not used")
}

func (c *schemaLockTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	finish, err := c.state.begin(query)
	if err != nil {
		return nil, err
	}
	defer finish()
	c.state.mu.Lock()
	c.state.execs++
	c.state.mu.Unlock()
	return schemaLockTestResult{}, nil
}

func (c *schemaLockTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	finish, err := c.state.begin(query)
	if err != nil {
		return nil, err
	}
	defer finish()
	c.state.mu.Lock()
	c.state.queries++
	c.state.mu.Unlock()
	return &schemaLockTestRows{value: schemaLockTestValue(query)}, nil
}

func schemaLockTestValue(query string) driver.Value {
	upper := strings.ToUpper(query)
	if strings.Contains(upper, "COLUMN_TYPE") {
		return "enum('recharge','consume','refund','purchase','transfer','bonus')"
	}
	if strings.Contains(upper, "CHARACTER_MAXIMUM_LENGTH") {
		return int64(65535)
	}
	return int64(1)
}

type schemaLockTestRows struct {
	value driver.Value
	done  bool
}

func (r *schemaLockTestRows) Columns() []string { return []string{"v"} }
func (r *schemaLockTestRows) Close() error      { return nil }

func (r *schemaLockTestRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

type schemaLockTestResult struct{}

func (schemaLockTestResult) LastInsertId() (int64, error) { return 0, nil }
func (schemaLockTestResult) RowsAffected() (int64, error) { return 1, nil }

func openSchemaLockTestDB(t *testing.T, state *schemaLockTestState) *sql.DB {
	t.Helper()
	previousBackoff := schemaEnsureBackoff
	schemaEnsureBackoff = func(int) {}
	t.Cleanup(func() { schemaEnsureBackoff = previousBackoff })
	resetAccountSchemaEnsureForTest()
	t.Cleanup(resetAccountSchemaEnsureForTest)
	db := sql.OpenDB(schemaLockTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestEnsureAccountUpgradeSchemaRetriesDeadlockThenSucceeds(t *testing.T) {
	state := &schemaLockTestState{deadlocksRemaining: 2, lockNumber: 1213}
	db := openSchemaLockTestDB(t, state)

	err := EnsureAccountUpgradeSchema(db)
	if err != nil {
		t.Fatalf("EnsureAccountUpgradeSchema returned deadlock to caller: %v", err)
	}
	_, _, deadlocks, _ := state.snapshot()
	if deadlocks != 2 {
		t.Fatalf("deadlock responses = %d, want 2 before success", deadlocks)
	}
	if state.operations() == 0 {
		t.Fatal("successful attempt did not run schema SQL")
	}
}

func TestEnsureAccountUpgradeSchemaRetriesLockWaitThenSucceeds(t *testing.T) {
	state := &schemaLockTestState{deadlocksRemaining: 1, lockNumber: 1205}
	db := openSchemaLockTestDB(t, state)

	if err := EnsureAccountUpgradeSchema(db); err != nil {
		t.Fatalf("EnsureAccountUpgradeSchema returned lock wait to caller: %v", err)
	}
	_, _, deadlocks, _ := state.snapshot()
	if deadlocks != 1 {
		t.Fatalf("lock-wait responses = %d, want 1 before success", deadlocks)
	}
}

func TestEnsureAccountUpgradeSchemaReturnsDeadlockAfterRetriesExhausted(t *testing.T) {
	state := &schemaLockTestState{deadlocksRemaining: 20, lockNumber: 1213}
	db := openSchemaLockTestDB(t, state)

	err := EnsureAccountUpgradeSchema(db)
	if err == nil {
		t.Fatal("expected deadlock after retries were exhausted")
	}
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1213 {
		t.Fatalf("error = %v, want wrapped MySQL 1213", err)
	}
	if !strings.Contains(err.Error(), "初始化代理等级失败") {
		t.Fatalf("error = %q, want agent-level wrapper", err)
	}
	_, _, deadlocks, _ := state.snapshot()
	if deadlocks != 5 {
		t.Fatalf("deadlock responses = %d, want 5 retries", deadlocks)
	}
}

func TestEnsureAccountUpgradeSchemaDoesNotRetryUnrelatedMySQLError(t *testing.T) {
	state := &schemaLockTestState{stickyErr: &mysql.MySQLError{Number: 1064, Message: "syntax error"}}
	db := openSchemaLockTestDB(t, state)

	err := EnsureAccountUpgradeSchema(db)
	if err == nil {
		t.Fatal("expected syntax error")
	}
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1064 {
		t.Fatalf("error = %v, want MySQL 1064", err)
	}
	_, _, _, others := state.snapshot()
	if others != 1 {
		t.Fatalf("unrelated error attempts = %d, want 1", others)
	}
}

func TestEnsureAccountUpgradeSchemaConcurrentHeavyWorkRunsOnce(t *testing.T) {
	state := &schemaLockTestState{deadlockOnOverlap: true, pause: 40 * time.Millisecond}
	db := openSchemaLockTestDB(t, state)

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = EnsureAccountUpgradeSchema(db)
		}(i)
	}
	close(start)
	waitGroup(t, &wg)

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d got %v; deadlock must not reach the API", i, err)
		}
	}
	creates, maxInFlight, _, _ := state.snapshot()
	if creates != 1 {
		t.Fatalf("agent_levels create attempts = %d, want 1 under the schema mutex", creates)
	}
	if maxInFlight != 1 {
		t.Fatalf("max in-flight schema SQL = %d, want 1", maxInFlight)
	}

	before := state.operations()
	if err := EnsureAccountUpgradeSchema(db); err != nil {
		t.Fatalf("fast path: %v", err)
	}
	if after := state.operations(); after != before {
		t.Fatalf("fast path ran SQL again: before=%d after=%d", before, after)
	}
}

func TestEnsureAgentLevelSchemaSharesMutexWithAccountUpgrade(t *testing.T) {
	state := &schemaLockTestState{deadlockOnOverlap: true, pause: 40 * time.Millisecond}
	db := openSchemaLockTestDB(t, state)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	wg.Add(len(errs))
	start := make(chan struct{})
	for i := range errs {
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				errs[i] = EnsureAccountUpgradeSchema(db)
				return
			}
			errs[i] = ensureAgentLevelSchema(db)
		}(i)
	}
	close(start)
	waitGroup(t, &wg)

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d got %v", i, err)
		}
	}
	creates, maxInFlight, _, _ := state.snapshot()
	if creates != 1 {
		t.Fatalf("agent_levels create attempts = %d, want 1 when both entry points share the mutex", creates)
	}
	if maxInFlight != 1 {
		t.Fatalf("max in-flight schema SQL = %d, want 1", maxInFlight)
	}
}

func TestEnsureAccountUpgradeSchemaFastPathSkipsRepeatedDDL(t *testing.T) {
	state := &schemaLockTestState{}
	db := openSchemaLockTestDB(t, state)

	if err := EnsureAccountUpgradeSchema(db); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	first := state.operations()
	if first == 0 {
		t.Fatal("first ensure did not touch the database")
	}
	if err := EnsureAccountUpgradeSchema(db); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second := state.operations(); second != first {
		t.Fatalf("second ensure still ran DDL/DML: before=%d after=%d", first, second)
	}
	if err := ensureAgentLevelSchema(db); err != nil {
		t.Fatalf("agent level after account upgrade: %v", err)
	}
	if third := state.operations(); third != first {
		t.Fatalf("agent level ensure reran SQL after account upgrade: before=%d after=%d", first, third)
	}
}

func TestResetAccountSchemaEnsureForTestRerunsDDL(t *testing.T) {
	state := &schemaLockTestState{}
	db := openSchemaLockTestDB(t, state)

	if err := EnsureAccountUpgradeSchema(db); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	first := state.operations()
	resetAccountSchemaEnsureForTest()
	if err := EnsureAccountUpgradeSchema(db); err != nil {
		t.Fatalf("ensure after reset: %v", err)
	}
	if second := state.operations(); second <= first {
		t.Fatalf("forced rerun did not execute schema SQL: before=%d after=%d", first, second)
	}
}

func waitGroup(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("schema ensure calls did not finish; mutex may be deadlocked")
	}
}
