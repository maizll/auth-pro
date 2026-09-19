# auth_pro

## 核心功能

- 公益开源交流群：169484041。

`auth_pro` 是一个授权管理与反盗版后台系统，包含后台管理端、代理端、用户端和授权校验 API。项目采用前后端分离开发，生产环境可将前端产物内嵌到 Go 后端统一部署。

## 核心功能

- 授权管理：应用、应用版本、套餐、授权码、授权状态、授权校验日志管理。
- 用户体系：管理员登录、用户注册登录、用户资料、余额和授权购买。
- 代理体系：代理登录、代理余额、授权购买、代理等级和额度管理。
- 反盗版：盗版追踪、告警、黑名单和数据报表。
- 系统管理：菜单、角色、用户、系统配置、邮件配置和邮件日志。
- 应用商店管理：独立 Dashboard、首页模板目录、启用/停用和可扩展模块框架。
- 安装向导：首次运行时配置数据库并创建管理员账号。

## 技术栈

### 后端

- Go 1.22
- Gin
- MySQL
- JWT

### 前端

- Vue 3
- TypeScript
- Vite
- Pinia
- Vue Router
- Element Plus
- Tailwind CSS

## 目录结构

```text
auth_pro/
├── backend/              # Go 后端服务
│   ├── appstore/          # 应用商店兼容 BFF 模块
│   ├── config/           # 配置读取与数据库配置持久化
│   ├── handler/          # API 处理器与业务入口
│   ├── middleware/       # CORS、JWT 等中间件
│   ├── model/            # 数据模型
│   ├── service/          # 服务层
│   ├── static/           # 前端构建产物，供后端内嵌部署
│   └── main.go           # 后端入口
├── software-source-system/ # 独立软件源 Go + Vue 系统
├── database/             # 数据库结构文件
├── frontend/             # Vue 前端项目
│   ├── src/api/          # API 请求封装
│   ├── src/router/       # 路由与权限处理
│   ├── src/store/        # Pinia 状态管理
│   └── src/views/        # 页面模块
└── scripts/              # 运维脚本
```

## 环境要求

- Go >= 1.22
- Node.js >= 20.19.0
- pnpm >= 11.15.1
- MySQL 5.7+ 或 MySQL 8.x

## 本地开发

### 1. 启动后端

```bash
cd backend
go mod download
go run .
```

默认端口为 `19127`，可通过环境变量修改：

```bash
PORT=19127 go run .
```

数据库配置由安装向导写入 `backend/db.json`，安装完成后会生成 `backend/install.lock`。

### 2. 启动前端

```bash
cd frontend
pnpm install
pnpm dev
```

开发环境下，前端通过 Vite 代理将 `/api` 请求转发到 `http://localhost:19127`。

本分叉默认**不连接**官方软件源 `plug.91ani.cn`。旧路径 `/admin/app-store` 会跳转到本站 `/plugin-store`。若要对接自建软件源服务，再设置下面的环境变量。

### 3. 首次安装

启动前后端后，在浏览器访问前端地址，进入安装流程：

1. 填写 MySQL 连接信息。
2. 初始化数据库表结构。
3. 创建管理员账号。
4. 进入后台管理系统。

## 首页模板与软件源

管理后台的“应用商店”提供“首页模板”分区。管理员可以在“软件源管理”中添加以下两类 HTTP(S) 地址：

- JSON 清单 URL，例如 `https://example.com/software-source/index.json` 或 `https://example.com/auth-pro/index.json`。
- Git 仓库 URL，例如 `https://git.example.com/team/auth-pro-templates.git`。服务端需要在 `PATH` 中安装 `git`，仓库根目录必须包含 `index.json`。

软件源允许使用内网地址。请仅添加可信仓库：服务端会拉取清单和模板文件，但声明式模板不会执行仓库中的 JavaScript。清单缓存 5 分钟；可在软件源管理中手动刷新，源暂时不可用时会保留已有缓存并显示错误状态。

现有 `plugins` 字段保持兼容，首页模板通过 `homeTemplates` 声明：

