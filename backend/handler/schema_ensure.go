package handler

import (
	"database/sql"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
)

const accountSchemaEnsureAttempts = 5

var (
	accountSchemaEnsureMu     sync.Mutex
	accountUpgradeSchemaReady atomic.Bool
	agentLevelSchemaReady     atomic.Bool
	schemaEnsureBackoff       = func(attempt int) {
		time.Sleep(time.Duration(attempt) * 15 * time.Millisecond)
	}
)

// EnsureAccountUpgradeSchema upgrades existing installations without changing
// legacy account ownership or enabling self-service sales by default.
//
// Requests used to run this DDL on every call. Concurrent AgentList / level
// pages then deadlocked on metadata locks (MySQL 1213). One in-process ensure
// runs at a time; after it succeeds, later calls skip the DDL until restart.
func EnsureAccountUpgradeSchema(db *sql.DB) error {
	if accountUpgradeSchemaReady.Load() {
		return nil
	}
	return runEnsuredSchema(&accountUpgradeSchemaReady, func() error {
		return ensureAccountUpgradeSchemaUnlocked(db)
	})
}

// ensureAgentLevelSchema is the standalone entry used by level pages and
// agent create/update. It shares the account-upgrade mutex so the two paths
// cannot interleave DDL.
func ensureAgentLevelSchema(db *sql.DB) error {
	if agentLevelSchemaReady.Load() || accountUpgradeSchemaReady.Load() {
		return nil
	}
	return runEnsuredSchema(&agentLevelSchemaReady, func() error {
		if accountUpgradeSchemaReady.Load() {
			return nil
		}
		return ensureAgentLevelSchemaBody(db)
	})
}

func runEnsuredSchema(ready *atomic.Bool, work func() error) error {
	var err error
	for attempt := 1; attempt <= accountSchemaEnsureAttempts; attempt++ {
		err = func() error {
			accountSchemaEnsureMu.Lock()
			defer accountSchemaEnsureMu.Unlock()
			if ready.Load() {
				return nil
			}
			if err := work(); err != nil {
				return err
			}
			ready.Store(true)
			return nil
		}()
		if err == nil || !isMySQLSchemaLockError(err) {
			return err
		}
		if attempt == accountSchemaEnsureAttempts {
			return err
		}
		log.Printf("账户结构初始化遇到数据库锁冲突，准备重试 (%d/%d): %v", attempt, accountSchemaEnsureAttempts, err)
		schemaEnsureBackoff(attempt)
	}
	return err
}

func isMySQLSchemaLockError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr == nil {
		return false
	}
	return mysqlErr.Number == 1213 || mysqlErr.Number == 1205
}

// resetAccountSchemaEnsureForTest clears the process-lifetime fast path so a
// test can force the DDL to run again. Production requests do not call it.
func resetAccountSchemaEnsureForTest() {
	accountSchemaEnsureMu.Lock()
	accountUpgradeSchemaReady.Store(false)
	agentLevelSchemaReady.Store(false)
	accountSchemaEnsureMu.Unlock()
}
