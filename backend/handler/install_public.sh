#!/usr/bin/env bash
# 客户唯一入口。安装、升级、修复进程守护、重设管理员密码都走官网下发的这一份。
# 发布包里没有本文件。面板辅助脚本和启动模板从官网安装包里取。官网地址写死，不能用环境变量改掉。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

install_die() {
  printf '[错误] %s\n' "$1" >&2
  exit 1
}

install_info() {
  printf '[信息] %s\n' "$1"
}

install_sha256() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return 0
  fi
  install_die "缺少 sha256sum，无法核对安装包"
}

# 带域名、又没有 --package / --source 时，从官网下载安装包，再用当前这份脚本继续。
# 已经拿着 --package / --source，或只写了本机 --site-root，就直接安装或升级。
install_should_download() {
  local arg prev="" domain=""
  for arg in "$@"; do
    case "$arg" in
      --package|--source|--dry-run|--reset-binary|-h|--help)
        return 1
        ;;
    esac
  done
  for arg in "$@"; do
    case "$prev" in
      --site-root|--port|--package|--source|--reset-binary)
        prev=""
        continue
        ;;
    esac
    case "$arg" in
      --site-root|--port|--package|--source|--reset-binary)
        prev="$arg"
        ;;
      -*)
        ;;
      *)
        domain="$arg"
        ;;
    esac
  done
  [[ -n "$domain" ]]
}

# 官网下载的临时目录。退出时由 baota_cleanup 删掉，一条命令中途失败也不会留下。
INSTALL_DOWNLOAD_DIR=""

# 发布包成员名只认根上的普通路径，不认子目录里的同名文件。
install_package_has() {
  local listing="$1" name="$2"
  printf '%s\n' "$listing" | grep -qx "${name}" && return 0
  printf '%s\n' "$listing" | grep -qx "./${name}"
}

install_extract_member() {
  local archive="$1" dest="$2" listing="$3" name="$4"
  if printf '%s\n' "$listing" | grep -qx "./${name}"; then
    tar -xzf "$archive" -C "$dest" "./${name}"
    return 0
  fi
  if printf '%s\n' "$listing" | grep -qx "${name}"; then
    tar -xzf "$archive" -C "$dest" "${name}"
    return 0
  fi
  install_die "安装包里缺少 ${name}"
}

