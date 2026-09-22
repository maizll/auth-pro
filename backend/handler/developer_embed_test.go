package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeveloperSkillAndStarterIgnoreDiskLayout(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	skill := developerSkillMarkdown()
	assertActionableDeveloperSkill(t, skill)
	embeddedSkill, err := developerEmbedFS.ReadFile(embeddedDeveloperSkillPath)
	if err != nil {
		t.Fatal(err)
	}
	if skill != string(embeddedSkill) {
		t.Fatal("skill download did not return the embedded SKILL.md")
	}
	if strings.TrimSpace(skill) == strings.TrimSpace(developerSkillStubSentence()) {
		t.Fatal("skill download still serves the production stub")
	}

	payload, err := buildDeveloperStarterZIP()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]byte{}
	for _, file := range archive.File {
		name := strings.TrimPrefix(file.Name, "auth-pro-developer-starter/")
		base := strings.ToLower(name)
		if strings.HasSuffix(base, "index.html") || strings.HasSuffix(base, "login.html") {
			t.Fatalf("starter zip must not ship %s", name)
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		found[name] = body
	}

	assertActionableDeveloperSkill(t, string(found["SKILL.md"]))
	assertActionableDeveloperSkill(t, string(found[".cursor/skills/auth-pro-plugin-template/SKILL.md"]))

	charter := string(found["docs/charter.md"])
	for _, needle := range []string{"hero.primaryAction", "stylePreset", "schemaVersion", "sha256"} {
		if !strings.Contains(charter, needle) {
			t.Fatalf("embedded charter missing %q", needle)
		}
	}
	if len(charter) < 1500 {
		t.Fatalf("charter fallback is still a one-liner: %d bytes", len(charter))
	}

	pluginJSON := found["plugin-example/plugin.json"]
	if _, err := fillPluginManifest(sourcePackageManifest{}, pluginJSON, "plugin.json"); err != nil {
		t.Fatalf("starter plugin.json rejected: %v\n%s", err, pluginJSON)
	}
	pluginReadme := string(found["plugin-example/README.md"])
	for _, needle := range []string{"sha256sum", "zip -X", "demo-widget"} {
		if !strings.Contains(pluginReadme, needle) {
			t.Fatalf("plugin readme missing %q", needle)
		}
	}

	templateJSON := found["template-example/template.json"]
	templateManifest, err := fillTemplateManifest(sourcePackageManifest{}, templateJSON, "template.json")
	if err != nil {
		t.Fatalf("starter template.json rejected: %v\n%s", err, templateJSON)
	}
	if templateManifest.Kind != sourceKindTemplate || templateManifest.Category != sourceCategoryHomeTemplate {
		t.Fatalf("cartoon template bind=%+v", templateManifest)
	}
	assertTemplateExample(t, templateJSON, "cartoon-blue")

	goldJSON := found["template-example/template.fintech-gold.json"]
	goldManifest, err := fillTemplateManifest(sourcePackageManifest{}, goldJSON, "template.json")
	if err != nil {
		t.Fatalf("fintech template rejected: %v\n%s", err, goldJSON)
	}
	if goldManifest.Kind != sourceKindTemplate {
		t.Fatalf("gold template bind=%+v", goldManifest)
	}
	assertTemplateExample(t, goldJSON, "fintech-gold")

	templateReadme := string(found["template-example/README.md"])
	for _, needle := range []string{"cartoon-blue", "fintech-gold", "sha256sum", "template.fintech-gold.json"} {
		if !strings.Contains(templateReadme, needle) {
			t.Fatalf("template readme missing %q", needle)
		}
	}
}

func assertActionableDeveloperSkill(t *testing.T, skill string) {
	t.Helper()
	needles := []string{
		"name: auth-pro-plugin-template",
		"sha256sum",
		"zip -X",
		"stylePreset",
		"cartoon-blue",
		"fintech-gold",
		"primaryColor",
		"backgroundColor",
		"textColor",
		"schemaVersion",
		"template.json",
		"plugin.json",
		"primaryAction",
		"downloadUrl",
		"templateUrl",
	}
	for _, needle := range needles {
		if !strings.Contains(skill, needle) {
			excerpt := skill
			if len(excerpt) > 240 {
				excerpt = excerpt[:240]
			}
			t.Fatalf("skill missing %q (len=%d): %s", needle, len(skill), excerpt)
		}
	}
	if strings.Contains(skill, developerSkillStubSentence()) && !strings.Contains(skill, "登记（中文标签") {
		t.Fatal("skill is still the production stub")
	}
}

func TestEmbeddedDeveloperDocsMatchRepository(t *testing.T) {
	dir := findDeveloperDocsDir()
	if dir == "" {
		t.Fatal("docs/developer not found")
	}
	err := filepath.Walk(dir, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, name)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		embedded, readErr := developerEmbedFS.ReadFile("developer_embed/docs/" + rel)
		if readErr != nil {
			t.Errorf("missing embed docs/%s: %v", rel, readErr)
			return nil
		}
		disk, readErr := os.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(disk, embedded) {
			t.Errorf("embed drift docs/%s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "developer-skills", "auth-pro-plugin-template", "SKILL.md")
	disk, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := developerEmbedFS.ReadFile(embeddedDeveloperSkillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, embedded) {
		t.Fatal("embed drift SKILL.md")
	}
}

func assertTemplateExample(t *testing.T, payload []byte, preset string) {
	t.Helper()
	text := string(payload)
	for _, needle := range []string{
		`"kind": "template"`,
		`"schemaVersion": 1`,
		`"type": "login"`,
		`"stylePreset": "` + preset + `"`,
		`"primaryColor"`,
		`"backgroundColor"`,
		`"textColor"`,
		`"features"`,
		`"footer"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("template %s missing %s", preset, needle)
		}
	}
	if strings.Contains(text, `"scripts"`) || strings.Contains(text, "index.html") || strings.Contains(text, "login.html") {
		t.Fatalf("template %s contains a forbidden page or scripts field", preset)
	}
}

func developerSkillStubSentence() string {
	return "template.json 必须包含 kind: template。插件自定义分类会出现在应用商店筛选页签。"
}
