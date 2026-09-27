# 安装部署

发布包只提供 Linux amd64，文件名是 `auth_pro-full-vX.Y.Z.tar.gz`。包里已经带了一键安装脚本 `baota-install.sh` 和升级脚本 `baota-upgrade.sh`。

先在宝塔面板里建好网站、一个空的 MySQL，并配好 SSL。脚本不改面板里的这些设置。数据库密码不要写进脚本，第一次用浏览器打开站点时，在安装向导里填写。

## 全新安装

把发布包放到网站目录，解压后执行安装脚本：

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

装完后用浏览器打开站点域名。还没有安装锁时会进入安装向导：填写事先建好的空 MySQL，初始化数据表，创建超级管理员。

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

默认向这个地址读取最新版本：

```text
https://api.github.com/repos/maizll/auth-pro/releases/latest
```

Release 附件里要有 `latest.json`。
