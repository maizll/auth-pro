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

func TestProductUpdateLatestUsesStoredTokenAndCaches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	t.Setenv(productUpdateRepoEnv, "acme/widgets")

	const token = "github_pat_update_secret"
	store := newMemorySourceStore()
	if err := store.SaveReleaseSettings(sourceReleaseSettings{
		Provider: "github", Owner: "other", Repo: "paid", Token: token,
	}); err != nil {
		t.Fatal(err)
	}
	restore := SetSourceStationStoreForTest(store)
	t.Cleanup(restore)

	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if strings.Contains(r.URL.Path, "/releases/assets/") {
			_, _ = w.Write([]byte(`{"version":"1.7.1","package":{"os":"linux","arch":"amd64","fileName":"auth_pro-full-v1.7.1.tar.gz","url":"https://github.com/acme/widgets/releases/download/v1.7.1/auth_pro-full-v1.7.1.tar.gz","sha256":"` + strings.Repeat("cd", 32) + `","size":9,"signature":"sha256:` + strings.Repeat("cd", 32) + `"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.7.1","assets":[{"id":7,"name":"latest.json","url":"` + strings.TrimRight(productUpdateGitHubAPI, "/") + `/repos/acme/widgets/releases/assets/7"}]}`))
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL

	router := gin.New()
	RegisterProductUpdateRoutes(router.Group("/api"))
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("status %d body %s", first.Code, first.Body.String())
	}
	if strings.Contains(first.Body.String(), token) || strings.Contains(first.Body.String(), "acme/widgets") || strings.Contains(first.Body.String(), "github.com") {
		t.Fatalf("response leaked secrets: %s", first.Body.String())
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("cached status %d", second.Code)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("upstream hits = %d, want 2 (release + asset, then cache)", atomic.LoadInt32(&hits))
	}
}

func TestProductUpdateWorksWithoutTokenWhenReleaseIsPublic(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	t.Setenv(productUpdateRepoEnv, "acme/widgets")
	restore := SetSourceStationStoreForTest(newMemorySourceStore())
	t.Cleanup(restore)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("public fetch sent authorization")
		}
		if strings.Contains(r.URL.Path, "/releases/assets/") {
			_, _ = w.Write([]byte(`{"releases":[{"version":"1.7.0","channel":"stable","releasedAt":"2026-09-27T00:00:00Z","notes":["请到 GitHub 上的 auth-pro 仓库查看","官网可以下载"]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"assets":[{"id":3,"name":"releases.json","url":"` + strings.TrimRight(productUpdateGitHubAPI, "/") + `/repos/acme/widgets/releases/assets/3"}]}`))
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL

	body, err := productUpdateReleasesBody(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "github") || strings.Contains(string(body), "acme/widgets") {
		t.Fatalf("releases leaked: %s", body)
	}
	if !strings.Contains(string(body), "官网可以下载") {
		t.Fatalf("kept note missing: %s", body)
	}
}

func TestProductUpdatePackageCachesByVersion(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv(productUpdateRepoEnv, "acme/widgets")
	restore := SetSourceStationStoreForTest(newMemorySourceStore())
	t.Cleanup(restore)

	var downloads int32
	payload := []byte("package-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/assets/") {
			atomic.AddInt32(&downloads, 1)
			_, _ = w.Write(payload)
			return
		}
		_, _ = w.Write([]byte(`{"assets":[{"id":9,"name":"auth_pro-full-v1.7.1.tar.gz","url":"` + strings.TrimRight(productUpdateGitHubAPI, "/") + `/repos/acme/widgets/releases/assets/9"}]}`))
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL

	first, name, err := productUpdatePackageFile(context.Background(), "1.7.1")
	if err != nil {
		t.Fatal(err)
	}
	if name != "auth_pro-full-v1.7.1.tar.gz" {
		t.Fatalf("name = %s", name)
	}
	second, _, err := productUpdatePackageFile(context.Background(), "1.7.1")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || atomic.LoadInt32(&downloads) != 1 {
		t.Fatalf("path %s/%s downloads %d", first, second, downloads)
	}
	cached, err := os.ReadFile(filepath.Join(dataDir, "update-cache", "1.7.1", name))
	if err != nil || string(cached) != string(payload) {
		t.Fatalf("cache %q err %v", cached, err)
	}
}

func TestProductUpdateMissingRepositoryDoesNotMentionHost(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	t.Setenv(productUpdateRepoEnv, "not a repo")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterProductUpdateRoutes(router.Group("/api"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/update/latest.json", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "github") || strings.Contains(recorder.Body.String(), "not a repo") || strings.Contains(strings.ToLower(recorder.Body.String()), "token") {
		t.Fatalf("error leaked: %s", recorder.Body.String())
	}
}

func TestProductUpdateDefaultRepositoryIsClientRepo(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	t.Setenv(productUpdateRepoEnv, "")
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL
	if _, err := productUpdateLatestBody(context.Background(), true); err == nil {
		t.Fatal("empty upstream should fail")
	}
	if gotPath != "/repos/maizll/auth-pro-client/releases/latest" {
		t.Fatalf("path = %s", gotPath)
	}
}

func TestProductUpdateRateLimit(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL
	productUpdateJSONLimiter = newProductUpdateRateLimiter(2, productUpdateRateWindow)
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
