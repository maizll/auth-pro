package handler

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed source_station_ui.html
var sourceStationPageHTML []byte

// SourceStationPage 是源站入驻、元数据登记、审核与上架/下架的最小静态页。
func SourceStationPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", sourceStationPageHTML)
}
