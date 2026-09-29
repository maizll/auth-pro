# 代码结构与维护指南

这份文档给接手源码的人用。产品版本以仓库根目录 `VERSION` 为准。当前是 **1.7.5**。

从这一版起，合并和发版前必须通过 `scripts/quality-check.sh`。检查失败时，GitHub 的 CI 和打标签发版都会停住，不会打出安装包。

## 目录

| 路径 | 职责 |
| --- | --- |
| `backend/` | Go 服务。模块名 `auto_pro`。入口是 `backend/main.go`，路由都在这里注册。 |
| `backend/handler/` | HTTP 处理和业务规则。大部分功能在这个目录。 |
| `backend/config/` | 端口、数据目录、数据库连接。 |
| `backend/middleware/` | 登录、安装锁、菜单权限。 |
| `backend/payment/alipayf2f/` | 支付宝当面付。 |
| `frontend/` | 管理后台、用户端、代理端、开发者端。Vue 3 + Vite。 |
| `frontend/src/views/` | 页面。后台菜单对应的页面在这里。 |
| `frontend/src/api/` | 前端调用的接口函数。 |
| `frontend/src/components/business/commercial/` | 顶栏商业版按钮和购买窗口。 |
| `scripts/build-release.sh` | 打 Linux amd64 安装包。 |
| `scripts/quality-check.sh` | 合并和发版前的检查。 |
| `scripts/commercial_mysql_e2e.py` | 商业版双站 MySQL 端到端。没有 MySQL 时退出码 77。 |
| `docs/` | 给管理员和接手人的说明。开发者登记章程在 `docs/developer/`。 |

前端页面由后端菜单决定侧栏。路由组件在 `frontend/src/router/`。`frontend/src/router/core/ComponentLoader.ts` 按路径加载 `views` 下的页面。

## 关键流程从哪个文件读起

### 授权

客户端校验入口是 `POST /api/license/verify`，处理函数在 `backend/handler/license_verify.go` 的 `LicenseVerify`。授权的增删改在 `backend/handler/license.go`。套餐在 `plan.go`，卡密在 `license_card.go`，站点绑定在 `license_site.go` 和 `license_domain_bind.go`。

后台页面是 `frontend/src/views/license/`。接口封装在 `frontend/src/api/license-manage.ts`。

### 商业版购买

买家站顶栏和购买窗口是 `frontend/src/components/business/commercial/CommercialHost.vue`。状态在 `frontend/src/utils/commercial.ts`，请求在 `frontend/src/api/store.ts`。

买家站接口在 `backend/handler/store_buyer.go`（`RegisterBuyerStoreRoutes`）：账号、绑定、注册、下单、查单、刷新、安装。购买窗口里的注册先请求源站 `POST /api/v1/store/register`（验证码是 `POST /api/v1/store/register/email-code`）。这两个接口和官网注册调用同一套 `UserRegister` / `UserSendRegisterEmailCode`。老源站没有这两个地址时，买家改请求 `/api/user-panel/register` 和 `/api/user-panel/register/email-code`。密码只转发给源站，不写入本机，也不打日志。已绑定后的「管理授权」由买家 `POST /api/store/manage-link` 用绑定签名向源站 `POST /api/v1/store/auth/handoff` 要一次性链接，源站只存票据哈希；浏览器打开 `https://auth.maizll.com/user/handoff` 或 `/agent-panel/handoff` 上的片段票据，换成短时登录状态后只进入「我的授权」。打开购买窗口时用 `GET /api/v1/store/binding` 向源站核对绑定是否还在，处理函数是 `store_source.go` 的 `StoreBindingCheck`。请求源站的 HTTP 客户端在 `store_buyer_client.go`，绑定和注册共用 `newSourceHTTPClient`。源站根地址在正式程序里固定为 `https://auth.maizll.com`，测试只在 `store_buyer_hooks_test.go` 里替换。

源站侧创建订单、快照和绑定在 `store_source.go`、`store_orders.go`、`store_policy.go`。买家本机快照和「是不是商业版」在 `store_access.go`。升级时删旧菜单、删旧连接配置的迁移在 `store_migrate.go` 和 `menu.go` 的 `removeRetiredStoreMenus`。这些迁移要留着，老客户升级还要走。

