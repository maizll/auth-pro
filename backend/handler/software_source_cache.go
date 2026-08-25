package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"auto_pro/config"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

func InternalSoftwareSourceCacheInvalidate(c *gin.Context) {
	expectedKey := strings.TrimSpace(config.GetSoftwareSourceAPIKey())
	providedKey := strings.TrimSpace(c.GetHeader("X-Software-Source-Key"))
	if expectedKey == "" || subtle.ConstantTimeCompare([]byte(providedKey), []byte(expectedKey)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "软件源目录 Key 无效"})
		return
	}
	client, err := softwaresource.Default()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "msg": err.Error()})
		return
	}
	catalog, err := client.Refresh(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": http.StatusBadGateway, "msg": "软件源目录刷新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "msg": "", "data": gin.H{"revision": catalog.Revision}})
}
