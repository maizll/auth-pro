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
	missing := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"`+secret+`"}`)
	if sourceBodyCode(t, missing) == 200 || !strings.Contains(missing.Body.String(), "请填写私有仓库") {
		t.Fatalf("owner required: %s", missing.Body.String())
	}
	saved := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"`+secret+`","owner":"station","repo":"paid-plugins"}`)
	if sourceBodyCode(t, saved) != 200 || strings.Contains(saved.Body.String(), secret) || !strings.Contains(saved.Body.String(), `"configured":true`) {
		t.Fatalf("save echoed token: %s", saved.Body.String())
	}
	got := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/settings/github-paid", admin, "")
	body := got.Body.String()
	if sourceBodyCode(t, got) != 200 || !strings.Contains(body, `"configured":true`) || !strings.Contains(body, `"owner":"station"`) || !strings.Contains(body, `"repo":"paid-plugins"`) || strings.Contains(body, secret) || strings.Contains(body, `"reminder":"`) && strings.Contains(body, paidLocalFallbackText) {
		t.Fatalf("get token view: %s", body)
	}
	if strings.Contains(body, `"reminder":"`+paidLocalFallbackText) {
		t.Fatalf("configured repo still reminds: %s", body)
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
	blank := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"","owner":"station","repo":"paid-plugins"}`)
	if sourceBodyCode(t, blank) != 200 || strings.Contains(blank.Body.String(), secret) {
		t.Fatalf("blank save: %s", blank.Body.String())
	}
	again, err := loadGitHubPaidToken()
	if err != nil || again != secret {
		t.Fatalf("blank save replaced token: %q %v", again, err)
	}
	owner, repo, token, err := loadGitHubPaidRepo()
	if err != nil || owner != "station" || repo != "paid-plugins" || token != secret {
		t.Fatalf("repo=%s/%s token=%q err=%v", owner, repo, token, err)
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
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+pat {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releases"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         9,
				"upload_url": "http://" + r.Host + r.URL.Path + "/9/assets{?name,label}",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets"):
			w.WriteHeader(http.StatusCreated)
			name := r.URL.Query().Get("name")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "name": name,
				"browser_download_url": "https://github.com/station/paid-plugins/releases/download/tag/" + name,
			})
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 9,
				"assets": []map[string]any{{
					"id": 7, "name": "demo-plugin-1.0.0.zip",
				}},
			})
		case strings.Contains(r.URL.Path, "/releases/assets/"):
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Errorf("asset accept=%s", r.Header.Get("Accept"))
			}
			http.Redirect(w, r, "https://release-assets.githubusercontent.com/plugin.zip?token=shortlived", http.StatusFound)
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
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"`+pat+`","owner":"station","repo":"paid-plugins"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save token: %s", save.Body.String())
	}
	publicURL := "https://release.example.com:" + zipPort + "/plugin.zip"
	published := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl":   publicURL,
		"packageSource": "public",
		"category":      "other",
		"appId":         "1",
		"priceCents":    "1990",
		"push":          "0",
		"shelf":         "1",
	})
	if sourceBodyCode(t, published) != 200 || strings.Contains(published.Body.String(), pat) || strings.Contains(published.Body.String(), `"storedPackage":true`) || !strings.Contains(published.Body.String(), "已存入收费仓库") {
		t.Fatalf("publish: %s", published.Body.String())
	}
	item, err := store.GetPlugin("demo-plugin")
	if err != nil {
		t.Fatalf("plugin: %v body=%s", err, published.Body.String())
	}
	wantRef := "github:station/paid-plugins/paid-plugin-demo-plugin-1.0.0/demo-plugin-1.0.0.zip"
	if item.DownloadURL != wantRef || item.PriceCents != 1990 || len(item.SHA256) != 64 {
		t.Fatalf("item=%#v", item)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 0 {
		t.Fatalf("stored zip files: %v", matches)
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	body := index.Body.String()
	for _, forbidden := range []string{"station", "paid-plugins", "github:", pat, "release-assets.githubusercontent.com", item.SHA256, item.DownloadURL} {
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

func TestDeveloperPaidPackagesUseStationRepo(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	pluginZIP := sourcePluginTestZIP(t)
	templateZIP := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{"kind":"template","id":"clean-home","name":"清新首页","version":"1.0.0","schemaVersion":1,
		"description":"模板","author":"设计组","hero":{"title":"欢迎"}}`})
	zipServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "home.zip") {
			_, _ = w.Write(templateZIP)
			return
		}
		_, _ = w.Write(pluginZIP)
	}))
	t.Cleanup(zipServer.Close)
	_, zipPort, err := net.SplitHostPort(zipServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	const pat = "github_pat_station_only"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+pat {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releases"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 9, "upload_url": "http://" + r.Host + r.URL.Path + "/9/assets{?name,label}",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets"):
			w.WriteHeader(http.StatusCreated)
			name := r.URL.Query().Get("name")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "name": name,
				"browser_download_url": "https://github.com/station/paid-plugins/releases/download/tag/" + name,
			})
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			name := "demo-plugin-1.0.0.zip"
			if strings.Contains(r.URL.Path, "clean-home") {
				name = "clean-home-1.0.0.zip"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 9, "assets": []map[string]any{{"id": 7, "name": name}},
			})
		case strings.Contains(r.URL.Path, "/releases/assets/"):
			http.Redirect(w, r, "https://release-assets.githubusercontent.com/plugin.zip?token=shortlived", http.StatusFound)
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
	gone := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/github-paid", "", "")
	if gone.Code != http.StatusNotFound && gone.Code != http.StatusUnauthorized {
		if sourceBodyCode(t, gone) == 200 {
			t.Fatalf("developer token route still exists: %s", gone.Body.String())
		}
	}
	_, tokenA, idA := sourceApproveDeveloper(t, router, "dev-paid-a", "secret")
	_, tokenB, _ := sourceApproveDeveloper(t, router, "dev-paid-b", "secret")
	devGone := sourceJSON(t, router, http.MethodPut, "/api/v1/source/developer/github-paid", tokenA, `{"token":"nope"}`)
	if devGone.Code != http.StatusNotFound {
		t.Fatalf("developer can still save a token: %d %s", devGone.Code, devGone.Body.String())
	}
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin, `{"token":"`+pat+`","owner":"station","repo":"paid-plugins"}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save repo: %s", save.Body.String())
	}

	pluginURL := "https://release.example.com:" + zipPort + "/plugin.zip"
	saved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", tokenA, `{
		"appId":1,"id":"demo-plugin","name":"演示插件","version":"1.0.0","category":"other",
		"priceCents":1990,"packageSource":"public","downloadUrl":"`+pluginURL+`"}`)
	if sourceBodyCode(t, saved) != 200 || strings.Contains(saved.Body.String(), pat) || strings.Contains(saved.Body.String(), "station/paid-plugins") || strings.Contains(saved.Body.String(), "githubOwner") || !strings.Contains(saved.Body.String(), "插件草稿已保存") || !strings.Contains(saved.Body.String(), `"storedBySite":true`) {
		t.Fatalf("developer plugin: %s", saved.Body.String())
	}
	item, err := store.GetPlugin("demo-plugin")
	if err != nil || item.DeveloperID != idA || item.DownloadURL != "github:station/paid-plugins/paid-plugin-demo-plugin-1.0.0/demo-plugin-1.0.0.zip" || len(item.SHA256) != 64 {
		t.Fatalf("plugin=%#v err=%v", item, err)
	}
	uploaded := sourceMultipart(t, router, "/api/v1/source/developer/packages/upload", tokenB, "demo-plugin.zip", pluginZIP, map[string]string{
		"kind": "plugin", "category": "other", "priceCents": "800",
	})
	if sourceBodyCode(t, uploaded) != 200 || !strings.Contains(uploaded.Body.String(), `"storedBySite":true`) || strings.Contains(uploaded.Body.String(), pat) {
		t.Fatalf("developer upload: %s", uploaded.Body.String())
	}
	homeURL := "https://release.example.com:" + zipPort + "/home.zip"
	templateSaved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/templates", tokenA, `{
		"appId":1,"id":"clean-home","templateKey":"clean-home","name":"清新首页","version":"1.0.0",
		"category":"home-template","schemaVersion":1,"priceCents":800,"packageSource":"public",
		"templateUrl":"`+homeURL+`"}`)
	if sourceBodyCode(t, templateSaved) != 200 || strings.Contains(templateSaved.Body.String(), pat) || strings.Contains(templateSaved.Body.String(), "githubOwner") || !strings.Contains(templateSaved.Body.String(), `"storedBySite":true`) {
		t.Fatalf("developer template: %s", templateSaved.Body.String())
	}
	tpl, err := store.GetTemplate("clean-home")
	if err != nil || tpl.DeveloperID != idA || tpl.TemplateURL != "github:station/paid-plugins/paid-template-clean-home-1.0.0/clean-home-1.0.0.zip" || len(tpl.SHA256) != 64 {
		t.Fatalf("template=%#v err=%v", tpl, err)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 0 {
		t.Fatalf("station repo left local zip: %v", matches)
	}
	store.mu.Lock()
	kept := store.plugins[item.ID]
	kept.Status = sourceItemPublished
	store.plugins[item.ID] = kept
	store.mu.Unlock()
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	body := index.Body.String()
	for _, forbidden := range []string{"station", "paid-plugins", "github:", pat, "release-assets.githubusercontent.com", item.SHA256, item.DownloadURL} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("buyer index contains %q", forbidden)
		}
	}
	if !strings.Contains(body, "演示插件") {
		t.Fatalf("buyer index missing name: %s", body)
	}
	if _, err := authorizeGitHubBuyerURL(context.Background(), false, item.DownloadURL); err == nil || !strings.Contains(err.Error(), "不能下载") {
		t.Fatalf("unpaid: %v", err)
	}
	temp, err := authorizeGitHubBuyerURL(context.Background(), true, item.DownloadURL)
	if err != nil || temp != "https://release-assets.githubusercontent.com/plugin.zip?token=shortlived" || strings.Contains(temp, pat) || strings.Contains(temp, "station/paid-plugins") {
		t.Fatalf("buyer url=%s err=%v", temp, err)
	}
}

func TestPaidSaveFallsBackWithoutStationRepo(t *testing.T) {
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
	_ = pinHostToServer(t, "release.example.com", zipServer, zipServer)
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	settings := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/settings/github-paid", admin, "")
	if sourceBodyCode(t, settings) != 200 || !strings.Contains(settings.Body.String(), paidLocalFallbackText) || !strings.Contains(settings.Body.String(), `"configured":false`) {
		t.Fatalf("settings reminder: %s", settings.Body.String())
	}
	publicURL := "https://release.example.com:" + zipPort + "/plugin.zip"
	published := sourceMultipart(t, router, "/api/v1/source/admin/packages/publish", admin, "", nil, map[string]string{
		"downloadUrl": publicURL, "packageSource": "public", "category": "other", "appId": "1", "priceCents": "1990", "push": "0",
	})
	if sourceBodyCode(t, published) != 200 || !strings.Contains(published.Body.String(), "暂存") {
		t.Fatalf("admin fallback: %s", published.Body.String())
	}
	item, err := store.GetPlugin("demo-plugin")
	if err != nil || !strings.HasPrefix(item.DownloadURL, "paid:") || len(item.SHA256) != 64 {
		t.Fatalf("fallback item=%#v err=%v", item, err)
	}
	matches, _ := filepath.Glob(filepath.Join(stationPaidPackageDirPath(), "*.zip"))
	if len(matches) != 1 {
		t.Fatalf("expected one local zip, got %v", matches)
	}
	_, devToken, devID := sourceApproveDeveloper(t, router, "dev-fallback", "secret")
	dev := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", devToken, `{
		"appId":1,"id":"other-plugin","name":"另一个","version":"1.0.0","category":"other","priceCents":100,
		"downloadUrl":"`+publicURL+`"}`)
	if sourceBodyCode(t, dev) != 200 || !strings.Contains(dev.Body.String(), "插件草稿已保存") || strings.Contains(dev.Body.String(), "尚未配置收费仓库") || strings.Contains(dev.Body.String(), "githubOwner") {
		t.Fatalf("developer fallback: %s", dev.Body.String())
	}
	other, err := store.GetPlugin("other-plugin")
	if err != nil || other.DeveloperID != devID || !strings.HasPrefix(other.DownloadURL, "paid:") {
		t.Fatalf("developer fallback item=%#v err=%v", other, err)
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
