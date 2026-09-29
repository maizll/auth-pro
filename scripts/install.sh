#!/usr/bin/env bash
# 官网一条命令安装。站点根路径 /install.sh 下发的就是当前已发布安装包里的这份脚本。
# 只从官网更新接口取客户端包，核对 SHA256 和签名后调用同包里的 baota-install.sh。
# 解压、安装、start.sh 和进程守护说明都在 baota-install.sh / baota-lib.sh，这里不另写一套。
set -euo pipefail

install_die() {
  printf '[错误] %s\n' "$1" >&2
  exit 1
}

install_info() {
  printf '[信息] %s\n' "$1"
}

install_print_help() {
  cat <<'EOF'
用法：
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- 域名
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- 域名 --port 端口 --site-root 目录

从官网下载已发布的安装包，核对 SHA256 和签名后安装。
需要本机已经装好宝塔面板，并且软件商店里已经安装 Nginx 和 MySQL。脚本不会替你安装 MySQL。
网站目录默认是 /www/wwwroot/域名。未写 --port 时从 19127 起自动找空闲端口。
装完后会创建管理员并打印账号。已经有 backend/install.lock 时会停下来，请改用升级，不会覆盖。

选项：
  --site-root DIR   网站根。不写则使用 /www/wwwroot/域名
  --port PORT       后端端口。不写则自动选择空闲端口
  --start           安装后启动并完成安装向导（默认）
  --no-start        只建站放文件，不启动、不创建管理员
  --repair-guardian 修复已经装好的站点：停掉脱管进程，重新登记进程守护并拉起。不改数据库、网站文件和 Nginx
  --reset-admin-password
                    本机 root 重设已装站点的管理员密码，打印新的 8 位数字密码。不改网站文件、Nginx 和数据库密码
  -h, --help        显示本说明

示例：
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- example.com
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- --repair-guardian example.com
  curl -fsSL https://auth.maizll.com/install.sh | bash -s -- --reset-admin-password example.com
EOF
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

DOMAIN=""
SITE_ROOT=""
PORT=""
START_FLAG="--start"
REPAIR_GUARDIAN=0
RESET_ADMIN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      install_print_help
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
  install_print_help >&2
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

# 先看安装锁，避免为已安装站点下载整包。修复守护和重设密码都不覆盖网站文件。
if [[ "$REPAIR_GUARDIAN" == "1" && "$RESET_ADMIN" == "1" ]]; then
  install_die "请分开执行 --repair-guardian 和 --reset-admin-password"
fi
if [[ "$RESET_ADMIN" == "1" && "$(id -u)" -ne 0 ]]; then
  install_die "只有 root 能在服务器本机重设管理员密码"
fi
if [[ -f "$SITE_ROOT/backend/install.lock" && "$REPAIR_GUARDIAN" != "1" && "$RESET_ADMIN" != "1" ]]; then
  install_die "检测到 ${SITE_ROOT}/backend/install.lock ，站点已经安装。请改用 baota-upgrade.sh ，或在后台使用「在线更新」，以免覆盖运行数据。若只是进程没进宝塔进程守护，请改用 --repair-guardian。若要重设管理员密码，请改用 --reset-admin-password。"
fi
if [[ "$REPAIR_GUARDIAN" == "1" && ! -f "$SITE_ROOT/backend/install.lock" ]]; then
  install_die "没有 ${SITE_ROOT}/backend/install.lock 。修复命令只处理已经装好的站点，请去掉 --repair-guardian 再安装。"
fi
if [[ "$RESET_ADMIN" == "1" && ! -f "$SITE_ROOT/backend/install.lock" ]]; then
  install_die "没有 ${SITE_ROOT}/backend/install.lock 。重设密码只处理已经装好的站点，请去掉 --reset-admin-password 再安装。"
fi

# 官网地址写死。不能用环境变量、参数或配置文件改掉。
command -v curl >/dev/null 2>&1 || install_die "缺少 curl，无法下载安装包"
command -v python3 >/dev/null 2>&1 || install_die "缺少 python3，无法读取版本清单"
command -v tar >/dev/null 2>&1 || install_die "缺少 tar，无法解开安装包"

install_info "正在从官网获取最新版本"
# -q 放在最前，不读 curl 的配置文件。协议只允许 https。
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
trap 'rm -rf "$WORKDIR"' EXIT
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

# 发布包成员可能是 baota-install.sh，也可能是 ./baota-install.sh。两种都认。
LISTING="$(tar -tzf "$PKG_FILE")"
for name in baota-install.sh baota-upgrade.sh baota-lib.sh baota-panel.py guardian-start.sh; do
  if printf '%s\n' "$LISTING" | grep -qx "./${name}"; then
    tar -xzf "$PKG_FILE" -C "$WORKDIR" "./${name}"
  elif printf '%s\n' "$LISTING" | grep -qx "${name}"; then
    tar -xzf "$PKG_FILE" -C "$WORKDIR" "${name}"
  else
    install_die "安装包里缺少 ${name}"
  fi
done
[[ -f "$WORKDIR/baota-install.sh" && -f "$WORKDIR/baota-lib.sh" ]] || install_die "安装包里缺少安装脚本"
chmod 755 "$WORKDIR/baota-install.sh" "$WORKDIR/baota-upgrade.sh" "$WORKDIR/guardian-start.sh"

if [[ -n "$DOMAIN" ]]; then
  export AUTH_PRO_PUBLIC_HOST="$DOMAIN"
fi

if [[ "$RESET_ADMIN" == "1" ]]; then
  if printf '%s\n' "$LISTING" | grep -qx "./backend/auth_pro"; then
    tar -xzf "$PKG_FILE" -C "$WORKDIR" "./backend/auth_pro"
  elif printf '%s\n' "$LISTING" | grep -qx "backend/auth_pro"; then
    tar -xzf "$PKG_FILE" -C "$WORKDIR" "backend/auth_pro"
  else
    install_die "安装包里缺少 backend/auth_pro，无法重设管理员密码"
  fi
  [[ -f "$WORKDIR/backend/auth_pro" ]] || install_die "安装包里缺少 backend/auth_pro，无法重设管理员密码"
  chmod 755 "$WORKDIR/backend/auth_pro"
  install_info "开始重设 ${SITE_ROOT} 的管理员密码，不覆盖网站文件和 Nginx"
  export AUTH_PRO_ONECLICK=1
  bash "$WORKDIR/baota-install.sh" --reset-admin-password --yes --site-root "$SITE_ROOT" --reset-binary "$WORKDIR/backend/auth_pro"
  exit 0
fi

if [[ "$REPAIR_GUARDIAN" == "1" ]]; then
  install_info "开始修复 ${SITE_ROOT} 的进程守护，不覆盖网站文件和数据库"
  export AUTH_PRO_ONECLICK=1
  bash "$WORKDIR/baota-install.sh" --repair-guardian --yes --site-root "$SITE_ROOT"
  exit 0
fi

install_info "开始安装到 ${SITE_ROOT}"
export AUTH_PRO_ONECLICK=1
ARGS=(--yes --site-root "$SITE_ROOT" --package "$PKG_FILE" "$START_FLAG")
if [[ -n "$PORT" ]]; then
  ARGS+=(--port "$PORT")
fi
bash "$WORKDIR/baota-install.sh" "${ARGS[@]}"
