package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"auto_pro/config"
	"auto_pro/updatesign"
)

func validOnlineUpdateManifestForTest() *onlineUpdateManifest {
	return &onlineUpdateManifest{
		Version:    "1.0.1",
		Channel:    "stable",
		MinVersion: "0.0.0",
		Package: onlineUpdatePackage{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			FileName:  "auth_pro-full-v1.0.1.tar.gz",
			URL:       "https://auth.maizll.com/api/v1/update/package/1.0.1",
			SHA256:    strings.Repeat("a", 64),
			Signature: "sha256:" + strings.Repeat("a", 64),
			Size:      1024,
		},
		Actions: onlineUpdateActions{
			UpdateFrontend: true,
			UpdateBackend:  true,
			RestartBackend: true,
			BackupDatabase: true,
		},
	}
}

func useOnlineUpdateManifestURL(t *testing.T, raw string) {
	t.Helper()
	previous := onlineUpdateManifestURLForTest
	onlineUpdateManifestURLForTest = func() string { return raw }
	t.Cleanup(func() { onlineUpdateManifestURLForTest = previous })
}

func TestValidateOnlineUpdateManifest(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := validateOnlineUpdateManifest(validOnlineUpdateManifestForTest()); err != nil {
			t.Fatalf("validateOnlineUpdateManifest() error = %v", err)
		}
	})

	t.Run("placeholder URL", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.URL = "https://updates.your-domain.com/packages/update.tar.gz"
		if err := validateOnlineUpdateManifest(manifest); err == nil {
			t.Fatal("placeholder URL was accepted")
		}
	})

	t.Run("invalid hash", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.SHA256 = "替换为真实SHA256"
		if err := validateOnlineUpdateManifest(manifest); err == nil {
			t.Fatal("invalid SHA256 was accepted")
		}
	})

	t.Run("untrusted Gitee repository", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.URL = "https://gitee.com/another/repository/releases/download/v1.0.1/update.tar.gz"
		if err := validateOnlineUpdateManifest(manifest); err == nil {
			t.Fatal("package from another Gitee repository was accepted")
		}
	})

	t.Run("untrusted third-party host", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.URL = "https://downloads.example.com/update.tar.gz"
		if err := validateOnlineUpdateManifest(manifest); err == nil {
			t.Fatal("package from a third-party host was accepted")
		}
	})

	t.Run("non-standard HTTPS port", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.URL = "https://gitee.com:8443/Zcy-sa/auth-pro/releases/download/v1.0.1/update.tar.gz"
		if err := validateOnlineUpdateManifest(manifest); err == nil {
			t.Fatal("non-standard HTTPS port was accepted")
		}
	})

	t.Run("insecure URL", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.URL = "http://gitee.com/Zcy-sa/auth-pro/releases/download/v1.0.1/update.tar.gz"
		if err := validateOnlineUpdateManifest(manifest); err == nil {
			t.Fatal("HTTP package URL was accepted")
		}
	})

	t.Run("repository hosts are rejected", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		for _, packageURL := range []string{
			"https://gitee.com/acme/auth-pro-mirror/releases/download/v1.0.1/auth_pro-full-v1.0.1.tar.gz",
			"https://github.com/example/widgets/releases/download/v1.2.2/auth_pro-full-v1.2.2.tar.gz",
			"https://api.github.com/repos/example/widgets/releases/latest",
			"https://raw.githubusercontent.com/example/widgets/master/latest.json",
		} {
			manifest.Package.URL = packageURL
			if err := validateOnlineUpdateManifest(manifest); err == nil {
				t.Fatalf("package URL %q was accepted", packageURL)
			}
		}
	})

	t.Run("wrong architecture", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		if runtime.GOARCH == "amd64" {
			manifest.Package.Arch = "arm64"
		} else {
			manifest.Package.Arch = "amd64"
		}
		if err := onlineUpdateRuntimeCompatibility(runtime.GOOS, runtime.GOARCH, manifest); err == nil {
			t.Fatal("incompatible architecture was accepted")
		}
	})
}

func TestParseOnlineUpdateVersionRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"1", "1.2", "1.2.3-beta", "release-1.2.3", "1/../../backend"} {
		if _, ok := parseOnlineUpdateVersion(value); ok {
			t.Fatalf("unsafe version %q was accepted", value)
		}
	}
	for _, value := range []string{"1.2.3", "v1.2.3"} {
		if _, ok := parseOnlineUpdateVersion(value); !ok {
			t.Fatalf("valid version %q was rejected", value)
		}
	}
}

