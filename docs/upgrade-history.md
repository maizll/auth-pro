# 旧版本升级记录

这份记录只给维护者留档，不进入官网文档。当前安装和升级看 [部署手册](deployment.md)。

## 从 1.5.5 或 1.5.6 升级到 1.5.7

1.5.5 和 1.5.6 的在线更新会先让守护停进程。宝塔「进程守护管理器」使用自己的 supervisor，更新脚本手里的 `supervisorctl` 通常停不到它。脚本接着结束本进程，守护立刻把**还没换上的旧二进制**拉起来，端口刚空又被占上，更新就报「端口刚释放又被占用」并回滚。所以后台在线更新在宝塔守护下不可用。

1.5.7 改成「替换文件后退出，由守护拉起」。这套逻辑在**新程序里**。从 1.5.5/1.5.6 升到 1.5.7 这一次，服务器上跑的仍是旧程序，在线更新还会走旧逻辑。这一次请用 **1.5.7 包里的** `baota-upgrade.sh`，不要用网站上旧的脚本。

推荐（守护开着也可以，脚本发现守护正在托管时不会去停它）：

```bash
tar -xzf /tmp/auth_pro-full-v1.5.7.tar.gz -C /tmp/auth-pro-1.5.7
bash /tmp/auth-pro-1.5.7/baota-upgrade.sh --yes --start \
  --site-root /www/wwwroot/auth.maizll.com \
  --package /tmp/auth_pro-full-v1.5.7.tar.gz
```

也可以先在宝塔「进程守护」里停止本站点，再执行同一条命令但把 `--start` 换成 `--no-start`，最后在守护里启动。启动命令仍是 `backend/start.sh`，运行目录仍是 `backend/`。不要再 `nohup` 一份。

装上 1.5.7 之后，下一次在线更新才会走新的交接。这一次不要在后台点「在线更新」。

## 从 1.5.3 或 1.5.4 在线更新到 1.5.5 卡住时的恢复

1.5.4 及更早的在线更新会自己 `nohup` 拉起进程，并在替换二进制之前结束旧进程。从 1.5.3 或 1.5.4 在线更新到 1.5.5 这一次，执行更新的仍是服务器上的旧程序。这次更新如果停在「服务正在切换并重启」、19127 仍是旧进程，本仓库里的新逻辑要等这次恢复之后的下一次更新才会生效。下面的命令按生产站默认路径写，站点根或端口不同时只改前两行。先在宝塔「进程守护」里停止本站点，再在 SSH 里执行：

```bash
SITE=/www/wwwroot/auth.maizll.com
PORT=19127
# 结束仍占用端口的 auth_pro。守护若还开着会立刻拉起，所以必须先在面板里停止。
pids=$(ss -lptn "sport = :${PORT}" | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | sort -u)
for pid in $pids; do
  kill "$pid" 2>/dev/null || true
done
sleep 2
for pid in $pids; do
  kill -9 "$pid" 2>/dev/null || true
done
ls -l "$SITE/backend/auth_pro" "$SITE/backend"/auth_pro.backup.* 2>/dev/null || true
# 若 backend/auth_pro 的修改时间仍是更新前，说明新文件没换上，当前文件就是可启动的旧版本。
# 若它已损坏，而某个 auth_pro.backup.<时间> 是更新前的好文件，再执行：
# cp -a "$SITE/backend/auth_pro.backup.<时间>" "$SITE/backend/auth_pro"
chmod 755 "$SITE/backend/auth_pro"
# 回到宝塔进程守护，启动命令填 $SITE/backend/start.sh，运行目录填 $SITE/backend。不要再另开 nohup。
```

恢复后访问 `http://127.0.0.1:19127/api/system/version`，版本应回到更新前。确认只有一个进程监听该端口，且父进程是守护进程而不是 1。
