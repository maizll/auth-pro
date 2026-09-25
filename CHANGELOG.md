# 更新日志

## [v1.5.4] 2026-09-25 — 付费商店阶段 1、宝塔一键脚本与文档重写

- 发布包附带宝塔一键安装与升级脚本 `baota-install.sh`、`baota-upgrade.sh`。升级前备份运行数据与数据库，替换后核对运行数据未变。
- 中文文档按当前代码重写。
- 插件和模板可以标价。付费条目必须是本站托管的 ZIP。公开目录不暴露付费下载信息。第三方付费上架暂不开放。
- 付费商店阶段 1：后台内绑定源站账号并扫码购买商业版，签名快照自动生效，不需要激活码。免费版仅能新建 1 个授权应用，老站点已有应用不受影响，只拦截新建。商业版限制处统一图标并加中文提示。新增 `auth_pro store-keygen`。
- 本发行包的商店验签公钥仍为占位符，不签发、不承认任何商业版快照，买方一律按免费版。源站生成密钥并打入公钥后会另行发版。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.5.4`。发布说明见 `docs/release-notes-1.5.4.txt`。

## [v1.5.3] 2026-09-25 — 管理端写接口鉴权、在线更新校验与命名迁移

- 管理端敏感写接口按角色菜单鉴权（`RequireMenu`）。没有对应菜单权限的管理员，不能再只凭 admin JWT 调用授权、卡密、应用等写接口。超级管理员仍放行。
- 在线更新要求 `package.signature` 为 `sha256:` 加上与包 SHA256 相同的 64 位十六进制，并核对 GitHub Release 附件 digest。前端目录不是符号链接时，先写入暂存目录再原子改名切换，避免更新中途半新半旧。
- 源站破坏性 ALTER/DROP 改为 `schema_migrations` 命名迁移。删列前先回填。迁移失败不标记已应用，启动 Fatal。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.5.3`。发布说明见 `docs/release-notes-1.5.3.txt`。

## [v1.5.2] 2026-09-24 — 授权校验、会话与插件安装加固

- 公开授权校验按客户端 IP 与 `app_key` 限流：每分钟最多 1200 次。超出返回 429「请求过于频繁，请稍后再试」，这次不写入 `verify_logs`。签名尚未通过时，应用不存在与签名错误对外都是同一句「授权校验失败」。
- 首页授权查询改为登录后只看自己的授权。未登录返回 401；账号、邮箱参数不再用来查出他人授权。默认首页改为「查看我的授权」。
- 盗版告警中心去掉未接通的「通知设置」。邮件和 Webhook 仍未接入，校验失败也不会自动写入告警列表。
- 远程安装插件必须带 SHA256，缺校验码或与压缩包不一致时不安装。公网源必须 HTTPS。只有软件源地址写成字面量私网 IP 时，才允许 HTTP 和私网。源站外链在下载前拒绝私网、链路本地和云元数据地址。
- 管理员代登录仅超级管理员可用，令牌有效期 2 小时，并写入操作日志，不再改目标的最后登录信息。`refreshToken` 不能当作访问令牌。管理员被禁用或角色与令牌不一致时，旧令牌立即失效。
- 授权被禁用或重新启用且状态确有变化时，持有人收到站内通知「授权状态已变更」。数据库配置 `db.json` 按 0600 保存，启动时收紧已有宽松权限。在线更新不再默认信任历史 Gitee 仓库 `Zcy-sa/auth-pro`。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.5.2`。发布说明见 `docs/release-notes-1.5.2.txt`。

## [v1.5.1] 2026-09-24 — 去掉原作者引流，并修复装锁接管与余额并发

- 摆脱原作者控制与引流：用户可见入口不再指向站外官网、文档、哔哩哔哩和官方群。保留 LICENSE 与合理署名。
- 修复安装锁丢失后，未登录仍可清空数据库或新建超级管理员（P0）。已有数据时拒绝重装接管。
- 修复用户与代理余额购买并发时少扣款或多发授权（P0）。同一事务内锁定并原子扣减，扣款未成功不发授权。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.5.1`。发布说明见 `docs/release-notes-1.5.1.txt`。

## [v1.5.0] 2026-09-23 — 开发者 ZIP 上传（干净重写）