授权购买、代理开通、财务充值和换绑付款的二维码弹窗共用 `frontend/src/components/core/pay/PayQrDialog.vue`。商业版购买窗口里的付款区是同一步骤中的二维码、倒计时和「打开付款页」，不要再包一层新组件。

### 付费包下载

源站判断授权能否下载、签发短时票据，在 `backend/handler/store_download.go` 的 `StoreDownloadTicket`。收费包和从仓库导入后确认写入的插件、模板，都经过 `paid_origin.go` 的 `settlePaidZipBytes`，再进存储管理的主备和分片（`storage_flow.go`）。没有启用的存储位置时才暂存本站。买家站把包装进本机插件或模板，在 `store_buyer_client.go` 的 `installPaidPackage`。应用商店页面是 `frontend/src/views/plugin-store/index.vue`。应用「授权系统」的发布版本仍放在本站更新目录，客户更新不走这套存储位置。

### 在线更新

后台页面是 `frontend/src/views/online-update/index.vue`，请求在 `frontend/src/api/update.ts`。页面不展示更新地址。

客户站只向 `https://auth.maizll.com/api/v1/update/latest.json` 要清单，安装包和历史版本也走源站。实现在 `backend/handler/update.go`。源站对外提供这些清单和安装包的接口在 `backend/handler/update_distribute.go`，数据来自应用 `app_f93896d80066_5811` 的发布版本。后台从仓库导入安装包的共用逻辑在 `backend/handler/release_import.go`。

后端路由在 `backend/main.go`：客户站管理接口是 `/api/system/update/status|history|check|apply` 和 `jobs/:id`。源站公开接口是 `/api/v1/update/latest.json`、`/api/v1/update/releases.json` 和 `/api/v1/update/package/:version`。拉包、校验、解压、重启仍在 `update.go`。进程守护启动模板是 `backend/handler/guardian_start.sh`，打包时复制为发布包根目录的 `guardian-start.sh`。

## 同一功能只留一处

新功能先找上面的文件，在原处改。不要为一次需求再复制一套付款区、购买窗口或订单接口。

数据库迁移和老客户升级代码保留。旧地址跳转也保留：`/source-station/edition`、`/source-station/store-orders`、`/source-station/store-revenue`、`/source-station/store-licenses`，以及当面付和 `/agent/finance` 的回跳。见 `frontend/src/router/routes/staticRoutes.ts`。

## 单文件行数

目标是单个 Go、Vue、TypeScript 文件不超过 800 行，文件名能看出职责。1.6.9 改手机列表排版、顶栏商业版胶囊和购买窗口，没有做这次拆分。下面这些文件已经超过 800 行，拆分留到后续版本，避免在这次改动里搬动下单、支付和授权代码。

商业版和商店，优先拆这些：

| 文件 | 大约行数 | 打算拆成 |
| --- | --- | --- |
| `backend/handler/source_station_store.go` | 3200 | 目录存储、插件、模板、开发者申请分开 |
| `backend/handler/source_developer.go` | 1000 | 开发者目录接口与校验分开 |
| `backend/handler/source_station_versions.go` | 960 | 版本状态单独一个文件 |
| `backend/handler/store_source.go` | 900 | 绑定、快照、源站下单分开 |
| `backend/handler/paid_origin.go` | 820 | 外链拉取与私有包落盘分开 |
| `frontend/src/components/business/commercial/CommercialHost.vue` | 1000 | 仍只保留这一套购买窗口，把样式和付款步骤拆到同目录 |
| `frontend/src/views/plugin-store/index.vue` | 1100 | 插件列表与模板列表分开 |
| `frontend/src/views/source-station/components/CatalogWorkbench.vue` | 1500 | 列表、编辑、上传分开 |

其余超过 800 行的还有授权、支付、实名、在线更新和若干页面（例如 `license.go`、`epay.go`、`realname.go`、`update.go`、用户购买页）。它们和行为稳定相关，同样留到后续版本，不在 1.6.9 里移动。

