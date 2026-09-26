package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"auto_pro/appstore"
	"auto_pro/config"
	"auto_pro/handler"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	handler.DispatchStoreKeygen(os.Args[1:])

	// 历史安装可能把数据库口令写成 0644。进程起来先收紧，不等到下次保存配置。
	if err := config.EnsureDBConfigPermissions(); err != nil {
		log.Printf("tighten db.json permissions failed: %v", err)
	}

	r := gin.Default()
	appStoreServer := appstore.NewServer(handler.NewAppStoreTemplateRepository())

	// CORS
	r.Use(middleware.Cors())

	// API 路由
	api := r.Group("/api")
	{
		appStoreServer.RegisterAdminRoutes(api)

		// 安装状态检查保持公开（前端路由守卫依赖）
		install := api.Group("/install")
		{
			install.GET("/status", handler.InstallStatus)

			// 安装写接口：已安装后整体关闭；未安装时拒绝跨域调用
			installProtected := install.Group("/")
			installProtected.Use(middleware.InstallGuard())
			{
				installProtected.POST("/test-db", handler.InstallTestDB)
				installProtected.POST("/init-tables", handler.InstallInitTables)
				installProtected.POST("/create-admin", handler.InstallCreateAdmin)
			}
		}

		// 公开系统配置（登录前可读取品牌信息）
		api.GET("/system-config/public", handler.PublicSystemConfig)
		api.GET("/system/version", handler.SystemVersion)

		// 公开授权校验（SDK 调用，无需后台登录）
		api.POST("/license/verify", handler.LicenseVerify)
		// 应用客户端使用签名授权检查版本，并通过短期令牌下载本地更新包。
		api.POST("/app/version/check", handler.AppVersionCheck)
		api.GET("/app/version/download", handler.AppVersionDownload)

		// 易支付回调（支付网关调用，无需登录）
		api.Any("/payment/easypay/notify", handler.EpayNotify)
		api.Any("/payment/easypay/return", handler.EpayReturn)

		// 易支付 V2 回调（支付网关调用，无需登录）
		api.Any("/payment/easypay-v2/notify", handler.EpayV2Notify)
		api.Any("/payment/easypay-v2/return", handler.EpayV2Return)

		// 支付渠道插件异步通知（如支付宝当面付 /api/payment/alipay-f2f/notify）
		api.Any("/payment/:channel/notify", handler.PaymentChannelNotify)

		// 快瞳 / 腾讯云增强人脸拍照提交（扫码手机端调用，token 即凭证，无需登录）
		api.POST("/realname/face/submit", handler.RealnameFaceSubmit)

		// 认证相关（无需鉴权）
		auth := api.Group("/auth")
		{
			auth.POST("/login", handler.Login)
		}

		// 代理端（无需管理员鉴权）
		agentAuth := api.Group("/agent-panel")
		{
			agentAuth.POST("/login", handler.AgentPanelLogin)
		}

		// 代理端（需鉴权）
		agentSecured := api.Group("/agent-panel")
		agentSecured.Use(middleware.JWTAuth(), middleware.RequireFreshPassword("agents"))
		{
			agentSecured.GET("/apps", handler.AgentPanelAppList)
			agentSecured.GET("/apps/purchase", handler.AgentPanelPurchaseApps)
			agentSecured.GET("/users/options", handler.AgentPanelUserOptions)
			agentSecured.GET("/licenses", handler.AgentPanelLicenseList)
			agentSecured.POST("/cards/redeem", handler.AgentLicenseCardRedeem)
			agentSecured.PUT("/licenses/:id", handler.AgentPanelLicenseUpdate)
			agentSecured.POST("/licenses/:id/refresh-key", handler.AgentPanelLicenseRefreshKey)
			agentSecured.GET("/licenses/:id/sites", handler.AgentLicenseSiteList)
			agentSecured.DELETE("/licenses/:id/sites/:siteId", handler.AgentLicenseSiteUnbind)
			agentSecured.GET("/balance", handler.AgentPanelBalance)
			agentSecured.GET("/profile", handler.AgentPanelProfile)
			agentSecured.PUT("/profile", handler.AgentPanelUpdateProfile)
			agentSecured.POST("/change-password", handler.AgentPanelChangePassword)
			agentSecured.POST("/realname/init", handler.AgentRealnameInit)
			agentSecured.GET("/realname/query", handler.AgentRealnameQuery)
			agentSecured.GET("/dashboard/stats", handler.AgentPanelStats)
			agentSecured.GET("/dashboard/info", handler.AgentPanelInfo)
			agentSecured.GET("/dashboard/trend", handler.AgentPanelTrend)
			agentSecured.GET("/dashboard/app-dist", handler.AgentPanelAppDist)
			agentSecured.GET("/dashboard/recent-licenses", handler.AgentPanelRecentLicenses)
			agentSecured.GET("/finance/overview", handler.AgentPanelFinanceOverview)
			agentSecured.GET("/finance/quotas", handler.AgentPanelFinanceQuotas)
			agentSecured.GET("/finance/transactions", handler.AgentPanelFinanceTransactions)
			agentSecured.GET("/recharge/options", handler.AgentPanelRechargeOptions)
			agentSecured.POST("/recharge/orders", handler.AgentPanelRechargeCreate)
			agentSecured.GET("/recharge/orders/:orderNo", handler.AgentPanelRechargeStatus)
			agentSecured.GET("/purchase/pay-options", handler.AgentPanelPurchasePayOptions)
			agentSecured.POST("/purchase", handler.AgentPanelPurchase)
			agentSecured.GET("/purchase/orders/:orderNo", handler.AgentPanelPurchaseOrderStatus)
			agentSecured.GET("/licenses/:id/versions", handler.PanelLicenseVersions)
			agentSecured.POST("/licenses/:id/versions/:versionId/download-url", handler.PanelLicenseVersionDownloadURL)
			agentSecured.POST("/tickets", handler.PanelTicketCreate)
			agentSecured.GET("/tickets", handler.PanelTicketList)
			agentSecured.GET("/tickets/unread-count", handler.PanelTicketUnreadCount)
			agentSecured.GET("/tickets/:id", handler.PanelTicketDetail)
			agentSecured.POST("/tickets/:id/replies", handler.PanelTicketReply)
			agentSecured.PUT("/tickets/:id/close", handler.PanelTicketClose)
		}

		// 用户端（无需管理员鉴权）
		userAuth := api.Group("/user-panel")
		{
			userAuth.POST("/login", handler.UserLogin)
			userAuth.GET("/agent-query", handler.PublicAgentQuery)
			userAuth.GET("/target-query", handler.PublicTargetQuery)
			userAuth.POST("/register/email-code", handler.UserSendRegisterEmailCode)
			userAuth.POST("/register", handler.UserRegister)
			userAuth.POST("/forgot-password", handler.UserForgotPassword)
			userAuth.POST("/reset-password", handler.UserResetPassword)
		}
		api.GET("/home-template/active", handler.PublicActiveHomeTemplate)
		api.GET("/home-template/assets/:id/:revision/*filepath", handler.PublicHomeTemplateAsset)
		// 内置软件源（内嵌远程仓库）：目录清单与模板示例图片
		api.GET("/software-source/plugins", handler.PublicSoftwareSourcePlugins)
		api.GET("/software-source/previews/:file", handler.PublicSoftwareSourcePreview)
		api.GET("/software-source/templates/:id/preview", handler.PublicSoftwareSourceTemplatePreview)
		api.POST("/internal/software-source/cache/invalidate", handler.InternalSoftwareSourceCacheInvalidate)

		// 广告投放：默认读本站 /api/v1/public/advertisements；需要时可用环境变量代理到其他源。
		api.GET("/advertisements", handler.PublicAdvertisements)
		api.GET("/v1/public/advertisements", handler.PublicLocalAdvertisements)

		// 本实例作为软件源源站：元数据目录、入驻审核、上架/下架、广告 CRUD。
		handler.RegisterSourceStationRoutes(r, api)
		handler.RegisterPaidStoreRoutes(r, api)
		handler.RegisterNotificationRoutes(api)
		handler.StartStoreSnapshotRefresher()

		// 用户端（需鉴权）
		userSecured := api.Group("/user-panel")
		userSecured.Use(middleware.JWTAuth(), middleware.RequireActiveUser(), middleware.RequireFreshPassword("users"))
		{
			userSecured.GET("/dashboard", handler.UserDashboard)
			userSecured.GET("/licenses", handler.UserLicenseList)
			userSecured.GET("/license-query", handler.UserOwnLicenseQuery)
			userSecured.POST("/cards/redeem", handler.UserLicenseCardRedeem)
			userSecured.PUT("/licenses/:id/target", handler.UserLicenseUpdateTarget)
			userSecured.POST("/licenses/:id/refresh-key", handler.UserLicenseRefreshKey)
			userSecured.GET("/licenses/:id/sites", handler.UserLicenseSiteList)
			userSecured.DELETE("/licenses/:id/sites/:siteId", handler.UserLicenseSiteUnbind)
			userSecured.GET("/apps", handler.UserAppList)
			userSecured.GET("/apps/purchase", handler.UserAppListForPurchase)
			userSecured.GET("/balance", handler.UserGetBalance)
			userSecured.GET("/recharge/options", handler.UserRechargeOptions)
			userSecured.POST("/recharge/orders", handler.UserRechargeCreate)
			userSecured.GET("/recharge/orders/:orderNo", handler.UserRechargeStatus)
			userSecured.GET("/recharge-v2/options", handler.UserRechargeV2Options)
			userSecured.POST("/recharge-v2/orders", handler.UserRechargeV2Create)
			userSecured.GET("/purchase/pay-options", handler.UserPurchasePayOptions)
			userSecured.POST("/purchase", handler.UserPurchase)
			userSecured.GET("/purchase/orders/:orderNo", handler.UserPurchaseOrderStatus)
			userSecured.GET("/agent-upgrade/levels", handler.UserAgentUpgradeLevels)
			userSecured.POST("/agent-upgrade/orders", handler.UserAgentUpgradeCreate)
			userSecured.GET("/agent-upgrade/orders/:orderNo", handler.UserAgentUpgradeOrderStatus)
			userSecured.DELETE("/agent-upgrade/orders/:orderNo", handler.UserAgentUpgradeCancel)
			userSecured.GET("/profile", handler.UserProfile)
			userSecured.PUT("/profile", handler.UserUpdateProfile)
			userSecured.POST("/change-password", handler.UserChangePassword)
			userSecured.POST("/realname/init", handler.UserRealnameInit)
			userSecured.GET("/realname/query", handler.UserRealnameQuery)
			userSecured.GET("/licenses/:id/versions", handler.PanelLicenseVersions)
			userSecured.POST("/licenses/:id/versions/:versionId/download-url", handler.PanelLicenseVersionDownloadURL)
			userSecured.POST("/tickets", handler.PanelTicketCreate)
			userSecured.GET("/tickets", handler.PanelTicketList)
			userSecured.GET("/tickets/unread-count", handler.PanelTicketUnreadCount)
			userSecured.GET("/tickets/:id", handler.PanelTicketDetail)
			userSecured.POST("/tickets/:id/replies", handler.PanelTicketReply)
			userSecured.PUT("/tickets/:id/close", handler.PanelTicketClose)
		}

		// 需要鉴权的路由（仅管理员角色）
		secured := api.Group("/")
		secured.Use(middleware.JWTAuth(), middleware.RequireAdmin(), middleware.RequireFreshPassword("admins"))

		// 超级管理员专属接口：后端强制对齐前端 R_SUPER 权限，防止 R_ADMIN 越权直接调用。
		superSecured := secured.Group("")
		superSecured.Use(middleware.RequireSuperAdmin())
		{
			// 敏感写接口按角色已分配的侧栏菜单鉴权（P1-05）。读接口仍只要求管理员身份。
			// R_SUPER 在 RequireMenu 内放行，不查 role_menus。
			// 自身资料与改密不挂菜单，避免只分配了工作台的管理员无法维护自己的账号。
			menu := func(names ...string) *gin.RouterGroup {
				return secured.Group("", middleware.RequireMenu(names...))
			}
			userWrites := menu(middleware.MenuUser)

			secured.GET("/user/info", handler.GetUserInfo)
			secured.PUT("/user/info", handler.UpdateUserInfo)
			secured.POST("/user/change-password", handler.ChangePassword)
			secured.GET("/user/list", handler.AdminUserList)
			userWrites.POST("/user/create", handler.AdminUserCreate)
			userWrites.PUT("/user/:id", handler.AdminUserUpdate)
			userWrites.PUT("/user/:id/toggle", handler.AdminUserToggle)
			userWrites.DELETE("/user/:id", handler.AdminUserDelete)
			superSecured.POST("/user/:id/impersonate", handler.AdminImpersonateUser)
			secured.GET("/system/menus", handler.GetMenuList)
			superSecured.GET("/system/config", handler.AdminSystemConfig)
			superSecured.PUT("/system/config", handler.AdminSystemConfigUpdate)
			superSecured.PUT("/system/config/switch/:key", handler.AdminSystemFeatureSwitchUpdate)
			superSecured.GET("/system/payment-config", handler.AdminPaymentConfig)
			superSecured.PUT("/system/payment-config", handler.AdminPaymentConfigUpdate)
			superSecured.POST("/system/payment-config/test", handler.AdminPaymentTestCreate)
			superSecured.GET("/system/payment-config/test/:orderNo", handler.AdminPaymentTestStatus)
			superSecured.GET("/system/payment-v2-config", handler.AdminPaymentV2Config)
			superSecured.PUT("/system/payment-v2-config", handler.AdminPaymentV2ConfigUpdate)
			superSecured.POST("/system/payment-v2-config/test", handler.AdminPaymentV2TestCreate)
			superSecured.GET("/system/payment-v2-config/test/:orderNo", handler.AdminPaymentV2TestStatus)
			superSecured.GET("/system/alipay-f2f-config", handler.AdminAlipayF2FConfig)
			superSecured.PUT("/system/alipay-f2f-config", handler.AdminAlipayF2FConfigUpdate)
			secured.GET("/system/payment-orders", handler.AdminPaymentOrderList)
			superSecured.GET("/system/plugins", handler.AdminPluginList)
			superSecured.POST("/system/plugins/:id/toggle", handler.AdminPluginToggle)
			superSecured.POST("/system/plugins/:id/download", handler.AdminPluginDownload)
			superSecured.POST("/system/plugin-sources", handler.AdminPluginSourceAdd)
			superSecured.DELETE("/system/plugin-sources/:id", handler.AdminPluginSourceDelete)
			superSecured.POST("/system/plugin-sources/:id/refresh", handler.AdminPluginSourceRefresh)
			superSecured.GET("/system/home-templates", handler.AdminHomeTemplateList)
			superSecured.POST("/system/home-templates/upload", handler.AdminHomeTemplateUpload)
			superSecured.POST("/system/home-templates/:id/enable", handler.AdminHomeTemplateEnable)
			superSecured.GET("/system/home-templates/:id/download", handler.AdminHomeTemplateDownload)
			superSecured.POST("/system/home-templates/:id/install", handler.AdminHomeTemplateInstall)
			superSecured.POST("/system/home-templates/:id/disable", handler.AdminHomeTemplateDisable)
			superSecured.POST("/system/home-templates/:id/uninstall", handler.AdminHomeTemplateUninstall)
			superSecured.GET("/system/realname-config", handler.AdminRealnameConfig)
			superSecured.PUT("/system/realname-config", handler.AdminRealnameConfigUpdate)
			superSecured.POST("/system/realname-products", handler.AdminRealnameProducts)
			superSecured.GET("/system/realname-records", handler.AdminRealnameRecordList)
			superSecured.GET("/system/mail-config", handler.AdminMailConfig)
			superSecured.PUT("/system/mail-config", handler.AdminMailConfigUpdate)
			superSecured.PUT("/system/mail-config/content-type", handler.AdminMailContentTypeUpdate)
			superSecured.POST("/system/mail-config/test", handler.AdminMailConfigTest)
			superSecured.GET("/system/mail-logs", handler.AdminMailLogList)
			superSecured.GET("/system/mail-logs/:id", handler.AdminMailLogDetail)
			superSecured.GET("/system/update/status", handler.AdminOnlineUpdateStatus)
			superSecured.GET("/system/update/history", handler.AdminOnlineUpdateHistory)
			superSecured.POST("/system/update/check", handler.AdminOnlineUpdateCheck)
			superSecured.POST("/system/update/apply", handler.AdminOnlineUpdateApply)
			superSecured.GET("/system/update/jobs/:id", handler.AdminOnlineUpdateJob)
			secured.GET("/license/dashboard", handler.LicenseDashboard)
			secured.GET("/dashboard/overview", handler.AdminDashboardOverview)
			secured.GET("/dashboard/cards", handler.AdminDashboardCards)
			secured.GET("/dashboard/trend", handler.AdminDashboardTrend)
			secured.GET("/dashboard/license-status", handler.AdminDashboardLicenseStatus)
			secured.GET("/dashboard/payment-methods", handler.AdminDashboardPaymentMethods)
			secured.GET("/dashboard/agent-metrics", handler.AdminDashboardAgentMetrics)
			secured.GET("/dashboard/user-metrics", handler.AdminDashboardUserMetrics)
			secured.GET("/dashboard/app-metrics", handler.AdminDashboardAppMetrics)
			secured.GET("/dashboard/app-ranking", handler.AdminDashboardAppRanking)
			secured.GET("/dashboard/agent-ranking", handler.AdminDashboardAgentRanking)
			secured.GET("/dashboard/activities", handler.AdminDashboardActivities)
			secured.GET("/dashboard/quick-entries", handler.AdminDashboardQuickEntries)
			licenseList := menu(middleware.MenuLicenseList)
			licenseCards := menu(middleware.MenuLicenseCards)
			licenseApps := menu(middleware.MenuLicenseApps)
			licenseVersions := menu(middleware.MenuLicenseVersions, middleware.MenuAppVersions)
			licensePlans := menu(middleware.MenuLicensePlans)
			licenseLogs := menu(middleware.MenuLicenseLogs)
			promotionWrites := menu(middleware.MenuPromotionCampaigns)
			agentList := menu(middleware.MenuAgentList)

			secured.GET("/license/list", handler.LicenseList)
			secured.GET("/license/:id/sites", handler.AdminLicenseSiteList)
			licenseList.DELETE("/license/:id/sites/:siteId", handler.AdminLicenseSiteUnbind)
			secured.GET("/license/query-by-user", handler.UserLicenseQuery)
			secured.GET("/license/cards/batches", handler.AdminLicenseCardBatchList)
			licenseCards.POST("/license/cards/batches", handler.AdminLicenseCardBatchCreate)
			licenseCards.PUT("/license/cards/batches/:id/status", handler.AdminLicenseCardBatchToggle)
			licenseCards.DELETE("/license/cards/batches/:id", handler.AdminLicenseCardBatchDelete)
			secured.GET("/license/cards/batches/:id/cards", handler.AdminLicenseCardList)
			secured.GET("/license/cards/batches/:id/export", handler.AdminLicenseCardExport)
			licenseCards.PUT("/license/cards/:id/status", handler.AdminLicenseCardToggle)
			secured.GET("/license/apps", handler.AppList)
			secured.GET("/license/owners", handler.LicenseOwnerOptions)
			secured.GET("/app/list", handler.AppManageList)
			// 打包下载不改业务数据，仍只要求管理员身份。
			secured.POST("/sdk/pack", handler.AdminSDKPackDownload)
			licenseApps.POST("/app/create", handler.AppCreate)
			licenseApps.PUT("/app/:id", handler.AppUpdate)
			licenseApps.PUT("/app/:id/license-required", handler.AppLicenseRequiredUpdate)
			licenseApps.PUT("/app/:id/reset-secret", handler.AppResetSecret)
			licenseApps.DELETE("/app/:id", handler.AppDelete)
			secured.GET("/app/:id/versions", handler.AppVersionList)
			licenseVersions.POST("/app/:id/versions", handler.AppVersionCreate)
			licenseVersions.PUT("/app/:id/versions/:versionId", handler.AppVersionUpdate)
			licenseVersions.DELETE("/app/:id/versions/:versionId", handler.AppVersionDelete)
			licenseVersions.POST("/app/:id/versions/:versionId/download-url", handler.AppVersionAdminDownloadURL)
			secured.GET("/plan/list", handler.PlanList)
			licensePlans.POST("/plan/create", handler.PlanCreate)
			licensePlans.PUT("/plan/:id", handler.PlanUpdate)
			licensePlans.PUT("/plan/:id/toggle", handler.PlanToggle)
			licensePlans.DELETE("/plan/:id", handler.PlanDelete)
			secured.GET("/promotion/campaigns", handler.AdminPromotionCampaignList)
			promotionWrites.POST("/promotion/campaigns", handler.AdminPromotionCampaignCreate)
			promotionWrites.PUT("/promotion/campaigns/:id", handler.AdminPromotionCampaignUpdate)
			promotionWrites.PUT("/promotion/campaigns/:id/toggle", handler.AdminPromotionCampaignToggle)
			promotionWrites.DELETE("/promotion/campaigns/:id", handler.AdminPromotionCampaignDelete)
			secured.GET("/verify-log/list", handler.VerifyLogList)
			licenseLogs.DELETE("/verify-log/clear", handler.VerifyLogClear)
			secured.GET("/agent/list", handler.AgentList)
			agentList.POST("/agent/create", handler.AgentCreate)
			agentList.PUT("/agent/:id", handler.AgentUpdate)
			agentList.PUT("/agent/:id/toggle", handler.AgentToggle)
			// 充值按钮在代理列表页，而不是财务流水页。
			agentList.POST("/agent/:id/recharge", handler.AgentRecharge)
			agentList.DELETE("/agent/:id", handler.AgentDelete)
			superSecured.POST("/agent/:id/impersonate", handler.AdminImpersonateAgent)
			secured.GET("/agent/select-list", handler.AgentSelectList)
			agentLevels := menu(middleware.MenuAgentLevel)
			secured.GET("/agent-level/list", handler.AgentLevelList)
			secured.GET("/agent-level/select-list", handler.AgentLevelSelectList)
			agentLevels.POST("/agent-level/create", handler.AgentLevelCreate)
			agentLevels.PUT("/agent-level/:id", handler.AgentLevelUpdate)
			agentLevels.DELETE("/agent-level/:id", handler.AgentLevelDelete)
			secured.GET("/admin/agent-upgrade/stats", handler.AdminAgentUpgradeStats)
			secured.GET("/admin/agent-upgrade/orders", handler.AdminAgentUpgradeOrderList)
			secured.GET("/admin/agent-upgrade/conversions", handler.AdminAccountConversionList)
			secured.GET("/admin/agent-upgrade/conversions/:id", handler.AdminAccountConversionDetail)
			secured.GET("/transaction/list", handler.TransactionList)
			secured.GET("/transaction/stats", handler.TransactionStats)
			quotas := menu(middleware.MenuAgentQuota)
			secured.GET("/quota/list", handler.QuotaList)
			quotas.POST("/quota/create", handler.QuotaCreate)
			quotas.PUT("/quota/:id", handler.QuotaUpdate)
			quotas.DELETE("/quota/:id", handler.QuotaDelete)
			licenseList.POST("/license/create", handler.LicenseCreate)
			licenseList.PUT("/license/:id", handler.LicenseUpdate)
			licenseList.PUT("/license/:id/toggle", handler.LicenseToggle)
			licenseList.DELETE("/license/:id", handler.LicenseDelete)

			// 反盗版 - 追踪
			piracyTracking := menu(middleware.MenuPiracyTracking)
			secured.GET("/piracy/tracking/stats", handler.PiracyTrackingStats)
			secured.GET("/piracy/tracking/list", handler.PiracyTrackingList)
			secured.GET("/piracy/tracking/:id", handler.PiracyTrackingDetail)
			piracyTracking.POST("/piracy/tracking/create", handler.PiracyTrackingCreate)
			piracyTracking.PUT("/piracy/tracking/:id/block", handler.PiracyTrackingBlock)
			piracyTracking.PUT("/piracy/tracking/:id/unblock", handler.PiracyTrackingUnblock)
			piracyTracking.POST("/piracy/tracking/batch-block", handler.PiracyTrackingBatchBlock)
			// 反盗版 - 告警
			piracyAlerts := menu(middleware.MenuPiracyAlerts)
			secured.GET("/piracy/alert/stats", handler.PiracyAlertStats)
			secured.GET("/piracy/alert/list", handler.PiracyAlertList)
			piracyAlerts.PUT("/piracy/alert/:id/mark", handler.PiracyAlertMark)
			piracyAlerts.POST("/piracy/alert/batch-mark", handler.PiracyAlertBatchMark)
			// 反盗版 - 黑名单
			piracyBlacklist := menu(middleware.MenuPiracyBlacklist)
			secured.GET("/piracy/blacklist/list", handler.PiracyBlacklistList)
			piracyBlacklist.POST("/piracy/blacklist/create", handler.PiracyBlacklistCreate)
			piracyBlacklist.PUT("/piracy/blacklist/:id", handler.PiracyBlacklistUpdate)
			piracyBlacklist.DELETE("/piracy/blacklist/:id", handler.PiracyBlacklistDelete)
			piracyBlacklist.POST("/piracy/blacklist/batch-delete", handler.PiracyBlacklistBatchDelete)
			// 反盗版 - 数据报表
			secured.GET("/piracy/report/overview", handler.ReportOverview)
			// 角色管理
			superSecured.GET("/role/list", handler.RoleList)
			superSecured.POST("/role/create", handler.RoleCreate)
			superSecured.PUT("/role/:id", handler.RoleUpdate)
			superSecured.DELETE("/role/:id", handler.RoleDelete)
			superSecured.GET("/role/:id/menus", handler.RoleMenus)
			handler.RegisterBuyerStoreRoutes(superSecured)
			handler.RegisterPanelStoreRoutes(userSecured, agentSecured)
			superSecured.PUT("/role/:id/menus", handler.RoleUpdateMenus)
			// 菜单管理
			superSecured.GET("/menu/list", handler.MenuManageList)
			superSecured.POST("/menu/create", handler.MenuManageCreate)
			superSecured.PUT("/menu/:id", handler.MenuManageUpdate)
			superSecured.DELETE("/menu/:id", handler.MenuManageDelete)

			tickets := menu(middleware.MenuTicketManage)
			secured.GET("/ticket/list", handler.AdminTicketList)
			secured.GET("/ticket/unread-count", handler.AdminTicketUnreadCount)
			secured.GET("/ticket/:id", handler.AdminTicketDetail)
			tickets.POST("/ticket/:id/reply", handler.AdminTicketReply)
			tickets.PUT("/ticket/:id/status", handler.AdminTicketStatus)
		}
	}

	// 快瞳 / 腾讯云增强人脸扫码拍照落地页（无需登录，token 即凭证）
	r.GET("/realname-face", handler.RealnameFacePage)

	// 静态文件：生产只服务盘上前端（AUTO_PRO_FRONTEND_DIR / frontend/current / 宝塔网站根）。
	// 缺盘上产物时启动失败，不会静默回退到 go:embed static。开发/引导需显式
	// AUTO_PRO_ALLOW_EMBEDDED_FRONTEND=1。
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal("failed to load static files:", err)
	}
	softwareSourceAdminURL := config.GetSoftwareSourceAdminURL()
	r.GET(appstore.PagePrefix, func(c *gin.Context) {
		if softwareSourceAdminURL == "" {
			c.Redirect(http.StatusFound, config.LocalSoftwareSourceAdminPath)
			return
		}
		c.Redirect(http.StatusFound, softwareSourceAdminURL)
	})
	r.GET(appstore.PagePrefix+"/*filepath", func(c *gin.Context) {
		if softwareSourceAdminURL == "" {
			c.Redirect(http.StatusFound, config.LocalSoftwareSourceAdminPath)
			return
		}
		target := softwareSourceAdminURL
		requestedPath := strings.Trim(strings.TrimSpace(c.Param("filepath")), "/")
		if requestedPath == "dashboard" || requestedPath == "templates" || requestedPath == "apps" || requestedPath == "sources" {
			target += requestedPath
		}
		c.Redirect(http.StatusFound, target)
	})
	r.GET("/source", handler.SourceStationPage)
	r.GET("/source/", handler.SourceStationPage)
	if err := handler.RegisterFrontend(r, staticSub); err != nil {
		log.Fatal(err)
	}

	// 启动后台邮件到期提醒任务
	handler.StartMailReminderWorker()
	handler.StartPurchaseOrderExpiryWorker()

	// 兜底迁移：补齐购买订单字段，修正历史线上购买流水与价格快照
	func() {
		db, err := config.DB()
		if err != nil {
			return
		}
		if err := handler.EnsureAppVersionsTable(db); err != nil {
			log.Printf("ensure app_versions table failed: %v", err)
		}
		if err := handler.EnsureAppPurchaseLicenseTypesColumn(db); err != nil {
			log.Printf("ensure app purchase license types failed: %v", err)
		}
		if err := handler.EnsurePurchaseOrderUserIDColumn(db); err != nil {
			log.Printf("ensure user_id column failed: %v", err)
		}
		if err := handler.EnsureLicensePurchasePriceSnapshotSchema(db); err != nil {
			log.Printf("ensure license purchase price snapshots failed: %v", err)
		}
		if err := handler.EnsurePromotionCampaignSchema(db); err != nil {
			log.Printf("ensure promotion campaign schema failed: %v", err)
		}
		if err := handler.EnsureAccountUpgradeSchema(db); err != nil {
			log.Printf("ensure account upgrade schema failed: %v", err)
		}
		if err := ensureLicenseSiteLimitSchema(db); err != nil {
			log.Printf("ensure license site limit schema failed: %v", err)
		}
		if err := handler.EnsureSourceStationSchema(); err != nil {
			log.Fatalf("ensure source station schema failed: %v", err)
		}
		handler.EnsureNotificationSchema()
		handler.BackfillLicensePurchaseTransactions(db)
	}()

	// 启动。SIGTERM/SIGINT 先在时限内关闭监听，避免在线更新或进程守护停进程时端口一直不释放。
	host := config.GetHost()
	port := config.GetPort()
	log.Printf("Server starting on %s:%s", host, port)
	if err := serveUntilSignal(r, host+":"+port); err != nil {
		if isAddrInUse(err) {
			log.Printf("%s", describeListenConflict(host+":"+port))
		}
		log.Fatal("Server failed:", err)
	}
}

func serveUntilSignal(handler http.Handler, addr string) error {
	server := &http.Server{Addr: addr, Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- err
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed, closing listeners: %v", err)
			_ = server.Close()
		}
		return nil
	}
}
