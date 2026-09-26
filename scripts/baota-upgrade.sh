#!/usr/bin/env bash
# 宝塔面板：原地升级 auth-pro，保留数据库配置、安装锁和运行数据。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=baota-lib.sh
source "$SCRIPT_DIR/baota-lib.sh"

baota_print_help() {
  cat <<'EOF'
用法：
  bash baota-upgrade.sh [选项]

替换网站根里的页面、静态资源和 backend/auth_pro，保留运行数据。
默认数据目录是网站根下的 backend/。会保留：

  db.json、install.lock、jwt.secret、baota.env
  plugins、home-templates、software-source-cache
  updates、app-releases、logs、advertisement-images、source-packages

替换前先把这些文件拷到 backend/updates/backups/baota-upgrade-<时间>-<pid>/ 。
db.json 能连上 MySQL 时，还会用 mysqldump 导出 db.sql（口令只放在环境变量里）。

不要先把新压缩包直接解压覆盖正在运行的站点。把包留在 /tmp，用 --package 指向它。
请运行新版本包里的本脚本，不要用站点上旧的 baota-upgrade.sh。

进程若已由宝塔进程守护或 systemd 托管：加上 --start。脚本不会去停守护，
而是替换文件后只结束本站进程，由守护按 start.sh 拉起。--no-start 在这种情况下会拒绝执行，
请先在面板里停止守护。

进程没有被守护托管时：脚本会先停止本站进程；端口上如果还有本站 auth_pro
（包括父进程为 1 的孤儿）会先 SIGTERM，超时后 SIGKILL。确认端口空闲后才替换并启动。
占用者不是本站程序时会拒绝执行，不会误杀同机其它站点。

选项：
  --site-root DIR   正在运行的网站根
  --package FILE    新版本 tar.gz
  --source DIR      已解压的新版本目录（不要指向正在服务的网站根）
  --port PORT       覆盖 baota.env 里的端口，默认 19127
  --start           升级后启动。找到本站进程守护时由守护启动，健康检查失败会回滚
  --no-start        只替换文件。替换前仍会先停本站旧进程
  --stop-port       兼容旧参数。现在默认就会先停本站进程
  --skip-mysql      不导出数据库（仍会备份 db.json 文件）
  --yes, -y         不再询问
  --dry-run         只打印步骤，不改文件、不导出数据库
  -h, --help        显示本说明

环境变量与安装脚本相同，另有 AUTH_PRO_SKIP_MYSQL=1。

示例：
  bash baota-upgrade.sh \
    --site-root /www/wwwroot/example.com \
    --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \
    --no-start
EOF
}

BAOTA_ACTION="upgrade"
baota_cmd_upgrade "$@"
