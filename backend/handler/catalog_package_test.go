package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSourcePublicEntriesUseOfficialPackageURL(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	free := sourcePublicPluginEntry(sourcePlugin{
		ID: "demo-plugin", Name: "示例", Version: "1.2.0", PriceCents: 0,
		DownloadURL: "https://github.com/acme/auth-pro-paid/releases/download/v1/demo.zip",
		SHA256:      sha, Status: sourceItemPublished,
	})
	raw := catalogMustJSON(t, free)
	if strings.Contains(raw, "github.com") || strings.Contains(raw, "auth-pro-paid") || strings.Contains(raw, "demo.zip") {
		t.Fatalf("public plugin leaked upstream: %s", raw)
	}
	if free["downloadUrl"] != catalogBuyerPackageURL("plugin", "demo-plugin") || free["sha256"] != sha || free["version"] != "1.2.0" {
		t.Fatalf("public plugin=%#v", free)
	}
	paid := sourcePublicPluginEntry(sourcePlugin{
		ID: "paid-plugin", PriceCents: 1990, SHA256: sha,
		DownloadURL: "github:acme/auth-pro-paid/paid-1/plugin.zip", Status: sourceItemPublished,
	})
	if _, ok := paid["downloadUrl"]; ok {
		t.Fatalf("paid plugin leaked downloadUrl: %#v", paid)
	}
	home := sourcePublicTemplateEntry(sourceTemplate{
		ID: "home", TemplateKey: "clean-home", Version: "1.0.0", SchemaVersion: 1, PriceCents: 0,
		TemplateURL: "https://cdn.example.com/templates/clean-home.json", SHA256: sha,
	})
	homeRaw := catalogMustJSON(t, home)
	if strings.Contains(homeRaw, "cdn.example.com") || home["templateUrl"] != catalogBuyerPackageURL("template", "clean-home") {
		t.Fatalf("public template=%s", homeRaw)
	}
}

func TestCatalogRepoHostBlocked(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/acme/auth-pro-paid/releases/download/v1/a.zip",
		"https://api.github.com/repos/acme/auth-pro-paid/releases/latest",
		"https://release-assets.githubusercontent.com/a.zip",
		"github:acme/auth-pro-paid/v1/a.zip",
	} {
		if !catalogRepoHostBlocked(raw) {
			t.Fatalf("expected blocked: %s", raw)
		}
	}
	if catalogRepoHostBlocked("https://auth.maizll.com/api/v1/catalog/package/plugin/demo") {
		t.Fatal("official package url must stay reachable")
	}
}

func TestDownloadPluginPackageRejectsGitHubHost(t *testing.T) {
	_, err := downloadPluginPackage(context.Background(), "https://github.com/acme/auth-pro-paid/releases/download/v1/a.zip", strings.Repeat("cd", 32), true)
	if err == nil || !strings.Contains(err.Error(), catalogPackageHostText) || strings.Contains(err.Error(), "auth-pro-paid") {
		t.Fatalf("err=%v", err)
	}
}

func TestApplyCatalogPluginUpdatesUsesCatalogVersion(t *testing.T) {
	local := []pluginInfo{{ID: "demo-plugin", Version: "1.0.0", Local: true}}
	official := catalogBuyerPackageURL("plugin", "demo-plugin")
	indexes := []*remotePluginIndex{{Plugins: []remotePluginEntry{{
		ID: "demo-plugin", Version: "1.0.1", DownloadURL: official,
	}}}}
	applyCatalogPluginUpdates(local, indexes)
	if !local[0].UpdateAvailable || local[0].DownloadURL != official {
		t.Fatalf("update=%#v", local[0])
	}
	local[0].Version = "1.0.1"
	local[0].UpdateAvailable = false
	local[0].DownloadURL = ""
	applyCatalogPluginUpdates(local, indexes)
	if local[0].UpdateAvailable {
		t.Fatal("same version must not offer an update")
	}
	builtin := []pluginInfo{{ID: "epay", Version: "1.0.0"}}
	applyCatalogPluginUpdates(builtin, []*remotePluginIndex{{Plugins: []remotePluginEntry{{
		ID: "epay", Version: "9.0.0", DownloadURL: official,
	}}}})
	if builtin[0].UpdateAvailable {
		t.Fatal("compiled plugin must not take a catalog zip")
	}
}

func TestCatalogPackageDownloadServesHostedZip(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := sourcePluginTestZIP(t)
	publicURL, fileSHA, err := storeStationPackage(payload)
	if err != nil {
		t.Fatal(err)
	}
	router, store := sourceStationRouter(t)
	store.mu.Lock()
	store.plugins["demo-plugin"] = sourcePlugin{
		ID: "demo-plugin", AppID: 1, Category: "other", Name: "示例插件", Version: "1.0.0",
		DownloadURL: publicURL, SHA256: fileSHA, Status: sourceItemPublished, PriceCents: 0,
	}
	store.mu.Unlock()
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(index.Body.String(), catalogBuyerPackageURL("plugin", "demo-plugin")) || strings.Contains(index.Body.String(), publicURL) {
		t.Fatalf("index=%s", index.Body.String())
	}
	got := sourceJSON(t, router, http.MethodGet, "/api/v1/catalog/package/plugin/demo-plugin", "", "")
	if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), payload) {
		t.Fatalf("download status=%d len=%d", got.Code, got.Body.Len())
	}
	if got.Header().Get("Location") != "" {
		t.Fatalf("download must not redirect: %s", got.Header().Get("Location"))
	}
	missing := sourceJSON(t, router, http.MethodGet, "/api/v1/catalog/package/plugin/missing-plugin", "", "")
	if missing.Code != http.StatusNotFound || strings.Contains(missing.Body.String(), publicURL) {
		t.Fatalf("missing=%d %s", missing.Code, missing.Body.String())
	}
}

func catalogMustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
