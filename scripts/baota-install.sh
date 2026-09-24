#!/usr/bin/env bash
# 宝塔面板一键安装：校验站点布局、赋权、检测端口、生成 Nginx/systemd/进程守护说明。
set -euo pipefail

DEFAULT_PORT=19127
SCRIPT_NAME="$(basename "$0")"

usage() {
  cat <<EOF
用法: bash $SCRIPT_NAME <SITE_ROOT> [PORT]

参数:
  SITE_ROOT   宝塔网站根目录（解压 auth_pro-full 后的目录）
  PORT        后端监听端口，默认 $DEFAULT_PORT

示例:
  bash $SCRIPT_NAME /www/wwwroot/example.com
  bash $SCRIPT_NAME /www/wwwroot/example.com 19127

说明:
  - 不会创建或猜测 MySQL 账号；请在浏览器打开站点，走安装向导填写数据库。
  - 若当前用户有权限，会写入 systemd 单元 auth-pro.service；否则打印宝塔「进程守护」粘贴字段。
  - 端口已被占用时会给出中文处理说明并退出（不会静默失败或反复拉起）。
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

print_nginx_snippet() {
  local site_root="$1"
  local port="$2"
  section "Nginx 反代 + 安全伪静态（粘贴到宝塔网站「配置文件」）"
  cat <<EOF
# ---- auth-pro 整站反代到本机后端 ----
# 建议：关闭「纯静态」；本段放在 server { ... } 内合适位置。
# 站点根: ${site_root}

# 禁止直接访问后端数据与密钥文件
if (\$uri ~* "^/backend(?:/|\$)") { return 404; }
if (\$uri ~* "^/(db\\.json|install\\.lock|jwt\\.secret)\$") { return 404; }

location / {
    proxy_pass http://127.0.0.1:${port};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_set_header Upgrade \$http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 300s;
}
EOF
}

print_bt_guardian() {
  local backend_dir="$1"
  local backend_bin="$2"
  local port="$3"
  section "宝塔「进程守护管理器」粘贴字段（无 systemd 权限时用）"
  cat <<EOF
名称:          auth-pro
启动用户:      root（或运行网站的用户，需对该目录有读写权限）
运行目录:      ${backend_dir}
启动命令:      ${backend_bin}
进程数量:      1
环境变量:      PORT=${port}
               AUTO_PRO_DATA_DIR=${backend_dir}
说明:          保存后启动；若提示端口占用，先停掉旧守护/旧进程再启动。
EOF
}

print_next_steps() {
  local backend_dir="$1"
  section "下一步"
  cat <<EOF
1. 在宝塔网站配置中粘贴上方 Nginx 片段，保存并重载 Nginx。
2. 浏览器打开你的域名，进入安装向导，自行填写 MySQL 主机/库名/账号/密码（脚本不会生成数据库凭据）。
3. 安装完成后访问 /api/system/version 确认后端正常。
4. 数据文件将位于: ${backend_dir}/db.json 、install.lock 、jwt.secret
   请确保 Nginx 已按上方规则拦截 /backend 与这些敏感文件名。
EOF
}

port_pids() {
  local port="$1"
  local pids=""
  if command -v lsof >/dev/null 2>&1; then
    pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  fi
  if [[ -z "$pids" ]] && command -v fuser >/dev/null 2>&1; then
    pids="$(fuser "${port}/tcp" 2>/dev/null | tr -s '[:space:]' '\n' | sed '/^$/d' || true)"
  fi
  if [[ -z "$pids" ]] && [[ -r /proc/net/tcp ]]; then
    local hex_port
    hex_port="$(printf '%04X' "$port")"
    if grep -Eiq ":${hex_port} .* 0A " /proc/net/tcp /proc/net/tcp6 2>/dev/null; then
      printf 'UNKNOWN\n'
      return 0
    fi
  fi
  printf '%s\n' "$pids"
}

write_systemd_unit() {
  local unit_path="$1"
  local unit_name="$2"
  local backend_dir="$3"
  local backend_bin="$4"
  local port="$5"
  local content
  content="$(cat <<EOF
[Unit]
Description=auth-pro license backend
After=network.target

[Service]
Type=simple
WorkingDirectory=${backend_dir}
Environment=PORT=${port}
Environment=AUTO_PRO_DATA_DIR=${backend_dir}
ExecStart=${backend_bin}
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
)"

  if [[ "$(id -u)" -eq 0 ]]; then
    printf '%s\n' "$content" > "$unit_path" || return 1
  elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    printf '%s\n' "$content" | sudo tee "$unit_path" >/dev/null || return 1
  else
    return 1
  fi

  info "已写入 systemd 单元文件: $unit_path"

  if [[ "$(id -u)" -eq 0 ]]; then
    if systemctl daemon-reload 2>/dev/null; then
      systemctl enable "$unit_name" >/dev/null 2>&1 || true
      info "已执行 systemctl daemon-reload / enable $unit_name"
      return 0
    fi
  elif command -v sudo >/dev/null 2>&1; then
    if sudo systemctl daemon-reload 2>/dev/null; then
      sudo systemctl enable "$unit_name" >/dev/null 2>&1 || true
      info "已执行 systemctl daemon-reload / enable $unit_name"
      return 0
    fi
  fi

  info "提示: 当前环境无法连接 systemd（常见于未以 systemd 启动的容器）。"
  info "单元文件已写好；在宝塔宿主机上请执行: sudo systemctl daemon-reload && sudo systemctl enable --now $unit_name"
  info "或改用上方「进程守护管理器」字段。"
  return 0
}