```json
{
  "name": "示例软件源",
  "plugins": [],
  "homeTemplates": [
    {
      "id": "clean-home",
      "name": "清新首页",
      "description": "简洁的授权服务首页",
      "version": "1.0.0",
      "schemaVersion": 1,
      "sha256": "模板 JSON 文件的 64 位 SHA256",
      "templateUrl": "templates/clean-home.json"
    }
  ]
}
```

JSON 清单可使用绝对或相对 `templateUrl`。Git 仓库应将 `templateUrl` 替换为仓库内相对路径，例如 `"templatePath": "templates/clean-home.json"`。路径越界和指向仓库外部的符号链接会被拒绝。

模板文件采用声明式 schema v1：

```json
{
  "schemaVersion": 1,
  "theme": {
    "primaryColor": "#16a085",
    "backgroundColor": "#f2fbf8",
    "textColor": "#17352d"
  },
  "hero": {
    "badge": "LICENSE SERVICE",
    "title": "专业授权服务",
    "highlight": "安全、稳定、易管理",
    "description": "为用户提供授权查询与账户服务",
    "imageUrl": "https://example.com/assets/hero.png",
    "primaryAction": { "label": "登录用户中心", "type": "login" }
  },
  "features": [
    {
      "icon": "ri:shield-check-line",
      "title": "安全验证",
      "description": "授权状态实时同步"
    }
  ],
  "footer": { "text": "© 示例授权服务" }
}
```

模板启用前会校验文件大小、SHA256 和 schema。系统只保存一个活动模板 ID，因此同一时间最多启用一个首页模板。模板拉取、校验、文件读取或渲染失败时，`/user/login` 自动使用内置默认模板，浏览器 URL 不会改变。

圆趣蓝白红与黑金金融科技 demo 的目录、配置、远程发布和完整验证说明见 [`docs/home-template-ui-demo.md`](docs/home-template-ui-demo.md)。

前端端到端测试命令：

```bash
cd frontend
pnpm exec playwright install chromium
pnpm test:e2e
```

## 生产构建

### 1. 构建主前端

```bash
cd frontend
pnpm install
pnpm build
```

### 2. 同步主前端产物到后端

```bash
rm -rf ../backend/static/*
cp -R dist/* ../backend/static/
```

### 3. 构建独立软件源系统

```bash
cd ../software-source-system
./scripts/build.sh 1.0.0
```

产物为独立 tar.gz，不进入授权发布包。

### 4. 构建并运行后端

```bash
cd ../backend
go build -o auth_pro .
PORT=19127 ./auth_pro
```

也可以使用项目提供的脚本重启后端：

```bash
./scripts/restart-backend.sh
```

## Gitee Release 发布与在线更新

项目默认通过 Gitee API 查询公开仓库 `Zcy-sa/auth-pro` 的最新 Release，并从附件列表读取 `latest.json`：

```text
https://gitee.com/api/v5/repos/Zcy-sa/auth-pro/releases/latest
```

当前发布和一键整包更新仅支持 `Linux amd64`。创建具有仓库写入权限的 Gitee 私人令牌后，发布严格语义版本 tag：

```powershell
git tag v1.2.3
git push origin v1.2.3
$env:GITEE_ACCESS_TOKEN = '<Gitee 私人令牌>'
pwsh -NoProfile -File .\scripts\publish-gitee-release.ps1 -Version 1.2.3
Remove-Item Env:GITEE_ACCESS_TOKEN
```

发布脚本要求工作区干净、版本 tag 指向当前提交且已经推送到 `origin`。脚本会构建前端和 Linux amd64 后端、运行后端测试、创建 Gitee Release，并上传以下三个附件：

```text
auth_pro-full-v1.2.3.tar.gz
latest.json
releases.json
```

`releases.json` 会保留历史版本，并自动把上一版本标签到当前标签之间的 Git 提交标题写入本次版本的 `notes`，作为在线更新页面展示的更新内容。需要人工整理发布说明时，可在构建环境中通过 `AUTO_PRO_RELEASE_NOTES` 提供 JSON 字符串数组或按行分隔文本覆盖自动内容。

