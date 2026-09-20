package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourcePackageTemplateKindAutoBindsHomeTemplate(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)
	payload := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{
		"kind":"template",
		"id":"clean-home",
		"name":"清新首页",
		"version":"1.0.0",
		"schemaVersion":1,
		"description":"简洁的授权服务首页",
		"author":{"name":"设计组"},
		"hero":{"title":"欢迎"}
	}`})
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "home.zip", payload, nil)
	if sourceBodyCode(t, rec) != 200 {
		t.Fatalf("kind=template should parse: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"kind":"template"`) || !strings.Contains(rec.Body.String(), `"category":"home-template"`) {
		t.Fatalf("auto-bind home-template: %s", rec.Body.String())
	}
}

func TestSourcePackageRejectsWrongKindAndCategory(t *testing.T) {
	router, _ := sourceStationRouter(t)
	admin := sourceAdminToken(t)

	missingKind := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{
		"id":"clean-home","name":"清新首页","version":"1.0.0","schemaVersion":1,
		"description":"缺 kind","author":"设计组","hero":{"title":"欢迎"}
	}`})
	rec := sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "home.zip", missingKind, nil)
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "kind") {
		t.Fatalf("template.json without kind must fail: %s", rec.Body.String())
	}

	pluginKindOnTemplate := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{
		"kind":"plugin","id":"clean-home","name":"清新首页","version":"1.0.0","schemaVersion":1,
		"description":"错误 kind","author":"设计组","hero":{"title":"欢迎"}
	}`})
	rec = sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "home.zip", pluginKindOnTemplate, nil)
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "kind") {
		t.Fatalf("template.json kind=plugin must fail: %s", rec.Body.String())
	}

	templateKindOnPlugin := makeTestZIP(t, testZIPEntry{name: "plugin.json", data: `{
		"kind":"template","id":"demo-plugin","name":"演示插件","version":"1.0.0",
		"description":"错误 kind","author":{"name":"源站"},"category":"other"
	}`})
	rec = sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "plugin.zip", templateKindOnPlugin, nil)
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "kind") {
		t.Fatalf("plugin.json kind=template must fail: %s", rec.Body.String())
	}

	pluginOnHomeCategory := makeTestZIP(t, testZIPEntry{name: "plugin.json", data: `{
		"kind":"plugin","id":"demo-plugin","name":"演示插件","version":"1.0.0",
		"description":"错分类","author":{"name":"源站"},"category":"home-template"
	}`})
	rec = sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "plugin.zip", pluginOnHomeCategory, nil)
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "分类") {
		t.Fatalf("plugin cannot use template category: %s", rec.Body.String())
	}

	templateOnPayment := makeTestZIP(t, testZIPEntry{name: "template.json", data: `{
		"kind":"template","id":"clean-home","name":"清新首页","version":"1.0.0","schemaVersion":1,
		"description":"错分类","author":"设计组","category":"payment","hero":{"title":"欢迎"}
	}`})
	rec = sourceMultipart(t, router, "/api/v1/source/admin/packages/parse", admin, "home.zip", templateOnPayment, nil)
	if sourceBodyCode(t, rec) != 400 || !strings.Contains(rec.Body.String(), "分类") {
		t.Fatalf("template cannot use payment: %s", rec.Body.String())
	}
}

func TestDeveloperStarterExamplesPassHardValidation(t *testing.T) {
	pluginJSON, err := os.ReadFile(findDeveloperDocFile(t, "starter/plugin-example/plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	templateJSON, err := os.ReadFile(findDeveloperDocFile(t, "starter/template-example/template.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fillPluginManifest(sourcePackageManifest{}, pluginJSON, "plugin.json"); err != nil {
		t.Fatalf("docs starter plugin.json rejected: %v\n%s", err, pluginJSON)
	}
	manifest, err := fillTemplateManifest(sourcePackageManifest{}, templateJSON, "template.json")
	if err != nil {
		t.Fatalf("docs starter template.json rejected: %v\n%s", err, templateJSON)
	}
	if manifest.Kind != sourceKindTemplate || manifest.Category != sourceCategoryHomeTemplate {
		t.Fatalf("starter template bind=%+v", manifest)
	}
	if !strings.Contains(string(templateJSON), `"kind": "template"`) && !strings.Contains(string(templateJSON), `"kind":"template"`) {
		t.Fatal("starter template.json must declare kind=template")
	}
}

func findDeveloperDocFile(t *testing.T, rel string) string {
	t.Helper()
	dir := findDeveloperDocsDir()
	if dir == "" {
		t.Fatal("docs/developer not found")
	}
	return filepath.Join(dir, filepath.FromSlash(rel))
}
