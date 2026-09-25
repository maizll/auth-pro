package middleware

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// 菜单 name 与 handler/menu_seed.sql、productMenuSpecs 保持一致。
// 写接口按侧栏已有菜单鉴权，不另起一套权限码。
const (
	MenuLicenseList                = "LicenseList"
	MenuLicenseCards               = "LicenseCards"
	MenuLicenseApps                = "LicenseApps"
	MenuLicenseVersions            = "LicenseVersions"
	MenuAppVersions                = "AppVersions"
	MenuLicensePlans               = "LicensePlans"
	MenuLicenseLogs                = "LicenseLogs"
	MenuUser                       = "User"
	MenuAgentList                  = "AgentList"
	MenuAgentLevel                 = "AgentLevel"
	MenuAgentQuota                 = "AgentQuota"
	MenuPromotionCampaigns         = "PromotionCampaigns"
	MenuTicketManage               = "TicketManage"
	MenuPiracyTracking             = "PiracyTracking"
	MenuPiracyBlacklist            = "PiracyBlacklist"
	MenuPiracyAlerts               = "PiracyAlerts"
	MenuSourceStationPackages      = "SourceStationPackages"
	MenuSourceStationPlugins       = "SourceStationPlugins"
	MenuSourceStationTemplates     = "SourceStationTemplates"
	MenuSourceStationApplications  = "SourceStationApplications"
	MenuSourceStationCatalog       = "SourceStationCatalog"
	MenuSourceStationAds           = "SourceStationAds"
	MenuSourceStationSettings      = "SourceStationSettings"
	MenuSourceStationEdition       = "SourceStationEdition"
	MenuSourceStationStoreOrders   = "SourceStationStoreOrders"
	MenuSourceStationStoreLicenses = "SourceStationStoreLicenses"
	MenuSourceStationStoreRevenue  = "SourceStationStoreRevenue"
)

// RequireMenu 要求当前管理员角色拥有任一指定菜单。
// 必须放在 JWTAuth 与 RequireAdmin 之后：RequireAdmin 已核对令牌 role_code 与库中一致。
// 超级管理员 R_SUPER 直接放行，不查 role_menus。
// 其余角色按 admins.role_id → role_menus → menus.name 判断，且菜单须为启用状态。
// 拒绝时与 RequireAdmin 相同：HTTP 200 + code 403，便于前端拦截器展示「无权限访问」。
func RequireMenu(menuNames ...string) gin.HandlerFunc {
	names := normalizeMenuNames(menuNames)
	if len(names) == 0 {
		panic("middleware.RequireMenu: 至少指定一个菜单 name")
	}

	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			c.JSON(http.StatusOK, gin.H{"code": 403, "message": "无权限访问"})
			c.Abort()
			return
		}
		if c.GetString("role_code") == "R_SUPER" {
			c.Next()
			return
		}

		db, err := config.DB()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "菜单权限校验失败"})
			c.Abort()
			return
		}

		granted, err := roleHasEnabledMenu(db, c.GetUint("user_id"), names)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "菜单权限校验失败"})
			c.Abort()
			return
		}
		if !granted {
			c.JSON(http.StatusOK, gin.H{"code": 403, "message": "无权限访问"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func normalizeMenuNames(menuNames []string) []string {
	seen := make(map[string]struct{}, len(menuNames))
	names := make([]string, 0, len(menuNames))
	for _, name := range menuNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func roleHasEnabledMenu(db *sql.DB, adminID uint, menuNames []string) (bool, error) {
	placeholders := strings.Repeat("?,", len(menuNames))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := `
		SELECT 1
		FROM admins a
		INNER JOIN role_menus rm ON rm.role_id = a.role_id
		INNER JOIN menus m ON m.id = rm.menu_id
		WHERE a.id = ? AND m.enabled = 1 AND m.name IN (` + placeholders + `)
		LIMIT 1`
	args := make([]any, 0, 1+len(menuNames))
	args = append(args, adminID)
	for _, name := range menuNames {
		args = append(args, name)
	}

	var granted int
	err := db.QueryRow(query, args...).Scan(&granted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
