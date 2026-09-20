# 更新日志

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