package handler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubPaidTokenIsEncryptedAndNotEchoed(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	const secret = "github_pat_super_secret_value"
	saved := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"`+secret+`"}`)
	if sourceBodyCode(t, saved) != 200 || strings.Contains(saved.Body.String(), secret) {
		t.Fatalf("save echoed token: %s", saved.Body.String())
	}
	got := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/settings/github-paid", admin, "")
	if sourceBodyCode(t, got) != 200 || !strings.Contains(got.Body.String(), `"configured":true`) || strings.Contains(got.Body.String(), secret) {
		t.Fatalf("get token view: %s", got.Body.String())
	}
	sealed, err := readGitHubPaidTokenSealed()
	if err != nil || sealed == "" || strings.Contains(sealed, secret) {
		t.Fatalf("sealed=%q err=%v", sealed, err)
	}
	plain, err := openGitHubPaidToken(sealed)
	if err != nil || plain != secret {
		t.Fatalf("open=%q err=%v", plain, err)
	}
	if _, err := openStoreSecret([]byte("wrong-key-wrong-key-wrong-key-32"), mustDecodeGitHubToken(t, sealed)); err == nil {
		t.Fatal("wrong key opened the token")
	}
	blank := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":""}`)
	if sourceBodyCode(t, blank) != 200 || strings.Contains(blank.Body.String(), secret) {
		t.Fatalf("blank save: %s", blank.Body.String())
	}
	again, err := loadGitHubPaidToken()
	if err != nil || again != secret {
		t.Fatalf("blank save replaced token: %q %v", again, err)
	}
}

