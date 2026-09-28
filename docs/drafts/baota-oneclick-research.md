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

## 3. 面板实测环境

禁止在 VM 宿主机上执行官方 `install_panel.sh`。该脚本会改防火墙、ufw 和服务，前几次都在安装期间或刚结束时把执行环境打崩，VM 被重置。

实测改到 Docker 容器内：`ubuntu:22.04`，需要时加 `--privileged`，使用容器自己的网络命名空间，不用 `--network host`。安装、Nginx、MySQL 和 `btpython` 调用都只在容器里做。宿主机只安装 Docker 引擎，不跑宝塔安装脚本。

若 Docker 起不来，则只下载面板压缩包做静态分析，并标明哪些调用没有实测。

2026-09-28 实测：宿主机只装了 `docker.io` 并用手动 `dockerd` 拉起（本机没有 systemd）。容器 `baota-lab` 使用 `ubuntu:22.04`、`--privileged`、默认 bridge 网络，没有 `--network host`。官方安装脚本只在容器内执行，面板装完。

容器内面板版本是 **Linux 面板 13.1.0**（`/www/server/panel/class/common.py` 的 `g.version`），`btpython` 是 Python 3.13.14（`/usr/bin/btpython` → `/www/server/panel/pyenv/bin/python3.13`）。端口文件 `/www/server/panel/data/port.pl` 为 8888。安装脚本在容器里跑 ufw 时把 INPUT 设成 DROP，但 ufw-init 报错，Docker 内置 DNS 的回包被丢掉。这只发生在容器网络里。处理是在容器内把 INPUT 改回 ACCEPT，没有动宿主机防火墙。

## 4. 面板 13.1.0 内部调用（源码已在容器里核对，成功调用见后文）

工作目录必须是 `/www/server/panel`，并把 `class/` 放进 `sys.path`。参数对象是 `public.dict_obj()`，用属性赋值。这些方法走 `public.M()` 读写面板 sqlite，不需要面板 HTTP 会话。`AddSite` 会在站点端口不是 80 时调用 `firewalls().AddAcceptPort`，80 则只检查 80 是否放行。

### 建站 `panelSite.panelSite.AddSite`

先要求 Nginx、Apache 或 OpenLiteSpeed 的二进制已经存在，否则返回「未安装任意 Web 服务」。关键参数：

- `webname`：JSON 字符串，形如 `{"domain":"example.com","domainlist":[],"count":0}`
- `path`：网站根，例如 `/www/wwwroot/example.com`。目录不存在会创建并 chown 给 `www`
- `type_id`：`0`
- `type`：`PHP`
- `version`：`00` 表示纯静态，且必须出现在 `GetPHPVersion` 的结果里
- `port`：`80`
- `ps`：备注
- `ftp`：`false`（不要顺手建 FTP）
- `sql`：`false`（数据库单独建，避免和下面的用户策略缠在一起）
- `ftp_username`、`ftp_password`、`datauser`、`datapassword`：sql/ftp 为 false 时仍要有空字符串，方法会读这些属性

同名站点：代码在 `sites.name` 已存在时把名字改成 `域名_端口` 再插入，不是直接拒绝。一条命令安装不能依赖这个行为。脚本应先查 `public.M('sites').where('name=?', (domain,)).count()`，已存在就停并打印手工说明，不调用 `AddSite`，避免面板再造一个 `域名_80`。

### 建库 `database.database.AddDatabase`

- `name`：库名，小写，`^[\w\.-]+$`，不能是 root/mysql/test/sys/panel_logs
- `db_user`：用户名，最长 32 字节；5.7/8.0/8.4/9.0/9.7 以外的版本限制 16
- `password`：不能含中文和一批标点；空则面板自己生成
- `address`：`127.0.0.1`。空字符串会被当成「指定 IP 但没填」
- `codeing`：`utf8mb4`（拼写就是 codeing）
- `ps`：备注
- `sid`：`0` 表示本机 MySQL

已存在则拒绝：sqlite 里同一 sid 已有该用户名，或 MySQL 里已有该库名（大小写都查）。返回 `status: false`，不会 DROP 重建。调用前再查一次，失败就整步退出，不删已有库。

### 反向代理 `panelSite.panelSite.CreateProxy`

