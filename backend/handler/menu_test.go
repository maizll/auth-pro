package handler

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestMenuSeedSQLFollowsWorkflowNav(t *testing.T) {
	seed := menuSeedSQL
	required := []string{
		"Dashboard", "License", "Agent", "SourceStation", "Piracy",
		"CustomerService", "Sdk", "System",
		"LicensePlans", "LicenseVersions", "LicenseCards",
		"SourceStationPackages", "SourceStationApplications",
		"SourceStationCatalog", "SourceStationAds", "SourceStationSettings",
		"TicketManage", "PluginStore", "OnlineUpdate",
		"User", "OrderList", "PromotionCampaigns",
	}
	for _, name := range required {
		if !strings.Contains(seed, "'"+name+"'") {
			t.Errorf("menu_seed.sql missing product menu %s", name)
		}
	}

	if !strings.Contains(seed, "'/admin/agent'") {
		t.Fatal("Agent path must be /admin/agent to match frontend workflow routes")
	}
	if strings.Contains(seed, "'/agent'") && !strings.Contains(seed, "'/admin/agent'") {
		t.Fatal("legacy Agent path /agent must not remain as the group path")
	}

	for _, name := range []string{"User", "OrderList", "PromotionCampaigns", "TicketManage"} {
		if seedHasTopLevelMenu(seed, name) {
			t.Errorf("%s must sit under CustomerService, not as a top-level menu", name)
		}
	}
	for _, name := range []string{"PluginStore", "OnlineUpdate"} {
		if !seedHasTopLevelMenu(seed, name) {
			t.Errorf("%s must be a top-level menu (parent_id=0), not nested under Sdk", name)
		}
	}
	if sdkChildren := seedChildNames(seed, 8); !sameStringSet(sdkChildren, []string{"SdkIndex", "DeveloperDoc", "DefaultHomeTemplateDoc"}) {
		t.Errorf("Sdk children = %v, want SDK/docs/template only", sdkChildren)
	}
}

func TestMenuSeedHidesDemoRoutes(t *testing.T) {
	seed := menuSeedSQL
	for _, name := range []string{"Result", "Exception"} {
		if !seedMenuIsHidden(seed, name) {
			t.Errorf("demo menu %s must be hidden (is_hide=1) so it is not in the default sidebar", name)
		}
	}
}

func TestProductMenuSpecFollowsWorkflowOrder(t *testing.T) {
	got := topLevelProductMenuNames()
	want := []string{
		"Dashboard", "License", "Agent", "SourceStation", "Piracy",
		"CustomerService", "Sdk", "PluginStore", "OnlineUpdate", "System",
	}
	if len(got) < len(want) {
		t.Fatalf("top-level product menus = %v, want at least %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("top-level[%d] = %s, want %s (full=%v)", i, got[i], name, got)
		}
	}
	hidden := map[string]bool{}
	for _, spec := range productMenuSpecs() {
		if spec.ParentName == "" && spec.IsHide {
			hidden[spec.Name] = true
		}
	}
	if !hidden["Result"] || !hidden["Exception"] {
		t.Fatalf("Result/Exception must be hidden top-level demo menus, hidden=%v", hidden)
	}
}

func TestProductMenuSpecIncludesWorkflowPages(t *testing.T) {
	byName := map[string]productMenuSpec{}
	for _, spec := range productMenuSpecs() {
		if _, exists := byName[spec.Name]; exists {
			t.Fatalf("duplicate menu name %s", spec.Name)
		}
		byName[spec.Name] = spec
	}

	wantParent := map[string]string{
		"LicensePlans":           "License",
		"LicenseVersions":        "License",
		"SourceStationPackages":  "SourceStation",
		"TicketManage":           "CustomerService",
		"User":                   "CustomerService",
		"OrderList":              "CustomerService",
		"PromotionCampaigns":     "CustomerService",
		"PluginStore":            "",
		"OnlineUpdate":           "",
		"DeveloperDoc":           "Sdk",
		"DefaultHomeTemplateDoc": "Sdk",
		"AgentList":              "Agent",
		"PiracyTracking":         "Piracy",
		"Menus":                  "System",
		"AppVersions":            "License",
		"SourceStationPlugins":   "SourceStation",
		"SourceStationTemplates": "SourceStation",
		"Console":                "Dashboard",
	}
	for name, parent := range wantParent {
		spec, ok := byName[name]
		if !ok {
			t.Errorf("missing product menu %s", name)
			continue
		}
		if spec.ParentName != parent {
			t.Errorf("%s parent = %q, want %q", name, spec.ParentName, parent)
		}
	}

	if byName["Agent"].Path != "/admin/agent" {
		t.Errorf("Agent path = %s, want /admin/agent", byName["Agent"].Path)
	}
	if !byName["AppVersions"].IsHide || !byName["Console"].IsHide {
		t.Fatal("AppVersions and Console must stay hidden detail routes")
	}
	if !containsString(byName["License"].Roles, "R_SUPER") || !containsString(byName["SourceStation"].Roles, "R_SUPER") {
		t.Fatal("super admin must keep access to 授权 and 源站")
	}
}

