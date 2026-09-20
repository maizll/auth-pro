-- 菜单种子：与运营工作流分组一致（授权 → 代理 → 源站 → 风控 → 客服 → 接入 → 系统）
-- 侧栏以「系统 → 菜单管理」为准；本文件只负责全新安装的初始树。
-- 一级菜单
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `redirect`, `title`, `icon`, `sort`, `keep_alive`) VALUES
(1,  0, 'Dashboard',        '/dashboard',         '/index/index', '/dashboard/console',          'menus.dashboard.title',        'ri:home-smile-2-line',       1, 0),
(3,  0, 'License',          '/license',           '/index/index', '/license/apps',               'menus.license.title',          'ri:apps-line',               2, 0),
(4,  0, 'Agent',            '/admin/agent',       '/index/index', '',                            'menus.agent.title',            'ri:team-line',               3, 0),
(9,  0, 'SourceStation',    '/source-station',    '/index/index', '/source-station/packages',    'menus.sourceStation.title',    'ri:database-2-line',         4, 0),
(5,  0, 'Piracy',           '/piracy',            '/index/index', '',                            'menus.security.title',         'ri:shield-flash-line',       5, 0),
(10, 0, 'CustomerService',  '/customer-service',  '/index/index', '/user-manage',                'menus.customerService.title',  'ri:customer-service-2-line', 6, 0),
(8,   0, 'Sdk',              '/sdk',               '/index/index', '',                            'menus.integration.title',      'ri:code-box-line',           7, 0),
(210, 0, 'PluginStore',      '/plugin-store',      '/plugin-store/index', '',                     'menus.integration.store',      'ri:store-2-line',            8, 1),
(211, 0, 'OnlineUpdate',     '/online-update',     '/online-update/index', '',                    'menus.integration.update',     'ri:download-cloud-2-line',   9, 1),
(2,   0, 'System',           '/system',            '/index/index', '',                            'menus.system.title',           'ri:settings-3-line',         10, 0);

-- 模板演示页：保留在库中但默认隐藏，不进侧栏
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`) VALUES
(6, 0, 'Result',    '/result',    '/index/index', 'menus.result.title',    'ri:checkbox-circle-line', 90, 0, 1),
(7, 0, 'Exception', '/exception', '/index/index', 'menus.exception.title', 'ri:error-warning-line',   91, 0, 1);

-- Dashboard
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`, `fixed_tab`) VALUES
(101, 1, 'Console', 'console', '/dashboard/console', 'menus.dashboard.console', '', 1, 0, 1, 1);

-- License
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`) VALUES
(303, 3, 'LicenseApps',      'apps',      '/license/apps',      'menus.license.apps',     'ri:apps-2-line',      1, 1),
(308, 3, 'LicenseVersions',  'versions',  '/license/versions',  'menus.license.versions', 'ri:git-branch-line',  2, 1),
(307, 3, 'LicensePlans',     'plans',     '/license/plans',     'menus.license.plans',    'ri:price-tag-3-line', 4, 1),
(306, 3, 'LicenseCards',     'cards',     '/license/cards',     'menus.license.cards',    'ri:coupon-3-line',    5, 1),
(302, 3, 'LicenseList',      'list',      '/license/list',      'menus.license.list',     'ri:file-list-3-line', 6, 1),
(304, 3, 'LicenseLogs',      'logs',      '/license/logs',      'menus.license.logs',     'ri:file-text-line',   7, 1),
(301, 3, 'LicenseDashboard', 'dashboard', '/license/dashboard', 'menus.license.overview', 'ri:dashboard-line',   8, 0);

INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`) VALUES
(305, 3, 'AppVersions', 'apps/:id/versions', '/license/app-versions', 'menus.license.versions', 'ri:git-branch-line', 99, 0, 1);

-- Agent
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`) VALUES
(401, 4, 'AgentList',     'list',     '/agent/list',     'menus.agent.list',    'ri:user-star-line',         1, 1),
(402, 4, 'AgentLevel',    'level',    '/agent/level',    'menus.agent.level',   'ri:vip-crown-line',         2, 1),
(404, 4, 'AgentQuota',    'quota',    '/agent/quota',    'menus.agent.quota',   'ri:key-2-line',             3, 1),
(403, 4, 'AgentRecharge', 'recharge', '/agent/recharge', 'menus.agent.finance', 'ri:money-cny-circle-line',  4, 1),
(405, 4, 'AgentUpgrade',  'upgrade',  '/agent/upgrade',  'menus.agent.upgrade', 'ri:user-shared-line',       5, 1);

-- Source station
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `redirect`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`) VALUES
(901, 9, 'SourceStationPackages',     'packages',     '/source-station/packages',     '',                                             'menus.sourceStation.packages',     'ri:apps-2-line',       1, 1, 0),
(902, 9, 'SourceStationApplications', 'applications', '/source-station/applications', '',                                             'menus.sourceStation.applications', 'ri:user-add-line',      2, 1, 0),
(903, 9, 'SourceStationCatalog',      'catalog',      '/source-station/catalog',      '',                                             'menus.sourceStation.catalog',      'ri:file-list-3-line',   3, 0, 0),
(904, 9, 'SourceStationAds',          'ads',          '/source-station/ads',          '',                                             'menus.sourceStation.ads',          'ri:advertisement-line', 4, 1, 0),
(905, 9, 'SourceStationSettings',     'settings',     '/source-station/settings',     '',                                             'menus.sourceStation.settings',     'ri:settings-3-line',    5, 1, 0),
(906, 9, 'SourceStationPlugins',      'plugins',      '/source-station/packages',      '/source-station/packages',                      'menus.sourceStation.plugins',      'ri:puzzle-line',        6, 1, 1),
(907, 9, 'SourceStationTemplates',    'templates',    '/source-station/packages',      '/source-station/packages?category=home-template', 'menus.sourceStation.templates',  'ri:layout-4-line',      7, 1, 1);