- `sitename`：上面创建的站点名
- `proxyname`：3 到 40 字节，例如 `auth-pro`
- `proxydir`：`/` 表示整站
- `proxysite`：`http://127.0.0.1:<端口>`，必须带协议，且不能含 `?=&` 等字符（所以不要在 URL 里写路径参数）
- `todomain`：`$host`
- `type`：`1`（启用）
- `cache`：`0`
- `cachetime`：`1`（整数字符串，不能空）
- `subfilter`：`[]`
- `advanced`：`0`

同名代理或同一站点已有全局/目录代理时拒绝。`proxydir=/` 时会把 PHP 设成纯静态。生成的配置在 `/www/server/panel/vhost/nginx/proxy/<站点>/`。自定义片段（拦截 `/backend/`、`db.json`、`install.lock` 和 502 页面）不要塞进 `proxysite`。在 `CreateProxy` 成功后用 `nginx -t` 通过再写入站点 server 里的标记块；`nginx -t` 失败就删掉刚加的标记，保留代理本身并打印手工合并说明。

### 证书 `acme_v2.acme_v2.apply_cert_api`

参数：`id`（站点 id）、`auth_type`（`http` 或 `tls` 或 `dns`）、`auth_to`（http 验证时用站点根目录）、`domains`（列表的 JSON 或面板约定的字段）。没有公网解析的域名会在下单或验证阶段返回 `status: false` 和错误文本。本环境没有真实域名，只调用并记录失败返回，不重试，不改已有证书文件。

### 进程守护

面板自带类里没有进程守护管理器。图标是插件 `supervisor`（软件商店「进程守护管理器」）。安装入口是 `panelPlugin.panelPlugin().install_plugin`，参数 `sName=supervisor`，并带商店里的 `version` / `min_version`。插件安装要联网下载。若容器里装不上，降级为 systemd 单元，且只给本站点的 `start.sh`，不改其他服务。systemd 在本容器里不是 PID 1，降级路径只验证单元文件内容，不在容器里 systemctl。

Nginx 使用面板脚本：`bash install_soft.sh 1 install nginx 1.26`。在这个 Ubuntu 22.04 容器里，脚本退回编译并先编译了 OpenSSL 1.0.2u。完成后 `nginx -v` 是 **nginx/1.26.3**，`nginx -t` 通过，`/etc/init.d/nginx` 已启动。退出码 0。

MySQL 极速脚本 `install_soft.sh 1 install mysql 5.7` 在 Ubuntu 上先请求 `https://download-cdn1.bt.cn/rpm/centos22/64/bt-mysql57.rpm`，返回 404（脚本用 rpm，容器里也没有 rpm 命令），然后改下 `install/0/mysql.sh` 从源码编译 GreatSQL。编译已在容器内停掉，避免占满内存。建库实测改用容器内 MariaDB 10.6.23：把 socket 写进 `/etc/my.cnf`，把 root 口令写入面板 sqlite 的 `config.mysql_root`。`database.AddDatabase` 仍是面板自己的类，连的是本机 3306。正式环境应使用已经装好的面板 MySQL，不要在 Ubuntu 上默默走源码编译。

## 5. 原型实测（19127 被占用，不传 --port）

原型是 `docs/drafts/baota-oneclick-prototype.py`。容器里用 Python 占住 `127.0.0.1:19127`。另外放了 `/www/wwwroot/other.example/index.html`，跑完 md5 仍是 `b260098afc93a054427d63c4de6be6a1`，19127 的监听还在。没有停止其它进程。

第一次运行的完整输出：

```text
[信息] 端口 19127 不可用，继续查找
[信息] 使用后端端口 19128
[信息] 面板版本 13.1.0
[信息] AddSite 返回：{'siteStatus': True, 'siteId': 1, 'ftpStatus': False, 'databaseStatus': False, 'gitStatus': False}
[信息] AddDatabase 返回：{'status': True, 'msg': '添加成功'}
[信息] CreateProxy 返回：{'status': True, 'msg': '添加成功'}
[信息] 自定义 Nginx 片段：/www/server/panel/vhost/nginx/oneclick.lab.test.conf
[信息] 读取 supervisor 插件信息失败：Working outside of request context.
[手动] 没有装上宝塔进程守护管理器，已降级为 systemd 单元说明
       在面板软件商店安装「进程守护管理器」，启动命令填 /www/wwwroot/oneclick.lab.test/backend/start.sh
       或使用下面的单元，只管理本站点，不要改其它服务
[Unit]
Description=auth-pro oneclick.lab.test
After=network.target

[Service]
WorkingDirectory=/www/wwwroot/oneclick.lab.test/backend
ExecStart=/www/wwwroot/oneclick.lab.test/backend/start.sh
Restart=always

[Install]
WantedBy=multi-user.target

[信息] 进程守护：{'status': False, 'mode': 'systemd-text', 'port': 19128}
[信息] apply_cert_api 返回：{'status': False, 'msg': ["无法为['oneclick.lab.test']颁发证书，不能直接用域名后缀申请通配符证书! ", {'type': 'urn:ietf:params:acme:error:rejectedIdentifier', 'detail': 'Invalid identifiers requested :: Cannot issue for "oneclick.lab.test": Domain name does not end with a valid public suffix (TLD)', 'status': 400}], 'index': None}
----
网址: http://oneclick.lab.test/
管理员账号: 安装向导完成后打印（本原型不写 install.lock）
后端端口: 19128
数据库: authpro_oneclick 用户 authpro_oneclick 密码 gqPMnukQ3DrD0EoP 主机 127.0.0.1
```