- 在 v1.4.10 上重写 v1.5.0。此前含管理端系统监控与开发文档可视化预览的 v1.5.0 已撤回，不进入本版。
- 开发者登记可以上传 ZIP：源站保存文件并回填地址与 SHA256；外部 HTTPS 仍然保留。提交审核时，外链检查可达、ZIP 与校验码；本站托管包只核对本地文件。
- 本版不包含管理端「定时任务 / 系统监控」页面、接口和进程内调度，也不包含开发者文档页的插件/模板可视化预览。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.5.0`（补丁位到 9 后升次版本，不使用 `1.4.11`）。发布说明见 `docs/release-notes-1.5.0.txt`。

## [v1.4.10] 2026-09-22 — 登录弹窗跟随首页模板

- 登录弹窗跟随已启用首页模板：`stylePreset` 为 `cartoon-blue` / `fintech-gold` 时套用对应外形，颜色取 `theme.primaryColor` / `backgroundColor` / `textColor`。
- 开发文档：首页模板与插件改为作者体例长指南，并同步到开发者面板，侧栏按 `##` 章节展开。
- 文档补上源站开发者章程（整站模板与插件登记；此前未进入已发布版本）。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.10`。发布说明见 `docs/release-notes-1.4.10.txt`。

## [v1.4.9] 2026-09-22 — 开发者端登记弹框窄屏适配

- 开发者端「登记插件」「登记模板」抽屉窄屏铺满宽度，内容区内滚，底栏按钮可点。
- 「新增版本」弹框窄屏适宽。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.9`。发布说明见 `docs/release-notes-1.4.9.txt`。

## [v1.4.8] 2026-09-22 — 开发者端手机导航可收起

- 开发者端窄屏导航改为抽屉：可汉堡打开，遮罩/换页/关闭/Esc 收起（与用户端、代理端一致）。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.8`。发布说明见 `docs/release-notes-1.4.8.txt`。

## [v1.4.7] 2026-09-22 — 用户端与代理端顶栏路径单行

- 用户端、代理端顶栏路径强制单行，末级过长省略。
- 窄屏隐藏「站点名 /」前缀，只显示当前页名，避免换行。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.7`。发布说明见 `docs/release-notes-1.4.7.txt`。

## [v1.4.6] 2026-09-22 — 用户端与代理端手机导航可收起

- 用户端、代理端手机导航可收起：窄屏侧栏改为抽屉，遮罩/换页/关闭/Esc 均可关闭。
- 购买授权、开通授权、工单等页在约 375px 下减少横向撑出。
- 宽屏侧栏折叠后真正收到约 64px。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.6`。发布说明见 `docs/release-notes-1.4.6.txt`。

## [v1.4.5] 2026-09-22 — 购买授权重排与铃铛未读

- 用户端「购买授权」整页重排：BizAppSelect 选应用，四段卡片流程，支付接口不变。
- 「我的工单」未读徽标改为行内显示，不再被菜单顶边裁成半个。
- 管理端铃铛未读红点完整显示在图标右上角，不再被顶栏切半。
- 管理端通知下拉紧贴铃铛并限制在后台主区域内。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.5`。发布说明见 `docs/release-notes-1.4.5.txt`。

## [v1.4.4] 2026-09-22 — 站内通知小版本

- 汇总审核类站内通知（入驻待审/通过/拒绝、取消开发者身份、插件/模板提交与审核/弃用）走统一写入。
- 取消开发者后前台收到「开发者身份已取消」。
- 铃铛面板 Teleport 定位、不透明实底，避免溢出站点边框与透底。
- 待办页只渲染当前 tab、正常行高；派生待办（入驻等）可读。
- 不依赖 WebSocket / 邮件 / 短信配置即可使用站内铃铛。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.4`。发布说明见 `docs/release-notes-1.4.4.txt`。

## [v1.4.3] 2026-09-21 — 单语言 SDK、当面付插件、业务组件

- SDK 接入包改为按单一语言下载（PHP / Node.js / Python / Go / 浏览器）。解压后为单根目录：入口文件 + 预填 `config.json` + 中文 README + 可选 example。浏览器包不含 appSecret。仓库仍保留五语言源码。
- 新增支付渠道插件契约（`backend/payment`）。官方插件「支付宝当面付」（`alipay-f2f`）支持正扫 `alipay.trade.precreate`，异步 RSA2 验签入账。已接入用户/代理授权购买、余额充值、用户升级代理。
- 当面付配置并入系统设置「支付配置」，去掉重复侧栏。网关与异步通知地址只读、自动生成。`epay` 与 `epay-v2` 硬互斥；官方直连与易支付软并存，配置页提示去重规则，收银台同一支付方式去重且优先官方当面付。
- 新增业务组件 `BizAppSelect`、`BizStatusTag`、`BizCopySecret`，替换应用下拉、状态标签与密钥显隐复制。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.3`。发布说明见 `docs/release-notes-1.4.3.txt`。

