package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicIndexOmitsPaidPackageLocation(t *testing.T) {
	router, store := sourceStationRouter(t)
	freeSHA := sourceTestSHA256()
	paidSHA := strings.Repeat("cd", 32)
	store.mu.Lock()
	store.plugins["free-one"] = sourcePlugin{
		ID: "free-one", AppID: 1, Category: "other", Name: "免费插件", Description: "免费说明",
		Version: "1.0.0", SHA256: freeSHA, DownloadURL: "https://cdn.example.com/free.zip",
		Status: sourceItemPublished, PriceCents: 0, Billing: sourceBillingFree, Delivery: sourceDeliveryZip,
	}
	store.plugins["paid-one"] = sourcePlugin{
		ID: "paid-one", AppID: 1, Category: "other", Name: "付费插件", Description: "付费说明",
		Version: "1.0.0", SHA256: paidSHA, DownloadURL: "paid:" + strings.Repeat("ab", 16) + ".zip",
		Status: sourceItemPublished, PriceCents: 1990, Billing: sourceBillingOneTime, Delivery: sourceDeliveryZip,
	}
	store.templates["paid-home"] = sourceTemplate{
		ID: "paid-home", TemplateKey: "paid-home", AppID: 1, Category: sourceCategoryHomeTemplate,
		Name: "付费模板", Description: "模板说明", Version: "1.0.0", SchemaVersion: 1,
		SHA256: paidSHA, TemplateURL: "paid:" + strings.Repeat("cd", 16) + ".zip",
		Status: sourceItemPublished, PriceCents: 500, Billing: sourceBillingOneTime, Delivery: sourceDeliveryZip,
	}
	store.mu.Unlock()

	rec := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", rec.Code, rec.Body.String())
	}
	var index struct {
		Plugins []map[string]any `json:"plugins"`
		Homes   []map[string]any `json:"homeTemplates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	free, paid := findIndexEntry(t, index.Plugins, "free-one"), findIndexEntry(t, index.Plugins, "paid-one")
	if free["downloadUrl"] != "https://cdn.example.com/free.zip" || free["sha256"] != freeSHA {
		t.Fatalf("free entry lost package fields: %#v", free)
	}
	if free["name"] != "免费插件" || free["priceCents"] != float64(0) || free["billing"] != sourceBillingFree {
		t.Fatalf("free metadata=%#v", free)
	}
	if _, ok := paid["downloadUrl"]; ok {
		t.Fatalf("paid plugin leaked downloadUrl: %#v", paid)
	}
	if _, ok := paid["sha256"]; ok {
		t.Fatalf("paid plugin leaked sha256: %#v", paid)
	}
	if paid["name"] != "付费插件" || paid["description"] != "付费说明" || paid["priceCents"] != float64(1990) || paid["billing"] != sourceBillingOneTime {
		t.Fatalf("paid metadata=%#v", paid)
	}
	home := findIndexEntry(t, index.Homes, "paid-home")
	if _, ok := home["templateUrl"]; ok {
		t.Fatalf("paid template leaked templateUrl: %#v", home)
	}
	if _, ok := home["sha256"]; ok {
		t.Fatalf("paid template leaked sha256: %#v", home)
	}
	if home["name"] != "付费模板" || home["description"] != "模板说明" || home["priceCents"] != float64(500) {
		t.Fatalf("paid template metadata=%#v", home)
	}
}

func TestPublicPackageRouteRefusesPrivatePaidFiles(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AUTO_PRO_DATA_DIR", dataDir)
	router, store := sourceStationRouter(t)
	_, dev, _ := sourceApproveDeveloper(t, router, "seal-dev", "secret")

	freePayload := sourcePluginTestZIP(t)
	freeUpload := sourceMultipart(t, router, "/api/v1/source/developer/packages/upload", dev, "free.zip", freePayload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, freeUpload) != 200 {
		t.Fatalf("free upload=%s", freeUpload.Body.String())
	}
	freeSHA := sha256Hex(freePayload)
	freeURL := "/api/v1/public/source-packages/" + freeSHA + ".zip"
	if got := sourceJSON(t, router, http.MethodGet, freeURL, "", ""); got.Code != http.StatusOK || got.Body.String() != string(freePayload) {
		t.Fatalf("free package status=%d", got.Code)
	}

	paidDir := filepath.Join(dataDir, "source-packages-paid")
	if err := os.MkdirAll(paidDir, 0750); err != nil {
		t.Fatal(err)
	}
	privateName := strings.Repeat("ab", 16) + ".zip"
	if err := os.WriteFile(filepath.Join(paidDir, privateName), freePayload, 0640); err != nil {
		t.Fatal(err)
	}
	decoyName := strings.Repeat("cd", 32) + ".zip"
	if err := os.WriteFile(filepath.Join(paidDir, decoyName), freePayload, 0640); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/public/source-packages/" + privateName,
		"/api/v1/public/source-packages/" + decoyName,
		"/api/v1/public/source-packages/" + privateName + "/../../" + decoyName,
		"/api/v1/public/source-packages/..%2Fsource-packages-paid%2F" + privateName,
	} {
		got := sourceJSON(t, router, http.MethodGet, path, "", "")
		if got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "PK") {
			t.Fatalf("private path %s status=%d body=%q", path, got.Code, got.Body.String())
		}
	}
	if got := sourceJSON(t, router, http.MethodGet, freeURL, "", ""); got.Code != http.StatusOK {
		t.Fatalf("free package broken after private probes: %d", got.Code)
	}

	paidPayload := makeTestZIP(t, testZIPEntry{name: "demo-plugin/plugin.json", data: `{
		"id":"paid-plugin","name":"付费插件","version":"1.0.0","description":"封口测试",
		"author":{"name":"源站","url":"https://example.com","email":"dev@example.com"},"category":"other"
	}`})
	paidUpload := sourceMultipart(t, router, "/api/v1/source/developer/packages/upload", dev, "paid.zip", paidPayload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, paidUpload) != 200 {
		t.Fatalf("paid upload=%s", paidUpload.Body.String())
	}
	var uploaded struct {
		Data struct {
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
		} `json:"data"`
	}
	if err := json.Unmarshal(paidUpload.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	paidPublic := uploaded.Data.URL
	if paidPublic == "" || paidPublic == freeURL {
		t.Fatalf("paid public url=%q", paidPublic)
	}
	body := `{"appId":1,"id":"paid-plugin","name":"付费插件","version":"1.0.0","description":"封口测试","category":"other","priceCents":1990,"downloadUrl":"` + paidPublic + `","sha256":"` + uploaded.Data.SHA256 + `"}`
	saved := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, body)
	if sourceBodyCode(t, saved) != 200 || !strings.Contains(saved.Body.String(), `"billing":"one_time"`) || !strings.Contains(saved.Body.String(), `"storedBySite":true`) || strings.Contains(saved.Body.String(), `"paid:`) {
		t.Fatalf("seal save=%s", saved.Body.String())
	}
	sealed, err := store.GetPlugin("paid-plugin")
	if err != nil || !isPrivatePackageRef(sealed.DownloadURL) {
		t.Fatalf("sealed item=%#v err=%v", sealed, err)
	}
	if got := sourceJSON(t, router, http.MethodGet, paidPublic, "", ""); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "PK") {
		t.Fatalf("sealed public file still served: status=%d", got.Code)
	}
	if got := sourceJSON(t, router, http.MethodGet, freeURL, "", ""); got.Code != http.StatusOK || got.Body.String() != string(freePayload) {
		t.Fatalf("unrelated free package changed: status=%d", got.Code)
	}
	matches, err := filepath.Glob(filepath.Join(paidDir, "*.zip"))
	if err != nil || len(matches) < 3 {
		t.Fatalf("private dir files=%v err=%v", matches, err)
	}
}

