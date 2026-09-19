package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sourceMultipart(t *testing.T, router http.Handler, path, token, filename string, payload []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if payload != nil {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func sourcePluginTestZIP(t *testing.T) []byte {
	t.Helper()
	return makeTestZIP(t, testZIPEntry{name: "demo-plugin/plugin.json", data: `{
		"id":"demo-plugin",
		"name":"演示插件",
		"version":"1.0.0",
		"description":"内存解析测试",
		"author":{"name":"源站","url":"https://example.com","email":"dev@example.com"},
		"category":"other"
	}`})
}

func TestSourcePackageParseAutofillPlugin(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := sourcePluginTestZIP(t)
	wantSHA := sha256Hex(payload)
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "demo-plugin.zip", payload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("parse=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"demo-plugin"`) || !strings.Contains(rec.Body.String(), `"演示插件"`) || !strings.Contains(rec.Body.String(), `"1.0.0"`) {
		t.Fatalf("autofill missing fields: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), wantSHA) || !strings.Contains(rec.Body.String(), `"stored":false`) {
		t.Fatalf("sha256/stored missing: %s", rec.Body.String())
	}
	plugins, err := store.ListPlugins("")
	if err != nil || len(plugins) != 0 {
		t.Fatalf("parse must not persist catalog: %v %#v", err, plugins)
	}
}

func TestSourcePackageParseMissingPluginJSON(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := makeTestZIP(t, testZIPEntry{name: "readme.txt", data: "no manifest"})
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "empty.zip", payload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "plugin.json") {
		t.Fatalf("want missing plugin.json, got %s", rec.Body.String())
	}
}

func TestSourcePackageParseBadTemplateSchema(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{"id":"bad-home","name":"坏模板","version":"1.0.0","schemaVersion":2,"hero":{"title":"x"}}`})
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "template.zip", payload, map[string]string{"kind": "template"})
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "schemaVersion") {
		t.Fatalf("want schemaVersion error, got %s", rec.Body.String())
	}
}

func TestSourceReleaseSettingsMaskToken(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/release", admin,
		`{"provider":"github","owner":"acme","repo":"pkgs","token":"ghs_super_secret_token","tagStrategy":"{id}-{version}","branch":"main"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save settings=%s", save.Body.String())
	}
	if strings.Contains(save.Body.String(), "ghs_super_secret_token") {
		t.Fatalf("response leaked token: %s", save.Body.String())
	}
	got := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/settings/release", admin, "")
	if sourceBodyCode(t, got) != 200 {
		t.Fatalf("get settings=%s", got.Body.String())
	}
	if strings.Contains(got.Body.String(), "ghs_super_secret_token") {
		t.Fatalf("GET leaked token: %s", got.Body.String())
	}
	if !strings.Contains(got.Body.String(), `"hasToken":true`) || !strings.Contains(got.Body.String(), "****") {
		t.Fatalf("want masked hasToken: %s", got.Body.String())
	}
	keep := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/release", admin,
		`{"provider":"github","owner":"acme","repo":"pkgs","token":"","tagStrategy":"{id}-{version}","branch":"main"}`)
	if sourceBodyCode(t, keep) != 200 {
		t.Fatalf("keep token=%s", keep.Body.String())
	}
	again := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/settings/release", admin, "")
	if !strings.Contains(again.Body.String(), `"hasToken":true`) {
		t.Fatalf("empty PUT must keep token: %s", again.Body.String())
	}
}

func TestSourcePackagePublishGitHubRelease(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := sourcePluginTestZIP(t)
	var githubURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghs_test_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/pkgs/releases":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 42, "assets": []any{},
				"upload_url": githubURL + "/repos/acme/pkgs/releases/42/assets{?name,label}",
			})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/repos/acme/pkgs/releases/42/assets"):
			name := r.URL.Query().Get("name")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "name": name,
				"browser_download_url": "https://github.com/acme/pkgs/releases/download/demo-plugin-1.0.0/" + name,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	githubURL = server.URL
	t.Cleanup(server.Close)
	prevBase := sourceGitHubAPIBase
	sourceGitHubAPIBase = server.URL
	t.Cleanup(func() { sourceGitHubAPIBase = prevBase })

	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/release", admin,
		`{"provider":"github","owner":"acme","repo":"pkgs","token":"ghs_test_token","tagStrategy":"{id}-{version}","branch":"main"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("settings=%s", save.Body.String())
	}
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "demo-plugin.zip", payload, map[string]string{
		"kind": "plugin", "push": "1", "shelf": "1", "changelog": "首发",
	})
	body := rec.Body.String()
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("publish=%s", body)
	}
	if !strings.Contains(body, "https://github.com/acme/pkgs/releases/download/demo-plugin-1.0.0/demo-plugin-1.0.0.zip") {
		t.Fatalf("missing asset downloadUrl: %s", body)
	}
	if !strings.Contains(body, `"storedPackage":false`) {
		t.Fatalf("must not claim package stored: %s", body)
	}
	plugin, err := store.GetPlugin("demo-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.DownloadURL != "https://github.com/acme/pkgs/releases/download/demo-plugin-1.0.0/demo-plugin-1.0.0.zip" {
		t.Fatalf("plugin downloadUrl=%s", plugin.DownloadURL)
	}
	if plugin.SHA256 != sha256Hex(payload) || plugin.Status != sourceItemPublished {
		t.Fatalf("plugin sha/status=%s %s", plugin.SHA256, plugin.Status)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if !strings.Contains(index.Body.String(), `"demo-plugin"`) || !strings.Contains(index.Body.String(), plugin.DownloadURL) {
		t.Fatalf("index missing published plugin: %s", index.Body.String())
	}
}

func TestSourcePackagePublishPasteableDownloadURL(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := sourcePluginTestZIP(t)
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "demo-plugin.zip", payload, map[string]string{
		"kind": "plugin", "push": "0", "shelf": "1",
		"downloadUrl": "https://cdn.example.com/demo-plugin-1.0.0.zip",
	})
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("publish=%s", rec.Body.String())
	}
	plugin, err := store.GetPlugin("demo-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.DownloadURL != "https://cdn.example.com/demo-plugin-1.0.0.zip" {
		t.Fatalf("downloadUrl=%s", plugin.DownloadURL)
	}
	if plugin.SHA256 != sha256Hex(payload) {
		t.Fatalf("sha256 not of uploaded bytes")
	}
}

func TestSourcePackageParseDoesNotPersistFiles(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{
		"id":"clean-home","name":"清新首页","version":"1.0.0","schemaVersion":1,
		"description":"模板","author":"设计组","hero":{"title":"欢迎"}
	}`})
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "home.zip", payload, map[string]string{"kind": "template"})
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("parse template=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"clean-home"`) || !strings.Contains(rec.Body.String(), `"schemaVersion":1`) {
		t.Fatalf("template autofill: %s", rec.Body.String())
	}
	templates, err := store.ListTemplates("")
	if err != nil || len(templates) != 0 {
		t.Fatalf("parse must not persist templates: %v %#v", err, templates)
	}
	plugins, err := store.ListPlugins("")
	if err != nil || len(plugins) != 0 {
		t.Fatalf("parse must not persist plugins: %v %#v", err, plugins)
	}
}

