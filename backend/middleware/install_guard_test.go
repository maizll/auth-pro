package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func TestInstallGuardBlocksWritesWhenLockExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	if err := config.CreateLockFile(); err != nil {
		t.Fatal(err)
	}

	called := false
	router := gin.New()
	router.POST("/init-tables", InstallGuard(), func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/init-tables", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if called {
		t.Fatal("install handler ran while install.lock exists")
	}
}

func TestInstallGuardAllowsFreshInstallWithoutOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	called := false
	router := gin.New()
	router.POST("/create-admin", InstallGuard(), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/create-admin", nil))

	if recorder.Code != http.StatusNoContent || !called {
		t.Fatalf("fresh install status=%d called=%v body=%s", recorder.Code, called, recorder.Body.String())
	}
}
