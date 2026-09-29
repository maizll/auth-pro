package handler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReleaseImportVersionAndChangelog(t *testing.T) {
	if got := releaseImportVersion("v1.7.2"); got != "1.7.2" {
		t.Fatalf("version=%s", got)
	}
	body := "修复登录。\n详见 https://github.com/maizll/auth-pro-client/releases/tag/v1.7.2\n## What's Changed\n* 自动说明 by @bot in https://github.com/acme/widgets/pull/1\n**Full Changelog**: https://github.com/acme/widgets/compare/v1.7.1...v1.7.2\n补上到期时间。"
	got := releaseImportChangelog(body)
	if got != "修复登录。\n补上到期时间。" {
		t.Fatalf("changelog=%q", got)
	}
	if strings.Contains(strings.ToLower(got), "github.com") || strings.Contains(got, "What's Changed") {
		t.Fatalf("template leaked: %q", got)
	}
}

func TestReleaseImportPrefersLatestNotes(t *testing.T) {
	notes := `{"version":"1.7.8","notes":["修复进程守护仍去下载旧的面板脚本。","启动前把 backend 交给网站运行用户。","证书失败时写明原因，并可单独申请。"]}`
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/widgets/releases" && r.URL.Query().Get("per_page") == "20":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"tag_name":"v1.7.8",
				"name":"v1.7.8",
				"body":"## What's Changed\n* something by @bot in https://github.com/acme/widgets/pull/9\n\n**Full Changelog**: https://github.com/acme/widgets/compare/v1.7.7...v1.7.8\n",
				"published_at":"2026-09-29T00:00:00Z",
				"draft":false,
				"assets":[
					{"name":"latest.json","size":120},
					{"name":"auth_pro-full-v1.7.8.tar.gz","size":99}
				]
			}]`))
		case r.URL.Path == "/repos/acme/widgets/releases/tags/v1.7.8":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"assets":[{"id":7,"name":"latest.json","url":"` + server.URL + `/asset/latest.json"},{"id":8,"name":"auth_pro-full-v1.7.8.tar.gz","url":"` + server.URL + `/asset/pkg"}]}`))
		case r.URL.Path == "/asset/latest.json":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(notes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := productUpdateGitHubAPI
	productUpdateGitHubAPI = server.URL
	t.Cleanup(func() { productUpdateGitHubAPI = previous })

	list, err := listReleaseImportReleases(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("releases=%d", len(list))
	}
	item := list[0]
	if item.AssetName != "auth_pro-full-v1.7.8.tar.gz" {
		t.Fatalf("asset=%s", item.AssetName)
	}
	if !strings.Contains(item.Title, "修复进程守护") {
		t.Fatalf("title=%q", item.Title)
	}
	if !strings.Contains(item.Changelog, "启动前把 backend") || !strings.Contains(item.Changelog, "证书失败时写明原因") {
		t.Fatalf("changelog=%q", item.Changelog)
	}
	if strings.Contains(strings.ToLower(item.Changelog), "github.com") || strings.Contains(item.Changelog, "What's Changed") || strings.Contains(item.Title, "What's Changed") {
		t.Fatalf("template leaked title=%q changelog=%q", item.Title, item.Changelog)
	}
}

