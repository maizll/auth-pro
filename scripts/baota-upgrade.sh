#!/usr/bin/env bash
# 宝塔面板一键升级：备份运行时配置，可选解压新包覆盖站点，再重启服务。
set -euo pipefail

DEFAULT_PORT=19127
SCRIPT_NAME="$(basename "$0")"

usage() {
  cat <<EOF
用法: bash $SCRIPT_NAME <SITE_ROOT> [PORT] [NEW_PACKAGE_TAR.gz]

参数:
  SITE_ROOT            宝塔网站根目录
  PORT                 后端端口（用于重启提示/systemd），默认 $DEFAULT_PORT
  NEW_PACKAGE_TAR.gz   可选；若提供则解压覆盖站点文件，但保留 db.json / install.lock / jwt.secret

示例:
  bash $SCRIPT_NAME /www/wwwroot/example.com
  bash $SCRIPT_NAME /www/wwwroot/example.com 19127
  bash $SCRIPT_NAME /www/wwwroot/example.com 19127 /tmp/auth_pro-full-v1.4.2.tar.gz

说明:
  - 升级前会将 backend 下关键配置复制到 backend/.baota-upgrade-backup-<时间戳>/
  - 解压时先抽出到临时目录，再覆盖站点；覆盖后立即恢复上述配置文件
  - 不会修改 MySQL 凭据；升级后请自行确认 /api/system/version
EOF
}

die() {
  printf '错误: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '%s\n' "$*"
}

section() {
  printf '\n======== %s ========\n' "$*"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 1
fi

SITE_ROOT="$(cd "$1" 2>/dev/null && pwd)" || die "站点根目录不存在: $1"
PORT="${2:-$DEFAULT_PORT}"
PACKAGE_TAR="${3:-}"

if [[ ! "$PORT" =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then
  die "端口无效: $PORT（应为 1-65535）"
fi

INDEX_HTML="$SITE_ROOT/index.html"
BACKEND_DIR="$SITE_ROOT/backend"
BACKEND_BIN="$BACKEND_DIR/auth_pro"
UNIT_NAME="auth-pro.service"
UNIT_PATH="/etc/systemd/system/$UNIT_NAME"

[[ -d "$BACKEND_DIR" ]] || die "缺少目录 $BACKEND_DIR。站点布局不正确。"

# 配置文件列表（相对 backend/）
PRESERVE_FILES=(db.json install.lock jwt.secret)

TS="$(date +%Y%m%d%H%M%S)"
BACKUP_DIR="$BACKEND_DIR/.baota-upgrade-backup-$TS"
mkdir -p "$BACKUP_DIR"

section "备份运行时配置"
BACKED_UP=0
for name in "${PRESERVE_FILES[@]}"; do
  src="$BACKEND_DIR/$name"
  if [[ -f "$src" ]]; then
    cp -a "$src" "$BACKUP_DIR/$name"
    info "已备份: backend/$name -> $BACKUP_DIR/$name"
    BACKED_UP=1
  else
    info "跳过（不存在）: backend/$name"
  fi
done

# 常见数据子目录（若存在则整目录备份，不覆盖恢复逻辑以外的字段）
for dir_name in data uploads logs plugins home-templates; do
  if [[ -d "$BACKEND_DIR/$dir_name" ]]; then
    cp -a "$BACKEND_DIR/$dir_name" "$BACKUP_DIR/$dir_name"
    info "已备份目录: backend/$dir_name"
    BACKED_UP=1
  fi
done

if (( BACKED_UP == 0 )); then
  info "未发现可备份的配置文件；若这是全新站点，可继续；若是生产站请确认路径无误。"
fi

if [[ -n "$PACKAGE_TAR" ]]; then
  [[ -f "$PACKAGE_TAR" ]] || die "升级包不存在: $PACKAGE_TAR"
  section "解压升级包"
  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/auth-pro-baota-upgrade.XXXXXX")"
  cleanup_tmp() {
    rm -rf "$TMP_DIR"
  }
  trap cleanup_tmp EXIT

  info "解压到临时目录: $TMP_DIR"
  tar -xzf "$PACKAGE_TAR" -C "$TMP_DIR"

  # 兼容包根直接是文件，或嵌套一层目录
  EXTRACT_ROOT="$TMP_DIR"
  if [[ ! -f "$TMP_DIR/index.html" && ! -f "$TMP_DIR/backend/auth_pro" ]]; then
    child_count=0
    child_dir=""
    for d in "$TMP_DIR"/*; do
      [[ -d "$d" ]] || continue
      child_count=$((child_count + 1))
      child_dir="$d"
    done
    if (( child_count == 1 )) && [[ -f "$child_dir/index.html" || -f "$child_dir/backend/auth_pro" ]]; then
      EXTRACT_ROOT="$child_dir"
    fi
  fi

  [[ -f "$EXTRACT_ROOT/backend/auth_pro" || -f "$EXTRACT_ROOT/index.html" ]] \
    || die "升级包布局无效：未找到 index.html 或 backend/auth_pro"

  info "覆盖站点文件到: $SITE_ROOT"
  # 使用 tar 管道覆盖，避免 rm -rf 误删数据
  tar -C "$EXTRACT_ROOT" -cf - . | tar -C "$SITE_ROOT" -xf -

  section "恢复运行时配置"
  for name in "${PRESERVE_FILES[@]}"; do
    if [[ -f "$BACKUP_DIR/$name" ]]; then
      cp -a "$BACKUP_DIR/$name" "$BACKEND_DIR/$name"
      info "已恢复: backend/$name"
    fi
  done

  trap - EXIT
  cleanup_tmp
fi

if [[ ! -f "$INDEX_HTML" ]]; then
  die "升级后缺少 $INDEX_HTML，请检查升级包。"
fi
if [[ ! -f "$BACKEND_BIN" ]]; then
  die "升级后缺少 $BACKEND_BIN，请检查升级包。"
fi

chmod +x "$BACKEND_BIN"
info "已执行: chmod +x backend/auth_pro"

restart_service() {
  if [[ -f "$UNIT_PATH" ]]; then
    if [[ "$(id -u)" -eq 0 ]]; then
      if systemctl restart "$UNIT_NAME" 2>/dev/null; then
        info "已重启 systemd 服务: $UNIT_NAME"
        return 0
      fi
    elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
      if sudo systemctl restart "$UNIT_NAME" 2>/dev/null; then
        info "已通过 sudo 重启 systemd 服务: $UNIT_NAME"
        return 0
      fi
    fi
  fi
  return 1
}

section "重启服务"
if restart_service; then
  :
else
  info "未能自动重启 systemd。请在宝塔「进程守护管理器」中重启 auth-pro，或执行："
  info "  sudo systemctl restart $UNIT_NAME"
  info "进程守护字段参考："
  info "  运行目录: $BACKEND_DIR"
  info "  启动命令: $BACKEND_BIN"
  info "  环境变量: PORT=$PORT  AUTO_PRO_DATA_DIR=$BACKEND_DIR"
fi

section "完成"
info "升级流程结束。"
info "备份目录: $BACKUP_DIR（确认无误后可手动删除）"
info "请打开站点并访问 /api/system/version 核对版本。"
exit 0
