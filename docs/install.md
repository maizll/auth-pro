# 安装部署

在宝塔终端粘贴下面这一条即可安装。把 `example.com` 换成站点域名。脚本从官网下载已发布的安装包，核对 SHA256 和签名后再安装。下载不需要登录，也不需要令牌。

```bash
curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com
```

网站目录默认是 `/www/wwwroot/example.com`。目录不存在时会自动创建。不写端口时，后端从 `19127` 起使用第一个空闲端口，最多到 `19227`。同机其它站点正在监听，或在它的 `backend/baota.env` 里登记了这个端口（进程暂停也算），就会改用下一个。装完会打印实际端口。要固定端口或目录时：

```bash
curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com --port 19127 --site-root /www/wwwroot/example.com
```

这样写了 `--port` 之后，若该端口已被占用，命令会退出，不会改用别的端口，也不会结束占用它的进程。

已经有 `backend/install.lock` 时，命令会停下来并提示改用升级，不会覆盖现有站点。

宝塔的 `bt` 命令是面板菜单，没有稳定的建站参数。面板 API 默认关闭，还要单独打开密钥和 IP 白名单；建站、空库、反向代理和证书在不同面板版本上也不一样。脚本因此不调用这些接口。装完后请在面板里完成：

1. 创建网站，根目录与上面的网站目录相同
2. 创建一个空的 MySQL 数据库。数据库密码在浏览器安装向导里填写，不要写进命令
3. 站点反向代理到安装结束时打印的地址，形如 `http://127.0.0.1:19128`，并把 `backend/baota-nginx.snippet.conf` 里的 location 放进站点配置
4. 需要 HTTPS 时在面板申请证书
5. 进程守护的启动命令是网站根下的 `backend/start.sh`，运行目录是 `backend/`。说明在 `backend/baota-guardian.txt`

然后用浏览器打开站点域名。还没有安装锁时会进入安装向导：填写事先建好的空 MySQL，初始化数据表，创建超级管理员。

## 已经拿到发布包

发布包只提供 Linux amd64，文件名是 `auth_pro-full-vX.Y.Z.tar.gz`。包里带有 `baota-install.sh` 和 `baota-upgrade.sh`。

```bash
cd /www/wwwroot/example.com
tar -xzf auth_pro-full-vX.Y.Z.tar.gz
bash baota-install.sh
```

不想中途确认、安装包留在 `/tmp`、装完先不启动，可以这样：

```bash
AUTH_PRO_YES=1 AUTH_PRO_START=0 \
bash baota-install.sh \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz
```

网站目录里已经有 `backend/install.lock` 时，安装脚本会拒绝执行，请改用下面的升级。

## 升级

不要把新压缩包直接解压覆盖正在运行的站点。把包留在 `/tmp`，运行新版本包里的 `baota-upgrade.sh`，不要用站点上旧的脚本。

```bash
bash baota-upgrade.sh \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \
  --no-start
```

进程已经由宝塔进程守护或 systemd 托管时，把上面的 `--no-start` 换成 `--start`。脚本不会去停守护，而是替换文件后只结束本站进程，由守护按 `start.sh` 拉起。守护还开着时，`--no-start` 会拒绝执行，请先在面板里停止守护。

守护的启动命令是网站根下的 `backend/start.sh`，运行目录是 `backend/`。

## 在线更新

登录后台，打开「在线更新」（仅超级管理员）。页面上可以点「检查更新」和「立即更新」。

点「检查更新」后，程序向源站读取最新版本并下载安装包：

```text
https://auth.maizll.com/api/v1/update/latest.json
```

后台不显示仓库地址，也不能手填更新地址。

安装包会核对 SHA256 和签名。更新失败时会尝试把页面和程序换回备份。
