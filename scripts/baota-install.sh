#!/usr/bin/env bash
# 宝塔面板：把 auth-pro 发布包装到一个尚未安装的网站根目录。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=baota-lib.sh
source "$SCRIPT_DIR/baota-lib.sh"

baota_print_help() {
  cat <<'EOF'
用法：
  bash baota-install.sh [选项]

把发布包装进宝塔网站根目录。检测到宝塔面板时会自动建站、建库、反代并完成安装向导。
没有面板时只放文件，并说明还要在面板里做的步骤。脚本不会替你安装 MySQL。
若已有 install.lock，脚本会拒绝执行，请改用 baota-upgrade.sh。

选项：
  --site-root DIR   网站根，例如 /www/wwwroot/example.com
  --package FILE    发布包 auth_pro-full-vX.Y.Z.tar.gz
  --source DIR      已解压的发布目录
  --port PORT       后端端口。不写则从 19127 起找空闲端口（已有 baota.env 时沿用其中的 PORT）
  --start           安装后在端口空闲时启动。已配置本站进程守护时由守护启动
  --no-start        只放好文件。覆盖安装仍会先停本站旧进程
  --stop-port       兼容旧参数。现在安装和升级都会先停本站进程，不必再加
  --yes, -y         不再询问
  --dry-run         只打印步骤，不改网站文件
  --repair-guardian 只修复本站点的进程守护。不停其它站点，不改数据库、网站文件和 Nginx
  --reset-admin-password
                    本机 root 重设已装站点的管理员密码，并打印新的 8 位数字密码。不改网站文件、Nginx 和数据库密码
  --reset-binary FILE
                    带 reset-admin-password 子命令的 auth_pro。不写则用网站目录里的那一份
  -h, --help        显示本说明

环境变量：
  AUTH_PRO_SITE_ROOT、AUTH_PRO_PACKAGE、AUTH_PRO_SOURCE、AUTH_PRO_PORT
  AUTH_PRO_START=0|1、AUTH_PRO_YES=1、AUTH_PRO_STOP_PORT=1、AUTH_PRO_DRY_RUN=1
  AUTH_PRO_DATA_DIR（默认是网站根下的 backend）
  AUTH_PRO_HOST（默认 127.0.0.1，由 Nginx 反代）

示例：
  cd /www/wwwroot/example.com
  tar -xzf auth_pro-full-vX.Y.Z.tar.gz
  bash baota-install.sh

  AUTH_PRO_YES=1 AUTH_PRO_START=0 \
  bash baota-install.sh \
    --site-root /www/wwwroot/example.com \
    --package /tmp/auth_pro-full-vX.Y.Z.tar.gz

  bash baota-install.sh --repair-guardian --yes \
    --site-root /www/wwwroot/example.com

  bash baota-install.sh --reset-admin-password --yes \
    --site-root /www/wwwroot/example.com
EOF
}

BAOTA_ACTION="install"
baota_cmd_install "$@"
