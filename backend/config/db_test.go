package config

import (
	"testing"
	"time"
)

const (
	testDSNPrimary   = "user:pass@tcp(127.0.0.1:3306)/db_primary?charset=utf8mb4"
	testDSNSecondary = "user:pass@tcp(127.0.0.1:3306)/db_secondary?charset=utf8mb4"
)

// 同一 DSN 必须复用同一连接池，否则每次请求仍会新建池，重构就失去意义。
func TestDBPoolReusesSameDSN(t *testing.T) {
	ResetDBPool()
	defer ResetDBPool()

	first, err := dbPoolFor(testDSNPrimary)
	if err != nil {
		t.Fatalf("首次建池失败: %v", err)
	}

	second, err := dbPoolFor(testDSNPrimary)
	if err != nil {
		t.Fatalf("复用建池失败: %v", err)
	}

	if first != second {
		t.Fatal("相同 DSN 返回了不同的连接池实例")
	}
}

// 安装流程会让配置从无到有，DSN 变化时必须切换到新池。
func TestDBPoolRebuildsOnDSNChange(t *testing.T) {
	ResetDBPool()
	defer ResetDBPool()

	first, err := dbPoolFor(testDSNPrimary)
	if err != nil {
		t.Fatalf("首次建池失败: %v", err)
	}

	second, err := dbPoolFor(testDSNSecondary)
	if err != nil {
		t.Fatalf("切换 DSN 建池失败: %v", err)
	}

	if first == second {
		t.Fatal("DSN 变化后仍复用了旧连接池")
	}
}

func TestDBPoolAppliesLimits(t *testing.T) {
	ResetDBPool()
	defer ResetDBPool()

	t.Setenv("AUTO_PRO_DB_MAX_OPEN_CONNS", "17")

	pool, err := dbPoolFor(testDSNPrimary)
	if err != nil {
		t.Fatalf("建池失败: %v", err)
	}

	if got := pool.Stats().MaxOpenConnections; got != 17 {
		t.Fatalf("最大连接数 = %d, 期望 17", got)
	}
}

func TestEnvOverridesFallBackOnInvalidInput(t *testing.T) {
	t.Setenv("AUTO_PRO_DB_MAX_OPEN_CONNS", "not-a-number")
	if got := envInt("AUTO_PRO_DB_MAX_OPEN_CONNS", 50); got != 50 {
		t.Fatalf("非法数值应回落默认值, 实得 %d", got)
	}

	t.Setenv("AUTO_PRO_DB_MAX_OPEN_CONNS", "-3")
	if got := envInt("AUTO_PRO_DB_MAX_OPEN_CONNS", 50); got != 50 {
		t.Fatalf("非正数应回落默认值, 实得 %d", got)
	}

	t.Setenv("AUTO_PRO_DB_CONN_MAX_LIFETIME", "abc")
	if got := envDuration("AUTO_PRO_DB_CONN_MAX_LIFETIME", time.Minute); got != time.Minute {
		t.Fatalf("非法时长应回落默认值, 实得 %v", got)
	}

	t.Setenv("AUTO_PRO_DB_CONN_MAX_LIFETIME", "90s")
	if got := envDuration("AUTO_PRO_DB_CONN_MAX_LIFETIME", time.Minute); got != 90*time.Second {
		t.Fatalf("合法时长应生效, 实得 %v", got)
	}
}