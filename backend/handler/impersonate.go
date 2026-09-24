package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// impersonateTokenTTL 是一次排障会话的有效期，短于普通用户登录的 7 天。
const impersonateTokenTTL = 2 * time.Hour

// signImpersonateToken 签发带代登录标记和操作者 ID 的用户/代理 token。
func signImpersonateToken(id uint, username, role string, operatorID uint) (string, error) {
	now := time.Now()
	claims := &middleware.Claims{
		UserID:     id,
		Username:   username,
		Role:       role,
		Act:        middleware.TokenActImpersonation,
		OperatorID: operatorID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(impersonateTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
}

func rejectUnlessSuperAdmin(c *gin.Context) bool {
	if c.GetString("role_code") == "R_SUPER" {
		return false
	}
	c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
	return true
}

func writeImpersonationLog(db *sql.DB, operatorID uint, action, targetType string, targetID uint64, username, ip string) error {
	_, err := db.Exec(`
		INSERT INTO operation_logs (operator_type, operator_id, action, target_type, target_id, detail, ip, created_at)
		VALUES ('admin', ?, ?, ?, ?, JSON_OBJECT('username', ?), ?, NOW())
	`, operatorID, action, targetType, targetID, username, ip)
	return err
}

// AdminImpersonateUser 超级管理员代登录用户账号（无需密码）。
// 仅签发用户端 token，不影响当前管理员会话，也不改写用户最后登录信息。
func AdminImpersonateUser(c *gin.Context) {
	if rejectUnlessSuperAdmin(c) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	var email, nickname string
	var enabled bool
	err = db.QueryRow("SELECT email, nickname, enabled FROM users WHERE id = ?", id).
		Scan(&email, &nickname, &enabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "用户不存在"})
		return
	}
	if !enabled {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "该账号已被禁用，无法登录"})
		return
	}

	operatorID := c.GetUint("user_id")
	token, err := signImpersonateToken(uint(id), email, "user", operatorID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成token失败"})
		return
	}
	if err := writeImpersonationLog(db, operatorID, "impersonate_user", "user", id, email, c.ClientIP()); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "记录操作日志失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "ok",
		"data": gin.H{
			"accessToken": token,
			"userId":      id,
			"email":       email,
			"nickname":    nickname,
		},
	})
}

// AdminImpersonateAgent 超级管理员代登录代理商账号（无需密码）。
func AdminImpersonateAgent(c *gin.Context) {
	if rejectUnlessSuperAdmin(c) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	var email, name string
	var balance float64
	var enabled bool
	err = db.QueryRow("SELECT email, name, balance, enabled FROM agents WHERE id = ?", id).
		Scan(&email, &name, &balance, &enabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "代理商不存在"})
		return
	}
	if !enabled {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "该代理商已被冻结，无法登录"})
		return
	}

	operatorID := c.GetUint("user_id")
	token, err := signImpersonateToken(uint(id), email, "agent", operatorID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成token失败"})
		return
	}
	if err := writeImpersonationLog(db, operatorID, "impersonate_agent", "agent", id, email, c.ClientIP()); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "记录操作日志失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "ok",
		"data": gin.H{
			"accessToken": token,
			"agentId":     id,
			"email":       email,
			"name":        name,
			"balance":     balance,
		},
	})
}