## [v1.4.2] 2026-09-21 — 菜单管理可改上级、商店/更新一级、开发者广告可本地上传

- 菜单管理可改上级，编辑时回填中文标题；禁止选自身或子孙，保存后刷新侧栏。
- 应用商店与在线更新默认一级菜单，不再挂在接入开发下。
- 开发者广告申请支持本地上传图片，https 外链仍可用。
- 根目录 `VERSION` / `AppVersion` / `VITE_VERSION` 默认 `1.4.2`。发布说明见 `docs/release-notes-1.4.2.txt`。

## [v1.4.1] 2026-09-21 — 发布面冻结、单一前端根、侧栏以后端菜单为准

- 在线更新默认源改为 GitHub `maizll/auth-pro` Releases（`latest.json` / 标签附件）。仓库默认、构建脚本与 `.github/workflows/release.yml` 不再指向 `Zcy-sa/auth-pro`（Gitee）或 `cy70923167/auth_pro`。不删除历史 Release，不 force-push 标签。
- 根目录 `VERSION` 作为产品线版本信源；`AppVersion` / `VITE_VERSION` 默认 `1.4.1`，未注入 `-ldflags` 时不再静默显示 `1.0.0`。发布构建仍从 tag 注入。
- 生产 HTTP 与在线更新共用同一套前端根解析。缺盘上 `index.html` 时启动失败或返回 503（带版本号），不再静默服务 `go:embed static`。开发/引导需显式 `AUTO_PRO_ALLOW_EMBEDDED_FRONTEND=1`。启动日志打印 disk/embed 根与内容指纹。
- 默认 `VITE_ACCESS_MODE=backend`（`.env` / `.env.production` / 开发环境）。侧栏树来自 `GET /api/system/menus`，「系统 → 菜单管理」改标题或排序后刷新即可生效。
- 菜单种子与启动 upsert 对齐当前工作流：授权 → 代理 → 源站 → 风控 → 客户服务 → 接入开发 → 系统。补齐源站、套餐、工单、应用商店、在线更新等产品页；按 `menus.name` 幂等写入，不重复插行。
- Result / Exception 等模板演示路由默认隐藏，仍可在菜单管理中看到。
- 改产品导航请走后台菜单，不要再改 `frontend/src/router/modules` 的顺序/标题。该目录只负责注册页面组件。本地对照模板演示可在 `.env.development` 临时设 `frontend`。
- 发布说明见 `docs/release-notes-1.4.1.txt`。

## [源站] 2026-09-20 — 取消开发者后清除冻结残留

- 取消开发者仍是硬删除资格与相关入驻申请（不冻结、不留单）。
- 启动时一次性清掉 `source_developer_applications` 中 `frozen/cancelled/approved/rejected` 行（例如 zxcv25 的 frozen 残留），以及 `source_developers` 中 `enabled=0` 的资格行；不碰 `source_applications`（该表不存在），也不动代理商账号。
- 开发者列表只返回启用中的资格；入驻审核列表仍仅待审核。
- 申请状态不再把禁用资格映射成「已冻结」鬼影行。

## [v1.4.0] 2026-09-20 — 四端统一站内通知中心

- 管理后台、用户端、代理端、开发者端顶栏共用 `ArtNotification` 铃铛，数据来自 `GET/POST /api/v1/notifications*`，不再使用演示 mock。
- 按当前 JWT 身份（admin/user/agent/developer）隔离；代理登录可看到本账号的开发者审核结果。跨角色读/已读返回 404。
- 标签：通知=系统/运营/安全；消息=对人结果；待办=当前身份可处理的待审项（派生，不落库）。
- 已接入：入驻/目录/广告审核、工单、授权开通与到期提醒、订单/充值余额、改密、实名结果、代理等级变更。
- 后续（代码里已留 `TODO(v1.4.x)` 桩，等独立事件落地）：提现/结算、下级分佣流水、授权禁用/过期落库事件。
- 打 `v1.4.0` 标签后由 `.github/workflows/release.yml` 出包；发布说明见 `docs/release-notes-1.4.0.txt`。

## [源站] 2026-09-20 — 源站目录自定义分类可删除

- 管理端「软件目录 → 管理分类」为非内置分类增加删除。保存 extras 全量列表（不含该 key）即可，内置 payment / realname / other / home-template 不可删。
- 若仍有目录项使用该分类，确认框会提示：只去掉分类标签，条目保留原 category 字段。

## [源站] 2026-09-20 — 弃用后从公开软件源目录清除

