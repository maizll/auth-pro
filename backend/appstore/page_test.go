package appstore

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestPageRoutesServeAssetsAndHistoryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	staticFS := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>app-store</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app-store')")},
	}
	router := gin.New()
	if err := RegisterPageRoutes(router, staticFS); err != nil {
		t.Fatal(err)
	}

	for _, route := range []string{PagePrefix, PagePrefix + "/", PagePrefix + "/dashboard", PagePrefix + "/templates"} {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "app-store") {
			t.Fatalf("route %s response = %d %s", route, recorder.Code, recorder.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, PagePrefix+"/assets/app.js", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "console.log") {
		t.Fatalf("asset response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, PagePrefix+"/assets/missing.js", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing asset response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPageRoutesRequireIndex(t *testing.T) {
	router := gin.New()
	err := RegisterPageRoutes(router, fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("x")}})
	if err == nil {
		t.Fatal("expected missing index error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unexpected error: %v", err)
	}
}
