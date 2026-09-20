package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSourceStationIndexRequiresAppScope(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":1,"id":"alpha-pay","name":"A支付","category":"payment","downloadUrl":"https://cdn.example.com/a.zip","sha256":"`+sha+`","shelf":true}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register app-a=%s", rec.Body.String())
	}

	unscoped := sourceJSON(t, router, http.MethodGet, "/software-source/index.json", "", "")
	if unscoped.Code != http.StatusOK {
		t.Fatalf("unscoped status=%d body=%s", unscoped.Code, unscoped.Body.String())
	}
	if strings.Contains(unscoped.Body.String(), "alpha-pay") {
		t.Fatalf("unscoped index must not leak catalog items: %s", unscoped.Body.String())
	}
	if !strings.Contains(unscoped.Body.String(), `"plugins":[]`) {
		t.Fatalf("unscoped index must stay empty: %s", unscoped.Body.String())
	}
	compatUnscoped := sourceJSON(t, router, http.MethodGet, "/auth-pro/index.json", "", "")
	if compatUnscoped.Code != http.StatusOK || strings.Contains(compatUnscoped.Body.String(), "alpha-pay") {
		t.Fatalf("unscoped /auth-pro must not leak: %s", compatUnscoped.Body.String())
	}

	unknown := sourceJSON(t, router, http.MethodGet, "/software-source/missing-app/index.json", "", "")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown app_key status=%d body=%s", unknown.Code, unknown.Body.String())
	}
	if strings.Contains(unknown.Body.String(), "alpha-pay") {
		t.Fatalf("unknown app_key must not leak: %s", unknown.Body.String())
	}
}

