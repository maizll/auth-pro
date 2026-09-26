package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCatalogRestoreDeprecatedItemToDraft(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()

	sourceRegisterAndPublish(t, router, admin, sourceKindPlugin, "back-plugin",
		`{"appId":1,"id":"back-plugin","name":"可恢复插件","category":"other","downloadUrl":"https://cdn.example.com/back-plugin.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, sourceKindTemplate, "back-home",
		`{"appId":1,"id":"back-home","name":"可恢复首页","templateUrl":"https://cdn.example.com/back-home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/back-plugin/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/back-home/deprecate", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("deprecate template=%s", rec.Body.String())
	}
	assertPublicIndexIDs(t, router, "app-a", nil, nil)

	blocked := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/back-plugin/shelf", admin, "{}")
	if sourceBodyCode(t, blocked) == 200 {
		t.Fatal("deprecated item must not shelf directly")
	}

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/back-plugin/restore", admin, "{}"); sourceBodyCode(t, rec) != 200 || !strings.Contains(rec.Body.String(), "草稿") {
		t.Fatalf("restore plugin=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/templates/back-home/restore", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("restore template=%s", rec.Body.String())
	}

	plugin, err := store.GetPlugin("back-plugin")
	if err != nil || plugin.Status != sourceItemDraft {
		t.Fatalf("plugin status=%s err=%v", plugin.Status, err)
	}
	tpl, err := store.GetTemplate("back-home")
	if err != nil || tpl.Status != sourceItemDraft {
		t.Fatalf("template status=%s err=%v", tpl.Status, err)
	}
	assertPublicIndexIDs(t, router, "app-a", nil, nil)

	audit := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/audit", admin, "")
	if sourceBodyCode(t, audit) != 200 || !strings.Contains(audit.Body.String(), `"action":"restore"`) {
		t.Fatalf("audit=%s", audit.Body.String())
	}

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/back-plugin/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve after restore=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/back-plugin/shelf", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("shelf after restore=%s", rec.Body.String())
	}
	assertPublicIndexIDs(t, router, "app-a", []string{"back-plugin"}, nil)

	again := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/back-plugin/restore", admin, "{}")
	if sourceBodyCode(t, again) == 200 {
		t.Fatal("published item must not restore")
	}
}

func TestLicenseDownloadAllowedRequiresActiveLicense(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		commercial  bool
		developerID int64
		entitlement bool
		want        bool
	}{
		{name: "revoked with entitlement", status: "revoked", commercial: false, developerID: 3, entitlement: true, want: false},
		{name: "disabled with commercial", status: "disabled", commercial: true, developerID: 0, entitlement: false, want: false},
		{name: "active entitlement", status: "active", commercial: false, developerID: 3, entitlement: true, want: true},
		{name: "active commercial site item", status: "active", commercial: true, developerID: 0, entitlement: false, want: true},
		{name: "active commercial developer item", status: "active", commercial: true, developerID: 3, entitlement: false, want: false},
		{name: "active without grant", status: "active", commercial: false, developerID: 0, entitlement: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := licenseDownloadAllowed(tc.status, tc.commercial, tc.developerID, tc.entitlement); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestLocalPaidHealthMarksMissingFile(t *testing.T) {
	_, store := sourceStationRouter(t)
	name := strings.Repeat("e1", 16) + ".zip"
	location := "paid:" + name
	store.mu.Lock()
	store.plugins["local-paid"] = sourcePlugin{
		ID: "local-paid", Name: "本地收费", PriceCents: 100, Status: sourceItemPublished,
		DownloadURL: location, OriginHealth: paidOriginHealthOK,
	}
	store.plugins["free-local"] = sourcePlugin{
		ID: "free-local", Name: "免费", PriceCents: 0, Status: sourceItemPublished,
		DownloadURL: location, OriginHealth: "",
	}
	store.mu.Unlock()

	touchLocalPaidHealth(context.Background(), store, sourceKindPlugin, "local-paid", "本地收费", 100, location, paidOriginHealthOK)
	got, err := store.GetPlugin("local-paid")
	if err != nil || got.OriginHealth != paidOriginHealthUnavailable || got.Status != sourceItemPublished {
		t.Fatalf("missing local package: %#v err=%v", got, err)
	}
	free, err := store.GetPlugin("free-local")
	if err != nil || free.OriginHealth != "" {
		t.Fatalf("free item must stay unmarked: %#v err=%v", free, err)
	}
	touchLocalPaidHealth(context.Background(), store, sourceKindPlugin, "free-local", "免费", 0, location, "")
	free, _ = store.GetPlugin("free-local")
	if free.OriginHealth != "" {
		t.Fatalf("zero price must not flag health: %#v", free)
	}
}