服务器可通过 `AUTO_PRO_UPDATE_URL` 改用自建 HTTPS 镜像清单。Gitee 默认源会限制 API、清单、更新包和下载重定向只能使用指定仓库及 Gitee 官方附件存储。

当前更新包只校验文件大小和 SHA256；该机制可发现下载损坏，但如果仓库或 Release 发布权限被攻破，攻击者仍可同时替换更新包和 SHA256，不能替代离线数字签名。

## 重要配置

| 配置项                | 说明                                             | 默认值                                                                        |
| --------------------- | ------------------------------------------------ | ----------------------------------------------------------------------------- |
| `PORT`                | 后端服务端口                                     | `19127`                                                                       |
| `AUTO_PRO_DATA_DIR`   | 后端运行数据目录，用于保存配置、更新包和运行数据 | 当前运行目录                                                                  |
| `AUTO_PRO_UPDATE_URL` | 在线更新清单地址；默认值为 Gitee 最新 Release API | `https://gitee.com/api/v5/repos/Zcy-sa/auth-pro/releases/latest`              |
| `AUTO_PRO_SOFTWARE_SOURCE_URL` | 远程软件源地址；默认空，不连接官方源 | （空，本站自托管） |
| `AUTO_PRO_SOFTWARE_SOURCE_API_KEY` | 远程软件源目录 Key；默认空，仓库不内置密钥 | （空） |
| `AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL` | 旧 `/admin/app-store/*` 跳转目标；未设置时落到本站 `/plugin-store` | `/plugin-store` |
| `AUTO_PRO_SOFTWARE_SOURCE_TIMEOUT` | 目录 HTTP 请求超时 | `5s` |
| `AUTO_PRO_SOFTWARE_SOURCE_STALE_TTL` | 最后成功目录快照最大降级时间 | `24h` |
| `AUTO_PRO_ADVERTISEMENT_URL` | 广告投放接口；相对路径走本进程，http(s) 才代理外网 | `/api/v1/public/advertisements` |
| `VITE_API_PROXY_URL`  | 前端开发代理目标地址                             | `http://localhost:19127`                                                      |

## API 入口

- `/api/install/*`：安装流程接口。
- `/api/auth/login`：后台管理员登录。
- `/api/license/verify`：公开授权校验接口。
- `/api/app/version/check`：应用客户端使用有效授权和 HMAC-SHA256 签名检查版本。
- `/api/app/version/download?token=...`：使用版本检查或管理员接口签发的短期令牌下载本地更新包。
- `/api/agent-panel/*`：代理端接口。
- `/api/user-panel/*`：用户端接口。
- `/api/app-store/*`：独立应用商店管理接口，要求管理员 JWT。
- `/api/home-template/active`：当前首页模板公开读取接口。
- `/api/advertisements`：前端广告位代理；默认读本站投放。
- `/api/v1/public/advertisements`：本站广告接口（`home-banner` / `sidebar` / `popup`）。
- `/software-source/index.json`：本实例作为软件源源站时的公开清单（兼容 `/auth-pro/index.json`，见下方「作为软件源源站」）。
- `/source`：兼容跳转，进入管理后台「源站」菜单（`/source-station/plugins`）。源站管理不再提供独立页面。
- `/api/*`：后台管理接口，除公开接口外默认需要 JWT 鉴权。

## 部署说明

生产环境推荐同源部署：前端构建后放入 `backend/static`，由 Go 后端统一提供静态资源和 `/api` 接口。这样可以减少跨域配置，并保持授权校验、管理后台和前端页面的一致部署入口。

## 自托管软件源与广告

本分叉默认**不连接** `plug.91ani.cn`，仓库中也不再内置官方目录 Key。

- 不设环境变量即可完成本地启动：软件源客户端不会访问外网，广告位走本站 `/api/v1/public/advertisements`（当前返回空列表，前端显示占位）。
- 若要对接自建软件源，再设置：

