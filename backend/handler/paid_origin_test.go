package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestImportPaidPackageFromURLRejectsUnsafeTargets(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	ctx := context.Background()
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"http", "http://cdn.example.com/plugin.zip", "来源外链必须是 https:// 地址"},
		{"loopback", "https://127.0.0.1/plugin.zip", "拒绝访问非公网地址"},
		{"private", "https://10.1.2.3/plugin.zip", "拒绝访问非公网地址"},
		{"metadata", "https://169.254.169.254/latest/meta-data", "拒绝访问链路本地或云元数据地址"},
		{"metadata host", "https://metadata.google.internal/computeMetadata/v1/", "拒绝访问链路本地或云元数据地址"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := importPaidPackageFromURL(ctx, sourceKindPlugin, "other", tc.url)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v", err)
			}
			matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
			if len(matches) != 0 {
				t.Fatalf("rejected import wrote %v", matches)
			}
		})
	}
}

func TestImportPaidPackageFromURLRejectsOversizedZIP(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 32*1024)
		buf[0], buf[1], buf[2], buf[3] = 'P', 'K', 3, 4
		remain := int64(pluginPackageMaxSize) + 1
		for remain > 0 {
			n := int64(len(buf))
			if n > remain {
				n = remain
			}
			if _, err := w.Write(buf[:n]); err != nil {
				return
			}
			remain -= n
		}
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "huge.example.com", server, server)
	_, err := importPaidPackageFromURL(context.Background(), sourceKindPlugin, "other", rawURL)
	if err == nil || !strings.Contains(err.Error(), "超过 20 MiB") {
		t.Fatalf("err=%v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 0 {
		t.Fatalf("oversized import wrote %v", matches)
	}
}

func TestImportPaidPackageFromURLRejectsRedirectToPrivate(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	secretHits := 0
	secret := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHits++
		_, _ = w.Write([]byte("PK\x03\x04secret"))
	}))
	t.Cleanup(secret.Close)
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, secret.URL+"/pkg.zip", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	rawURL := pinHostToServer(t, "redir.example.com", redirector, secret)
	_, err := importPaidPackageFromURL(context.Background(), sourceKindPlugin, "other", rawURL)
	if err == nil || !errors.Is(err, errSafePrivateAddress) {
		t.Fatalf("err=%v", err)
	}
	if secretHits != 0 {
		t.Fatalf("private redirect target was read %d times", secretHits)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 0 {
		t.Fatalf("redirect import wrote %v", matches)
	}
}

func TestImportPaidPackageFromURLRejectsTooManyRedirects(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "loop.example.com", server, server)
	_, err := importPaidPackageFromURL(context.Background(), sourceKindPlugin, "other", rawURL)
	if err == nil || !strings.Contains(err.Error(), "重定向过多") {
		t.Fatalf("err=%v", err)
	}
}

func TestImportPaidPackageFromURLRejectsInvalidZIP(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := makeTestZIP(t, testZIPEntry{name: "readme.txt", data: "no manifest"})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "badzip.example.com", server, server)
	_, err := importPaidPackageFromURL(context.Background(), sourceKindPlugin, "other", rawURL)
	if err == nil || !strings.Contains(err.Error(), "plugin.json") {
		t.Fatalf("err=%v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 0 {
		t.Fatalf("invalid zip was stored: %v", matches)
	}
}

