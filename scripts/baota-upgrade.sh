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
升级前先在宝塔「进程守护」里停止本站点，否则结束残留进程会被立刻拉起。

选项：
  --site-root DIR   正在运行的网站根
  --package FILE    新版本 tar.gz
  --source DIR      已解压的新版本目录（不要指向正在服务的网站根）
  --port PORT       覆盖 baota.env 里的端口，默认 19127
  --start           升级后立即后台启动
  --no-start        只替换文件，启动交给进程守护
  --stop-port       结束本站 backend/auth_pro 占用的端口后再替换
  --skip-mysql      不导出数据库（仍会备份 db.json 文件）
  --yes, -y         不再询问
  --dry-run         只打印步骤，不改文件、不导出数据库
  -h, --help        显示本说明

环境变量与安装脚本相同，另有 AUTH_PRO_SKIP_MYSQL=1。

示例：
  bash baota-upgrade.sh \
    --site-root /www/wwwroot/example.com \
    --package /tmp/auth_pro-full-vX.Y.Z.tar.gz \
    --stop-port --no-start
EOF
}

BAOTA_ACTION="upgrade"
baota_cmd_upgrade "$@"
