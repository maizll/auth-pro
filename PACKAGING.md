# 宝塔发布包目录规范

本项目每次编译打包都必须生成“宝塔当前网站根目录直接解压即用”的目录结构。

## 标准目录

```text
auth_pro-full-v1.4.1.tar.gz
├── index.html
├── version.json
├── favicon.ico
├── assets/
│   ├── index-xxxx.js
│   ├── index-xxxx.css
│   └── ...
├── backend/
│   └── auth_pro
└── manifest.json
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
├── version.json
├── favicon.ico
├── assets/
├── backend/
│   └── auth_pro
└── manifest.json
```

这样浏览器请求 `/assets/index-xxxx.js` 时会命中真实文件，不会 fallback 到 `index.html`。

**二进制只会从这个盘上目录提供前端。** 解压必须让 `index.html` 落在进程将解析到的根上（宝塔网站根，或 `AUTO_PRO_FRONTEND_DIR` / `data/frontend/current`）。缺文件时进程 **启动失败** 或对页面返回 **503 + 版本号**，不会静默改走 `go:embed static` 里的旧页。开发机引导才可设 `AUTO_PRO_ALLOW_EMBEDDED_FRONTEND=1`。启动日志会打印 `frontend root: mode=disk|embed` 以及 `index.html` 指纹 / 资源名。

## 版本号单一信源

仓库根目录 `VERSION`（当前 `1.4.1`）是产品线默认版本：

- 后端 `auto_pro/config.AppVersion` 仓库默认与 `VERSION` 一致；`./scripts/build-release.sh` / `.ps1` 无参数时读该文件，并用 `-ldflags` 注入 `AppVersion` / `BuildTime`。
- 前端 `VITE_VERSION` 与 `vite.config.ts` 的 `version.json` 同样对齐 `VERSION`；发布脚本会把参数版本写入 `VITE_VERSION`。
- GitHub Actions 打 `vX.Y.Z` 标签时用 tag 注入，覆盖仓库默认。
- 本地 `go build` 若忘记 `-ldflags`，二进制仍应报 `1.4.x`，不得静默显示 `1.0.0`。

## 构建命令

macOS / Linux：

```bash
./scripts/build-release.sh          # 使用根目录 VERSION
./scripts/build-release.sh 1.4.1
```

Windows PowerShell：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version 1.4.1
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
git tag v1.4.1
git push origin v1.4.1
```

每个 Release 必须包含：

```text
auth_pro-full-v1.4.1.tar.gz
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

当前更新链路校验压缩包大小和 SHA256，不校验离线数字签名。SHA256 可以发现下载损坏，但仓库或 Release 发布权限一旦被攻破，攻击者仍可同时替换更新包和校验值。请严格控制仓库管理员、私人令牌和 Release 发布权限。