func TestSourcePackagePublishGiteeRelease(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := sourcePluginTestZIP(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/pkgs/releases":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":9,"assets":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/pkgs/releases/9/attach_files":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"name":"demo-plugin-1.0.0.zip","browser_download_url":"https://gitee.com/acme/pkgs/releases/download/demo-plugin-1.0.0/demo-plugin-1.0.0.zip"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	prev := sourceGiteeAPIBase
	sourceGiteeAPIBase = server.URL
	t.Cleanup(func() { sourceGiteeAPIBase = prev })
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/release", admin,
		`{"provider":"gitee","owner":"acme","repo":"pkgs","token":"gitee_token","branch":"master"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("settings=%s", save.Body.String())
	}
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "demo-plugin.zip", payload, map[string]string{
		"kind": "plugin", "push": "1",
	})
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("gitee publish=%s", rec.Body.String())
	}
	plugin, err := store.GetPlugin("demo-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.DownloadURL != "https://gitee.com/acme/pkgs/releases/download/demo-plugin-1.0.0/demo-plugin-1.0.0.zip" {
		t.Fatalf("gitee url=%s", plugin.DownloadURL)
	}
	if plugin.Status != sourceItemDraft {
		t.Fatalf("without shelf, expect draft, got %s", plugin.Status)
	}
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