退出码 0。站点配置里反代是 `proxy_pass http://127.0.0.1:19128;`，`# BEGIN AUTH_PRO_ONECLICK` 已写入并通过 `nginx -t`。库 `authpro_oneclick` 在 MariaDB 里存在。

再次执行同一条命令被拒绝，没有改站点：

```text
[错误] 站点 oneclick.lab.test 已存在，拒绝覆盖。请在面板里查看这个站点，本次没有修改它。
```

显式 `--port 19127` 也被拒绝，没有新建 `busyport.lab.test`：

```text
[错误] 端口 19127 已被占用或已写在其它 auth-pro 站点配置里，已停止。请换一个端口，本次没有新建站点。
```

### 进程守护的稳妥路径

`panelPlugin.get_soft_find("supervisor")` 离开面板 HTTP 请求就会报 `Working outside of request context`，不能当作一条命令安装的入口。

插件安装脚本 `https://download.bt.cn/install/plugin/supervisor/install.sh` 可以不登录面板就下载（HTTP 200）。它用豆瓣源 `pip install supervisor`，本次返回 `No matching distribution found for supervisor`。`systemctl` 在容器里也不存在，脚本照样以退出码 0 结束，`supervisord` 二进制当时没有装上。

补上 `btpip install supervisor`（PyPI，装上 4.3.0）后，生成 `/etc/supervisor/supervisord.conf`，执行插件 `config.py`，直接启动 `pyenv/bin/supervisord`（不调用 systemctl）。然后：

```text
AddProcess 返回：{'status': True, 'msg': '增加守护进程成功!'}
```

参数：`pjname=auth_pro_oneclick`，`user=www`，`path` 为 `backend/`，`command` 为 `backend/start.sh`，`numprocs=1`。配置写在 `/www/server/panel/plugin/supervisor/profile/auth_pro_oneclick.ini`。原型里的 `start.sh` 只是占位，马上退出，所以 `supervisorctl status` 是 `FATAL Exited too quickly`。这只说明守护已经接到这个命令，不是后端程序已经跑起来。

一条命令安装应优先走这条插件路径：自己用 `btpip install supervisor`，不要依赖脚本里的豆瓣源，也不要调用 `panelPlugin.install_plugin`。插件或 `supervisord` 起不来时，只打印本站点的 systemd 单元，不 `systemctl` 其它服务。

### 每步失败时

| 步骤 | 失败时 |
| --- | --- |
| 端口 | 显式端口被占用就退出。自动端口找不到空闲端口也退出。都不建站。 |
| 站点已存在或目录非空 | 退出，不调用 `AddSite`。 |
| 建站失败 | 打印面板手工建站说明，不建库。 |
| 建库失败 | 删掉本次新建的站点，打印手工建库说明。已有同名库时 `AddDatabase` 自己返回失败，原型不 DROP。 |
| 反代失败 | 删掉本次新建的库和站点，打印反代地址。 |
| 自定义片段写入后 `nginx -t` 失败 | 还原该站点配置，反代保留，打印手工合并 `baota-nginx.snippet.conf`。 |
| 进程守护失败 | 不删站，打印手工添加守护或 systemd 单元。 |
| 证书失败 | 不删站，保留 HTTP，打印到面板申请证书。本次 `.test` 域名被 Let's Encrypt 拒绝，返回见上面的输出。 |

免向导的接口结论仍见第 2 节。本原型没有启动 auth-pro 二进制，所以没有实际 POST 安装接口。
