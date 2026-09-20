package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPublicSoftwareSourceIndexFollowsShelfAndUnshelf(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "live-plugin",
		`{"appId":1,"id":"live-plugin","name":"上架插件","category":"other","downloadUrl":"https://cdn.example.com/live-plugin.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, sourceKindTemplate, "live-home",
		`{"appId":1,"id":"live-home","name":"上架首页","templateUrl":"https://cdn.example.com/live-home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	assertPublicIndexIDs(t, router, "app-a", []string{"live-plugin"}, []string{"live-home"})
	assertLocalSoftwareSourceIDs(t, "http://127.0.0.1/software-source/app-a/index.json", []string{"live-plugin"}, []string{"live-home"})

	snap, err := store.LatestIndexSnapshot()
	if err != nil || snap.GeneratedBy == "" {
		t.Fatalf("shelf must auto-publish the public index (snapshot): %+v err=%v", snap, err)
	}

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/live-plugin/unshelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("unshelf plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/live-home/unshelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("unshelf template=%s", rec.Body.String())
	}

	assertPublicIndexIDs(t, router, "app-a", nil, nil)
	assertLocalSoftwareSourceIDs(t, "http://127.0.0.1/software-source/app-a/index.json", nil, nil)
	hiddenSnap, err := store.LatestIndexSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hiddenSnap.Payload, "live-plugin") || strings.Contains(hiddenSnap.Payload, "live-home") {
		t.Fatalf("unshelf must republish index without stale entries: %s", hiddenSnap.Payload)
	}
}

func TestDeprecateClearsPublicIndexAndAdminDefaultCatalog(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "gone-plugin",
		`{"appId":1,"id":"gone-plugin","name":"待弃用插件","category":"other","downloadUrl":"https://cdn.example.com/gone-plugin.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, sourceKindTemplate, "gone-home",
		`{"appId":1,"id":"gone-home","name":"待弃用首页","templateUrl":"https://cdn.example.com/gone-home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	assertPublicIndexIDs(t, router, "app-a", []string{"gone-plugin"}, []string{"gone-home"})
	assertAdminDefaultCatalogIDs(t, router, admin, []string{"gone-plugin", "gone-home"})

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/gone-plugin/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/gone-home/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate template=%s", rec.Body.String())
	}

	assertPublicIndexIDs(t, router, "app-a", nil, nil)
	assertLocalSoftwareSourceIDs(t, "http://127.0.0.1/software-source/app-a/index.json", nil, nil)
	assertAdminDefaultCatalogIDs(t, router, admin, nil)

	audit := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?status=deprecated", admin, "")
	if ids := sourceCatalogItemIDs(t, audit.Body.Bytes()); !sameStringSet(ids, []string{"gone-plugin", "gone-home"}) {
		t.Fatalf("status=deprecated should keep audit rows, got %v body=%s", ids, audit.Body.String())
	}

	snap, err := store.LatestIndexSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snap.Payload, "gone-plugin") || strings.Contains(snap.Payload, "gone-home") {
		t.Fatalf("deprecate must republish index without stale entries: %s", snap.Payload)
	}
}

func TestDeprecateLatestPublishedVersionClearsPublicIndex(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "solo-plugin",
		`{"appId":1,"id":"solo-plugin","name":"单版本插件","version":"1.0.0","category":"other","downloadUrl":"https://cdn.example.com/solo-1.0.0.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, sourceKindTemplate, "solo-home",
		`{"appId":1,"id":"solo-home","name":"单版本首页","version":"1.0.0","templateUrl":"https://cdn.example.com/solo-home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	assertPublicIndexIDs(t, router, "app-a", []string{"solo-plugin"}, []string{"solo-home"})
	assertAdminDefaultCatalogIDs(t, router, admin, []string{"solo-plugin", "solo-home"})

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/solo-plugin/versions/1.0.0/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate plugin version=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/solo-home/versions/1.0.0/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate template version=%s", rec.Body.String())
	}

	assertPublicIndexIDs(t, router, "app-a", nil, nil)
	assertLocalSoftwareSourceIDs(t, "http://127.0.0.1/software-source/app-a/index.json", nil, nil)
	assertAdminDefaultCatalogIDs(t, router, admin, nil)
}

