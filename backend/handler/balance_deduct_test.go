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
)

const balanceMemDriverName = "balance-mem-test"

var registerBalanceMemDriver sync.Once
var balanceMemStates sync.Map

type balanceMemState struct {
	mu       sync.Mutex
	cond     *sync.Cond
	balance  float64
	lockHeld bool
	licenses int
}

type balanceMemDriver struct{}
type balanceMemConn struct {
	state          *balanceMemState
	holdsLock      bool
	pendingInserts int
}
type balanceMemTx struct{ conn *balanceMemConn }
type balanceMemRows struct {
	values [][]driver.Value
	index  int
}
type balanceMemResult struct {
	rows    int64
	rowsSet bool
}

func openBalanceMemDB(t *testing.T, state *balanceMemState) *sql.DB {
	t.Helper()
	if state.cond == nil {
		state.cond = sync.NewCond(&state.mu)
	}
	registerBalanceMemDriver.Do(func() {
		sql.Register(balanceMemDriverName, balanceMemDriver{})
	})
	name := strings.ReplaceAll(t.Name(), "/", "-")
	balanceMemStates.Store(name, state)
	t.Cleanup(func() { balanceMemStates.Delete(name) })
	db, err := sql.Open(balanceMemDriverName, name)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func (balanceMemDriver) Open(name string) (driver.Conn, error) {
	value, ok := balanceMemStates.Load(name)
	if !ok {
		return nil, errors.New("balance memory state not found")
	}
	return &balanceMemConn{state: value.(*balanceMemState)}, nil
}

func (c *balanceMemConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (c *balanceMemConn) Close() error { return nil }
func (c *balanceMemConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *balanceMemConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &balanceMemTx{conn: c}, nil
}

func (c *balanceMemConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT balance") {
		return nil, errors.New("unexpected balance query: " + query)
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if strings.Contains(query, "FOR UPDATE") {
		for c.state.lockHeld && !c.holdsLock {
			c.state.cond.Wait()
		}
		c.state.lockHeld = true
		c.holdsLock = true
	}
	return &balanceMemRows{values: [][]driver.Value{{c.state.balance}}}, nil
}

func (c *balanceMemConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO licenses") {
		c.pendingInserts++
		return balanceMemResult{rows: 1, rowsSet: true}, nil
	}
	if !strings.Contains(query, "UPDATE") || !strings.Contains(query, "balance") {
		return nil, errors.New("unexpected balance exec: " + query)
	}
	if len(args) < 3 {
		return nil, errors.New("balance update requires amount, id, and threshold")
	}
	amount, ok := balanceMemFloat(args[0].Value)
	if !ok {
		return nil, errors.New("balance update amount is not numeric")
	}
	threshold, ok := balanceMemFloat(args[2].Value)
	if !ok {
		return nil, errors.New("balance update threshold is not numeric")
	}

	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if strings.Contains(query, "balance = balance -") {
		if c.state.balance < amount || c.state.balance < threshold {
			return balanceMemResult{rows: 0, rowsSet: true}, nil
		}
		c.state.balance -= amount
		return balanceMemResult{rows: 1, rowsSet: true}, nil
	}
	if c.state.balance < threshold {
		return balanceMemResult{rows: 0, rowsSet: true}, nil
	}
	c.state.balance = amount
	return balanceMemResult{rows: 1, rowsSet: true}, nil
}

func (tx *balanceMemTx) Commit() error {
	tx.conn.state.mu.Lock()
	defer tx.conn.state.mu.Unlock()
	tx.conn.state.licenses += tx.conn.pendingInserts
	tx.conn.pendingInserts = 0
	tx.conn.releaseLockLocked()
	return nil
}

func (tx *balanceMemTx) Rollback() error {
	tx.conn.state.mu.Lock()
	defer tx.conn.state.mu.Unlock()
	tx.conn.pendingInserts = 0
	tx.conn.releaseLockLocked()
	return nil
}

func (c *balanceMemConn) releaseLockLocked() {
	if !c.holdsLock {
		return
	}
	c.holdsLock = false
	c.state.lockHeld = false
	c.state.cond.Broadcast()
}

func (balanceMemRows) Columns() []string { return []string{"balance"} }
func (balanceMemRows) Close() error      { return nil }
func (r *balanceMemRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func (balanceMemResult) LastInsertId() (int64, error) { return 1, nil }
func (r balanceMemResult) RowsAffected() (int64, error) {
	if !r.rowsSet {
		return 1, nil
	}
	return r.rows, nil
}

func balanceMemFloat(value driver.Value) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func purchaseWithDeduct(db *sql.DB, table string, id int64, amount float64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := deductPurchaseBalance(tx, table, id, amount); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO licenses (owner_id) VALUES (?)", id); err != nil {
		return err
	}
	return tx.Commit()
}

func TestConcurrentDeductPurchaseBalanceDifferentAmounts(t *testing.T) {
	for _, table := range []string{"users", "agents"} {
		t.Run(table, func(t *testing.T) {
			state := &balanceMemState{balance: 100}
			db := openBalanceMemDB(t, state)
			errCh := make(chan error, 2)
			var wg sync.WaitGroup
			start := make(chan struct{})
			for _, amount := range []float64{60, 30} {
				wg.Add(1)
				go func(amount float64) {
					defer wg.Done()
					<-start
					errCh <- purchaseWithDeduct(db, table, 1, amount)
				}(amount)
			}
			close(start)
			wg.Wait()
			close(errCh)
			for err := range errCh {
				if err != nil {
					t.Fatalf("concurrent deduct failed: %v", err)
				}
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.balance != 10 {
				t.Fatalf("balance = %v, want 10", state.balance)
			}
			if state.licenses != 2 {
				t.Fatalf("licenses = %d, want 2", state.licenses)
			}
		})
	}
}

func TestConcurrentDeductPurchaseBalanceRejectsSecondPurchase(t *testing.T) {
	state := &balanceMemState{balance: 50}
	db := openBalanceMemDB(t, state)
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- purchaseWithDeduct(db, "users", 7, 40)
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	success, rejected := 0, 0
	for err := range errCh {
		switch {
		case err == nil:
			success++
		case errors.Is(err, errPurchaseBalanceInsufficient):
			rejected++
		default:
			t.Fatalf("unexpected deduct error: %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("success=%d rejected=%d, want 1 and 1", success, rejected)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.balance != 10 {
		t.Fatalf("balance = %v, want 10", state.balance)
	}
	if state.licenses != 1 {
		t.Fatalf("licenses = %d, want 1", state.licenses)
	}
}
