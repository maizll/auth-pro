# AuthPro 客户端 SDK 混合架构设计

日期：2026-09-21  
状态：已批准落地（本 PR）

## 背景

旧「SDK 接入包」把应用密钥与模块开关烘焙进单文件 PHP/JS 模板汤，无法把可复用的核心库直接丢进客户源码树。用户批准改为 **版本化多语言库 + 薄 ZIP 接入包**。

## 架构

1. **版本化 SDK 库**（仓库根目录，与一次性 codegen 分离）
   - `sdk/php`
   - `sdk/node`
   - `sdk/python`
   - `sdk/go`
   - `sdk/browser`
2. **跨语言同一公共 API**
   - `boot(config)`：加载配置；按 `modules` 执行启动逻辑；license 失败时拦截/抛错（piracy → 拦截页或结构化错误）
   - `verify()`：仅授权校验，返回 `{ ok, code, message, data }`，不强制退出进程
   - `checkUpdate(currentVersion)`：对照 `/api/app/version/check`
   - `ads(slot)`：广告位 `home-banner` / `sidebar` / `popup` → `/api/v1/public/advertisements`
   - `pluginSourceUrl()`：应用隔离清单 `{baseUrl}/software-source/{appKey}/index.json`
3. **应用差异只在 config**，不写死进库源码。
4. **管理端「下载接入包」** 生成薄 ZIP：

```text
auth-pro-client-{app}/
  README.md
  config.json
  examples/{php,node,python,go,browser}/…
  vendor/{php,node,python,go,browser}/…   # sdk/* 快照（离线 require/import）
```

5. **浏览器 SDK 不嵌入 `appSecret`**：聚焦 ads / pluginSource；verify / checkUpdate 文档要求服务端 SDK 或同源代理（`proxyVerifyUrl` / `proxyCheckUpdateUrl`）。
6. **完整模块套件**：license、piracy、update、ads、plugin_source（由 `config.modules` 开关控制 boot 行为；库内 API 始终为真实实现）。

## config.json（最小字段）

- `baseUrl`、`appId`、`appKey`
- `appSecret`（仅服务端语言包；浏览器示例配置省略）
- `modules` 布尔映射
- 可选：`licenseKey`、`domain`、`serverIp`、`appVersion`

## 接线

- `backend/handler/sdk_pack.go` 从嵌入的 `sdk_assets`（同步自 `sdk/*`）组装 ZIP，并渲染 README / examples / config。
- 旧单文件 `auth_pro_sdk.php` / `auth-pro-sdk.js` 模板汤不再作为主产物。
- 管理端 / 开发者文档文案与上述结构对齐。
- 不在本变更中 bump 产品 `VERSION` 或切割 GitHub Release。
- 本阶段不强制发布到 Packagist/npm；`composer.json` / `package.json` / `go.mod` 按可发布形态布局，ZIP vendor 足够 v1。

## 同步嵌入快照

修改 `sdk/` 后执行（**不要**把 `sdk/go/go.mod` 直接放进 `sdk_assets/go/`，嵌套 module 会导致 `go:embed` 跳过整棵树）：

```bash
rm -rf backend/handler/sdk_assets
mkdir -p backend/handler/sdk_assets/_meta backend/handler/sdk_assets/go
cp -a sdk/php sdk/node sdk/python sdk/browser backend/handler/sdk_assets/
cp -a sdk/go/authpro backend/handler/sdk_assets/go/
cp sdk/go/go.mod backend/handler/sdk_assets/_meta/go.mod.txt
rm -rf backend/handler/sdk_assets/python/authpro/__pycache__
```

打包时会把 `_meta/go.mod.txt` 写回 ZIP 内的 `vendor/go/go.mod`。发布脚本 `scripts/build-release.sh` 在 `go build` 前会执行同样同步。
