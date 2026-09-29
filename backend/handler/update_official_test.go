package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	clientOwner, clientRepo, err := productUpdateRepository()
	if err != nil || clientOwner+"/"+clientRepo != productUpdateDefaultRepository {
		t.Fatalf("distribution repo = %s/%s err=%v", clientOwner, clientRepo, err)
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