# 下载并核对官网安装包，然后用当前这份脚本安装或升级。
# 不执行包里的 install.sh。baota-panel.py 和 guardian-start.sh 从包里取出。
install_download_and_continue() {
  local action="$1"
  shift
  local DOMAIN="" SITE_ROOT="" PORT="" START_FLAG="--start" REPAIR_GUARDIAN=0 RESET_ADMIN=0 SKIP_MYSQL=0
  case "$action" in
    repair) REPAIR_GUARDIAN=1 ;;
    reset-admin-password) RESET_ADMIN=1 ;;
  esac
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)
        baota_print_help
        exit 0
        ;;
      --repair-guardian)
        REPAIR_GUARDIAN=1
        shift
        ;;
      --reset-admin-password)
        RESET_ADMIN=1
        shift
        ;;
      --site-root)
        [[ $# -ge 2 ]] || install_die "--site-root 需要网站根目录"
        SITE_ROOT="$2"
        shift 2
        ;;
      --port)
        [[ $# -ge 2 ]] || install_die "--port 需要端口号"
        PORT="$2"
        shift 2
        ;;
      --start)
        START_FLAG="--start"
        shift
        ;;
      --no-start)
        START_FLAG="--no-start"
        shift
        ;;
      --skip-mysql)
        SKIP_MYSQL=1
        shift
        ;;
      --)
        shift
        break
        ;;
      -*)
        install_die "未知参数：$1（可用 --help 查看）"
        ;;
      *)
        [[ -z "$DOMAIN" ]] || install_die "只能写一个域名"
        DOMAIN="$1"
        shift
        ;;
    esac
  done
  if [[ $# -gt 0 ]]; then
    install_die "只能写一个域名"
  fi
  if [[ -z "$DOMAIN" && -z "$SITE_ROOT" ]]; then
    baota_print_help >&2
    install_die "请在命令末尾写上域名，例如：bash -s -- example.com"
  fi
  if [[ -n "$DOMAIN" ]]; then
    if [[ ! "$DOMAIN" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}$ ]]; then
      install_die "域名不正确：$DOMAIN"
    fi
    DOMAIN="$(printf '%s' "$DOMAIN" | tr '[:upper:]' '[:lower:]')"
  fi
  if [[ -z "$SITE_ROOT" ]]; then
    SITE_ROOT="/www/wwwroot/${DOMAIN}"
  fi
  SITE_ROOT="${SITE_ROOT%/}"
  [[ "$SITE_ROOT" == /* ]] || install_die "网站根目录必须是绝对路径：$SITE_ROOT"
  if [[ -n "$PORT" ]]; then
    [[ "$PORT" =~ ^[0-9]+$ ]] || install_die "端口必须是数字：$PORT"
    if [[ "$PORT" -lt 1 || "$PORT" -gt 65535 ]]; then
      install_die "端口超出范围：$PORT"
    fi
  fi
  if [[ "$REPAIR_GUARDIAN" == "1" && "$RESET_ADMIN" == "1" ]]; then
    install_die "请分开执行 --repair-guardian 和 --reset-admin-password"
  fi
  if [[ "$RESET_ADMIN" == "1" && "$(id -u)" -ne 0 ]]; then
    install_die "只有 root 能在服务器本机重设管理员密码"
  fi
  if [[ "$action" != "upgrade" && -f "$SITE_ROOT/backend/install.lock" && "$REPAIR_GUARDIAN" != "1" && "$RESET_ADMIN" != "1" ]]; then
    install_die "检测到 ${SITE_ROOT}/backend/install.lock ，站点已经安装。请改用 curl -fsSL https://auth.maizll.com/install.sh | bash -s -- upgrade ${DOMAIN} ，或在后台使用「在线更新」，以免覆盖运行数据。若只是进程没进宝塔进程守护，请改用 --repair-guardian。若要重设管理员密码，请改用 --reset-admin-password。"
  fi
  if [[ "$REPAIR_GUARDIAN" == "1" && ! -f "$SITE_ROOT/backend/install.lock" ]]; then
    install_die "没有 ${SITE_ROOT}/backend/install.lock 。修复命令只处理已经装好的站点，请去掉 --repair-guardian 再安装。"
  fi
  if [[ "$RESET_ADMIN" == "1" && ! -f "$SITE_ROOT/backend/install.lock" ]]; then
    install_die "没有 ${SITE_ROOT}/backend/install.lock 。重设密码只处理已经装好的站点，请去掉 --reset-admin-password 再安装。"
  fi

  command -v curl >/dev/null 2>&1 || install_die "缺少 curl，无法下载安装包"
  command -v python3 >/dev/null 2>&1 || install_die "缺少 python3，无法读取版本清单"
  command -v tar >/dev/null 2>&1 || install_die "缺少 tar，无法解开安装包"

  local MANIFEST PARSED VERSION SHA SIGNATURE SIZE PKG_URL PKG_NAME EXPECT_URL WORKDIR PKG_FILE
  local ACTUAL_SIZE ACTUAL_SHA LISTING HELPERS ARGS
  install_info "正在从官网获取最新版本"
  if ! MANIFEST="$(curl -q -fsSL --proto '=https' --max-time 60 "https://auth.maizll.com/api/v1/update/latest.json")"; then
    install_die "无法从官网获取最新版本"
  fi
  if ! PARSED="$(printf '%s' "$MANIFEST" | python3 -c '
import json
import sys
data = json.load(sys.stdin)
pkg = data.get("package") or {}

def clean(value):
    text = "" if value is None else str(value)
    if "\n" in text or "\r" in text:
        raise SystemExit(2)
    return text

version = clean(data.get("version")).lstrip("v")
sha = clean(pkg.get("sha256") or data.get("sha256")).lower()
sig = clean(pkg.get("signature"))
url = clean(pkg.get("url") or data.get("url"))
name = clean(pkg.get("fileName") or "")
size = pkg.get("size")
if size is None:
    size = data.get("size") or 0
try:
    size = int(size)
except (TypeError, ValueError):
    size = 0
sys.stdout.write("\n".join([version, sha, sig, str(size), url, name]) + "\n")
')"; then
    install_die "无法读取官网版本清单"
  fi
  local FIELDS
  mapfile -t FIELDS <<< "$PARSED"
  if [[ "${#FIELDS[@]}" -lt 6 ]]; then
    install_die "无法读取官网版本清单"
  fi
  VERSION="${FIELDS[0]}"
  SHA="${FIELDS[1]}"
  SIGNATURE="${FIELDS[2]}"
  SIZE="${FIELDS[3]}"
  PKG_URL="${FIELDS[4]}"
  PKG_NAME="${FIELDS[5]}"
  [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || install_die "版本号不正确"
  [[ "$SHA" =~ ^[a-f0-9]{64}$ ]] || install_die "清单里的 SHA256 不正确"
  [[ "$SIGNATURE" == "sha256:${SHA}" ]] || install_die "安装包签名与 SHA256 不一致"
  [[ "$PKG_NAME" == "auth_pro-full-v${VERSION}.tar.gz" ]] || install_die "安装包文件名与版本不一致"
  EXPECT_URL="https://auth.maizll.com/api/v1/update/package/${VERSION}"
  [[ "$PKG_URL" == "$EXPECT_URL" ]] || install_die "清单里的下载地址不是官网更新接口"
  [[ "$SIZE" =~ ^[0-9]+$ ]] || install_die "清单里的安装包大小不正确"
  if [[ "$SIZE" -gt 536870912 ]]; then
    install_die "安装包超过 512MB 上限，已停止安装"
  fi

  WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/auth-pro-install.XXXXXX")"
  INSTALL_DOWNLOAD_DIR="$WORKDIR"
  PKG_FILE="$WORKDIR/$PKG_NAME"
  install_info "最新版本 ${VERSION}，开始下载安装包"
  if ! curl -q -fsSL --proto '=https' --retry 2 --retry-delay 1 --max-time 600 -o "$PKG_FILE" "$EXPECT_URL"; then
    install_die "安装包下载失败，已停止安装"
  fi
  ACTUAL_SIZE="$(stat -c '%s' "$PKG_FILE" 2>/dev/null || stat -f '%z' "$PKG_FILE")"
  if [[ "$ACTUAL_SIZE" -gt 536870912 ]]; then
    install_die "安装包超过 512MB 上限，已停止安装"
  fi
  if [[ "$SIZE" -gt 0 && "$ACTUAL_SIZE" != "$SIZE" ]]; then
    install_die "安装包大小与清单不一致，已停止安装"
  fi
  install_info "正在核对 SHA256 和签名"
  ACTUAL_SHA="$(install_sha256 "$PKG_FILE")"
  [[ "$ACTUAL_SHA" == "$SHA" ]] || install_die "安装包 SHA256 不一致，已停止安装"

  LISTING="$(tar -tzf "$PKG_FILE")"
  install_package_has "$LISTING" "baota-panel.py" || install_die "安装包里缺少 baota-panel.py"
  install_package_has "$LISTING" "guardian-start.sh" || install_die "安装包里缺少 guardian-start.sh"
  install_package_has "$LISTING" "backend/auth_pro" || install_die "安装包里缺少 backend/auth_pro"
  # 发布包不带安装脚本。这里只取出面板辅助脚本和启动模板，不执行包内的 install.sh。
  HELPERS="$WORKDIR/helpers"
  mkdir -p "$HELPERS"
  install_extract_member "$PKG_FILE" "$HELPERS" "$LISTING" "baota-panel.py"
  install_extract_member "$PKG_FILE" "$HELPERS" "$LISTING" "guardian-start.sh"
  chmod 755 "$HELPERS/guardian-start.sh"
  BAOTA_PAYLOAD="$HELPERS"

  if [[ -n "$DOMAIN" ]]; then
    export AUTH_PRO_PUBLIC_HOST="$DOMAIN"
  fi
  if [[ "$RESET_ADMIN" == "1" ]]; then
    install_extract_member "$PKG_FILE" "$HELPERS" "$LISTING" "backend/auth_pro"
    [[ -f "$HELPERS/backend/auth_pro" ]] || install_die "安装包里缺少 backend/auth_pro，无法重设管理员密码"
    chmod 755 "$HELPERS/backend/auth_pro"
    install_info "开始重设 ${SITE_ROOT} 的管理员密码，不覆盖网站文件和 Nginx"
    baota_cmd_install --reset-admin-password --yes --site-root "$SITE_ROOT" --reset-binary "$HELPERS/backend/auth_pro"
    return 0
  fi
  if [[ "$REPAIR_GUARDIAN" == "1" ]]; then
    install_info "开始修复 ${SITE_ROOT} 的进程守护，不覆盖网站文件和数据库"
    baota_cmd_install --repair-guardian --yes --site-root "$SITE_ROOT"
    return 0
  fi
  ARGS=(--yes --site-root "$SITE_ROOT" --package "$PKG_FILE" "$START_FLAG")
  if [[ -n "$PORT" ]]; then
    ARGS+=(--port "$PORT")
  fi
  if [[ "$SKIP_MYSQL" == "1" ]]; then
    ARGS+=(--skip-mysql)
  fi
  if [[ "$action" == "upgrade" ]]; then
    install_info "开始升级 ${SITE_ROOT}"
    BAOTA_ACTION="upgrade"
    baota_cmd_upgrade "${ARGS[@]}"
    return 0
  fi
  export AUTH_PRO_ONECLICK=1
  install_info "开始安装到 ${SITE_ROOT}"
  baota_cmd_install "${ARGS[@]}"
}

baota_print_help() {
  cat <<'EOF'
用法：
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- 域名
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- upgrade 域名
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- --repair-guardian 域名
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- --reset-admin-password 域名

这是安装、升级、修复进程守护和重设管理员密码的唯一入口。脚本只从官网下载，不在发布包里。
一条命令会从官网下载已发布的安装包，核对 SHA256 和签名后再安装或升级。
需要本机已经装好宝塔面板，并且软件商店里已经安装 Nginx 和 MySQL。脚本不会替你安装 MySQL。
网站目录默认是 /www/wwwroot/域名。未写 --port 时从 19127 起自动找空闲端口。
已经有 backend/install.lock 时，安装会停下来。请改用 upgrade，或在后台使用「在线更新」。

upgrade 从官网取最新包，替换页面和 backend/auth_pro，保留 db.json、install.lock 和运行数据。
不要把新压缩包直接解压覆盖正在运行的站点。
进程已由宝塔进程守护或 systemd 托管时，upgrade 默认在替换后只结束本站进程，由守护拉起。
--no-start 在守护仍托管时会拒绝。--skip-mysql 不导出数据库，仍会备份 db.json。
已经把发布包放在本机时，可以加上 --package，不再从官网下载。

选项：
  --site-root DIR   网站根
  --package FILE    发布包 auth_pro-full-vX.Y.Z.tar.gz
  --source DIR      已解压的发布目录
  --port PORT       后端端口。不写则从 19127 起找空闲端口（已有 baota.env 时沿用其中的 PORT）
  --start           安装或升级后启动。已配置本站进程守护时由守护启动
  --no-start        只放好文件。覆盖安装和升级仍会先停本站旧进程
  --stop-port       兼容旧参数。现在安装和升级都会先停本站进程
  --skip-mysql      升级时不导出数据库（仍会备份 db.json）
  --yes, -y         不再询问
  --dry-run         只打印步骤，不改网站文件
  --repair-guardian 只修复本站点的进程守护。不停其它站点，不改数据库、网站文件和 Nginx
  --reset-admin-password
                    本机 root 重设已装站点的管理员密码，并打印新的 8 位数字密码
  --reset-binary FILE
                    带 reset-admin-password 子命令的 auth_pro。不写则用网站目录里的那一份
  -h, --help        显示本说明

示例：
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com --port 19127 --site-root /www/wwwroot/example.com
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- upgrade example.com
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- --repair-guardian example.com
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- --reset-admin-password example.com
  bash install.sh --yes --site-root /www/wwwroot/example.com --package /tmp/auth_pro-full-vX.Y.Z.tar.gz
  bash install.sh upgrade --yes --site-root /www/wwwroot/example.com --package /tmp/auth_pro-full-vX.Y.Z.tar.gz --no-start
EOF
}

# 安装、升级、修复和重设密码的实现。只由同文件前面的 install.sh 入口调用。
: "${SCRIPT_DIR:?SCRIPT_DIR 未设置}"

# 这些路径相对数据目录（默认是网站根下的 backend/）。
# 与后端 getDataDir() 一致：db.json、install.lock、jwt.secret，
# 以及插件、模板、更新包、日志等运行期目录。升级不得删除它们。
BAOTA_DURABLE_FILES=(
  db.json
  install.lock
  jwt.secret
  baota.env
  auto_pro.log
  auto_pro.pid
)
BAOTA_DURABLE_DIRS=(
  plugins
  home-templates
  software-source-cache
  updates
  app-releases
  logs
  advertisement-images
  source-packages
)

BAOTA_ACTION="${BAOTA_ACTION:-}"
BAOTA_DRY_RUN=0
BAOTA_YES=0
BAOTA_START=""
BAOTA_STOP_PORT=0
BAOTA_REPAIR_GUARDIAN=0
BAOTA_RESET_ADMIN=0
BAOTA_RESET_BINARY=""
BAOTA_SKIP_MYSQL=0
BAOTA_PORT_SET=0
BAOTA_SITE_ROOT="${AUTH_PRO_SITE_ROOT:-}"
BAOTA_PACKAGE="${AUTH_PRO_PACKAGE:-}"
BAOTA_SOURCE="${AUTH_PRO_SOURCE:-}"
BAOTA_PORT="${AUTH_PRO_PORT:-}"
BAOTA_PAYLOAD=""
BAOTA_BACKUP_DIR=""
BAOTA_DATA_DIR_RESOLVED=""
BAOTA_TMP_DIRS=()
BAOTA_STAGING_ASSETS=""

if [[ "${AUTH_PRO_DRY_RUN:-}" == "1" ]]; then
  BAOTA_DRY_RUN=1
fi
if [[ "${AUTH_PRO_YES:-}" == "1" ]]; then
  BAOTA_YES=1
fi
if [[ "${AUTH_PRO_STOP_PORT:-}" == "1" ]]; then
  BAOTA_STOP_PORT=1
fi
if [[ "${AUTH_PRO_SKIP_MYSQL:-}" == "1" ]]; then
  BAOTA_SKIP_MYSQL=1
fi
if [[ -n "${AUTH_PRO_START:-}" ]]; then
  BAOTA_START="${AUTH_PRO_START}"
fi
if [[ -n "${AUTH_PRO_PORT:-}" ]]; then
  BAOTA_PORT_SET=1
fi

baota_info() { printf '[信息] %s\n' "$1"; }
baota_warn() { printf '[注意] %s\n' "$1"; }
baota_die() { printf '[错误] %s\n' "$1" >&2; exit 1; }

baota_cleanup() {
  local dir
  if [[ -n "$BAOTA_STAGING_ASSETS" && -d "$BAOTA_STAGING_ASSETS" ]]; then
    rm -rf "$BAOTA_STAGING_ASSETS"
  fi
  if [[ ${#BAOTA_TMP_DIRS[@]} -gt 0 ]]; then
    for dir in "${BAOTA_TMP_DIRS[@]}"; do
      rm -rf "$dir"
    done
  fi
  # 官网下载的临时目录。一条命令失败退出时也要删掉。
  if [[ -n "${INSTALL_DOWNLOAD_DIR:-}" && -d "$INSTALL_DOWNLOAD_DIR" ]]; then
    rm -rf "$INSTALL_DOWNLOAD_DIR"
  fi
}
trap baota_cleanup EXIT

baota_parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)
        baota_print_help
        exit 0
        ;;
      --dry-run)
        BAOTA_DRY_RUN=1
        shift
        ;;
      --yes|-y)
        BAOTA_YES=1
        shift
        ;;
      --start)
        BAOTA_START=1
        shift
        ;;
      --no-start)
        BAOTA_START=0
        shift
        ;;
      --stop-port)
        BAOTA_STOP_PORT=1
        shift
        ;;
      --repair-guardian)
        BAOTA_REPAIR_GUARDIAN=1
        shift
        ;;
      --reset-admin-password)
        BAOTA_RESET_ADMIN=1
        shift
        ;;
      --reset-binary)
        [[ $# -ge 2 ]] || baota_die "--reset-binary 需要 auth_pro 路径"
        BAOTA_RESET_BINARY="$2"
        shift 2
        ;;
      --skip-mysql)
        BAOTA_SKIP_MYSQL=1
        shift
        ;;
      --site-root)
        [[ $# -ge 2 ]] || baota_die "--site-root 需要网站根目录"
        BAOTA_SITE_ROOT="$2"
        shift 2
        ;;
      --package)
        [[ $# -ge 2 ]] || baota_die "--package 需要 tar.gz 路径"
        BAOTA_PACKAGE="$2"
        shift 2
        ;;
      --source)
        [[ $# -ge 2 ]] || baota_die "--source 需要已解压的发布目录"
        BAOTA_SOURCE="$2"
        shift 2
        ;;
      --port)
        [[ $# -ge 2 ]] || baota_die "--port 需要端口号"
        BAOTA_PORT="$2"
        BAOTA_PORT_SET=1
        shift 2
        ;;
      *)
        baota_die "未知参数：$1（可用 --help 查看）"
        ;;
    esac
  done
}

baota_assert_port_number() {
  local port="$1"
  [[ "$port" =~ ^[0-9]+$ ]] || baota_die "端口必须是数字：$port"
  if [[ "$port" -lt 1 || "$port" -gt 65535 ]]; then
    baota_die "端口超出范围：$port"
  fi
}

baota_denied_root() {
  local path="$1"
  case "$path" in
    /|/usr|/bin|/sbin|/lib|/lib64|/etc|/boot|/dev|/proc|/sys|/root|/home|/opt|/var|/tmp|/www|/www/wwwroot|/usr/bin|/usr/local|/usr/local/bin|/var/www)
      return 0
      ;;
    /usr/*|/bin/*|/sbin/*|/lib/*|/lib64/*|/etc/*|/boot/*|/dev/*|/proc/*|/sys/*)
      return 0
      ;;
  esac
  return 1
}

baota_assert_safe_dir() {
  local path="$1" label="$2"
  [[ "$path" == /* ]] || baota_die "${label}必须是绝对路径：$path"
  if baota_denied_root "$path"; then
    baota_die "拒绝把系统目录当作${label}：$path"
  fi
}

baota_resolve_site_root() {
  if [[ -z "$BAOTA_SITE_ROOT" ]]; then
    if [[ -f "$SCRIPT_DIR/index.html" && -f "$SCRIPT_DIR/backend/auth_pro" ]]; then
      BAOTA_SITE_ROOT="$SCRIPT_DIR"
      baota_info "使用脚本所在目录作为网站根：$BAOTA_SITE_ROOT"
    elif [[ "$BAOTA_YES" == "1" || ! -t 0 ]]; then
      baota_die "请指定网站根目录：--site-root /www/wwwroot/你的域名 ，或设置 AUTH_PRO_SITE_ROOT"
    else
      local answer=""
      read -r -p "网站根目录（例如 /www/wwwroot/example.com）： " answer
      BAOTA_SITE_ROOT="$answer"
    fi
  fi
  BAOTA_SITE_ROOT="${BAOTA_SITE_ROOT%/}"
  [[ -n "$BAOTA_SITE_ROOT" ]] || baota_die "网站根目录不能为空"
  baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
  baota_reject_dotdot "$BAOTA_SITE_ROOT" "网站根目录"
  if [[ ! -d "$BAOTA_SITE_ROOT" ]]; then
    if [[ "$BAOTA_ACTION" != "install" ]]; then
      baota_die "网站根目录不存在：$BAOTA_SITE_ROOT 。请先在宝塔创建网站。"
    fi
    if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
      baota_info "将创建网站目录：$BAOTA_SITE_ROOT"
      return 0
    fi
    mkdir -p -- "$BAOTA_SITE_ROOT" || baota_die "无法创建网站目录：$BAOTA_SITE_ROOT"
    baota_info "已创建网站目录：$BAOTA_SITE_ROOT"
  fi
  BAOTA_SITE_ROOT="$(cd "$BAOTA_SITE_ROOT" && pwd -P)"
  baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
}

baota_reject_dotdot() {
  local path="$1" label="$2" part
  local -a parts=()
  IFS='/' read -r -a parts <<< "$path"
  for part in "${parts[@]}"; do
    if [[ "$part" == ".." ]]; then
      baota_die "${label}不能包含 .. ：$path"
    fi
  done
}

baota_data_dir() {
  if [[ -n "$BAOTA_DATA_DIR_RESOLVED" ]]; then
    printf '%s\n' "$BAOTA_DATA_DIR_RESOLVED"
    return 0
  fi
  local dir=""
  if [[ -n "${AUTH_PRO_DATA_DIR:-}" ]]; then
    dir="${AUTH_PRO_DATA_DIR%/}"
  elif [[ -f "$BAOTA_SITE_ROOT/backend/baota.env" ]]; then
    dir="$(sed -n 's/^AUTO_PRO_DATA_DIR=//p' "$BAOTA_SITE_ROOT/backend/baota.env" | head -n 1)"
    dir="${dir%/}"
  fi
  if [[ -z "$dir" ]]; then
    dir="$BAOTA_SITE_ROOT/backend"
  fi
  baota_assert_safe_dir "$dir" "数据目录"
  BAOTA_DATA_DIR_RESOLVED="$dir"
  printf '%s\n' "$dir"
}

baota_effective_port() {
  local data port=""
  data="$(baota_data_dir)"
  if [[ "$BAOTA_PORT_SET" == "1" ]]; then
    port="$BAOTA_PORT"
  elif [[ -f "$data/baota.env" ]]; then
    port="$(sed -n 's/^PORT=//p' "$data/baota.env" | head -n 1)"
  fi
  if [[ -z "$port" ]]; then
    port="${BAOTA_PORT:-19127}"
  fi
  baota_assert_port_number "$port"
  printf '%s\n' "$port"
}

baota_resolve_start_flag() {
  if [[ -n "$BAOTA_START" ]]; then
    case "$BAOTA_START" in
      1|true|yes|TRUE|YES) BAOTA_START=1 ;;
      0|false|no|FALSE|NO) BAOTA_START=0 ;;
      *) baota_die "是否启动只能是 0 或 1（--start / --no-start / AUTH_PRO_START）" ;;
    esac
    return 0
  fi
  if [[ "$BAOTA_DRY_RUN" == "1" || "$BAOTA_YES" == "1" || ! -t 0 ]]; then
    BAOTA_START=0
    return 0
  fi
  local answer=""
  read -r -p "是否由脚本立即后台启动？若准备用宝塔进程守护，请输入 n。[Y/n] " answer
  case "$answer" in
    n|N|no|NO) BAOTA_START=0 ;;
    *) BAOTA_START=1 ;;
  esac
}

baota_tar_entry_safe() {
  local name="$1"
  name="${name//$'\r'/}"
  while [[ "$name" == ./* ]]; do
    name="${name#./}"
  done
  [[ -n "$name" && "$name" != "." ]] || return 0
  case "$name" in
    /*|..|../*|*/..|*/../*|*\\*)
      baota_die "压缩包包含不安全路径：$1"
      ;;
  esac
}

baota_assert_tar_safe() {
  local pkg="$1" name
  # 用 Python 读成员名。tar 在非 UTF-8 语言环境下会把中文文件名转义成带反斜杠的八进制，
  # 误伤正常的前端资源。拒绝规则仍是绝对路径、.. 和反斜杠。
  while IFS= read -r name; do
    baota_tar_entry_safe "$name"
  done < <(python3 -c '
import sys
import tarfile
with tarfile.open(sys.argv[1], "r:gz") as tar:
    for name in tar.getnames():
        sys.stdout.write(name.replace("\r", "") + "\n")
' "$pkg")
}

baota_assert_payload() {
  local dir="$1" link
  [[ -d "$dir" ]] || baota_die "发布目录不存在：$dir"
  [[ -f "$dir/index.html" ]] || baota_die "发布包缺少 index.html"
  [[ -f "$dir/version.json" ]] || baota_die "发布包缺少 version.json"
  [[ -f "$dir/manifest.json" ]] || baota_die "发布包缺少 manifest.json"
  [[ -f "$dir/backend/auth_pro" ]] || baota_die "发布包缺少 backend/auth_pro"
  [[ -d "$dir/assets" ]] || baota_die "发布包缺少 assets 目录"
  if [[ -L "$dir/index.html" || -L "$dir/backend/auth_pro" || -L "$dir/assets" ]]; then
    baota_die "发布包包含符号链接，已拒绝"
  fi
  link="$(find "$dir" -type l -print -quit 2>/dev/null || true)"
  if [[ -n "$link" ]]; then
    baota_die "发布包包含符号链接，已拒绝：$link"
  fi
  if command -v python3 >/dev/null 2>&1; then
    local manifest_note=""
    manifest_note="$(python3 -c '
import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
frontend = str(data.get("frontendDir", "."))
backend = str(data.get("backendFile", "backend/auth_pro"))
if frontend != "." or backend != "backend/auth_pro":
    sys.stdout.write("manifest 与宝塔目录规范不一致：frontendDir=%s backendFile=%s" % (frontend, backend))
' "$dir/manifest.json")" || baota_die "manifest.json 不是合法 JSON"
    if [[ -n "$manifest_note" ]]; then
      baota_warn "$manifest_note"
    fi
  fi
}

baota_prepare_payload() {
  if [[ -n "$BAOTA_PACKAGE" && -n "$BAOTA_SOURCE" ]]; then
    baota_die "不能同时指定 --package 和 --source"
  fi
  if [[ -n "$BAOTA_PACKAGE" ]]; then
    [[ -f "$BAOTA_PACKAGE" ]] || baota_die "找不到压缩包：$BAOTA_PACKAGE"
    baota_info "检查压缩包路径：$BAOTA_PACKAGE"
    baota_assert_tar_safe "$BAOTA_PACKAGE"
    if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
      local probe
      probe="$(mktemp -d "${TMPDIR:-/tmp}/auth-pro-probe.XXXXXX")"
      BAOTA_TMP_DIRS+=("$probe")
      tar -xzf "$BAOTA_PACKAGE" -C "$probe"
      baota_normalize_payload_root "$probe"
      return 0
    fi
    local dest
    dest="$(mktemp -d "${TMPDIR:-/tmp}/auth-pro-pkg.XXXXXX")"
    BAOTA_TMP_DIRS+=("$dest")
    tar -xzf "$BAOTA_PACKAGE" -C "$dest"
    baota_normalize_payload_root "$dest"
    return 0
  fi
  if [[ -n "$BAOTA_SOURCE" ]]; then
    [[ -d "$BAOTA_SOURCE" ]] || baota_die "发布目录不存在：$BAOTA_SOURCE"
    BAOTA_PAYLOAD="$(cd "$BAOTA_SOURCE" && pwd -P)"
    baota_assert_payload "$BAOTA_PAYLOAD"
    return 0
  fi
  if [[ -f "$SCRIPT_DIR/index.html" && -f "$SCRIPT_DIR/backend/auth_pro" ]]; then
    BAOTA_PAYLOAD="$(cd "$SCRIPT_DIR" && pwd -P)"
    baota_assert_payload "$BAOTA_PAYLOAD"
    baota_info "使用脚本所在目录作为发布内容：$BAOTA_PAYLOAD"
    return 0
  fi
  baota_die "未找到发布包。请在解压后的网站根执行，或传入 --package / --source"
}

baota_normalize_payload_root() {
  local dest="$1" nested
  if [[ -f "$dest/index.html" ]]; then
    BAOTA_PAYLOAD="$dest"
    baota_assert_payload "$BAOTA_PAYLOAD"
    return 0
  fi
  nested="$(find "$dest" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  if [[ -n "$nested" && -f "$nested/index.html" ]]; then
    local count
    count="$(find "$dest" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')"
    if [[ "$count" == "1" ]]; then
      BAOTA_PAYLOAD="$(cd "$nested" && pwd -P)"
      baota_assert_payload "$BAOTA_PAYLOAD"
      return 0
    fi
  fi
  baota_die "压缩包根目录没有 index.html，也不是单层包装目录"
}

baota_same_payload() {
  [[ "$(cd "$BAOTA_PAYLOAD" && pwd -P)" == "$BAOTA_SITE_ROOT" ]]
}

baota_confirm() {
  local summary="$1"
  baota_info "$summary"
  if [[ "$BAOTA_DRY_RUN" == "1" || "$BAOTA_YES" == "1" || ! -t 0 ]]; then
    return 0
  fi
  local answer=""
  read -r -p "继续？[y/N] " answer
  case "$answer" in
    y|Y|yes|YES) ;;
    *) baota_die "已取消" ;;
  esac
}

baota_listen_inodes() {
  local port="$1" hex file line local_addr state inode port_hex
  printf -v hex '%04X' "$port"
  for file in /proc/net/tcp /proc/net/tcp6; do
    [[ -r "$file" ]] || continue
    set -f
    while read -r line; do
      set -- $line
      [[ "${1:-}" == "sl" ]] && continue
      [[ $# -ge 10 ]] || continue
      local_addr="$2"
      state="$4"
      inode="${10}"
      [[ "$state" == "0A" ]] || continue
      port_hex="${local_addr##*:}"
      port_hex="${port_hex^^}"
      [[ "$port_hex" == "$hex" ]] || continue
      printf '%s\n' "$inode"
    done < "$file"
  done | sort -u
}

baota_pids_for_port() {
  local port="$1" inode pid fd target known
  known="$(baota_listen_inodes "$port")"
  [[ -n "$known" ]] || return 0
  for pid_path in /proc/[0-9]*; do
    pid="${pid_path#/proc/}"
    for fd in "$pid_path"/fd/*; do
      target="$(readlink "$fd" 2>/dev/null || true)"
      case "$target" in
        socket:\[*\])
          inode="${target#socket:[}"
          inode="${inode%]}"
          if printf '%s\n' "$known" | grep -qx "$inode"; then
            printf '%s\n' "$pid"
            break
          fi
          ;;
      esac
    done
  done | sort -u
}

baota_port_is_open() {
  local port="$1"
  [[ -n "$(baota_listen_inodes "$port")" ]]
}

baota_cmd_is_ours() {
  local pid="$1" site="$2" exe cmd bin
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  [[ "$pid" -gt 1 ]] || return 1
  if [[ "$pid" == "$$" || "$pid" == "${PPID:-0}" ]]; then
    return 1
  fi
  bin="$site/backend/auth_pro"
  exe="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
  exe="${exe% (deleted)}"
  cmd="$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)"
  if [[ "$exe" == "$bin" || "$cmd" == "$bin" ]]; then
    return 0
  fi
  # 只匹配命令行里的完整路径，避免误伤其它站点的 auth_pro。
  if [[ " $cmd " == *" $bin "* || " $cmd " == *" $bin" ]]; then
    return 0
  fi
  return 1
}

baota_our_root() {
  local pid="$1" site="$2" current="$1" i=0 parent
  while [[ -n "$current" && "$current" != "1" && "$i" -lt 5 ]]; do
    if baota_cmd_is_ours "$current" "$site"; then
      printf '%s\n' "$current"
      return 0
    fi
    parent="$(awk '/^PPid:/ {print $2}' "/proc/$current/status" 2>/dev/null || true)"
    [[ -n "$parent" && "$parent" != "$current" ]] || return 1
    current="$parent"
    i=$((i + 1))
  done
  return 1
}

baota_signal_pid() {
  local pid="$1" wait_s="${AUTH_PRO_TERM_WAIT:-15}" i
  [[ "$pid" =~ ^[0-9]+$ ]] || return 0
  [[ "$pid" -gt 1 ]] || return 0
  kill -0 "$pid" 2>/dev/null || return 0
  kill -TERM "$pid" 2>/dev/null || true
  for ((i = 1; i <= wait_s; i++)); do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 1
  done
  baota_warn "PID ${pid} 在 ${wait_s} 秒内没有退出，发送 SIGKILL"
  kill -KILL "$pid" 2>/dev/null || true
  sleep 1
}

baota_signal_tree() {
  local pid="$1" child
  [[ "$pid" =~ ^[0-9]+$ ]] || return 0
  for child in $(ps -o pid= --ppid "$pid" 2>/dev/null || true); do
    baota_signal_tree "$child"
  done
  baota_signal_pid "$pid"
}

baota_env_get() {
  local key="$1" file
  file="$(baota_data_dir)/baota.env"
  [[ -f "$file" ]] || return 0
  sed -n "s/^${key}=//p" "$file" | head -n 1
}

baota_supervisor_command_is_ours() {
  local conf="$1" program="$2" site="$3" cmd
  [[ -f "$conf" && -n "$program" ]] || return 1
  cmd="$(awk -v p="[program:${program}]" '
    $0 == p { found = 1; next }
    found && /^\[/ { exit }
    found && /^command=/ { sub(/^command=/, ""); print; exit }
  ' "$conf")"
  case "$cmd" in
    "$site"/backend/start.sh|"$site"/backend/auth_pro) return 0 ;;
  esac
  return 1
}

# 插件把本站写在 profile/*.ini，主配置只有 include。只扫主配置里的 [program:] 会找不到，接着就会 nohup。
baota_supervisor_name_in_file() {
  local file="$1" site="$2"
  [[ -f "$file" ]] || return 0
  awk -v site="$site" '
    /^\[program:/ {
      name = $0
      sub(/^\[program:/, "", name)
      sub(/\]$/, "", name)
      next
    }
    /^command=/ && name != "" {
      cmd = substr($0, 9)
      gsub(/^[ \t]+|[ \t]+$/, "", cmd)
      if (cmd == site "/backend/start.sh" || cmd == site "/backend/auth_pro") {
        print name
        exit
      }
    }
    /^\[/ && $0 !~ /^\[program:/ { name = "" }
  ' "$file"
}

baota_supervisor_include_files() {
  local conf="$1" line glob match
  [[ -f "$conf" ]] || return 0
  line="$(awk '
    $0 == "[include]" { inside = 1; next }
    inside && /^\[/ { exit }
    inside && /^[ \t]*files[ \t]*=/ {
      sub(/^[ \t]*files[ \t]*=[ \t]*/, "")
      print
      exit
    }
  ' "$conf")"
  [[ -n "$line" ]] || return 0
  shopt -s nullglob
  for glob in $line; do
    for match in $glob; do
      printf '%s\n' "$match"
    done
  done
  shopt -u nullglob
}