func TestOnlineUpdateRedirectStaysOnSourceHost(t *testing.T) {
	useOnlineUpdateManifestURL(t, "https://auth.maizll.com/api/v1/update/latest.json")
	client, err := newOnlineUpdateHTTPClient("https://auth.maizll.com/api/v1/update/package/1.7.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	via := []*http.Request{{URL: mustParseOnlineUpdateTestURL(t, "https://auth.maizll.com/api/v1/update/package/1.7.1")}}
	if err := client.CheckRedirect(&http.Request{URL: mustParseOnlineUpdateTestURL(t, "https://auth.maizll.com/api/v1/update/package/1.7.1?retry=1")}, via); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"https://github.com/example/widgets/releases/download/v1.7.1/pkg.tar.gz",
		"https://release-assets.githubusercontent.com/pkg.tar.gz",
		"http://auth.maizll.com/api/v1/update/package/1.7.1",
	} {
		if err := client.CheckRedirect(&http.Request{URL: mustParseOnlineUpdateTestURL(t, target)}, via); err == nil {
			t.Fatalf("redirect %s was accepted", target)
		}
	}
}

func mustParseOnlineUpdateTestURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestValidateExtractedOnlineUpdatePackageAcceptsUTF8BOM(t *testing.T) {
	stagingDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stagingDir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(stagingDir, "index.html"):          "index",
		filepath.Join(stagingDir, "version.json"):        `{"version":"1.0.5"}`,
		filepath.Join(stagingDir, "backend", "auth_pro"): "binary",
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{
		"version":"1.0.5",
		"frontendDir":".",
		"backendFile":"backend/auth_pro",
		"requiredFiles":[]
	}`)...)
	if err := os.WriteFile(filepath.Join(stagingDir, "manifest.json"), manifest, 0644); err != nil {
		t.Fatal(err)
	}

	pkg, err := validateExtractedOnlineUpdatePackage(stagingDir, "1.0.5")
	if err != nil {
		t.Fatalf("BOM manifest was rejected: %v", err)
	}
	if pkg.Version != "1.0.5" || pkg.FrontendDir != "." || pkg.BackendFile != "backend/auth_pro" {
		t.Fatalf("unexpected manifest: %#v", pkg)
	}
}

func TestOnlineUpdateHistory(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"releases": [
				{"version":"1.0.0","channel":"","releasedAt":"2026-01-01T00:00:00Z","notes":[" 首个版本 ",""]},
				{"version":"1.2.0","channel":"stable","releasedAt":"2026-03-01T00:00:00Z","notes":["功能更新"]},
				{"version":"1.0.0","channel":"stable","releasedAt":"2026-02-01T00:00:00Z","notes":["重复记录"]}
			]
		}`))
	}))
	defer server.Close()
	originalTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = originalTransport }()

	useOnlineUpdateManifestURL(t, server.URL+"/latest.json")
	manifest := validOnlineUpdateManifestForTest()
	manifest.ReleasesURL = server.URL + "/releases.json"

	releases, releasesURL, err := fetchOnlineUpdateReleases(manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	if releasesURL != manifest.ReleasesURL {
		t.Fatalf("releases URL = %q, want %q", releasesURL, manifest.ReleasesURL)
	}
	if len(releases) != 2 || releases[0].Version != "1.2.0" || releases[1].Version != "1.0.0" {
		t.Fatalf("unexpected releases: %#v", releases)
	}
	if releases[1].Channel != "stable" || len(releases[1].Notes) != 1 || releases[1].Notes[0] != "首个版本" {
		t.Fatalf("release was not normalized: %#v", releases[1])
	}
}

func TestOnlineUpdateHistoryRejectsCrossOriginURL(t *testing.T) {
	manifest := validOnlineUpdateManifestForTest()
	manifest.ReleasesURL = "https://mirror.example.com/releases.json"
	if _, err := resolveOnlineUpdateReleasesURL(manifest); err == nil {
		t.Fatal("cross-origin releases URL was accepted")
	}
}

