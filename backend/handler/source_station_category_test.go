package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSourceCatalogCategoriesIncludesPluginAndHomeTemplate(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	rec := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/categories", admin, "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("categories=%s", rec.Body.String())
	}
	var body struct {
		Data struct {
			List []struct {
				Key     string `json:"key"`
				Label   string `json:"label"`
				Kind    string `json:"kind"`
				Builtin bool   `json:"builtin"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, item := range body.Data.List {
		found[item.Key] = item.Kind
		if !item.Builtin {
			t.Fatalf("builtin list should not include extras yet: %+v", item)
		}
	}
	if found["payment"] != sourceKindPlugin || found["realname"] != sourceKindPlugin || found["other"] != sourceKindPlugin {
		t.Fatalf("plugin categories missing: %+v body=%s", found, rec.Body.String())
	}
	if found[sourceCategoryHomeTemplate] != sourceKindTemplate {
		t.Fatalf("home template category missing: %+v body=%s", found, rec.Body.String())
	}
}

func TestSourceCatalogItemsFilterByCategory(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"pay-plugin","name":"支付插件","category":"payment","downloadUrl":"https://cdn.example.com/pay.zip","sha256":"`+sha+`"}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register payment=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"misc-plugin","name":"其他插件","category":"other","downloadUrl":"https://cdn.example.com/misc.zip","sha256":"`+sha+`"}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register other=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"id":"clean-home","name":"清新首页","templateUrl":"https://cdn.example.com/home.zip","sha256":"`+sha+`","schemaVersion":1}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register template=%s", rec.Body.String())
	}

	all := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items", admin, "")
	if sourceBodyCode(t, all) != 200 {
		t.Fatalf("catalog items=%s", all.Body.String())
	}
	if got := sourceCatalogItemIDs(t, all.Body.Bytes()); len(got) != 3 {
		t.Fatalf("want 3 unified items, got %v body=%s", got, all.Body.String())
	}

	payment := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?category=payment", admin, "")
	if ids := sourceCatalogItemIDs(t, payment.Body.Bytes()); len(ids) != 1 || ids[0] != "pay-plugin" {
		t.Fatalf("payment filter=%v body=%s", ids, payment.Body.String())
	}
	home := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?category=home-template", admin, "")
	if ids := sourceCatalogItemIDs(t, home.Body.Bytes()); len(ids) != 1 || ids[0] != "clean-home" {
		t.Fatalf("home-template filter=%v body=%s", ids, home.Body.String())
	}
}

func TestSourceCatalogIndexDerivesPluginsAndHomeTemplatesByCategory(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	sourceRegisterAndPublish(t, router, admin, "plugin", "pay-plugin",
		`{"id":"pay-plugin","name":"支付插件","category":"payment","downloadUrl":"https://cdn.example.com/pay.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, "template", "clean-home",
		`{"id":"clean-home","name":"清新首页","category":"home-template","templateUrl":"https://cdn.example.com/home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	live := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if live.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", live.Code, live.Body.String())
	}
	var manifest struct {
		Name       string           `json:"name"`
		Plugins    []map[string]any `json:"plugins"`
		HomeTpls   []map[string]any `json:"homeTemplates"`
		Categories []struct {
			Key  string `json:"key"`
			Kind string `json:"kind"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(live.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Plugins) != 1 || manifest.Plugins[0]["id"] != "pay-plugin" || manifest.Plugins[0]["category"] != "payment" {
		t.Fatalf("plugins shim=%s", live.Body.String())
	}
	if len(manifest.HomeTpls) != 1 || manifest.HomeTpls[0]["id"] != "clean-home" || manifest.HomeTpls[0]["category"] != sourceCategoryHomeTemplate {
		t.Fatalf("homeTemplates shim=%s", live.Body.String())
	}
	foundHome := false
	for _, category := range manifest.Categories {
		if category.Key == sourceCategoryHomeTemplate && category.Kind == sourceKindTemplate {
			foundHome = true
		}
	}
	if !foundHome {
		t.Fatalf("index categories should include home-template: %s", live.Body.String())
	}
}

func TestSourceRegisterRejectsCrossKindCategory(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	plugin := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"bad-plugin","name":"错分类","category":"home-template","downloadUrl":"https://cdn.example.com/x.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, plugin) != 400 || !strings.Contains(plugin.Body.String(), "分类") {
		t.Fatalf("plugin home-template should be rejected: %s", plugin.Body.String())
	}
	tpl := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"id":"bad-home","name":"错分类","category":"payment","templateUrl":"https://cdn.example.com/t.zip","sha256":"`+sha+`","schemaVersion":1}`)
	if sourceBodyCode(t, tpl) != 400 || !strings.Contains(tpl.Body.String(), "分类") {
		t.Fatalf("template payment should be rejected: %s", tpl.Body.String())
	}
}

func TestSourceTemplateDefaultsHomeTemplateCategory(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"id":"plain-home","name":"默认分类","templateUrl":"https://cdn.example.com/t.zip","sha256":"`+sha+`","schemaVersion":1}`)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"category":"home-template"`) {
		t.Fatalf("template should default to home-template: %s", rec.Body.String())
	}
}

func TestSourceCustomCategoryAppearsInFilterAndIndex(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/categories", admin,
		`{"extras":[{"key":"theme","label":"主题","kind":"plugin"}]}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save categories=%s", save.Body.String())
	}
	listed := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/categories", admin, "")
	if !strings.Contains(listed.Body.String(), `"theme"`) || !strings.Contains(listed.Body.String(), `"主题"`) {
		t.Fatalf("custom category missing: %s", listed.Body.String())
	}
	sha := sourceTestSHA256()
	sourceRegisterAndPublish(t, router, admin, "plugin", "theme-pack",
		`{"id":"theme-pack","name":"主题包","category":"theme","downloadUrl":"https://cdn.example.com/theme.zip","sha256":"`+sha+`"}`)
	filtered := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?category=theme", admin, "")
	if ids := sourceCatalogItemIDs(t, filtered.Body.Bytes()); len(ids) != 1 || ids[0] != "theme-pack" {
		t.Fatalf("theme filter=%v body=%s", ids, filtered.Body.String())
	}
	live := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if !strings.Contains(live.Body.String(), `"theme-pack"`) || !strings.Contains(live.Body.String(), `"category":"theme"`) {
		t.Fatalf("index should publish custom plugin category: %s", live.Body.String())
	}
	if strings.Contains(live.Body.String(), `"homeTemplates":[{"id":"theme-pack"`) {
		t.Fatalf("plugin category must not leak into homeTemplates: %s", live.Body.String())
	}
}

