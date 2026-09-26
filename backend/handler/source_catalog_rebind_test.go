package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestCatalogRebindKeepsVersionPriceAndMovesPublicIndex(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	created := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":1,"id":"move-plugin","name":"搬家插件","downloadUrl":"https://cdn.example.com/move.zip","sha256":"`+sha+`","priceCents":0,"shelf":true}`)
	if sourceBodyCode(t, created) != 200 {
		t.Fatalf("register=%s", created.Body.String())
	}
	store.mu.Lock()
	item := store.plugins["move-plugin"]
	item.PriceCents = 880
	item.DownloadURL = "paid:keep-me"
	item.SHA256 = sha
	store.plugins["move-plugin"] = item
	store.pluginVersions["move-plugin"] = map[string]sourceRelease{
		"1.0.0": {Kind: sourceKindPlugin, ItemID: "move-plugin", Version: "1.0.0", Location: "paid:keep-me", SHA256: sha},
	}
	store.mu.Unlock()

	moved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/catalog-items/rebind", admin,
		`{"appId":2,"items":[{"kind":"plugin","id":"move-plugin"}]}`)
	if sourceBodyCode(t, moved) != 200 || !strings.Contains(moved.Body.String(), "已切换绑定应用") {
		t.Fatalf("rebind=%s", moved.Body.String())
	}
	got, err := store.GetPlugin("move-plugin")
	if err != nil || got.AppID != 2 || got.PriceCents != 880 || got.DownloadURL != "paid:keep-me" || got.SHA256 != sha {
		t.Fatalf("plugin after rebind=%#v err=%v", got, err)
	}
	versions, err := store.ListVersions(sourceKindPlugin, "move-plugin")
	if err != nil || len(versions) != 1 || versions[0].Version != "1.0.0" || versions[0].Location != "paid:keep-me" {
		t.Fatalf("versions=%#v err=%v", versions, err)
	}
	appA := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	appB := sourceJSON(t, router, http.MethodGet, "/software-source/app-b/index.json", "", "")
	if strings.Contains(appA.Body.String(), "move-plugin") || !strings.Contains(appB.Body.String(), "move-plugin") {
		t.Fatalf("index A=%s B=%s", appA.Body.String(), appB.Body.String())
	}
	audit := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/audit", admin, "")
	if sourceBodyCode(t, audit) != 200 || !strings.Contains(audit.Body.String(), "rebind_app") || !strings.Contains(audit.Body.String(), "move-plugin") {
		t.Fatalf("audit=%s", audit.Body.String())
	}
}

func TestCatalogRebindRejectsDuplicateTemplateIdentity(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	store.mu.Lock()
	store.templates["home-a"] = sourceTemplate{ID: "home-a", TemplateKey: "shared-home", AppID: 1, Name: "甲", Status: sourceItemDraft}
	store.templates["home-b"] = sourceTemplate{ID: "home-b", TemplateKey: "shared-home", AppID: 2, Name: "乙", Status: sourceItemDraft}
	store.mu.Unlock()

	conflict := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/catalog-items/rebind", admin,
		`{"appId":1,"items":[{"kind":"template","id":"home-b"}]}`)
	if sourceBodyCode(t, conflict) != 400 || !strings.Contains(conflict.Body.String(), "目标应用里已经有标识「shared-home」") {
		t.Fatalf("conflict=%s", conflict.Body.String())
	}
	got, err := store.GetTemplate("home-b")
	if err != nil || got.AppID != 2 {
		t.Fatalf("template should stay on app 2: %#v err=%v", got, err)
	}
}