func TestSourceStationAppACannotSeeAppBItems(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":1,"id":"app-a-plugin","name":"A插件","downloadUrl":"https://cdn.example.com/a-plugin.zip","sha256":"`+sha+`","shelf":true}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register A plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"appId":1,"id":"app-a-home","name":"A首页","templateUrl":"https://cdn.example.com/a-home.json","sha256":"`+sha+`","schemaVersion":1,"shelf":true}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register A template=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":2,"id":"app-b-plugin","name":"B插件","downloadUrl":"https://cdn.example.com/b-plugin.zip","sha256":"`+sha+`","shelf":true}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register B plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates", admin,
		`{"appId":2,"id":"app-b-home","name":"B首页","templateUrl":"https://cdn.example.com/b-home.json","sha256":"`+sha+`","schemaVersion":1,"shelf":true}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register B template=%s", rec.Body.String())
	}

	for _, path := range []string{
		"/software-source/app-a/index.json",
		"/software-source/index.json?app_key=app-a",
		"/auth-pro/app-a/index.json",
		"/auth-pro/index.json?app_key=app-a",
	} {
		index := sourceJSON(t, router, http.MethodGet, path, "", "")
		if index.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, index.Code, index.Body.String())
		}
		body := index.Body.String()
		if !strings.Contains(body, `"app-a-plugin"`) || !strings.Contains(body, `"app-a-home"`) {
			t.Fatalf("%s missing App A items: %s", path, body)
		}
		if strings.Contains(body, "app-b-plugin") || strings.Contains(body, "app-b-home") {
			t.Fatalf("%s leaked App B items: %s", path, body)
		}
		var manifest struct {
			AppKey        string           `json:"appKey"`
			AppID         int64            `json:"appId"`
			Plugins       []map[string]any `json:"plugins"`
			HomeTemplates []map[string]any `json:"homeTemplates"`
		}
		if err := json.Unmarshal(index.Body.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.AppKey != "app-a" || manifest.AppID != 1 {
			t.Fatalf("%s app identity=%+v body=%s", path, manifest, body)
		}
		if len(manifest.Plugins) != 1 || len(manifest.HomeTemplates) != 1 {
			t.Fatalf("%s want 1 plugin + 1 template, got %+v", path, manifest)
		}
	}

	bIndex := sourceJSON(t, router, http.MethodGet, "/software-source/app-b/index.json", "", "")
	if bIndex.Code != http.StatusOK || !strings.Contains(bIndex.Body.String(), `"app-b-plugin"`) {
		t.Fatalf("app-b index=%s", bIndex.Body.String())
	}
	if strings.Contains(bIndex.Body.String(), "app-a-plugin") || strings.Contains(bIndex.Body.String(), "app-a-home") {
		t.Fatalf("app-b leaked App A: %s", bIndex.Body.String())
	}
}

func TestSourceAdminUploadAndListHonorAppID(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()

	missing := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"id":"no-app","name":"缺应用","downloadUrl":"https://cdn.example.com/x.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, missing) != 400 || !strings.Contains(missing.Body.String(), "应用") {
		t.Fatalf("missing appId=%s", missing.Body.String())
	}

	unknown := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":99,"id":"ghost-app","name":"不存在的应用","downloadUrl":"https://cdn.example.com/x.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, unknown) != 400 {
		t.Fatalf("unknown appId=%s", unknown.Body.String())
	}

	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":1,"id":"listed-a","name":"A目录","downloadUrl":"https://cdn.example.com/listed-a.zip","sha256":"`+sha+`"}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register A=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":2,"id":"listed-b","name":"B目录","downloadUrl":"https://cdn.example.com/listed-b.zip","sha256":"`+sha+`"}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("register B=%s", rec.Body.String())
	}

	all := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items", admin, "")
	if ids := sourceCatalogItemIDs(t, all.Body.Bytes()); len(ids) != 2 {
		t.Fatalf("unfiltered catalog=%v body=%s", ids, all.Body.String())
	}
	onlyA := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=1", admin, "")
	if ids := sourceCatalogItemIDs(t, onlyA.Body.Bytes()); len(ids) != 1 || ids[0] != "listed-a" {
		t.Fatalf("app_id=1 filter=%v body=%s", ids, onlyA.Body.String())
	}
	onlyB := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=2", admin, "")
	if ids := sourceCatalogItemIDs(t, onlyB.Body.Bytes()); len(ids) != 1 || ids[0] != "listed-b" {
		t.Fatalf("app_id=2 filter=%v body=%s", ids, onlyB.Body.String())
	}

	plugin, err := store.GetPlugin("listed-a")
	if err != nil || plugin.AppID != 1 {
		t.Fatalf("stored app_id=%+v err=%v", plugin, err)
	}
}

func TestSourcePackagePublishHonorsAppID(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := sourcePluginTestZIP(t)

	rejected := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "demo-plugin.zip", payload, map[string]string{
		"kind": "plugin", "push": "0", "shelf": "1",
		"downloadUrl": "https://cdn.example.com/demo-plugin-1.0.0.zip",
	})
	if sourceBodyCode(t, rejected) != 400 || !strings.Contains(rejected.Body.String(), "应用") {
		t.Fatalf("publish without appId=%s", rejected.Body.String())
	}

	published := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "demo-plugin.zip", payload, map[string]string{
		"kind": "plugin", "push": "0", "shelf": "1", "appId": "2",
		"downloadUrl": "https://cdn.example.com/demo-plugin-1.0.0.zip",
	})
	if sourceBodyCode(t, published) != 200 {
		t.Fatalf("publish=%s", published.Body.String())
	}
	plugin, err := store.GetPlugin("demo-plugin")
	if err != nil || plugin.AppID != 2 || plugin.Status != sourceItemPublished {
		t.Fatalf("published plugin=%+v err=%v", plugin, err)
	}

	appA := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(appA.Body.String(), "demo-plugin") {
		t.Fatalf("App A index leaked package for App B: %s", appA.Body.String())
	}
	appB := sourceJSON(t, router, http.MethodGet, "/software-source/app-b/index.json", "", "")
	if !strings.Contains(appB.Body.String(), `"demo-plugin"`) {
		t.Fatalf("App B index missing published package: %s", appB.Body.String())
	}
}

func TestSourceDeveloperSubmitIsAppScoped(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "dev-scoped", "secret1")
	sha := sourceTestSHA256()

	missing := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev,
		`{"id":"dev-plugin","name":"开发者插件","downloadUrl":"https://cdn.example.com/dev.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, missing) != 400 {
		t.Fatalf("developer missing appId=%s", missing.Body.String())
	}

	save := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev,
		`{"appId":2,"id":"dev-plugin","name":"开发者插件","downloadUrl":"https://cdn.example.com/dev.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("developer save=%s", save.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/dev-plugin/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/dev-plugin/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/dev-plugin/shelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("shelf=%s", rec.Body.String())
	}

	devApps := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/apps", dev, "")
	if sourceBodyCode(t, devApps) != 200 || !strings.Contains(devApps.Body.String(), `"appKey":"app-b"`) {
		t.Fatalf("developer apps=%s", devApps.Body.String())
	}

	items := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/items", dev, "")
	if !strings.Contains(items.Body.String(), `"appId":2`) || !strings.Contains(items.Body.String(), `"dev-plugin"`) {
		t.Fatalf("developer items=%s", items.Body.String())
	}

	reviewA := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=1&status=published", admin, "")
	if ids := sourceCatalogItemIDs(t, reviewA.Body.Bytes()); len(ids) != 0 {
		t.Fatalf("App A review list leaked developer item: %v %s", ids, reviewA.Body.String())
	}
	appB := sourceJSON(t, router, http.MethodGet, "/software-source/app-b/index.json", "", "")
	if !strings.Contains(appB.Body.String(), `"dev-plugin"`) {
		t.Fatalf("App B index missing developer plugin: %s", appB.Body.String())
	}
	appA := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(appA.Body.String(), "dev-plugin") {
		t.Fatalf("App A index leaked developer plugin: %s", appA.Body.String())
	}
}

func TestSourceAdminAppsList(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	rec := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/apps", admin, "")
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("apps=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"appKey":"app-a"`) || !strings.Contains(rec.Body.String(), `"appKey":"app-b"`) {
		t.Fatalf("expected seeded catalog apps: %s", rec.Body.String())
	}
}
