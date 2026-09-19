package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auto_pro/middleware"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func sourceStationRouter(t *testing.T) (*gin.Engine, *memorySourceStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newMemorySourceStore()
	t.Cleanup(SetSourceStationStoreForTest(store))
	router := gin.New()
	RegisterSourceStationRoutes(router.Group("/api"))
	router.GET("/source", SourceStationPage)
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

func sourceMultipart(t *testing.T, router http.Handler, path, token string, fields map[string]string, fileField, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if filename != "" {
		part, err := writer.CreateFormFile(fileField, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &buffer)
	request.Header.Set("Content-Type", writer.FormDataContentType())
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

func TestSourceCatalogEmptyMatchesSoftwareSourceClient(t *testing.T) {
	router, _ := sourceStationRouter(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client, err := softwaresource.NewClient(softwaresource.ClientConfig{
		BaseURL: server.URL, CatalogKey: "any-non-empty-key", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Revision < 1 || len(catalog.Sources) != 1 || catalog.Sources[0].ID != sourceStationSourceID {
		t.Fatalf("empty catalog=%+v", catalog)
	}
	if len(catalog.Templates) != 0 {
		t.Fatalf("empty source should have no templates: %+v", catalog.Templates)
	}
}

func TestSourceCatalogServesPluginAndHomeTemplateForClients(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", "",
		`{"username":"dev-alice","password":"secret1","displayName":"Alice","reason":"publish demo"}`)
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
	approve := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(applyBody.Data.ID)+"/approve", admin, "{}")
	if sourceBodyCode(t, approve) != 200 || !strings.Contains(approve.Body.String(), sourceDeveloperRoleCode) {
		t.Fatalf("approve=%s", approve.Body.String())
	}
	login := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/login", "",
		`{"username":"dev-alice","password":"secret1"}`)
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

	pluginZIP := makeTestZIP(t,
		testZIPEntry{name: "plugin.json", data: `{"id":"demo-plugin","name":"Demo"}`},
		testZIPEntry{name: "readme.txt", data: "demo plugin"},
	)
	pluginResp := sourceMultipart(t, router, "/api/v1/source/developer/plugins", loginBody.Data.Token, map[string]string{
		"id": "demo-plugin", "name": "Demo Plugin", "version": "1.0.0", "description": "授权本地插件源测试", "category": "other",
	}, "file", "demo-plugin.zip", pluginZIP)
	if sourceBodyCode(t, pluginResp) != 200 {
		t.Fatalf("publish plugin=%s", pluginResp.Body.String())
	}

	templateJSON := []byte(`{"schemaVersion":1,"hero":{"title":"源站首页"}}`)
	templateResp := sourceMultipart(t, router, "/api/v1/source/developer/templates", loginBody.Data.Token, map[string]string{
		"templateKey": "source-home", "name": "源站首页", "version": "1.0.0", "description": "最小首页模板",
	}, "file", "source-home.json", templateJSON)
	if sourceBodyCode(t, templateResp) != 200 {
		t.Fatalf("publish template=%s", templateResp.Body.String())
	}

	previewPart := sourceMultipart(t, router, "/api/v1/source/developer/templates", loginBody.Data.Token, map[string]string{
		"templateKey": "source-home", "name": "源站首页", "version": "1.0.1",
	}, "file", "source-home.json", templateJSON)
	if sourceBodyCode(t, previewPart) != 200 {
		t.Fatalf("republish template=%s", previewPart.Body.String())
	}

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client, err := softwaresource.NewClient(softwaresource.ClientConfig{
		BaseURL: server.URL, CatalogKey: "client-key", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.Refresh(context.Background())
	if err != nil || len(catalog.Templates) != 1 {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
	item := catalog.Templates[0]
	if item.TemplateKey != "source-home" || item.SchemaVersion != 1 || item.Format != "json" || item.Source.ID != sourceStationSourceID {
		t.Fatalf("template=%+v", item)
	}
	content, err := client.TemplateContent(context.Background(), item)
	if err != nil || string(content) != string(templateJSON) {
		t.Fatalf("content=%s err=%v", content, err)
	}

	index := sourceJSON(t, router, http.MethodGet, "/api/v1/catalog/index.json", "", "")
	parsed, err := parsePluginSourceManifest(index.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != sourceStationSourceName || len(parsed.Plugins) != 1 || parsed.Plugins[0].ID != "demo-plugin" {
		t.Fatalf("plugin index=%s", index.Body.String())
	}
	if len(parsed.HomeTemplates) != 1 {
		t.Fatalf("homeTemplates=%s", index.Body.String())
	}
	pkg := sourceJSON(t, router, http.MethodGet, "/api/v1/catalog/plugins/demo-plugin/package", "", "")
	if pkg.Code != http.StatusOK || pkg.Header().Get("X-Checksum-SHA256") == "" {
		t.Fatalf("plugin package status=%d headers=%v body=%s", pkg.Code, pkg.Header(), pkg.Body.String())
	}
}

func TestSourceDeveloperRejectAndCatalogKey(t *testing.T) {
	router, store := sourceStationRouter(t)
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

	if err := store.SetCatalogKey("locked-key"); err != nil {
		t.Fatal(err)
	}
	unauthorized := sourceJSON(t, router, http.MethodGet, "/api/v1/catalog/sources", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing catalog key status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/sources", nil)
	request.Header.Set("X-Software-Source-Key", "locked-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), sourceStationSourceID) {
		t.Fatalf("valid catalog key=%s", recorder.Body.String())
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

func TestSourceStationPageAndInvalidTemplateRejected(t *testing.T) {
	router, _ := sourceStationRouter(t)
	page := sourceJSON(t, router, http.MethodGet, "/source", "", "")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "本站软件源") {
		t.Fatalf("source page=%d %s", page.Code, page.Body.String())
	}
	admin := sourceAdminToken(t)
	apply := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/apply", "",
		`{"username":"dev-cara","password":"secret1"}`)
	var applyBody struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(apply.Body.Bytes(), &applyBody)
	sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/applications/"+itoa64(applyBody.Data.ID)+"/approve", admin, "{}")
	login := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/login", "",
		`{"username":"dev-cara","password":"secret1"}`)
	var loginBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &loginBody)
	bad := sourceMultipart(t, router, "/api/v1/source/developer/templates", loginBody.Data.Token, map[string]string{
		"templateKey": "bad-home", "name": "坏模板",
	}, "file", "bad.json", []byte(`{"schemaVersion":2,"hero":{"title":"x"}}`))
	if sourceBodyCode(t, bad) != 400 {
		t.Fatalf("invalid template=%s", bad.Body.String())
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