func TestCatalogUnassignedListsOrphansThenRebind(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	created := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":1,"id":"orphan-plugin","name":"孤儿","downloadUrl":"https://cdn.example.com/orphan.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, created) != 200 {
		t.Fatalf("register=%s", created.Body.String())
	}
	store.mu.Lock()
	item := store.plugins["orphan-plugin"]
	item.AppID = 99
	store.plugins["orphan-plugin"] = item
	store.mu.Unlock()

	orphans := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?unassigned=1", admin, "")
	ids := sourceCatalogItemIDs(t, orphans.Body.Bytes())
	if len(ids) != 1 || ids[0] != "orphan-plugin" {
		t.Fatalf("orphans=%v body=%s", ids, orphans.Body.String())
	}
	onA := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=1", admin, "")
	if ids := sourceCatalogItemIDs(t, onA.Body.Bytes()); len(ids) != 0 {
		t.Fatalf("app 1 should not list orphan: %v", ids)
	}
	usage := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-app-usage?app_id=1", admin, "")
	if sourceBodyCode(t, usage) != 200 || !strings.Contains(usage.Body.String(), `"count":0`) {
		t.Fatalf("live app 1 should not count the orphan: %s", usage.Body.String())
	}

	moved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/catalog-items/rebind", admin,
		`{"appId":2,"items":[{"kind":"plugin","id":"orphan-plugin"}]}`)
	if sourceBodyCode(t, moved) != 200 {
		t.Fatalf("rebind orphan=%s", moved.Body.String())
	}
	again := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?unassigned=1", admin, "")
	if ids := sourceCatalogItemIDs(t, again.Body.Bytes()); len(ids) != 0 {
		t.Fatalf("orphan remains: %v", ids)
	}
	onB := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/catalog-items?app_id=2", admin, "")
	if ids := sourceCatalogItemIDs(t, onB.Body.Bytes()); len(ids) != 1 || ids[0] != "orphan-plugin" {
		t.Fatalf("app 2=%v", ids)
	}
}

func TestDeveloperRebindStaysInsideOwnItemsAndKnownApps(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin, tokenA, idA := sourceApproveDeveloper(t, router, "rebind-a", "secret")
	_, tokenB, _ := sourceApproveDeveloper(t, router, "rebind-b", "secret")
	sha := sourceTestSHA256()
	saved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", tokenA,
		`{"appId":1,"id":"own-plugin","name":"自己的","version":"1.0.0","category":"other","downloadUrl":"https://cdn.example.com/own.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, saved) != 200 {
		t.Fatalf("save=%s", saved.Body.String())
	}
	foreign := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/items/rebind", tokenB,
		`{"appId":2,"items":[{"kind":"plugin","id":"own-plugin"}]}`)
	if sourceBodyCode(t, foreign) != 400 || !strings.Contains(foreign.Body.String(), "只能切换自己的条目") {
		t.Fatalf("foreign=%s", foreign.Body.String())
	}
	unknown := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/items/rebind", tokenA,
		`{"appId":99,"items":[{"kind":"plugin","id":"own-plugin"}]}`)
	if sourceBodyCode(t, unknown) != 400 || !strings.Contains(unknown.Body.String(), "没有该应用的权限") {
		t.Fatalf("unknown app=%s", unknown.Body.String())
	}
	moved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/items/rebind", tokenA,
		`{"appId":2,"items":[{"kind":"plugin","id":"own-plugin"}]}`)
	if sourceBodyCode(t, moved) != 200 {
		t.Fatalf("own rebind=%s", moved.Body.String())
	}
	got, err := store.GetPlugin("own-plugin")
	if err != nil || got.AppID != 2 || got.DeveloperID != idA {
		t.Fatalf("plugin=%#v err=%v", got, err)
	}
	_ = admin
}

func TestAppDeleteRequiresCatalogMigrate(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	router.DELETE("/api/app/:id", AppDelete)
	sha := sourceTestSHA256()
	created := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin,
		`{"appId":1,"id":"stay-plugin","name":"留下","downloadUrl":"https://cdn.example.com/stay.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, created) != 200 {
		t.Fatalf("register=%s", created.Body.String())
	}
	blocked := sourceJSON(t, router, http.MethodDelete, "/api/app/1", admin, "")
	if sourceBodyCode(t, blocked) != 409 || !strings.Contains(blocked.Body.String(), "请选择要迁移到的其他应用") {
		t.Fatalf("blocked=%s", blocked.Body.String())
	}
	got, err := store.GetPlugin("stay-plugin")
	if err != nil || got.AppID != 1 {
		t.Fatalf("plugin moved despite blocked delete: %#v err=%v", got, err)
	}
	same := sourceJSON(t, router, http.MethodDelete, "/api/app/1?migrateAppId=1", admin, "")
	if sourceBodyCode(t, same) != 400 || !strings.Contains(same.Body.String(), "不能迁移到正在删除的应用") {
		t.Fatalf("same app=%s", same.Body.String())
	}
	migrated := sourceJSON(t, router, http.MethodDelete, "/api/app/1?migrateAppId=2", admin, "")
	if sourceBodyCode(t, migrated) == 409 {
		t.Fatalf("migrate should leave the catalog gate: %s", migrated.Body.String())
	}
	got, err = store.GetPlugin("stay-plugin")
	if err != nil || got.AppID != 2 || got.DownloadURL != "https://cdn.example.com/stay.zip" {
		t.Fatalf("plugin after migrate=%#v err=%v body=%s", got, err, migrated.Body.String())
	}
}
