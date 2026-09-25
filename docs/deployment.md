# 部署手册

当前版本 **1.5.4**。发布包只提供 **Linux amd64**。压缩包里有哪些文件见 [PACKAGING.md](../PACKAGING.md)。

运行数据目录与进程的工作目录一致。宝塔脚本默认把它放在网站根下的 `backend/`。后端解析数据目录的顺序是：环境变量 `AUTO_PRO_DATA_DIR`，否则在当前工作目录或其子目录 `backend/` 中寻找 `install.lock`、`db.json` 或 `go.mod`，再否则用可执行文件所在目录。

这个目录里会有 `db.json`（数据库口令，权限应收成 `600`）、`install.lock`、`jwt.secret`，以及插件、模板、更新包和日志。不要把该目录暴露到公网。

源站签发商业版快照前，要在这台机器上生成签名私钥，并把打印出来的公钥交给维护者打进发行包。私钥只留在本机。

```bash
cd /www/wwwroot/example.com/backend
./auth_pro store-keygen
```

命令把私钥写到数据目录 `store/snapshot-ed25519.key`（权限 `0600`）。文件已存在时会拒绝覆盖，确认更换才加 `--force`。屏幕上只有公钥的 base64 和一行中文提示，把公钥发给维护者。维护者用下面任一方式打进发行包后再发布（`<打印出的公钥>` 换成命令打印的第一行）：

```bash
AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY='<打印出的公钥>' ./scripts/build-release.sh
```

```bash
go build -ldflags "-X auto_pro/handler.embeddedStoreSnapshotPublicKey=<打印出的公钥>" -o auth_pro .
```

未打入公钥的包会把所有快照视为无效：买方保持免费版，商店账号条显示警告；源站拒绝签发并返回明确错误。升级源站时保留 `store/snapshot-ed25519.key`，不要把私钥放进仓库或环境变量。完整说明见 [商业版](commercial.md) 和 [发布包目录](../PACKAGING.md)。

## 宝塔：全新安装

脚本在发布包根目录，仓库里对应 `scripts/baota-install.sh`（逻辑在 `scripts/baota-lib.sh`）。面板里的建站、空 MySQL、SSL 和进程守护开关仍要手工做，脚本不改面板数据库。

已有 `backend/install.lock` 时安装脚本会拒绝，应改走升级。

```bash
cd /www/wwwroot/example.com
tar -xzf auth_pro-full-v1.5.4.tar.gz
bash baota-install.sh
```

非交互、包留在 `/tmp`、先不启动：

```bash
AUTH_PRO_YES=1 AUTH_PRO_START=0 \
bash baota-install.sh \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-v1.5.4.tar.gz
```

### 安装脚本会做的事

1. 确认网站根（`--site-root`、`AUTH_PRO_SITE_ROOT`，或脚本就在已解压的站点里）。拒绝把 `/`、`/tmp`、`/www/wwwroot` 这类目录本身当成网站根。
2. 校验发布包，拒绝 `..` 和符号链接。放入 `index.html`、`assets/`、`backend/auth_pro` 等，不删除 `.user.ini` 这类面板文件。
3. 把 `backend/auth_pro` 设为 `755`。
4. 生成 `backend/baota.env`（`PORT`、`HOST=127.0.0.1`、`AUTO_PRO_DATA_DIR`）、`backend/start.sh`、`backend/baota-nginx.snippet.conf`、`backend/baota-guardian.txt`。`baota.env` 为 `600`。已有 `baota.env` 且没有用 `--port` 或 `AUTH_PRO_HOST` 覆盖时，默认保留。
5. 端口被占用时默认不杀进程。只有 `--stop-port` 且能确认占用者是本站 `backend/auth_pro` 才结束进程树；无关进程直接拒绝，并且此时还不会替换文件。
6. `--start` 时在 `backend/` 后台启动，并请求 `http://127.0.0.1:19127/api/install/status`。数据库口令只在网页安装向导里填写。

常用选项：

