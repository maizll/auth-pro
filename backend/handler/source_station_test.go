package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func sourceStationRouter(t *testing.T) (*gin.Engine, *memorySourceStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newMemorySourceStore()
	t.Cleanup(SetSourceStationStoreForTest(store))
	router := gin.New()
	RegisterSourceStationRoutes(router, router.Group("/api"))
	router.GET("/source", SourceStationPage)
	router.GET("/source/", SourceStationPage)
	router.GET("/api/v1/public/advertisements", PublicLocalAdvertisements)
	return router, store
}

func sourceJSON(t *testing.T, router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func sourceAdminToken(t *testing.T) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: 1, Username: "admin", Role: "admin", RoleCode: "R_SUPER",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func sourceBodyCode(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %s", recorder.Body.String())
	}
	return body.Code
}

func sourceTestSHA256() string {
	return strings.Repeat("ab", 32)
}

func sourceApproveDeveloper(t *testing.T, router http.Handler, username, password string) (adminToken, devToken string, appID int64) {
	t.Helper()
	adminToken = sourceAdminToken(t)
	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", "",
		`{"username":"`+username+`","password":"`+password+`","displayName":"`+username+`","reason":"publish demo"}`)
	if sourceBodyCode(t, apply) != 200 {
		t.Fatalf("apply=%s", apply.Body.String())
	}
	var applyBody struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(apply.Body.Bytes(), &applyBody); err != nil {
		t.Fatal(err)
	}
	appID = applyBody.Data.ID
	approve := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(appID)+"/approve", adminToken, "{}")
	if sourceBodyCode(t, approve) != 200 || !strings.Contains(approve.Body.String(), sourceDeveloperRoleCode) {
		t.Fatalf("approve=%s", approve.Body.String())
	}
	login := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/login", "",
		`{"username":"`+username+`","password":"`+password+`"}`)
	if sourceBodyCode(t, login) != 200 {
		t.Fatalf("login=%s", login.Body.String())
	}
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil {
		t.Fatal(err)
	}
	return adminToken, loginBody.Data.Token, appID
}

