# 宝塔一条命令部署调研（1.7.5 草案）

状态：调研和原型，不合并。官网地址保持写死为 `https://auth.maizll.com`。

本文按结论追加。尚未在本机跑完的步骤标为「待实测」。

## 1. 网站根旁的整目录备份从哪来

用户服务器上 `/www/wwwroot/<站点>.overlay-backup.<时间>` 和 `/www/wwwroot/<站点>.backup.<时间>` 都是在线更新脚本生成的，不是 `scripts/baota-upgrade.sh` 生成的。

在线更新脚本模板是 `backend/handler/update_restart.sh.tmpl`，由 `backend/handler/update.go` 的 `writeOnlineUpdateScript` 填进 `__FRONTEND_CURRENT__`（即 `config.GetFrontendDir()`）。宝塔布局下可执行文件在 `<站点>/backend/auth_pro`，`config/frontend.go` 的 `defaultFrontendApplyDir` 会把前端目录解析成网站根。于是 `FRONTEND_CURRENT=/www/wwwroot/<站点>`。

### overlay-backup

`update_restart.sh.tmpl` 在二进制或数据目录落在前端目录里面时走覆盖模式：

```71:82:backend/handler/update_restart.sh.tmpl
case "$APP_BIN" in
  "$FRONTEND_CURRENT"/*) OVERLAY=1 ;;
esac
case "$DATA_DIR" in
  "$FRONTEND_CURRENT"|"$FRONTEND_CURRENT"/*) OVERLAY=1 ;;
esac

if [ "$OVERLAY" = "1" ]; then
  # 宝塔网站根同时放着页面和 backend/。不能把整个网站根改名，否则运行中的程序和数据目录会一起被搬走。
  FRONTEND_MODE="overlay"
  STAGING="${FRONTEND_CURRENT}.staging.$TS"
  FRONTEND_BACKUP="${FRONTEND_CURRENT}.overlay-backup.$TS"
```

时间戳格式是 `date '+%Y%m%d%H%M%S'`。备份目录与网站根同级：`/www/wwwroot/<站点>.overlay-backup.<时间>`。脚本只把将被替换的前端文件移进去（跳过 `backend` 和 `manifest.json`），成功后不删除。这就是宝塔站点每次在线更新多出一个整份前端目录的原因。

### .backup

同一模板的改名模式生成 `<前端目录>.backup.<时间>`：

```147:150:backend/handler/update_restart.sh.tmpl
else
  FRONTEND_MODE="rename"
  STAGING="${FRONTEND_CURRENT}.staging.$TS"
  FRONTEND_BACKUP="${FRONTEND_CURRENT}.backup.$TS"
```

当前端目录是普通目录，且二进制和数据目录都不在它下面时走这条。`mv` 把整个线上目录改名为 `<目录>.backup.<时间>`。若当时前端目录就是网站根，产物就是 `/www/wwwroot/<站点>.backup.<时间>`，里面是改名前的整站。覆盖模式的注释说明：宝塔网站根不能这样改名，所以现行宝塔布局应走 overlay。历史上或 `AUTO_PRO_FRONTEND_DIR` 指到独立目录时，仍会留下 `.backup.<时间>`。符号链接或目录名叫 `current` 时走 release 模式，备份名是同级的 `current.backup.<时间>`，不是 `<站点>.backup.<时间>`。

### 宝塔升级脚本实际写到哪里

`scripts/baota-lib.sh` 的 `baota_prepare_backup_dir` 写的是数据目录内部，不是网站根的同级目录：

```644:648:scripts/baota-lib.sh
baota_prepare_backup_dir() {
  local data ts
  data="$(baota_data_dir)"
  ts="$(date '+%Y%m%d%H%M%S')"
  BAOTA_BACKUP_DIR="${data}/updates/backups/baota-${BAOTA_ACTION}-${ts}-$$"
```

默认数据目录是 `<站点>/backend`，所以升级包备份在 `<站点>/backend/updates/backups/baota-upgrade-<时间>-<pid>/`。这里同样没有保留份数上限。

### 为什么会堆到 30 多份

`prune_binary_backups` 只删 `$APP_BIN.backup.*`（`backend/auth_pro.backup.<时间>` 这个文件），并且只留最近 3 份。成功交接 `handoff_mark_success` 里也只做这一处清理。`overlay-backup`、改名模式的 `.backup` 目录、以及 `backend/updates/backups/baota-*` 都没有清理。

### 清理方案（本轮不实现）

- 新备份改到 `/www/backup/auth-pro/<站点>/`，与网站根分开。overlay 前端备份、改名备份、宝塔脚本的 `baota-upgrade-*` 都放这里，各自只留最近 3 份（按目录 mtime）。
- 只处理名字严格等于「本站点目录名 + `.overlay-backup.` 或 `.backup.` + 14 位时间」的同级目录。不碰其它站点，不碰 `auth_pro.backup.*` 的现有 3 份策略。
- 旧位置：安装或升级开始时把匹配的历史目录移动到新目录，移完再按 3 份裁剪。移动失败则只打印说明，不删除旧目录。用户也可手工执行：确认目录名后 `mv /www/wwwroot/<站点>.overlay-backup.* /www/wwwroot/<站点>.backup.* /www/backup/auth-pro/<站点>/`，再按时间删掉第 4 份及更早的。
- 回滚仍要能找到最近一份。清理必须发生在健康检查成功之后，失败路径保留刚生成的那份。

## 2. 能否免掉网页安装向导

可以。向导要的输入就是数据库和管理员，对应三个已有接口，脚本用本机 curl 调用即可。

向导页面 `frontend/src/views/install/index.vue`：

| 步骤 | 字段 | 接口 |
| --- | --- | --- |
| 数据库 | host、port、database、username、password | `POST /api/install/test-db` |
| 建表 | 同上 | `POST /api/install/init-tables`（内部写 `db.json` 并执行 schema 与菜单种子） |
| 管理员 | adminUsername、adminPassword | `POST /api/install/create-admin`（bcrypt、JWT 密钥、`install.lock`） |

`backend/middleware/install_guard.go`：已有 `install.lock` 时一律 403。未安装时，带 `Origin` 且与请求 Host 不同源则 403；没有 `Origin` 的 curl 放行。因此安装脚本在后端起来之后，对 `http://127.0.0.1:<端口>` 发不带 Origin 的 POST 就能走完向导。库里已有本系统数据时，建表和创建管理员都会拒绝，不会覆盖。

脚本侧生成随机管理员密码（不要用页面前端默认的 `admin` / `123456`），用户名可用 `admin`。完成后打印：访问网址、管理员账号、数据库名、用户和密码。数据库口令仍不要出现在进程参数里，只在结束时打印一次。

后端没起来时不能代跑这三个接口。降级是打印中文手工步骤：浏览器打开站点，填写已建好的库和管理员，不要再自动写 `db.json`。

## 3. 面板实测

待实测。本环境 PID 1 是 tini，`systemctl` 不可用。官方安装脚本仍会先跑；Ubuntu 分支用 `update-rc.d` 注册面板，不依赖 systemd 启用 `btpanel.service`。Nginx / MySQL 是否能在无 systemd 时被面板脚本拉起，装完再记。
