package handler

import (
	"database/sql"
)

// productMenuSpec is the upgrade/install catalog for admin navigation.
// Fresh installs load the same rows from menu_seed.sql; existing DBs upsert by name.
type productMenuSpec struct {
	ID         int64
	ParentName string
	Name       string
	Path       string
	Component  string
	Redirect   string
	Title      string
	Icon       string
	Sort       int
	IsHide     bool
	IsHideTab  bool
	IsFullPage bool
	KeepAlive  bool
	FixedTab   bool
	Roles      []string
}

func productRoles(adminOnly bool) []string {
	if adminOnly {
		return []string{"R_SUPER"}
	}
	return []string{"R_SUPER", "R_ADMIN"}
}

func productMenuSpecs() []productMenuSpec {
	ops := productRoles(false)
	super := productRoles(true)
	return []productMenuSpec{
		{ID: 1, Name: "Dashboard", Path: "/dashboard", Component: "/index/index", Redirect: "/dashboard/console", Title: "menus.dashboard.title", Icon: "ri:home-smile-2-line", Sort: 1, Roles: ops},
		{ID: 101, ParentName: "Dashboard", Name: "Console", Path: "console", Component: "/dashboard/console", Title: "menus.dashboard.console", Sort: 1, IsHide: true, FixedTab: true, Roles: ops},

		{ID: 3, Name: "License", Path: "/license", Component: "/index/index", Redirect: "/license/apps", Title: "menus.license.title", Icon: "ri:apps-line", Sort: 2, Roles: ops},
		{ID: 303, ParentName: "License", Name: "LicenseApps", Path: "apps", Component: "/license/apps", Title: "menus.license.apps", Icon: "ri:apps-2-line", Sort: 1, KeepAlive: true, Roles: ops},
		{ID: 308, ParentName: "License", Name: "LicenseVersions", Path: "versions", Component: "/license/versions", Title: "menus.license.versions", Icon: "ri:git-branch-line", Sort: 2, KeepAlive: true, Roles: ops},
		{ID: 305, ParentName: "License", Name: "AppVersions", Path: "apps/:id/versions", Component: "/license/app-versions", Title: "menus.license.versions", Icon: "ri:git-branch-line", Sort: 99, IsHide: true, Roles: ops},
		{ID: 307, ParentName: "License", Name: "LicensePlans", Path: "plans", Component: "/license/plans", Title: "menus.license.plans", Icon: "ri:price-tag-3-line", Sort: 4, KeepAlive: true, Roles: ops},
		{ID: 306, ParentName: "License", Name: "LicenseCards", Path: "cards", Component: "/license/cards", Title: "menus.license.cards", Icon: "ri:coupon-3-line", Sort: 5, KeepAlive: true, Roles: ops},
		{ID: 302, ParentName: "License", Name: "LicenseList", Path: "list", Component: "/license/list", Title: "menus.license.list", Icon: "ri:file-list-3-line", Sort: 6, KeepAlive: true, Roles: ops},
		{ID: 304, ParentName: "License", Name: "LicenseLogs", Path: "logs", Component: "/license/logs", Title: "menus.license.logs", Icon: "ri:file-text-line", Sort: 7, KeepAlive: true, Roles: ops},
		{ID: 301, ParentName: "License", Name: "LicenseDashboard", Path: "dashboard", Component: "/license/dashboard", Title: "menus.license.overview", Icon: "ri:dashboard-line", Sort: 8, Roles: ops},

		{ID: 4, Name: "Agent", Path: "/admin/agent", Component: "/index/index", Title: "menus.agent.title", Icon: "ri:team-line", Sort: 3, Roles: ops},
		{ID: 401, ParentName: "Agent", Name: "AgentList", Path: "list", Component: "/agent/list", Title: "menus.agent.list", Icon: "ri:user-star-line", Sort: 1, KeepAlive: true, Roles: ops},
		{ID: 402, ParentName: "Agent", Name: "AgentLevel", Path: "level", Component: "/agent/level", Title: "menus.agent.level", Icon: "ri:vip-crown-line", Sort: 2, KeepAlive: true, Roles: ops},
		{ID: 404, ParentName: "Agent", Name: "AgentQuota", Path: "quota", Component: "/agent/quota", Title: "menus.agent.quota", Icon: "ri:key-2-line", Sort: 3, KeepAlive: true, Roles: ops},
		{ID: 403, ParentName: "Agent", Name: "AgentRecharge", Path: "recharge", Component: "/agent/recharge", Title: "menus.agent.finance", Icon: "ri:money-cny-circle-line", Sort: 4, KeepAlive: true, Roles: ops},
		{ID: 405, ParentName: "Agent", Name: "AgentUpgrade", Path: "upgrade", Component: "/agent/upgrade", Title: "menus.agent.upgrade", Icon: "ri:user-shared-line", Sort: 5, KeepAlive: true, Roles: ops},

		{ID: 9, Name: "SourceStation", Path: "/source-station", Component: "/index/index", Redirect: "/source-station/packages", Title: "menus.sourceStation.title", Icon: "ri:database-2-line", Sort: 4, Roles: ops},
		{ID: 901, ParentName: "SourceStation", Name: "SourceStationPackages", Path: "packages", Component: "/source-station/packages", Title: "menus.sourceStation.packages", Icon: "ri:apps-2-line", Sort: 1, KeepAlive: true, Roles: ops},
		{ID: 902, ParentName: "SourceStation", Name: "SourceStationApplications", Path: "applications", Component: "/source-station/applications", Title: "menus.sourceStation.applications", Icon: "ri:user-add-line", Sort: 2, KeepAlive: true, Roles: ops},
		{ID: 903, ParentName: "SourceStation", Name: "SourceStationCatalog", Path: "catalog", Component: "/source-station/catalog", Title: "menus.sourceStation.catalog", Icon: "ri:file-list-3-line", Sort: 3, Roles: ops},
		{ID: 904, ParentName: "SourceStation", Name: "SourceStationAds", Path: "ads", Component: "/source-station/ads", Title: "menus.sourceStation.ads", Icon: "ri:advertisement-line", Sort: 4, KeepAlive: true, Roles: ops},
		{ID: 905, ParentName: "SourceStation", Name: "SourceStationSettings", Path: "settings", Component: "/source-station/settings", Title: "menus.sourceStation.settings", Icon: "ri:settings-3-line", Sort: 5, KeepAlive: true, Roles: ops},
		{ID: 906, ParentName: "SourceStation", Name: "SourceStationPlugins", Path: "plugins", Redirect: "/source-station/packages", Title: "menus.sourceStation.plugins", Icon: "ri:puzzle-line", Sort: 6, IsHide: true, KeepAlive: true, Roles: ops},
		{ID: 907, ParentName: "SourceStation", Name: "SourceStationTemplates", Path: "templates", Redirect: "/source-station/packages?category=home-template", Title: "menus.sourceStation.templates", Icon: "ri:layout-4-line", Sort: 7, IsHide: true, KeepAlive: true, Roles: ops},

		{ID: 5, Name: "Piracy", Path: "/piracy", Component: "/index/index", Title: "menus.security.title", Icon: "ri:shield-flash-line", Sort: 5, Roles: ops},
		{ID: 501, ParentName: "Piracy", Name: "PiracyTracking", Path: "tracking", Component: "/piracy/tracking", Title: "menus.security.tracking", Icon: "ri:spy-line", Sort: 1, KeepAlive: true, Roles: ops},
		{ID: 502, ParentName: "Piracy", Name: "PiracyBlacklist", Path: "blacklist", Component: "/piracy/blacklist", Title: "menus.security.blacklist", Icon: "ri:forbid-line", Sort: 2, KeepAlive: true, Roles: ops},
		{ID: 503, ParentName: "Piracy", Name: "PiracyAlerts", Path: "alerts", Component: "/piracy/alerts", Title: "menus.security.alerts", Icon: "ri:alarm-warning-line", Sort: 3, Roles: ops},
		{ID: 504, ParentName: "Piracy", Name: "PiracyReports", Path: "reports", Component: "/piracy/reports", Title: "menus.security.reports", Icon: "ri:bar-chart-box-line", Sort: 4, Roles: ops},

		{ID: 10, Name: "CustomerService", Path: "/customer-service", Component: "/index/index", Redirect: "/user-manage", Title: "menus.customerService.title", Icon: "ri:customer-service-2-line", Sort: 6, Roles: ops},
		{ID: 201, ParentName: "CustomerService", Name: "User", Path: "/user-manage", Component: "/system/user", Title: "menus.customerService.users", Icon: "ri:user-line", Sort: 1, KeepAlive: true, Roles: ops},
		{ID: 209, ParentName: "CustomerService", Name: "OrderList", Path: "/order-list", Component: "/system/payment-orders", Title: "menus.customerService.orders", Icon: "ri:file-list-3-line", Sort: 2, KeepAlive: true, Roles: ops},
		{ID: 212, ParentName: "CustomerService", Name: "PromotionCampaigns", Path: "/promotion-campaigns", Component: "/promotion-campaigns/index", Title: "menus.customerService.campaigns", Icon: "ri:discount-percent-line", Sort: 3, KeepAlive: true, Roles: ops},
		{ID: 1001, ParentName: "CustomerService", Name: "TicketManage", Path: "/tickets", Component: "/system/tickets", Title: "menus.customerService.tickets", Icon: "ri:question-answer-line", Sort: 4, KeepAlive: true, Roles: ops},

		{ID: 8, Name: "Sdk", Path: "/sdk", Component: "/index/index", Title: "menus.integration.title", Icon: "ri:code-box-line", Sort: 7, Roles: ops},
		{ID: 801, ParentName: "Sdk", Name: "SdkIndex", Path: "index", Component: "/sdk/index", Title: "menus.integration.sdk", Icon: "ri:code-s-slash-line", Sort: 1, KeepAlive: true, Roles: ops},
		{ID: 802, ParentName: "Sdk", Name: "DeveloperDoc", Path: "developer-doc", Component: "/sdk/developer-doc", Title: "menus.integration.docs", Icon: "ri:file-code-line", Sort: 2, KeepAlive: true, Roles: ops},
		{ID: 803, ParentName: "Sdk", Name: "DefaultHomeTemplateDoc", Path: "default-home-template", Component: "/sdk/default-home-template-doc", Title: "menus.integration.templateDoc", Icon: "ri:layout-4-line", Sort: 3, KeepAlive: true, Roles: ops},
		{ID: 210, ParentName: "Sdk", Name: "PluginStore", Path: "/plugin-store", Component: "/plugin-store/index", Title: "menus.integration.store", Icon: "ri:store-2-line", Sort: 4, KeepAlive: true, Roles: super},
		{ID: 211, ParentName: "Sdk", Name: "OnlineUpdate", Path: "/online-update", Component: "/online-update/index", Title: "menus.integration.update", Icon: "ri:download-cloud-2-line", Sort: 5, KeepAlive: true, Roles: super},

		{ID: 2, Name: "System", Path: "/system", Component: "/index/index", Title: "menus.system.title", Icon: "ri:settings-3-line", Sort: 8, Roles: ops},
		{ID: 202, ParentName: "System", Name: "Role", Path: "role", Component: "/system/role", Title: "menus.system.role", Icon: "ri:shield-user-line", Sort: 1, KeepAlive: true, Roles: super},
		{ID: 204, ParentName: "System", Name: "Menus", Path: "menu", Component: "/system/menu", Title: "menus.system.menu", Icon: "ri:menu-2-line", Sort: 2, KeepAlive: true, Roles: super},
		{ID: 205, ParentName: "System", Name: "SystemConfig", Path: "config", Component: "/system/config", Title: "menus.system.config", Icon: "ri:settings-3-line", Sort: 3, KeepAlive: true, Roles: super},
		{ID: 208, ParentName: "System", Name: "EpayConfig", Path: "epay-config", Component: "/system/epay-config", Title: "menus.system.epayConfig", Icon: "ri:bank-card-line", Sort: 4, KeepAlive: true, Roles: super},
		{ID: 206, ParentName: "System", Name: "MailConfig", Path: "mail-config", Component: "/system/mail-config", Title: "menus.system.mailConfig", Icon: "ri:mail-settings-line", Sort: 5, KeepAlive: true, Roles: super},
		{ID: 207, ParentName: "System", Name: "MailLogs", Path: "mail-logs", Component: "/system/mail-logs", Title: "menus.system.mailLogs", Icon: "ri:mail-check-line", Sort: 6, KeepAlive: true, Roles: super},
		{ID: 203, ParentName: "System", Name: "UserCenter", Path: "user-center", Component: "/system/user-center", Title: "menus.system.userCenter", Icon: "ri:user-settings-line", Sort: 7, KeepAlive: true, IsHideTab: true, Roles: ops},

		{ID: 6, Name: "Result", Path: "/result", Component: "/index/index", Title: "menus.result.title", Icon: "ri:checkbox-circle-line", Sort: 90, IsHide: true, Roles: super},
		{ID: 601, ParentName: "Result", Name: "ResultSuccess", Path: "success", Component: "/result/success", Title: "menus.result.success", Icon: "ri:checkbox-circle-line", Sort: 1, KeepAlive: true, IsHide: true, Roles: super},
		{ID: 602, ParentName: "Result", Name: "ResultFail", Path: "fail", Component: "/result/fail", Title: "menus.result.fail", Icon: "ri:close-circle-line", Sort: 2, KeepAlive: true, IsHide: true, Roles: super},

		{ID: 7, Name: "Exception", Path: "/exception", Component: "/index/index", Title: "menus.exception.title", Icon: "ri:error-warning-line", Sort: 91, IsHide: true, Roles: super},
		{ID: 701, ParentName: "Exception", Name: "Exception403", Path: "403", Component: "/exception/403", Title: "menus.exception.forbidden", Sort: 1, KeepAlive: true, IsHide: true, IsHideTab: true, IsFullPage: true, Roles: super},
		{ID: 702, ParentName: "Exception", Name: "Exception404", Path: "404", Component: "/exception/404", Title: "menus.exception.notFound", Sort: 2, KeepAlive: true, IsHide: true, IsHideTab: true, IsFullPage: true, Roles: super},
		{ID: 703, ParentName: "Exception", Name: "Exception500", Path: "500", Component: "/exception/500", Title: "menus.exception.serverError", Sort: 3, KeepAlive: true, IsHide: true, IsHideTab: true, IsFullPage: true, Roles: super},
	}
}