| 选项 | 作用 |
| --- | --- |
| `--site-root DIR` | 网站根 |
| `--package FILE` | `auth_pro-full-vX.Y.Z.tar.gz` |
| `--source DIR` | 已经解压好的发布目录 |
| `--port PORT` | 后端端口，默认 `19127` |
| `--start` / `--no-start` | 装完是否立刻后台启动。不启动时交给进程守护 |
| `--stop-port` | 只结束本站 `backend/auth_pro` 占用的端口 |
| `--yes` / `-y` | 不再询问。也可用 `AUTH_PRO_YES=1` |
| `--dry-run` | 只打印步骤，不改网站文件 |
| `-h` / `--help` | 帮助 |

对应环境变量：`AUTH_PRO_SITE_ROOT`、`AUTH_PRO_PACKAGE`、`AUTH_PRO_SOURCE`、`AUTH_PRO_PORT`、`AUTH_PRO_START=0|1`、`AUTH_PRO_YES=1`、`AUTH_PRO_STOP_PORT=1`、`AUTH_PRO_DRY_RUN=1`、`AUTH_PRO_DATA_DIR`、`AUTH_PRO_HOST`（默认 `127.0.0.1`）。

装完后用浏览器打开站点域名。没有 `install.lock` 时会进入安装向导：填写事先建好的空 MySQL，初始化表，创建超级管理员。库里已有 `admins` 或业务数据时，初始化与创建管理员会被拒绝。

## 宝塔：升级

不要先把新 tar 解压覆盖正在运行的站点。把包留在 `/tmp`，用 `--package` 指向它。升级前先在宝塔「进程守护」里停止本站点，否则刚结束的进程会被立刻拉起，脚本会停在替换之前。

```bash
bash baota-upgrade.sh \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-v1.5.4.tar.gz \
  --stop-port --no-start
```

要求数据目录里已有 `db.json` 或 `install.lock`。

### 升级会备份和核对的内容

替换前把运行数据拷到：

```text
backend/updates/backups/baota-upgrade-<时间>-<pid>/
```

能连上 MySQL 时用 `mysqldump` 导出 `db.sql`。口令只放在 `MYSQL_PWD` 环境变量里，不放进命令参数。没有客户端或暂时不能连库时加 `--skip-mysql`（或 `AUTH_PRO_SKIP_MYSQL=1`），`db.json` 文件副本仍会备份。

只替换页面、`assets/`、`backend/auth_pro` 以及 `baota-install.sh`、`baota-upgrade.sh`、`baota-lib.sh`。替换后核对下列路径的 SHA256 与替换前一致：

- 文件：`db.json`、`install.lock`、`jwt.secret`、`auto_pro.log`、`auto_pro.pid`
- 目录：`plugins/`、`home-templates/`、`software-source-cache/`、`updates/`（不含本次 `backups/`）、`app-releases/`、`logs/`、`advertisement-images/`、`source-packages/`

`db.json` 与 `jwt.secret` 会收成 `600`。选项与安装脚本相同，另有 `--skip-mysql`。`--dry-run` 不改文件、不导出数据库。

## 进程守护

守护的启动命令用安装脚本生成的 `backend/start.sh`，运行目录用 `backend/`（与 `start.sh` 所在目录相同）。`start.sh` 会加载同目录的 `baota.env`，然后执行 `./auth_pro`。说明写在 `backend/baota-guardian.txt`。

升级或清理残留进程之前，先在守护里停止此项。守护开着时在外面杀进程，会被立刻拉起来。

## Nginx 与 SSL

后端默认只监听 `127.0.0.1:19127`（`baota.env` 的 `HOST` 与 `PORT`）。站点反代到该地址。证书在宝塔面板申请，脚本不申请证书。

把 `backend/baota-nginx.snippet.conf` 里的 `location` 放进站点 `server`。脚本生成的拦截是：

```nginx
location ^~ /backend/ { return 404; }
location = /baota-install.sh { return 404; }
location = /baota-upgrade.sh { return 404; }
location = /baota-lib.sh { return 404; }
location ~* ^/(db\.json|install\.lock|jwt\.secret)$ { return 404; }
location ~* \.(log|pid)$ { return 404; }
```

反代示例（片段文件里以注释给出，需自行放进 `server`）：