func TestSourcePackageParseUsesCategoryHintForKind(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	pluginZIP := sourcePluginTestZIP(t)
	wrong := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "demo-plugin.zip", pluginZIP, map[string]string{"category": "home-template"})
	if sourceBodyCode(t, wrong) != 400 {
		t.Fatalf("plugin zip + home-template should fail: %s", wrong.Body.String())
	}
	ok := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "demo-plugin.zip", pluginZIP, map[string]string{"category": "payment"})
	if sourceBodyCode(t, ok) != 200 || !strings.Contains(ok.Body.String(), `"category":"payment"`) {
		t.Fatalf("plugin zip + payment should accept and override category: %s", ok.Body.String())
	}
}

func sourceRegisterAndPublish(t *testing.T, router http.Handler, admin, kind, id, body string) {
	t.Helper()
	path := "/api/v1/source/admin/plugins"
	approve := "/api/v1/source/admin/plugins/" + id + "/approve"
	shelf := "/api/v1/source/admin/plugins/" + id + "/shelf"
	if kind == sourceKindTemplate {
		path = "/api/v1/source/admin/templates"
		approve = "/api/v1/source/admin/templates/" + id + "/approve"
		shelf = "/api/v1/source/admin/templates/" + id + "/shelf"
	}
	if rec := sourceJSON(t, router, http.MethodPut, path, admin, body); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register %s=%s", id, rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, approve, admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve %s=%s", id, rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, shelf, admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("shelf %s=%s", id, rec.Body.String())
	}
}

func sourceCatalogItemIDs(t *testing.T, payload []byte) []string {
	t.Helper()
	var body struct {
		Data struct {
			List []struct {
				ID       string `json:"id"`
				Kind     string `json:"kind"`
				Category string `json:"category"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("catalog items json: %v body=%s", err, payload)
	}
	ids := make([]string, 0, len(body.Data.List))
	for _, item := range body.Data.List {
		ids = append(ids, item.ID)
	}
	return ids
}
