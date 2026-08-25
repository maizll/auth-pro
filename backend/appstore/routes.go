package appstore

import (
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

func (s *Server) RegisterAdminRoutes(api *gin.RouterGroup) {
	router := api.Group("/app-store")
	router.Use(middleware.JWTAuth(), middleware.RequireAdmin())
	router.GET("/dashboard", s.dashboard)
	router.GET("/templates", s.templates)
	router.PUT("/templates/:id/enable", s.enableTemplate)
	router.PUT("/templates/:id/disable", s.disableTemplate)
}
