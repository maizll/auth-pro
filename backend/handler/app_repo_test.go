package handler

import (
	"strings"
	"testing"
)

func TestSuggestAppRepoName(t *testing.T) {
	if got := suggestAppRepoName("Customer Portal", "app_f93896d80066_5811", "acme"); got != "acme/customer-portal" {
		t.Fatalf("english name: %s", got)
	}
	got := suggestAppRepoName("授权系统", "app_f93896d80066_5811", "acme")
	if !strings.HasPrefix(got, "acme/app-") {
		t.Fatalf("chinese name: %s", got)
	}
}

func TestSplitOwnerRepo(t *testing.T) {
	if _, _, err := splitOwnerRepo("acme"); err == nil || !strings.Contains(err.Error(), "所有者/仓库") {
		t.Fatalf("format: %v", err)
	}
	if _, _, err := splitOwnerRepo("acme/客户"); err == nil || !strings.Contains(err.Error(), "连字符") {
		t.Fatalf("charset: %v", err)
	}
	owner, repo, err := splitOwnerRepo("acme/app-5811")
	if err != nil || owner != "acme" || repo != "app-5811" {
		t.Fatalf("ok: %s/%s %v", owner, repo, err)
	}
}

func TestAppRepoPrefixAndFilter(t *testing.T) {
	if appRepoPrefixFor(sourceKindPlugin, true) != appRepoPrefixPluginPaid {
		t.Fatal("paid plugin prefix")
	}
	if appRepoPrefixFor("client", false) != appRepoPrefixClient {
		t.Fatal("client prefix")
	}
	items := filterReleasesByPrefix([]releaseImportListItem{
		{Tag: "plugins/paid/demo-1.0.0"},
		{Tag: "client/auth-1.8.0"},
		{Tag: "paid-plugin-old"},
	}, appRepoPrefixPluginPaid)
	if len(items) != 1 || items[0].Tag != "plugins/paid/demo-1.0.0" {
		t.Fatalf("filter: %+v", items)
	}
}

func TestProductUpdateAndOfficialReposStaySeparate(t *testing.T) {
	t.Setenv(productUpdateRepoEnv, "")
	t.Setenv(officialUpdateRepoEnv, "")
	if _, _, err := productUpdateRepository(); err == nil {
		t.Fatal("import default should be gone")
	}
	owner, repo, err := officialUpdateRepository()
	if err != nil || owner+"/"+repo != officialUpdateDefaultRepository {
		t.Fatalf("official update changed: %s/%s %v", owner, repo, err)
	}
}

func TestUnboundImportMessage(t *testing.T) {
	useAppRepoMemoryForTest(t)
	if _, _, err := bindingForImport(7, sourceKindPlugin, true); err == nil || err.Error() != appRepoUnboundText {
		t.Fatalf("unbound: %v", err)
	}
}

func TestRepoConflictMessage(t *testing.T) {
	useAppRepoMemoryForTest(t)
	if err := saveAppRepo(appRepoRow{AppID: 1, AppName: "客户甲", Owner: "acme", Repo: "customer-a", Status: appRepoStatusReady, Private: true}); err != nil {
		t.Fatal(err)
	}
	name, taken := repoTakenByOther(2, "acme", "customer-a")
	if !taken || name != "客户甲" {
		t.Fatalf("taken=%v name=%s", taken, name)
	}
}

func TestLegacyReleaseTags(t *testing.T) {
	paid, ok := legacyPaidReleaseTag("paid-plugin-demo-1.0.0")
	if !ok || paid != "plugins/paid/demo-1.0.0" {
		t.Fatalf("plugin: %s %v", paid, ok)
	}
	tpl, ok := legacyPaidReleaseTag("paid-template-home-2.0.0")
	if !ok || tpl != "templates/paid/home-2.0.0" {
		t.Fatalf("template: %s %v", tpl, ok)
	}
	client, ok := legacyClientReleaseTag("v1.8.0")
	if !ok || client != "client/v1.8.0" {
		t.Fatalf("client: %s %v", client, ok)
	}
	if _, ok := legacyClientReleaseTag("paid-plugin-demo-1.0.0"); ok {
		t.Fatal("plugin tag must not land in client")
	}
	name := sourceReleaseAssetName(sourcePackageManifest{ID: "x", Version: "0", Filename: "demo-1.0.0.zip", ReleaseTag: "plugins/paid/demo-1.0.0"})
	if name != "demo-1.0.0.zip" {
		t.Fatalf("asset name: %s", name)
	}
}