# 只选用 command 指向本站 start.sh / auth_pro 的守护项，避免停掉同机其它站点。
baota_find_supervisor() {
  BAOTA_SUP_CONF=""
  BAOTA_SUP_PROGRAM=""
  local site conf program candidate file included
  site="$BAOTA_SITE_ROOT"
  conf="$(baota_env_get AUTO_PRO_SUPERVISOR_CONF)"
  program="$(baota_env_get AUTO_PRO_SUPERVISOR_PROGRAM)"
  [[ -n "$program" ]] || program="${AUTH_PRO_SUPERVISOR_PROGRAM:-}"
  if [[ -n "$conf" && -n "$program" ]] && baota_supervisor_command_is_ours "$conf" "$program" "$site"; then
    BAOTA_SUP_CONF="$conf"
    BAOTA_SUP_PROGRAM="$program"
    return 0
  fi
  if [[ -n "$conf" && -n "$program" ]]; then
    for file in \
      "/www/server/panel/plugin/supervisor/profile/${program}.ini" \
      "/etc/supervisor/auth-pro.d/${program}.ini"
    do
      if [[ "$(baota_supervisor_name_in_file "$file" "$site")" == "$program" ]]; then
        BAOTA_SUP_CONF="$conf"
        BAOTA_SUP_PROGRAM="$program"
        return 0
      fi
    done
  fi
  for candidate in /etc/supervisor/supervisord.conf /etc/supervisord.conf; do
    [[ -f "$candidate" ]] || continue
    program="$(baota_supervisor_name_in_file "$candidate" "$site")"
    if [[ -n "$program" ]]; then
      BAOTA_SUP_CONF="$candidate"
      BAOTA_SUP_PROGRAM="$program"
      return 0
    fi
    while IFS= read -r included; do
      [[ -n "$included" ]] || continue
      program="$(baota_supervisor_name_in_file "$included" "$site")"
      if [[ -n "$program" ]]; then
        BAOTA_SUP_CONF="$candidate"
        BAOTA_SUP_PROGRAM="$program"
        return 0
      fi
    done < <(baota_supervisor_include_files "$candidate")
  done
  # 主配置一度被清空时，include 行没了，但本站 ini 还在。ctl 仍指向面板这份主配置。
  for file in /www/server/panel/plugin/supervisor/profile/*.ini /etc/supervisor/auth-pro.d/*.ini; do
    [[ -f "$file" ]] || continue
    program="$(baota_supervisor_name_in_file "$file" "$site")"
    if [[ -n "$program" && -f /etc/supervisor/supervisord.conf ]]; then
      BAOTA_SUP_CONF="/etc/supervisor/supervisord.conf"
      BAOTA_SUP_PROGRAM="$program"
      return 0
    fi
  done
  return 1
}

# 插件的 ini 带 numprocs，进程名是 program_00，组名才是 program。启动和停止要用组名。
baota_supervisor_target() {
  local program="$1" ini
  for ini in \
    "/www/server/panel/plugin/supervisor/profile/${program}.ini" \
    "/etc/supervisor/auth-pro.d/${program}.ini"
  do
    if [[ -f "$ini" ]] && grep -q '^numprocs=' "$ini"; then
      printf '%s:\n' "$program"
      return 0
    fi
  done
  printf '%s\n' "$program"
}

baota_supervisorctl() {
  local ctl
  [[ -n "${BAOTA_SUP_CONF:-}" ]] || return 127
  if [[ -x /www/server/panel/pyenv/bin/supervisorctl ]]; then
    ctl=/www/server/panel/pyenv/bin/supervisorctl
  elif command -v supervisorctl >/dev/null 2>&1; then
    ctl="$(command -v supervisorctl)"
  else
    return 127
  fi
  "$ctl" -c "$BAOTA_SUP_CONF" "$@"
}

baota_supervisor_stop_ours() {
  baota_find_supervisor || return 0
  [[ -n "${BAOTA_SUP_PROGRAM:-}" ]] || return 0
  baota_supervisorctl stop "$(baota_supervisor_target "$BAOTA_SUP_PROGRAM")" >/dev/null 2>&1 || true
}

baota_supervisor_start_ours() {
  baota_find_supervisor || return 127
  [[ -n "${BAOTA_SUP_PROGRAM:-}" ]] || return 127
  baota_supervisorctl start "$(baota_supervisor_target "$BAOTA_SUP_PROGRAM")"
}

# 安装失败回滚时只卸下本站点的守护。ini 的 command 必须指向本站，其它站点的配置不动。
baota_drop_our_supervisor() {
  local program ini
  baota_find_supervisor || return 0
  program="${BAOTA_SUP_PROGRAM:-}"
  [[ -n "$program" ]] || return 0
  baota_supervisor_stop_ours
  for ini in \
    "/www/server/panel/plugin/supervisor/profile/${program}.ini" \
    "/etc/supervisor/auth-pro.d/${program}.ini"
  do
    [[ -f "$ini" ]] || continue
    if [[ "$(baota_supervisor_name_in_file "$ini" "$BAOTA_SITE_ROOT")" == "$program" ]]; then
      rm -f -- "$ini"
      baota_info "已卸下本站点的守护配置 ${ini}"
    fi
  done
  if [[ -n "${BAOTA_SUP_CONF:-}" ]]; then
    baota_supervisorctl update >/dev/null 2>&1 || true
  fi
}

# 先让进程守护停止，再清本站残留（含 PPID=1 的孤儿）。端口空闲才返回。
# 占用者不是本站 auth_pro 时直接失败，不杀进程、不替换文件。
baota_stop_and_reclaim() {
  local port="$1" site="$2"
  local i pid pids root exe cmd parent
  baota_find_supervisor || true
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    if baota_port_is_open "$port"; then
      baota_info "预演：将先停止本站进程守护，确认端口 ${port} 空闲后再替换文件"
    else
      baota_info "预演：端口 ${port} 空闲"
    fi
    return 0
  fi
  if [[ -n "${BAOTA_SUP_PROGRAM:-}" ]]; then
    baota_info "通过进程守护停止 ${BAOTA_SUP_PROGRAM}"
    baota_supervisor_stop_ours || baota_warn "进程守护停止 ${BAOTA_SUP_PROGRAM} 没有成功，继续检查端口 ${port} 上是不是本站残留进程"
  fi
  for i in 1 2 3; do
    baota_port_is_open "$port" || break
    sleep 1
  done
  if ! baota_port_is_open "$port"; then
    sleep 1
    if ! baota_port_is_open "$port"; then
      baota_info "端口 ${port} 已空闲"
      return 0
    fi
  fi
  pids="$(baota_pids_for_port "$port")"
  if [[ -z "$pids" ]]; then
    baota_die "端口 ${port} 仍在监听，但当前用户看不到占用进程。请用 root 运行，或先在宝塔进程守护中停止本站点。本次没有替换文件，也没有启动新进程。"
  fi
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    exe="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
    cmd="$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)"
    parent="$(awk '/^PPid:/ {print $2}' "/proc/$pid/status" 2>/dev/null || true)"
    baota_warn "端口 ${port} 的监听进程 PID ${pid} PPID ${parent} 程序 ${exe} 命令 ${cmd}"
    if ! root="$(baota_our_root "$pid" "$site")"; then
      baota_die "端口 ${port} 被 PID ${pid}（PPID ${parent}，程序 ${exe}，命令 ${cmd}）占用，不是本站 ${site}/backend/auth_pro。已拒绝结束该进程，也没有替换文件或启动新进程。请确认是不是同机其它站点。"
    fi
    baota_info "结束本站进程 PID ${root}（包括脱离守护、父进程为 1 的孤儿）"
    baota_signal_tree "$root"
    if [[ "$pid" != "$root" ]]; then
      baota_signal_pid "$pid"
    fi
  done <<< "$pids"
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if ! baota_port_is_open "$port"; then
      sleep 1
      if ! baota_port_is_open "$port"; then
        baota_info "端口 ${port} 已空闲"
        return 0
      fi
      baota_die "端口 ${port} 刚释放又被占用。进程守护可能在停止后立刻拉起了进程。请先在宝塔进程守护里停止本站点。本次没有替换文件，也没有启动新进程。"
    fi
    sleep 1
  done
  baota_die "本站 auth_pro 没有在时限内退出，端口 ${port} 仍被占用。本次没有替换文件，也没有启动新进程。"
}

baota_ensure_port_available() {
  baota_stop_and_reclaim "$1" "$2"
}

# 面板自带解释器。系统 python 没有面板的类路径，不能拿来建站。
baota_panel_python() {
  if [[ -x /www/server/panel/pyenv/bin/python3 ]]; then
    printf '%s\n' /www/server/panel/pyenv/bin/python3
    return 0
  fi
  if command -v btpython >/dev/null 2>&1; then
    command -v btpython
    return 0
  fi
  return 1
}

baota_panel_script() {
  if [[ -n "${BAOTA_PAYLOAD:-}" && -f "$BAOTA_PAYLOAD/baota-panel.py" ]]; then
    printf '%s\n' "$BAOTA_PAYLOAD/baota-panel.py"
    return 0
  fi
  if [[ -f "$SCRIPT_DIR/baota-panel.py" ]]; then
    printf '%s\n' "$SCRIPT_DIR/baota-panel.py"
    return 0
  fi
  return 1
}

# 面板目录、解释器和辅助脚本都在时才走自动建站。缺一则由调用方决定是退出还是只放文件。
baota_panel_available() {
  [[ -d /www/server/panel ]] || return 1
  baota_panel_python >/dev/null || return 1
  baota_panel_script >/dev/null || return 1
}

# 辅助脚本的标准输出是 AUTH_PRO_* 键值，只写进临时文件再读入下面这些变量。
# 中文说明仍走标准错误，操作者能看见。非 0 表示这一步失败，调用方负责撤掉本次新建的资源。
# 不要用命令替换调用本函数，否则读到的变量会随子进程消失。
baota_panel_run() {
  local py script capture line code
  py="$(baota_panel_python)" || baota_die "找不到宝塔面板的 Python（btpython）。请确认面板已安装。"
  script="$(baota_panel_script)" || baota_die "缺少 baota-panel.py。请使用当前版本的安装包。"
  capture="$(mktemp "${TMPDIR:-/tmp}/auth-pro-panel.XXXXXX")"
  BAOTA_PANEL_RESULT=""
  BAOTA_PANEL_VERSION=""
  BAOTA_PANEL_SITE_ID=""
  BAOTA_PANEL_NGINX=""
  BAOTA_PANEL_PROGRAM=""
  BAOTA_PANEL_SUP_CONF=""
  BAOTA_PANEL_PLUGIN=""
  BAOTA_PANEL_LIST=""
  BAOTA_PANEL_CTL=""
  set +e
  "$py" "$script" "$@" >"$capture"
  code=$?
  set -e
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      AUTH_PRO_RESULT=*) BAOTA_PANEL_RESULT="${line#AUTH_PRO_RESULT=}" ;;
      AUTH_PRO_PANEL_VERSION=*) BAOTA_PANEL_VERSION="${line#AUTH_PRO_PANEL_VERSION=}" ;;
      AUTH_PRO_SITE_ID=*) BAOTA_PANEL_SITE_ID="${line#AUTH_PRO_SITE_ID=}" ;;
      AUTH_PRO_NGINX=*) BAOTA_PANEL_NGINX="${line#AUTH_PRO_NGINX=}" ;;
      AUTH_PRO_PROGRAM=*) BAOTA_PANEL_PROGRAM="${line#AUTH_PRO_PROGRAM=}" ;;
      AUTH_PRO_SUP_CONF=*) BAOTA_PANEL_SUP_CONF="${line#AUTH_PRO_SUP_CONF=}" ;;
      AUTH_PRO_PLUGIN=*) BAOTA_PANEL_PLUGIN="${line#AUTH_PRO_PLUGIN=}" ;;
      AUTH_PRO_LIST=*) BAOTA_PANEL_LIST="${line#AUTH_PRO_LIST=}" ;;
      AUTH_PRO_CTL=*) BAOTA_PANEL_CTL="${line#AUTH_PRO_CTL=}" ;;
    esac
  done < "$capture"
  rm -f "$capture"
  return "$code"
}

baota_site_domain() {
  local name="${AUTH_PRO_PUBLIC_HOST:-}"
  if [[ -z "$name" ]]; then
    name="$(basename "$BAOTA_SITE_ROOT")"
  fi
  printf '%s' "$name" | tr '[:upper:]' '[:lower:]'
}

# 其它 auth-pro 站点写在 baota.env 里的端口。没有 /www/wwwroot 时不输出，也不报错。
baota_reserved_ports() {
  local envfile port
  [[ -d /www/wwwroot ]] || return 0
  while IFS= read -r envfile; do
    [[ -f "$envfile" ]] || continue
    port="$(sed -n 's/^PORT=//p' "$envfile" | head -n 1)"
    if [[ "$port" =~ ^[0-9]+$ ]]; then
      printf '%s\n' "$port"
    fi
  done < <(find /www/wwwroot -path '*/backend/baota.env' -type f 2>/dev/null)
}

