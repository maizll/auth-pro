# 宝塔发布包目录规范

本项目每次编译打包都必须生成“宝塔当前网站根目录直接解压即用”的目录结构。

## 标准目录

```text
auth_pro-full-v1.5.0.tar.gz
├── index.html
├── backend-unavailable.html
├── version.json
├── favicon.ico
├── assets/
│   ├── index-xxxx.js
│   ├── index-xxxx.css
│   └── ...
├── backend/
│   └── auth_pro
├── manifest.json
├── baota-install.sh
├── baota-upgrade.sh
└── baota-lib.sh
```

## 必须遵守

- `index.html` 必须位于压缩包根目录。
- `assets/` 必须位于压缩包根目录，并且和 `index.html` 同级。
- 不得在宝塔网站根目录外再嵌套一层 `frontend/`。
- Go 二进制固定放在 `backend/auth_pro`。
- `manifest.json` 中的 `frontendDir` 固定为 `.`。
- `manifest.json` 中的 `backendFile` 固定为 `backend/auth_pro`。

## 解压后的服务器目录

如果宝塔当前网站根目录是 `/www/wwwroot/example.com`，解压后必须是：

```text
/www/wwwroot/example.com/
├── index.html
├── backend-unavailable.html
├── version.json
├── favicon.ico
├── assets/
├── backend/
│   └── auth_pro
├── manifest.json
├── baota-install.sh
├── baota-upgrade.sh
└── baota-lib.sh
```

这样浏览器请求 `/assets/index-xxxx.js` 时会命中真实文件，不会 fallback 到 `index.html`。

安装完成后，运行数据写在 `backend/`（与进程守护的运行目录一致），这些文件不在发布包里，升级时必须保留：`db.json`、`install.lock`、`jwt.secret`，以及 `plugins/`、`home-templates/`、`software-source-cache/`、`updates/`、`app-releases/`、`logs/`、`advertisement-images/`、`source-packages/`。

## 宝塔一键安装 / 升级

脚本在发布包根目录，解压后和 `index.html` 同级。面板里的建站、空库、SSL、进程守护开关和 Nginx 保存仍要手工做。仓库里可跑 `bash scripts/test-baota-scripts.sh` 检查权限、端口冲突、配置文件和升级备份；真实面板上的守护拉起和反代需要人工再看一遍。

全新安装（站点目录还没有 `backend/install.lock`）：

```bash
cd /www/wwwroot/example.com
tar -xzf auth_pro-full-vX.Y.Z.tar.gz
bash baota-install.sh
```

已有站点升级。不要先把新包解压覆盖正在运行的目录。1.5.7 起，守护仍在托管本进程时用新包里的脚本加 `--start`，脚本会替换文件后只结束本站进程。也可以先在「进程守护」里停止本站点，再执行 `--no-start`：

```bash
bash baota-upgrade.sh \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \
  --stop-port --no-start
```

脚本会把运行数据拷到 `backend/updates/backups/baota-upgrade-<时间>-<pid>/`，能连上数据库时再用 `mysqldump` 导出 `db.sql`。替换的是页面、`assets/` 和 `backend/auth_pro`，并把它设为 `755`。

进程守护建议：

- 启动命令：`/www/wwwroot/example.com/backend/start.sh`
- 运行目录：`/www/wwwroot/example.com/backend`
- 环境在 `backend/baota.env`（默认 `PORT=19127`、`HOST=127.0.0.1`）
- 在线更新（1.5.7 及以后）会退出并交给守护拉起。手工替换或 `--no-start` 前先停止守护，否则进程会被立刻拉起

Nginx 反代到 `127.0.0.1:19127`，并把 `backend/baota-nginx.snippet.conf` 里的 `location` 放进站点 `server`，至少拦截 `/backend/`、`db.json`、`install.lock`。同一说明也写在 `backend/baota-guardian.txt`。非交互执行可设 `AUTH_PRO_YES=1`；只看步骤用 `--dry-run`。

**二进制只会从这个盘上目录提供前端。** 解压必须让 `index.html` 落在进程将解析到的根上（宝塔网站根，或 `AUTO_PRO_FRONTEND_DIR` / `data/frontend/current`）。缺文件时进程 **启动失败** 或对页面返回 **503 + 版本号**，不会静默改走 `go:embed static` 里的旧页。开发机引导才可设 `AUTO_PRO_ALLOW_EMBEDDED_FRONTEND=1`。启动日志会打印 `frontend root: mode=disk|embed` 以及 `index.html` 指纹 / 资源名。

## 版本号单一信源

仓库根目录 `VERSION`（当前 `1.6.0`）是产品线默认版本：

- 后端 `auto_pro/config.AppVersion` 仓库默认与 `VERSION` 一致；`./scripts/build-release.sh` / `.ps1` 无参数时读该文件，并用 `-ldflags` 注入 `AppVersion` / `BuildTime`。
- 前端 `VITE_VERSION` 与 `vite.config.ts` 的 `version.json` 同样对齐 `VERSION`；发布脚本会把参数版本写入 `VITE_VERSION`。
- GitHub Actions 打 `vX.Y.Z` 标签时用 tag 注入，覆盖仓库默认。
- 本地 `go build` 若忘记 `-ldflags`，二进制仍应报 `1.5.x`，不得静默显示 `1.0.0`。

