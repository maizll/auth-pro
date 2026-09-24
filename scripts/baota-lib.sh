#!/usr/bin/env bash
# 宝塔安装/升级共用逻辑。请运行同目录的 baota-install.sh 或 baota-upgrade.sh。
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  printf '%s\n' "请运行 baota-install.sh 或 baota-upgrade.sh，不要直接执行 baota-lib.sh。" >&2
  exit 1
fi

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
  if [[ ${#BAOTA_TMP_DIRS[@]} -eq 0 ]]; then
    return 0
  fi
  for dir in "${BAOTA_TMP_DIRS[@]}"; do
    rm -rf "$dir"
  done
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
  if [[ ! -d "$BAOTA_SITE_ROOT" ]]; then
    baota_die "网站根目录不存在：$BAOTA_SITE_ROOT 。请先在宝塔创建网站。"
  fi
  BAOTA_SITE_ROOT="$(cd "$BAOTA_SITE_ROOT" && pwd -P)"
  baota_assert_safe_dir "$BAOTA_SITE_ROOT" "网站根目录"
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
  while IFS= read -r name; do
    baota_tar_entry_safe "$name"
  done < <(tar -tzf "$pkg")
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
  local pid="$1" site="$2" exe cmd
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  [[ "$pid" -gt 1 ]] || return 1
  if [[ "$pid" == "$$" || "$pid" == "${PPID:-0}" ]]; then
    return 1
  fi
  exe="$(readlink -f "/proc/$pid/exe" 2>/dev/null || true)"
  exe="${exe% (deleted)}"
  cmd="$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)"
  if [[ "$exe" == "$site/backend/auth_pro" ]]; then
    return 0
  fi
  if [[ "$cmd" == *"$site/backend/auth_pro"* ]]; then
    case "$exe" in
      */bash|*/sh|*/dash) return 0 ;;
    esac
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

baota_kill_tree() {
  local pid="$1" child
  [[ "$pid" =~ ^[0-9]+$ ]] || return 0
  for child in $(ps -o pid= --ppid "$pid" 2>/dev/null || true); do
    baota_kill_tree "$child"
  done
  kill "$pid" 2>/dev/null || true
}

baota_ensure_port_available() {
  local port="$1" site="$2" must_clear="$3"
  local pids pid root cmdline
  if ! baota_port_is_open "$port"; then
    baota_info "端口 ${port} 空闲"
    return 0
  fi
  pids="$(baota_pids_for_port "$port")"
  baota_warn "端口 ${port} 已被占用"
  if [[ -z "$pids" ]]; then
    baota_die "看到端口 ${port} 在监听，但当前用户看不到进程。请用 root 运行，或先在宝塔进程守护中停止本站点。"
  fi
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    cmdline="$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)"
    baota_warn "  监听进程 PID ${pid} ${cmdline}"
  done <<< "$pids"

  if [[ "$must_clear" != "1" && "$BAOTA_STOP_PORT" != "1" ]]; then
    baota_warn "本次不启动后端，因此不会结束占用进程。若随后要手工启动，请先释放 ${port}。"
    return 0
  fi
  if [[ "$BAOTA_STOP_PORT" != "1" ]]; then
    baota_die "端口 ${port} 已被占用。请先在宝塔进程守护中停止本站点（否则杀掉会被立刻拉起），确认端口不再自动复活后，加上 --stop-port 再执行。未确认时脚本不会结束进程。"
  fi

  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    if ! root="$(baota_our_root "$pid" "$site")"; then
      baota_die "PID ${pid} 占用 ${port}，但不是本站 backend/auth_pro。已拒绝结束该进程。"
    fi
    if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
      baota_info "预演：将结束本站进程树 PID ${root}"
      continue
    fi
    baota_info "结束本站进程树 PID ${root}"
    baota_kill_tree "$root"
  done <<< "$pids"

  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    return 0
  fi

  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if ! baota_port_is_open "$port"; then
      baota_info "端口 ${port} 已释放"
      return 0
    fi
    sleep 1
  done
  baota_die "端口 ${port} 仍被占用。进程守护可能已把它拉起。请先在宝塔面板停止守护，再重新执行。"
}

baota_prepare_backup_dir() {
  local data ts
  data="$(baota_data_dir)"
  ts="$(date '+%Y%m%d%H%M%S')"
  BAOTA_BACKUP_DIR="${data}/updates/backups/baota-${BAOTA_ACTION}-${ts}-$$"
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
    mkdir -p "$BAOTA_BACKUP_DIR"
    rm -rf "$BAOTA_BACKUP_DIR/assets"
    mv "$dest" "$BAOTA_BACKUP_DIR/assets"
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
  if [[ -f "$dest" ]]; then
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

baota_install_scripts() {
  local name src dest
  for name in baota-install.sh baota-upgrade.sh baota-lib.sh; do
    if [[ -f "$BAOTA_PAYLOAD/$name" ]]; then
      src="$BAOTA_PAYLOAD/$name"
    elif [[ -f "$SCRIPT_DIR/$name" ]]; then
      src="$SCRIPT_DIR/$name"
    else
      baota_die "缺少脚本 $name"
    fi
    dest="$BAOTA_SITE_ROOT/$name"
    if [[ -f "$dest" ]] && [[ "$(readlink -f "$src")" == "$(readlink -f "$dest")" ]]; then
      if [[ "$BAOTA_DRY_RUN" != "1" && "$name" != "baota-lib.sh" ]]; then
        chmod 755 "$dest" || true
      fi
      continue
    fi
    if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
      baota_info "将复制 $name 到网站根"
      continue
    fi
    cp -a "$src" "$dest"
    if [[ "$name" != "baota-lib.sh" ]]; then
      chmod 755 "$dest"
    fi
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
EOF
  chmod 600 "$tmp"
  mv -f "$tmp" "$file"
  baota_info "已写入 $file"
}

baota_write_start_script() {
  local data file
  data="$(baota_data_dir)"
  file="$data/start.sh"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将写入启动脚本 $file"
    return 0
  fi
  mkdir -p "$data"
  cat > "$file" <<'EOF'
#!/usr/bin/env bash
# 宝塔进程守护的启动命令。运行目录可以是本文件所在目录。
set -euo pipefail
cd "$(dirname "$(readlink -f "$0" 2>/dev/null || realpath "$0")")"
if [[ -f ./baota.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source ./baota.env
  set +a
fi
exec ./auth_pro
EOF
  chmod 755 "$file"
  baota_info "已写入 $file"
}

baota_write_nginx_snippet() {
  local data port file
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
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
location = /baota-install.sh { return 404; }
location = /baota-upgrade.sh { return 404; }
location = /baota-lib.sh { return 404; }
location ~* ^/(db\\.json|install\\.lock|jwt\\.secret)$ { return 404; }
location ~* \\.(log|pid)$ { return 404; }
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
说明: 升级或处理残留进程前，先在宝塔进程守护里停止此项，否则进程会被立刻拉起。
Nginx 反代: 127.0.0.1:${port}
Nginx 拦截片段: ${data}/baota-nginx.snippet.conf
EOF
  baota_info "已写入 $file"
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
    baota_replace_assets
    baota_install_binary
  fi
  baota_install_scripts
  baota_chmod_binary
  baota_write_env
  baota_write_start_script
  baota_write_nginx_snippet
  baota_write_guardian_note
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

baota_start_backend() {
  local data port pid i health url
  [[ "$BAOTA_START" == "1" ]] || return 0
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  url="http://127.0.0.1:${port}/api/install/status"
  if [[ "$BAOTA_DRY_RUN" == "1" ]]; then
    baota_info "将后台启动 ${data}/auth_pro ，并请求 ${url}"
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    baota_die "启动后需要 curl 或 wget 做健康检查"
  fi
  mkdir -p "$data/logs"
  baota_ensure_port_available "$port" "$BAOTA_SITE_ROOT" "1"
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
  local timeout="${AUTH_PRO_HEALTH_TIMEOUT:-30}"
  for i in $(seq 1 "$timeout"); do
    sleep 1
    if command -v curl >/dev/null 2>&1; then
      health="$(curl -fsS "$url" 2>/dev/null || true)"
    else
      health="$(wget -qO- "$url" 2>/dev/null || true)"
    fi
    if [[ -n "$health" ]]; then
      baota_info "后端已启动 PID=${pid} 端口=${port}"
      baota_info "本机检查：${url}"
      baota_info "浏览器打开站点域名。全新安装会进入安装向导，请填写事先建好的空数据库。"
      return 0
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      baota_die "后端进程已退出。请查看 ${data}/logs/auto_pro.log"
    fi
  done
  baota_die "健康检查超时（${timeout}s）。请查看 ${data}/logs/auto_pro.log 。若要用进程守护启动，请先结束刚才拉起的进程。"
}

baota_print_manual_steps() {
  local data port
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  cat <<EOF

仍需在宝塔面板手工完成（脚本不改面板数据库）：
  1. 创建网站，根目录为 ${BAOTA_SITE_ROOT}
  2. 创建空 MySQL 库和用户。全新安装在网页向导里填写，不要在脚本里写数据库口令
  3. 站点 Nginx 反代到 127.0.0.1:${port}，并把 ${data}/baota-nginx.snippet.conf 中的 location 放进 server，避免直接下载 /backend、db.json、install.lock
  4. 需要 HTTPS 时在面板申请证书
  5. 进程守护：启动命令 ${data}/start.sh ，运行目录 ${data} 。说明见 ${data}/baota-guardian.txt
     升级或清理残留进程前，先在守护里停止。守护开着时结束进程会被立刻拉起，形成重启循环

运行数据目录：${data}
升级会保留：db.json、install.lock、jwt.secret、plugins、home-templates、software-source-cache、updates、app-releases、logs、advertisement-images、source-packages，以及 baota.env
EOF
}

baota_cmd_install() {
  baota_parse_args "$@"
  baota_resolve_site_root
  baota_resolve_start_flag
  local data port
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  if [[ -f "$data/install.lock" ]]; then
    baota_die "检测到 ${data}/install.lock ，站点已经安装。请改用 baota-upgrade.sh ，以免覆盖运行数据。"
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

baota_cmd_upgrade() {
  baota_parse_args "$@"
  baota_resolve_site_root
  baota_resolve_start_flag
  local data port manifest
  data="$(baota_data_dir)"
  port="$(baota_effective_port)"
  if [[ ! -f "$data/install.lock" && ! -f "$data/db.json" ]]; then
    baota_die "在 ${data} 未找到 db.json 或 install.lock 。这像是新站点，请改用 baota-install.sh 。"
  fi
  if [[ ! -f "$data/install.lock" ]]; then
    baota_warn "没有 install.lock ，仍会按升级处理并保留已有 db.json 。若安装向导还没做完，请先完成向导。"
  fi
  baota_prepare_payload
  baota_confirm "升级 ${BAOTA_SITE_ROOT} ，保留 ${data} 中的运行数据，端口 ${port}"
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
  baota_info "升级完成。备份在 ${BAOTA_BACKUP_DIR}"
  baota_print_manual_steps
}