func topLevelProductMenuNames() []string {
	names := make([]string, 0)
	for _, spec := range productMenuSpecs() {
		if spec.ParentName == "" && !spec.IsHide {
			names = append(names, spec.Name)
		}
	}
	return names
}

func productMenuRoleIndex() map[string][]string {
	out := make(map[string][]string, 64)
	for _, spec := range productMenuSpecs() {
		out[spec.Name] = spec.Roles
	}
	return out
}

func needsWorkflowMenuMigrationState(hasSourceStation, hasCustomerService bool, _ int64) bool {
	return !hasSourceStation || !hasCustomerService
}

func needsWorkflowMenuMigration(db *sql.DB) bool {
	var source, customer int
	_ = db.QueryRow("SELECT COUNT(*) FROM menus WHERE name = 'SourceStation'").Scan(&source)
	_ = db.QueryRow("SELECT COUNT(*) FROM menus WHERE name = 'CustomerService'").Scan(&customer)
	return needsWorkflowMenuMigrationState(source > 0, customer > 0, 0)
}

func mergeMenuRow(existing menuRow, spec productMenuSpec, parentID int64, migrate bool) menuRow {
	out := existing
	if out.ID == 0 {
		out.ID = spec.ID
	}
	out.Name = spec.Name
	out.ParentID = parentID
	out.Path = spec.Path
	out.Component = spec.Component
	if migrate || out.Redirect == "" {
		out.Redirect = spec.Redirect
	}
	if spec.IsHide {
		out.IsHide = true
	} else if migrate || existing.ID == 0 {
		out.IsHide = spec.IsHide
	}
	if migrate || existing.ID == 0 {
		out.Title = spec.Title
		out.Icon = spec.Icon
		out.Sort = spec.Sort
		out.IsHideTab = spec.IsHideTab
		out.IsFullPage = spec.IsFullPage
		out.KeepAlive = spec.KeepAlive
		out.FixedTab = spec.FixedTab
	}
	out.Roles = spec.Roles
	return out
}

