package handler

import (
	"testing"

	"auto_pro/middleware"
)

// 写接口绑定的菜单 name 必须来自现有侧栏种子，避免鉴权词表和菜单树各写一套。
func TestMenuWriteGatesUseSeededMenuNames(t *testing.T) {
	seeded := map[string]struct{}{}
	for _, item := range productMenuSpecs() {
		seeded[item.Name] = struct{}{}
	}

	for _, name := range []string{
		middleware.MenuLicenseList,
		middleware.MenuLicenseCards,
		middleware.MenuLicenseApps,
		middleware.MenuLicenseVersions,
		middleware.MenuAppVersions,
		middleware.MenuLicensePlans,
		middleware.MenuLicenseLogs,
		middleware.MenuUser,
		middleware.MenuAgentList,
		middleware.MenuAgentLevel,
		middleware.MenuAgentQuota,
		middleware.MenuPromotionCampaigns,
		middleware.MenuTicketManage,
		middleware.MenuPiracyTracking,
		middleware.MenuPiracyBlacklist,
		middleware.MenuPiracyAlerts,
		middleware.MenuSourceStationPackages,
		middleware.MenuSourceStationPlugins,
		middleware.MenuSourceStationTemplates,
		middleware.MenuSourceStationApplications,
		middleware.MenuSourceStationCatalog,
		middleware.MenuSourceStationAds,
		middleware.MenuSourceStationSettings,
		middleware.MenuSourceStationEdition,
		middleware.MenuSourceStationStoreOrders,
		middleware.MenuSourceStationStoreLicenses,
		middleware.MenuSourceStationStoreRevenue,
	} {
		if _, ok := seeded[name]; !ok {
			t.Errorf("menu %q is not in productMenuSpecs", name)
		}
	}
}
