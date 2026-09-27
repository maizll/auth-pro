package handler

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteDocWhitelistSkipsInternalRecords(t *testing.T) {
	allowed := []string{
		"install.md",
		"deployment.md",
		"commercial.md",
		"developer/template-package.md",
		"developer/starter/enterprise-template/README.md",
	}
	for _, rel := range allowed {
		if !siteDocAllowed(rel) {
			t.Fatalf("%s should be public", rel)
		}
	}
	denied := []string{
		"admin-navigation.md",
		"audit/p0-p1-2026-09.md",
		"design/payment-channel-plugin.md",
		"superpowers/specs/2026-09-21-client-sdk-hybrid-design.md",
		"release-notes-1.6.8.txt",
		"developer/SKILL.md",
		"upgrade-history.md",
	}
	for _, rel := range denied {
		if siteDocAllowed(rel) {
			t.Fatalf("%s must stay off the public site", rel)
		}
	}
}

func TestInstallDocUsesPublishedBaotaCommands(t *testing.T) {
	name, slug, catSort := siteDocCategoryFor("install.md")
	if name != "部署与运维" || slug != "ops" || catSort != 10 {
		t.Fatalf("category=%s %s %d", name, slug, catSort)
	}
	if siteDocSeedSort("install.md", 30) != 5 || siteDocSeedSort("deployment.md", 20) != 20 {
		t.Fatal("install.md should lead the ops category without moving other articles")
	}
	body := readRepoDoc(t, "install.md")
	for _, snippet := range []string{
		"cd /www/wwwroot/example.com\ntar -xzf auth_pro-full-vX.Y.Z.tar.gz\nbash baota-install.sh",
		"AUTH_PRO_YES=1 AUTH_PRO_START=0 \\\nbash baota-install.sh \\\n  --site-root /www/wwwroot/example.com \\\n  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz",
		"bash baota-upgrade.sh \\\n  --site-root /www/wwwroot/example.com \\\n  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \\\n  --no-start",
		"https://auth.maizll.com/api/v1/update/latest.json",
		"检查更新",
		"立即更新",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("install.md missing %q", snippet)
		}
	}
	if strings.Contains(body, "v1.5.7") || strings.Contains(body, "1.5.5") || strings.Contains(body, "上传安装包") {
		t.Fatal("install.md still has a stale or invented install step")
	}
	deployment := readRepoDoc(t, "deployment.md")
	if strings.Contains(deployment, "auth_pro-full-v1.5.7.tar.gz") || strings.Contains(deployment, "从 1.5.5") || strings.Contains(deployment, "从 1.5.3") {
		t.Fatal("public deployment guide still has a historical upgrade")
	}
	if !strings.Contains(deployment, "auth_pro-full-vX.Y.Z.tar.gz") {
		t.Fatal("current deployment examples lost the package placeholder")
	}
	history := readRepoDoc(t, "upgrade-history.md")
	if !strings.Contains(history, "auth_pro-full-v1.5.7.tar.gz") {
		t.Fatal("maintainer upgrade history should keep the 1.5.7 command")
	}
	for rel := range sitePublicDocs {
		publicBody := readRepoDoc(t, rel)
		if strings.Contains(publicBody, "从 1.5.5") || strings.Contains(publicBody, "从 1.5.3") || strings.Contains(publicBody, "升到 1.5.7") {
			t.Fatalf("%s still tells customers how to upgrade a specific old release", rel)
		}
	}
	for rel := range sitePublicDocs {
		publicBody := readRepoDoc(t, rel)
		for _, leaked := range []string{
			"github.com/maizll",
			"api.github.com/repos/maizll",
			"raw.githubusercontent.com/maizll",
		} {
			if strings.Contains(publicBody, leaked) {
				t.Fatalf("%s still exposes %s", rel, leaked)
			}
		}
	}
	admin := readRepoDoc(t, "admin.md")
	if !strings.Contains(admin, "| 官网页面 | `/system/site-pages` | 仅超管 |") {
		t.Fatal("admin.md is missing 官网页面")
	}
}

func readRepoDoc(t *testing.T, name string) string {
	t.Helper()
	for _, candidate := range []string{
		filepath.Join("..", "..", "docs", name),
		filepath.Join("docs", name),
	} {
		payload, err := os.ReadFile(candidate)
		if err == nil {
			return string(payload)
		}
	}
	t.Fatalf("docs/%s not found", name)
	return ""
}

func TestSplitAndClassifyReleaseNotes(t *testing.T) {
	text := "auth-pro 1.6.8\n\n打开购买窗口时会核对绑定。\n\n优化了顶栏按钮的主色。\n\n修复了保存应用时的误报。"
	notes := splitReleaseNoteParagraphs(text)
	if len(notes) != 3 {
		t.Fatalf("notes=%q", notes)
	}
	if classifyReleaseNote(notes[0]) != siteTagAdded || classifyReleaseNote(notes[1]) != siteTagImproved || classifyReleaseNote(notes[2]) != siteTagFixed {
		t.Fatalf("tags=%s %s %s", classifyReleaseNote(notes[0]), classifyReleaseNote(notes[1]), classifyReleaseNote(notes[2]))
	}
}

func TestChangelogDateUsesDocumentedReleaseDay(t *testing.T) {
	dates := changelogDatesFromMarkdown("## [v1.6.8] 2026-09-27 — 核对绑定\n")
	if dates["1.6.8"] != "2026-09-27" {
		t.Fatalf("dates=%v", dates)
	}
	// 16:30 UTC 在北京时间是次日。
	if got := shanghaiReleaseDate("2026-09-26T16:30:00Z"); got != "2026-09-27" {
		t.Fatalf("shanghai date=%s", got)
	}
	if shanghaiReleaseDate("not-a-date") != "" {
		t.Fatal("invalid timestamp should not invent a date")
	}
}

func TestCommercialSalePlanExposesSiteChangeFields(t *testing.T) {
	unlimited := commercialSalePlanItem(3, "一年", 365, 9900, -1, sql.NullString{})
	if unlimited["free_site_changes"] != -1 || unlimited["site_change_price"] != nil {
		t.Fatalf("unlimited=%v", unlimited)
	}
	priced := commercialSalePlanItem(4, "永久", 0, 19900, 1, sql.NullString{String: "8.50", Valid: true})
	if priced["free_site_changes"] != 1 || priced["site_change_price"] != 8.5 {
		t.Fatalf("priced=%v", priced)
	}
}

func TestEnterpriseTemplatePassesRegistration(t *testing.T) {
	payload, err := readLimitedFile(findDeveloperDocFile(t, "starter/enterprise-template/template.json"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := fillTemplateManifest(sourcePackageManifest{}, payload, "template.json")
	if err != nil {
		t.Fatalf("enterprise template rejected: %v\n%s", err, payload)
	}
	if manifest.Kind != sourceKindTemplate || manifest.ID == "" {
		t.Fatalf("manifest=%+v", manifest)
	}
}