-- Piracy
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`) VALUES
(501, 5, 'PiracyTracking',  'tracking',  '/piracy/tracking',  'menus.security.tracking',  'ri:spy-line',            1, 1),
(502, 5, 'PiracyBlacklist', 'blacklist', '/piracy/blacklist', 'menus.security.blacklist', 'ri:forbid-line',         2, 1),
(503, 5, 'PiracyAlerts',    'alerts',    '/piracy/alerts',    'menus.security.alerts',    'ri:alarm-warning-line',  3, 0),
(504, 5, 'PiracyReports',   'reports',   '/piracy/reports',   'menus.security.reports',   'ri:bar-chart-box-line',  4, 0);

-- Customer service (absolute child paths keep existing bookmarks)
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`) VALUES
(201,  10, 'User',                '/user-manage',          '/system/user',                 'menus.customerService.users',     'ri:user-line',              1, 1),
(209,  10, 'OrderList',           '/order-list',           '/system/payment-orders',       'menus.customerService.orders',    'ri:file-list-3-line',       2, 1),
(212,  10, 'PromotionCampaigns',  '/promotion-campaigns',  '/promotion-campaigns/index',   'menus.customerService.campaigns', 'ri:discount-percent-line',  3, 1),
(1001, 10, 'TicketManage',        '/tickets',              '/system/tickets',              'menus.customerService.tickets',   'ri:question-answer-line',   4, 1);

-- SDK / integration（仅 SDK / 文档 / 模板；应用商店与在线更新为一级菜单）
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`) VALUES
(801, 8, 'SdkIndex',                 'index',                  '/sdk/index',                       'menus.integration.sdk',         'ri:code-s-slash-line',       1, 1),
(802, 8, 'DeveloperDoc',             'developer-doc',          '/sdk/developer-doc',               'menus.integration.docs',        'ri:file-code-line',          2, 1),
(803, 8, 'DefaultHomeTemplateDoc',   'default-home-template',  '/sdk/default-home-template-doc',   'menus.integration.templateDoc', 'ri:layout-4-line',           3, 1);

-- System
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`, `is_hide_tab`) VALUES
(202, 2, 'Role',          'role',        '/system/role',        'menus.system.role',       'ri:shield-user-line',    1, 1, 0, 0),
(204, 2, 'Menus',         'menu',        '/system/menu',        'menus.system.menu',       'ri:menu-2-line',         2, 1, 0, 0),
(205, 2, 'SystemConfig',  'config',      '/system/config',      'menus.system.config',     'ri:settings-3-line',     3, 1, 0, 0),
(208, 2, 'EpayConfig',    'epay-config', '/system/epay-config', 'menus.system.epayConfig', 'ri:bank-card-line',      4, 1, 0, 0),
(206, 2, 'MailConfig',    'mail-config', '/system/mail-config', 'menus.system.mailConfig', 'ri:mail-settings-line',  5, 1, 0, 0),
(207, 2, 'MailLogs',      'mail-logs',   '/system/mail-logs',   'menus.system.mailLogs',   'ri:mail-check-line',     6, 1, 0, 0),
(203, 2, 'UserCenter',    'user-center', '/system/user-center', 'menus.system.userCenter', 'ri:user-settings-line',  7, 1, 0, 1);

-- Demo children (hidden)
INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`) VALUES
(601, 6, 'ResultSuccess', 'success', '/result/success', 'menus.result.success', 'ri:checkbox-circle-line', 1, 1, 1),
(602, 6, 'ResultFail',    'fail',    '/result/fail',    'menus.result.fail',    'ri:close-circle-line',    2, 1, 1);

INSERT INTO `menus` (`id`, `parent_id`, `name`, `path`, `component`, `title`, `icon`, `sort`, `keep_alive`, `is_hide`, `is_hide_tab`, `is_full_page`) VALUES
(701, 7, 'Exception403', '403', '/exception/403', 'menus.exception.forbidden',   '', 1, 1, 1, 1, 1),
(702, 7, 'Exception404', '404', '/exception/404', 'menus.exception.notFound',    '', 2, 1, 1, 1, 1),
(703, 7, 'Exception500', '500', '/exception/500', 'menus.exception.serverError', '', 3, 1, 1, 1, 1);
