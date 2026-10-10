package handler

import (
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterConsoleAdRoutes 客户站广告自助购买 + 隐藏开关（管理员）。
func RegisterConsoleAdRoutes(secured *gin.RouterGroup) {
	secured.GET("/ads/slots", ClientAdSlotCatalog)
	secured.GET("/ads/calendar", ClientAdCalendar)
	secured.GET("/ads/pay-options", ClientAdPayOptions)
	secured.POST("/ads/orders", ClientAdOrderCreate)
	secured.POST("/ads/orders/pay", ClientAdOrderPay)
	secured.GET("/ads/orders", ClientAdOrderList)
	secured.POST("/ads/orders/resubmit", ClientAdOrderResubmit)
	secured.GET("/ads/hide", AdminAdsHideGet)
	secured.PUT("/ads/hide", AdminAdsHideSet)
}

// RegisterConsoleAdRoutesWithAuth 挂到已鉴权管理员组。
func RegisterConsoleAdRoutesWithAuth(api *gin.RouterGroup) {
	secured := api.Group("/")
	secured.Use(middleware.JWTAuth(), middleware.RequireAdmin(), middleware.RequireFreshPassword("admins"))
	RegisterConsoleAdRoutes(secured)
}