baota_port_reserved() {
  local port="$1" known
  known="$(baota_reserved_ports)"
  [[ -n "$known" ]] || return 1
  printf '%s\n' "$known" | grep -qx "$port"
}

# 全新安装选端口。显式 --port 被占用就退出，不结束占用进程。
# 未指定时从 19127 找到 19227，跳过正在监听的端口和已有 auth-pro 配置里的端口。
# 结果写到 BAOTA_SELECTED_PORT。不要用命令替换接这个函数：查找过程的提示也在标准输出。
baota_select_install_port() {
  local data port candidate
  data="$(baota_data_dir)"
  if [[ "$BAOTA_PORT_SET" == "1" ]]; then
    port="$BAOTA_PORT"
    baota_assert_port_number "$port"
    if baota_port_is_open "$port" || baota_port_reserved "$port"; then
      baota_die "端口 ${port} 已被占用，或已写在其它 auth-pro 的配置里。已停止。请换一个端口。本次没有新建站点或数据库。"
    fi
    BAOTA_SELECTED_PORT="$port"
    return 0
  fi
  if [[ -f "$data/baota.env" ]]; then
    port="$(sed -n 's/^PORT=//p' "$data/baota.env" | head -n 1)"
    if [[ -n "$port" ]]; then
      baota_assert_port_number "$port"
      BAOTA_SELECTED_PORT="$port"
      return 0
    fi
  fi
  for candidate in $(seq 19127 19227); do
    if baota_port_is_open "$candidate" || baota_port_reserved "$candidate"; then
      baota_info "端口 ${candidate} 不可用，继续查找"
      continue
    fi
    BAOTA_PORT="$candidate"
    BAOTA_PORT_SET=1
    BAOTA_SELECTED_PORT="$candidate"
    return 0
  done
  baota_die "从 19127 到 19227 没有空闲端口。已停止。本次没有新建站点或数据库。"
}

# 管理员初始密码只要 8 位数字。数据库密码仍走 baota_random_alnum，不在这里生成。
baota_random_digits() {
  python3 - "$1" <<'PY'
import secrets
import sys
length = int(sys.argv[1])
if length < 1:
    raise SystemExit(2)
sys.stdout.write("".join(secrets.choice("0123456789") for _ in range(length)))
PY
}

# kind=lower 只生成小写，给数据库名用；kind=mixed 给密码用。长度含第一个字符。
baota_random_alnum() {
  python3 - "$1" "$2" <<'PY'
import secrets
import string
import sys
length = int(sys.argv[1])
kind = sys.argv[2]
if kind == "lower":
    alphabet = string.ascii_lowercase + string.digits
    first = string.ascii_lowercase
else:
    alphabet = string.ascii_letters + string.digits
    first = string.ascii_letters
sys.stdout.write(secrets.choice(first) + "".join(secrets.choice(alphabet) for _ in range(length - 1)))
PY
}

# 中央备份目录。没有 /www，或目录建不出来时返回非 0，调用方改回原来的路径，更新不能因此中断。
# 宝塔的 /www/backup 通常是 700。在线更新由网站用户 www 执行，必须能进入本站这一份备份并删掉旧的。
# 只给 /www/backup 加执行位，不开放列目录，其它备份子目录仍按原权限。
baota_central_backup_root() {
  local name root
  [[ -d /www ]] || return 1
  name="$(basename "$BAOTA_SITE_ROOT")"
  [[ -n "$name" && "$name" != "." && "$name" != ".." ]] || return 1
  if [[ -d /www/backup ]]; then
    chmod a+x /www/backup 2>/dev/null || true
  fi
  root="/www/backup/auth-pro/${name}"
  if ! mkdir -p "$root" 2>/dev/null; then
    baota_warn "无法创建 ${root}，备份仍放在原来的位置，更新继续"
    return 1
  fi
  chmod a+x /www/backup/auth-pro 2>/dev/null || true
  if id www >/dev/null 2>&1; then
    chown -R www:www "$root" 2>/dev/null || baota_warn "无法把 ${root} 交给 www。在线更新可能删不掉这里的旧备份。"
  fi
  printf '%s\n' "$root"
}

# 把一个旧备份移到目标目录。失败只打印说明，不删除源目录。
baota_move_backup() {
  local src="$1" dest_dir="$2" base
  [[ -e "$src" ]] || return 0
  base="$(basename "$src")"
  if ! mkdir -p "$dest_dir" 2>/dev/null; then
    baota_warn "无法创建 ${dest_dir}，已保留 ${src}"
    return 1
  fi
  if ! mv "$src" "$dest_dir/$base"; then
    baota_warn "旧备份移动失败，未删除：${src}"
    return 1
  fi
  baota_info "已迁移旧备份：${src} -> ${dest_dir}/${base}"
}

# 只迁移名字严格匹配本站点的旧备份。其它站点的目录原样留下。
# 匹配：<父目录>/<站点>.backup.<14位时间>、<站点>.overlay-backup.<14位时间>，
# 以及本站 backend/updates/backups/baota-upgrade-*。
baota_migrate_matching_backups() {
  local site_root="$1" dest_root="$2" site_name parent old digits
  site_name="$(basename "$site_root")"
  parent="$(dirname "$site_root")"
  digits='[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]'
  shopt -s nullglob
  for old in "$parent/${site_name}.overlay-backup."$digits; do
    baota_move_backup "$old" "$dest_root/overlay" || true
  done
  for old in "$parent/${site_name}.backup."$digits; do
    baota_move_backup "$old" "$dest_root/rename" || true
  done
  if [[ -d "$site_root/backend/updates/backups" ]]; then
    for old in "$site_root/backend/updates/backups"/baota-upgrade-*; do
      [[ -d "$old" ]] || continue
      baota_move_backup "$old" "$dest_root/upgrade" || true
    done
  fi
  shopt -u nullglob
}

baota_migrate_old_backups() {
  local root
  root="$(baota_central_backup_root)" || return 0
  baota_migrate_matching_backups "$BAOTA_SITE_ROOT" "$root" || true
}

