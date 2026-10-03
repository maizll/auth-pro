package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 复现 H1：默认 gin 信任所有代理，任何人带一个 X-Forwarded-For 就能换 IP。
func TestClientIPIgnoresForgedForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name   string
		remote string
		xff    string
		realIP string
		want   string
	}{
		{name: "公网直连伪造 XFF", remote: "198.51.100.7:5000", xff: "1.2.3.4", want: "198.51.100.7"},
		{name: "公网直连伪造 X-Real-IP", remote: "198.51.100.7:5000", realIP: "1.2.3.4", want: "198.51.100.7"},
		{name: "本机 nginx 用 $remote_addr", remote: "127.0.0.1:40000", xff: "203.0.113.9", want: "203.0.113.9"},
		{name: "老配置 $proxy_add_x_forwarded_for 带伪造前缀", remote: "127.0.0.1:40000", xff: "1.2.3.4, 203.0.113.9", want: "203.0.113.9"},
		{name: "本机 nginx 只给 X-Real-IP", remote: "127.0.0.1:40000", realIP: "203.0.113.9", want: "203.0.113.9"},
		{name: "IPv6 本机反代", remote: "[::1]:40000", xff: "2001:db8::5", want: "2001:db8::5"},
		{name: "反代只设 X-Real-IP、原样转发伪造的 XFF", remote: "127.0.0.1:40000", xff: "1.2.3.4", realIP: "203.0.113.9", want: "203.0.113.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			if err := TrustLocalProxiesOnly(engine); err != nil {
				t.Fatal(err)
			}
			engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
			req := httptest.NewRequest(http.MethodGet, "/ip", nil)
			req.RemoteAddr = tc.remote
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Body.String() != tc.want {
				t.Fatalf("ClientIP = %q, want %q", rec.Body.String(), tc.want)
			}
		})
	}
}