```bash
export AUTO_PRO_SOFTWARE_SOURCE_URL="http://127.0.0.1:19128"
export AUTO_PRO_SOFTWARE_SOURCE_API_KEY="your-catalog-key"
# 可选：旧 /admin/app-store 跳转到远程管理后台；不设则跳到本站 /plugin-store
export AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL="http://127.0.0.1:19128/admin"
```

- 若要把广告代理到其他投放服务（可选）：

```bash
export AUTO_PRO_ADVERTISEMENT_URL="https://ads.example.com/api/v1/public/advertisements"
```

重启后端后可用 `ss`/`tcpdump` 或代理日志确认没有对 `plug.91ani.cn` 的出站请求。

## 作为软件源源站

本实例可以直接当软件源（源站）用。协议与管理后台「软件源管理」一致：公开一个可 HTTP GET 的 `index.json`。源站主机**不需要**设置 `AUTO_PRO_SOFTWARE_SOURCE_*`（那是客户端用来可选对接独立目录服务的，cut-1 已改成环境变量、默认关闭）。

**源站只保存目录元数据，从不保存插件或模板源码。** 目录字段为 id、name、version、description、author、status、sha256、外部 `downloadUrl` / `templateUrl`、审核信息与时间戳。发布包必须放在外部 HTTPS（或后续 OSS 对象键），不要把 git 源码树写入后端 data 目录。

其它授权实例在「软件源管理」里添加：

```text
https://<host>/software-source/index.json
```

兼容路径：`/auth-pro/index.json`（同样的 JSON）。清单缓存由消费者侧完成（约 5 分钟，可手动刷新）；源站在上架/下架时从数据库重新生成公开目录。

清单形状：

```json
{
  "name": "本站软件源",
  "plugins": [],
  "homeTemplates": [
    {
      "id": "clean-home",
      "name": "清新首页",
      "description": "简洁的授权服务首页",
      "version": "1.0.0",
      "schemaVersion": 1,
      "sha256": "<模板文件 64 位 hex>",
      "templateUrl": "https://cdn.example.com/templates/clean-home.json"
    }
  ]
}
```

`templateUrl` 可以是 `https://` 绝对地址，或相对路径（由消费者相对 `index.json` URL 解析）。插件 `downloadUrl` 必须是外部 `https://` 地址。上架要求 64 位 sha256 与下载/模板地址同时存在。

**下架 ≠ 远程卸载。** 下架只是把条目从公开 `index.json` 隐藏（status=`hidden`）。已经安装到其它授权实例本地的插件/模板不会被源站删除或停用。

锁定流水线（方向不再改）：管理员上传 ZIP → 硬规范校验（不合规拒绝，不写库/不推 Release/不留临时文件）→ 从 `plugin.json` / `template.json` 自动填表 → 用设置页令牌推送 Gitee/GitHub Release → **只把元数据 + downloadUrl/templateUrl + SHA256 入库** → 审核 / 多版本更新 / 上架下架 → 公开 `GET /software-source/index.json`（`plugins` + `homeTemplates`）。应用服务器不保存插件源码。

工作流：

1. 启动本仓库后端（源站无需 `AUTO_PRO_SOFTWARE_SOURCE_*`）。
2. **管理员登录管理后台**（与其它后台功能同一套账号/会话/布局），侧栏「源站」：
   - 入驻审核：开发者申请通过/拒绝/冻结
   - 插件管理：上传 ZIP 硬校验、多版本、上架/下架
   - 首页模板管理：同上
   - 软件源目录：公开 `index.json` 预览与快照重生、审计日志
   - 广告投放：本站 `home-banner` / `sidebar` / `popup`
   - Release 设置：GitHub/Gitee 仓库与 Token
   旧地址 `/source` 会 302 到 `/source-station/plugins`。不要再使用独立控制面页面。
