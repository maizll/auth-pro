# auth-pro

auth-pro 是一套授权与许可证管理系统，卖给需要自建授权中心的企业。它包含管理后台、代理端、用户端、开发者面板，以及给业务系统调用的授权校验 API。生产环境把前端构建产物和 Go 后端放在同一站点，由后端提供页面和 `/api`。

当前产品版本见仓库根目录 `VERSION`（现为 **1.6.0**）。

## 核心能力

- 授权：应用、版本、套餐、卡密、授权码，以及公开校验接口。
- 用户与代理：注册登录、余额、购买授权、代理等级与开码配额。
- 风控：盗版追踪、黑名单、告警与报表。校验失败不会自动发邮件或 Webhook。
- 源站：开发者入驻、插件与首页模板登记、审核、上架，并按应用公开软件源清单。
- 支付：系统设置里的支付配置页，按已启用插件分栏（易支付、易支付 V2、支付宝当面付）。
- 安装向导：首次运行写入数据库配置并创建超级管理员，完成后用 `install.lock` 关掉安装接口。

## 技术栈

- 后端：Go 1.22、Gin、MySQL、JWT
- 前端：Vue 3、TypeScript、Vite、Pinia、Vue Router、Element Plus、Tailwind CSS
- 本地前端要求：Node.js >= 20.19.0，pnpm >= 8.8.0（见 `frontend/package.json` 的 `engines`）

## 文档

| 文档 | 读者 |
| --- | --- |
| [文档索引](docs/README.md) | 全部 |
| [部署手册](docs/deployment.md) | 安装、升级、Nginx、在线更新 |
| [管理手册](docs/admin.md) | 后台菜单 |
| [开发者章程](docs/developer/README.md) | 插件与首页模板登记 |
| [API 与 SDK](docs/api-sdk.md) | 业务系统接入 |
| [安全说明](docs/security.md) | 采购与运维 |
| [更新日志](CHANGELOG.md) | 版本记录 |
| [发布包目录](PACKAGING.md) | 打进宝塔站点的压缩包结构 |

## 本地开发

### 后端

```bash
cd backend
go mod download
go run .
```

默认监听 `19127`。也可 `PORT=19127 go run .`。数据库配置由安装向导写入数据目录的 `db.json`，完成后生成 `install.lock`。

### 前端

```bash
cd frontend
pnpm install
pnpm dev
```

开发环境把 `/api` 代理到 `http://localhost:19127`（可用 `VITE_API_PROXY_URL` 修改）。

浏览器打开前端地址，按向导填写 MySQL 并创建管理员。侧栏以「系统 → 菜单管理」为准，默认 `VITE_ACCESS_MODE=backend`，菜单来自 `GET /api/system/menus`。`frontend/src/router/modules` 只注册页面组件。

## 生产构建

```bash
cd frontend
pnpm install
pnpm build
```

发布包的目录约定见 [PACKAGING.md](PACKAGING.md)。服务器上的安装、升级、反代和回滚见 [部署手册](docs/deployment.md)。

## 署名

本仓库基于上游开源项目继续开发，许可证与界面来源见 [NOTICE](NOTICE)。