func TestMenuSeedSQLMatchesProductSpec(t *testing.T) {
	seed := menuSeedSQL
	for _, spec := range productMenuSpecs() {
		if !strings.Contains(seed, "'"+spec.Name+"'") {
			t.Errorf("seed missing spec menu %s", spec.Name)
		}
		if spec.Path != "" && !strings.Contains(seed, "'"+spec.Path+"'") {
			t.Errorf("seed missing path %s for %s", spec.Path, spec.Name)
		}
	}
}

func TestProductMenuSpecOmitsSystemMonitor(t *testing.T) {
	for _, spec := range productMenuSpecs() {
		if spec.Name == "SystemMonitor" || spec.Component == "/system/monitor" || spec.Title == "menus.system.monitor" {
			t.Fatalf("system monitor menu must be removed, found %+v", spec)
		}
	}
	if strings.Contains(menuSeedSQL, "SystemMonitor") || strings.Contains(menuSeedSQL, "/system/monitor") || strings.Contains(menuSeedSQL, "menus.system.monitor") {
		t.Fatal("menu seed must not insert the system monitor page")
	}
}

func TestProductMenuSpecKeepsSinglePaymentConfigEntry(t *testing.T) {
	epay := 0
	for _, spec := range productMenuSpecs() {
		if spec.Name == "AlipayF2FConfig" || spec.Path == "alipay-f2f-config" || spec.Component == "/system/alipay-f2f-config" {
			t.Fatalf("payment config must stay on EpayConfig, found parallel menu %+v", spec)
		}
		if spec.Name == "EpayConfig" {
			epay++
			if spec.Path != "epay-config" || spec.Component != "/system/epay-config" {
				t.Fatalf("EpayConfig path/component = %s %s", spec.Path, spec.Component)
			}
		}
	}
	if epay != 1 {
		t.Fatalf("EpayConfig count = %d, want 1", epay)
	}
	if strings.Contains(menuSeedSQL, "AlipayF2FConfig") || strings.Contains(menuSeedSQL, "alipay-f2f-config") {
		t.Fatal("menu seed must not insert the Alipay F2F sidebar entry")
	}
}

func TestBuildMenuTreeExposesProcessorFields(t *testing.T) {
	rows := []menuRow{
		{
			ID: 3, ParentID: 0, Name: "License", Path: "/license", Component: "/index/index",
			Title: "menus.license.title", Icon: "ri:apps-line", Sort: 2, Roles: []string{"R_SUPER", "R_ADMIN"},
		},
		{
			ID: 303, ParentID: 3, Name: "LicenseApps", Path: "apps", Component: "/license/apps",
			Title: "menus.license.apps", Icon: "ri:apps-2-line", Sort: 1, KeepAlive: true, Roles: []string{"R_SUPER", "R_ADMIN"},
		},
		{
			ID: 305, ParentID: 3, Name: "AppVersions", Path: "apps/:id/versions", Component: "/license/app-versions",
			Title: "menus.license.versions", Icon: "ri:git-branch-line", Sort: 99, IsHide: true, Roles: []string{"R_SUPER", "R_ADMIN"},
		},
	}

	tree := buildMenuTree(rows, 0)
	if len(tree) != 1 {
		t.Fatalf("tree len = %d, want 1", len(tree))
	}
	raw, err := json.Marshal(tree[0])
	if err != nil {
		t.Fatal(err)
	}
	var node map[string]any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"name", "path", "component", "meta", "children"} {
		if _, ok := node[key]; !ok {
			t.Errorf("menu JSON missing %s: %s", key, raw)
		}
	}
	meta, _ := node["meta"].(map[string]any)
	if meta["title"] != "应用授权" {
		t.Errorf("meta.title = %v, want 应用授权", meta["title"])
	}
	if meta["icon"] != "ri:apps-line" {
		t.Errorf("meta.icon = %v", meta["icon"])
	}
	roles, _ := meta["roles"].([]any)
	if len(roles) == 0 {
		t.Fatalf("meta.roles missing in %s", raw)
	}

	if len(tree[0].Children) != 2 {
		t.Fatalf("children = %d, want 2", len(tree[0].Children))
	}
	var hidden *menuResponse
	for _, child := range tree[0].Children {
		if child.Name == "AppVersions" {
			hidden = child
		}
	}
	if hidden == nil || !hidden.Meta.IsHide {
		t.Fatal("hidden AppVersions must keep isHide for MenuProcessor sidebar filter")
	}
}

