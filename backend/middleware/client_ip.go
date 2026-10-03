package middleware

import "github.com/gin-gonic/gin"

// localProxies 是唯一可信的反向代理：同一台机器上的 nginx（宝塔布局、安装脚本生成的配置都在本机）。
// 不信任的来源发来的 X-Forwarded-For / X-Real-IP 一律忽略，c.ClientIP() 直接取 TCP 对端地址，
// 这样攻击者无法靠伪造请求头换 IP，绕过登录锁定和授权校验限流，也不能往日志里写假 IP。
var localProxies = []string{"127.0.0.1/8", "::1/128"}

// TrustLocalProxiesOnly 让 gin 只信任本机反代转来的客户端地址。
// 先看 X-Real-IP：安装脚本、宝塔默认反代和部署文档都把它设成 $remote_addr，客户端带来的同名头会被覆盖；
// 有的反代配置只设 X-Real-IP、原样转发客户端的 X-Forwarded-For，先看 XFF 就会采用伪造值。
// 没有 X-Real-IP 时再看 X-Forwarded-For：老配置的 $proxy_add_x_forwarded_for 由 gin 从右往左取第一个不可信地址，
// 也就是 nginx 自己追加的真实对端地址，伪造的左侧部分不会被采用。
func TrustLocalProxiesOnly(engine *gin.Engine) error {
	engine.ForwardedByClientIP = true
	engine.RemoteIPHeaders = []string{"X-Real-IP", "X-Forwarded-For"}
	return engine.SetTrustedProxies(localProxies)
}
