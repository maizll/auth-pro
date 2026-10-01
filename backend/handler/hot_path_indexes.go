package handler

import (
	"database/sql"
	"fmt"
)

// migrateHotPathListIndexes 为列表和按天统计补二级索引。
// 大表上的 ALTER 可能要一会儿，但每个库只做一次；已有同名索引会跳过。
// 先尝试在线加索引，失败再退回普通 ALTER，避免旧库因为算法子句直接失败。
func migrateHotPathListIndexes(db *sql.DB) error {
	indexes := []struct {
		table   string
		name    string
		columns string
	}{
		{"verify_logs", "idx_verify_logs_created_at", "created_at"},
		{"licenses", "idx_licenses_created_at", "created_at"},
		{"transactions", "idx_transactions_type_created", "type, created_at"},
		{"transactions", "idx_transactions_ref", "ref_type, ref_id"},
	}
	for _, item := range indexes {
		var tables int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		`, item.table).Scan(&tables); err != nil {
			return err
		}
		if tables == 0 {
			continue
		}
		online := fmt.Sprintf(
			"ALTER TABLE %s ADD INDEX %s (%s), ALGORITHM=INPLACE, LOCK=NONE",
			item.table, item.name, item.columns,
		)
		if err := ensureSourceStationIndex(db, item.table, item.name, online); err != nil {
			plain := fmt.Sprintf("ALTER TABLE %s ADD INDEX %s (%s)", item.table, item.name, item.columns)
			if err := ensureSourceStationIndex(db, item.table, item.name, plain); err != nil {
				return err
			}
		}
	}
	return nil
}
