package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAdminCanUpdateExistingCatalogItemFields(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	nextSHA := strings.Repeat("cd", 32)
	sourceRegisterAndPublish(t, router, admin, "plugin", "pay-plugin",
		`{"appId":1,"id":"pay-plugin","name":"支付插件","category":"payment","description":"旧描述","downloadUrl":"https://cdn.example.com/pay.zip","sha256":"`+sha+`","author":{"name":"旧作者"},"icon":"ri:bank-card-line"}`)

	rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/pay-plugin", admin,
		`{"name":"聚合支付","description":"新描述","category":"other","downloadUrl":"https://cdn.example.com/pay-v2.zip","sha256":"`+nextSHA+`","author":{"name":"新作者"},"changelog":"修正下载地址","icon":"ri:wallet-line","note":"管理员更正元数据"}`)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("update plugin=%s", rec.Body.String())
	}
	var body struct {
		Data struct {
			ID          string `json:"id"`
			AppID       int64  `json:"appId"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Category    string `json:"category"`
			DownloadURL string `json:"downloadUrl"`
			SHA256      string `json:"sha256"`
			Status      string `json:"status"`
			Icon        string `json:"icon"`
			Changelog   string `json:"changelog"`
			Author      struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID != "pay-plugin" || body.Data.AppID != 1 {
		t.Fatalf("id/appId must stay locked: %+v", body.Data)
	}
	if body.Data.Status != sourceItemPublished {
		t.Fatalf("published edit must keep published, got %s body=%s", body.Data.Status, rec.Body.String())
	}
	if body.Data.Name != "聚合支付" || body.Data.Description != "新描述" || body.Data.Category != "other" {
		t.Fatalf("metadata not updated: %+v", body.Data)
	}
	if body.Data.DownloadURL != "https://cdn.example.com/pay-v2.zip" || body.Data.SHA256 != nextSHA {
		t.Fatalf("location/sha256 not updated: %+v", body.Data)
	}
	if body.Data.Author.Name != "新作者" || body.Data.Icon != "ri:wallet-line" || body.Data.Changelog != "修正下载地址" {
		t.Fatalf("author/icon/changelog not updated: %+v", body.Data)
	}

	item, err := store.GetPlugin("pay-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != sourceItemPublished || item.AppID != 1 {
		t.Fatalf("store status/appId drifted: %+v", item)
	}
	audits, err := store.ListAudit(20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range audits {
		if entry.Action == "metadata_edit" && entry.TargetID == "pay-plugin" {
			found = true
			if !strings.Contains(entry.Detail, "管理员更正元数据") {
				t.Fatalf("audit detail=%q", entry.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("missing metadata_edit audit: %+v", audits)
	}

	live := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if !strings.Contains(live.Body.String(), "聚合支付") || !strings.Contains(live.Body.String(), "pay-v2.zip") {
		t.Fatalf("published index not refreshed: %s", live.Body.String())
	}

	locked := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/pay-plugin", admin,
		`{"id":"other-id","appId":99,"name":"仍应锁定","category":"payment","downloadUrl":"https://cdn.example.com/pay-v2.zip","sha256":"`+nextSHA+`"}`)
	if sourceBodyCode(t, locked) != 200 {
		t.Fatalf("locked update=%s", locked.Body.String())
	}
	again, err := store.GetPlugin("pay-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != "pay-plugin" || again.AppID != 1 {
		t.Fatalf("id/appId must ignore request override: %+v", again)
	}
	if _, err := store.GetPlugin("other-id"); err == nil {
		t.Fatal("must not create a new item when editing")
	}
}

func TestAdminCanUpdateExistingTemplateFields(t *testing.T) {
	router, store := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	sha := sourceTestSHA256()
	sourceRegisterAndPublish(t, router, admin, "template", "clean-home",
		`{"appId":1,"id":"clean-home","name":"清新首页","templateUrl":"https://cdn.example.com/home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates/clean-home", admin,
		`{"name":"极简首页","description":"改版","category":"home-template","templateUrl":"https://cdn.example.com/home-v2.zip","sha256":"`+sha+`","author":{"name":"设计组"},"changelog":"换包"}`)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("update template=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"极简首页"`) || !strings.Contains(rec.Body.String(), `"status":"published"`) {
		t.Fatalf("template metadata/status: %s", rec.Body.String())
	}
	item, err := store.GetTemplate("clean-home")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != sourceItemPublished || item.TemplateURL != "https://cdn.example.com/home-v2.zip" || item.AppID != 1 {
		t.Fatalf("template not updated in place: %+v", item)
	}

	cross := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/templates/clean-home", admin,
		`{"name":"错分类","category":"payment","templateUrl":"https://cdn.example.com/home-v2.zip","sha256":"`+sha+`"}`)
	if sourceBodyCode(t, cross) != 400 || !strings.Contains(cross.Body.String(), "分类") {
		t.Fatalf("template cannot use plugin category: %s", cross.Body.String())
	}
}

func TestAdminUpdateRejectsUnknownItem(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	rec := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/plugins/missing-item", admin,
		`{"name":"不存在","category":"other","downloadUrl":"https://cdn.example.com/x.zip","sha256":"`+sourceTestSHA256()+`"}`)
	if sourceBodyCode(t, rec) != 404 && sourceBodyCode(t, rec) != 400 {
		t.Fatalf("missing item should fail: %s", rec.Body.String())
	}
}
