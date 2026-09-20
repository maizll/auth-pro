# 更新日志

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