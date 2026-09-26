package handler

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestRegisterFrontendServesDiskRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>disk-root</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_FRONTEND_DIR", dir)
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "")

	router := gin.New()
	if err := RegisterFrontend(router, fstest.MapFS{"index.html": {Data: []byte("<html>stale-embed</html>")}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "disk-root") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Fatalf("index.html Cache-Control = %q", cache)
	}
	indexReq := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	indexReq.Header.Set("If-Modified-Since", "Mon, 02 Jan 2006 15:04:05 GMT")
	indexRec := httptest.NewRecorder()
	router.ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusOK || !strings.Contains(indexRec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("index.html status=%d cache=%q", indexRec.Code, indexRec.Header().Get("Cache-Control"))
	}
	if strings.Contains(rec.Body.String(), "stale-embed") {
		t.Fatal("disk root must not fall back to embed")
	}
}

func TestRegisterFrontendReturns503WhenDiskDisappears(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>ok</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_FRONTEND_DIR", dir)
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "")

	router := gin.New()
	if err := RegisterFrontend(router, fstest.MapFS{"index.html": {Data: []byte("<html>embed</html>")}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "503") || !strings.Contains(rec.Body.String(), "frontend") {
		t.Fatalf("503 page missing diagnostics: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<html>embed</html>") {
		t.Fatal("missing disk frontend must not silently serve embed")
	}
}

func TestRegisterFrontendEmbedRequiresOptIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_FRONTEND_DIR", "")
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "")
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	router := gin.New()
	err := RegisterFrontend(router, fstest.MapFS{"index.html": {Data: []byte("<html>embed</html>")}})
	if err == nil {
		t.Fatal("production register must fail without disk frontend")
	}
}

func TestRegisterFrontendServesEmbedWhenOptedIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AUTO_PRO_FRONTEND_DIR", "")
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "1")
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	var embedFS fs.FS = fstest.MapFS{"index.html": {Data: []byte("<html>bootstrap-embed</html>")}}
	router := gin.New()
	if err := RegisterFrontend(router, embedFS); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "bootstrap-embed") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Fatalf("embed index Cache-Control = %q", cache)
	}
}
