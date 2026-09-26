package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestArchivedAppIndexReturnsJSONAndAliasServesTarget(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":2,"id":"kept-plugin","name":"目标插件","downloadUrl":"https://cdn.example.com/kept.zip","sha256":"`+sha+`","shelf":true}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register=%s", rec.Body.String())
	}
	store.mu.Lock()
	app := store.catalogApps[1]
	app.Archived = true
	store.catalogApps[1] = app
	store.mu.Unlock()

	gone := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if gone.Code != http.StatusGone || !strings.Contains(gone.Header().Get("Content-Type"), "json") {
		t.Fatalf("archived status=%d type=%s body=%s", gone.Code, gone.Header().Get("Content-Type"), gone.Body.String())
	}
	if !strings.Contains(gone.Body.String(), softwareSourceAppGoneMessage) || strings.Contains(strings.ToLower(gone.Body.String()), "<html") || strings.Contains(gone.Body.String(), `"plugins"`) {
		t.Fatalf("archived body=%s", gone.Body.String())
	}

	saved := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/source-aliases", admin,
		`{"oldAppKey":"app_4e85b4724223_2603","targetAppId":2}`)
	if sourceBodyCode(t, saved) != 200 {
		t.Fatalf("save missing key=%s", saved.Body.String())
	}
	again := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/source-aliases", admin,
		`{"oldAppKey":"app_4e85b4724223_2603","targetAppId":2}`)
	if sourceBodyCode(t, again) != 200 {
		t.Fatalf("idempotent save=%s", again.Body.String())
	}
	moved := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/source-aliases", admin,
		`{"oldAppKey":"app-a","targetAppId":2}`)
	if sourceBodyCode(t, moved) != 200 {
		t.Fatalf("save archived key=%s", moved.Body.String())
	}
	list := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/settings/source-aliases", admin, "")
	if sourceBodyCode(t, list) != 200 || strings.Count(list.Body.String(), `"oldAppKey"`) != 2 {
		t.Fatalf("list=%s", list.Body.String())
	}

	alias := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if alias.Code != http.StatusOK || !strings.Contains(alias.Body.String(), "kept-plugin") {
		t.Fatalf("alias body=%s", alias.Body.String())
	}
	legacy := sourceJSON(t, router, http.MethodGet, "/software-source/app_4e85b4724223_2603/index.json", "", "")
	if legacy.Code != http.StatusOK || !strings.Contains(legacy.Body.String(), "kept-plugin") {
		t.Fatalf("legacy key body=%s", legacy.Body.String())
	}
	compat := sourceJSON(t, router, http.MethodGet, "/auth-pro/app-a/index.json", "", "")
	if compat.Code != http.StatusOK || !strings.Contains(compat.Body.String(), "kept-plugin") {
		t.Fatalf("compat body=%s", compat.Body.String())
	}

	store.mu.Lock()
	store.catalogApps[3] = sourceCatalogApp{ID: 3, AppKey: "app-c", Name: "应用C", Enabled: true}
	store.mu.Unlock()
	live := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/source-aliases", admin,
		`{"oldAppKey":"app-b","targetAppId":3}`)
	if sourceBodyCode(t, live) != 400 || !strings.Contains(live.Body.String(), "还在使用") {
		t.Fatalf("live key=%s", live.Body.String())
	}
}

func TestArchiveValidatesSourceRedirectBeforeDeleting(t *testing.T) {
	store := newMemorySourceStore()
	restore := SetSourceStationStoreForTest(store)
	defer restore()
	if err := validateSoftwareSourceRedirect("app-a", 2, false); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.GetSoftwareSourceAlias("app-a"); err != nil || found {
		t.Fatalf("validate must not save alias found=%v err=%v", found, err)
	}
	if err := validateSoftwareSourceRedirect("app-a", 1, false); err == nil {
		t.Fatal("expected same-app rejection")
	}
	app := store.catalogApps[2]
	app.Archived = true
	store.catalogApps[2] = app
	if err := validateSoftwareSourceRedirect("app-a", 2, false); err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("archived target err=%v", err)
	}
}
