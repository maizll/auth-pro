package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 管理员调整用户余额的唯一入口。编辑用户资料不再带余额：
// 以前保存资料会把打开窗口时的旧余额整个写回去，期间用户买过东西就等于白拿，还能写成负数，也不留记录。
// 这里按增减量在事务里锁行改余额，结果不能小于 0，每次都写财务流水和操作日志。

var (
	errUserBalanceNegative = errors.New("调整后余额不能小于 0")
	errUserBalanceMissing  = errors.New("用户不存在")
)

// adjustUserBalance 给用户余额加上 deltaCents（可正可负），返回调整后的余额（元，两位小数）。
func adjustUserBalance(db *sql.DB, userID int64, deltaCents int64, remark string, operatorID uint, ip string) (string, error) {
	if deltaCents == 0 {
		return "", errors.New("调整金额不能为 0")
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var current float64
	if err := tx.QueryRow("SELECT balance FROM users WHERE id = ? FOR UPDATE", userID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errUserBalanceMissing
		}
		return "", err
	}
	if floatAmountToCents(current)+deltaCents < 0 {
		return "", errUserBalanceNegative
	}
	delta := formatSignedCents(deltaCents)
	result, err := tx.Exec("UPDATE users SET balance = balance + ? WHERE id = ? AND balance + ? >= 0", delta, userID, delta)
	if err != nil {
		return "", err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return "", errUserBalanceNegative
	}
	var after string
	if err := tx.QueryRow("SELECT balance FROM users WHERE id = ?", userID).Scan(&after); err != nil {
		return "", err
	}

	txType, amount, verb := "recharge", deltaCents, "增加"
	if deltaCents < 0 {
		txType, amount, verb = "consume", -deltaCents, "扣减"
	}
	note := fmt.Sprintf("后台管理员 %d %s余额", operatorID, verb)
	if remark != "" {
		note += "：" + remark
	}
	if _, err := tx.Exec(`
		INSERT INTO transactions (tx_no, subject_type, subject_id, type, amount, balance_after, ref_type, ref_id, remark)
		VALUES (?, 'user', ?, ?, ?, ?, 'admin_balance_adjust', ?, ?)
	`, generateTransactionNo(), userID, txType, formatCents(amount), after, operatorID, truncateText(note, 255)); err != nil {
		return "", fmt.Errorf("记录流水失败: %w", err)
	}
	detail, _ := json.Marshal(map[string]any{
		"delta": delta, "before": formatSignedCents(floatAmountToCents(current)), "after": after, "remark": remark,
	})
	if _, err := tx.Exec(`
		INSERT INTO operation_logs (operator_type, operator_id, action, target_type, target_id, detail, ip)
		VALUES ('admin', ?, 'user_balance_adjust', 'user', ?, ?, ?)
	`, operatorID, userID, string(detail), ip); err != nil {
		return "", fmt.Errorf("记录操作日志失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return after, nil
}

// formatSignedCents 是 formatCents 的带符号版本，给 SQL 增减量和日志用。
func formatSignedCents(cents int64) string {
	if cents < 0 {
		return "-" + formatCents(-cents)
	}
	return formatCents(cents)
}

// AdminUserBalanceAdjust 管理端-增减用户余额。amount 为正是加，为负是扣，单位元，最多两位小数。
func AdminUserBalanceAdjust(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "用户ID无效"})
		return
	}
	var req struct {
		Amount float64 `json:"amount"`
		Remark string  `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	cents := floatAmountToCents(req.Amount)
	if cents == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写要增加或扣减的金额"})
		return
	}
	if cents > 100_000_000 || cents < -100_000_000 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "单次调整不能超过 100 万元"})
		return
	}
	remark := strings.TrimSpace(req.Remark)
	if remark == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写调整原因，会记进流水"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	after, err := adjustUserBalance(db, userID, cents, truncateText(remark, 100), c.GetUint("user_id"), c.ClientIP())
	switch {
	case errors.Is(err, errUserBalanceNegative), errors.Is(err, errUserBalanceMissing):
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	case err != nil:
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "调整失败，余额未变更"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "余额已调整", "data": gin.H{"balance": after}})
}