- 弃用插件/模板后重建公开 `index.json`，条目不再出现在 `plugins` / `homeTemplates`。弃用唯一或当前 latest 已发布版本时同样清出目录；若仍有其他已发布版本则回指该版本。
- 管理端「软件目录」默认列表不再把已弃用条目当作在架货架项；可用状态筛「已弃用」做审计。
- 弃用与下架走同一条公开目录发布/重建路径。

## [源站] 2026-09-20 — 上架后自动发布软件源目录

- 上架 / 下架 / 弃用 / 审核状态变更、已上架元数据或 latest 版本变更、自定义分类保存后，自动重建并发布该应用公开 `index.json`（插件进 `plugins`，模板进 `homeTemplates`，extras 仍写入 `categories`）。
- 指向本实例回环地址的软件源不再吃 5 分钟旧缓存，直接读当前已上架目录；手工「从数据库重生快照」保留。
- 回归：上架后公开目录含 id，下架后不含。

## [源站] 2026-09-20 — 目录编辑、自定义分类商店筛选、模板 kind

- 管理员可在软件目录对草稿/待审/已通过/已上架/已下架条目**编辑**元数据（名称、描述、分类、地址、sha256、作者、changelog、图标）；id/appId 创建后锁定。已上架编辑保持 published，写入 `metadata_edit` 审计。
- 公开 `index.json` 的 `categories` 含内置 + extras。应用商店页签改为读取软件源分类，不再写死支付/实名/其他。自定义插件分类（产品复现：标识 `template`、名称「模板」、plugin.json）出现为独立筛选，条目留在 `plugins`。
- **Breaking：** `template.json` 必须 `"kind": "template"`，缺省 category 自动绑定 `home-template`。plugin.json 写 kind=template、或模板使用支付/实名/其他，均拒绝。
- 开发者面板「开发文档」按企业级章节重写，与 `docs/developer/`、starter、Skill、硬校验同步。

## [源站] 2026-09-20 — 开发者登记表单精简为中文傻瓜式

- 开发者「登记插件 / 登记模板」默认只显示：应用、分类、名称、版本、下载/模板地址、校验码、简介。
- 标识随名称自动生成（可改）；作者取当前登录名并只读；图标与更新说明收进「高级选项」。
- 按钮为「保存草稿 / 提交审核」；校验码可先存草稿，提交审核时再必填。支持在浏览器能读到文件时「自动计算」。

## [源站] 2026-09-20 — 开发者入驻复用代理商账号

- 代理商在「开发者入驻」一键申请，不再填写独立用户名/密码/邮箱/说明。
- 管理员在源站「入驻审核」通过后，该代理商用同一套 `agent_panel_token` 进入开发者端。
- 申请与开发者记录绑定 `agent_id`；通过后不另存密码。每个代理商仅一条待审核或已启用绑定。
- 拒绝或取消后可再次申请；取消只停用绑定，不删除历史目录归属。
- 未登录的旧 `POST /api/v1/source/developer/apply` 返回 410。存量无 `agent_id` 的独立开发者账号仍可读；若 `email` 能唯一匹配代理商则回填 `agent_id`。

## [源站] 2026-09-20 — 未配置远程软件源时刷新模板不再告警

- 首页模板与插件共用同一软件源（本站源站公开清单 / 按应用隔离的 index），不再依赖第二套远程模板源。
- 管理端 `?refresh=1`：未配置远程软件源时静默跳过刷新，返回列表且不带 warning。
- 远程源已配置但刷新失败时，保留软警告（不阻断列表）：本站上传与源站模板仍可使用。

## [SDK] 2026-09-20 — 按应用生成客户端接入包

### 新增

- 管理后台「SDK 接入」可按应用勾选模块并下载 ZIP：授权验证、盗版入口、在线更新、广告接入、插件源引用。
- PHP 一文件接入（`AuthPro::boot()`），可选附带 Node/JS。密钥只写入所选应用，不带其它应用 secret。
- 插件源固化应用隔离清单：`{origin}/software-source/{appKey}/index.json`（不要用未带应用的 `/software-source/index.json`）。

## [源站] 2026-09-20 — 软件源按应用隔离

### 行为

- 目录项（插件 / 首页模板）必须绑定 `apps.id`（`app_id NOT NULL`）。管理端「源站」先按应用分区，分类（支付 / 实名 / 其他 / 首页模板）是应用内二级筛选。
- 公开清单按应用隔离：
  - 推荐（SDK 固化）：`GET /software-source/{app_key}/index.json`
  - 兼容：`?app_key=` / `?appKey=`、`/auth-pro/{app_key}/index.json`
  - 未带应用标识的 `/software-source/index.json` 返回空数组，不泄漏其它应用目录。
  - 清单与 `GET /api/v1/source/admin/apps` 均返回 `indexUrl`，便于 SDK ZIP 直接写入。
