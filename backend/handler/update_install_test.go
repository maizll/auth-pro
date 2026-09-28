package handler

import (
	"archive/tar"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func writeGzipTar(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth_pro-full-v1.7.4.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPublicInstallRouteServesScriptFromPackage(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(script)), "github.com") || strings.Contains(string(script), "githubusercontent") {
		t.Fatal("scripts/install.sh exposes a repository address")
	}
	if strings.Contains(string(script), "AUTH_PRO_UPDATE_BASE") {
		t.Fatal("scripts/install.sh can retarget the official site")
	}
	if !strings.Contains(string(script), "https://auth.maizll.com/api/v1/update/latest.json") || !strings.Contains(string(script), "baota-install.sh") {
		t.Fatal("scripts/install.sh does not download the official package or reuse baota-install.sh")
	}
	if !strings.Contains(string(script), "https://auth.maizll.com/api/v1/update/package/") || !installScriptPinsOfficialOrigin(script) {
		t.Fatal("scripts/install.sh does not pin the official package URL")
	}
	pkg := writeGzipTar(t, map[string]string{
		"install.sh":        string(script),
		"nested/install.sh": "#!/bin/sh\necho nested\n",
		"note.txt":          "https://github.com/example/hidden",
	})
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		info, statErr := os.Stat(pkg)
		if statErr != nil {
			return nil, statErr
		}
		return []productUpdateRecord{{
			Version: "1.7.4", PackagePath: pkg, FileSize: info.Size(),
		}}, nil
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicInstallRoute(router)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != string(script) {
		t.Fatal("served script is not the copy inside the published package")
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "text/x-shellscript") {
		t.Fatalf("content type %s", recorder.Header().Get("Content-Type"))
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "github.com") {
		t.Fatal("response leaked a repository address")
	}
	rewritten := httptest.NewRequest(http.MethodGet, "/install.sh?base=https://evil.example", nil)
	rewritten.Host = "evil.example"
	rewritten.Header.Set("X-Forwarded-Host", "evil.example")
	rewrittenRec := httptest.NewRecorder()
	router.ServeHTTP(rewrittenRec, rewritten)
	if rewrittenRec.Code != http.StatusOK || rewrittenRec.Body.String() != string(script) {
		t.Fatal("request host or query rewrote the install script")
	}
	if strings.Contains(rewrittenRec.Body.String(), "evil.example") {
		t.Fatal("request host leaked into the install script")
	}
}

func TestInstallScriptPinsOfficialOrigin(t *testing.T) {
	pinned := "#!/bin/sh\ncurl \"https://auth.maizll.com/api/v1/update/latest.json\"\nurl=\"https://auth.maizll.com/api/v1/update/package/${VERSION}\"\n"
	if !installScriptPinsOfficialOrigin([]byte(pinned)) {
		t.Fatal("a script with a fixed official origin was rejected")
	}
	rejected := []string{
		"#!/bin/sh\nBASE=\"${AUTH_PRO_UPDATE_BASE:-https://auth.maizll.com}\"\ncurl \"${BASE}/api/v1/update/latest.json\"\n",
		"#!/bin/sh\ncurl \"https://auth.maizll.com/api/v1/update/latest.json\"\ncurl \"${BASE}/api/v1/update/package/${VERSION}\"\n",
		"#!/bin/sh\ncurl \"http://127.0.0.1/api/v1/update/latest.json\"\nurl=\"http://127.0.0.1/api/v1/update/package/${VERSION}\"\n",
		"#!/bin/sh\n# https://auth.maizll.com/api/v1/update/latest.json\ncurl \"$BASE/api/v1/update/latest.json\"\n",
	}
	for _, body := range rejected {
		if installScriptPinsOfficialOrigin([]byte(body)) {
			t.Fatalf("accepted a script that can leave the official site: %s", body)
		}
	}
}

func TestPublicInstallRouteRejectsRetargetableScript(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	hooked := "#!/bin/sh\nBASE=\"${AUTH_PRO_UPDATE_BASE:-https://auth.maizll.com}\"\ncurl \"${BASE}/api/v1/update/latest.json\"\n"
	pkg := writeGzipTar(t, map[string]string{"install.sh": hooked})
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		info, err := os.Stat(pkg)
		if err != nil {
			return nil, err
		}
		return []productUpdateRecord{{Version: "1.7.4", PackagePath: pkg, FileSize: info.Size()}}, nil
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicInstallRoute(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/install.sh", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "AUTH_PRO_UPDATE_BASE") || strings.Contains(recorder.Body.String(), "auth.maizll.com") {
		t.Fatalf("error leaked the script: %s", recorder.Body.String())
	}
}

func TestPublicInstallRouteRejectsScriptThatLeaksRepository(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	pkg := writeGzipTar(t, map[string]string{
		"install.sh": "#!/bin/sh\ncurl https://github.com/acme/widgets/releases/latest\n",
	})
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		info, err := os.Stat(pkg)
		if err != nil {
			return nil, err
		}
		return []productUpdateRecord{{Version: "1.7.4", PackagePath: pkg, FileSize: info.Size()}}, nil
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicInstallRoute(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/install.sh", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "github") || strings.Contains(recorder.Body.String(), "acme/widgets") {
		t.Fatalf("error leaked: %s", recorder.Body.String())
	}
}

func TestPublicInstallRouteMissingScript(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	pkg := writeGzipTar(t, map[string]string{"readme.txt": "no script"})
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		info, err := os.Stat(pkg)
		if err != nil {
			return nil, err
		}
		return []productUpdateRecord{{Version: "1.7.4", PackagePath: pkg, FileSize: info.Size()}}, nil
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicInstallRoute(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/install.sh", nil))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "没有安装脚本") {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestProductUpdatePackageDownloadNeedsNoCredential(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	clientPkg := []byte("client-full-package")
	otherPkg := []byte("older-client-package")
	dir := t.TempDir()
	latest := filepath.Join(dir, "latest.tar.gz")
	older := filepath.Join(dir, "older.tar.gz")
	if err := os.WriteFile(latest, clientPkg, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(older, otherPkg, 0o644); err != nil {
		t.Fatal(err)
	}
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		return []productUpdateRecord{
			{Version: "1.7.4", PackagePath: latest, FileSize: int64(len(clientPkg))},
			{Version: "1.7.3", PackagePath: older, FileSize: int64(len(otherPkg))},
		}, nil
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterProductUpdateRoutes(router.Group("/api"))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/update/package/1.7.4", nil)
	if request.Header.Get("Authorization") != "" {
		t.Fatal("test request must not carry credentials")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(clientPkg) {
		t.Fatalf("status %d body %q", recorder.Code, recorder.Body.String())
	}
	olderRec := httptest.NewRecorder()
	router.ServeHTTP(olderRec, httptest.NewRequest(http.MethodGet, "/api/v1/update/package/1.7.3", nil))
	if olderRec.Code != http.StatusOK || olderRec.Body.String() != string(otherPkg) {
		t.Fatal("versioned download did not stay on that published client package")
	}
	plugin := httptest.NewRecorder()
	router.ServeHTTP(plugin, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/package/plugin/paid-plugin", nil))
	if plugin.Code == http.StatusOK || strings.Contains(plugin.Body.String(), "client-full-package") {
		t.Fatal("update package route served a catalog plugin path")
	}
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/update/package/9.9.9", nil))
	if missing.Code == http.StatusOK || strings.Contains(missing.Body.String(), dir) {
		t.Fatalf("missing version leaked a path: %d %s", missing.Code, missing.Body.String())
	}
}