# 目录里按修改时间只留最近 keep 份，多出来的删掉。目录不存在就跳过。
baota_prune_backup_dir() {
  local dir="$1" keep="${2:-3}" path i=0
  [[ -d "$dir" ]] || return 0
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    i=$((i + 1))
    if [[ "$i" -gt "$keep" ]]; then
      rm -rf "$path"
    fi
  done < <(ls -1dt "$dir"/* 2>/dev/null || true)
}

# 健康检查已经成功才调用。失败的更新要留着刚做的那份备份。
baota_prune_after_health() {
  local root data
  if root="$(baota_central_backup_root)"; then
    baota_prune_backup_dir "$root/overlay"
    baota_prune_backup_dir "$root/rename"
    baota_prune_backup_dir "$root/upgrade"
    baota_prune_backup_dir "$root/install"
  fi
  data="$(baota_data_dir)/updates/backups"
  [[ -d "$data" ]] || return 0
  baota_prune_backup_dir_glob "$data/baota-upgrade-"*
  baota_prune_backup_dir_glob "$data/baota-install-"*
}

baota_prune_backup_dir_glob() {
  local keep=3 path i=0
  while IFS= read -r path; do
    [[ -n "$path" && -e "$path" ]] || continue
    i=$((i + 1))
    if [[ "$i" -gt "$keep" ]]; then
      rm -rf "$path"
    fi
  done < <(ls -1dt "$@" 2>/dev/null || true)
}

baota_prepare_backup_dir() {
  local data ts root kind
  data="$(baota_data_dir)"
  ts="$(date '+%Y%m%d%H%M%S')"
  BAOTA_BACKUP_DIR=""
  if [[ "$BAOTA_DRY_RUN" != "1" ]] && root="$(baota_central_backup_root)"; then
    case "$BAOTA_ACTION" in
      upgrade) kind="upgrade" ;;
      *) kind="install" ;;
    esac
    if mkdir -p "$root/$kind" 2>/dev/null; then
      BAOTA_BACKUP_DIR="${root}/${kind}/baota-${BAOTA_ACTION}-${ts}-$$"
    else
      baota_warn "无法创建 ${root}/${kind}，备份仍放在数据目录"
    fi
  fi
  if [[ -z "$BAOTA_BACKUP_DIR" ]]; then
    BAOTA_BACKUP_DIR="${data}/updates/backups/baota-${BAOTA_ACTION}-${ts}-$$"
  fi
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将创建备份：$BAOTA_BACKUP_DIR"
    return 0
  fi
  mkdir -p "$BAOTA_BACKUP_DIR/data"
  cat > "$BAOTA_BACKUP_DIR/README.txt" <<EOF
本目录由宝塔${BAOTA_ACTION}脚本在替换程序文件前生成。
data/ 是当时的运行数据副本（不含 updates/backups 自身，避免递归）。
db.sql 是 mysqldump 结果；没有该文件表示当时跳过或未能导出。
auth_pro.prev、index.html.prev、assets/ 是替换前的程序文件，可用于手工回滚。
回滚前请先在宝塔进程守护中停止站点。
EOF
  baota_info "备份目录：$BAOTA_BACKUP_DIR"
}

baota_copy_durable_into_backup() {
  local data dest file dir name
  data="$(baota_data_dir)"
  dest="$BAOTA_BACKUP_DIR/data"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将备份 db.json、install.lock、jwt.secret 以及插件/模板/日志等运行目录"
    return 0
  fi
  for file in "${BAOTA_DURABLE_FILES[@]}"; do
    if [[ -e "$data/$file" ]]; then
      cp -a "$data/$file" "$dest/$file"
    fi
  done
  for dir in "${BAOTA_DURABLE_DIRS[@]}"; do
    [[ -d "$data/$dir" ]] || continue
    mkdir -p "$dest/$dir"
    if [[ "$dir" == "updates" ]]; then
      for name in "$data/$dir"/* "$data/$dir"/.[!.]* "$data/$dir"/..?*; do
        [[ -e "$name" ]] || continue
        [[ "$(basename "$name")" == "backups" ]] && continue
        cp -a "$name" "$dest/$dir/"
      done
      continue
    fi
    cp -a "$data/$dir"/. "$dest/$dir/"
  done
}

baota_backup_program_file() {
  local src="$1" name="$2"
  [[ -e "$src" ]] || return 0
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将备份程序文件：$src"
    return 0
  fi
  # 面板刚建好的默认页会在放文件时被换掉。没有备份目录时直接替换，避免 mkdir 空路径。
  [[ -n "$BAOTA_BACKUP_DIR" ]] || return 0
  mkdir -p "$BAOTA_BACKUP_DIR"
  cp -a "$src" "$BAOTA_BACKUP_DIR/$name"
}

baota_durable_manifest() {
  local data="$1" out="$2" file dir path sum
  data="$(baota_data_dir)"
  : > "$out"
  for file in "${BAOTA_DURABLE_FILES[@]}"; do
    path="$data/$file"
    if [[ -f "$path" ]]; then
      sum="$(sha256sum "$path" | awk '{print $1}')"
      printf '%s\t%s\n' "$sum" "$path" >> "$out"
    fi
  done
  for dir in "${BAOTA_DURABLE_DIRS[@]}"; do
    [[ -d "$data/$dir" ]] || continue
    while IFS= read -r -d '' path; do
      sum="$(sha256sum "$path" | awk '{print $1}')"
      printf '%s\t%s\n' "$sum" "$path" >> "$out"
    done < <(find "$data/$dir" -type f ! -path "$data/updates/backups/*" -print0 | sort -z)
  done
}

baota_verify_manifest() {
  local manifest="$1" sum path now
  [[ "$BAOTA_DRY_RUN" == "1" ]] && return 0
  while IFS=$'\t' read -r sum path; do
    [[ -n "${sum:-}" ]] || continue
    [[ -f "$path" ]] || baota_die "升级后运行数据丢失：$path"
    now="$(sha256sum "$path" | awk '{print $1}')"
    [[ "$now" == "$sum" ]] || baota_die "升级后运行数据内容变化：$path"
  done < "$manifest"
  baota_info "已核对运行数据内容未变"
}

baota_mysql_backup() {
  local data cfg dest
  data="$(baota_data_dir)"
  cfg="$data/db.json"
  dest="$BAOTA_BACKUP_DIR/db.sql"
  if [[ ! -f "$cfg" ]]; then
    baota_warn "没有 db.json，跳过 MySQL 导出"
    return 0
  fi
  if [[ "$BAOTA_SKIP_MYSQL" == "1" ]]; then
    baota_warn "已按 --skip-mysql 跳过 MySQL 导出。db.json 文件副本仍会备份。"
    return 0
  fi
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将在替换文件前用 mysqldump 导出数据库（口令只通过环境变量传递）"
    return 0
  fi
  command -v python3 >/dev/null 2>&1 || baota_die "需要 python3 读取 db.json 才能导出数据库。已有手工备份时可加 --skip-mysql。"
  command -v mysqldump >/dev/null 2>&1 || baota_die "未找到 mysqldump，已停止，程序文件尚未替换。安装 MySQL 客户端，或确认已有备份后加 --skip-mysql。"
  local status=0
  python3 - "$cfg" "$dest" <<'PY' || status=$?
import json, os, subprocess, sys
cfg_path, dest = sys.argv[1], sys.argv[2]
cfg = json.load(open(cfg_path, encoding="utf-8"))
host = str(cfg.get("host") or "")
port = str(cfg.get("port") or "")
database = str(cfg.get("database") or "")
user = str(cfg.get("username") or "")
password = str(cfg.get("password") or "")
if not database or not user:
    sys.stderr.write("db.json 缺少数据库名或用户名，跳过导出\n")
    sys.exit(2)
args = ["mysqldump", "--single-transaction", "--quick"]
if host.startswith("unix:"):
    args += ["--socket", host[len("unix:"):]]
else:
    args += ["-h", host or "127.0.0.1"]
    if port:
        args += ["-P", port]
args += ["-u", user, database]
env = os.environ.copy()
env["MYSQL_PWD"] = password
with open(dest, "wb") as out:
    proc = subprocess.run(args, stdout=out, stderr=subprocess.PIPE, env=env)
if proc.returncode != 0:
    err = proc.stderr.decode("utf-8", "replace").strip()
    sys.stderr.write(err + "\n")
    sys.exit(proc.returncode or 1)
PY
  if [[ "$status" -eq 2 ]]; then
    rm -f "$dest"
    baota_warn "db.json 不完整，已跳过 MySQL 导出"
    return 0
  fi
  if [[ "$status" -ne 0 ]]; then
    rm -f "$dest"
    baota_die "数据库备份失败，已停止，程序文件尚未替换。可检查 MySQL 是否可连；确认已有备份后可加 --skip-mysql。"
  fi
  chmod 600 "$dest" 2>/dev/null || true
  baota_info "数据库已导出到 $dest"
}

baota_sync_root_file() {
  local name="$1"
  local src="$BAOTA_PAYLOAD/$name"
  local dest="$BAOTA_SITE_ROOT/$name"
  [[ -f "$src" ]] || return 0
  if [[ -L "$src" ]]; then
    baota_die "拒绝安装符号链接：$name"
  fi
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将更新 $dest"
    return 0
  fi
  cp -a "$src" "$dest.tmp.$$"
  mv -f "$dest.tmp.$$" "$dest"
}

baota_replace_assets() {
  local src="$BAOTA_PAYLOAD/assets" dest="$BAOTA_SITE_ROOT/assets"
  [[ -d "$src" ]] || baota_die "发布包缺少 assets 目录"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将替换 $dest"
    return 0
  fi
  BAOTA_STAGING_ASSETS="$BAOTA_SITE_ROOT/.assets.staging.$$"
  rm -rf "$BAOTA_STAGING_ASSETS"
  cp -a "$src" "$BAOTA_STAGING_ASSETS"
  if [[ -d "$dest" ]]; then
    if [[ -n "$BAOTA_BACKUP_DIR" ]]; then
      mkdir -p "$BAOTA_BACKUP_DIR"
      rm -rf "$BAOTA_BACKUP_DIR/assets"
      mv "$dest" "$BAOTA_BACKUP_DIR/assets"
    else
      rm -rf "$dest"
    fi
  fi
  if ! mv "$BAOTA_STAGING_ASSETS" "$dest"; then
    if [[ -d "$BAOTA_BACKUP_DIR/assets" && ! -e "$dest" ]]; then
      mv "$BAOTA_BACKUP_DIR/assets" "$dest" || true
    fi
    baota_die "替换 assets 失败，已尝试恢复旧目录"
  fi
  BAOTA_STAGING_ASSETS=""
}

baota_install_binary() {
  local src="$BAOTA_PAYLOAD/backend/auth_pro"
  local dest="$BAOTA_SITE_ROOT/backend/auth_pro"
  local stage
  [[ -f "$src" ]] || baota_die "发布包缺少 backend/auth_pro"
  if [[ -L "$src" ]]; then
    baota_die "拒绝安装符号链接：backend/auth_pro"
  fi
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将安装二进制到 $dest 并设置 755"
    return 0
  fi
  mkdir -p "$BAOTA_SITE_ROOT/backend"
  if [[ -f "$dest" && -n "$BAOTA_BACKUP_DIR" ]]; then
    mkdir -p "$BAOTA_BACKUP_DIR"
    cp -a "$dest" "$BAOTA_BACKUP_DIR/auth_pro.prev"
  fi
  stage="$dest.next.$$"
  cp -a "$src" "$stage"
  chmod 755 "$stage"
  mv -f "$stage" "$dest"
}

baota_chmod_binary() {
  local bin="$BAOTA_SITE_ROOT/backend/auth_pro" mode
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将把 $bin 设为 755"
    return 0
  fi
  [[ -f "$bin" ]] || baota_die "找不到后端程序：$bin"
  chmod 755 "$bin" || baota_die "无法设置可执行权限：$bin 。请用网站所属用户或 root 运行。"
  mode="$(stat -c '%a' "$bin" 2>/dev/null || stat -f '%OLp' "$bin")"
  [[ "$mode" == "755" ]] || baota_die "后端程序权限不是 755（当前 ${mode}）：$bin"
  if command -v file >/dev/null 2>&1; then
    if ! file -b "$bin" | grep -Eq 'ELF 64-bit.*x86-64'; then
      baota_warn "backend/auth_pro 不是 Linux amd64 ELF。正式环境请使用 Release 包里的二进制。"
    fi
  fi
  baota_info "已设置可执行权限 755：$bin"
}

# 网站根只留页面和 backend/。安装入口不复制到站点里，旧包留下的脚本删掉。
baota_install_scripts() {
  local name
  for name in install.sh baota-install.sh baota-upgrade.sh baota-lib.sh; do
    [[ -f "$BAOTA_SITE_ROOT/$name" ]] || continue
    if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
      baota_info "将删除过时的网站根脚本 $name"
      continue
    fi
    rm -f "$BAOTA_SITE_ROOT/$name"
    baota_info "已删除过时的网站根脚本 $name"
  done
}

baota_write_env() {
  local data port host file tmp existing_host
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  host="${AUTH_PRO_HOST:-127.0.0.1}"
  file="$data/baota.env"
  if [[ -f "$file" && "$BAOTA_PORT_SET" != "1" && -z "${AUTH_PRO_HOST:-}" ]]; then
    baota_info "保留已有环境文件：$file"
    return 0
  fi
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将写入 $file （PORT=${port} HOST=${host}）"
    return 0
  fi
  mkdir -p "$data"
  if [[ -f "$file" ]]; then
    existing_host="$(sed -n 's/^HOST=//p' "$file" | head -n 1)"
    if [[ -z "${AUTH_PRO_HOST:-}" && -n "$existing_host" ]]; then
      host="$existing_host"
    fi
  fi
  tmp="$file.tmp.$$"
  cat > "$tmp" <<EOF
# 由宝塔安装/升级脚本生成。进程守护请启动 start.sh，由它加载本文件。
PORT=${port}
HOST=${host}
AUTO_PRO_DATA_DIR=${data}
AUTO_PRO_PROCESS_MANAGER=supervisor
EOF
  chmod 600 "$tmp"
  mv -f "$tmp" "$file"
  baota_info "已写入 $file"
}

baota_guardian_start_template() {
  if [[ -n "${BAOTA_PAYLOAD:-}" && -f "$BAOTA_PAYLOAD/guardian-start.sh" ]]; then
    printf '%s\n' "$BAOTA_PAYLOAD/guardian-start.sh"
    return 0
  fi
  if [[ -f "$SCRIPT_DIR/guardian-start.sh" ]]; then
    printf '%s\n' "$SCRIPT_DIR/guardian-start.sh"
    return 0
  fi
  if [[ -f "$SCRIPT_DIR/../backend/handler/guardian_start.sh" ]]; then
    printf '%s\n' "$SCRIPT_DIR/../backend/handler/guardian_start.sh"
    return 0
  fi
  return 1
}

baota_write_start_script() {
  local data file template
  data="$(baota_data_dir)"
  file="$data/start.sh"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将写入启动脚本 $file"
    return 0
  fi
  template="$(baota_guardian_start_template)" || baota_die "缺少 guardian-start.sh，无法写入 $file"
  mkdir -p "$data"
  cp "$template" "$file"
  chmod 755 "$file"
  grep -q 'auth-pro-guardian-start' "$file" || baota_die "启动脚本 $file 不是进程守护交接版本"
  baota_info "已写入 $file"
}

baota_write_nginx_snippet() {
  local data port file site_root
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  site_root="$BAOTA_SITE_ROOT"
  file="$data/baota-nginx.snippet.conf"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将写入 Nginx 片段 $file"
    return 0
  fi
  mkdir -p "$data"
  cat > "$file" <<EOF
# 把下面的 location 放进宝塔站点的 server { } 中。
# 反代到本机后端（域名和证书仍在面板里配置）：
#
# location / {
#     proxy_pass http://127.0.0.1:${port};
#     proxy_http_version 1.1;
#     proxy_set_header Host \$host;
#     proxy_set_header X-Real-IP \$remote_addr;
#     proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
#     proxy_set_header X-Forwarded-Proto \$scheme;
# }

location ^~ /backend/ { return 404; }
location = /install.sh { return 404; }
location = /baota-install.sh { return 404; }
location = /baota-upgrade.sh { return 404; }
location = /baota-lib.sh { return 404; }
location = /baota-panel.py { return 404; }
location = /guardian-start.sh { return 404; }
location ~* ^/(db\\.json|install\\.lock|jwt\\.secret)$ { return 404; }
location ~* \\.(log|pid)$ { return 404; }

# 后端 502/503/504 时返回站点根的静态说明页。仓库模板：deploy/nginx/backend-unavailable.conf
error_page 502 503 504 /backend-unavailable.html;
location = /backend-unavailable.html {
    root ${site_root};
    default_type text/html;
}

# 入口页不要缓存。在线更新完成后浏览器必须重新获取 index.html，才能加载新的前端资源。
location = /index.html {
    proxy_pass http://127.0.0.1:${port};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_hide_header Cache-Control;
    proxy_hide_header Pragma;
    proxy_hide_header Expires;
    add_header Cache-Control "no-cache, no-store, must-revalidate" always;
    add_header Pragma "no-cache" always;
    add_header Expires "0" always;
}
EOF
  baota_info "已写入 $file"
}

baota_write_guardian_note() {
  local data port file
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  file="$data/baota-guardian.txt"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将写入进程守护说明 $file"
    return 0
  fi
  mkdir -p "$data"
  cat > "$file" <<EOF
名称: auth_pro
启动命令: ${data}/start.sh
运行目录: ${data}
端口: ${port}
说明: 在线更新会替换文件后退出，由本守护按 start.sh 拉起。手工替换程序或 --no-start 升级前，仍要先在宝塔进程守护里停止此项，否则进程会被立刻拉起。
Nginx 反代: 127.0.0.1:${port}
Nginx 拦截片段: ${data}/baota-nginx.snippet.conf
EOF
  baota_info "已写入 $file"
}

baota_write_process_manager_marker() {
  local data file
  data="$(baota_data_dir)"
  file="$data/process-manager"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将写入进程守护标记 $file"
    return 0
  fi
  mkdir -p "$data"
  printf 'supervisor\n' > "$file"
  baota_info "已写入进程守护标记 $file"
}

baota_nginx_vhost_dirs() {
  if [[ -n "${AUTH_PRO_NGINX_VHOST_DIR:-}" ]]; then
    printf '%s\n' "$AUTH_PRO_NGINX_VHOST_DIR"
  fi
  printf '%s\n' \
    /www/server/panel/vhost/nginx \
    /www/server/nginx/conf/vhost \
    /etc/nginx/conf.d \
    /etc/nginx/sites-enabled
}

baota_configure_nginx_error_page() {
  local site port nginx dir conf backup
  site="$BAOTA_SITE_ROOT"
  port="$(baota_effective_port)"
  nginx="${AUTH_PRO_NGINX_BIN:-nginx}"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将尝试把后端不可达静态页写入引用 ${site} 的 Nginx 站点配置"
    return 0
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    baota_warn "没有 python3，跳过自动写入 Nginx error_page。请按 backend/baota-nginx.snippet.conf 手工配置。"
    return 0
  fi
  local found=0
  while IFS= read -r dir; do
    [[ -d "$dir" ]] || continue
    while IFS= read -r conf; do
      [[ -f "$conf" ]] || continue
      # 只改引用本站目录的配置。不能按端口去改，否则同机其它站点反代到同一端口时会被写到。
      if ! grep -Fq "$site" "$conf"; then
        continue
      fi
      found=1
      if grep -Fq "location = /backend-unavailable.html" "$conf" && grep -Fq "BEGIN AUTH_PRO_INDEX_NO_CACHE" "$conf"; then
        baota_info "Nginx 已包含后端不可达页面和入口页 no-cache：$conf"
        continue
      fi
      backup="${conf}.bak.auth-pro-$(date '+%Y%m%d%H%M%S')"
      cp -a "$conf" "$backup" || { baota_warn "无法备份 $conf ，已跳过"; continue; }
      if ! python3 - "$conf" "$site" "$port" <<'PY'
import pathlib, sys
path, site, port = sys.argv[1], sys.argv[2], sys.argv[3]
if any(ch in site for ch in "\n;{}\\"):
    raise SystemExit("网站根不能包含换行或 nginx 元字符")
if not port.isdigit():
    raise SystemExit("端口不正确")
text = pathlib.Path(path).read_text(encoding="utf-8", errors="replace")
marker = "# BEGIN AUTH_PRO_BACKEND_UNAVAILABLE"
index_marker = "# BEGIN AUTH_PRO_INDEX_NO_CACHE"
if marker in text or "location = /backend-unavailable.html" in text:
    if index_marker in text or "location = /index.html" in text:
        raise SystemExit(0)
    index_block = f"""
    {index_marker}
    location = /index.html {{
        proxy_pass http://127.0.0.1:{port};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_hide_header Cache-Control;
        proxy_hide_header Pragma;
        proxy_hide_header Expires;
        add_header Cache-Control "no-cache, no-store, must-revalidate" always;
        add_header Pragma "no-cache" always;
        add_header Expires "0" always;
    }}
    # END AUTH_PRO_INDEX_NO_CACHE
"""
    idx = text.rfind("}")
    if idx < 0:
        raise SystemExit("找不到 server 块结束括号")
    pathlib.Path(path).write_text(text[:idx] + index_block + "\n" + text[idx:], encoding="utf-8")
    raise SystemExit(0)
block = f"""
    {marker}
    error_page 502 503 504 /backend-unavailable.html;
    location = /backend-unavailable.html {{
        root {site};
        default_type text/html;
    }}
    {index_marker}
    location = /index.html {{
        proxy_pass http://127.0.0.1:{port};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_hide_header Cache-Control;
        proxy_hide_header Pragma;
        proxy_hide_header Expires;
        add_header Cache-Control "no-cache, no-store, must-revalidate" always;
        add_header Pragma "no-cache" always;
        add_header Expires "0" always;
    }}
    # END AUTH_PRO_INDEX_NO_CACHE
    # END AUTH_PRO_BACKEND_UNAVAILABLE
"""
idx = text.rfind("}")
if idx < 0:
    raise SystemExit("找不到 server 块结束括号")
pathlib.Path(path).write_text(text[:idx] + block + "\n" + text[idx:], encoding="utf-8")
PY
      then
        cp -a "$backup" "$conf" || true
        baota_warn "写入 Nginx 失败，已还原 $conf"
        continue
      fi
      if ! command -v "$nginx" >/dev/null 2>&1; then
        cp -a "$backup" "$conf" || true
        baota_warn "未找到 nginx，已还原 $conf 。请手工合并 backend/baota-nginx.snippet.conf"
        continue
      fi
      if ! "$nginx" -t >/dev/null 2>&1; then
        cp -a "$backup" "$conf" || true
        baota_warn "nginx -t 未通过，已还原 $conf"
        continue
      fi
      if "$nginx" -s reload >/dev/null 2>&1; then
        baota_info "已写入后端不可达页面并 reload：$conf （备份 $backup）"
      else
        baota_warn "配置已通过 nginx -t，但 reload 失败。请手工执行 nginx -s reload。备份在 $backup"
      fi
    done < <(find "$dir" -maxdepth 1 -type f -name '*.conf' | sort)
  done < <(baota_nginx_vhost_dirs)
  if [[ "$found" -eq 0 ]]; then
    baota_warn "没有找到引用本站点的 Nginx 配置。请把 backend/baota-nginx.snippet.conf 里的 error_page 放进站点 server。模板见 deploy/nginx/backend-unavailable.conf"
  fi
}

baota_apply_payload() {
  local site="$BAOTA_SITE_ROOT"
  if baota_same_payload; then
    baota_warn "发布目录就是网站根，只整理权限和运行配置，不再从另一份包覆盖文件。"
  else
    baota_backup_program_file "$site/index.html" "index.html.prev"
    baota_sync_root_file "index.html"
    baota_sync_root_file "version.json"
    baota_sync_root_file "favicon.ico"
    baota_sync_root_file "manifest.json"
    baota_sync_root_file "backend-unavailable.html"
    baota_replace_assets
    baota_install_binary
  fi
  baota_install_scripts
  baota_chmod_binary
  baota_write_env
  baota_write_process_manager_marker
  baota_write_start_script
  baota_write_nginx_snippet
  baota_write_guardian_note
  # 面板自动安装会在反代之后写入同一份片段。这里再改一次会和反代的 location 重复，nginx -t 会失败。
  if [[ "${BAOTA_PANEL_INSTALL:-}" != "1" ]]; then
    baota_configure_nginx_error_page
  fi
}

baota_tighten_secrets() {
  local data
  data="$(baota_data_dir)"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    return 0
  fi
  if [[ -f "$data/db.json" ]]; then
    chmod 600 "$data/db.json" || baota_warn "无法把 db.json 收紧为 600"
  fi
  if [[ -f "$data/jwt.secret" ]]; then
    chmod 600 "$data/jwt.secret" || baota_warn "无法把 jwt.secret 收紧为 600"
  fi
}

baota_rollback_programs() {
  local site="$BAOTA_SITE_ROOT" data
  data="$(baota_data_dir)"
  [[ -n "${BAOTA_BACKUP_DIR:-}" && -d "$BAOTA_BACKUP_DIR" ]] || return 0
  baota_warn "健康检查未通过，正在把程序文件换回升级前的版本"
  if [[ -n "${BAOTA_SUP_PROGRAM:-}" ]]; then
    baota_supervisor_stop_ours
  fi
  if [[ -f "$BAOTA_BACKUP_DIR/auth_pro.prev" ]]; then
    cp -a "$BAOTA_BACKUP_DIR/auth_pro.prev" "$data/auth_pro"
    chmod 755 "$data/auth_pro" || true
  fi
  if [[ -f "$BAOTA_BACKUP_DIR/index.html.prev" ]]; then
    cp -a "$BAOTA_BACKUP_DIR/index.html.prev" "$site/index.html"
  fi
  if [[ -d "$BAOTA_BACKUP_DIR/assets" ]]; then
    rm -rf "$site/assets"
    mv "$BAOTA_BACKUP_DIR/assets" "$site/assets" || true
  fi
}

baota_health_body() {
  local url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsS --max-time 2 "$url" 2>/dev/null || true
    return 0
  fi
  wget -qO- -T 2 "$url" 2>/dev/null || true
}

# 宝塔 systemd 单元是「面板 python 解释器 + supervisord 脚本」，/proc/comm 只有 python。
# 直接执行 supervisord 可执行文件时 comm 才是 supervisord。两种都算被守护拉起。
baota_parent_is_supervisord() {
  local ppid="$1" comm cmd
  [[ "$ppid" =~ ^[0-9]+$ ]] || return 1
  [[ "$ppid" -gt 1 ]] || return 1
  comm="$(tr -d ' \n' < "/proc/$ppid/comm" 2>/dev/null || true)"
  if [[ "$comm" == "supervisord" ]]; then
    return 0
  fi
  cmd="$(tr '\0' ' ' < "/proc/$ppid/cmdline" 2>/dev/null || true)"
  [[ "$cmd" == *supervisord* ]]
}

# 监听者必须是本站 auth_pro，且直接父进程是 supervisord。父进程为 1 的是脱管进程。
baota_listener_supervised() {
  local port="$1" site="$2" pid count root ppid
  count="$(baota_pids_for_port "$port" | wc -l | tr -d ' ')"
  [[ "$count" == "1" ]] || return 1
  pid="$(baota_pids_for_port "$port" | head -n 1)"
  root="$(baota_our_root "$pid" "$site" || true)"
  [[ -n "$root" ]] || return 1
  ppid="$(awk '/^PPid:/ {print $2}' "/proc/$root/status" 2>/dev/null || true)"
  baota_parent_is_supervisord "$ppid"
}

baota_guardian_manual() {
  local data port program
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  program="${BAOTA_SUP_PROGRAM:-auth_pro}"
  cat <<EOF >&2
[手动] 进程守护没有就绪，这次不能当作安装完成。请只添加本站点，不要改其它站点，也不要再 nohup。
       名称：${program}
       启动用户：www
       运行目录：${data}
       启动命令：${data}/start.sh
       进程数量：1
       后端端口：${port}
       保存后，面板列表里要有这一项，supervisorctl 为 RUNNING，监听进程的父进程是 supervisord。
EOF
}

baota_verify_guardian() {
  local port site data program count root ppid comm
  port="$(baota_effective_port)"
  site="$BAOTA_SITE_ROOT"
  data="$(baota_data_dir)"
  baota_panel_run supervisor-check --domain "$(baota_site_domain)" --command "${data}/start.sh" || baota_die "无法读取进程守护状态"
  program="${BAOTA_PANEL_PROGRAM:-}"
  [[ -n "$program" ]] || baota_die "没有得到本站点的进程守护名称"
  BAOTA_SUP_PROGRAM="$program"
  if [[ -n "${BAOTA_PANEL_SUP_CONF:-}" ]]; then
    BAOTA_SUP_CONF="$BAOTA_PANEL_SUP_CONF"
  fi
  if [[ "${BAOTA_PANEL_PLUGIN:-}" == "yes" ]]; then
    if [[ " ${BAOTA_PANEL_LIST:-} " != *" ${program} "* ]]; then
      baota_guardian_manual
      baota_die "面板进程守护列表里没有 ${program}。当前列表：${BAOTA_PANEL_LIST:-（空）}"
    fi
  fi
  if [[ "${BAOTA_PANEL_RESULT:-}" != "running" ]]; then
    baota_guardian_manual
    baota_die "supervisorctl 未显示 ${program} 为 RUNNING。状态：${BAOTA_PANEL_CTL:-无} 核验结果：${BAOTA_PANEL_RESULT:-无}"
  fi
  count="$(baota_pids_for_port "$port" | wc -l | tr -d ' ')"
  if [[ "$count" != "1" ]]; then
    baota_guardian_manual
    baota_die "端口 ${port} 上有 ${count} 个监听进程。只能留 supervisord 拉起的那一个，不能有两个进程抢端口。"
  fi
  if ! baota_listener_supervised "$port" "$site"; then
    root="$(baota_pids_for_port "$port" | head -n 1)"
    ppid="$(awk '/^PPid:/ {print $2}' "/proc/${root:-0}/status" 2>/dev/null || true)"
    comm="$(tr -d ' \n' < "/proc/${ppid:-0}/comm" 2>/dev/null || true)"
    cmd="$(tr '\0' ' ' < "/proc/${ppid:-0}/cmdline" 2>/dev/null || true)"
    baota_guardian_manual
    baota_die "端口 ${port} 的进程 PID ${root:-无} 父进程是 ${ppid:-无}（${comm:-无}，命令 ${cmd:-无}），不是 supervisord。这是脱管进程，崩溃后不会被拉起。"
  fi
  if [[ "${BAOTA_PANEL_PLUGIN:-}" == "yes" ]]; then
    baota_info "进程守护核验通过：面板列表包含 ${program}，supervisorctl 为 RUNNING，监听进程的父进程是 supervisord"
  else
    baota_info "进程守护核验通过：插件未安装，supervisorctl 为 RUNNING，监听进程的父进程是 supervisord"
  fi
}

baota_start_backend() {
  local data port pid i health url started_by timeout
  [[ "$BAOTA_START" == "1" ]] || return 0
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  url="http://127.0.0.1:${port}/api/install/status"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将在端口 ${port} 空闲后由进程守护启动，并请求 ${url}"
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    baota_die "启动后需要 curl 或 wget 做健康检查"
  fi
  mkdir -p "$data/logs"
  baota_find_supervisor || true
  if baota_port_is_open "$port"; then
    if baota_listener_supervised "$port" "$BAOTA_SITE_ROOT"; then
      baota_info "端口 ${port} 已由 supervisord 监听本站，不再另起进程"
      started_by="supervisor"
      pid=""
      timeout="${AUTH_PRO_HEALTH_TIMEOUT:-30}"
      for i in $(seq 1 "$timeout"); do
        health="$(baota_health_body "$url")"
        if [[ -n "$health" ]]; then
          baota_info "后端已启动 端口=${port}"
          baota_info "本机检查：${url}"
          return 0
        fi
        sleep 1
      done
      baota_die "进程守护已拉起进程，但健康检查超时（${timeout}s）。请查看 ${data}/logs/auto_pro.log 。不要再 nohup 一份。"
    fi
    baota_stop_and_reclaim "$port" "$BAOTA_SITE_ROOT"
  fi
  if baota_port_is_open "$port"; then
    baota_die "端口 ${port} 还没空闲，拒绝启动新进程。"
  fi
  pid=""
  started_by="direct"
  if [[ -n "${BAOTA_SUP_PROGRAM:-}" ]]; then
    baota_info "端口 ${port} 已空闲，由进程守护启动 ${BAOTA_SUP_PROGRAM}"
    if ! baota_supervisor_start_ours; then
      baota_rollback_programs
      baota_die "进程守护没有启动新版本，已尝试回滚程序文件。请查看守护日志。不要另外 nohup 一份。"
    fi
    started_by="supervisor"
  elif [[ "${BAOTA_PANEL_INSTALL:-}" == "1" ]]; then
    baota_die "面板安装必须由进程守护拉起。拒绝 nohup，避免再留下一个父进程为 1 的脱管进程。"
  else
    baota_info "未找到指向本站的进程守护配置。端口 ${port} 已确认空闲，改为直接启动。生产环境请只在宝塔进程守护里启动 backend/start.sh。"
    pid="$(
      cd "$data"
      set -a
      # shellcheck disable=SC1091
      source ./baota.env
      set +a
      nohup "$data/auth_pro" >> "$data/logs/auto_pro.log" 2>&1 &
      echo $!
    )"
    printf '%s\n' "$pid" > "$data/auto_pro.pid"
  fi
  timeout="${AUTH_PRO_HEALTH_TIMEOUT:-30}"
  for i in $(seq 1 "$timeout"); do
    sleep 1
    health="$(baota_health_body "$url")"
    if [[ -n "$health" ]]; then
      baota_info "后端已启动 端口=${port}"
      [[ -n "$pid" ]] && baota_info "直接启动的 PID=${pid}"
      baota_info "本机检查：${url}"
      baota_info "浏览器打开站点域名。全新安装会进入安装向导，请填写事先建好的空数据库。"
      return 0
    fi
    if [[ "$started_by" == "direct" && -n "$pid" ]] && ! kill -0 "$pid" 2>/dev/null; then
      baota_rollback_programs
      baota_die "后端进程已退出，已尝试回滚程序文件。请查看 ${data}/logs/auto_pro.log"
    fi
  done
  if [[ "$started_by" == "supervisor" ]]; then
    baota_supervisor_stop_ours
  elif [[ -n "$pid" ]]; then
    baota_signal_pid "$pid"
  fi
  baota_rollback_programs
  if [[ "$started_by" == "supervisor" ]] && ! baota_port_is_open "$port"; then
    baota_supervisor_start_ours >/dev/null 2>&1 || true
  fi
  baota_die "健康检查超时（${timeout}s），已尝试回滚。请查看 ${data}/logs/auto_pro.log 。不要在旧进程还占着端口时再启动一份。"
}

baota_print_manual_steps() {
  local data port host
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  host="${AUTH_PRO_PUBLIC_HOST:-}"
  if [[ -f "$data/install.lock" ]]; then
    cat <<EOF

升级已完成。站点仍使用原来的网站、数据库和反向代理。
后端端口：${port}
运行数据目录：${data}
EOF
    return 0
  fi
  if [[ -n "$host" ]]; then
    cat <<EOF

请用浏览器打开：
  http://${host}
配好 HTTPS 证书后改用 https://${host} 。还没有安装锁时会进入安装向导。
EOF
  else
    printf '\n请用浏览器打开站点域名。还没有安装锁时会进入安装向导。\n'
  fi
  cat <<EOF

还要在宝塔面板里完成这几步（脚本不会改面板里的网站、数据库、反向代理和证书）：
  1. 创建网站，根目录设为 ${BAOTA_SITE_ROOT}
  2. 创建空 MySQL 库和用户。数据库密码在网页安装向导里填写，不要写进命令
  3. 站点反向代理到 127.0.0.1:${port}，并把 ${data}/baota-nginx.snippet.conf 中的 location 放进 server，避免直接下载 /backend、db.json、install.lock。502/503/504 使用同文件里的 error_page，返回网站根 backend-unavailable.html
  4. 需要 HTTPS 时在面板申请证书
  5. 进程守护：启动命令 ${data}/start.sh ，运行目录 ${data} 。说明见 ${data}/baota-guardian.txt
     在线更新会自己退出并交给守护拉起。手工停进程或 --no-start 升级前，先在守护里停止，否则进程会被立刻拉起

运行数据目录：${data}
升级会保留：db.json、install.lock、jwt.secret、plugins、home-templates、software-source-cache、updates、app-releases、logs、advertisement-images、source-packages，以及 baota.env
EOF
}

# 只撤本次新建的站点和库。没有标记的目录不会删。失败时打印手工步骤。
baota_oneclick_rollback() {
  local domain="$1"
  [[ -n "$domain" ]] || return 0
  if [[ "$BAOTA_CREATED_SITE" != "1" && "$BAOTA_CREATED_DB" != "1" ]]; then
    return 0
  fi
  baota_warn "正在撤掉本次新建的站点或数据库，不会动其它站点"
  baota_drop_our_supervisor || true
  local args=(rollback --domain "$domain" --path "$BAOTA_SITE_ROOT")
  if [[ "$BAOTA_CREATED_SITE" == "1" ]]; then
    args+=(--remove-site)
  fi
  if [[ "$BAOTA_CREATED_DB" == "1" && -n "$BAOTA_DB_NAME" ]]; then
    args+=(--remove-db --db-name "$BAOTA_DB_NAME")
  fi
  BAOTA_CREATED_SITE=0
  BAOTA_CREATED_DB=0
  baota_panel_run "${args[@]}" || baota_warn "自动回滚没有全部完成。请按上面的手工说明处理，不要删除其它站点或数据库。"
}

baota_oneclick_exit() {
  local code=$?
  if [[ "$code" -ne 0 && "${BAOTA_ONECLICK_ROLLBACK:-}" == "1" ]]; then
    BAOTA_ONECLICK_ROLLBACK=0
    baota_oneclick_rollback "${BAOTA_ONECLICK_DOMAIN:-}" || true
  fi
  baota_cleanup
}

# 向导接口不带 Origin。返回 0 表示 code=200，2 表示已有数据被拒绝，1 表示其它失败。
baota_post_install() {
  local port="$1" path="$2" body="$3" tmp http code
  tmp="$(mktemp)"
  http="$(curl -sS --max-time 90 -o "$tmp" -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    --data-binary "$body" \
    "http://127.0.0.1:${port}${path}" || true)"
  code="$(python3 -c 'import json,sys
try:
    print(json.load(open(sys.argv[1], encoding="utf-8")).get("code", ""))
except Exception:
    print("")
' "$tmp")"
  baota_info "接口 ${path} 返回 HTTP ${http} code ${code}"
  if [[ "$code" == "403" || "$http" == "403" ]]; then
    rm -f "$tmp"
    return 2
  fi
  if [[ "$code" != "200" ]]; then
    cat "$tmp" >&2 || true
    rm -f "$tmp"
    return 1
  fi
  rm -f "$tmp"
  return 0
}

baota_wizard_body() {
  DB_NAME="$1" DB_USER="$2" DB_PASS="$3" ADMIN_USER="${4:-}" ADMIN_PASS="${5:-}" python3 - <<'PY'
import json
import os
body = {
    "host": "127.0.0.1",
    "port": "3306",
    "database": os.environ["DB_NAME"],
    "username": os.environ["DB_USER"],
    "password": os.environ["DB_PASS"],
}
if os.environ.get("ADMIN_USER"):
    body["adminUsername"] = os.environ["ADMIN_USER"]
    body["adminPassword"] = os.environ["ADMIN_PASS"]
print(json.dumps(body, ensure_ascii=False))
PY
}

# 后端已经健康才调用。有锁或库里已有业务数据就跳过，不覆盖管理员。
# 成功时打印网址、账号和数据库，并写到仅 root 可读的文件。
baota_run_wizard() {
  local domain="$1" port="$2" db_name="$3" db_user="$4" db_pass="$5" scheme="${6:-http}"
  local data body admin_pass dest status
  data="$(baota_data_dir)"
  if [[ -f "$data/install.lock" ]]; then
    baota_info "已有 install.lock，跳过安装向导，不覆盖管理员和业务数据。"
    return 0
  fi
  status="$(baota_health_body "http://127.0.0.1:${port}/api/install/status")"
  if printf '%s' "$status" | grep -q '"installed":true'; then
    baota_info "站点已经安装，跳过安装向导，不覆盖。"
    return 0
  fi
  body="$(baota_wizard_body "$db_name" "$db_user" "$db_pass")"
  baota_post_install "$port" "/api/install/test-db" "$body" || {
    baota_warn "数据库连接测试没有通过。站点已保留。请用浏览器打开 http://${domain}/ ，在安装向导里填写下面的数据库信息。"
    baota_print_db_hint "$domain" "$port" "$db_name" "$db_user" "$db_pass"
    return 0
  }
  set +e
  baota_post_install "$port" "/api/install/init-tables" "$body"
  local init_code=$?
  set -e
  if [[ "$init_code" -ne 0 ]]; then
    if [[ "$init_code" -eq 2 ]]; then
      baota_warn "数据库已有业务数据，已跳过建表和管理员创建，没有覆盖。"
    else
      baota_warn "初始化数据表没有成功。请用浏览器打开 http://${domain}/ 继续安装向导。数据库信息如下。"
    fi
    baota_print_db_hint "$domain" "$port" "$db_name" "$db_user" "$db_pass"
    return 0
  fi
  admin_pass="$(baota_random_digits 8)"
  [[ "$admin_pass" =~ ^[0-9]{8}$ ]] || baota_die "没有生成 8 位数字管理员密码，已停止。没有写入凭据。"
  body="$(baota_wizard_body "$db_name" "$db_user" "$db_pass" "admin" "$admin_pass")"
  set +e
  baota_post_install "$port" "/api/install/create-admin" "$body"
  local admin_code=$?
  set -e
  if [[ "$admin_code" -ne 0 ]]; then
    if [[ "$admin_code" -eq 2 ]]; then
      baota_warn "已有管理员或业务数据，没有覆盖管理员密码。"
    else
      baota_warn "创建管理员没有成功。请用浏览器打开 http://${domain}/ 在安装向导里创建管理员。数据库信息如下。"
    fi
    baota_print_db_hint "$domain" "$port" "$db_name" "$db_user" "$db_pass"
    return 0
  fi
  scheme="http"
  dest="$(baota_write_credentials "$domain" "$port" "$db_name" "$db_user" "$db_pass" "$admin_pass" "$scheme")"
  BAOTA_CREDENTIALS_FILE="$dest"
  BAOTA_ADMIN_PASS="$admin_pass"
  baota_print_credentials "$domain" "$port" "$db_name" "$db_user" "$db_pass" "$admin_pass" "$scheme" "$dest"
}

baota_print_db_hint() {
  local domain="$1" port="$2" db_name="$3" db_user="$4" db_pass="$5"
  cat <<EOF
网址: http://${domain}/
后端端口: ${port}
数据库主机: 127.0.0.1
数据库端口: 3306
数据库名: ${db_name}
数据库用户: ${db_user}
数据库密码: ${db_pass}
EOF
}

baota_write_credentials() {
  local domain="$1" port="$2" db_name="$3" db_user="$4" db_pass="$5" admin_pass="$6" scheme="$7"
  local dest data
  data="$(baota_data_dir)"
  dest="/root/auth-pro-${domain}.txt"
  if [[ ! -d /root || ! -w /root ]]; then
    dest="${data}/install-credentials.txt"
  fi
  umask 077
  cat > "$dest" <<EOF
管理后台: ${scheme}://${domain}/admin
管理员账号: admin
管理员密码: ${admin_pass}
登录后请在后台修改密码
网址: ${scheme}://${domain}/
后端端口: ${port}
数据库主机: 127.0.0.1
数据库端口: 3306
数据库名: ${db_name}
数据库用户: ${db_user}
数据库密码: ${db_pass}
EOF
  chmod 600 "$dest" || baota_warn "无法把 ${dest} 收紧为仅所有者可读"
  printf '%s\n' "$dest"
}

baota_print_credentials() {
  local domain="$1" port="$2" db_name="$3" db_user="$4" db_pass="$5" admin_pass="$6" scheme="$7" dest="$8"
  cat <<EOF

安装完成。
管理后台: ${scheme}://${domain}/admin
管理员账号: admin
管理员密码: ${admin_pass}
登录后请在后台修改密码
网址: ${scheme}://${domain}/
后端端口: ${port}
数据库主机: 127.0.0.1
数据库端口: 3306
数据库名: ${db_name}
数据库用户: ${db_user}
数据库密码: ${db_pass}
凭据文件: ${dest}
EOF
}

# 安装向导创建的是 admins 表里的管理员。用户端首页查的是 users 表，用这份密码会提示账号或密码错误。
# 凭据文件只更新管理员密码这一段，数据库密码原样保留。
baota_store_admin_password() {
  local domain="$1" admin_user="$2" admin_pass="$3" dest scheme
  [[ "$admin_pass" =~ ^[0-9]{8}$ ]] || baota_die "拒绝把非 8 位数字写入凭据文件"
  domain="$(printf '%s' "$domain" | tr '[:upper:]' '[:lower:]')"
  dest="/root/auth-pro-${domain}.txt"
  if [[ ! -d /root || ! -w /root ]]; then
    baota_warn "无法写入 ${dest}。新密码只打印在上面，请立刻抄下。"
    return 0
  fi
  scheme="http"
  if [[ -f "$dest" ]] && grep -q '^网址: https://' "$dest"; then
    scheme="https"
  fi
  umask 077
  DEST="$dest" ADMIN_USER="$admin_user" ADMIN_PASS="$admin_pass" SCHEME="$scheme" DOMAIN="$domain" python3 - <<'PY'
import os
path = os.environ["DEST"]
user = os.environ["ADMIN_USER"]
password = os.environ["ADMIN_PASS"]
scheme = os.environ["SCHEME"]
domain = os.environ["DOMAIN"]
hint = "登录后请在后台修改密码"
backend = "管理后台: %s://%s/admin" % (scheme, domain)
lines = []
if os.path.exists(path):
    with open(path, encoding="utf-8") as handle:
        lines = handle.read().splitlines()

def upsert(prefix, value, rows):
    found = False
    output = []
    for line in rows:
        if line.startswith(prefix):
            output.append(value)
            found = True
        else:
            output.append(line)
    if not found:
        output.append(value)
    return output

if not lines:
    lines = [backend, "管理员账号: %s" % user, "管理员密码: %s" % password, hint]
else:
    lines = upsert("管理后台:", backend, lines)
    lines = upsert("管理员账号:", "管理员账号: %s" % user, lines)
    lines = upsert("管理员密码:", "管理员密码: %s" % password, lines)
    if hint not in lines:
        lines.append(hint)
temporary = path + ".tmp"
with open(temporary, "w", encoding="utf-8") as handle:
    handle.write("\n".join(lines) + "\n")
os.chmod(temporary, 0o600)
os.replace(temporary, path)
os.chmod(path, 0o600)
PY
  chmod 600 "$dest" || baota_warn "无法把 ${dest} 收紧为仅所有者可读"
  printf '%s\n' "$dest"
}

# 面板已安装时的全新安装：建站、建库、反代、守护、向导。任一步失败只撤本次新建的对象。
baota_oneclick_install() {
  local domain port data db_name db_user db_pass snippet scheme cert_log
  domain="$(baota_site_domain)"
  [[ "$domain" =~ ^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$ ]] || baota_die "域名不正确：${domain}"
  BAOTA_ONECLICK_DOMAIN="$domain"
  BAOTA_CREATED_SITE=0
  BAOTA_CREATED_DB=0
  BAOTA_ONECLICK_ROLLBACK=1
  trap baota_oneclick_exit EXIT
  baota_panel_run preflight
  baota_select_install_port
  port="$BAOTA_SELECTED_PORT"
  baota_info "使用后端端口 ${port}"
  baota_panel_run check-site --domain "$domain" --path "$BAOTA_SITE_ROOT"
  db_name="ap$(baota_random_alnum 8 lower)"
  db_user="$db_name"
  db_pass="$(baota_random_alnum 20 mixed)"
  BAOTA_DB_NAME="$db_name"
  baota_panel_run check-database --name "$db_name" --user "$db_user"
  baota_resolve_start_flag
  baota_confirm "全新安装到 ${BAOTA_SITE_ROOT} ，域名 ${domain} ，端口 ${port}"
  # 先记上标记。AddSite 若创建了站点再失败，退出钩子会按站点名撤掉，没有标记文件的目录不会删。
  BAOTA_CREATED_SITE=1
  baota_panel_run add-site --domain "$domain" --path "$BAOTA_SITE_ROOT"
  BAOTA_CREATED_DB=1
  baota_panel_run add-database --name "$db_name" --user "$db_user" --password "$db_pass"
  BAOTA_SITE_ROOT="$(cd "$BAOTA_SITE_ROOT" && pwd -P)"
  BAOTA_DATA_DIR_RESOLVED=""
  baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
  baota_prepare_payload
  if [[ -e "$BAOTA_SITE_ROOT/index.html" || -d "$BAOTA_SITE_ROOT/assets" || -e "$BAOTA_SITE_ROOT/backend/auth_pro" ]]; then
    baota_prepare_backup_dir
  fi
  BAOTA_PANEL_INSTALL=1
  baota_apply_payload
  baota_tighten_secrets
  if id www >/dev/null 2>&1; then
    # 宝塔给 .user.ini 加不可变属性。先去掉，交给 www 后再加回去，避免整次 chown 被这一份文件打断。
    if [[ -f "$BAOTA_SITE_ROOT/.user.ini" ]]; then
      chattr -i "$BAOTA_SITE_ROOT/.user.ini" 2>/dev/null || true
    fi
    if ! chown -R www:www "$BAOTA_SITE_ROOT"; then
      baota_warn "无法把网站目录交给 www。进程守护若以 www 运行，请手工修正目录所有者。"
    fi
    if [[ -f "$BAOTA_SITE_ROOT/.user.ini" ]]; then
      chattr +i "$BAOTA_SITE_ROOT/.user.ini" 2>/dev/null || true
    fi
    baota_tighten_secrets
  fi
  snippet="$(baota_data_dir)/baota-nginx.snippet.conf"
  baota_panel_run proxy --domain "$domain" --port "$port" --snippet "$snippet"
  if ! baota_panel_run supervisor --domain "$domain" --command "$(baota_data_dir)/start.sh" --workdir "$(baota_data_dir)"; then
    baota_guardian_manual
    baota_die "进程守护没有登记成功。请按上面的手工说明只添加本站点。本次不能当作安装完成。"
  fi
  if [[ "$BAOTA_PANEL_RESULT" != "ok" ]]; then
    baota_guardian_manual
    baota_die "进程守护没有进入运行状态（结果 ${BAOTA_PANEL_RESULT:-空}）。请按上面的手工说明处理。本次不能当作安装完成。"
  fi
  if [[ -n "$BAOTA_PANEL_PROGRAM" ]]; then
    BAOTA_SUP_PROGRAM="$BAOTA_PANEL_PROGRAM"
  fi
  if [[ -n "$BAOTA_PANEL_SUP_CONF" ]]; then
    BAOTA_SUP_CONF="$BAOTA_PANEL_SUP_CONF"
  fi
  if [[ "$BAOTA_START" == "1" ]]; then
    baota_start_backend
    baota_wait_listening_health || baota_die "后端健康检查未通过。已尝试撤掉本次新建的站点和数据库。请查看 $(baota_data_dir)/logs/auto_pro.log"
    baota_verify_guardian || baota_die "进程守护核验没有通过。本次不能当作安装完成。"
  else
    baota_find_supervisor || true
    baota_supervisor_stop_ours
    baota_warn "已指定不启动。进程守护已登记并停在停止状态，没有 nohup，也没有创建管理员。站点、数据库和反代已就绪。"
  fi
  scheme="http"
  # 证书失败时的那一句中文和日志路径由辅助脚本打印。这里只根据捕获到的结果决定网址用 http 还是 https。
  cert_log="$(baota_data_dir)/logs/baota-install.log"
  if baota_panel_run cert --domain "$domain" --webroot "$BAOTA_SITE_ROOT" --log "$cert_log"; then
    if [[ "$BAOTA_PANEL_RESULT" == "https" ]]; then
      scheme="https"
      baota_info "证书已申请，请使用 https://${domain}/"
    fi
  else
    baota_warn "证书申请步骤没有完成，站点保持 HTTP。"
  fi
  if [[ "$BAOTA_START" == "1" ]]; then
    baota_run_wizard "$domain" "$port" "$db_name" "$db_user" "$db_pass" "$scheme"
  else
    baota_print_db_hint "$domain" "$port" "$db_name" "$db_user" "$db_pass"
  fi
  BAOTA_ONECLICK_ROLLBACK=0
  baota_info "一条命令安装结束。"
}

# 已装站点的管理员密码只能由 root 在本机重设。程序从本站 db.json 连库，不监听端口。
# 不替换网站文件，不改 Nginx，也不改数据库密码和其它业务数据。
baota_reset_admin_password() {
  local data bin out errfile admin_user admin_pass dest domain
  [[ "$(id -u)" -eq 0 ]] || baota_die "只有 root 能在服务器本机重设管理员密码。没有改动数据库。"
  if [[ "$BAOTA_REPAIR_GUARDIAN" == "1" ]]; then
    baota_die "重设管理员密码和修复进程守护请分开执行"
  fi
  if [[ -z "$BAOTA_SITE_ROOT" ]]; then
    baota_die "重设管理员密码需要 --site-root"
  fi
  BAOTA_SITE_ROOT="${BAOTA_SITE_ROOT%/}"
  baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
  baota_reject_dotdot "$BAOTA_SITE_ROOT" "网站根目录"
  data="$(baota_data_dir)"
  [[ -f "$data/install.lock" ]] || baota_die "没有 ${data}/install.lock。重设命令只处理已经装好的站点，没有改动网站文件。"
  [[ -f "$data/db.json" ]] || baota_die "没有 ${data}/db.json，无法连接本站数据库。没有改动网站文件。"
  bin="${BAOTA_RESET_BINARY:-$data/auth_pro}"
  [[ -f "$bin" ]] || baota_die "找不到 ${bin}。没有改动数据库和网站文件。"
  domain="$(baota_site_domain)"
  baota_info "重设 ${BAOTA_SITE_ROOT} 的管理员密码。不改网站文件、Nginx 和数据库密码。"
  errfile="$(mktemp)"
  set +e
  out="$(AUTO_PRO_DATA_DIR="$data" "$bin" reset-admin-password 2>"$errfile")"
  local code=$?
  set -e
  if [[ "$code" -ne 0 ]]; then
    cat "$errfile" >&2 || true
    rm -f "$errfile"
    baota_die "重设管理员密码失败。没有改动网站文件和 Nginx。"
  fi
  rm -f "$errfile"
  printf '%s\n' "$out"
  admin_user="$(printf '%s\n' "$out" | sed -n 's/^管理员账号: //p' | head -n 1)"
  admin_pass="$(printf '%s\n' "$out" | sed -n 's/^管理员密码: //p' | head -n 1)"
  if [[ ! "$admin_pass" =~ ^[0-9]{8}$ || -z "$admin_user" ]]; then
    baota_die "上面如果已经打印了新密码，请立刻抄下。凭据文件没有更新。"
  fi
  dest="$(baota_store_admin_password "$domain" "$admin_user" "$admin_pass")"
  baota_info "管理后台: http://${domain}/admin"
  if [[ -n "$dest" ]]; then
    baota_info "凭据文件: ${dest}"
  fi
  baota_info "登录后请在后台修改密码。没有改动网站文件、Nginx、数据库密码和其它站点。"
}

# 给 1.7.5 已装好、进程脱管的站点用。只停本站脱管进程并重新登记守护，不改数据库、网站文件和 Nginx。
baota_repair_guardian() {
  local data port command sum_before
  if [[ -z "$BAOTA_SITE_ROOT" ]]; then
    baota_die "修复进程守护需要 --site-root"
  fi
  BAOTA_SITE_ROOT="${BAOTA_SITE_ROOT%/}"
  baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
  baota_reject_dotdot "$BAOTA_SITE_ROOT" "网站根目录"
  data="$(baota_data_dir)"
  [[ -f "$data/install.lock" ]] || baota_die "没有 ${data}/install.lock。修复命令只处理已经装好的站点，不会新建网站。"
  [[ -f "$data/start.sh" && -f "$data/auth_pro" ]] || baota_die "缺少 ${data}/start.sh 或 auth_pro。没有改动网站文件。"
  baota_panel_available || baota_die "未检测到宝塔面板或 baota-panel.py。没有改动网站文件。"
  BAOTA_START=1
  port="$(baota_effective_port)"
  command="${data}/start.sh"
  if [[ -f "$BAOTA_SITE_ROOT/index.html" ]]; then
    sum_before="$(sha256sum "$BAOTA_SITE_ROOT/index.html" | awk '{print $1}')"
  fi
  baota_info "修复 ${BAOTA_SITE_ROOT} 的进程守护，端口 ${port}。不改数据库、网站文件和 Nginx。"
  baota_find_supervisor || true
  baota_supervisor_stop_ours
  if baota_port_is_open "$port"; then
    baota_stop_and_reclaim "$port" "$BAOTA_SITE_ROOT"
  fi
  if baota_port_is_open "$port"; then
    baota_guardian_manual
    baota_die "端口 ${port} 仍被占用，没有登记新的守护，也没有改网站文件。"
  fi
  if ! baota_panel_run supervisor --repair --domain "$(baota_site_domain)" --command "$command" --workdir "$data"; then
    baota_guardian_manual
    baota_die "进程守护没有登记成功。网站文件和数据库没有改动。"
  fi
  if [[ "$BAOTA_PANEL_RESULT" != "ok" ]]; then
    baota_guardian_manual
    baota_die "进程守护没有进入运行状态（结果 ${BAOTA_PANEL_RESULT:-空}）。网站文件和数据库没有改动。"
  fi
  if [[ -n "$BAOTA_PANEL_PROGRAM" ]]; then
    BAOTA_SUP_PROGRAM="$BAOTA_PANEL_PROGRAM"
  fi
  if [[ -n "$BAOTA_PANEL_SUP_CONF" ]]; then
    BAOTA_SUP_CONF="$BAOTA_PANEL_SUP_CONF"
  fi
  baota_wait_listening_health || {
    baota_guardian_manual
    baota_die "守护已登记，但健康检查未通过。请查看 ${data}/logs/auto_pro.log 。没有改动数据库和网站文件。"
  }
  baota_verify_guardian || baota_die "进程守护核验没有通过。没有改动数据库和网站文件。"
  if [[ -n "${sum_before:-}" ]]; then
    local sum_after
    sum_after="$(sha256sum "$BAOTA_SITE_ROOT/index.html" | awk '{print $1}')"
    [[ "$sum_before" == "$sum_after" ]] || baota_die "修复过程中网站首页被改动，已停止。请检查 ${BAOTA_SITE_ROOT}/index.html 。"
  fi
  baota_info "进程守护已修复。面板列表里有本站点，状态为 RUNNING。没有改动数据库、网站文件和 Nginx，也没有动其它站点的守护项。"
}

baota_cmd_install() {
  baota_parse_args "$@"
  if [[ "$BAOTA_RESET_ADMIN" == "1" && "$BAOTA_REPAIR_GUARDIAN" == "1" ]]; then
    baota_die "重设管理员密码和修复进程守护请分开执行"
  fi
  if [[ "$BAOTA_RESET_ADMIN" == "1" ]]; then
    baota_reset_admin_password
    return
  fi
  if [[ "$BAOTA_REPAIR_GUARDIAN" == "1" ]]; then
    baota_repair_guardian
    return
  fi
  if [[ -z "$BAOTA_SITE_ROOT" ]]; then
    baota_resolve_site_root
  else
    BAOTA_SITE_ROOT="${BAOTA_SITE_ROOT%/}"
    [[ -n "$BAOTA_SITE_ROOT" ]] || baota_die "网站根目录不能为空"
    baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
    baota_reject_dotdot "$BAOTA_SITE_ROOT" "网站根目录"
  fi
  if [[ "$BAOTA_DRY_RUN" != "1" ]] && baota_panel_available; then
    baota_oneclick_install
    return
  fi
  if [[ "${AUTH_PRO_ONECLICK:-}" == "1" ]]; then
    baota_die "未检测到宝塔面板，或安装包里没有面板辅助脚本。请先安装宝塔面板，并在软件商店安装 Nginx 和 MySQL 后再执行。脚本不会替你安装 MySQL。"
  fi
  baota_resolve_site_root
  baota_resolve_start_flag
  local data port
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  if [[ -f "$data/install.lock" ]]; then
    baota_die "检测到 ${data}/install.lock ，站点已经安装。请改用 curl -fsSL https://auth.maizll.com/install.sh | bash -s -- upgrade <域名> ，以免覆盖运行数据。"
  fi
  if [[ -f "$data/db.json" ]]; then
    baota_warn "已存在 ${data}/db.json 。安装不会删除它；若向导已经完成，请改用升级脚本。"
  fi
  baota_prepare_payload
  baota_confirm "全新安装到 ${BAOTA_SITE_ROOT} ，数据目录 ${data} ，端口 ${port}"
  local must_clear=0
  if [[ "$BAOTA_START" == "1" ]]; then
    must_clear=1
  fi
  baota_ensure_port_available "$port" "$BAOTA_SITE_ROOT" "$must_clear"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_apply_payload
    baota_info "以上为预演，没有改动网站文件。"
    return 0
  fi
  if [[ -f "$BAOTA_SITE_ROOT/index.html" || -d "$BAOTA_SITE_ROOT/assets" || -f "$BAOTA_SITE_ROOT/backend/auth_pro" ]]; then
    baota_prepare_backup_dir
    baota_copy_durable_into_backup
  fi
  baota_apply_payload
  baota_tighten_secrets
  baota_start_backend
  baota_info "全新安装的文件已就绪。"
  baota_print_manual_steps
}

# 父进程链上有 supervisord，或带 INVOCATION_ID 且最终回到 PID 1（systemd 服务）。
# 直接父进程为 1 且没有 INVOCATION_ID 的是孤儿，守护不会把它拉起来。
baota_guardian_owns_pid() {
  local pid="$1" current parent comm i
  [[ "$pid" =~ ^[0-9]+$ && "$pid" -gt 1 ]] || return 1
  current="$pid"
  for i in 1 2 3 4 5 6 7 8; do
    parent="$(awk '/^PPid:/ {print $2}' "/proc/$current/status" 2>/dev/null || true)"
    [[ "$parent" =~ ^[0-9]+$ ]] || return 1
    if [[ "$parent" == "1" ]]; then
      if tr '\0' '\n' < "/proc/$pid/environ" 2>/dev/null | grep -q '^INVOCATION_ID='; then
        return 0
      fi
      return 1
    fi
    comm="$(tr -d ' \n' < "/proc/$parent/comm" 2>/dev/null || true)"
    case "$comm" in
      supervisord|supervisor|systemd) return 0 ;;
    esac
    if tr '\0' ' ' < "/proc/$parent/cmdline" 2>/dev/null | grep -Fq 'supervisord'; then
      return 0
    fi
    current="$parent"
  done
  return 1
}

baota_find_guardian_owned_pid() {
  local port="$1" site="$2" pid root
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    root="$(baota_our_root "$pid" "$site" || true)"
    [[ -n "$root" ]] || continue
    if baota_guardian_owns_pid "$root"; then
      printf '%s\n' "$root"
      return 0
    fi
  done < <(baota_pids_for_port "$port")
  return 1
}

baota_wait_listening_health() {
  local port url timeout i health
  port="$(baota_effective_port)"
  url="http://127.0.0.1:${port}/api/install/status"
  timeout="${AUTH_PRO_HEALTH_TIMEOUT:-30}"
  for i in $(seq 1 "$timeout"); do
    health="$(baota_health_body "$url")"
    if [[ -n "$health" ]]; then
      baota_info "后端已响应 ${url}"
      return 0
    fi
    sleep 1
  done
  return 1
}

# 守护会在进程退出后立刻拉起。先换文件，再只结束本站这一个 PID。
baota_upgrade_under_guardian() {
  local pid="$1" data manifest again
  data="$(baota_data_dir)"
  if [[ "$BAOTA_START" != "1" ]]; then
    baota_die "端口由进程守护托管（PID ${pid}）。守护会在进程退出后立刻拉起，--no-start 无法让它保持停止。请先在宝塔进程守护里停止本站点，或改用 --start，由脚本替换文件后交给守护拉起。"
  fi
  baota_info "进程守护正在托管本站 PID ${pid}。不停止守护。先替换文件，再只结束这个进程，由守护拉起。"
  baota_prepare_backup_dir
  manifest="$BAOTA_BACKUP_DIR/durable.sha256"
  baota_durable_manifest "$data" "$manifest"
  baota_copy_durable_into_backup
  baota_mysql_backup
  baota_apply_payload
  baota_verify_manifest "$manifest"
  baota_tighten_secrets
  baota_signal_pid "$pid"
  if baota_wait_listening_health; then
    baota_prune_after_health || true
    baota_info "升级完成。备份在 ${BAOTA_BACKUP_DIR}"
    baota_print_manual_steps
    return 0
  fi
  baota_warn "新版本没有通过健康检查，正在回滚程序文件"
  baota_rollback_programs
  again="$(baota_find_guardian_owned_pid "$(baota_effective_port)" "$BAOTA_SITE_ROOT" || true)"
  if [[ -n "$again" ]]; then
    baota_signal_pid "$again"
  fi
  if baota_wait_listening_health; then
    baota_die "新版本没有通过健康检查，已回滚并交给进程守护拉起旧版本。备份在 ${BAOTA_BACKUP_DIR}"
  fi
  baota_die "新版本没有通过健康检查，已回滚程序文件，但旧版本没有在时限内恢复。请查看进程守护日志。备份在 ${BAOTA_BACKUP_DIR}"
}

baota_cmd_upgrade() {
  baota_parse_args "$@"
  baota_resolve_site_root
  baota_resolve_start_flag
  local data port manifest
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  if [[ ! -f "$data/install.lock" && ! -f "$data/db.json" ]]; then
    baota_die "在 ${data} 未找到 db.json 或 install.lock 。这像是新站点，请改用 curl -fsSL https://auth.maizll.com/install.sh | bash -s -- <域名> 。"
  fi
  if [[ ! -f "$data/install.lock" ]]; then
    baota_warn "没有 install.lock ，仍会按升级处理并保留已有 db.json 。若安装向导还没做完，请先完成向导。"
  fi
  baota_prepare_payload
  baota_confirm "升级 ${BAOTA_SITE_ROOT} ，保留 ${data} 中的运行数据，端口 ${port}"
  if [[ "$BAOTA_DRY_RUN" != "1" ]]; then
    baota_migrate_old_backups || true
  fi
  local guardian_pid=""
  if [[ "$BAOTA_DRY_RUN" != "1" ]] && baota_port_is_open "$port"; then
    guardian_pid="$(baota_find_guardian_owned_pid "$port" "$BAOTA_SITE_ROOT" || true)"
  fi
  if [[ -n "$guardian_pid" ]]; then
    baota_upgrade_under_guardian "$guardian_pid"
    return 0
  fi
  baota_ensure_port_available "$port" "$BAOTA_SITE_ROOT" "1"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_prepare_backup_dir
    baota_copy_durable_into_backup
    baota_mysql_backup
    baota_apply_payload
    baota_info "以上为预演，没有改动网站文件，也没有导出数据库。"
    return 0
  fi
  baota_prepare_backup_dir
  manifest="$BAOTA_BACKUP_DIR/durable.sha256"
  baota_durable_manifest "$data" "$manifest"
  baota_copy_durable_into_backup
  baota_mysql_backup
  baota_apply_payload
  baota_verify_manifest "$manifest"
  baota_tighten_secrets
  baota_start_backend
  if [[ "$BAOTA_START" == "1" ]]; then
    baota_prune_after_health || true
  fi
  baota_info "升级完成。备份在 ${BAOTA_BACKUP_DIR}"
  baota_print_manual_steps
}

install_main() {
  local cmd="install"
  if [[ $# -gt 0 ]]; then
    case "$1" in
      upgrade|install|repair|reset-admin-password)
        cmd="$1"
        shift
        ;;
    esac
  fi
  case "$cmd" in
    repair)
      set -- --repair-guardian "$@"
      ;;
    reset-admin-password)
      set -- --reset-admin-password "$@"
      ;;
  esac
  if install_should_download "$@"; then
    install_download_and_continue "$cmd" "$@"
    return
  fi
  if [[ "$cmd" == "upgrade" ]]; then
    BAOTA_ACTION="upgrade"
    baota_cmd_upgrade "$@"
    return
  fi
  if [[ $# -eq 0 ]]; then
    baota_print_help >&2
    install_die "请在命令末尾写上域名，例如：bash -s -- example.com"
  fi
  BAOTA_ACTION="install"
  baota_cmd_install "$@"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  install_main "$@"
fi
