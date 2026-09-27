package handler

import (
	"database/sql"
	"testing"
)

func TestSiteDocWhitelistSkipsInternalRecords(t *testing.T) {
	allowed := []string{
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
	}
	for _, rel := range denied {
		if siteDocAllowed(rel) {
			t.Fatalf("%s must stay off the public site", rel)
		}
	}
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