3. 开发者保存插件/模板**元数据草稿**（外部 URL + sha256），提交审核；管理员在后台通过或驳回后上架。
4. **管理员上传包（失败即拒绝）**：后台「插件管理 / 首页模板」上传，或 `POST /api/v1/source/admin/packages/parse|publish`。只接受 ZIP。包内必须有 `plugin.json`（插件）或 `template.json`（首页模板，schemaVersion=1 且含 `hero.title`），必填 id/name/version/description/author；路径穿越、符号链接等不安全布局直接 400。失败时返回 `error.field` + `error.rule`，不写库、不推 Release、删除临时文件。校验通过后才自动填表、可选推送 Release，并进入草稿/审核（可选上架）。清单规范：`GET /software-source/package-schema.json`（兼容 `GET /api/v1/source/admin/packages/schema`）。
5. **发布地址**：在管理后台「源站 → Release 设置」填写 provider（`github`|`gitee`）、owner/repo、令牌（仅服务端保存，GET 只返回掩码）、默认 tag 策略（如 `{id}-{version}`）和 Gitee 分支。可用「测试连接」验证仓库可达与令牌权限（`POST /api/v1/source/admin/settings/release/test`，只 GET 仓库元数据，不创建 Release、不落库）。保存元数据时优先创建/更新 Release 并上传 zip 附件，把 `downloadUrl`/`templateUrl` 设为附件的 https 地址；未配置时可粘贴已有 https 地址。目录只持久化元数据 + URL + sha256。
6. **更新**：为同一插件创建新版本行（version / changelog / 外部 URL / sha256）→ 提交审核 → 管理员通过后该版本 `published` 并成为 `latest`。旧版本元数据保留，可弃用，不可删源码（源站本来就不存源码）。
7. 管理员可将 `latest` 回滚到先前已发布版本；下架只从公开目录隐藏整个插件。公开 `index.json` 只展示当前 `latest`（含 version、downloadUrl、sha256、可选 changelog，以及预留的 `minVersion` / `forceUpdate`）。
8. 查询历史版本：管理后台「插件管理 → 版本」，或 `GET /api/v1/source/admin/plugins/:id/versions`（兼容 `GET /api/admin/source/plugins/:id/versions`）。
9. 消费者实例在「软件源管理」添加 `https://<host>/software-source/index.json`。

```bash
# 源站本机（无需软件源环境变量）
curl -s http://127.0.0.1:19127/software-source/index.json
curl -s http://127.0.0.1:19127/auth-pro/index.json

# 上传闸门：不合规包 400，不入库
curl -s http://127.0.0.1:19127/software-source/package-schema.json
curl -s -H "Authorization: Bearer <admin-jwt>" \
  -F "file=@bad.zip" -F "kind=plugin" \
  http://127.0.0.1:19127/api/v1/source/admin/packages/parse
# {"code":400,"msg":"...缺少 plugin.json...","error":{"field":"plugin.json","rule":"require_manifest"}}

# 管理员：解析合规包（不入库）
curl -s -H "Authorization: Bearer <admin-jwt>" \
  -F "file=@demo-plugin.zip" -F "kind=plugin" \
  http://127.0.0.1:19127/api/v1/source/admin/packages/parse

# 管理员：校验通过后再保存草稿（已配置 Release 则推送；否则加 downloadUrl）
curl -s -H "Authorization: Bearer <admin-jwt>" \
  -F "file=@demo-plugin.zip" -F "kind=plugin" -F "submit=1" \
  http://127.0.0.1:19127/api/v1/source/admin/packages/publish
```

本 PR 不包含：OSS 直传发布包、Git 软件源充当源站、代码签名、按客户可见性。源站管理已接入现有 Vue 管理后台，不再提供独立 `/source/` 页面。后续可把发布包对象键记入目录，仍不入库 git 源码树。

## 从自建软件源安装首页模板

远程软件源默认关闭。指向自建源并配置目录 Key 后，管理后台「应用商店」可刷新目录并启用模板。密钥只由服务端发送，不进入前端环境变量、浏览器代码或模板文件。