func TestImportPaidPackageFromURLStoresPrivateCopy(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{"kind":"template","id":"clean-home","name":"清新首页","version":"1.4.0","schemaVersion":1,
		"description":"模板","author":"设计组","hero":{"title":"欢迎"}}`})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "tpl.example.com", server, server)
	got, err := importPaidPackageFromURL(context.Background(), sourceKindTemplate, "home-template", rawURL)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	if got.SHA256 != want || got.Version != "1.4.0" || got.Origin != rawURL || !isPrivatePackageRef(got.Ref) {
		t.Fatalf("import=%#v want sha %s", got, want)
	}
	name, _ := privatePackageName(got.Ref)
	stored, err := os.ReadFile(filepath.Join(stationPaidPackageDirPath(), name))
	if err != nil || string(stored) != string(payload) {
		t.Fatalf("stored bytes err=%v", err)
	}
}

func TestRefetchPaidPackageKeepsOldOnFailure(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	_, store := sourceStationRouter(t)
	var fail atomic.Bool
	first := makeTestZIP(t, testZIPEntry{name: "demo-plugin/plugin.json", data: `{
		"id":"paid-keep","name":"付费插件","version":"1.0.0","description":"第一版",
		"author":{"name":"源站"},"category":"other"}`})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "gone", http.StatusBadGateway)
			return
		}
		_, _ = w.Write(first)
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "keep.example.com", server, server)
	imported, err := importPaidPackageFromURL(context.Background(), sourceKindPlugin, "other", rawURL)
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.plugins["paid-keep"] = sourcePlugin{
		ID: "paid-keep", AppID: 1, Category: "other", Name: "付费插件", Version: "1.0.0",
		SHA256: imported.SHA256, DownloadURL: imported.Ref, OriginURL: rawURL, OriginHealth: paidOriginHealthOK,
		PriceCents: 1990, Billing: sourceBillingOneTime, Delivery: sourceDeliveryZip, Status: sourceItemPublished,
	}
	store.mu.Unlock()
	fail.Store(true)
	if err := refetchPaidCatalogPackage(context.Background(), sourceKindPlugin, "paid-keep"); err == nil || !strings.Contains(err.Error(), "状态码") {
		t.Fatalf("refetch err=%v", err)
	}
	item, err := store.GetPlugin("paid-keep")
	if err != nil {
		t.Fatal(err)
	}
	if item.SHA256 != imported.SHA256 || item.DownloadURL != imported.Ref || item.Version != "1.0.0" || item.Status != sourceItemPublished {
		t.Fatalf("old package was replaced: %#v", item)
	}
	name, _ := privatePackageName(imported.Ref)
	stored, err := os.ReadFile(filepath.Join(stationPaidPackageDirPath(), name))
	if err != nil || sha256Hex(stored) != imported.SHA256 {
		t.Fatalf("old file changed err=%v", err)
	}
}

func TestPaidHTTPSImportHiddenFromBuyers(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	router, store := sourceStationRouter(t)
	_, dev, _ := sourceApproveDeveloper(t, router, "paid-url", "secret")
	var body atomic.Value
	first := makeTestZIP(t, testZIPEntry{name: "paid-remote/plugin.json", data: `{
		"id":"paid-remote","name":"远程付费","version":"1.2.0","description":"外链托管",
		"author":{"name":"源站"},"category":"other"}`})
	next := makeTestZIP(t, testZIPEntry{name: "paid-remote/plugin.json", data: `{
		"id":"paid-remote","name":"远程付费","version":"1.3.0","description":"外链托管",
		"author":{"name":"源站"},"category":"other"}`})
	body.Store(first)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := body.Load().([]byte)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	rawURL := pinHostToServer(t, "paid-origin.example.com", server, server)
	originHost := "paid-origin.example.com"
	req := `{"appId":1,"id":"paid-remote","name":"远程付费","version":"9.9.9","description":"外链托管","category":"other","priceCents":2500,"downloadUrl":"` + rawURL + `"}`
	saved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, req)
	if sourceBodyCode(t, saved) != 200 {
		t.Fatalf("save=%s", saved.Body.String())
	}
	var created struct {
		Data struct {
			DownloadURL  string `json:"downloadUrl"`
			SHA256       string `json:"sha256"`
			Version      string `json:"version"`
			OriginURL    string `json:"originUrl"`
			OriginHealth string `json:"originHealth"`
		} `json:"data"`
	}
	if err := json.Unmarshal(saved.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	wantSHA := sha256Hex(first)
	stored, err := store.GetPlugin("paid-remote")
	if err != nil {
		t.Fatal(err)
	}
	if created.Data.SHA256 != wantSHA || created.Data.Version != "1.2.0" || created.Data.OriginURL != rawURL || created.Data.DownloadURL != "" || !strings.Contains(saved.Body.String(), `"storedBySite":true`) || !isPrivatePackageRef(stored.DownloadURL) {
		t.Fatalf("created=%+v stored=%s sha=%s", created.Data, stored.DownloadURL, wantSHA)
	}
	if created.Data.OriginHealth != paidOriginHealthOK {
		t.Fatalf("health=%s", created.Data.OriginHealth)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(index.Body.String(), originHost) || strings.Contains(index.Body.String(), "originUrl") || strings.Contains(index.Body.String(), stored.DownloadURL) {
		t.Fatalf("public index leaked origin or private ref: %s", index.Body.String())
	}
	items := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/items", dev, "")
	if !strings.Contains(items.Body.String(), originHost) || !strings.Contains(items.Body.String(), `"originUrl"`) {
		t.Fatalf("developer view hid origin: %s", items.Body.String())
	}
	body.Store(next)
	pulled := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/paid-remote/pull", dev, "")
	if sourceBodyCode(t, pulled) != 200 || !strings.Contains(pulled.Body.String(), sha256Hex(next)) || !strings.Contains(pulled.Body.String(), `"version":"1.3.0"`) {
		t.Fatalf("pull=%s", pulled.Body.String())
	}
	afterPull, err := store.GetPlugin("paid-remote")
	if err != nil || afterPull.DownloadURL == stored.DownloadURL || afterPull.SHA256 != sha256Hex(next) {
		t.Fatalf("pull kept old package ref: %#v err=%v", afterPull, err)
	}
	if strings.Contains(pulled.Body.String(), stored.DownloadURL) || strings.Contains(pulled.Body.String(), afterPull.DownloadURL) {
		t.Fatalf("pull showed storage ref: %s", pulled.Body.String())
	}
	again := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(again.Body.String(), originHost) {
		t.Fatalf("index leaked origin after pull: %s", again.Body.String())
	}
}

func TestPaidOriginHealthDoesNotUnpublish(t *testing.T) {
	router, store := sourceStationRouter(t)
	store.mu.Lock()
	store.plugins["paid-one"] = sourcePlugin{
		ID: "paid-one", AppID: 1, Category: "other", Name: "付费插件", Description: "仍在售",
		Version: "1.0.0", SHA256: strings.Repeat("ab", 32),
		DownloadURL: "paid:" + strings.Repeat("ab", 16) + ".zip",
		OriginURL:   "https://127.0.0.1/secret.zip", OriginHealth: paidOriginHealthOK,
		Status: sourceItemPublished, PriceCents: 1990, Billing: sourceBillingOneTime, Delivery: sourceDeliveryZip,
	}
	store.templates["free-home"] = sourceTemplate{
		ID: "free-home", TemplateKey: "free-home", AppID: 1, Category: sourceCategoryHomeTemplate,
		Name: "免费模板", Version: "1.0.0", SchemaVersion: 1,
		TemplateURL: "https://cdn.example.com/free.zip", SHA256: strings.Repeat("cd", 32),
		Status: sourceItemPublished, PriceCents: 0, Billing: sourceBillingFree,
	}
	store.mu.Unlock()
	checkPaidOriginHealth(context.Background())
	item, err := store.GetPlugin("paid-one")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != sourceItemPublished || item.OriginHealth != paidOriginHealthUnavailable || item.DownloadURL == "" {
		t.Fatalf("health check changed the hosted item: %#v", item)
	}
	free, err := store.GetTemplate("free-home")
	if err != nil || free.Status != sourceItemPublished || free.TemplateURL != "https://cdn.example.com/free.zip" || free.OriginHealth != "" {
		t.Fatalf("free external changed: %#v err=%v", free, err)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(index.Body.String(), "付费插件") || strings.Contains(index.Body.String(), "127.0.0.1") || strings.Contains(index.Body.String(), "originUrl") {
		t.Fatalf("buyer index=%s", index.Body.String())
	}
	if !strings.Contains(index.Body.String(), "https://cdn.example.com/free.zip") {
		t.Fatalf("free external url removed: %s", index.Body.String())
	}
	view := sourcePluginView(item)
	raw, _ := json.Marshal(view)
	if !strings.Contains(string(raw), paidOriginUnavailableHint) || !strings.Contains(string(raw), "127.0.0.1") {
		t.Fatalf("developer hint missing: %s", raw)
	}
	public := sourcePublicPluginEntry(item)
	publicRaw, _ := json.Marshal(public)
	if strings.Contains(string(publicRaw), "127.0.0.1") || strings.Contains(string(publicRaw), "originUrl") || strings.Contains(string(publicRaw), "downloadUrl") {
		t.Fatalf("public entry leaked: %s", publicRaw)
	}
}

func TestPaidImportDoesNotReadBodyOnPrivateDial(t *testing.T) {
	hits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("PK\x03\x04"))
	}))
	t.Cleanup(server.Close)
	_, err := importPaidPackageFromURL(context.Background(), sourceKindPlugin, "other", server.URL+"/plugin.zip")
	if err == nil || !errors.Is(err, errSafePrivateAddress) || hits != 0 {
		t.Fatalf("err=%v hits=%d", err, hits)
	}
}
