package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SourceStationAdminPath 是管理后台「源站」菜单入口（插件管理）。
const SourceStationAdminPath = "/source-station/plugins"

// SourceStationPage 兼容旧 /source 入口，跳转到管理后台同一套登录与布局。
func SourceStationPage(c *gin.Context) {
	c.Redirect(http.StatusFound, SourceStationAdminPath)
}