本系统已包含黑金首页的布局、玻璃卡片、移动端导航、查询入口和登录弹窗，仍复用主应用的用户登录、代理账号转换和代登录流程，默认及蓝色模板不受影响。

指向自建软件源并配置目录 Key 后：

1. 在本系统「应用商店」的首页模板分区点击刷新。显式刷新会重新读取目录，不必等待 5 分钟缓存。
2. 点击启用：后端下载 JSON、校验 SHA-256、验证 schema，并原子保存安装文件后切换首页。访问路径仍为 `/user/login`。
3. 已安装模板内容更新后会显示「待更新 / 更新并启用」；预览图 URL 带更新时间，避免继续使用旧封面缓存。恢复默认模板沿用原有操作。

上传入口统一放在 auth-pro-plug 的「模板管理」，本系统隐藏「上传首页模板 ZIP」按钮，保留既有上传接口与已安装模板的兼容。软件源可分发 JSON 或 ZIP；两端需一起升级 ZIP 目录协议后再发布 ZIP。分发端下架不等于远程卸载已安装文件。

### ZIP 插件的自动安装

软件源 `plugins[].downloadUrl` 应指向有效 ZIP（URL 扩展名不限）。点击「下载并安装」后，后端自动解压至服务端数据目录的 `plugins/<插件ID>/`，写入安装状态并在本地插件列表显示。包可直接包含文件，也可额外套一层目录；可选 `plugin.json` 中的 `id` 必须与软件源一致。

只有成功解压、校验并登记的目录才算已安装。旧版只保存了 `plugin.pkg` 的目录可以重新点击下载，失败不会破坏旧文件。插件安装不自动切换支付/实名认证服务商，也不会执行 `install.sh`、可执行文件或热加载 Go 代码；未包含在当前服务端代码中的插件显示为已安装资源，启用其业务能力仍需部署相应运行实现。

### 在 auth-pro-plug 发布首页模板 ZIP

支持两种包结构（允许外层包含一个 `dist/` 等目录）：

```text
# 声明式模板：复用系统布局、登录与验证码
custom-home.zip
├── template.json          # schemaVersion: 1，并包含 hero.title
└── assets/
    └── cover.png          # hero.imageUrl 可写 assets/cover.png

# 静态首页：支持独立 HTML 或已构建的 Vue 等静态产物
custom-site.zip
├── index.html
└── assets/
    ├── index.js
    └── index.css
```

- 推荐入口：auth-pro-plug「模板管理 → 上传模板」，使用 multipart 字段 `metadata`、`template` 和可选 `preview` 保存并上架。本系统原 `POST /api/system/home-templates/upload` 仍保留超级管理员认证用于兼容，但不再展示本地上传入口。
- 在发布端上传不会自动启用。在本系统应用商店刷新目录并点击「启用」后，才下载、校验 ZIP 的 SHA-256、安全解压并激活，访问路径仍为 `/user/login`；可以随时恢复默认模板。原有本地上传模板继续可用。
- 静态 ZIP 必须是构建产物，不是 Vue 源码；推荐资源使用相对路径（Vite 设置 `base: './'`，路由使用 hash 模式）；安装时会自动适配入口 HTML 中指向包内文件的 `/assets/...` 等根路径引用，不改写主站导航、外部链接或脚本中的业务逻辑。若同时包含 `index.html` 与 `template.json`，以静态首页为准。
- 静态页面运行在禁止同源权限的 iframe/CSP 沙箱中，不可访问主站 Token、Cookie 或调用业务接口。系统不额外叠加悬浮按钮；模板内的登录按钮应调用 `window.parent.postMessage({ type: 'auth-pro:login' }, '*')` 打开系统登录框。恢复默认模板由管理员在后台应用商店操作。不要在模板中实现密码收集或保存 Token。
- ZIP 上限 20 MiB，解压总大小上限 100 MiB，最多 2048 个条目，入口文件上限 2 MiB。路径穿越、符号链接、特殊文件、重复路径和损坏包会被拒绝；只在独立模板目录部署，不覆盖主站代码。
- 部署安全：模板文件只应通过 `/api/home-template/assets/...` 访问。Go 已阻止通过普通静态路由直接读取插件/模板目录；若 Nginx/宝塔直接提供网站根目录文件，也必须禁止访问运行数据目录（默认配置可添加 `location ^~ /backend/ { return 404; }`），不要为模板目录另配静态 alias 或脚本执行，以免绕过沙箱响应头。