func TestDeprecateLatestVersionRepointsToRemainingPublished(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha1 := sourceTestSHA256()
	sha2 := strings.Repeat("cd", 32)

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "multi-plugin",
		`{"appId":1,"id":"multi-plugin","name":"多版本插件","version":"1.0.0","category":"other","downloadUrl":"https://cdn.example.com/multi-1.0.0.zip","sha256":"`+sha1+`"}`)
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/multi-plugin/versions", admin,
		`{"version":"1.0.1","downloadUrl":"https://cdn.example.com/multi-1.0.1.zip","sha256":"`+sha2+`","changelog":"下一版"}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("create 1.0.1=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/multi-plugin/versions/1.0.1/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve 1.0.1=%s", rec.Body.String())
	}

	live := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(live.Body.String(), `"version":"1.0.1"`) || !strings.Contains(live.Body.String(), "multi-1.0.1.zip") {
		t.Fatalf("latest should be 1.0.1: %s", live.Body.String())
	}

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/multi-plugin/versions/1.0.1/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate 1.0.1=%s", rec.Body.String())
	}

	assertPublicIndexIDs(t, router, "app-a", []string{"multi-plugin"}, nil)
	rolled := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(rolled.Body.String(), `"version":"1.0.0"`) || !strings.Contains(rolled.Body.String(), "multi-1.0.0.zip") {
		t.Fatalf("remaining published version should stay in public index: %s", rolled.Body.String())
	}
	if strings.Contains(rolled.Body.String(), "multi-1.0.1.zip") {
		t.Fatalf("deprecated latest must not remain the public version: %s", rolled.Body.String())
	}
	assertAdminDefaultCatalogIDs(t, router, admin, []string{"multi-plugin"})
}

func TestPublishedIndexUpdatesOnMetadataVersionAndCategories(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	nextSHA := strings.Repeat("cd", 32)

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "pay-core",
		`{"appId":1,"id":"pay-core","name":"支付核心","category":"payment","version":"1.0.0","downloadUrl":"https://cdn.example.com/pay-1.0.0.zip","sha256":"`+sha+`"}`)

	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/pay-core", admin,
		`{"name":"聚合支付","category":"payment","version":"1.0.1","downloadUrl":"https://cdn.example.com/pay-1.0.1.zip","sha256":"`+nextSHA+`"}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("edit published=%s", rec.Body.String())
	}
	live := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(live.Body.String(), "聚合支付") || !strings.Contains(live.Body.String(), "pay-1.0.1.zip") || !strings.Contains(live.Body.String(), nextSHA) {
		t.Fatalf("published edit must update public index: %s", live.Body.String())
	}

	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/categories", admin,
		`{"extras":[{"key":"template","label":"模板","kind":"plugin"}]}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save extras=%s", rec.Body.String())
	}
	live = sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(live.Body.String(), `"key":"template"`) || !strings.Contains(live.Body.String(), `"label":"模板"`) {
		t.Fatalf("custom categories must stay in published index: %s", live.Body.String())
	}

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/pay-core/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate=%s", rec.Body.String())
	}
	assertPublicIndexIDs(t, router, "app-a", nil, nil)
	assertLocalSoftwareSourceIDs(t, "http://127.0.0.1/software-source/app-a/index.json", nil, nil)
}

func assertAdminDefaultCatalogIDs(t *testing.T, router http.Handler, admin string, want []string) {
	t.Helper()
	rec := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items", admin, "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("admin catalog=%s", rec.Body.String())
	}
	got := sourceCatalogItemIDs(t, rec.Body.Bytes())
	if !sameStringSet(got, want) {
		t.Fatalf("admin default catalog=%v want %v body=%s", got, want, rec.Body.String())
	}
}

func assertPublicIndexIDs(t *testing.T, router http.Handler, appKey string, plugins, templates []string) {
	t.Helper()
	live := sourceJSON(t, router, http.MethodGet, "/software-source/"+appKey+"/index.json", "", "")
	if live.Code != http.StatusOK {
		t.Fatalf("public index status=%d body=%s", live.Code, live.Body.String())
	}
	assertIndexIDs(t, live.Body.Bytes(), plugins, templates)
}

func assertLocalSoftwareSourceIDs(t *testing.T, rawURL string, plugins, templates []string) {
	t.Helper()
	index, payload, sourceType, err := fetchPluginSourceManifest(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("local software source %s: %v", rawURL, err)
	}
	if sourceType != "json" && sourceType != "local" {
		t.Fatalf("same-instance index must resolve from the live catalog, got %q payload=%s", sourceType, payload)
	}
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	assertIndexIDs(t, raw, plugins, templates)
}

func assertIndexIDs(t *testing.T, payload []byte, plugins, templates []string) {
	t.Helper()
	var catalog struct {
		Plugins       []struct{ ID string `json:"id"` } `json:"plugins"`
		HomeTemplates []struct{ ID string `json:"id"` } `json:"homeTemplates"`
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatalf("index json: %v body=%s", err, payload)
	}
	gotPlugins := make([]string, 0, len(catalog.Plugins))
	for _, item := range catalog.Plugins {
		gotPlugins = append(gotPlugins, item.ID)
	}
	gotTemplates := make([]string, 0, len(catalog.HomeTemplates))
	for _, item := range catalog.HomeTemplates {
		gotTemplates = append(gotTemplates, item.ID)
	}
	if !sameStringSet(gotPlugins, plugins) {
		t.Fatalf("plugins=%v want %v body=%s", gotPlugins, plugins, payload)
	}
	if !sameStringSet(gotTemplates, templates) {
		t.Fatalf("homeTemplates=%v want %v body=%s", gotTemplates, templates, payload)
	}
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] == 0 {
			return false
		}
		seen[id]--
	}
	return true
}