## 商店快照验签公钥

自 1.5.6 起，源码默认内置源站 Ed25519 公钥 `pwAizm/sOyWCu+qi8+Dl/xJr0Upuamh5u7vL3wGT14A=`。`./scripts/build-release.sh` 在未设置 `AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY` 时直接使用这个默认值。源站升级到 1.5.6 后可以签发快照，买方可以验签。私钥不要打进包，也不要写进本文件。源站升级时保留数据目录 `store/snapshot-ed25519.key`，不要重新执行 `store-keygen`。

私钥和内置公钥不一致时，源站拒绝签发。构建时仍可用环境变量或 `-ldflags` 覆盖公钥，用于更换密钥。覆盖值不能是占位符 `PLACEHOLDER_NOT_CONFIGURED`，且必须是 32 字节公钥的标准 base64。覆盖成占位符或空字符串的包会把快照一律视为无效：买方保持免费版，源站拒绝签发。

```bash
AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY='<打印出的公钥>' ./scripts/build-release.sh
```

Windows：

```powershell
$env:AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY = '<打印出的公钥>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release.ps1
```

等价的 Go 链接参数（与版本号等其它 `-X` 一起写）：

```bash
go build -ldflags "-X auto_pro/handler.embeddedStoreSnapshotPublicKey=<打印出的公钥>" -o auth_pro .
```

`<打印出的公钥>` 只替换成 `store-keygen` 打印的第一行。换钥后的包要同时装到源站和买方站点，源站同时保留本机私钥文件。

## 构建命令

macOS / Linux：

```bash
./scripts/build-release.sh          # 使用根目录 VERSION
./scripts/build-release.sh 1.5.0
```

Windows PowerShell：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version 1.5.0
```

输出文件固定为：

```text
release/packages/auth_pro-full-v<版本号>.tar.gz
release/packages/latest.json
release/packages/releases.json
```

构建脚本只生成 `Linux amd64` 后端，版本参数必须匹配 `X.Y.Z`。`latest.json` 中记录平台、文件名、**GitHub** `maizll/auth-pro` Release 下载地址、文件大小和 SHA256；`releases.json` 合并保留已有历史版本，并将上一版本标签到当前版本之间的 Git 提交标题自动记录到对应版本的 `notes`。首次发布会记录当前 Git 历史；无 Git 历史时才使用兜底说明。可通过 `AUTO_PRO_RELEASE_NOTES` 显式覆盖本次更新内容（JSON 字符串数组或按行分隔文本）。

## GitHub Release 发布（规范发布面）

推送 `vX.Y.Z` 标签后由 `.github/workflows/release.yml` 构建并上传。不要删除历史 Releases，也不要 force-push 标签。

```bash
git tag v1.5.0
git push origin v1.5.0
```

每个 Release 必须包含：

```text
auth_pro-full-v1.5.0.tar.gz
latest.json
releases.json
```

在线更新默认读取 GitHub 最新 Release，再定位 `latest.json` 附件：

```text
https://api.github.com/repos/maizll/auth-pro/releases/latest
https://github.com/maizll/auth-pro/releases/latest/download/latest.json
```

服务端可通过 `AUTO_PRO_UPDATE_URL` 指向自建 HTTPS 镜像清单。GitHub 默认源只信任 `maizll/auth-pro` 的 Release API / 附件路径及 GitHub 官方附件重定向目标。

可选的 `scripts/publish-gitee-release.sh` / `.ps1` 只用于自建 Gitee **镜像**，必须显式传入 `--repository` / `-Repository`，避免误发到历史 fork。

## 完整性边界

在线更新同时核对两件事：

1. 压缩包大小和 SHA256。SHA256 可以发现下载损坏。
2. `latest.json` 的 `package.signature` 必须是 `sha256:<64 位十六进制>`，并且与包的 SHA256 一致。应用更新时，客户端再向固定地址 `https://api.github.com/repos/maizll/auth-pro/releases/tags/vX.Y.Z` 读取同名附件的 `digest`。摘要缺失、与本地哈希不一致，或清单签名为空/错误，都会拒绝安装。

因此只改镜像上的 `latest.json`（同时改下载地址和 SHA256）不能通过应用。自定义 `AUTO_PRO_UPDATE_URL` 仍可作为 HTTPS 镜像，但包内容必须与 `maizll/auth-pro` 上该版本附件的 GitHub 摘要一致，并且应用时要能访问 `api.github.com`。

发布脚本会把 `package.signature` 写成 `sha256:` 加包的 SHA256。GitHub 上传附件后计算的 `digest` 与这个值相同。下一次打 `vX.Y.Z` 标签发布时会带上该字段。签名为空的历史 `latest.json` 不能被已包含本校验的实例继续在线安装。

非 `current` 符号链接的网站目录会先把新前端复制到同级暂存目录，再改名切换；拷贝失败不会改正在服务的目录。名为 `current` 或本身是符号链接的安装仍写入 `releases/<版本>` 后切换链接。后端二进制仍先复制到 `*.next.<时间>`，再 `mv` 覆盖原文件。

仓库或 Release 发布权限一旦被攻破，攻击者仍可上传新的附件并由 GitHub 计算新摘要。请严格控制仓库管理员、私人令牌和 Release 发布权限。
