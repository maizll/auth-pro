package handler

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
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
	script, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(script)), "github.com") || strings.Contains(string(script), "githubusercontent") {
		t.Fatal("backend/handler/install.sh exposes a repository address")
	}
	if strings.Contains(string(script), "AUTH_PRO_UPDATE_BASE") {
		t.Fatal("backend/handler/install.sh can retarget the official site")
	}
	if !strings.Contains(string(script), "https://auth.maizll.com/api/v1/update/latest.json") || strings.Contains(string(script), `bash "$WORKDIR/install.sh"`) {
		t.Fatal("backend/handler/install.sh does not download the official package, or it still runs install.sh from the package")
	}
	if !strings.Contains(string(script), "https://auth.maizll.com/api/v1/update/helpers.json") || !strings.Contains(string(script), "https://auth.maizll.com/baota-panel.py") || !strings.Contains(string(script), "https://auth.maizll.com/guardian-start.sh") {
		t.Fatal("backend/handler/install.sh does not download helpers from the official site")
	}
	if strings.Contains(string(script), "安装包里缺少 baota-panel.py") || strings.Contains(string(script), "安装包里缺少 guardian-start.sh") {
		t.Fatal("backend/handler/install.sh still takes helpers from the client package")
	}
	if !strings.Contains(string(script), "https://auth.maizll.com/api/v1/update/package/") || !installScriptPinsOfficialOrigin(script) {
		t.Fatal("backend/handler/install.sh does not pin the official package URL")
	}
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		t.Fatal("install.sh must not be read from a published package")
		return nil, nil
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
		t.Fatal("served script is not backend/handler/install.sh")
	}
	if string(productInstallScript) != string(script) {
		t.Fatal("embedded install script drifted from backend/handler/install.sh")
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

func TestPublicHelperScriptsMatchManifest(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	if string(productBaotaPanelScript) == "" || !strings.Contains(string(productBaotaPanelScript), "supervisor") {
		t.Fatal("embedded panel helper is empty")
	}
	linked, err := os.ReadFile("baota-panel.py")
	if err != nil {
		t.Fatal(err)
	}
	if string(productBaotaPanelScript) != string(linked) {
		t.Fatal("embedded panel helper is not scripts/baota-panel.py")
	}
	source, err := os.ReadFile("../../scripts/baota-panel.py")
	if err != nil {
		t.Fatal(err)
	}
	if string(linked) != string(source) {
		t.Fatal("panel helper symlink drifted from scripts/baota-panel.py")
	}
	guardian, err := os.ReadFile("guardian_start.sh")
	if err != nil {
		t.Fatal(err)
	}
	if string(productGuardianStartScript) != string(guardian) {
		t.Fatal("embedded guardian template drifted")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicInstallRoute(router)
	manifestRec := httptest.NewRecorder()
	router.ServeHTTP(manifestRec, httptest.NewRequest(http.MethodGet, "/api/v1/update/helpers.json", nil))
	if manifestRec.Code != http.StatusOK {
		t.Fatalf("helpers status %d %s", manifestRec.Code, manifestRec.Body.String())
	}
	var manifest struct {
		BaotaPanel struct {
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
		} `json:"baotaPanel"`
		GuardianStart struct {
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
		} `json:"guardianStart"`
	}
	if err := json.Unmarshal(manifestRec.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.BaotaPanel.URL != "https://auth.maizll.com/baota-panel.py" || manifest.GuardianStart.URL != "https://auth.maizll.com/guardian-start.sh" {
		t.Fatalf("helper urls %#v %#v", manifest.BaotaPanel, manifest.GuardianStart)
	}
	panelRec := httptest.NewRecorder()
	router.ServeHTTP(panelRec, httptest.NewRequest(http.MethodGet, "/baota-panel.py", nil))
	guardRec := httptest.NewRecorder()
	router.ServeHTTP(guardRec, httptest.NewRequest(http.MethodGet, "/guardian-start.sh", nil))
	if panelRec.Code != http.StatusOK || guardRec.Code != http.StatusOK {
		t.Fatalf("panel %d guardian %d", panelRec.Code, guardRec.Code)
	}
	if sha256Hex(panelRec.Body.Bytes()) != manifest.BaotaPanel.SHA256 || sha256Hex(guardRec.Body.Bytes()) != manifest.GuardianStart.SHA256 {
		t.Fatal("helper checksum does not match helpers.json")
	}
	if !strings.Contains(panelRec.Body.String(), "--repair") {
		t.Fatal("served panel helper has no supervisor --repair")
	}
	if strings.Contains(strings.ToLower(panelRec.Body.String()), "github.com") || strings.Contains(strings.ToLower(manifestRec.Body.String()), "github.com") {
		t.Fatal("helper response leaked a repository address")
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

func TestPublicInstallRouteIgnoresPackageScript(t *testing.T) {
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	pkg := writeGzipTar(t, map[string]string{
		"install.sh": "#!/bin/sh\ncurl https://github.com/acme/widgets/releases/latest\n",
		"readme.txt": "no official script",
	})
	loadProductUpdateRecords = func() ([]productUpdateRecord, error) {
		t.Fatal("install.sh must not be read from a published package")
		info, err := os.Stat(pkg)
		if err != nil {
			return nil, err
		}
		return []productUpdateRecord{{Version: "1.7.7", PackagePath: pkg, FileSize: info.Size()}}, nil
	}
	script, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicInstallRoute(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/install.sh", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(script) {
		t.Fatalf("status %d", recorder.Code)
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "github.com/acme") {
		t.Fatal("response used the script packed inside the release archive")
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