// 服务端 v1.7.9 和客户端 v1.7.8 的 Release 正文都是自动生成的变更列表。
// 导入必须改读各自 latest.json 的 notes，表单用的就是拉取安装包时返回的标题和更新日志。
func TestReleaseImportFillsServerAndClientNotes(t *testing.T) {
	serverOwner, serverRepo, err := releaseImportRepo(releaseImportActorSite, "app", "maizll/auth-pro")
	if err != nil || serverOwner != "maizll" || serverRepo != "auth-pro" {
		t.Fatalf("server repo owner=%s repo=%s err=%v", serverOwner, serverRepo, err)
	}
	clientOwner, clientRepo, err := releaseImportRepo(releaseImportActorSite, "app", "maizll/auth-pro-client")
	if err != nil || clientOwner != "maizll" || clientRepo != "auth-pro-client" {
		t.Fatalf("client repo owner=%s repo=%s err=%v", clientOwner, clientRepo, err)
	}

	const serverBody = "## What's Changed\n* 1.7.9：修复重设管理员密码和站点列表 by @maizll in https://github.com/maizll/auth-pro/pull/117\n\n**Full Changelog**: https://github.com/maizll/auth-pro/compare/v1.7.8...v1.7.9\n"
	const clientBody = "## What's Changed\n* 1.7.8：安装脚本交互菜单 by @maizll in https://github.com/maizll/auth-pro-client/pull/4\n\n**Full Changelog**: https://github.com/maizll/auth-pro-client/compare/v1.7.7...v1.7.8\n"
	const serverNotes = `{"version":"1.7.9","notes":["auth-pro 1.7.9","- 重设管理员密码优先调用站点已装的程序，旧版程序改为直接更新数据库，无需先升级站点。","- 站点列表只显示已安装的 auth-pro 站点，不再列出 .bak 等备份目录。"]}`
	const clientNotes = `{"version":"1.7.8","notes":["auth-pro 1.7.8","- 不带参数运行 install.sh 时显示编号菜单，站点类操作会列出本机已装站点供选择。","- 新增启动、停止、重启，备份和恢复数据，以及修改后台端口。","- 新增卸载站点，删除前自动备份网站目录和数据库，数据库默认保留。"]}`
	const serverChangelog = "- 重设管理员密码优先调用站点已装的程序，旧版程序改为直接更新数据库，无需先升级站点。\n- 站点列表只显示已安装的 auth-pro 站点，不再列出 .bak 等备份目录。"
	const clientChangelog = "- 不带参数运行 install.sh 时显示编号菜单，站点类操作会列出本机已装站点供选择。\n- 新增启动、停止、重启，备份和恢复数据，以及修改后台端口。\n- 新增卸载站点，删除前自动备份网站目录和数据库，数据库默认保留。"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/maizll/auth-pro/releases":
			writeReleaseList(w, "v1.7.9", serverBody)
		case "/repos/maizll/auth-pro/releases/tags/v1.7.9":
			writeReleaseAssets(w, server.URL+"/asset/server-latest.json", server.URL+"/asset/server-pkg", "auth_pro-full-v1.7.9.tar.gz")
		case "/asset/server-latest.json":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(serverNotes))
		case "/asset/server-pkg":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("server-package"))
		case "/repos/maizll/auth-pro-client/releases":
			writeReleaseList(w, "v1.7.8", clientBody)
		case "/repos/maizll/auth-pro-client/releases/tags/v1.7.8":
			writeReleaseAssets(w, server.URL+"/asset/client-latest.json", server.URL+"/asset/client-pkg", "auth_pro-full-v1.7.8.tar.gz")
		case "/asset/client-latest.json":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(clientNotes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := productUpdateGitHubAPI
	productUpdateGitHubAPI = server.URL
	t.Cleanup(func() { productUpdateGitHubAPI = previous })
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	serverList, err := listReleaseImportReleases(context.Background(), "maizll", "auth-pro")
	if err != nil {
		t.Fatal(err)
	}
	if len(serverList) != 1 || serverList[0].Title != "auth-pro 1.7.9" || serverList[0].Changelog != serverChangelog {
		t.Fatalf("server list title=%q changelog=%q", serverList[0].Title, serverList[0].Changelog)
	}
	clientList, err := listReleaseImportReleases(context.Background(), "maizll", "auth-pro-client")
	if err != nil {
		t.Fatal(err)
	}
	if len(clientList) != 1 || clientList[0].Title != "auth-pro 1.7.8" || clientList[0].Changelog != clientChangelog {
		t.Fatalf("client list title=%q changelog=%q", clientList[0].Title, clientList[0].Changelog)
	}

	filled, err := fetchReleaseImportAsset(context.Background(), "maizll", "auth-pro", "v1.7.9", "auth_pro-full-v1.7.9.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if filled.Version != "1.7.9" || filled.Title != "auth-pro 1.7.9" || filled.Changelog != serverChangelog {
		t.Fatalf("server form version=%q title=%q changelog=%q", filled.Version, filled.Title, filled.Changelog)
	}
	if strings.Contains(strings.ToLower(filled.Title+"\n"+filled.Changelog), "github.com") || strings.Contains(filled.Changelog, "What's Changed") {
		t.Fatalf("server form leaked title=%q changelog=%q", filled.Title, filled.Changelog)
	}
}

