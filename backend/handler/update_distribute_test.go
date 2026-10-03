package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRewriteProductUpdateManifestKeepsHashAndHidesRepository(t *testing.T) {
	raw := []byte(`{
		"version":"1.7.1",
		"channel":"stable",
		"releasesUrl":"https://github.com/acme/widgets/releases/download/v1.7.1/releases.json",
		"package":{
			"os":"linux",
			"arch":"amd64",
			"fileName":"auth_pro-full-v1.7.1.tar.gz",
			"url":"https://github.com/acme/widgets/releases/download/v1.7.1/auth_pro-full-v1.7.1.tar.gz",
			"sha256":"` + strings.Repeat("ab", 32) + `",
			"size":42,
			"signature":"sha256:` + strings.Repeat("ab", 32) + `"
		},
		"notes":["从源站更新","请到 github.com/acme/widgets 查看"]
	}`)
	body, err := rewriteProductUpdateManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "github.com") || strings.Contains(text, "acme/widgets") || strings.Contains(text, "githubusercontent") {
		t.Fatalf("rewritten manifest leaked the repository: %s", text)
	}
	var manifest onlineUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Package.URL != "https://auth.maizll.com/api/v1/update/package/1.7.1" {
		t.Fatalf("package url = %s", manifest.Package.URL)
	}
	if manifest.ReleasesURL != "https://auth.maizll.com/api/v1/update/releases.json" {
		t.Fatalf("releases url = %s", manifest.ReleasesURL)
	}
	if manifest.Package.SHA256 != strings.Repeat("ab", 32) || manifest.Package.Size != 42 || manifest.Package.Signature != "sha256:"+strings.Repeat("ab", 32) {
		t.Fatalf("hash fields changed: %+v", manifest.Package)
	}
	if len(manifest.Notes) != 1 || manifest.Notes[0] != "从源站更新" {
		t.Fatalf("notes = %#v", manifest.Notes)
	}
}

func TestProductUpdateLatestUsesPublishedVersionAndCaches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	dir := t.TempDir()
	payload := []byte("package-bytes-for-manifest")
	path := filepath.Join(dir, "pkg.tar.gz")
	if err := os.WriteFile(path, payload, 0644); err != nil {
		t.Fatal(err)
	}
	var loads int32
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		atomic.AddInt32(&loads, 1)
		return []productUpdateRecord{{
			Version: "1.7.2", Title: "从源站更新", Changelog: "请到 github.com/acme/widgets 查看",
			PublishedAt: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
			PackagePath: path, FileSize: int64(len(payload)),
		}}, nil
	}
	router := gin.New()
	RegisterProductUpdateRoutes(router.Group("/api"))
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("status %d body %s", first.Code, first.Body.String())
	}
	if strings.Contains(first.Body.String(), "github.com") || strings.Contains(first.Body.String(), "acme/widgets") {
		t.Fatalf("response leaked the repository: %s", first.Body.String())
	}
	if !strings.Contains(first.Body.String(), "https://auth.maizll.com/api/v1/update/package/1.7.2") {
		t.Fatalf("body = %s", first.Body.String())
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
	if second.Code != http.StatusOK || atomic.LoadInt32(&loads) != 1 {
		t.Fatalf("cached status %d loads %d", second.Code, loads)
	}
}

func TestProductUpdateReleasesHideRepositoryNotes(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.tar.gz")
	if err := os.WriteFile(path, []byte("abc"), 0644); err != nil {
		t.Fatal(err)
	}
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		return []productUpdateRecord{{
			Version: "1.7.0", Title: "官网可以下载",
			Changelog:   "请到 GitHub 上的 auth-pro 仓库查看",
			PackagePath: path, PublishedAt: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC),
		}}, nil
	}
	body, err := productUpdateReleasesBody(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(body)), "github") || strings.Contains(string(body), "acme/widgets") {
		t.Fatalf("releases leaked: %s", body)
	}
	if !strings.Contains(string(body), "官网可以下载") {
		t.Fatalf("kept note missing: %s", body)
	}
}

func TestProductUpdatePackageServesPublishedFile(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	payload := []byte("package-bytes")
	path := filepath.Join(t.TempDir(), "auth_pro-full-v1.7.1.tar.gz")
	if err := os.WriteFile(path, payload, 0644); err != nil {
		t.Fatal(err)
	}
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		return []productUpdateRecord{{Version: "1.7.1", PackagePath: path, FileSize: int64(len(payload))}}, nil
	}
	first, name, err := productUpdatePackageFile(context.Background(), "1.7.1")
	if err != nil {
		t.Fatal(err)
	}
	if name != "auth_pro-full-v1.7.1.tar.gz" || first != path {
		t.Fatalf("path %s name %s", first, name)
	}
	cached, err := os.ReadFile(first)
	if err != nil || string(cached) != string(payload) {
		t.Fatalf("file %q err %v", cached, err)
	}
}

func TestProductUpdateMissingVersionDoesNotMentionHost(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		return nil, errProductUpdateUnavailable
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterProductUpdateRoutes(router.Group("/api"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "github") || strings.Contains(strings.ToLower(recorder.Body.String()), "token") {
		t.Fatalf("error leaked: %s", recorder.Body.String())
	}
}

func TestProductUpdateRepositoryRequiresExplicitConfig(t *testing.T) {
	t.Setenv(productUpdateRepoEnv, "")
	if _, _, err := productUpdateRepository(); err == nil {
		t.Fatal("empty env should not invent a repository")
	}
}

func TestProductUpdateRateLimit(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		return nil, errProductUpdateUnavailable
	}
	productUpdateJSONLimiter = newRateLimiter(2, productUpdateRateWindow)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterProductUpdateRoutes(router.Group("/api"))
	var last int
	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
		last = recorder.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("last status %d", last)
	}
}
