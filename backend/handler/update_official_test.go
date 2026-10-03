package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// 仓库改私有后：存储管理里的令牌能读就用它，记下“用的是哪个存储令牌”；读不了只记原因，不再有单独的令牌入口。
func TestOfficialUpdateUsesStorageTokenAfterPrivate(t *testing.T) {
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

	const good = "github_pat_storage_0123456789abcdef"
	sum := strings.Repeat("ef", 32)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟私有仓库：不带正确令牌一律 404。
		if r.Header.Get("Authorization") != "Bearer "+good {
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

	// 没有任何令牌：只剩匿名，私有仓库读不到，记下原因（页面用中性颜色显示，不报红）。
	if _, err := fetchOnlineUpdateManifest(); err == nil {
		t.Fatal("没有令牌时私有仓库不应能读到")
	}
	if view := currentOfficialUpdateSource(); view == nil || view.LastCheck == nil || view.LastCheck.OK || view.LastCheck.Source != "" || view.LastCheck.Reason == "" {
		t.Fatalf("应记下匿名读取失败: %+v", view)
	}

	sealed, err := sealStorageSecret(good)
	if err != nil {
		t.Fatal(err)
	}
	loc := storageLocation{ID: newStorageID(), Name: "安装包仓库", Kind: packageStorageGitHub, Role: storageRolePrimary, Owner: "acme", Repo: "packages", SecretSealed: sealed}
	if err := saveStorageBlob(storageConfigBlob{Locations: []storageLocation{loc}}); err != nil {
		t.Fatal(err)
	}
	manifest, err := fetchOnlineUpdateManifest()
	if err != nil || manifest.Version != "1.8.6" {
		t.Fatalf("存储令牌应能读到私有仓库: %+v err=%v", manifest, err)
	}
	view := currentOfficialUpdateSource()
	if view.LastCheck == nil || !view.LastCheck.OK || view.LastCheck.Source != "存储「安装包仓库」的令牌" || view.Repository != "maizll/auth-pro" {
		t.Fatalf("来源信息不对: %+v %+v", view, view.LastCheck)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), good) {
		t.Fatal("来源信息不应带出令牌")
	}
}

// 老站升级：1.8.5–1.8.7 单独保存的令牌文件和密钥一并删掉，只删一次。
func TestRemoveLegacyOfficialUpdateToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dir)
	t.Cleanup(SetNotificationStoreForTest(newMemoryNotificationStore()))
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{legacyOfficialUpdateTokenFile, legacyOfficialUpdateTokenKeyFile, "storage-locations.key"} {
		if err := os.WriteFile(filepath.Join(store, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !RemoveLegacyOfficialUpdateToken() {
		t.Fatal("有旧令牌文件时应删除并返回 true")
	}
	for _, name := range []string{legacyOfficialUpdateTokenFile, legacyOfficialUpdateTokenKeyFile} {
		if _, err := os.Stat(filepath.Join(store, name)); !os.IsNotExist(err) {
			t.Fatalf("%s 应已删除", name)
		}
	}
	if _, err := os.Stat(filepath.Join(store, "storage-locations.key")); err != nil {
		t.Fatal("不能误删存储管理的密钥")
	}
	if RemoveLegacyOfficialUpdateToken() {
		t.Fatal("第二次启动不应再删、再通知")
	}
}
