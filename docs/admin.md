# 管理手册

登录后的侧栏来自 `GET /api/system/menus`。改标题、排序、显隐或上级，用「系统设置 → 菜单管理」，不要改 `frontend/src/router/modules` 的顺序来当产品导航。保存后侧栏会刷新。默认构建是 `VITE_ACCESS_MODE=backend`。

下面按 `backend/handler/menu_spec.go` 里的默认菜单。超级管理员（`R_SUPER`）才能看到标注了「仅超管」的项。结果页、异常页是界面模板遗留，默认隐藏，不是产品功能。

工作台（`/dashboard/console`）默认不出现在侧栏（菜单为隐藏），固定标签仍可打开控制台。

## 应用授权 `/license`

| 菜单 | 路径 | 作用 |
| --- | --- | --- |
| 应用管理 | `/license/apps` | 创建应用，获得 `appKey` / `appSecret`。公开校验用这组密钥签名。归档后应用还在，授权和版本保留，只是不能再登记新的目录条目。已归档的行可以点「恢复」 |
| 版本管理 | `/license/versions` | 应用客户端版本、更新说明、更新包或外部下载地址 |
| 套餐管理 | `/license/plans` | 授权套餐 |
| 卡密管理 | `/license/cards` | 生成与管理卡密 |
| 授权列表 | `/license/list` | 域名、泛域名、IP、密钥授权及状态 |
| 验证日志 | `/license/logs` | 公开校验写入的 `verify_logs` |
| 授权概览 | `/license/dashboard` | 授权数量与状态汇总 |

某个应用下的版本页 `/license/apps/:id/versions` 挂在同一组，侧栏不单列。

## 代理业务 `/admin/agent`

| 菜单 | 路径 | 作用 |
| --- | --- | --- |
| 代理列表 | `/admin/agent/list` | 代理账号 |
| 等级管理 | `/admin/agent/level` | 代理等级 |
| 开码配额 | `/admin/agent/quota` | 代理可开通授权的额度 |
| 财务流水 | `/admin/agent/recharge` | 代理充值与流水 |
| 升级审计 | `/admin/agent/upgrade` | 用户升级为代理的记录 |

## 源站运营 `/source-station`

本实例可以当软件源。公开清单是 `GET /software-source/{app_key}/index.json`（也接受 `?app_key=`，以及兼容路径 `/auth-pro/{app_key}/index.json`）。未带应用标识时返回空目录。旧地址 `/source` 会转到 `/source-station/packages`。

| 菜单 | 路径 | 作用 |
| --- | --- | --- |
| 软件目录 | `/source-station/packages` | 按应用查看插件与首页模板。审核、上架、下架、弃用、恢复为草稿、编辑元数据、版本与回滚 latest。恢复后不会自动上架。可单条或批量切换绑定应用，不能切到已归档的应用。下拉里的「未归属（应用已删除）」只找回升级前就对不上应用的旧条目。已归档的应用仍在下拉里，并标成已归档 |
| 入驻审核 | `/source-station/applications` | 开发者入驻申请。通过后对方才能登录开发者面板 |
| 公开目录 | `/source-station/catalog` | 预览公开 `index.json`，可从数据库重生快照，并看审计 |
| 广告投放 | `/source-station/ads` | 广告位 `home-banner`、`sidebar`、`popup` 的审核与投放 |
| 源站设置 | `/source-station/settings` | GitHub 或 Gitee 的 owner/repo 与令牌 |

商业版产品在「应用管理」里打开「作为本站商业版出售」。价格在该应用的「套餐管理」。授权在「授权列表」（来源含商店绑定、商店购买，行内可授予或吊销商业版）。订单在「订单列表」，收款在「支付配置」。旧的商业版设置、商店订单、主授权与权益、商业版收入地址会转到这些页面。

`/source-station/plugins` 与 `/source-station/templates` 是隐藏入口，分别转到软件目录和 `?category=home-template`。