func mustDecodeGitHubToken(t *testing.T, sealed string) []byte {
	t.Helper()
	raw, err := openGitHubPaidToken(sealed)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw
	key, err := loadOrCreateStoreFileKey("github-paid.key")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := sealStoreSecret(key, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func TestGitHubPaidRegisterBuyerURLAndTokenFailure(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	payload := sourcePluginTestZIP(t)
	zipServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(zipServer.Close)
	_, zipPort, err := net.SplitHostPort(zipServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	const pat = "github_pat_do_not_leak"
	buyerHits := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+pat {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 1,
				"assets": []map[string]any{{
					"id": 7, "name": "plugin.zip",
				}},
			})
		case strings.Contains(r.URL.Path, "/releases/assets/"):
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Errorf("asset accept=%s", r.Header.Get("Accept"))
			}
			buyerHits++
			target := "https://release-assets.githubusercontent.com:" + zipPort + "/plugin.zip"
			if buyerHits > 1 {
				target = "https://release-assets.githubusercontent.com/plugin.zip?token=shortlived"
			}
			http.Redirect(w, r, target, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	previousAPI := sourceGitHubAPIBase
	sourceGitHubAPIBase = api.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previousAPI })
	_ = pinTwoPublicHosts(t, "release.example.com", zipServer, "release-assets.githubusercontent.com", zipServer)

	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"`+pat+`"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save token: %s", save.Body.String())
	}
	published := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl":   "https://github.com/acme/paid/releases/download/v1.0.0/plugin.zip",
		"packageSource": "github",
		"category":      "other",
		"appId":         "1",
		"priceCents":    "1990",
		"push":          "0",
		"shelf":         "1",
	})
	if sourceBodyCode(t, published) != 200 || strings.Contains(published.Body.String(), pat) || strings.Contains(published.Body.String(), `"storedPackage":true`) {
		t.Fatalf("publish: %s", published.Body.String())
	}
	item, err := store.GetPlugin("demo-plugin")
	if err != nil {
		t.Fatalf("plugin: %v body=%s", err, published.Body.String())
	}
	wantRef := "github:acme/paid/v1.0.0/plugin.zip"
	if item.DownloadURL != wantRef || item.PriceCents != 1990 || item.OriginURL != "" || len(item.SHA256) != 64 {
		t.Fatalf("item=%#v", item)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 0 {
		t.Fatalf("stored zip files: %v", matches)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	body := index.Body.String()
	for _, forbidden := range []string{"acme", "github:", pat, "release-assets.githubusercontent.com", item.SHA256, item.DownloadURL} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("buyer index contains %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, item.Name) {
		t.Fatalf("buyer index missing name: %s", body)
	}

	if _, err := authorizeGitHubBuyerURL(context.Background(), false, wantRef); err == nil || !strings.Contains(err.Error(), "不能下载") {
		t.Fatalf("unpaid: %v", err)
	}

	temp, err := authorizeGitHubBuyerURL(context.Background(), true, wantRef)
	if err != nil {
		t.Fatal(err)
	}
	if temp != "https://release-assets.githubusercontent.com/plugin.zip?token=shortlived" || strings.Contains(temp, pat) || strings.Contains(temp, "acme/paid") {
		t.Fatalf("temp url=%s", temp)
	}

	sourceGitHubAPIBase = api.URL
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(bad.Close)
	sourceGitHubAPIBase = bad.URL
	if _, err := authorizeGitHubBuyerURL(context.Background(), true, wantRef); err == nil || !strings.Contains(err.Error(), "令牌无效") {
		t.Fatalf("invalid token: %v", err)
	}
	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+pat {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(missing.Close)
	sourceGitHubAPIBase = missing.URL
	if _, err := authorizeGitHubBuyerURL(context.Background(), true, wantRef); err == nil || !strings.Contains(err.Error(), "找不到") {
		t.Fatalf("missing asset: %v", err)
	}

	store.mu.Lock()
	kept := store.plugins[item.ID]
	kept.Status = sourceItemPublished
	kept.OriginHealth = paidOriginHealthOK
	store.plugins[item.ID] = kept
	store.mu.Unlock()
	touchGitHubPaidHealth(context.Background(), store, sourceKindPlugin, item.ID, item.Name, item.PriceCents, item.DownloadURL, paidOriginHealthOK)
	after, err := store.GetPlugin(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != sourceItemPublished || after.OriginHealth != paidOriginHealthUnavailable {
		t.Fatalf("health changed shelf: %#v", after)
	}
}

func TestPaidExternalRestoreDoesNotOverwriteHealthyRows(t *testing.T) {
	if !paidExternalNeedsRestore(1990, sourceItemDeprecated, "https://github.com/acme/paid/releases/download/v1/a.zip") {
		t.Fatal("deprecated paid https should restore")
	}
	if paidExternalNeedsRestore(1990, sourceItemPublished, "https://cdn.example.com/a.zip") {
		t.Fatal("published row must stay")
	}
	if paidExternalNeedsRestore(0, sourceItemDeprecated, "https://cdn.example.com/a.zip") {
		t.Fatal("free row must stay")
	}
	if paidExternalNeedsRestore(1990, sourceItemDeprecated, "github:acme/paid/v1/a.zip") {
		t.Fatal("github ref must stay")
	}
	if paidExternalNeedsRestore(1990, sourceItemDeprecated, "paid:"+strings.Repeat("ab", 16)+".zip") {
		t.Fatal("hosted zip must stay")
	}
	if got := appendPaidLegacyNote("已有备注"); got != "已有备注；"+paidLegacyFulfillmentNote {
		t.Fatalf("note=%s", got)
	}
	if got := appendPaidLegacyNote(paidLegacyFulfillmentNote); got != paidLegacyFulfillmentNote {
		t.Fatalf("note duplicated: %s", got)
	}

	state := newLegacySourceSchemaState()
	state.plugins = []sourceSchemaPluginRow{
		{id: "old", downloadURL: "https://github.com/acme/x/releases/download/v1/a.zip", status: sourceItemDeprecated, priceCents: 1000},
		{id: "live", downloadURL: "https://cdn.example.com/live.zip", status: sourceItemPublished, priceCents: 1000, reviewNote: "保持"},
		{id: "free", downloadURL: "https://cdn.example.com/free.zip", status: sourceItemDeprecated, priceCents: 0},
		{id: "gh", downloadURL: "github:acme/paid/v1/a.zip", status: sourceItemHidden, priceCents: 1000},
	}
	state.templates = []sourceSchemaTemplateRow{
		{id: "old-tpl", templateURL: "https://cdn.example.com/t.zip", status: sourceItemHidden, priceCents: 500},
	}
	db := openSourceSchemaMigrateDB(t, state)
	if err := migratePaidExternalVisible(db); err != nil {
		t.Fatal(err)
	}
	if err := migratePaidExternalVisible(db); err != nil {
		t.Fatal(err)
	}
	got := map[string]sourceSchemaPluginRow{}
	for _, row := range state.plugins {
		got[row.id] = row
	}
	if got["old"].status != sourceItemDraft || !strings.Contains(got["old"].reviewNote, paidLegacyFulfillmentNote) {
		t.Fatalf("old=%+v", got["old"])
	}
	if got["live"].status != sourceItemPublished || got["live"].reviewNote != "保持" || got["live"].downloadURL != "https://cdn.example.com/live.zip" {
		t.Fatalf("live overwritten: %+v", got["live"])
	}
	if got["free"].status != sourceItemDeprecated || got["gh"].status != sourceItemHidden {
		t.Fatalf("free=%+v gh=%+v", got["free"], got["gh"])
	}
	if state.templates[0].status != sourceItemDraft || !strings.Contains(state.templates[0].reviewNote, paidLegacyFulfillmentNote) {
		t.Fatalf("template=%+v", state.templates[0])
	}
	if strings.Count(got["old"].reviewNote, paidLegacyFulfillmentNote) != 1 {
		t.Fatalf("note repeated: %s", got["old"].reviewNote)
	}
}

func TestGitHubReleaseAssetURLParse(t *testing.T) {
	ref, err := parseGitHubReleaseAssetURL("https://github.com/maizll/authproPlus-source/releases/download/alipay-f2f-1.0.0/alipay-f2f-1.0.0.zip")
	if err != nil || ref.Owner != "maizll" || ref.Repo != "authproPlus-source" || ref.Tag != "alipay-f2f-1.0.0" || ref.Asset != "alipay-f2f-1.0.0.zip" {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}
	if _, err := parseGitHubReleaseAssetURL("https://github.com/maizll/authproPlus-source/archive/refs/heads/main.zip"); err == nil {
		t.Fatal("non-release url accepted")
	}
}