func TestOnlineUpdateHistoryKeepsChineseNotesFromReleasesJSON(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{
			"releases": [
				{"version":"1.2.2","channel":"stable","releasedAt":"2026-09-20T00:00:00Z","notes":["在线更新改为 GitHub Releases，支持 latest.json 与 SHA256 校验","开发者入驻：申请、后台审核通过/拒绝、取消开发者"]}
			]
		}`))
	}))
	defer server.Close()
	originalTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = originalTransport }()

	useOnlineUpdateManifestURL(t, server.URL+"/latest.json")
	manifest := validOnlineUpdateManifestForTest()
	manifest.ReleasesURL = server.URL + "/releases.json"

	releases, _, err := fetchOnlineUpdateReleases(manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || len(releases[0].Notes) != 2 {
		t.Fatalf("unexpected releases: %#v", releases)
	}
	if releases[0].Notes[0] != "在线更新改为 GitHub Releases，支持 latest.json 与 SHA256 校验" {
		t.Fatalf("Chinese releases.json notes were lost: %#v", releases[0].Notes)
	}
}

func TestOnlineUpdateAvailable(t *testing.T) {
	manifest := validOnlineUpdateManifestForTest()
	if available, versionErr := onlineUpdateAvailable("1.0.0", manifest); !available || versionErr != "" {
		t.Fatalf("onlineUpdateAvailable() = %v, %q", available, versionErr)
	}
	if available, versionErr := onlineUpdateAvailable("1.0.1", manifest); available || versionErr != "" {
		t.Fatalf("same version result = %v, %q", available, versionErr)
	}
	manifest.MinVersion = "1.0.0"
	if available, versionErr := onlineUpdateAvailable("0.9.9", manifest); available || versionErr == "" {
		t.Fatalf("minimum version was not enforced: %v, %q", available, versionErr)
	}
}

func TestOnlineUpdateJobResultSurvivesRestart(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	job := &onlineUpdateJob{
		ID:        "U-test-persist",
		Status:    "restarting",
		Message:   "服务正在切换并重启",
		Progress:  95,
		Version:   "1.0.1",
		Logs:      []string{"开始更新"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	persistOnlineUpdateJob(job)
	if err := os.WriteFile(onlineUpdateJobStatePath(job.ID)+".result", []byte("success\n"), 0600); err != nil {
		t.Fatal(err)
	}

	loaded := loadOnlineUpdateJob(job.ID)
	if loaded == nil || loaded.Status != "success" || loaded.Message != "更新完成" || loaded.Progress != 100 {
		t.Fatalf("unexpected persisted job: %#v", loaded)
	}
	loadedAgain := loadOnlineUpdateJob(job.ID)
	if loadedAgain == nil || len(loadedAgain.Logs) != 2 {
		t.Fatalf("result reconciliation was not idempotent: %#v", loadedAgain)
	}
}

func TestWriteOnlineUpdateScriptSupportsWebsiteRoot(t *testing.T) {
	shell, err := exec.LookPath("/bin/sh")
	if err != nil {
		t.Skip("/bin/sh is not available")
	}

	root := t.TempDir()
	dataDir := filepath.Join(root, "backend")
	frontendSource := filepath.Join(root, "staging-frontend")
	stagingDir := filepath.Join(root, "staging")
	for _, dir := range []string{dataDir, frontendSource, filepath.Join(stagingDir, "backend")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(root, "index.html"):                "old",
		filepath.Join(frontendSource, "index.html"):      "new",
		filepath.Join(frontendSource, "version.json"):    `{"version":"1.0.1"}`,
		filepath.Join(stagingDir, "backend", "auth_pro"): "binary",
	} {
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv("AUTO_PRO_SERVICE_NAME", "auth_pro_test")

	scriptPath, err := writeOnlineUpdateScript(
		"U-script-test",
		stagingDir,
		&extractedOnlineUpdateManifest{BackendFile: "backend/auth_pro"},
		frontendSource,
		"1.0.1",
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, expected := range []string{
		`FRONTEND_MODE="rename"`,
		`cp -a "$FRONTEND_SOURCE" "$STAGING"`,
		`mv "$STAGING" "$FRONTEND_CURRENT"`,
		`mv -f "$APP_STAGE" "$APP_BIN"`,
		`finish_job success`,
		`rollback_frontend`,
		`if [ "$FRONTEND_ONLY" = "1" ]; then`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("generated script missing %q", expected)
		}
	}
	if strings.Contains(script, `cp -a "$FRONTEND_SOURCE/assets/." "$FRONTEND_ROOT/assets/"`) {
		t.Fatal("generated script still copies assets into the live frontend directory")
	}
	standalone := script[strings.Index(script, "# BEGIN STANDALONE RESTART"):]
	stopAt := strings.Index(standalone, `if ! stop_old_process; then`)
	mvAt := strings.Index(standalone, `mv -f "$APP_STAGE" "$APP_BIN"`)
	startAt := strings.Index(standalone, `log "port free, starting new process"`)
	if stopAt < 0 || mvAt < 0 || startAt < 0 || stopAt > mvAt || mvAt > startAt {
		t.Fatal("standalone path must stop the old process, confirm the port is free, replace the binary, then start")
	}
	if !strings.Contains(script, "不是本站") || !strings.Contains(script, "kill -KILL") {
		t.Fatal("script must refuse foreign listeners and SIGKILL this site's auth_pro after SIGTERM times out")
	}
	if !strings.Contains(script, `PROCESS_MANAGER='none'`) {
		t.Fatal("default process manager should be standalone when no supervisor is detected")
	}
	if !strings.Contains(script, `--max-time 2`) {
		t.Fatal("health check must time out individual probes")
	}
	if output, err := exec.Command(shell, "-n", scriptPath).CombinedOutput(); err != nil {
		t.Fatalf("generated script syntax error: %v\n%s", err, output)
	}

	if config.GetDataDir() != dataDir {
		t.Fatalf("unexpected data directory: %s", config.GetDataDir())
	}
}

func TestValidateOnlineUpdateManifestRejectsMissingOrWrongSignature(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.Signature = "  "
		err := validateOnlineUpdateManifest(manifest)
		if err == nil || !strings.Contains(err.Error(), "缺少独立签名") {
			t.Fatalf("missing signature was accepted: %v", err)
		}
	})

	t.Run("wrong", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.Signature = "sha256:" + strings.Repeat("b", 64)
		err := validateOnlineUpdateManifest(manifest)
		if err == nil || !strings.Contains(err.Error(), "签名与 SHA256 不一致") {
			t.Fatalf("wrong signature was accepted: %v", err)
		}
	})

	t.Run("unrelated asset name", func(t *testing.T) {
		manifest := validOnlineUpdateManifestForTest()
		manifest.Package.FileName = "latest.json"
		err := validateOnlineUpdateManifest(manifest)
		if err == nil || !strings.Contains(err.Error(), "文件名与版本不一致") {
			t.Fatalf("digest for another asset was accepted: %v", err)
		}
	})
}

func TestExecuteOnlineUpdateRejectsMissingSignature(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	restore := stubOnlineUpdateApply(t, func(string, *onlineUpdateManifest) (string, error) {
		t.Fatal("download started without a signature")
		return "", nil
	})
	defer restore()

	manifest := validOnlineUpdateManifestForTest()
	manifest.Package.Signature = ""
	err := executeOnlineUpdate("job-missing-signature", manifest)
	if err == nil || !strings.Contains(err.Error(), "缺少独立签名") {
		t.Fatalf("missing signature was applied: %v", err)
	}
}

func TestExecuteOnlineUpdateRejectsWrongSignature(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	restore := stubOnlineUpdateApply(t, func(string, *onlineUpdateManifest) (string, error) {
		t.Fatal("download started with a wrong signature")
		return "", nil
	})
	defer restore()

	manifest := validOnlineUpdateManifestForTest()
	manifest.Package.Signature = "sha256:" + strings.Repeat("b", 64)
	err := executeOnlineUpdate("job-wrong-signature", manifest)
	if err == nil || !strings.Contains(err.Error(), "签名与 SHA256 不一致") {
		t.Fatalf("wrong signature was applied: %v", err)
	}
}

// TestExecuteOnlineUpdateVerifiesReleaseSignatureBeforeExtract 覆盖 1.8.6 起的强制验签：
// 正常签名包能走到解压之后；没签名、别的私钥签的、签完又改过的包都在解压前被拒，下载的包也删掉。
func TestExecuteOnlineUpdateVerifiesReleaseSignatureBeforeExtract(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	onlineUpdatePublicKeyForTest = pub
	t.Cleanup(func() { onlineUpdatePublicKeyForTest = nil })

	build := func(t *testing.T, key ed25519.PrivateKey, tamper bool) string {
		return buildSignedOnlineUpdatePackage(t, key, "1.0.1", "client", tamper)
	}
	apply := func(t *testing.T, packagePath string) error {
		t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
		t.Setenv("AUTO_PRO_FRONTEND_DIR", filepath.Join(t.TempDir(), "missing-frontend"))
		body, err := os.ReadFile(packagePath)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		manifest := signedOnlineUpdateManifest(hex.EncodeToString(sum[:]), int64(len(body)))
		manifest.Actions.BackupDatabase = false
		restore := stubOnlineUpdateApply(t, func(string, *onlineUpdateManifest) (string, error) {
			return packagePath, nil
		})
		defer restore()
		// 这里只验签名。目录权限预检另有测试，前端目录故意不存在，先跳过预检。
		previousPreflight := onlineUpdatePreflight
		onlineUpdatePreflight = func() error { return nil }
		defer func() { onlineUpdatePreflight = previousPreflight }()
		return executeOnlineUpdate("job-signature", manifest)
	}

	t.Run("正常签名包通过验签并解压", func(t *testing.T) {
		err := apply(t, build(t, priv, false))
		if err == nil || !strings.Contains(err.Error(), "前端目录") {
			t.Fatalf("signed package should pass signature and extraction, got %v", err)
		}
	})
	cases := []struct {
		name    string
		key     ed25519.PrivateKey
		edition string
		tamper  bool
		want    string
	}{
		{"没签名", nil, "client", false, "没有官方签名"},
		{"别的私钥签名", otherPriv, "client", false, "没有通过官方签名校验"},
		{"签完又改过", priv, "client", true, "没有通过官方签名校验"},
		{"没有包内发布信息", priv, "", false, "旧格式的包"},
		{"官网的包", priv, "official", false, "这是官网的更新包，客户站不能用"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packagePath := buildSignedOnlineUpdatePackage(t, tc.key, "1.0.1", tc.edition, tc.tamper)
			err := apply(t, packagePath)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "网站没有任何改动") {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if strings.Contains(strings.ToLower(err.Error()), "github") {
				t.Fatalf("customer-facing error mentions the code host: %v", err)
			}
			if _, statErr := os.Stat(packagePath); !os.IsNotExist(statErr) {
				t.Fatalf("rejected package should be removed: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(config.GetUpdateDir(), "job-signature", "staging")); !os.IsNotExist(statErr) {
				t.Fatalf("rejected package must not be extracted: %v", statErr)
			}
		})
	}
}

func TestExtractOnlineUpdatePackageRejectsTraversalAndSymlink(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	outside := filepath.Join(dataDir, "outside.txt")

	t.Run("parent path", func(t *testing.T) {
		packagePath := writeOnlineUpdateTar(t, tarEntry{name: "../outside.txt", body: "escaped"})
		if _, err := extractOnlineUpdatePackage("slip-parent", packagePath); err == nil {
			t.Fatal("parent path was extracted")
		}
		if _, err := os.Stat(outside); !os.IsNotExist(err) {
			t.Fatalf("escaped file exists: %v", err)
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		packagePath := writeOnlineUpdateTar(t, tarEntry{name: "/tmp/auth-pro-update-escape.txt", body: "escaped"})
		if _, err := extractOnlineUpdatePackage("slip-absolute", packagePath); err == nil {
			t.Fatal("absolute path was extracted")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		packagePath := writeOnlineUpdateTar(t, tarEntry{name: "link", body: "../outside.txt", typeflag: tar.TypeSymlink})
		if _, err := extractOnlineUpdatePackage("slip-symlink", packagePath); err == nil {
			t.Fatal("symlink was extracted")
		}
		linkPath := filepath.Join(config.GetUpdateDir(), "slip-symlink", "staging", "link")
		if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
			t.Fatalf("symlink was created: %v", err)
		}
	})
}

func TestOnlineUpdateFrontendCopyFailurePreservesLiveTree(t *testing.T) {
	shell, err := exec.LookPath("/bin/sh")
	if err != nil {
		t.Skip("/bin/sh is not available")
	}
	root := t.TempDir()
	liveDir := filepath.Join(root, "site")
	sourceDir := filepath.Join(root, "incoming")
	dataDir := filepath.Join(root, "data")
	writeFrontendTree(t, liveDir, "old-index", "old-asset")
	writeFrontendTree(t, sourceDir, "new-index", "new-asset")
	if err := os.WriteFile(filepath.Join(sourceDir, "assets", "extra.js"), []byte("extra"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv("AUTO_PRO_SERVICE_NAME", "auth_pro_test")

	scriptPath := writeFrontendSwitchScript(t, sourceDir, liveDir, "1.2.3")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	requireFrontendOnlyGuard(t, string(script))

	wrapperDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(wrapperDir, 0755); err != nil {
		t.Fatal(err)
	}
	wrapper := `#!/bin/sh
live=$ONLINE_UPDATE_LIVE_DIR
dest=
for arg in "$@"; do
  case "$arg" in
    -*) ;;
    *) dest=$arg ;;
  esac
