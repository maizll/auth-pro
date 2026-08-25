package appstore

import (
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

const PagePrefix = "/admin/app-store"

func RegisterPageRoutes(router *gin.Engine, staticFS fs.FS) error {
	indexHTML, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		return fmt.Errorf("load app-store index: %w", err)
	}

	handler := func(c *gin.Context) {
		requestedPath := strings.TrimPrefix(c.Param("filepath"), "/")
		if requestedPath != "" {
			cleanPath := path.Clean(requestedPath)
			if cleanPath != "." && cleanPath != ".." && !strings.HasPrefix(cleanPath, "../") {
				if info, statErr := fs.Stat(staticFS, cleanPath); statErr == nil && !info.IsDir() {
					payload, readErr := fs.ReadFile(staticFS, cleanPath)
					if readErr == nil {
						contentType := mime.TypeByExtension(path.Ext(cleanPath))
						if contentType == "" {
							contentType = http.DetectContentType(payload)
						}
						c.Data(http.StatusOK, contentType, payload)
						return
					}
				}
				if strings.HasPrefix(cleanPath, "assets/") || path.Ext(cleanPath) != "" {
					c.Status(http.StatusNotFound)
					return
				}
			}
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}

	router.GET(PagePrefix, handler)
	router.GET(PagePrefix+"/*filepath", handler)
	return nil
}