start_systemd_if_possible() {
  local unit_path="$1"
  local unit_name="$2"
  if [[ ! -f "$unit_path" ]]; then
    return 1
  fi
  if [[ "$(id -u)" -eq 0 ]]; then
    systemctl restart "$unit_name" 2>/dev/null && return 0
    return 1
  fi
  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    sudo systemctl restart "$unit_name" 2>/dev/null && return 0
    return 1
  fi
  return 1
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

if [[ ! "$PORT" =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then
  die "端口无效: $PORT（应为 1-65535）"
fi

INDEX_HTML="$SITE_ROOT/index.html"
BACKEND_BIN="$SITE_ROOT/backend/auth_pro"
BACKEND_DIR="$SITE_ROOT/backend"
UNIT_NAME="auth-pro.service"
UNIT_PATH="/etc/systemd/system/$UNIT_NAME"

[[ -f "$INDEX_HTML" ]] || die "缺少 $INDEX_HTML。请确认已将 auth_pro-full 发布包解压到网站根目录。"
[[ -f "$BACKEND_BIN" ]] || die "缺少 $BACKEND_BIN。请确认发布包布局正确（backend/auth_pro）。"
[[ -d "$BACKEND_DIR" ]] || die "缺少目录 $BACKEND_DIR。"

info "站点根目录: $SITE_ROOT"
info "后端端口:   $PORT"

chmod +x "$BACKEND_BIN"
info "已执行: chmod +x backend/auth_pro"

PORT_PIDS="$(port_pids "$PORT" | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
PORT_IN_USE=0
if [[ -n "$PORT_PIDS" ]]; then
  PORT_IN_USE=1
fi

print_nginx_snippet "$SITE_ROOT" "$PORT"
print_bt_guardian "$BACKEND_DIR" "$BACKEND_BIN" "$PORT"

SYSTEMD_OK=0
if write_systemd_unit "$UNIT_PATH" "$UNIT_NAME" "$BACKEND_DIR" "$BACKEND_BIN" "$PORT"; then
  SYSTEMD_OK=1
else
  section "systemd"
  info "当前无 root/sudo 权限，未写入 $UNIT_PATH。"
  info "请使用上方「进程守护管理器」字段，或手动创建单元后执行:"
  info "  sudo systemctl daemon-reload && sudo systemctl enable --now $UNIT_NAME"
fi

if (( PORT_IN_USE )); then
  section "端口占用"
  info "端口 $PORT 已被占用（PID: ${PORT_PIDS}）。"
  info "请先处理旧进程后再启动后端，例如："
  info "  1) 宝塔 → 进程守护管理器 → 停止重复的 auth-pro / 旧守护"
  info "  2) 若已装 systemd: sudo systemctl stop $UNIT_NAME"
  info "  3) 查看占用: lsof -iTCP:$PORT -sTCP:LISTEN"
  info "  4) 确认无误后结束旧进程: kill <PID>（切勿盲目 kill -9 生产库相关进程）"
  info ""
  info "本脚本不会自动杀进程，也不会反复拉起服务，以免端口冲突刷屏。"
  print_next_steps "$BACKEND_DIR"
  exit 2
fi

if (( SYSTEMD_OK )); then
  if start_systemd_if_possible "$UNIT_PATH" "$UNIT_NAME"; then
    info "已执行: systemctl restart $UNIT_NAME"
  else
    info "单元已写入，但未能自动启动。请执行: sudo systemctl restart $UNIT_NAME"
  fi
fi

print_next_steps "$BACKEND_DIR"

section "完成"
info "宝塔一键安装脚本已执行完毕。"
exit 0