## 注释

注释写给后来改这段代码的人。不写「设置变量」「返回结果」这种复述代码的句子。

1. 每个 Go 文件开头用中文写这个文件负责什么、不负责什么。`//go:build` 仍放在第一行。这不是 `package` 文档，和 `package` 之间空一行。
2. 导出函数、HTTP handler 和关键内部函数写中文：做什么、输入是什么、成功返回什么、什么情况下返回错误或哪个业务码。
3. 授权校验、商业版签名和快照、支付回调、付费包下载、在线更新，在分支处写清为什么这样分，而不是只写走了哪条分支。
4. Vue 组件在文件顶部说明这个组件给谁用、点下去做什么。复杂的状态（例如重新绑定、付款轮询）在计算属性或函数旁边写原因。
5. 1.6.8 覆盖购买窗口按源站核对绑定。1.6.9 覆盖手机列表排版、顶栏商业版胶囊和购买窗口账号栏，以及这一版改过的文件。授权、实名、支付渠道的其余函数在后续版本补齐。

## 本地检查

需要 Go 1.22、Node.js 22、pnpm 8 以上。在仓库根目录：

```bash
pnpm -C frontend install --frozen-lockfile
./scripts/quality-check.sh
```

脚本依次执行：

1. `go vet ./...`（在 `backend/`）
2. `staticcheck ./...`，配置在 `backend/staticcheck.conf`，包含未使用代码。面向用户的错误文案经常以 GitHub、Gitee、Logo、ZIP 开头，所以关闭了 ST1005，避免为了检查去改接口文案。包注释和导出名风格（ST1000、ST1003 等）保持 staticcheck 默认关闭，避免改 JSON 字段名。
3. `vue-tsc --noEmit`
4. `eslint . --max-warnings 0`。配置在启动时读取已提交的 `frontend/.auto-import.json`（自动导入的全局变量）。改了 `vite.config.ts` 里的自动导入后，要在本地跑一次 Vite 把这份文件更新并提交，否则 CI 上的 eslint 会缺全局变量或直接打不开配置。
5. 未使用导出：`frontend/knip.json` 把页面和组件当作入口，接口文件不算入口。结果必须是 `frontend/knip-unused-baseline.json` 的子集。多出一个未使用导出，检查就失败。自动导入的函数 knip 可能看成未使用，已经写进基线；新代码请在调用处显式 import。删掉导出后，把基线里对应的行也删掉。

后端测试：

```bash
cd backend && go test ./...
```

商业版双站（要本机 MySQL，会占用 18081 等端口，并编译带 `-tags e2e` 的买家程序）：

```bash
cd backend && go test -count=1 -timeout 30m -run TestCommercialMysqlE2E ./handler
```

没有 MySQL 时这个测试会跳过。前端菜单标题等小测试在 `frontend/package.json` 的 `build` 脚本里，用 `pnpm -C frontend exec tsx src/utils/form/menu-title.test.ts` 可以单独跑。

## 发版

1. 改根目录 `VERSION`、后端版本常量和 `frontend` 的 `VITE_VERSION`，三者一致。补丁号不要超过 9。
2. 写 `docs/release-notes-X.Y.Z.txt` 和 `CHANGELOG.md`。在线更新展示的是发布说明文件。
3. 商店签名公钥不要换，除非单独做密钥轮换。当前公钥是 `pwAizm/sOyWCu+qi8+Dl/xJr0Upuamh5u7vL3wGT14A=`。
4. 本地先跑 `./scripts/quality-check.sh` 和 `go test ./...`。
5. 打标签 `vX.Y.Z` 并推送。`.github/workflows/release.yml` 会先跑质量检查，再执行 `scripts/build-release.sh`，最后上传安装包。检查失败就不会发版。
6. 同一个版本号不会再次触发已安装站点的在线更新。已经发出的标签不要覆盖。

合并请求走 `.github/workflows/ci.yml`：质量检查加上 `go test ./...`。请在仓库设置里把这个检查设为合并前必须通过。
