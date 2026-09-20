package handler

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// RegisterFrontend 用同一套 ResolveFrontendRoot 结果挂 NoRoute。
// 生产路径缺盘上前端时返回错误（由调用方 fatal）；不得静默改走 embed。
func RegisterFrontend(engine *gin.Engine, embedFS fs.FS) error {
	root, err := config.ResolveFrontendRoot()
	if err != nil {
		return err
	}
	if root.Mode == config.FrontendModeEmbed {
		if embedFS == nil {
			return fmt.Errorf("已设置 %s 但 embed 文件系统为空", config.EnvAllowEmbeddedFrontend)
		}
		if root.Fingerprint == "" {
			root.Fingerprint = config.FingerprintFS(embedFS)
		}
		log.Printf("frontend root: mode=embed fingerprint=%s applyDir=%s (%s=1)",
			root.Fingerprint, root.ApplyDir, config.EnvAllowEmbeddedFrontend)
		engine.NoRoute(func(c *gin.Context) {
			if rejectAPINotFound(c) {
				return
			}
			serveEmbedFrontend(c, embedFS)
		})
		return nil
	}

	log.Printf("frontend root: mode=disk dir=%s fingerprint=%s", root.Dir, root.Fingerprint)
	diskServer := http.FileServer(http.Dir(root.Dir))
	dir := root.Dir
	engine.NoRoute(func(c *gin.Context) {
		if rejectAPINotFound(c) {
			return
		}
		serveDiskFrontend(c, dir, diskServer)
	})
	return nil
}

func rejectAPINotFound(c *gin.Context) bool {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "请求的资源不存在"})
		return true
	}
	return false
}

func serveDiskFrontend(c *gin.Context, dir string, diskServer http.Handler) {
	if !config.FrontendDirValid(dir) {
		writeFrontendUnavailable(c, dir)
		return
	}
	cleanPath := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
	if cleanPath != "." {
		full := filepath.Join(dir, cleanPath)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			if IsInstalledPackageFile(full) {
				c.Status(http.StatusNotFound)
				return
			}
			diskServer.ServeHTTP(c.Writer, c.Request)
			return
		}
	}
	c.File(filepath.Join(dir, "index.html"))
}

func serveEmbedFrontend(c *gin.Context, embedFS fs.FS) {
	cleanPath := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
	if cleanPath != "." {
		if info, err := fs.Stat(embedFS, cleanPath); err == nil && !info.IsDir() {
			http.FileServer(http.FS(embedFS)).ServeHTTP(c.Writer, c.Request)
			return
		}
	}
	indexHTML, err := fs.ReadFile(embedFS, "index.html")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "前端入口文件不存在"})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}

func writeFrontendUnavailable(c *gin.Context, dir string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintf(c.Writer, `<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>503 Frontend unavailable</title></head>
<body>
<h1>503 Frontend unavailable</h1>
<p>Disk frontend is missing or invalid. This process will not silently serve a stale embedded build.</p>
<p>version=%s</p>
<p>expected=%s</p>
<p>Unpack the release so index.html is on disk, or set AUTO_PRO_FRONTEND_DIR. Embed is opt-in via %s=1.</p>
</body>
</html>
`, config.AppVersion, dir, config.EnvAllowEmbeddedFrontend)
}