func TestUpsertMovesPluginStoreAndOnlineUpdateToTopLevel(t *testing.T) {
	existing := []menuRow{
		{ID: 8, ParentID: 0, Name: "Sdk", Title: "接入开发", Sort: 7},
		{ID: 210, ParentID: 8, Name: "PluginStore", Title: "我的商店", Icon: "ri:star-line", Sort: 4},
		{ID: 211, ParentID: 8, Name: "OnlineUpdate", Title: "自定义更新", Sort: 5},
		{ID: 2, ParentID: 0, Name: "System", Title: "我的系统", Icon: "ri:settings-3-line", Sort: 8},
		{ID: 801, ParentID: 8, Name: "SdkIndex", Title: "SDK 示例", Sort: 1},
	}

	got := upsertMenuRows(existing, productMenuSpecs(), false)
	byName := map[string]menuRow{}
	for _, row := range got {
		byName[row.Name] = row
	}

	if byName["PluginStore"].ParentID != 0 || byName["OnlineUpdate"].ParentID != 0 {
		t.Fatalf("store/update parent_id = %d/%d, want 0 (top-level)", byName["PluginStore"].ParentID, byName["OnlineUpdate"].ParentID)
	}
	if byName["PluginStore"].Title != "我的商店" || byName["OnlineUpdate"].Title != "自定义更新" {
		t.Fatalf("custom titles must stick: store=%q update=%q", byName["PluginStore"].Title, byName["OnlineUpdate"].Title)
	}
	if byName["System"].Title != "我的系统" || byName["System"].Sort != 8 {
		t.Fatalf("unrelated System title/sort must stick: %+v", byName["System"])
	}
	if byName["SdkIndex"].ParentID != 8 {
		t.Fatalf("SdkIndex must stay under Sdk, parent=%d", byName["SdkIndex"].ParentID)
	}
}

func TestInvalidMenuParentRejectsSelfAndDescendant(t *testing.T) {
	parentByID := map[int64]int64{8: 0, 801: 8, 802: 8}
	if invalidMenuParent(8, 8, parentByID) != true {
		t.Fatal("cannot parent Sdk to itself")
	}
	if invalidMenuParent(8, 801, parentByID) != true {
		t.Fatal("cannot parent Sdk to its child")
	}
	if invalidMenuParent(801, 0, parentByID) != false {
		t.Fatal("top-level parent is valid")
	}
	if invalidMenuParent(801, 2, parentByID) != false {
		t.Fatal("sibling top-level parent is valid")
	}
}

func TestResolveMenuTitleUsesChinese(t *testing.T) {
	if got := resolveMenuTitle("menus.integration.store"); got != "应用商店" {
		t.Fatalf("store title = %q", got)
	}
	if got := resolveMenuTitle("我的商店"); got != "我的商店" {
		t.Fatalf("custom title should pass through, got %q", got)
	}
	if !isDemoProductMenu("Result") || !isDemoProductMenu("Exception404") {
		t.Fatal("demo menus must be recognized")
	}
	if isDemoProductMenu("PluginStore") {
		t.Fatal("product menus are not demo")
	}
}

