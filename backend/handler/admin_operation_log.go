package handler

import (
	"database/sql"
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// recordAdminOperation 写一条后台管理员操作日志（操作者、动作、目标、前后差异、来源 IP）。
// 角色、角色菜单、清空校验日志这类改权限或删记录的操作，事后要能查到是谁做的（B6）。
func recordAdminOperation(db *sql.DB, c *gin.Context, action, targetType string, targetID int64, detail map[string]any) error {
	body, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO operation_logs (operator_type, operator_id, action, target_type, target_id, detail, ip)
		VALUES ('admin', ?, ?, ?, ?, ?, ?)
	`, c.GetUint("user_id"), action, targetType, targetID, string(body), c.ClientIP())
	return err
}