```nginx
location / {
    proxy_pass http://127.0.0.1:19127;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

若 Nginx 直接读网站根的静态文件，而不是整站反代到 Go，仍必须拦截上面的路径。不要给 `backend/`、插件目录或模板目录再配可执行的静态 alias。

生产进程从盘上的 `index.html` 提供前端。找不到盘上前端时启动失败，或对页面返回 503 并带版本号，不会静默改用编译进二进制的旧页面。只有开发或引导才设置 `AUTO_PRO_ALLOW_EMBEDDED_FRONTEND=1`。盘上前端根可用 `AUTO_PRO_FRONTEND_DIR` 指定；不设时按 `data/frontend/current` 或宝塔网站根解析。

## 手工部署（不用宝塔脚本）

1. 构建前端：`cd frontend && pnpm install && pnpm build`。
2. 按 [PACKAGING.md](../PACKAGING.md) 把 `index.html`、`assets/` 放在网站根，二进制放在 `backend/auth_pro`。
3. 工作目录设为 `backend/`，设置 `PORT=19127`。需要与脚本相同的数据目录时，再设 `AUTO_PRO_DATA_DIR` 为该 `backend/` 的绝对路径。
4. 用上面的 Nginx 片段反代并拦截敏感路径。
5. 浏览器完成安装向导。

仓库里的 `scripts/restart-backend.sh` 会在本机编译并按 `PORT`（默认 19127）重启，适合开发机，不是宝塔进程守护的替代品。

## 在线更新

管理端「在线更新」（仅超级管理员）默认向 GitHub 仓库 `maizll/auth-pro` 读取最新 Release：

```text
https://api.github.com/repos/maizll/auth-pro/releases/latest
```

Release 附件里要有 `latest.json`。清单里的 `package.signature` 必须是 `sha256:` 加上与包 SHA256 相同的 64 位十六进制，并且要与 GitHub Release 附件的 digest 一致。前端目录不是符号链接时，先写入暂存目录再原子改名。更新失败时安装脚本会尝试把前端和二进制换回备份（见 `backend/handler/update.go` 里的回滚步骤）。勾选备份数据库时，SQL 导出到数据目录 `updates/backups/db-<时间>.sql`。

可用 `AUTO_PRO_UPDATE_URL` 改成自建 HTTPS 清单。默认源只接受 `maizll/auth-pro` 及 GitHub 官方附件存储。Gitee 只在显式配置镜像且 owner/repo 与配置一致时可用，不会默认信任历史仓库。

页面上可以「检查更新」和「立即更新」。失败提示为已尝试回滚。这不能代替升级前在进程守护里停止站点，也不能代替 `baota-upgrade.sh` 那份运行数据备份。

## 回滚

- 宝塔脚本升级：用 `backend/updates/backups/baota-upgrade-<时间>-<pid>/` 里的旧二进制、页面备份和 `db.sql` 手工还原。脚本没有单独的「一键回滚」子命令。
- 在线更新：失败路径会尝试恢复更新前的前端目录和二进制。数据库备份在 `updates/backups/db-*.sql`，需要时自行导入。
- 还原数据库之后不要删除 `install.lock`。锁丢失时，只要库里已有管理员或业务表数据，安装写接口仍会拒绝接管；但锁文件本身仍应留在数据目录。

## 故障：前后端版本不一致，端口上仍是旧进程

现象是页面版本和接口版本对不上，或更新后行为仍像旧包。常见原因是 **19127 上还留着旧的 `auth_pro`**，新的守护或新的启动又起了一份，请求打到了旧进程。

处理顺序：

1. 在宝塔进程守护里 **停止** auth-pro，避免杀掉之后被立刻拉起。
2. 确认谁占用了端口，例如 `ss -ltnp | grep 19127`。结束的应是本站 `backend/auth_pro`。不要结束无关进程。
3. 只通过守护启动：命令为网站根下的 `backend/start.sh`，运行目录为 `backend/`。不要再另开一个 `go run` 或旧目录里的二进制。
4. 看 `backend/logs/auto_pro.log`（脚本 `--start` 时写这里）以及启动日志里的前端根和版本。

健康检查地址是 `http://127.0.0.1:19127/api/install/status`。已安装时应表示系统已安装，且安装写接口返回 403。
