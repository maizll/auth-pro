package middleware

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
)

var (
	cachedSecret    []byte
	ephemeralSecret []byte
	secretStateMu   sync.Mutex
)

// JWTSecret 返回签名密钥。优先使用安装时持久化的密钥；
// 读取失败时退化为进程内随机密钥（重启后旧 token 失效，但不会泄露硬编码密钥）。
func JWTSecret() []byte {
	secretStateMu.Lock()
	defer secretStateMu.Unlock()

	if cachedSecret != nil {
		return cachedSecret
	}
	if secret, err := config.LoadOrCreateJWTSecret(); err == nil {
		cachedSecret = secret
		return cachedSecret
	} else {
		log.Printf("jwt secret unavailable (%v), falling back to ephemeral secret", err)
	}
	if ephemeralSecret == nil {
		raw := make([]byte, 48)
		_, _ = rand.Read(raw)
		ephemeralSecret = []byte(hex.EncodeToString(raw))
	}
	return ephemeralSecret
}

const (
	// TokenTypeRefresh 标记只能用于换发访问令牌的 JWT，不能当作接口 Bearer。
	TokenTypeRefresh = "refresh"
	// TokenActImpersonation 标记超级管理员代登录签发的用户/代理令牌。
	TokenActImpersonation = "impersonation"
)

type Claims struct {
	UserID     uint   `json:"user_id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	RoleCode   string `json:"role_code,omitempty"`
	Typ        string `json:"typ,omitempty"`
	Act        string `json:"act,omitempty"`
	OperatorID uint   `json:"operator_id,omitempty"`
	jwt.RegisteredClaims
}

// adminSessionQuery 在每次管理接口请求时复查账号是否仍启用、角色是否仍与令牌一致。
const adminSessionQuery = `
	SELECT a.enabled, COALESCE(r.role_code, '')
	FROM admins a
	LEFT JOIN roles r ON r.id = a.role_id
	WHERE a.id = ?
`

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未提供认证信息"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "认证格式错误"})
			c.Abort()
			return
		}

		token, err := jwt.ParseWithClaims(parts[1], &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return JWTSecret(), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "认证已过期或无效"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "认证信息解析失败"})
			c.Abort()
			return
		}
		if claims.Typ == TokenTypeRefresh {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "刷新令牌不能用于接口访问"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("role_code", claims.RoleCode)
		if claims.Act != "" {
			c.Set("act", claims.Act)
		}
		if claims.OperatorID != 0 {
			c.Set("operator_id", claims.OperatorID)
		}
		if claims.IssuedAt != nil {
			c.Set("token_issued_at", claims.IssuedAt.Time)
		}
		c.Next()
	}
}

// ImpersonationOperatorID 返回代登录令牌里的真实管理员 ID，供后续审计读取。
func ImpersonationOperatorID(c *gin.Context) (uint, bool) {
	if c.GetString("act") != TokenActImpersonation {
		return 0, false
	}
	id := c.GetUint("operator_id")
	if id == 0 {
		return 0, false
	}
	return id, true
}

func activeUserRejection(enabled bool, accountStatus string, convertedAgentID sql.NullInt64) (string, bool) {
	if accountStatus == "converted" || convertedAgentID.Valid {
		return "该账号已升级为代理，请前往代理端登录", true
	}
	if !enabled {
		return "用户账户已禁用", false
	}
	return "", false
}

// RequireActiveUser rejects stale user tokens immediately after account conversion.
func RequireActiveUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "user" {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "无权限访问用户端"})
			c.Abort()
			return
		}

		db, err := config.DB()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "用户状态校验失败"})
			c.Abort()
			return
		}

		var enabled bool
		var accountStatus string
		var convertedAgentID sql.NullInt64
		err = db.QueryRow(`
			SELECT enabled, account_status, converted_agent_id
			FROM users WHERE id = ?
		`, c.GetUint("user_id")).Scan(&enabled, &accountStatus, &convertedAgentID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户账户不存在或已失效"})
			c.Abort()
			return
		}
		message, converted := activeUserRejection(enabled, accountStatus, convertedAgentID)
		if message != "" {
			response := gin.H{"code": 401, "message": message}
			if converted {
				response["data"] = gin.H{"converted": true, "agentId": convertedAgentID.Int64}
			}
			c.JSON(http.StatusUnauthorized, response)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAdmin 仅允许管理员角色访问，需置于 JWTAuth 之后。
// 每次请求复查 admins.enabled 与当前 role_code。禁用、账号消失，或令牌角色与库中不一致时拒绝，
// 避免停用或降权后旧令牌继续可用。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			c.JSON(http.StatusOK, gin.H{"code": 403, "message": "无权限访问"})
			c.Abort()
			return
		}

		db, err := config.DB()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "管理员状态校验失败"})
			c.Abort()
			return
		}

		var enabled sql.NullBool
		var roleCode string
		err = db.QueryRow(adminSessionQuery, c.GetUint("user_id")).Scan(&enabled, &roleCode)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "管理员账户不存在或已失效"})
			c.Abort()
			return
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "管理员状态校验失败"})
			c.Abort()
			return
		}
		if !enabled.Valid || !enabled.Bool {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "管理员账户已禁用"})
			c.Abort()
			return
		}
		if roleCode != c.GetString("role_code") {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "管理员权限已变更，请重新登录"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireSuperAdmin 仅允许超级管理员（R_SUPER）访问，需置于 JWTAuth + RequireAdmin 之后。
// 用于后端强制对齐前端的 R_SUPER 专属权限，防止 R_ADMIN 绕过前端菜单直接调用接口。
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role_code") != "R_SUPER" {
			c.JSON(http.StatusOK, gin.H{"code": 403, "message": "无权限访问"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAgent 仅允许代理商角色访问，需置于 JWTAuth 之后。
func RequireAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "agent" {
			c.JSON(http.StatusOK, gin.H{"code": 403, "message": "无权限访问代理商接口"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireDeveloper 允许软件源开发者 JWT，或已绑定开发者资格的代理商 JWT。
func RequireDeveloper() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "developer" && role != "agent" {
			c.JSON(http.StatusOK, gin.H{"code": 403, "message": "无权限访问开发者接口"})
			c.Abort()
			return
		}
		c.Next()
	}
}
