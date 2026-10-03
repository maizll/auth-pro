package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOfficialUpdateUsesAuthProNotClientRepo(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previous })
	writeSnapshotKey(t, privateKey)
	if !officialSite() {
		t.Fatal("fixture should be the official site")
	}

	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	t.Setenv(officialUpdateRepoEnv, "")
	t.Setenv(productUpdateRepoEnv, "")
	restore := SetSourceStationStoreForTest(newMemorySourceStore())
	t.Cleanup(restore)

	owner, repo, err := officialUpdateRepository()
	if err != nil || owner+"/"+repo != officialUpdateDefaultRepository {
		t.Fatalf("official repo = %s/%s err=%v", owner, repo, err)
	}
	if _, _, err := productUpdateRepository(); err == nil {
		t.Fatal("import no longer falls back to a hardcoded client repository")
	}

	sum := strings.Repeat("ab", 32)
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/assets/") {
			_, _ = w.Write([]byte(`{
				"version":"1.7.1",
				"channel":"stable",
				"releasesUrl":"https://github.com/maizll/auth-pro/releases/download/v1.7.1/releases.json",
				"package":{
					"os":"linux","arch":"amd64",
					"fileName":"auth_pro-full-v1.7.1.tar.gz",
					"url":"https://github.com/maizll/auth-pro/releases/download/v1.7.1/auth_pro-full-v1.7.1.tar.gz",
					"sha256":"` + sum + `",
					"size":32,
					"signature":"sha256:` + sum + `"
				},
				"notes":["从官网更新","详见 https://github.com/maizll/auth-pro"]
			}`))
			return
		}
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"assets":[{"id":4,"name":"latest.json","url":"` + strings.TrimRight(productUpdateGitHubAPI, "/") + `/repos/maizll/auth-pro/releases/assets/4"}]}`))
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL

	manifest, err := fetchOnlineUpdateManifest()
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/repos/maizll/auth-pro/releases/latest" {
		t.Fatalf("path = %s", gotPath)
	}
	if manifest.Package.URL != "" || manifest.ReleasesURL != "" || strings.Contains(manifest.Package.URL, "github.com") {
		t.Fatalf("official manifest kept a download URL: %+v", manifest.Package)
	}
	for _, note := range manifest.Notes {
		if strings.Contains(strings.ToLower(note), "github.com") {
			t.Fatalf("note leaked a repository URL: %s", note)
		}
	}
	if len(manifest.Notes) != 1 || manifest.Notes[0] != "从官网更新" {
		t.Fatalf("notes = %#v", manifest.Notes)
	}
}

func TestLegacy170AcceptsAuthProReleaseManifest(t *testing.T) {
	sum := strings.Repeat("cd", 32)
	raw := `{
		"version":"1.7.1",
		"channel":"stable",
		"minVersion":"0.0.0",
		"force":false,
		"releasedAt":"2026-09-27T00:00:00Z",
		"releasesUrl":"https://github.com/maizll/auth-pro/releases/download/v1.7.1/releases.json",
		"package":{
			"os":"linux",
			"arch":"amd64",
			"fileName":"auth_pro-full-v1.7.1.tar.gz",
			"url":"https://github.com/maizll/auth-pro/releases/download/v1.7.1/auth_pro-full-v1.7.1.tar.gz",
			"sha256":"` + sum + `",
			"size":128,
			"signature":"sha256:` + sum + `"
		},
		"actions":{"updateFrontend":true,"updateBackend":true,"restartBackend":true,"backupDatabase":true},
		"notes":["官网以外的站点只从官网更新"]
	}`
	var manifest onlineUpdateManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatal(err)
	}
	normalizeOnlineUpdateManifest(&manifest)
	if err := manifestAcceptedBy170(manifest); err != nil {
		t.Fatal(err)
	}
	// 装上 1.7.1 之后，客户站只认官网，不再接受这份托管站地址。
	if _, err := parseOnlineUpdateURL(manifest.Package.URL); err == nil {
		t.Fatal("upgraded client accepted the old release URL")
	}
}

// manifestAcceptedBy170 复述 1.7.0 安装更新包时对清单的要求：
// 地址在 maizll/auth-pro 的 Release 下，signature 等于 sha256: 加包的哈希，文件名带版本号。
func manifestAcceptedBy170(manifest onlineUpdateManifest) error {
	if _, ok := parseOnlineUpdateVersion(manifest.Version); !ok {
		return errProductUpdateUnavailable
	}
	if !strings.HasPrefix(manifest.Package.URL, "https://github.com/maizll/auth-pro/releases/") {
		return errProductUpdateUnavailable
	}
	if !isHexSHA256(manifest.Package.SHA256) {
		return errProductUpdateUnavailable
	}
	if manifest.Package.Signature != "sha256:"+manifest.Package.SHA256 {
		return errProductUpdateUnavailable
	}
	if manifest.Package.FileName != "auth_pro-full-v"+manifest.Version+".tar.gz" {
		return errProductUpdateUnavailable
	}
	if manifest.Package.Size <= 0 {
		return errProductUpdateUnavailable
	}
	return nil
}

// 保存了官网专用只读令牌后只用它：仓库改私有（匿名 404）后照样能读；令牌失效时直接报原因，不退回匿名。
func TestOfficialUpdateDedicatedTokenAfterPrivate(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previous })
	writeSnapshotKey(t, privateKey)
	resetProductUpdateStateForTest()
	t.Cleanup(resetProductUpdateStateForTest)
	t.Setenv(officialUpdateRepoEnv, "")
	t.Cleanup(SetSourceStationStoreForTest(newMemorySourceStore()))

	const good = "github_pat_readonly_0123456789abcdef"
	sum := strings.Repeat("ef", 32)
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		// 模拟私有仓库：不带正确令牌一律 404。
		if auth != "Bearer "+good {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/releases/assets/") {
			_, _ = w.Write([]byte(`{"version":"1.8.6","channel":"stable","package":{"os":"linux","arch":"amd64","fileName":"auth_pro-full-v1.8.6.tar.gz","sha256":"` + sum + `","size":32,"signature":"sha256:` + sum + `"},"notes":["一条说明"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"assets":[{"id":7,"name":"latest.json","url":"` + "http://" + r.Host + `/repos/maizll/auth-pro/releases/assets/7"}]}`))
	}))
	defer upstream.Close()
	productUpdateGitHubAPI = upstream.URL

	// 没保存令牌：沿用原来的顺序，最后匿名，私有仓库读不到。
	if _, err := fetchOnlineUpdateManifest(); err == nil {
		t.Fatal("没有令牌时私有仓库不应能读到")
	}
	if view := currentOfficialUpdateSource(); view == nil || view.TokenSaved || view.LastCheck == nil || view.LastCheck.Credential != officialCredentialAnonymous || view.LastCheck.OK {
		t.Fatalf("应记下匿名读取失败: %+v", view)
	}

	if err := saveOfficialUpdateToken("bad token!"); err == nil {
		t.Fatal("格式不对的令牌应被拒绝")
	}
	if err := saveOfficialUpdateToken(good); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(officialUpdateTokenPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("令牌文件权限应为 0600: %v %v", info, err)
	}
	if raw, _ := os.ReadFile(officialUpdateTokenPath()); strings.Contains(string(raw), good) {
		t.Fatal("令牌不应明文落盘")
	}
	seen = nil
	manifest, err := fetchOnlineUpdateManifest()
	if err != nil || manifest.Version != "1.8.6" {
		t.Fatalf("专用令牌应能读到私有仓库: %+v err=%v", manifest, err)
	}
	for _, auth := range seen {
		if auth != "Bearer "+good {
			t.Fatalf("保存了专用令牌后不应再试别的凭据: %q", auth)
		}
	}
	view := currentOfficialUpdateSource()
	if !view.TokenSaved || view.TokenHint != "cdef" || view.LastCheck.Credential != officialCredentialToken || !view.LastCheck.OK {
		t.Fatalf("来源信息不对: %+v %+v", view, view.LastCheck)
	}

	// 测试读取：令牌能读，匿名读不到（说明仓库已私有）。
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/system/update/official-token/test", nil)
	AdminOfficialUpdateSourceTest(ctx)
	if body := recorder.Body.String(); !strings.Contains(body, `"tokenOk":true`) || !strings.Contains(body, `"anonymousReadable":false`) || !strings.Contains(body, "1.8.6") {
		t.Fatalf("测试读取结果不对: %s", body)
	}

	// 令牌失效：报出原因，不退回匿名。
	if err := saveOfficialUpdateToken("github_pat_expired_000000000000000"); err != nil {
		t.Fatal(err)
	}
	if _, err := fetchOnlineUpdateManifest(); err == nil || !strings.Contains(err.Error(), "官网更新令牌读取失败") {
		t.Fatalf("令牌失效应直接报原因: %v", err)
	}
	if err := saveOfficialUpdateToken(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(officialUpdateTokenPath()); !os.IsNotExist(err) {
		t.Fatal("清空后令牌文件应删除")
	}
}