func TestPaidExternalURLRejected(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "paid-ext", "secret")
	sha := sourceTestSHA256()
	devBody := `{"appId":1,"id":"paid-ext","name":"付费外链","version":"1.0.0","description":"x","category":"other","priceCents":100,"billing":"permanent","downloadUrl":"https://127.0.0.1/paid.zip","sha256":"` + sha + `"}`
	devRec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, devBody)
	if sourceBodyCode(t, devRec) != 400 || !strings.Contains(devRec.Body.String(), "拒绝访问非公网地址") {
		t.Fatalf("developer external=%s", devRec.Body.String())
	}
	if _, err := store.GetPlugin("paid-ext"); !errors.Is(err, errSourceNotFound) {
		t.Fatalf("rejected import was stored: %v", err)
	}
	httpBody := `{"appId":1,"id":"paid-http","name":"明文","version":"1.0.0","description":"x","category":"other","priceCents":100,"downloadUrl":"http://cdn.example.com/paid.zip","sha256":"` + sha + `"}`
	httpRec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, httpBody)
	if sourceBodyCode(t, httpRec) != 400 || !strings.Contains(httpRec.Body.String(), "来源外链必须是 https:// 地址") {
		t.Fatalf("http external=%s", httpRec.Body.String())
	}
	adminBody := `{"appId":1,"id":"paid-admin-ext","name":"管理外链","version":"1.0.0","description":"x","category":"other","priceCents":100,"downloadUrl":"https://10.1.2.3/admin.zip","sha256":"` + sha + `"}`
	adminRec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins", admin, adminBody)
	if sourceBodyCode(t, adminRec) != 400 || !strings.Contains(adminRec.Body.String(), "拒绝访问非公网地址") {
		t.Fatalf("admin external=%s", adminRec.Body.String())
	}
	tplBody := `{"appId":1,"id":"paid-tpl","templateKey":"paid-tpl","name":"付费模板外链","version":"1.0.0","description":"x","category":"home-template","priceCents":100,"templateUrl":"templates/home.json","sha256":"` + sha + `"}`
	tplRec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/templates", dev, tplBody)
	if sourceBodyCode(t, tplRec) != 400 || !strings.Contains(tplRec.Body.String(), "付费条目请上传 ZIP，或填写 HTTPS 外链由本站拉取托管") {
		t.Fatalf("template external=%s", tplRec.Body.String())
	}
	yearly := `{"appId":1,"id":"yearly-plugin","name":"年付","version":"1.0.0","description":"x","category":"other","priceCents":100,"billing":"yearly","downloadUrl":"","sha256":""}`
	yearRec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, yearly)
	if sourceBodyCode(t, yearRec) != 400 || !strings.Contains(yearRec.Body.String(), "年付尚未开放") {
		t.Fatalf("yearly=%s", yearRec.Body.String())
	}
}