- 客户端 URL 契约：[docs/software-source-client-url.md](./docs/software-source-client-url.md)
- 开发者提交与管理员审核同样按 `app_id` 隔离；上传 ZIP / 登记外部地址必须选择应用。

### 迁移

现有 `source_catalog_plugins` / `source_catalog_templates` 启动时 `ALTER` 增加 `app_id`（`BIGINT UNSIGNED NOT NULL DEFAULT 0`），并把 `app_id=0` 的旧行一次性回填为 `apps` 表中 **id 最小的应用**。新写入必须显式绑定存在的应用，禁止再产生未绑定行。若库中没有应用，旧行保持 `0`，下次编辑/上架会要求选择应用。

## [源站] 2026-09-19 — 自托管软件源与仓库对接

### 新增

- 本实例可作为自托管软件源（源站）：公开 `GET /software-source/index.json`（`plugins` + `homeTemplates`）。
- 管理后台侧栏「源站」：入驻审核、插件/首页模板、目录快照、广告、Release 仓库设置。
- 上传 ZIP 硬校验（必须含合规 `plugin.json` / `template.json`），失败不写库、不推 Release、不留临时文件。
- 校验通过后自动填表；可推送 GitHub/Gitee Release，源站只保存元数据 + https 下载地址 + SHA256（不存插件源码）。
- 多版本发布、latest、回滚、上架/下架；下架不等于远程卸载。
- 未配置远程软件源时回退本站目录/内置模板，不再整页报「未配置远程软件源」。

### 对接 GitHub / Gitee 仓库（必读）

目标：后台上传合规 ZIP → 自动校验填表 → 推到仓库 Release → 源站只保存附件 https 地址与 SHA256。

#### 1. 准备发包仓库

- 在 Gitee 或 GitHub 新建仓库（如 `yourname/auth-pro-packages`）。
- 建议单独用作「附件柜」，与日常开发仓分离亦可。

#### 2. 创建 Token

**Gitee**：设置 → 安全设置 → 私人令牌；勾选仓库与发行版（projects / release）相关权限；复制令牌（只显示一次）。

**GitHub**：Settings → Developer settings → Personal access tokens；建议 Fine-grained 仅授权目标仓；Contents + Releases 读写（或经典令牌勾选 `repo`）。

#### 3. 后台填写

1. 管理员登录后台。
2. 打开 **源站 → Release / 仓库 Token**。
3. 填写：
   - Provider：`gitee` 或 `github`
   - Owner：用户名或组织名
   - 仓库：仓库名（不要带 `.git`）
   - Token：私人令牌（仅服务端保存，再次打开只显示掩码）
   - Tag 策略：默认 `{id}-{version}`（如 `demo-plugin-1.0.0`）
   - Gitee 时填写分支（多为 `master` 或 `main`）
4. 保存，状态变为「已配置」即可。

#### 4. 上传发包

1. ZIP 根目录或一层子目录必须有 `plugin.json`（模板为 `template.json`），缺字段直接拒收。
2. **源站 → 插件管理** 上传 ZIP。
3. 系统：硬校验 → 自动填表 →（已配仓库则）创建/更新 Release 并上传附件 → 只把 `downloadUrl` + `sha256` 写入目录。
4. 审核 → 上架后公开目录：`http://<源站>/software-source/{app_key}/index.json`  
   未配仓库时仍可解析入库，但需自行粘贴外部 https 下载地址。

#### 5. 消费端使用

在「软件源管理」为对应应用添加：`http://<源站主机:端口>/software-source/{app_key}/index.json`  
本机示例：`http://127.0.0.1:19127/software-source/demo-app/index.json`

#### 6. 常见问题

- Token 权限不足 → 推 Release 失败。
- Owner/仓库名错误 → 上传推送报错。
- ZIP 无清单或字段非法 → 拦截，不写库、不推 Release。
- 私有仓 Release 无法被消费端匿名下载 → 安装失败（附件链接须可访问）。
- 同一 `{id}-{version}` 重复发版通常更新同一 Release 附件，勿随意打乱版本号规则。

### 说明

- 源站默认不连接官方 `plug.91ani.cn`。
- 包规范：`GET /software-source/package-schema.json`。
- 下次发布在线更新包时，可将 `docs/release-notes-source-station.txt` 写入 `latest.json` / `releases.json` 的 `notes`，供后台「在线更新」页展示。