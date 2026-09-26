package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminSwitchesPublicFreePluginToPaidAndBack(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := sourcePluginTestZIP(t)
	publicURL, sha, err := storeStationPackage(payload)
	if err != nil {
		t.Fatal(err)
	}
	sourceRegisterAndPublish(t, router, admin, "plugin", "switch-plugin",
		`{"appId":1,"id":"switch-plugin","name":"可改价插件","category":"other","version":"1.0.0","downloadUrl":"`+publicURL+`","sha256":"`+sha+`"}`)

	denied := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/switch-plugin", admin,
		`{"name":"可改价插件","category":"other","version":"1.0.0","downloadUrl":"`+publicURL+`","sha256":"`+sha+`","priceCents":1990}`)
	if sourceBodyCode(t, denied) != 400 || !strings.Contains(denied.Body.String(), "已下载过的老用户继续免费") {
		t.Fatalf("missing policy: %s", denied.Body.String())
	}

	paid := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/switch-plugin", admin,
		`{"name":"可改价插件","category":"other","version":"1.0.0","downloadUrl":"`+publicURL+`","sha256":"`+sha+`","priceCents":1990,"priceSwitch":"grandfather"}`)
	if sourceBodyCode(t, paid) != 200 {
		t.Fatalf("switch to paid: %s", paid.Body.String())
	}
	item, err := store.GetPlugin("switch-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if item.PriceCents != 1990 || item.Status != sourceItemPublished || !isPrivatePackageRef(item.DownloadURL) {
		t.Fatalf("paid item=%#v", item)
	}
	if _, statErr := os.Stat(stationPackagePath(publicURL)); !os.IsNotExist(statErr) {
		t.Fatalf("old public file still present: %v", statErr)
	}
	if got := sourceJSON(t, router, http.MethodGet, publicURL, "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("old link status=%d", got.Code)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(index.Body.String(), publicURL) || strings.Contains(index.Body.String(), item.DownloadURL) || strings.Contains(index.Body.String(), sha) {
		t.Fatalf("public index still exposes the package: %s", index.Body.String())
	}
	if !strings.Contains(index.Body.String(), `"priceCents":1990`) {
		t.Fatalf("index lost price: %s", index.Body.String())
	}

	free := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/switch-plugin", admin,
		`{"name":"可改价插件","category":"other","version":"1.0.0","downloadUrl":"`+item.DownloadURL+`","sha256":"`+item.SHA256+`","priceCents":0}`)
	if sourceBodyCode(t, free) != 200 {
		t.Fatalf("switch back: %s", free.Body.String())
	}
	again, err := store.GetPlugin("switch-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if again.PriceCents != 0 || !isStationHostedPackageURL(again.DownloadURL) {
		t.Fatalf("free item=%#v old=%s", again, publicURL)
	}
	if got := sourceJSON(t, router, http.MethodGet, again.DownloadURL, "", ""); got.Code != http.StatusOK {
		t.Fatalf("restored link status=%d", got.Code)
	}
	audits, err := store.ListAudit(20)
	if err != nil {
		t.Fatal(err)
	}
	var toPaid, toFree bool
	for _, entry := range audits {
		if entry.TargetID != "switch-plugin" {
			continue
		}
		if entry.Action == "price_to_paid" && strings.Contains(entry.Detail, "老用户继续免费") {
			toPaid = true
		}
		if entry.Action == "price_to_free" {
			toFree = true
		}
	}
	if !toPaid || !toFree {
		t.Fatalf("audits=%+v", audits)
	}
}

func TestExternalFreePluginIsHostedWhenSwitchingToPaid(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := sourcePluginTestZIP(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	useExternalPackageClientForTest(t, server.Client())
	rawURL := pinHostToServer(t, "switch.example.com", server, server)
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sha256Hex(payload)
	sourceRegisterAndPublish(t, router, admin, "plugin", "ext-switch",
		`{"appId":1,"id":"ext-switch","name":"外链插件","category":"other","version":"1.0.0","downloadUrl":"`+rawURL+`","sha256":"`+sha+`"}`)
	paid := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/ext-switch", admin,
		`{"name":"外链插件","category":"other","version":"1.0.0","downloadUrl":"`+rawURL+`","sha256":"`+sha+`","priceCents":800,"priceSwitch":"purchase_only"}`)
	if sourceBodyCode(t, paid) != 200 {
		t.Fatalf("external switch: %s", paid.Body.String())
	}
	item, err := store.GetPlugin("ext-switch")
	if err != nil {
		t.Fatal(err)
	}
	if !isPrivatePackageRef(item.DownloadURL) || item.OriginURL != rawURL || item.PriceCents != 800 {
		t.Fatalf("hosted item=%#v", item)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(index.Body.String(), rawURL) || strings.Contains(index.Body.String(), "originUrl") {
		t.Fatalf("index leaked origin: %s", index.Body.String())
	}
	audits, err := store.ListAudit(20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range audits {
		if entry.Action == "price_to_paid" && entry.TargetID == "ext-switch" && strings.Contains(entry.Detail, "所有人都需购买") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing purchase-only audit: %+v", audits)
	}
}

func stationPackagePath(publicURL string) string {
	name, _ := stationPackageNameFromURL(publicURL)
	return filepath.Join(stationPackageDir(), name)
}
