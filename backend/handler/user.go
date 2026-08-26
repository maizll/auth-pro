package handler

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var adminEmailRegexp = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// GetUserInfo 获取当前登录用户信息
func GetUserInfo(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	var email, avatar, nickname string
	var roleID sql.NullInt64
	err = db.QueryRow("SELECT email, avatar, nickname, role_id FROM admins WHERE id = ?", userID).Scan(&email, &avatar, &nickname, &roleID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询用户失败"})
		return
	}

	// 查询角色编码
	roles := []string{}
	if roleID.Valid {
		var roleCode string
		err = db.QueryRow("SELECT role_code FROM roles WHERE id = ?", roleID.Int64).Scan(&roleCode)
		if err == nil {
			roles = append(roles, roleCode)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "",
		"data": gin.H{
			"userId":   userID,
			"userName": username,
			"nickname": nickname,
			"email":    email,
			"avatar":   avatar,
			"roles":    roles,
			"buttons":  []string{},
		},
	})
}

type updateUserInfoRequest struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
}

// UpdateUserInfo 更新当前登录管理员的资料
func UpdateUserInfo(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req updateUserInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	nickname := strings.TrimSpace(req.Nickname)
	email := strings.TrimSpace(req.Email)
	avatar := strings.TrimSpace(req.Avatar)

	if nickname == "" && email == "" && avatar == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请至少修改一项"})
		return
	}

	if nickname != "" && len([]rune(nickname)) > 50 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "昵称长度不能超过 50 个字符"})
		return
	}

	if email != "" && !adminEmailRegexp.MatchString(email) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "邮箱格式不正确"})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	if email != "" {
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM admins WHERE email = ? AND id != ?", email, userID).Scan(&exists); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "校验邮箱失败"})
			return
		}
		if exists > 0 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "该邮箱已被其他管理员使用"})
			return
		}
	}

	sets := []string{}
	args := []interface{}{}
	if nickname != "" {
		sets = append(sets, "nickname = ?")
		args = append(args, nickname)
	}
	if email != "" {
		sets = append(sets, "email = ?")
		args = append(args, email)
	}
	if avatar != "" {
		sets = append(sets, "avatar = ?")
		args = append(args, avatar)
	}
	args = append(args, userID)

	if _, err := db.Exec("UPDATE admins SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "资料更新成功",
		"data": gin.H{
			"nickname": nickname,
			"email":    email,
			"avatar":   avatar,
		},
	})
}

type changePasswordRequest struct {
	OldPassword     string `json:"oldPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required,min=6"`
	ConfirmPassword string `json:"confirmPassword" binding:"required"`
}

// ChangePassword 管理员修改密码
func ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误，密码至少6位"})
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "两次输入的新密码不一致"})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}

	// 查询当前密码哈希
	var passwordHash string
	err = db.QueryRow("SELECT password_hash FROM admins WHERE id = ?", userID).Scan(&passwordHash)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询用户失败"})
		return
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "当前密码错误"})
		return
	}

	// 生成新密码哈希
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "密码加密失败"})
		return
	}

	// 更新密码，并记录变更时刻使此前签发的 token 失效
	if err := middleware.EnsurePasswordChangedAtColumn(db, "admins"); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "修改密码失败"})
		return
	}
	_, err = db.Exec("UPDATE admins SET password_hash = ?, password_changed_at = ? WHERE id = ?",
		string(newHash), middleware.NewPasswordChangeStamp(), userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "修改密码失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "密码修改成功，请重新登录"})
}