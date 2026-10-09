package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Cors(), SecurityHeaders())
	router.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200}) })
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/ping", nil),
		httptest.NewRequest(http.MethodGet, "/missing", nil),
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		for name, want := range map[string]string{
			"X-Frame-Options":        "SAMEORIGIN",
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "strict-origin-when-cross-origin",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Fatalf("%s %s: %s = %q, want %q", req.Method, req.URL.Path, name, got, want)
			}
		}
	}
}
