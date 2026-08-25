package appstore

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Server struct {
	service *Service
}

func NewServer(repository TemplateRepository) *Server {
	return &Server{service: NewService(repository)}
}

func (s *Server) dashboard(c *gin.Context) {
	data, err := s.service.Dashboard(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": data})
}

func (s *Server) templates(c *gin.Context) {
	items, err := s.service.Templates(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": items}})
}

func (s *Server) enableTemplate(c *gin.Context) {
	if err := s.service.EnableTemplate(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已启用"})
}

func (s *Server) disableTemplate(c *gin.Context) {
	if err := s.service.DisableTemplate(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已禁用"})
}

func writeError(c *gin.Context, err error) {
	code, message := ErrorResponse(err)
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": message})
}
