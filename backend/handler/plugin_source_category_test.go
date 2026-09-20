package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPluginSourceGroupsCustomCategoryFromIndex(t *testing.T) {
	index, err := parsePluginSourceManifest([]byte(`{
		"name":"App A",
		"categories":[
			{"key":"payment","label":"支付","kind":"plugin","builtin":true},
			{"key":"realname","label":"实名认证","kind":"plugin","builtin":true},
			{"key":"other","label":"其他","kind":"plugin","builtin":true},
			{"key":"home-template","label":"首页模板","kind":"template","builtin":true},
			{"key":"theme","label":"主题","kind":"plugin"}
		],
		"plugins":[
			{"id":"pay-core","category":"payment","name":"支付核心","version":"1.0.0"},
			{"id":"theme-pack","category":"theme","name":"节日主题","description":"自定义分类","version":"2.0.0"}
		],
		"homeTemplates":[{"id":"clean-home","category":"home-template"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Categories) != 5 {
		t.Fatalf("categories=%d %+v", len(index.Categories), index.Categories)
	}

	remote := make([]pluginInfo, 0, len(index.Plugins))
	for _, item := range index.Plugins {
		remote = append(remote, pluginInfo{
			ID: item.ID, Category: displayPluginCategory(item.Category),
			Name: item.Name, Description: item.Description, Version: item.Version, Remote: true,
		})
	}
	groups := buildPluginStoreGroups(nil, remote, []*remotePluginIndex{index}, "")

	byKey := map[string]categoryGroup{}
	for _, group := range groups {
		byKey[group.Category] = group
	}
	theme, ok := byKey["theme"]
	if !ok {
		t.Fatalf("custom category missing from groups: %+v", groups)
	}
	if theme.Title != "主题" {
		t.Fatalf("want Chinese label from index, got %q", theme.Title)
	}
	if len(theme.Plugins) != 1 || theme.Plugins[0].ID != "theme-pack" {
		t.Fatalf("theme plugins=%+v", theme.Plugins)
	}
	other := byKey["other"]
	for _, plugin := range other.Plugins {
		if plugin.ID == "theme-pack" {
			t.Fatalf("custom category must not collapse into other")
		}
	}
	if _, ok := byKey["payment"]; !ok {
		t.Fatal("builtin payment group missing")
	}
}

func TestPluginSourceGroupsUnknownCategoryWithoutIndexLabel(t *testing.T) {
	remote := []pluginInfo{{
		ID: "analytics-kit", Category: "analytics", Name: "统计套件", Version: "1.0.0", Remote: true,
	}}
	groups := buildPluginStoreGroups(nil, remote, nil, "")
	found := false
	for _, group := range groups {
		if group.Category == "analytics" {
			found = true
			if len(group.Plugins) != 1 || group.Plugins[0].ID != "analytics-kit" {
				t.Fatalf("analytics group=%+v", group)
			}
		}
		if group.Category == "other" {
			for _, plugin := range group.Plugins {
				if plugin.ID == "analytics-kit" {
					t.Fatalf("unknown category dropped into other: %s", plugin.ID)
				}
			}
		}
	}
	if !found {
		t.Fatalf("unknown category not grouped: %s", mustJSON(t, groups))
	}
}

func TestPluginStoreIncludesExtraCategoryNamedTemplate(t *testing.T) {
	// Exact product repro: extra Name=模板, Id=template, Manifest=plugin.json.
	index, err := parsePluginSourceManifest([]byte(`{
		"name":"App A",
		"categories":[
			{"key":"payment","label":"支付","kind":"plugin","builtin":true},
			{"key":"realname","label":"实名认证","kind":"plugin","builtin":true},
			{"key":"other","label":"其他","kind":"plugin","builtin":true},
			{"key":"home-template","label":"首页模板","kind":"template","builtin":true},
			{"key":"template","label":"模板","kind":"plugin"}
		],
		"plugins":[
			{"id":"tpl-skin","category":"template","name":"皮肤模板插件","version":"1.0.0"}
		],
		"homeTemplates":[{"id":"clean-home","category":"home-template","name":"清新首页"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	remote := make([]pluginInfo, 0, len(index.Plugins))
	for _, item := range index.Plugins {
		remote = append(remote, pluginInfo{
			ID: item.ID, Category: displayPluginCategory(item.Category),
			Name: item.Name, Version: item.Version, Remote: true,
		})
	}
	groups := buildPluginStoreGroups(nil, remote, []*remotePluginIndex{index}, "")
	var extra *categoryGroup
	for i := range groups {
		if groups[i].Category == "template" {
			extra = &groups[i]
		}
	}
	if extra == nil {
		t.Fatalf("custom plugin category key=template must become a store tab/group: %s", mustJSON(t, groups))
	}
	if extra.Title != "模板" {
		t.Fatalf("want Chinese label 模板, got %q", extra.Title)
	}
	if len(extra.Plugins) != 1 || extra.Plugins[0].ID != "tpl-skin" {
		t.Fatalf("plugins filed under category=template must stay in that tab: %+v", extra.Plugins)
	}
	for _, group := range groups {
		if group.Category == "other" || group.Category == sourceCategoryHomeTemplate {
			for _, plugin := range group.Plugins {
				if plugin.ID == "tpl-skin" {
					t.Fatalf("category=template plugin leaked into %s", group.Category)
				}
			}
		}
	}
}

func TestSourceCustomCategoryPublishedIndexConsumedByPluginSource(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	save := sourceJSON(t, router, http.MethodPut, "/api/v1/source/admin/categories", admin,
		`{"extras":[{"key":"template","label":"模板","kind":"plugin"}]}`)
	if sourceBodyCode(t, save) != 200 {
		t.Fatalf("save categories=%s", save.Body.String())
	}
	sha := sourceTestSHA256()
	sourceRegisterAndPublish(t, router, admin, "plugin", "tpl-skin",
		`{"appId":1,"id":"tpl-skin","name":"皮肤模板插件","category":"template","downloadUrl":"https://cdn.example.com/tpl.zip","sha256":"`+sha+`"}`)
	sourceRegisterAndPublish(t, router, admin, "template", "clean-home",
		`{"appId":1,"id":"clean-home","name":"清新首页","category":"home-template","templateUrl":"https://cdn.example.com/home.zip","sha256":"`+sha+`","schemaVersion":1}`)

	live := sourceJSON(t, router, http.MethodGet, "/software-source/app-a/index.json", "", "")
	if live.Code != 200 {
		t.Fatalf("index=%s", live.Body.String())
	}
	index, err := parsePluginSourceManifest(live.Body.Bytes())
	if err != nil {
		t.Fatalf("parse published index: %v body=%s", err, live.Body.String())
	}
	foundExtra := false
	for _, category := range index.Categories {
		if category.Key == "template" && category.Label == "模板" && category.Kind == sourceKindPlugin {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Fatalf("published index missing extras category template/模板: %s", live.Body.String())
	}
	if !strings.Contains(live.Body.String(), `"tpl-skin"`) || strings.Contains(live.Body.String(), `"homeTemplates":[{"id":"tpl-skin"`) {
		t.Fatalf("plugin extra must stay in plugins, not homeTemplates: %s", live.Body.String())
	}

	remote := make([]pluginInfo, 0, len(index.Plugins))
	for _, item := range index.Plugins {
		remote = append(remote, pluginInfo{
			ID: item.ID, Category: displayPluginCategory(item.Category),
			Name: item.Name, Version: item.Version, Remote: true,
		})
	}
	groups := buildPluginStoreGroups(nil, remote, []*remotePluginIndex{index}, "")
	matched := false
	for _, group := range groups {
		if group.Category == "template" && group.Title == "模板" && len(group.Plugins) == 1 && group.Plugins[0].ID == "tpl-skin" {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("plugin_source did not consume custom category template: %s groups=%s", live.Body.String(), mustJSON(t, groups))
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
