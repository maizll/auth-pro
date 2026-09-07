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

软件源管理后台已独立到 `software-source-system/`：

```bash
cd software-source-system/frontend
pnpm install --frozen-lockfile
pnpm run build
cd ../backend
go run ./cmd/server
```

访问软件源服务 `/admin/` 使用独立管理员登录。授权侧旧 `/admin/app-store` 会跳转到该入口。详见 [`docs/app-store-management.md`](docs/app-store-management.md)。

### 3. 首次安装

启动前后端后，在浏览器访问前端地址，进入安装流程：

1. 填写 MySQL 连接信息。
2. 初始化数据库表结构。
3. 创建管理员账号。
4. 进入后台管理系统。

## 首页模板与软件源

管理后台的“应用商店”提供“首页模板”分区。管理员可以在“软件源管理”中添加以下两类 HTTP(S) 地址：

- JSON 清单 URL，例如 `https://example.com/auth-pro/index.json`。
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
| `AUTO_PRO_SOFTWARE_SOURCE_ADMIN_URL` | 旧 `/admin/app-store/*` 跳转目标 | `<软件源地址>/admin/` |
| `AUTO_PRO_SOFTWARE_SOURCE_TIMEOUT` | 目录 HTTP 请求超时 | `5s` |
| `AUTO_PRO_SOFTWARE_SOURCE_STALE_TTL` | 最后成功目录快照最大降级时间 | `24h` |

软件源连接信息（服务地址 `https://plug.91ani.cn` 和目录只读 Key）已固定编译进后端二进制，不再读取 `AUTO_PRO_SOFTWARE_SOURCE_URL` / `AUTO_PRO_SOFTWARE_SOURCE_API_KEY` 环境变量。
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
- `/api/*`：后台管理接口，除公开接口外默认需要 JWT 鉴权。

## 部署说明

生产环境推荐同源部署：前端构建后放入 `backend/static`，由 Go 后端统一提供静态资源和 `/api` 接口。这样可以减少跨域配置，并保持授权校验、管理后台和前端页面的一致部署入口。

## 从 auth-pro-plug 安装首页模板

软件源服务地址和目录 Key 已固定编译进 auth-pro 后端，**无需在环境变量中配置** `AUTO_PRO_SOFTWARE_SOURCE_URL` / `AUTO_PRO_SOFTWARE_SOURCE_API_KEY`；默认连接 `https://plug.91ani.cn`。

分发后台（auth-pro-plug）侧的 `SOFTWARE_SOURCE_API_KEY` 必须与 auth-pro 后端内置的目录 Key 保持一致，否则目录请求会被拒绝。密钥只由服务端发送，不进入前端环境变量、浏览器代码或模板文件。

首次升级需部署两个项目的新版本。auth-pro 已包含黑金首页的布局、玻璃卡片、移动端导航、查询入口和登录弹窗，仍复用主应用的用户登录、代理账号转换和代登录流程，默认及蓝色模板不受影响。

1. 在 auth-pro-plug 的首页模板管理中上传 `templates/fintech-gold.json`，填写 `fintech-gold` 标识、版本与作者；可附预览图，保存并上架。
2. 在本系统「首页模板管理」或应用商店点击刷新。显式刷新会重新读取目录，不必等待 5 分钟缓存。
3. 点击启用：后端下载 JSON、校验 SHA-256、验证 schema，并原子保存安装文件后切换首页。访问路径仍为 `/user/login`。
4. 已安装模板内容更新后会显示「待更新 / 更新并启用」；预览图 URL 带更新时间，避免继续使用旧封面缓存。恢复默认模板沿用原有操作。

此协议安装的是声明式 JSON，不是独立 Vue 构建 ZIP。后续文案、配色与既有布局配置可以直接发布 JSON；新增任意布局或修改渲染代码仍需更新本系统前端。分发端下架不等于远程卸载已安装文件。

### 验证

- 后端：在 `backend` 目录运行 `go test ./...`。
- 黑金渲染回归：在 `frontend` 目录运行 `pnpm test:gold-template`，覆盖实际模板数据、卡片语义、离线图标、颜色校验、样式范围和减少动态效果。
- 前端：运行 `pnpm exec vue-tsc --noEmit` 与 `pnpm build`，再按原有发布流程部署。

上线前应在测试环境完成实际数据库与两个服务的连通性验证；仅通过单元测试或生成构建产物，不代表已经在生产后台发布模板。