func TestSourceStationIndexJSONShape(t *testing.T) {
	router, _ := sourceStationRouter(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	for _, path := range []string{"/software-source/index.json", "/auth-pro/index.json"} {
		index, payload, sourceType, err := fetchPluginSourceManifest(context.Background(), server.URL+path)
		if err != nil || sourceType != "json" {
			t.Fatalf("%s type=%q err=%v body=%s", path, sourceType, err, payload)
		}
		if index.Name != sourceStationSourceName || index.Plugins == nil || index.HomeTemplates == nil {
			t.Fatalf("%s empty index=%+v body=%s", path, index, payload)
		}
		if len(index.Plugins) != 0 || len(index.HomeTemplates) != 0 {
			t.Fatalf("%s unpublished catalog must be empty: %s", path, payload)
		}
		var raw map[string]any
		if err := json.Unmarshal(payload, &raw); err != nil {
			t.Fatal(err)
		}
		if _, exists := raw["data"]; exists {
			t.Fatalf("index.json must be a raw manifest, not an API envelope: %s", payload)
		}
		if _, ok := raw["plugins"].([]any); !ok {
			t.Fatalf("plugins must be array: %s", payload)
		}
		if _, ok := raw["homeTemplates"].([]any); !ok {
			t.Fatalf("homeTemplates must be array: %s", payload)
		}
	}
}

func TestSourceStationApproveFlowFeedsPublishedIndex(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "dev-alice", "secret1")
	sha := sourceTestSHA256()
	pluginBody := `{"id":"demo-plugin","name":"Demo Plugin","version":"1.0.0","description":"授权本地插件源测试","category":"other","downloadUrl":"https://cdn.example.com/demo-plugin.zip","sha256":"` + sha + `"}`
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, pluginBody); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/demo-plugin/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/demo-plugin/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/demo-plugin/shelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("shelf plugin=%s", rec.Body.String())
	}

	templateBody := `{"templateKey":"source-home","name":"源站首页","version":"1.0.0","description":"最小首页模板","schemaVersion":1,"templateUrl":"https://cdn.example.com/templates/source-home.json","sha256":"` + sha + `"}`
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/templates", dev, templateBody); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save template=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/templates/source-home/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit template=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/source-home/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve template=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/source-home/shelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("shelf template=%s", rec.Body.String())
	}

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	index, payload, sourceType, err := fetchPluginSourceManifest(context.Background(), server.URL+"/software-source/index.json")
	if err != nil || sourceType != "json" || len(index.Plugins) != 1 || len(index.HomeTemplates) != 1 {
		t.Fatalf("index=%+v err=%v body=%s", index, err, payload)
	}
	if index.Plugins[0].ID != "demo-plugin" || index.Plugins[0].DownloadURL != "https://cdn.example.com/demo-plugin.zip" {
		t.Fatalf("plugin=%+v", index.Plugins[0])
	}
	var manifest struct {
		Name          string `json:"name"`
		HomeTemplates []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Description   string `json:"description"`
			Version       string `json:"version"`
			SchemaVersion int    `json:"schemaVersion"`
			SHA256        string `json:"sha256"`
			TemplateURL   string `json:"templateUrl"`
		} `json:"homeTemplates"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	item := manifest.HomeTemplates[0]
	if item.ID != "source-home" || item.Name == "" || item.Version != "1.0.0" || item.SchemaVersion != 1 {
		t.Fatalf("homeTemplate=%+v body=%s", item, payload)
	}
	if item.SHA256 != sha || item.TemplateURL != "https://cdn.example.com/templates/source-home.json" {
		t.Fatalf("homeTemplate location=%+v", item)
	}

	status := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply/status", "", `{"username":"dev-alice"}`)
	if sourceBodyCode(t, status) != 200 || !strings.Contains(status.Body.String(), `"approved"`) {
		t.Fatalf("apply status=%s", status.Body.String())
	}
}

func TestSourceStationUnshelvedItemsAbsentFromIndex(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	register := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"ext-plugin","name":"外部插件","downloadUrl":"https://cdn.example.com/ext.zip","sha256":"`+sha+`","shelf":true}`)
	if sourceBodyCode(t, register) != 200 {
		t.Fatalf("register=%s", register.Body.String())
	}
	tpl := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"id":"ext-home","name":"外部首页","templateUrl":"templates/ext-home.json","sha256":"`+sha+`","schemaVersion":1,"shelf":true}`)
	if sourceBodyCode(t, tpl) != 200 {
		t.Fatalf("register template=%s", tpl.Body.String())
	}

	live := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), `"ext-plugin"`) || !strings.Contains(live.Body.String(), `"ext-home"`) {
		t.Fatalf("published index=%s", live.Body.String())
	}

	unshelf := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/ext-plugin/unshelf", admin, "{}")
	if sourceBodyCode(t, unshelf) != 200 {
		t.Fatalf("unshelf plugin=%s", unshelf.Body.String())
	}
	unshelfTpl := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/ext-home/unshelf", admin, "{}")
	if sourceBodyCode(t, unshelfTpl) != 200 {
		t.Fatalf("unshelf template=%s", unshelfTpl.Body.String())
	}

	hidden := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if strings.Contains(hidden.Body.String(), "ext-plugin") || strings.Contains(hidden.Body.String(), "ext-home") {
		t.Fatalf("unshelved items must be absent: %s", hidden.Body.String())
	}
	if !strings.Contains(hidden.Body.String(), `"plugins":[]`) || !strings.Contains(hidden.Body.String(), `"homeTemplates":[]`) {
		t.Fatalf("empty arrays expected: %s", hidden.Body.String())
	}

	audit := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/audit", admin, "")
	if sourceBodyCode(t, audit) != 200 || !strings.Contains(audit.Body.String(), `"unshelf"`) {
		t.Fatalf("audit=%s", audit.Body.String())
	}
}

func TestSourceStationPublishRequiresSHA256AndURL(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	incomplete := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"no-hash","name":"缺校验","downloadUrl":"https://cdn.example.com/x.zip","shelf":true}`)
	if sourceBodyCode(t, incomplete) != 400 || !strings.Contains(incomplete.Body.String(), "sha256") {
		t.Fatalf("missing sha256=%s", incomplete.Body.String())
	}
	noURL := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"no-url","name":"缺地址","sha256":"`+sourceTestSHA256()+`","shelf":true}`)
	if sourceBodyCode(t, noURL) != 400 {
		t.Fatalf("missing url=%s", noURL.Body.String())
	}
	httpURL := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"plain-http","name":"明文","downloadUrl":"http://cdn.example.com/x.zip","sha256":"`+sourceTestSHA256()+`"}`)
	if sourceBodyCode(t, httpURL) != 400 {
		t.Fatalf("http url=%s", httpURL.Body.String())
	}
	draft := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"id":"draft-home","name":"草稿首页"}`)
	if sourceBodyCode(t, draft) != 200 {
		t.Fatalf("draft template=%s", draft.Body.String())
	}
	shelf := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/draft-home/shelf", admin, "{}")
	if sourceBodyCode(t, shelf) != 400 {
		t.Fatalf("shelf without sha/url=%s", shelf.Body.String())
	}
	live := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if strings.Contains(live.Body.String(), "no-hash") || strings.Contains(live.Body.String(), "draft-home") {
		t.Fatalf("incomplete items leaked into index: %s", live.Body.String())
	}
}

func TestSourceDeveloperRejectBlocksLogin(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", "",
		`{"username":"dev-bob","password":"secret1","reason":"nope"}`)
	var applyBody struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(apply.Body.Bytes(), &applyBody)
	reject := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(applyBody.Data.ID)+"/reject",
		admin, `{"note":"not now"}`)
	if sourceBodyCode(t, reject) != 200 {
		t.Fatalf("reject=%s", reject.Body.String())
	}
	login := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/login", "",
		`{"username":"dev-bob","password":"secret1"}`)
	if sourceBodyCode(t, login) != 401 {
		t.Fatalf("rejected developer login=%s", login.Body.String())
	}
}

func TestSourceAdvertisementCRUDFeedsLocalEndpoint(t *testing.T) {
	router, _ := sourceStationRouter(t)
	resetAdvertisementCache(t)
	admin := sourceAdminToken(t)
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/advertisements", admin,
		`{"id":"welcome","title":"欢迎","imageUrl":"https://example.com/a.png","destinationUrl":"https://example.com","position":"home-banner","weight":9}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save ad=%s", save.Body.String())
	}
	public := sourceJSON(t, router, http.MethodGet, "/api/v1/public/advertisements?position=home-banner", "", "")
	if sourceBodyCode(t, public) != 200 || !strings.Contains(public.Body.String(), `"id":"welcome"`) {
		t.Fatalf("public ads=%s", public.Body.String())
	}
	del := sourceJSON(t, router, http.MethodDelete, "/api/v1/source/admin/advertisements/welcome", admin, "")
	if sourceBodyCode(t, del) != 200 {
		t.Fatalf("delete ad=%s", del.Body.String())
	}
}

