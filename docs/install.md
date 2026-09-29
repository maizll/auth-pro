# 安装部署

在已经安装宝塔面板，并且软件商店里已经安装 Nginx 和 MySQL 的服务器上，把下面这一条粘贴到宝塔终端。把 `example.com` 换成站点域名。脚本从官网下载已发布的安装包，核对 SHA256 和签名后再安装。下载不需要登录，也不需要令牌。脚本不会替你安装 MySQL。

```bash
curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com
```

网站目录默认是 `/www/wwwroot/example.com`。不写 `--port` 时从 `19127` 起自动找空闲端口。要指定端口或目录：

```bash
curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com --port 19127 --site-root /www/wwwroot/example.com
```

命令会创建纯静态站点、只允许本机访问的数据库、反向代理和进程守护，然后在本机完成安装向导。结束时打印网址、管理员账号 `admin`、随机密码、后端端口和数据库信息，并写到一个仅所有者可读的文件，终端里会给出这个文件的路径。

已经有同名站点、非空网站目录或同名数据库时，命令会停下来，不会覆盖或删除原有内容。已经有 `backend/install.lock` 时同样停下来，请改用升级。

证书申请失败时站点保持 HTTP，终端会提示之后在面板里申请。进程守护没有装上时，终端会打印只包含本站点的 systemd 单元。

只要放文件、先不启动、也不创建管理员：

```bash
curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com --no-start
```

## 已经拿到发布包

发布包只提供 Linux amd64，文件名是 `auth_pro-full-vX.Y.Z.tar.gz`。包里没有安装脚本。安装脚本只从官网 `https://auth.maizll.com/install.sh` 下载。

包已经在本机、只放文件不启动时，先把官网脚本存下来再带上 `--package`：

```bash
curl -fsSL https://auth.maizll.com/install.sh -o /tmp/install.sh
bash /tmp/install.sh \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \
  --no-start
```

没有宝塔面板时，这个脚本只把文件放到网站目录，并说明还要在面板里做的步骤。

网站目录里已经有 `backend/install.lock` 时，安装脚本会拒绝执行，请改用下面的升级。

## 升级

不要把新压缩包直接解压覆盖正在运行的站点。升级和安装一样从官网执行。脚本自己下载最新包，核对 SHA256 后再替换程序，并保留原来的运行数据。升级会沿用原来的端口，不会重新找端口。

```bash
curl -fsSL https://auth.maizll.com/install.sh | bash -s -- upgrade example.com
```

进程已经由宝塔进程守护或 systemd 托管时，这条命令会替换文件后只结束本站进程，由守护按 `start.sh` 拉起。已经把包放在本机、并且要先停守护再升级时：

```bash
curl -fsSL https://auth.maizll.com/install.sh -o /tmp/install.sh
bash /tmp/install.sh upgrade \
  --site-root /www/wwwroot/example.com \
  --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \
  --no-start
```

守护还开着时，`--no-start` 会拒绝执行，请先在面板里停止守护。

新的升级备份写到 `/www/backup/auth-pro/example.com/upgrade/`。健康检查通过后，每一类备份只保留最近 3 份。机器上没有 `/www` 时，备份仍放在原来的数据目录里。

## 在线更新

登录后台，打开「在线更新」（仅超级管理员）。页面上可以点「检查更新」和「立即更新」。

点「检查更新」后，程序向源站读取最新版本并下载安装包：

```text
https://auth.maizll.com/api/v1/update/latest.json
```

后台不显示仓库地址，也不能手填更新地址。

安装包会核对 SHA256 和签名。更新失败时会尝试把页面和程序换回备份。健康检查成功之后，才删除超出 3 份的旧备份。