func TestPaidItemsCannotBePublishedYet(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	router, _ := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "paid-shelf", "secret")
	payload := sourcePluginTestZIP(t)
	upload := sourceMultipart(t, router, "/api/v1/source/developer/packages/upload", dev, "paid.zip", payload, map[string]string{"kind": "plugin"})
	if sourceBodyCode(t, upload) != 200 {
		t.Fatalf("upload=%s", upload.Body.String())
	}
	var uploaded struct {
		Data struct {
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	body := `{"appId":1,"id":"paid-shelf","name":"暂不上架","version":"1.0.0","description":"付费说明","category":"other","priceCents":990,"downloadUrl":"` + uploaded.Data.URL + `","sha256":"` + uploaded.Data.SHA256 + `"}`
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins", dev, body); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("save=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/plugins/paid-shelf/submit", dev, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("submit=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/paid-shelf/approve", admin, "{}"); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("approve=%s", rec.Body.String())
	}
	shelf := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/plugins/paid-shelf/shelf", admin, "{}")
	if sourceBodyCode(t, shelf) != 400 || !strings.Contains(shelf.Body.String(), "付费条目暂不能上架") {
		t.Fatalf("shelf=%s", shelf.Body.String())
	}
	index := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if strings.Contains(index.Body.String(), "paid-shelf") || strings.Contains(index.Body.String(), uploaded.Data.SHA256) {
		t.Fatalf("paid item leaked into index: %s", index.Body.String())
	}
}

func findIndexEntry(t *testing.T, items []map[string]any, id string) map[string]any {
	t.Helper()
	for _, item := range items {
		if item["id"] == id {
			return item
		}
	}
	t.Fatalf("index missing %s", id)
	return nil
}
