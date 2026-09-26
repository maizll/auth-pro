package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClassifyPackageRefAndStampVersion(t *testing.T) {
	const paidName = "0123456789abcdef0123456789abcdef.zip"
	cases := []struct {
		raw, driver, key string
	}{
		{"paid:" + paidName, packageStorageLocal, paidName},
		{"github:station/paid-plugins/paid-plugin-demo-1.0.0/demo-1.0.0.zip", packageStorageGitHub, "station/paid-plugins/paid-plugin-demo-1.0.0/demo-1.0.0.zip"},
		{"https://cdn.example.com/free.zip", packageStorageExternal, "https://cdn.example.com/free.zip"},
		{"plugins/legacy.zip", "", ""},
	}
	for _, tc := range cases {
		driver, key := classifyPackageRef(tc.raw)
		if driver != tc.driver || key != tc.key {
			t.Fatalf("classify %q = %s %q, want %s %q", tc.raw, driver, key, tc.driver, tc.key)
		}
	}

	store := newMemorySourceStore()
	store.mu.Lock()
	store.plugins["demo"] = sourcePlugin{ID: "demo", AppID: 1, Name: "demo", Status: sourceItemDraft}
	store.mu.Unlock()
	rel, err := store.UpsertVersion(sourceRelease{
		Kind: sourceKindPlugin, ItemID: "demo", Version: "1.0.0",
		Location: "paid:" + paidName, SHA256: sourceTestSHA256(),
	}, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if rel.StorageDriver != packageStorageLocal || rel.ObjectKey != paidName || rel.SHA256 == "" {
		t.Fatalf("version storage=%s key=%s sha=%s", rel.StorageDriver, rel.ObjectKey, rel.SHA256)
	}
	external, ok := packageStorageByName(packageStorageExternal)
	if !ok {
		t.Fatal("external driver missing")
	}
	got, err := external.SignedURL(context.Background(), "https://cdn.example.com/free.zip", 0)
	if err != nil || got != "https://cdn.example.com/free.zip" {
		t.Fatalf("external signed url=%q err=%v", got, err)
	}
	if _, err := external.Put(context.Background(), "x", []byte("nope")); err == nil {
		t.Fatal("external put should fail")
	}
}

func TestLocalPackageStoragePutExistsDelete(t *testing.T) {
	driver := localPackageStorage{dir: t.TempDir()}
	payload := sourcePluginTestZIP(t)
	key, err := driver.Put(context.Background(), "", payload)
	if err != nil || key == "" {
		t.Fatalf("put key=%q err=%v", key, err)
	}
	ok, err := driver.Exists(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("exists=%v err=%v", ok, err)
	}
	listed, err := driver.List(context.Background(), key[:4])
	if err != nil || len(listed) != 1 || listed[0].Key != key {
		t.Fatalf("list=%#v err=%v", listed, err)
	}
	if _, err := driver.SignedURL(context.Background(), key, 0); err != errLocalPackageNeedsTicket {
		t.Fatalf("signed url err=%v", err)
	}
	if err := driver.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	ok, err = driver.Exists(context.Background(), key)
	if err != nil || ok {
		t.Fatalf("exists after delete=%v err=%v", ok, err)
	}
}

func TestSettlePaidZipDoesNotStoreLocallyWhenGitHubPutFails(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	t.Cleanup(api.Close)
	previous := sourceGitHubAPIBase
	sourceGitHubAPIBase = api.URL
	t.Cleanup(func() { sourceGitHubAPIBase = previous })

	saved := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/settings/github-paid", admin,
		`{"token":"ghp_test_token_value","owner":"station","repo":"paid-plugins"}`)
	if sourceBodyCode(t, saved) != 200 {
		t.Fatalf("save settings=%s", saved.Body.String())
	}
	before := paidPackageFileCount(t)
	ref, _, local, err := settlePaidZipBytes(context.Background(), sourceKindPlugin, "demo-plugin", "1.0.0", sourcePluginTestZIP(t))
	if err == nil || local || ref != "" {
		t.Fatalf("ref=%q local=%v err=%v", ref, local, err)
	}
	if !strings.Contains(err.Error(), "上传到收费仓库失败") && !strings.Contains(err.Error(), "GitHub") {
		t.Fatalf("err=%v", err)
	}
	if got := paidPackageFileCount(t); got != before {
		t.Fatalf("local paid files %d -> %d", before, got)
	}
}

func paidPackageFileCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(stationPaidPackageDir())
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
