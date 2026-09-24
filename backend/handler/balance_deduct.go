package handler

import (
	"database/sql"
	"errors"
	"fmt"
)

// errPurchaseBalanceInsufficient 表示扣款时余额不足，或主体行已经不存在。
var errPurchaseBalanceInsufficient = errors.New("余额不足")

// deductPurchaseBalance 在调用方事务内锁定余额行，并用 balance = balance - ? 扣减。
// 不把事务外读到的余额写成绝对值，避免并发购买少扣款或多发授权。
// amount 同时作为减量和 balance >= ? 的阈值；RowsAffected 不是 1 时回滚由调用方完成。
func deductPurchaseBalance(tx *sql.Tx, table string, id int64, amount float64) (float64, error) {
	if amount < 0 {
		return 0, fmt.Errorf("扣款金额无效")
	}
	var selectSQL, updateSQL string
	switch table {
	case "users":
		selectSQL = "SELECT balance FROM users WHERE id = ? FOR UPDATE"
		updateSQL = "UPDATE users SET balance = balance - ? WHERE id = ? AND balance >= ?"
	case "agents":
		selectSQL = "SELECT balance FROM agents WHERE id = ? AND enabled = 1 FOR UPDATE"
		updateSQL = "UPDATE agents SET balance = balance - ? WHERE id = ? AND enabled = 1 AND balance >= ?"
	default:
		return 0, fmt.Errorf("不支持的扣款主体")
	}

	var balance float64
	if err := tx.QueryRow(selectSQL, id).Scan(&balance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errPurchaseBalanceInsufficient
		}
		return 0, err
	}
	if balance < amount {
		return 0, errPurchaseBalanceInsufficient
	}

	result, err := tx.Exec(updateSQL, amount, id, amount)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected != 1 {
		return 0, errPurchaseBalanceInsufficient
	}
	return balance - amount, nil
}