### 首页模板的下载、安装、停用和卸载

在「应用商店 → 首页模板」与旧「首页模板管理」页面共用同一组操作：

- **下载**：保存软件源的原始 JSON / ZIP 文件到浏览器，不安装、不启用。已安装 ZIP 保留经校验的原包副本，软件源离线/下架时仍可下载。
- **安装**：下载、校验并登记服务端安装，不切换当前首页。已启用模板的更新需选「更新并启用」，或者先停用后更新。
- **启用**：必要时下载安装，再切换 `/user/login`；刷新目录本身不会替换已运行的版本。
- **停用**：恢复默认首页，保留本地文件；只停用指定模板，不影响另一个当前模板。
- **卸载**：先恢复默认（仅当卸载当前模板），清空安装状态并删除受控目录的文件；分发端记录不删除，仍可重新安装。内置默认模板禁止卸载。
- 下架、离线不等于卸载：已安装的远程模板仍列出，允许本地停用、启用和卸载。卸载先隔离目录，数据库写入失败会回滚，拒绝越界路径或链接目录。

接口均要求管理员 JWT 与超级管理员权限：

| 方法 | 路径 | 含义 |
| --- | --- | --- |
| GET | `/api/system/home-templates/:id/download` | 下载文件（attachment） |
| POST | `/api/system/home-templates/:id/install` | 仅安装 |
| POST | `/api/system/home-templates/:id/enable` | 安装并启用 / 更新并启用 |
| POST | `/api/system/home-templates/:id/disable` | 停用 |
| POST | `/api/system/home-templates/:id/uninstall` | 卸载本地安装 |

ZIP 分发必须同时部署 auth-pro-plug 的 `017_template_zip` 迁移和本项目客户端兼容代码。发布端仍为只读密钥目录，不把软件源密钥下发浏览器。

### 验证

- 后端：在 `backend` 目录运行 `go test ./...`。
- 黑金渲染回归：在 `frontend` 目录运行 `pnpm test:gold-template`，覆盖实际模板数据、卡片语义、离线图标、颜色校验、样式范围和减少动态效果。
- 前端：运行 `pnpm exec vue-tsc --noEmit` 与 `pnpm build`，再按原有发布流程部署。

上线前应在测试环境完成实际数据库与两个服务的连通性验证；仅通过单元测试或生成构建产物，不代表已经在生产后台发布模板。

上传安装的旧表兼容回归可设置 `AUTO_PRO_TEST_UPLOAD_DB=1` 后运行 `go test ./handler -run TestUploadedTemplateRegistrationLegacyMySQL -v`；可用 `AUTO_PRO_TEST_TEMPLATE_ZIP` 指定真实 ZIP。该测试只在当前数据库连接内创建临时表，不修改现有表结构、模板记录或启用状态。

跨项目真实链路回归从 auth-pro-plug 运行 `go test -tags integration ./internal/template -run TestMySQLCrossProjectTemplateLifecycle -v`。
需要 `AUTH_PRO_INTEGRATION_DSN`（发布端一次性 `*_test` 库）、`AUTH_PRO_TEMPLATE_CONSUMER_DIR`（本项目 backend 绝对路径）、`AUTO_PRO_DB_HOST/PORT/NAME/USER/PASSWORD`（消费端另一个一次性 `*_test` 库）。测试使用真实 multipart 上传、两个 HTTP 服务、真实 MySQL 和安装目录，验证两种 ZIP、下载不激活、安装不激活、启停/卸载/重装/更新、离线操作、权限和失败回滚；不连接业务库。