done
case "$dest" in
  "$live"|"$live"/*)
    /bin/cp "$@"
    echo "copy into live frontend failed" >&2
    exit 1
    ;;
  "$live".staging.*|*/updates/frontend-staging.*)
    /bin/cp "$@"
    echo "staging copy failed" >&2
    exit 1
    ;;
esac
exec /bin/cp "$@"
`
	if err := os.WriteFile(filepath.Join(wrapperDir, "cp"), []byte(wrapper), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(shell, scriptPath, "--frontend-only")
	cmd.Env = append(os.Environ(),
		"PATH="+wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ONLINE_UPDATE_LIVE_DIR="+liveDir,
	)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("copy failure was treated as success\n%s", output)
	}
	assertFrontendTree(t, liveDir, "old-index", "old-asset")
	if _, err := os.Stat(filepath.Join(liveDir, "assets", "extra.js")); !os.IsNotExist(err) {
		t.Fatalf("new asset leaked into the live tree: %v", err)
	}
	if info, err := os.Lstat(liveDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("live frontend is no longer a directory: %v", err)
	}
	for _, pattern := range []string{liveDir + ".backup.*", liveDir + ".staging.*", filepath.Join(dataDir, "updates", "frontend-staging.*"), filepath.Join(dataDir, "updates", "backups", "rename", "*")} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("failed copy left %v", matches)
		}
	}
}

func TestOnlineUpdateFrontendRenameSwitch(t *testing.T) {
	shell, err := exec.LookPath("/bin/sh")
	if err != nil {
		t.Skip("/bin/sh is not available")
	}
	root := t.TempDir()
	liveDir := filepath.Join(root, "site")
	sourceDir := filepath.Join(root, "incoming")
	dataDir := filepath.Join(root, "data")
	writeFrontendTree(t, liveDir, "old-index", "old-asset")
	writeFrontendTree(t, sourceDir, "new-index", "new-asset")
	if err := os.WriteFile(filepath.Join(sourceDir, "assets", "extra.js"), []byte("extra"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv("AUTO_PRO_SERVICE_NAME", "auth_pro_test")

	scriptPath := writeFrontendSwitchScript(t, sourceDir, liveDir, "1.2.3")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	requireFrontendOnlyGuard(t, string(script))
	if output, err := exec.Command(shell, scriptPath, "--frontend-only").CombinedOutput(); err != nil {
		t.Fatalf("frontend switch failed: %v\n%s", err, output)
	}

	assertFrontendTree(t, liveDir, "new-index", "new-asset")
	extra, err := os.ReadFile(filepath.Join(liveDir, "assets", "extra.js"))
	if err != nil || string(extra) != "extra" {
		t.Fatalf("switched frontend missing new asset: %q %v", extra, err)
	}
	// 没有 /www/backup 时备份放在数据目录，不再放到网站根旁边。
	if sideways, _ := filepath.Glob(liveDir + ".backup.*"); len(sideways) != 0 {
		t.Fatalf("backup was placed next to the site root: %v", sideways)
	}
	backups, err := filepath.Glob(filepath.Join(dataDir, "updates", "backups", "rename", "site.backup.*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("previous frontend backup = %v, %v", backups, err)
	}
	assertFrontendTree(t, backups[0], "old-index", "old-asset")
	if _, err := os.Stat(filepath.Join(backups[0], "assets", "extra.js")); !os.IsNotExist(err) {
		t.Fatalf("previous frontend contains the new asset: %v", err)
	}
}

func TestOnlineUpdateSymlinkReleaseSwitch(t *testing.T) {
	shell, err := exec.LookPath("/bin/sh")
	if err != nil {
		t.Skip("/bin/sh is not available")
	}
	root := t.TempDir()
	frontRoot := filepath.Join(root, "frontend")
	previous := filepath.Join(frontRoot, "releases", "1.0.0")
	current := filepath.Join(frontRoot, "current")
	sourceDir := filepath.Join(root, "incoming")
	dataDir := filepath.Join(root, "data")
	writeFrontendTree(t, previous, "old-index", "old-asset")
	if err := os.Symlink(previous, current); err != nil {
		t.Fatal(err)
	}
	writeFrontendTree(t, sourceDir, "new-index", "new-asset")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv("AUTO_PRO_SERVICE_NAME", "auth_pro_test")

	scriptPath := writeFrontendSwitchScript(t, sourceDir, current, "1.2.3")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	requireFrontendOnlyGuard(t, string(script))
	if output, err := exec.Command(shell, scriptPath, "--frontend-only").CombinedOutput(); err != nil {
		t.Fatalf("symlink switch failed: %v\n%s", err, output)
	}

	info, err := os.Lstat(current)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("current is no longer a symlink: %v", err)
	}
	assertFrontendTree(t, current, "new-index", "new-asset")
	assertFrontendTree(t, previous, "old-index", "old-asset")
	target, err := os.Readlink(current)
	if err != nil || !strings.Contains(target, "1.2.3") {
		t.Fatalf("current target = %q, %v", target, err)
	}
}

// buildSignedOnlineUpdatePackage 造一个和正式发布同结构的整包。edition 为空时不写包内发布信息（模拟旧包），
// key 为 nil 时不签名，tamper 为真时签完再改后端文件。
func buildSignedOnlineUpdatePackage(t *testing.T, key ed25519.PrivateKey, version, edition string, tamper bool) string {
	t.Helper()
	dir := t.TempDir()
	writeFrontendTree(t, dir, "<html>v"+version+"</html>", "console.log(1)")
	if err := os.WriteFile(filepath.Join(dir, "version.json"), []byte(`{"version":"`+version+`"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "auth_pro"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if edition != "" {
		info := `{"version":"` + version + `","edition":"` + edition + `","channel":"stable","minVersion":"1.0.0","releasedAt":"2026-10-03T00:00:00Z","notes":["修复问题"]}`
		if err := os.WriteFile(filepath.Join(dir, "backend", "release.json"), []byte(info), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := updatesign.WriteManifest(dir, version, key); err != nil {
		t.Fatal(err)
	}
	if tamper {
		if err := os.WriteFile(filepath.Join(dir, "backend", "auth_pro"), []byte("evil"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	packagePath := filepath.Join(t.TempDir(), "auth_pro-full-v"+version+".tar.gz")
	if out, err := exec.Command("tar", "-czf", packagePath, "-C", dir, ".").CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
	return packagePath
}

func signedOnlineUpdateManifest(hexSum string, size int64) *onlineUpdateManifest {
	manifest := validOnlineUpdateManifestForTest()
	manifest.Package.SHA256 = hexSum
	manifest.Package.Signature = "sha256:" + hexSum
	manifest.Package.Size = size
	return manifest
}

func stubOnlineUpdateApply(t *testing.T, download func(string, *onlineUpdateManifest) (string, error)) func() {
	t.Helper()
	previousDownload := downloadOnlineUpdatePackageForApply
	downloadOnlineUpdatePackageForApply = download
	return func() {
		downloadOnlineUpdatePackageForApply = previousDownload
	}
}

type tarEntry struct {
	name     string
	body     string
	typeflag byte
}

func writeOnlineUpdateTar(t *testing.T, entry tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{Name: entry.name, Mode: 0644, Size: int64(len(entry.body))}
	if entry.typeflag == 0 {
		header.Typeflag = tar.TypeReg
	} else {
		header.Typeflag = entry.typeflag
	}
	if header.Typeflag == tar.TypeSymlink {
		header.Linkname = entry.body
		header.Size = 0
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Typeflag == tar.TypeReg {
		if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(t.TempDir(), "update.tar.gz")
	if err := os.WriteFile(packagePath, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return packagePath
}

func writeFrontendTree(t *testing.T, dir, indexBody, assetBody string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexBody), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte(assetBody), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertFrontendTree(t *testing.T, dir, indexBody, assetBody string) {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil || string(index) != indexBody {
		t.Fatalf("index.html = %q, %v", index, err)
	}
	asset, err := os.ReadFile(filepath.Join(dir, "assets", "app.js"))
	if err != nil || string(asset) != assetBody {
		t.Fatalf("assets/app.js = %q, %v", asset, err)
	}
}

func TestOnlineUpdateJobFailureReasonSurvivesRestart(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	job := &onlineUpdateJob{
		ID:        "U-test-fail-reason",
		Status:    "restarting",
		Message:   "服务正在切换并重启",
		Progress:  95,
		Version:   "1.0.1",
		Logs:      []string{"开始更新"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	persistOnlineUpdateJob(job)
	reason := "新版本在 12 秒内没有健康启动，已尝试回滚到 1.5.3"
	if err := os.WriteFile(onlineUpdateJobStatePath(job.ID)+".result", []byte("failed\n"+reason+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := loadOnlineUpdateJob(job.ID)
	if loaded == nil || loaded.Status != "failed" || loaded.Error != reason || loaded.Progress != 95 {
		t.Fatalf("unexpected failed job: %#v", loaded)
	}
	loadedAgain := loadOnlineUpdateJob(job.ID)
	if loadedAgain == nil || len(loadedAgain.Logs) != 2 || loadedAgain.Error != reason {
		t.Fatalf("failure reconciliation was not idempotent: %#v", loadedAgain)
	}
}

func TestResolveProcessManagerPrefersExplicitMode(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	if err := os.WriteFile(filepath.Join(dataDir, processManagerFileName), []byte("supervisor\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_PROCESS_MANAGER", "none")
	if got := resolveOnlineUpdateProcessManager(); got != processManagerNone {
		t.Fatalf("explicit none = %s", got)
	}
	t.Setenv("AUTO_PRO_PROCESS_MANAGER", "")
	if got := resolveOnlineUpdateProcessManager(); got != processManagerSupervisor {
		t.Fatalf("marker file = %s", got)
	}
}

func TestSupervisedUpdateScriptDoesNotSelfSpawn(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "backend")
	frontendSource := filepath.Join(root, "staging-frontend")
	stagingDir := filepath.Join(root, "staging")
	for _, dir := range []string{dataDir, frontendSource, filepath.Join(stagingDir, "backend")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontendSource, "index.html"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "backend", "auth_pro"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	t.Setenv("AUTO_PRO_PROCESS_MANAGER", "supervisor")
	t.Setenv("AUTO_PRO_SUPERVISOR_PROGRAM", "auth_pro")
	scriptPath, err := writeOnlineUpdateScript(
		"U-supervisor-script",
		stagingDir,
		&extractedOnlineUpdateManifest{BackendFile: "backend/auth_pro"},
		frontendSource,
		"1.0.1",
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	if !strings.Contains(text, `PROCESS_MANAGER='supervisor'`) {
		t.Fatal("supervisor mode was not baked into the script")
	}
	handoffStart := strings.Index(text, "# BEGIN GUARDIAN HANDOFF")
	handoffEnd := strings.Index(text, "# END GUARDIAN HANDOFF")
	if handoffStart < 0 || handoffEnd < 0 || handoffStart > handoffEnd {
		t.Fatal("guardian handoff section is missing")
	}
	section := text[handoffStart:handoffEnd]
	if strings.Contains(section, "supervisorctl") || strings.Contains(section, "start_standalone") || strings.Contains(section, `nohup "$APP_BIN"`) {
		t.Fatal("guardian handoff must not stop the supervisor or start its own backend")
	}
	if !strings.Contains(section, `guardian owns pid`) || !strings.Contains(section, "write_pending_handoff") || !strings.Contains(section, `stop_pid "$APP_PID"`) {
		t.Fatal("guardian handoff must replace files, write the pending marker, then signal only this process")
	}
	replaceAt := strings.Index(section, `mv -f "$APP_STAGE" "$APP_BIN"`)
	signalAt := strings.Index(section, `stop_pid "$APP_PID"`)
	if replaceAt < 0 || signalAt < 0 || replaceAt > signalAt {
		t.Fatal("guardian handoff must replace the binary before signaling the current process")
	}
	encoded := text[strings.Index(text, "GUARDIAN_START_B64='")+len("GUARDIAN_START_B64='"):]
	encoded = encoded[:strings.Index(encoded, "'")]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("guardian start script is not valid base64: %v", err)
	}
	if !strings.Contains(string(decoded), "auth-pro-guardian-start") || !strings.Contains(string(decoded), "pending-restart/handoff.sh") {
		t.Fatal("generated script does not embed the guardian start script")
	}
}

func writeFrontendSwitchScript(t *testing.T, sourceDir, liveDir, version string) string {
	t.Helper()
	stagingDir := filepath.Join(filepath.Dir(sourceDir), "package")
	if err := os.MkdirAll(filepath.Join(stagingDir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "backend", "auth_pro"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath, err := writeOnlineUpdateScript(
		"U-frontend-"+version,
		stagingDir,
		&extractedOnlineUpdateManifest{BackendFile: "backend/auth_pro"},
		sourceDir,
		version,
		liveDir,
	)
	if err != nil {
		t.Fatal(err)
	}
	return scriptPath
}

func requireFrontendOnlyGuard(t *testing.T, script string) {
	t.Helper()
	guard := strings.Index(script, `if [ "$FRONTEND_ONLY" = "1" ]; then`)
	kill := strings.Index(script, `if ! stop_old_process; then`)
	if guard < 0 || kill < 0 || guard > kill {
		t.Fatal("generated script must finish frontend-only before stopping the process")
	}
}

// 停后端前把静态说明页切到「正在更新」，结束时清回 0；守护交接文件里也带同一个函数。
func TestUpdateScriptMarksUnavailablePageWhileRestarting(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("没有 sh")
	}
	root := t.TempDir()
	page, err := os.ReadFile(filepath.Join("..", "..", "frontend", "public", "backend-unavailable.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `data-update-since="0"`) {
		t.Fatal("静态说明页缺少 data-update-since 标记")
	}
	live := filepath.Join(root, "site")
	source := filepath.Join(root, "new")
	for _, dir := range []string{live, source} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	pagePath := filepath.Join(live, "backend-unavailable.html")
	if err := os.WriteFile(pagePath, page, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_PRO_DATA_DIR", filepath.Join(root, "data"))
	scriptPath := writeFrontendSwitchScript(t, source, live, "1.0.1")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	guard := strings.Index(text, `if [ "$FRONTEND_ONLY" = "1" ]; then`)
	mark := strings.Index(text, `set_update_marker "$(date +%s)"`)
	stop := strings.Index(text, `if ! stop_old_process; then`)
	if guard < 0 || mark < guard || stop < mark {
		t.Fatal("标记要在只换前端的分支之后、停旧进程之前写上")
	}
	if !strings.Contains(text, `sed -n '/^# AUTH_PRO_PAGE_FUNCS_BEGIN$/,/^# AUTH_PRO_PAGE_FUNCS_END$/p' "$0" >> "$pending/handoff.sh"`) {
		t.Fatal("守护交接文件要带上清除标记的函数")
	}
	begin := strings.Index(text, "# AUTH_PRO_PAGE_FUNCS_BEGIN")
	end := strings.Index(text, "# AUTH_PRO_PAGE_FUNCS_END")
	funcs := text[begin:end]
	run := func(value string) string {
		body := "FRONTEND_CURRENT='" + live + "'\n" + funcs + "\nset_update_marker " + value + "\n"
		if output, err := exec.Command(shell, "-c", body).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, output)
		}
		current, err := os.ReadFile(pagePath)
		if err != nil {
			t.Fatal(err)
		}
		return string(current)
	}
	if got := run("1767225600"); !strings.Contains(got, `data-update-since="1767225600"`) || strings.Count(got, "data-update-since=\"") != 1 {
		t.Fatal("没有写上更新标记")
	}
	if got := run("0"); got != string(page) {
		t.Fatal("清除标记后页面应恢复原样")
	}
}
