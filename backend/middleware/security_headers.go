package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders 给所有响应加最基本的安全响应头（M5 第一步）：
// 不允许别的网站把后台嵌进 iframe（点击劫持），浏览器不猜文件类型，跨站跳转只带来源域名。
// CSP 和 CORS 白名单牵涉插件、模板和远程首页，留到后续版本单独做。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("X-Frame-Options", "SAMEORIGIN")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