func TestSourceStationPageRedirectsToAdmin(t *testing.T) {
	router, _ := sourceStationRouter(t)
	for _, path := range []string{"/source", "/source/"} {
		page := sourceJSON(t, router, http.MethodGet, path, "", "")
		if page.Code != http.StatusFound {
			t.Fatalf("%s redirect status=%d %s", path, page.Code, page.Body.String())
		}
		if loc := page.Header().Get("Location"); loc != SourceStationAdminPath {
			t.Fatalf("%s redirect location=%s want %s", path, loc, SourceStationAdminPath)
		}
	}
}

func TestSourceStationPluginVersionUpdateUnshelfAndRollback(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha1 := sourceTestSHA256()
	sha2 := strings.Repeat("cd", 32)
	register := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"up-plugin","name":"可更新插件","version":"1.0.0","downloadUrl":"https://cdn.example.com/up-1.0.0.zip","sha256":"`+sha1+`","changelog":"首发","minVersion":"1.0.0","forceUpdate":false,"shelf":true}`)
	if sourceBodyCode(t, register) != 200 {
		t.Fatalf("register v1=%s", register.Body.String())
	}
	v1 := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if !strings.Contains(v1.Body.String(), `"1.0.0"`) || !strings.Contains(v1.Body.String(), "up-1.0.0.zip") {
		t.Fatalf("index v1=%s", v1.Body.String())
	}

	create := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/up-plugin/versions", admin,
		`{"version":"1.0.1","downloadUrl":"https://cdn.example.com/up-1.0.1.zip","sha256":"`+sha2+`","changelog":"修复下载"}`)
	if sourceBodyCode(t, create) != 200 {
		t.Fatalf("create v1.0.1=%s", create.Body.String())
	}
	submit := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/up-plugin/versions/1.0.1/approve", admin, "{}")
	if sourceBodyCode(t, submit) != 200 {
		t.Fatalf("approve v1.0.1=%s", submit.Body.String())
	}
	latest := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if !strings.Contains(latest.Body.String(), `"version":"1.0.1"`) || !strings.Contains(latest.Body.String(), "up-1.0.1.zip") {
		t.Fatalf("index should show latest 1.0.1: %s", latest.Body.String())
	}
	if !strings.Contains(latest.Body.String(), `"changelog":"修复下载"`) || !strings.Contains(latest.Body.String(), `"forceUpdate":false`) {
		t.Fatalf("index missing changelog/forceUpdate: %s", latest.Body.String())
	}
	if strings.Contains(latest.Body.String(), "up-1.0.0.zip") {
		t.Fatalf("index should not keep old downloadUrl as current: %s", latest.Body.String())
	}

	versions := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/plugins/up-plugin/versions", admin, "")
	if sourceBodyCode(t, versions) != 200 || !strings.Contains(versions.Body.String(), `"1.0.0"`) || !strings.Contains(versions.Body.String(), `"1.0.1"`) {
		t.Fatalf("versions=%s", versions.Body.String())
	}
	alias := sourceJSON(t, router, http.MethodGet, "/api/admin/source/plugins/up-plugin/versions", admin, "")
	if sourceBodyCode(t, alias) != 200 || !strings.Contains(alias.Body.String(), `"1.0.1"`) {
		t.Fatalf("alias versions=%s", alias.Body.String())
	}

	unshelf := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/up-plugin/unshelf", admin, "{}")
	if sourceBodyCode(t, unshelf) != 200 {
		t.Fatalf("unshelf=%s", unshelf.Body.String())
	}
	hidden := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if strings.Contains(hidden.Body.String(), "up-plugin") {
		t.Fatalf("unshelf must hide plugin: %s", hidden.Body.String())
	}

	shelf := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/up-plugin/shelf", admin, "{}")
	if sourceBodyCode(t, shelf) != 200 {
		t.Fatalf("reshelf=%s", shelf.Body.String())
	}
	rollback := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/up-plugin/versions/1.0.0/latest", admin, "{}")
	if sourceBodyCode(t, rollback) != 200 {
		t.Fatalf("rollback=%s", rollback.Body.String())
	}
	rolled := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if !strings.Contains(rolled.Body.String(), `"version":"1.0.0"`) || !strings.Contains(rolled.Body.String(), "up-1.0.0.zip") {
		t.Fatalf("rollback should restore 1.0.0 in index: %s", rolled.Body.String())
	}
	if strings.Contains(rolled.Body.String(), "up-1.0.1.zip") {
		t.Fatalf("rolled index still points at 1.0.1: %s", rolled.Body.String())
	}

	kept := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/plugins/up-plugin/versions", admin, "")
	if !strings.Contains(kept.Body.String(), `"1.0.1"`) {
		t.Fatalf("prior version metadata must remain: %s", kept.Body.String())
	}
}

func itoa64(value int64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
