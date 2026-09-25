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

把发布包装进宝塔网站根目录，适合空站点。数据库口令不要写进本脚本，
首次用浏览器打开站点，在安装向导里填写事先建好的空 MySQL。
若已有 install.lock，脚本会拒绝执行，请改用 baota-upgrade.sh。

选项：
  --site-root DIR   网站根，例如 /www/wwwroot/example.com
  --package FILE    发布包 auth_pro-full-vX.Y.Z.tar.gz
  --source DIR      已解压的发布目录
  --port PORT       后端端口，默认 19127（已有 baota.env 时沿用其中的 PORT）
  --start           安装后在端口空闲时启动。已配置本站进程守护时由守护启动
  --no-start        只放好文件。覆盖安装仍会先停本站旧进程
  --stop-port       兼容旧参数。现在安装和升级都会先停本站进程，不必再加
  --yes, -y         不再询问
  --dry-run         只打印步骤，不改网站文件
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
EOF
}

BAOTA_ACTION="install"
baota_cmd_install "$@"