开发者在独立面板登记，不在这个后台里填表。登记字段与审核状态见 [开发者文档](developer/README.md)。

## 安全风控 `/piracy`

| 菜单 | 路径 | 作用 |
| --- | --- | --- |
| 盗版追踪 | `/piracy/tracking` | 签名已通过但未匹配授权时，若站点打开盗版检测，会累计命中 |
| 黑名单 | `/piracy/blacklist` | 按应用拉黑域名或 IP。校验命中后返回 `blacklisted` |
| 告警中心 | `/piracy/alerts` | 查看告警。没有可用的「通知设置」：邮件和 Webhook 未接入，校验失败也不会自动写入告警列表 |
| 数据报表 | `/piracy/reports` | 风控统计 |

## 客户服务 `/customer-service`

| 菜单 | 路径 | 作用 |
| --- | --- | --- |
| 用户管理 | `/user-manage` | 前台用户，不是后台管理员 |
| 订单列表 | `/order-list` | 支付订单 |
| 活动管理 | `/promotion-campaigns` | 促销活动 |
| 工单管理 | `/tickets` | 用户工单 |

## 接入开发 `/sdk`

| 菜单 | 路径 | 作用 |
| --- | --- | --- |
| SDK 示例 | `/sdk/index` | 选择应用和语言，下载接入包。协议见 [API 与 SDK](api-sdk.md) |
| 开发文档 | `/sdk/developer-doc` | 授权校验与版本检查说明（页面内文案） |
| 模板文档 | `/sdk/default-home-template` | 本站安装首页模板的入口格式，见 [首页模板](home-template.md) |

## 应用商店 `/plugin-store`（仅超管）

从软件源安装或启用插件与首页模板。默认不连接外部官方源。要对接别的目录时，在服务器上设置 `AUTO_PRO_SOFTWARE_SOURCE_URL` 与 `AUTO_PRO_SOFTWARE_SOURCE_API_KEY`。未设置时，旧路径 `/admin/app-store` 转到本站 `/plugin-store`。

远程 ZIP 必须带 64 位 SHA256。公网地址必须是 HTTPS。只有软件源 URL 写成字面量回环或私网 IP 时，才允许 HTTP 和私网。主机名一律按公网处理。

## 在线更新 `/online-update`（仅超管）

检查 GitHub Release 的 `latest.json` 并执行整包更新。签名、备份与回滚见 [部署手册](deployment.md)。

## 系统设置 `/system`

| 菜单 | 路径 | 谁能看 | 作用 |
| --- | --- | --- | --- |
| 角色管理 | `/system/role` | 仅超管 | 角色与菜单权限。写接口在服务端按角色菜单校验，不只有前端隐藏 |
| 菜单管理 | `/system/menu` | 仅超管 | 侧栏标题、排序、上级、显隐 |
| 系统配置 | `/system/config` | 仅超管 | 站点名称等系统项 |
| 支付配置 | `/system/epay-config` | 仅超管 | 已启用的支付插件各一块：易支付、易支付 V2、支付宝当面付。`epay` 与 `epay-v2` 不能同时启用。当面付与易支付可以同时配置，收银台同一种支付方式只留一条，并优先官方当面付 |
| 邮件配置 | `/system/mail-config` | 仅超管 | SMTP |
| 邮件日志 | `/system/mail-logs` | 仅超管 | 发信记录 |
| 个人中心 | `/system/user-center` | 管理员 | 当前管理员资料。默认不作为标签页展示 |

支付页没有单独的「当面付」侧栏。插件未启用时，对应分栏不出现。网关与异步通知地址由系统生成，页面上为只读。

## 顶栏通知

管理端、用户端、代理端、开发者端顶栏铃铛使用 `GET/POST /api/v1/notifications*`，按当前 JWT 身份隔离。站内通知不依赖 WebSocket、邮件或短信。
