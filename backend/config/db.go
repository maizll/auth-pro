package config

import (
	"database/sql"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 连接池默认容量。MySQL 默认 max_connections 为 151，
// 这里留出余量给管理连接与同机其他应用。
const (
	defaultDBMaxOpenConns    = 50
	defaultDBMaxIdleConns    = 25
	defaultDBConnMaxLifetime = 3 * time.Minute
	defaultDBConnMaxIdleTime = time.Minute
)

var (
	dbPool    *sql.DB
	dbPoolDSN string
	dbPoolMu  sync.Mutex
)

// DB 返回进程级共享的数据库连接池。
//
// 连接池惰性创建：安装流程写入配置前没有可用 DSN，因此不能在启动时强制建立。
// 配置变更导致 DSN 变化时会自动重建，旧池延迟关闭以放行仍在执行的请求。
//
// 调用方不得关闭返回的 *sql.DB，它由本包持有并在整个进程生命周期内复用。
func DB() (*sql.DB, error) {
	cfg, err := LoadDBConfig()
	if err != nil {
		return nil, err
	}
	return dbPoolFor(GetDSN(cfg))
}

func dbPoolFor(dsn string) (*sql.DB, error) {
	dbPoolMu.Lock()
	defer dbPoolMu.Unlock()

	if dbPool != nil && dbPoolDSN == dsn {
		return dbPool, nil
	}

	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(envInt("AUTO_PRO_DB_MAX_OPEN_CONNS", defaultDBMaxOpenConns))
	pool.SetMaxIdleConns(envInt("AUTO_PRO_DB_MAX_IDLE_CONNS", defaultDBMaxIdleConns))
	pool.SetConnMaxLifetime(envDuration("AUTO_PRO_DB_CONN_MAX_LIFETIME", defaultDBConnMaxLifetime))
	pool.SetConnMaxIdleTime(envDuration("AUTO_PRO_DB_CONN_MAX_IDLE_TIME", defaultDBConnMaxIdleTime))

	if previous := dbPool; previous != nil {
		go retirePool(previous)
	}
	dbPool = pool
	dbPoolDSN = dsn
	return dbPool, nil
}

// retirePool 延迟关闭被替换的连接池，避免切换瞬间打断仍在执行的查询。
func retirePool(pool *sql.DB) {
	time.Sleep(30 * time.Second)
	_ = pool.Close()
}

// ResetDBPool 丢弃当前连接池，仅供测试与配置重载使用。
func ResetDBPool() {
	dbPoolMu.Lock()
	defer dbPoolMu.Unlock()

	if dbPool != nil {
		_ = dbPool.Close()
	}
	dbPool = nil
	dbPoolDSN = ""
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}