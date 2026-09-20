package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSourceDeveloperAdApplicationApproveCreatesAdvertisement(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "dev-ads", "secret1")

	create := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", dev,
		`{"title":"春季活动","positions":["home-banner","sidebar"],"imageUrl":"https://cdn.example.com/ad.png","linkUrl":"https://example.com/promo","note":"希望放首页"}`)
	if sourceBodyCode(t, create) != 200 {
		t.Fatalf("create=%s", create.Body.String())
	}

	mine := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/ad-applications", dev, "")
	if sourceBodyCode(t, mine) != 200 || !strings.Contains(mine.Body.String(), `"pending"`) || !strings.Contains(mine.Body.String(), "春季活动") {
		t.Fatalf("developer list=%s", mine.Body.String())
	}

	adminList := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/ad-applications", admin, "")
	if sourceBodyCode(t, adminList) != 200 {
		t.Fatalf("admin list=%s", adminList.Body.String())
	}
	var listed struct {
		Data struct {
			List []struct {
				ID int64 `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &listed); err != nil || len(listed.Data.List) != 1 {
		t.Fatalf("admin list parse=%s", adminList.Body.String())
	}

	approve := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/ad-applications/"+itoa64(listed.Data.List[0].ID)+"/approve", admin, "{}")
	if sourceBodyCode(t, approve) != 200 || !strings.Contains(approve.Body.String(), `"approved"`) {
		t.Fatalf("approve=%s", approve.Body.String())
	}
	if !strings.Contains(approve.Body.String(), `"advertisementId"`) {
		t.Fatalf("approve should return advertisementId: %s", approve.Body.String())
	}

	ads := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/advertisements", admin, "")
	if sourceBodyCode(t, ads) != 200 || !strings.Contains(ads.Body.String(), "春季活动") {
		t.Fatalf("ads after approve=%s", ads.Body.String())
	}

	public := sourceJSON(t, router, http.MethodGet, "/api/v1/public/advertisements?position=home-banner", "", "")
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), "春季活动") {
		t.Fatalf("public ads=%s", public.Body.String())
	}

	again := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/ad-applications/"+itoa64(listed.Data.List[0].ID)+"/approve", admin, "{}")
	if sourceBodyCode(t, again) != 400 {
		t.Fatalf("double approve=%s", again.Body.String())
	}
}

func TestSourceDeveloperAdApplicationRejectDoesNotPublish(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin, dev, _ := sourceApproveDeveloper(t, router, "dev-ads-rej", "secret1")

	create := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", dev,
		`{"title":"拒绝投放","positions":["popup"],"imageUrl":"https://cdn.example.com/reject.png"}`)
	if sourceBodyCode(t, create) != 200 {
		t.Fatalf("create=%s", create.Body.String())
	}

	adminList := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/ad-applications?status=pending", admin, "")
	var listed struct {
		Data struct {
			List []struct {
				ID int64 `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &listed); err != nil || len(listed.Data.List) != 1 {
		t.Fatalf("admin list=%s", adminList.Body.String())
	}

	reject := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/ad-applications/"+itoa64(listed.Data.List[0].ID)+"/reject", admin, `{"note":"素材不合规"}`)
	if sourceBodyCode(t, reject) != 200 {
		t.Fatalf("reject=%s", reject.Body.String())
	}

	mine := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/ad-applications", dev, "")
	if sourceBodyCode(t, mine) != 200 || !strings.Contains(mine.Body.String(), `"rejected"`) || !strings.Contains(mine.Body.String(), "素材不合规") {
		t.Fatalf("developer list after reject=%s", mine.Body.String())
	}

	ads := sourceJSON(t, router, http.MethodGet, "/api/v1/source/admin/advertisements", admin, "")
	if sourceBodyCode(t, ads) != 200 || strings.Contains(ads.Body.String(), "拒绝投放") {
		t.Fatalf("rejected application must not create advertisement: %s", ads.Body.String())
	}

	again := sourceJSON(t, router, http.MethodPost, "/api/v1/source/admin/ad-applications/"+itoa64(listed.Data.List[0].ID)+"/reject", admin, `{"note":"again"}`)
	if sourceBodyCode(t, again) != 400 {
		t.Fatalf("double reject=%s", again.Body.String())
	}
}

func TestSourceDeveloperAdApplicationValidationAndIsolation(t *testing.T) {
	router, _ := sourceStationRouter(t)
	_, alice, _ := sourceApproveDeveloper(t, router, "dev-alice-ad", "secret1")
	_, bob, _ := sourceApproveDeveloper(t, router, "dev-bob-ad", "secret1")

	missingTitle := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", alice,
		`{"positions":["sidebar"]}`)
	if sourceBodyCode(t, missingTitle) != 400 {
		t.Fatalf("missing title=%s", missingTitle.Body.String())
	}
	badSlot := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", alice,
		`{"title":"非法广告位","positions":["unknown-slot"]}`)
	if sourceBodyCode(t, badSlot) != 400 {
		t.Fatalf("bad slot=%s", badSlot.Body.String())
	}
	badURL := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", alice,
		`{"title":"非法跳转","positions":["sidebar"],"linkUrl":"http://insecure.example"}`)
	if sourceBodyCode(t, badURL) != 400 {
		t.Fatalf("bad link=%s", badURL.Body.String())
	}

	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", alice,
		`{"title":"Alice 广告","positions":["sidebar"]}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("alice create=%s", rec.Body.String())
	}
	if rec := sourceJSON(t, router, http.MethodPost, "/api/v1/source/developer/ad-applications", bob,
		`{"title":"Bob 广告","positions":["popup"]}`); sourceBodyCode(t, rec) != 200 {
		t.Fatalf("bob create=%s", rec.Body.String())
	}

	aliceList := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/ad-applications", alice, "")
	if sourceBodyCode(t, aliceList) != 200 || !strings.Contains(aliceList.Body.String(), "Alice 广告") || strings.Contains(aliceList.Body.String(), "Bob 广告") {
		t.Fatalf("alice must only see own applications: %s", aliceList.Body.String())
	}
}

func TestSourceDeveloperStarterPackAndSkill(t *testing.T) {
	router, _ := sourceStationRouter(t)
	_, dev, _ := sourceApproveDeveloper(t, router, "dev-starter", "secret1")

	skill := sourceJSON(t, router, http.MethodGet, "/api/v1/source/developer/skill.md", dev, "")
	if skill.Code != http.StatusOK || !strings.Contains(skill.Body.String(), "name: auth-pro-plugin-template") {
		t.Fatalf("skill.md=%s", skill.Body.String())
	}
	if !strings.Contains(skill.Body.String(), "plugin.json") || !strings.Contains(skill.Body.String(), "template.json") {
		t.Fatalf("skill must mention manifests: %s", skill.Body.String())
	}

	zipRec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/source/developer/starter.zip", nil)
	req.Header.Set("Authorization", "Bearer "+dev)
	router.ServeHTTP(zipRec, req)
	if zipRec.Code != http.StatusOK {
		t.Fatalf("starter zip http=%d body=%s", zipRec.Code, zipRec.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(zipRec.Body.Bytes()), int64(zipRec.Body.Len()))
	if err != nil {
		t.Fatalf("starter zip: %v", err)
	}
	found := map[string][]byte{}
	for _, file := range archive.File {
		name := strings.TrimPrefix(file.Name, "auth-pro-developer-starter/")
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		payload, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		found[name] = payload
	}
	pluginJSON, ok := found["plugin-example/plugin.json"]
	if !ok {
		t.Fatalf("missing plugin example")
	}
	templateJSON, ok := found["template-example/template.json"]
	if !ok {
		t.Fatalf("missing template example")
	}
	if _, err := fillPluginManifest(sourcePackageManifest{}, pluginJSON, "plugin.json"); err != nil {
		t.Fatalf("example plugin.json rejected: %v", err)
	}
	if _, err := fillTemplateManifest(sourcePackageManifest{}, templateJSON, "template.json"); err != nil {
		t.Fatalf("example template.json rejected: %v", err)
	}
	if _, ok := found["SKILL.md"]; !ok {
		t.Fatal("starter zip missing SKILL.md")
	}
	if _, ok := found["docs/plugin-package.md"]; !ok {
		t.Fatal("starter zip missing plugin docs")
	}
	if !bytes.Contains(templateJSON, []byte(`"kind"`)) || !bytes.Contains(templateJSON, []byte("template")) {
		t.Fatalf("starter template.json must declare kind=template: %s", templateJSON)
	}
	if doc, ok := found["docs/template-package.md"]; !ok || !bytes.Contains(doc, []byte("kind")) {
		t.Fatal("starter template docs must document kind")
	}
}