func TestNeedsWorkflowMenuMigration(t *testing.T) {
	if !needsWorkflowMenuMigrationState(false, false, 0) {
		t.Fatal("legacy DB without SourceStation/CustomerService must migrate")
	}
	if needsWorkflowMenuMigrationState(true, true, 10) {
		t.Fatal("already-migrated DB must not rewrite admin title/sort on every request")
	}
}

func TestMergeMenuPreservesCustomTitleAfterMigration(t *testing.T) {
	spec := productMenuSpec{
		Name: "License", Path: "/license", Title: "menus.license.title", Icon: "ri:apps-line", Sort: 2,
	}
	existing := menuRow{ID: 3, Name: "License", Path: "/license", Title: "我的授权", Icon: "ri:star-line", Sort: 99}

	migrated := mergeMenuRow(existing, spec, 0, true)
	if migrated.Title != spec.Title || migrated.Sort != spec.Sort {
		t.Fatalf("migration should align title/sort, got title=%s sort=%d", migrated.Title, migrated.Sort)
	}

	preserved := mergeMenuRow(existing, spec, 0, false)
	if preserved.Title != "我的授权" || preserved.Sort != 99 || preserved.Icon != "ri:star-line" {
		t.Fatalf("after migration, admin title/sort/icon must stick: %+v", preserved)
	}
	if preserved.Path != spec.Path {
		t.Fatalf("structural path still updates: %s", preserved.Path)
	}
}

func TestUpsertMenusByNameDoesNotDuplicate(t *testing.T) {
	existing := []menuRow{
		{ID: 201, ParentID: 0, Name: "User", Path: "/user-manage", Title: "用户管理"},
	}
	specs := []productMenuSpec{
		{ID: 10, Name: "CustomerService", Path: "/customer-service", Title: "menus.customerService.title", Sort: 6},
		{ID: 201, ParentName: "CustomerService", Name: "User", Path: "/user-manage", Title: "menus.customerService.users", Sort: 1},
		{ID: 201, ParentName: "CustomerService", Name: "User", Path: "/user-manage", Title: "menus.customerService.users", Sort: 1},
	}
	got := upsertMenuRows(existing, specs, true)
	var users int
	for _, row := range got {
		if row.Name == "User" {
			users++
			if row.ParentID == 0 {
				t.Fatal("User must move under CustomerService without inserting a second row")
			}
		}
	}
	if users != 1 {
		t.Fatalf("User rows = %d, want 1", users)
	}
}

func seedHasTopLevelMenu(seed, name string) bool {
	for _, line := range strings.Split(seed, "\n") {
		if !strings.Contains(line, "'"+name+"'") {
			continue
		}
		fields := splitSQLValueLine(line)
		if len(fields) >= 3 && fields[1] == "0" && fields[2] == name {
			return true
		}
	}
	return false
}

func seedMenuIsHidden(seed, name string) bool {
	for _, line := range strings.Split(seed, "\n") {
		if !strings.Contains(line, "'"+name+"'") {
			continue
		}
		if strings.Contains(line, "is_hide") {
			return strings.Contains(line, ", 1,") || strings.Contains(line, ",1,") || strings.Contains(line, ", 1)")
		}
		// Hidden top-level demo rows encode is_hide in the values list after sort.
		fields := splitSQLValueLine(line)
		if len(fields) >= 10 && fields[2] == name {
			return fields[9] == "1"
		}
	}
	return false
}

func seedChildNames(seed string, parentID int) []string {
	var names []string
	wantParent := strconv.Itoa(parentID)
	for _, line := range strings.Split(seed, "\n") {
		fields := splitSQLValueLine(line)
		if len(fields) >= 3 && fields[1] == wantParent && fields[2] != "" {
			names = append(names, fields[2])
		}
	}
	return names
}

func splitSQLValueLine(line string) []string {
	start := strings.Index(line, "(")
	end := strings.LastIndex(line, ")")
	if start < 0 || end <= start {
		return nil
	}
	raw := strings.Split(line[start+1:end], ",")
	fields := make([]string, 0, len(raw))
	for _, part := range raw {
		fields = append(fields, strings.Trim(strings.TrimSpace(part), "'`"))
	}
	return fields
}
