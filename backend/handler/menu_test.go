package handler

import (
	"encoding/json"
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
		if seedHasTopLevelMenu(seed, name) {
			t.Errorf("%s must sit under Sdk, not as a top-level menu", name)
		}
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
		"CustomerService", "Sdk", "System",
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
		"PluginStore":            "Sdk",
		"OnlineUpdate":           "Sdk",
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
	if meta["title"] != "menus.license.title" {
		t.Errorf("meta.title = %v", meta["title"])
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
