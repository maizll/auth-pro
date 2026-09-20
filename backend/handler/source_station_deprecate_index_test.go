package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDeprecateRemovesPluginAndTemplateFromPublicIndex(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "old-pay",
		`{"appId":1,"id":"old-pay","name":"旧支付","category":"payment","downloadUrl":"https://cdn.example.com/old-pay.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, sourceKindTemplate, "old-home",
		`{"appId":1,"id":"old-home","name":"旧首页","templateUrl":"https://cdn.example.com/old-home.zip","sha256":"`+sha+`","schemaVersion":1}`)
	assertPublicIndexIDs(t, router, "app-a", []string{"old-pay"}, []string{"old-home"})

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/old-pay/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/old-home/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate template=%s", rec.Body.String())
	}

	assertPublicIndexIDs(t, router, "app-a", nil, nil)
	assertLocalSoftwareSourceIDs(t, "http://127.0.0.1/software-source/app-a/index.json", nil, nil)

	plugin, err := store.GetPlugin("old-pay")
	if err != nil || plugin.Status != sourceItemDeprecated {
		t.Fatalf("plugin should remain deprecated for history: %+v err=%v", plugin, err)
	}
	template, err := store.GetTemplate("old-home")
	if err != nil || template.Status != sourceItemDeprecated {
		t.Fatalf("template should remain deprecated for history: %+v err=%v", template, err)
	}

	liveShelf := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=1", admin, "")
	if sourceBodyCode(t, liveShelf) != 200 {
		t.Fatalf("catalog items=%s", liveShelf.Body.String())
	}
	if strings.Contains(liveShelf.Body.String(), `"id":"old-pay"`) || strings.Contains(liveShelf.Body.String(), `"id":"old-home"`) {
		t.Fatalf("deprecated items must not appear as live catalog rows: %s", liveShelf.Body.String())
	}

	history := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=1&status=deprecated", admin, "")
	if sourceBodyCode(t, history) != 200 || !strings.Contains(history.Body.String(), `"old-pay"`) || !strings.Contains(history.Body.String(), `"old-home"`) {
		t.Fatalf("explicit deprecated filter should keep audit rows: %s", history.Body.String())
	}
}

func TestDeprecateLatestVersionClearsPublishedIndexEntry(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "pay-core",
		`{"appId":1,"id":"pay-core","name":"支付核心","category":"payment","version":"1.0.0","downloadUrl":"https://cdn.example.com/pay-1.0.0.zip","sha256":"`+sha+`"}`)
	assertPublicIndexIDs(t, router, "app-a", []string{"pay-core"}, nil)

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/pay-core/versions/1.0.0/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate latest version=%s", rec.Body.String())
	}
	assertPublicIndexIDs(t, router, "app-a", nil, nil)

	live := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(live.Body.String(), "pay-core") {
		t.Fatalf("deprecated latest must not leave a stale public entry: %s", live.Body.String())
	}
}

func TestDeprecateDoesNotLeakThroughUnfilteredPublicBuilder(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "gone-plug",
		`{"appId":1,"id":"gone-plug","name":"已弃用插件","category":"other","downloadUrl":"https://cdn.example.com/gone.zip","sha256":"`+sha+`"}`)
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/gone-plug/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate=%s", rec.Body.String())
	}

	app, err := store.GetCatalogAppByKey("app-a")
	if err != nil {
		t.Fatal(err)
	}
	payload, catalog, err := sourceCatalogJSONForApp(app)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "gone-plug") {
		t.Fatalf("public builder must exclude deprecated: %s", payload)
	}
	raw, _ := json.Marshal(catalog.Plugins)
	if strings.Contains(string(raw), "gone-plug") {
		t.Fatalf("catalog plugins still contain deprecated id: %s", raw)
	}
}
