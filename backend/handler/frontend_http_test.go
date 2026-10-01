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

func TestHashedAssetLongCacheAndGzip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>disk-root</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	jsPath := filepath.Join(dir, "assets", "index-Ab12Cd34.js")
	if err := os.WriteFile(jsPath, []byte("console.log(1)"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsPath+".gz", []byte("gzip-body"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_FRONTEND_DIR", dir)
	t.Setenv("AUTO_PRO_ALLOW_EMBEDDED_FRONTEND", "")

	router := gin.New()
	if err := RegisterFrontend(router, fstest.MapFS{"index.html": {Data: []byte("<html>embed</html>")}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/index-Ab12Cd34.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "gzip-body" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("encoding=%q", rec.Header().Get("Content-Encoding"))
	}
	cache := rec.Header().Get("Cache-Control")
	if !strings.Contains(cache, "immutable") || !strings.Contains(cache, "31536000") {
		t.Fatalf("cache=%q", cache)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("type=%q", rec.Header().Get("Content-Type"))
	}

	plain := httptest.NewRequest(http.MethodGet, "/assets/index-Ab12Cd34.js", nil)
	plainRec := httptest.NewRecorder()
	router.ServeHTTP(plainRec, plain)
	if plainRec.Body.String() != "console.log(1)" || plainRec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("plain body=%q encoding=%q", plainRec.Body.String(), plainRec.Header().Get("Content-Encoding"))
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