func upsertMenuRows(existing []menuRow, specs []productMenuSpec, migrate bool) []menuRow {
	out := append([]menuRow(nil), existing...)
	byName := make(map[string]int, len(out)+len(specs))
	ids := make(map[string]int64, len(out)+len(specs))
	for i, row := range out {
		byName[row.Name] = i
		ids[row.Name] = row.ID
	}
	for _, spec := range specs {
		parentID := int64(0)
		if spec.ParentName != "" {
			if id, ok := ids[spec.ParentName]; ok {
				parentID = id
			} else {
				parentID = specParentID(specs, spec.ParentName)
			}
		}
		if idx, ok := byName[spec.Name]; ok {
			out[idx] = mergeMenuRow(out[idx], spec, parentID, migrate)
			ids[spec.Name] = out[idx].ID
			continue
		}
		row := mergeMenuRow(menuRow{ID: spec.ID}, spec, parentID, true)
		out = append(out, row)
		byName[spec.Name] = len(out) - 1
		ids[spec.Name] = row.ID
	}
	return out
}

func specParentID(specs []productMenuSpec, name string) int64 {
	for _, spec := range specs {
		if spec.Name == name {
			return spec.ID
		}
	}
	return 0
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func ensureProductMenus(db *sql.DB) {
	if db == nil {
		return
	}
	removeHomeTemplateMenu(db)
	cleanupPurchaseLimitCampaignMenu(db)
	_, _ = db.Exec(`
		DELETE rm FROM role_menus rm
		INNER JOIN menus m ON m.id = rm.menu_id
		WHERE m.name = 'PaymentOrders'
	`)
	_, _ = db.Exec("DELETE FROM menus WHERE name = 'PaymentOrders'")

	migrate := needsWorkflowMenuMigration(db)
	for _, spec := range productMenuSpecs() {
		parentID := int64(0)
		if spec.ParentName != "" {
			_ = db.QueryRow("SELECT id FROM menus WHERE name = ? LIMIT 1", spec.ParentName).Scan(&parentID)
		}
		upsertProductMenu(db, spec, parentID, migrate)
		bindProductMenuRoles(db, spec)
	}
	propagateChildRolesToParents(db)
	hideDemoMenus(db)
	_, _ = db.Exec("UPDATE menus SET path = '/admin/agent' WHERE name = 'Agent' AND path IN ('/agent', 'agent')")
}

func upsertProductMenu(db *sql.DB, spec productMenuSpec, parentID int64, migrate bool) {
	if migrate {
		_, _ = db.Exec(`
			INSERT INTO menus (id, parent_id, name, path, component, redirect, title, icon, sort, is_hide, is_hide_tab, is_full_page, keep_alive, fixed_tab, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
			ON DUPLICATE KEY UPDATE
				parent_id = VALUES(parent_id), path = VALUES(path), component = VALUES(component), redirect = VALUES(redirect),
				title = VALUES(title), icon = VALUES(icon), sort = VALUES(sort),
				is_hide = VALUES(is_hide), is_hide_tab = VALUES(is_hide_tab), is_full_page = VALUES(is_full_page),
				keep_alive = VALUES(keep_alive), fixed_tab = VALUES(fixed_tab), enabled = 1
		`, spec.ID, parentID, spec.Name, spec.Path, spec.Component, spec.Redirect, spec.Title, spec.Icon, spec.Sort,
			boolToInt(spec.IsHide), boolToInt(spec.IsHideTab), boolToInt(spec.IsFullPage), boolToInt(spec.KeepAlive), boolToInt(spec.FixedTab))
		return
	}

	_, _ = db.Exec(`
		INSERT INTO menus (id, parent_id, name, path, component, redirect, title, icon, sort, is_hide, is_hide_tab, is_full_page, keep_alive, fixed_tab, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE
			path = VALUES(path),
			component = VALUES(component),
			redirect = IF(redirect IS NULL OR redirect = '', VALUES(redirect), redirect),
			is_hide = IF(VALUES(is_hide) = 1, 1, is_hide)
	`, spec.ID, parentID, spec.Name, spec.Path, spec.Component, spec.Redirect, spec.Title, spec.Icon, spec.Sort,
		boolToInt(spec.IsHide), boolToInt(spec.IsHideTab), boolToInt(spec.IsFullPage), boolToInt(spec.KeepAlive), boolToInt(spec.FixedTab))
}

func bindProductMenuRoles(db *sql.DB, spec productMenuSpec) {
	roles := spec.Roles
	if len(roles) == 0 {
		roles = []string{"R_SUPER"}
	}
	if !containsString(roles, "R_SUPER") {
		roles = append([]string{"R_SUPER"}, roles...)
	}
	args := make([]any, 0, 1+len(roles))
	placeholders := make([]string, 0, len(roles))
	var menuID int64
	if err := db.QueryRow("SELECT id FROM menus WHERE name = ? LIMIT 1", spec.Name).Scan(&menuID); err != nil || menuID == 0 {
		return
	}
	args = append(args, menuID)
	for _, role := range roles {
		placeholders = append(placeholders, "?")
		args = append(args, role)
	}
	_, _ = db.Exec(`
		INSERT IGNORE INTO role_menus (role_id, menu_id)
		SELECT id, ? FROM roles WHERE enabled = 1 AND role_code IN (`+joinPlaceholders(placeholders)+`)
	`, args...)
}

func propagateChildRolesToParents(db *sql.DB) {
	_, _ = db.Exec(`
		INSERT IGNORE INTO role_menus (role_id, menu_id)
		SELECT rm.role_id, p.id
		FROM menus c
		INNER JOIN menus p ON p.id = c.parent_id AND p.id > 0
		INNER JOIN role_menus rm ON rm.menu_id = c.id
	`)
}

func hideDemoMenus(db *sql.DB) {
	_, _ = db.Exec(`
		UPDATE menus SET is_hide = 1
		WHERE name IN (
			'Result', 'ResultSuccess', 'ResultFail',
			'Exception', 'Exception403', 'Exception404', 'Exception500'
		)
	`)
}

func joinPlaceholders(parts []string) string {
	if len(parts) == 0 {
		return "NULL"
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "," + parts[i]
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
