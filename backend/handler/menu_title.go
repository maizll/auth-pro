package handler

var menuTitleZH = map[string]string{
	"menus.dashboard.title":             "工作台",
	"menus.dashboard.console":           "工作台",
	"menus.license.title":               "应用授权",
	"menus.license.apps":                "应用管理",
	"menus.license.versions":            "版本管理",
	"menus.license.plans":               "套餐管理",
	"menus.license.cards":               "卡密管理",
	"menus.license.list":                "授权列表",
	"menus.license.logs":                "验证日志",
	"menus.license.overview":            "授权概览",
	"menus.agent.title":                 "代理业务",
	"menus.agent.list":                  "代理列表",
	"menus.agent.level":                 "等级管理",
	"menus.agent.quota":                 "开码配额",
	"menus.agent.finance":               "财务流水",
	"menus.agent.upgrade":               "升级审计",
	"menus.sourceStation.title":         "源站运营",
	"menus.sourceStation.applications":  "入驻审核",
	"menus.sourceStation.packages":      "软件目录",
	"menus.sourceStation.plugins":       "软件目录",
	"menus.sourceStation.templates":     "软件目录",
	"menus.sourceStation.catalog":       "公开目录",
	"menus.sourceStation.ads":           "广告投放",
	"menus.sourceStation.settings":      "源站设置",
	"menus.sourceStation.edition":       "商业版设置",
	"menus.sourceStation.storeOrders":   "商店订单",
	"menus.sourceStation.storeLicenses": "主授权与权益",
	"menus.sourceStation.storeRevenue":  "商业版收入",
	"menus.security.title":              "安全风控",
	"menus.security.tracking":           "盗版追踪",
	"menus.security.blacklist":          "黑名单",
	"menus.security.alerts":             "告警中心",
	"menus.security.reports":            "数据报表",
	"menus.customerService.title":       "客户服务",
	"menus.customerService.users":       "用户管理",
	"menus.customerService.orders":      "订单列表",
	"menus.customerService.campaigns":   "活动管理",
	"menus.customerService.tickets":     "工单管理",
	"menus.integration.title":           "接入开发",
	"menus.integration.sdk":             "SDK示例",
	"menus.integration.docs":            "开发文档",
	"menus.integration.templateDoc":     "模板文档",
	"menus.integration.store":           "应用商店",
	"menus.integration.update":          "在线更新",
	"menus.result.title":                "结果页面",
	"menus.result.success":              "成功页",
	"menus.result.fail":                 "失败页",
	"menus.exception.title":             "异常页面",
	"menus.exception.forbidden":         "403",
	"menus.exception.notFound":          "404",
	"menus.exception.serverError":       "500",
	"menus.system.title":                "系统设置",
	"menus.system.user":                 "用户管理",
	"menus.system.role":                 "角色管理",
	"menus.system.userCenter":           "个人中心",
	"menus.system.developerDoc":         "开发文档",
	"menus.system.menu":                 "菜单管理",
	"menus.system.config":               "系统配置",
	"menus.system.epayConfig":           "支付配置",
	"menus.system.paymentOrders":        "订单列表",
	"menus.system.mailConfig":           "邮件配置",
	"menus.system.mailLogs":             "邮件日志",
	"menus.pluginStore":                 "应用商店",
	"menus.homeTemplate":                "首页模板",
	"menus.promotionCampaigns":          "活动管理",
	"menus.onlineUpdate":                "在线更新",
	"menus.login.title":                 "登录",
	"menus.forgetPassword.title":        "忘记密码",
	"menus.outside.title":               "内嵌页面",
}

func resolveMenuTitle(title string) string {
	if zh, ok := menuTitleZH[title]; ok {
		return zh
	}
	return title
}

func isDemoProductMenu(name string) bool {
	switch name {
	case "Result", "ResultSuccess", "ResultFail",
		"Exception", "Exception403", "Exception404", "Exception500":
		return true
	default:
		return false
	}
}
