package main

import (
	"os"
	"strings"
	"testing"
)

// 路由绑定不经过中间件单测里的临时路由。这里锁住 main 与源站注册处的实际写法，
// 避免 DELETE /api/license/:id 再次落回只检查 admin 角色的 secured 分组。
func TestSensitiveAdminWritesStayOnMenuGroups(t *testing.T) {
	mainSource := readSource(t, "main.go")
	sourceStation := readSource(t, "handler/source_station.go")

	required := []string{
		`licenseList.POST("/license/create", handler.LicenseCreate)`,
		`licenseList.PUT("/license/:id", handler.LicenseUpdate)`,
		`licenseList.PUT("/license/:id/toggle", handler.LicenseToggle)`,
		`licenseList.DELETE("/license/:id", handler.LicenseDelete)`,
		`licenseList.DELETE("/license/:id/sites/:siteId", handler.AdminLicenseSiteUnbind)`,
		`userWrites.DELETE("/user/:id", handler.AdminUserDelete)`,
		`agentList.POST("/agent/create", handler.AgentCreate)`,
		`agentList.DELETE("/agent/:id", handler.AgentDelete)`,
		`licenseApps.DELETE("/app/:id", handler.AppDelete)`,
		`licensePlans.DELETE("/plan/:id", handler.PlanDelete)`,
		`quotas.DELETE("/quota/:id", handler.QuotaDelete)`,
		`piracyBlacklist.DELETE("/piracy/blacklist/:id", handler.PiracyBlacklistDelete)`,
		`superSecured.POST("/user/:id/impersonate", handler.AdminImpersonateUser)`,
		`superSecured.PUT("/role/:id/menus", handler.RoleUpdateMenus)`,
	}
	for _, snippet := range required {
		if !strings.Contains(mainSource, snippet) {
			t.Errorf("main.go missing menu-gated route: %s", snippet)
		}
	}
	if strings.Contains(mainSource, `secured.DELETE("/license/:id", handler.LicenseDelete)`) {
		t.Error("license delete is still registered on the plain secured group")
	}

	for _, snippet := range []string{
		`applications.POST("/applications/:id/approve", AdminSourceDeveloperApprove)`,
		`ads.DELETE("/advertisements/:id", AdminSourceAdvertisementDelete)`,
		`pluginWrites.POST("/plugins/:id/deprecate", AdminSourcePluginDeprecate)`,
	} {
		if !strings.Contains(sourceStation, snippet) {
			t.Errorf("source station missing menu-gated route: %s", snippet)
		}
	}
}

func readSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
