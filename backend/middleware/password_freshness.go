package middleware

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// passwordFreshnessDDL 限定允许参与密码时效校验的账号表，同时充当表名白名单。
var passwordFreshnessDDL = map[string]string{
	"admins": "ALTER TABLE admins ADD COLUMN password_changed_at DATETIME DEFAULT NULL COMMENT '密码最后变更时间，早于该时刻签发的 token 失效' AFTER password_hash",
	"users":  "ALTER TABLE users ADD COLUMN password_changed_at DATETIME DEFAULT NULL COMMENT '密码最后变更时间，早于该时刻签发的 token 失效' AFTER password_hash",
	"agents": "ALTER TABLE agents ADD COLUMN password_changed_at DATETIME DEFAULT NULL COMMENT '密码最后变更时间，早于该时刻签发的 token 失效' AFTER password_hash",
}

type columnEnsureState struct {
	mu   sync.Mutex
	done bool
}

var columnEnsureStates sync.Map

// NewPasswordChangeStamp 生成写入 password_changed_at 的时间戳。
// 截断到秒可避免 MySQL DATETIME 的小数秒进位晚于随后签发 token 的 iat，
// 否则用户刚改完密码重新登录会被立即判为过期。
func NewPasswordChangeStamp() time.Time {
	return time.Now().Truncate(time.Second)
}

// EnsurePasswordChangedAtColumn 幂等地为账号表补齐 password_changed_at 列。
// 仅在成功后记忆状态，瞬时故障不会让后续调用永久跳过。
func EnsurePasswordChangedAtColumn(db *sql.DB, table string) error {
	ddl, ok := passwordFreshnessDDL[table]
	if !ok {
		return fmt.Errorf("password freshness: unsupported table %q", table)
	}

	value, _ := columnEnsureStates.LoadOrStore(table, &columnEnsureState{})
	state := value.(*columnEnsureState)

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.done {
		return nil
	}

	var exists int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'password_changed_at'
	`, table).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		if _, err := db.Exec(ddl); err != nil {
			return err
		}
	}

	state.done = true
	return nil
}

// TokenIssuedAt 取出 JWTAuth 写入的签发时间。
func TokenIssuedAt(c *gin.Context) (time.Time, bool) {
	value, exists := c.Get("token_issued_at")
	if !exists {
		return time.Time{}, false
	}
	issuedAt, ok := value.(time.Time)
	return issuedAt, ok
}

// PasswordTokenStale 判断 token 是否早于密码变更时刻。
// 严格小于：同一秒内签发的 token 视为有效，避免误杀改密后立即重新登录取得的新 token。
func PasswordTokenStale(issuedAt time.Time, changedAt sql.NullTime) bool {
	if !changedAt.Valid {
		return false
	}
	return issuedAt.Before(changedAt.Time)
}

// RequireFreshPassword 拒绝密码变更前签发的 token，需置于 JWTAuth 之后。
func RequireFreshPassword(table string) gin.HandlerFunc {
	if _, ok := passwordFreshnessDDL[table]; !ok {
		log.Panicf("RequireFreshPassword: unsupported table %q", table)
	}
	query := fmt.Sprintf("SELECT password_changed_at FROM %s WHERE id = ?", table)

	return func(c *gin.Context) {
		issuedAt, ok := TokenIssuedAt(c)
		if !ok {
			c.Next()
			return
		}

		db, err := config.DB()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "密码状态校验失败"})
			c.Abort()
			return
		}

		// 列缺失时放行：该场景等同于功能尚未启用，锁死整个后台的代价高于收益。
		if err := EnsurePasswordChangedAtColumn(db, table); err != nil {
			log.Printf("password freshness: ensure column on %s failed: %v", table, err)
			c.Next()
			return
		}

		var changedAt sql.NullTime
		switch err := db.QueryRow(query, c.GetUint("user_id")).Scan(&changedAt); {
		case err == sql.ErrNoRows:
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "账号不存在或已失效"})
			c.Abort()
			return
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "密码状态校验失败"})
			c.Abort()
			return
		}

		if PasswordTokenStale(issuedAt, changedAt) {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "密码已变更，请重新登录"})
			c.Abort()
			return
		}

		c.Next()
	}
}