func writeReleaseList(w http.ResponseWriter, tag, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`[{
		"tag_name":"` + tag + `",
		"name":"` + tag + `",
		"body":"` + jsonEscape(body) + `",
		"published_at":"2026-09-29T00:00:00Z",
		"draft":false,
		"assets":[
			{"name":"latest.json","size":120},
			{"name":"auth_pro-full-` + tag + `.tar.gz","size":99}
		]
	}]`))
}

func writeReleaseAssets(w http.ResponseWriter, latestURL, packageURL, packageName string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"assets":[{"id":7,"name":"latest.json","url":"` + latestURL + `"},{"id":8,"name":"` + packageName + `","url":"` + packageURL + `"}]}`))
}

func jsonEscape(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return replacer.Replace(value)
}

func TestReleaseImportRequiresRepoWithoutSavedValue(t *testing.T) {
	restore := SetSourceStationStoreForTest(newMemorySourceStore())
	t.Cleanup(restore)
	if _, _, err := releaseImportRepo(releaseImportActorSite, "app", ""); err == nil || !strings.Contains(err.Error(), "请填写仓库") {
		t.Fatalf("err=%v", err)
	}
	if _, _, err := releaseImportRepo(releaseImportActorDeveloper, "plugin", ""); err == nil || !strings.Contains(err.Error(), "请填写仓库") {
		t.Fatalf("developer err=%v", err)
	}
	if officialImportRepoSeed("app") != "" || officialImportRepoSeed("catalog") != "" {
		t.Fatal("non-official seed must be empty")
	}
}

func TestReleaseImportRemembersRepoByPurpose(t *testing.T) {
	restore := SetSourceStationStoreForTest(newMemorySourceStore())
	t.Cleanup(restore)
	rememberReleaseImportRepo(releaseImportActorSite, "app", "acme", "widgets")
	rememberReleaseImportRepo(releaseImportActorSite, "plugin", "acme", "paid-widgets")
	rememberReleaseImportRepo(releaseImportActorDeveloper, "template", "evil", "overwrite")
	appOwner, appRepo, err := releaseImportRepo(releaseImportActorSite, "app", "")
	if err != nil || appOwner != "acme" || appRepo != "widgets" {
		t.Fatalf("app %s/%s err=%v", appOwner, appRepo, err)
	}
	pluginOwner, pluginRepo, err := releaseImportRepo(releaseImportActorSite, "plugin", "")
	if err != nil || pluginOwner != "acme" || pluginRepo != "paid-widgets" {
		t.Fatalf("plugin %s/%s err=%v", pluginOwner, pluginRepo, err)
	}
	templateOwner, templateRepo, err := releaseImportRepo(releaseImportActorSite, "template", "")
	if err != nil || templateOwner != "acme" || templateRepo != "paid-widgets" {
		t.Fatalf("template %s/%s err=%v", templateOwner, templateRepo, err)
	}
	if _, _, err := releaseImportRepo(releaseImportActorDeveloper, "plugin", ""); err == nil {
		t.Fatal("developer should not reuse the site repository")
	}
}

func TestOfficialImportMigrationSeedsDefaults(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := embeddedStoreSnapshotPublicKey
	embeddedStoreSnapshotPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { embeddedStoreSnapshotPublicKey = previous })
	writeSnapshotKey(t, privateKey)
	restore := SetSourceStationStoreForTest(newMemorySourceStore())
	t.Cleanup(restore)
	if !officialSite() {
		t.Fatal("fixture should be the official site")
	}
	owner, repo, err := releaseImportRepo(releaseImportActorSite, "app", "")
	if err != nil || owner+"/"+repo != "maizll/auth-pro-client" {
		t.Fatalf("app seed %s/%s err=%v", owner, repo, err)
	}
	owner, repo, err = releaseImportRepo(releaseImportActorSite, "template", "")
	if err != nil || owner+"/"+repo != "maizll/auth-pro-paid" {
		t.Fatalf("catalog seed %s/%s err=%v", owner, repo, err)
	}
	rememberReleaseImportRepo(releaseImportActorSite, "app", "acme", "kept")
	owner, repo, err = releaseImportRepo(releaseImportActorSite, "app", "")
	if err != nil || owner+"/"+repo != "acme/kept" {
		t.Fatalf("saved repo overwritten: %s/%s err=%v", owner, repo, err)
	}
}

func TestDeveloperImportDoesNotUseSiteToken(t *testing.T) {
	store := newMemorySourceStore()
	store.releaseSettings.Token = "site-owner-secret"
	store.releaseSettings.Provider = "github"
	restore := SetSourceStationStoreForTest(store)
	t.Cleanup(restore)

	var gotAuth []string
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		switch {
		case strings.HasSuffix(r.URL.Path, "/asset/pkg"):
			_, _ = w.Write([]byte("pkg"))
		case strings.HasSuffix(r.URL.Path, "/asset/latest.json"):
			_, _ = w.Write([]byte(`{"notes":["auth-pro 1.0.0","- 修复登录。"]}`))
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			_, _ = w.Write([]byte(`{"assets":[{"id":7,"name":"latest.json","url":"` + upstream.URL + `/asset/latest.json"},{"id":8,"name":"auth_pro-full-v1.0.0.tar.gz","url":"` + upstream.URL + `/asset/pkg"}]}`))
		default:
			_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0","name":"v1.0.0","body":"## What's Changed\n* note https://github.com/acme/widgets/pull/1","draft":false,"assets":[{"name":"latest.json","size":10},{"name":"auth_pro-full-v1.0.0.tar.gz","size":3}]}]`))
		}
	}))
	defer upstream.Close()
	previous := productUpdateGitHubAPI
	productUpdateGitHubAPI = upstream.URL
	t.Cleanup(func() { productUpdateGitHubAPI = previous })
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterDeveloperReleaseImportRoutes(router.Group("/developer"))
	body := []byte(`{"purpose":"plugin","repo":"acme/widgets","token":"dev-owner-secret"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/developer/release-import/releases", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "auth-pro 1.0.0") || !strings.Contains(recorder.Body.String(), "修复登录") {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "site-owner-secret") || strings.Contains(recorder.Body.String(), "github.com") || strings.Contains(recorder.Body.String(), "What's Changed") {
		t.Fatalf("response leaked: %s", recorder.Body.String())
	}
	if len(gotAuth) == 0 {
		t.Fatal("upstream was not called")
	}
	for _, header := range gotAuth {
		if header != "Bearer dev-owner-secret" {
			t.Fatalf("authorization=%q", header)
		}
		if strings.Contains(header, "site-owner-secret") {
			t.Fatalf("site token used: %q", header)
		}
	}

	gotAuth = nil
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/developer/release-import/releases", bytes.NewReader([]byte(`{"purpose":"plugin","repo":"acme/widgets"}`)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("anonymous status %d body %s", recorder.Code, recorder.Body.String())
	}
	for _, header := range gotAuth {
		if header != "" {
			t.Fatalf("anonymous request sent %q", header)
		}
	}
}

func TestReleaseImportPlaceholderHasNoOwner(t *testing.T) {
	path := filepath.Join("..", "..", "frontend", "src", "components", "business", "release-import", "ReleaseRepoImport.vue")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "maizll/") {
		t.Fatal("import placeholder still names a built-in repository")
	}
}
