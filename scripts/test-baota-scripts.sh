#!/usr/bin/env bash
# 无宝塔面板时的安装/升级脚本自检。不启动真实 auth-pro 服务。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL="$ROOT/backend/handler/install.sh"
# 升级和安装是同一个入口，用子命令区分。
run_upgrade() { "$INSTALL" upgrade "$@"; }
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/auth-pro-baota-test.XXXXXX")"
PIDS=()

cleanup() {
  local pid
  for pid in "${PIDS[@]+"${PIDS[@]}"}"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      pkill -P "$pid" 2>/dev/null || true
      kill "$pid" 2>/dev/null || true
    fi
  done
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

fail() {
  printf '[失败] %s\n' "$1" >&2
  exit 1
}

ok() {
  printf '[通过] %s\n' "$1"
}

assert_eq() {
  [[ "$1" == "$2" ]] || fail "$3：期望 [$2]，实际 [$1]"
}

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

make_payload() {
  local dir="$1" marker="$2"
  mkdir -p "$dir/assets" "$dir/backend"
  printf '<html>%s</html>\n' "$marker" > "$dir/index.html"
  printf '<html>unavailable-%s</html>\n' "$marker" > "$dir/backend-unavailable.html"
  printf '{"version":"%s"}\n' "$marker" > "$dir/version.json"
  printf '{"version":"%s","frontendDir":".","backendFile":"backend/auth_pro","requiredFiles":[]}\n' "$marker" > "$dir/manifest.json"
  printf 'asset-%s\n' "$marker" > "$dir/assets/app.js"
  printf 'binary-%s\n' "$marker" > "$dir/backend/auth_pro"
  chmod 644 "$dir/backend/auth_pro"
  cp "$ROOT/scripts/baota-panel.py" "$ROOT/backend/handler/guardian_start.sh" "$dir/"
  mv "$dir/guardian_start.sh" "$dir/guardian-start.sh"
}

pack_payload() {
  local dir="$1" archive="$2"
  tar -czf "$archive" -C "$dir" .
}

# 只改测试副本。成品 backend/handler/install.sh 里的官网地址保持写死。
install_for_test() {
  local dest="$1" origin="$2"
  cp "$ROOT/backend/handler/install.sh" "$dest"
  sed -i \
    -e "s|https://auth.maizll.com|${origin}|g" \
    -e "s|--proto '=https'|--proto '=http'|g" \
    "$dest"
  chmod 755 "$dest"
  if grep -q 'AUTH_PRO_UPDATE_BASE' "$dest"; then
    fail "测试副本仍读取 AUTH_PRO_UPDATE_BASE"
  fi
  if grep -q 'https://auth.maizll.com' "$dest"; then
    fail "测试副本仍指向官网"
  fi
  grep -q "${origin}/api/v1/update/latest.json" "$dest" || fail "测试副本没有换成临时地址"
}

bash -n "$INSTALL"
ok "bash -n"

help_out="$("$INSTALL" --help)"
printf '%s\n' "$help_out" | grep -q -- '--site-root' || fail "安装脚本 --help 缺少 --site-root"
printf '%s\n' "$help_out" | grep -q 'install.lock' || fail "安装脚本 --help 未说明 install.lock"
printf '%s\n' "$help_out" | grep -q -- '--repair-guardian' || fail "安装脚本 --help 缺少 --repair-guardian"
"$ROOT/backend/handler/install.sh" --help | grep -q -- '--repair-guardian' || fail "一条命令安装 --help 缺少 --repair-guardian"
"$ROOT/backend/handler/install.sh" --help | grep -F -q 'auth.maizll.com/install.sh | bash -s -- --repair-guardian' || fail "一条命令安装 --help 没有写死修复命令"
"$ROOT/backend/handler/install.sh" --help | grep -F -q 'auth.maizll.com/install.sh | bash -s -- upgrade' || fail "一条命令安装 --help 没有写死升级命令"
printf '%s\n' "$help_out" | grep -q -- '--reset-admin-password' || fail "安装脚本 --help 缺少 --reset-admin-password"
"$ROOT/backend/handler/install.sh" --help | grep -q -- '--reset-admin-password' || fail "一条命令安装 --help 缺少 --reset-admin-password"
"$ROOT/backend/handler/install.sh" --help | grep -F -q 'auth.maizll.com/install.sh | bash -s -- --reset-admin-password' || fail "一条命令安装 --help 没有写死重设密码命令"
printf '%s\n' "$help_out" | grep -F -q 'auth.maizll.com/install.sh | bash' || fail "帮助里没有不带参数的菜单命令"
printf '%s\n' "$help_out" | grep -q -- '--status' || fail "帮助里没有 --status"
printf '%s\n' "$help_out" | grep -q -- '--backup' || fail "帮助里没有 --backup"
printf '%s\n' "$help_out" | grep -q -- '--restore' || fail "帮助里没有 --restore"
printf '%s\n' "$help_out" | grep -q -- '--uninstall' || fail "帮助里没有 --uninstall"
printf '%s\n' "$help_out" | grep -q -- '--change-port' || fail "帮助里没有 --change-port"
printf '%s\n' "$help_out" | grep -q -- '--restart' || fail "帮助里没有 --restart"
printf '%s\n' "$help_out" | grep -q -- '--show-admin' || fail "帮助里没有 --show-admin"
ver="$(tr -d '[:space:]' < "$ROOT/VERSION")"
grep -F -q "auth-pro ${ver}" "$INSTALL" || fail "菜单标题与 VERSION 不一致"
if grep -E -q '033\[(33|93|38;5)' "$INSTALL"; then
  fail "安装脚本使用了黄色或橙色"
fi
grep -q 'baota_random_digits 8' "$INSTALL" || fail "管理员密码没有改成 8 位数字"
grep -q 'baota_random_alnum 20 mixed' "$INSTALL" || fail "数据库密码不再是原来的随机强密码"
grep -q '登录后请在后台修改密码' "$INSTALL" || fail "打印凭据时没有提示登录后修改密码"
grep -q '/root/auth-pro-' "$INSTALL" || fail "凭据文件路径变了"
grep -q 'chmod 600' "$INSTALL" || fail "凭据文件没有收紧为仅所有者可读"
run_upgrade --help | grep -q -- '--skip-mysql' || fail "升级脚本 --help 缺少 --skip-mysql"
if grep -q 'plugin/supervisor/config.py' "$ROOT/scripts/baota-panel.py"; then
  fail "面板辅助脚本不能调用会清空主配置的整理脚本"
fi
grep -q '拒绝 nohup' "$INSTALL" || fail "面板安装失败时没有拒绝 nohup"
[[ ! -e "$ROOT/scripts/baota-install.sh" && ! -e "$ROOT/scripts/baota-upgrade.sh" && ! -e "$ROOT/scripts/baota-lib.sh" ]] || fail "旧的安装入口脚本还在"
[[ ! -e "$ROOT/scripts/install.sh" && ! -e "$ROOT/backend/handler/install_public.sh" ]] || fail "安装脚本还有第二份"
ok "--help 与单一安装入口"

(
  set -euo pipefail
  SCRIPT_DIR="$ROOT/scripts"
  # shellcheck source=/dev/null
  source "$INSTALL"
  digits="$(baota_random_digits 8)"
  [[ "$digits" =~ ^[0-9]{8}$ ]] || exit 1
  mixed="$(baota_random_alnum 20 mixed)"
  [[ "$mixed" =~ ^[A-Za-z][A-Za-z0-9]{19}$ ]] || exit 1
  body="$(baota_wizard_body 'dbname' 'dbuser' 'db-pass' 'admin' "$digits")"
  python3 - "$body" "$digits" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
assert data["adminUsername"] == "admin", data
assert data["adminPassword"] == sys.argv[2], data
assert data["password"] == "db-pass", data
assert "adminPassword" in data and data["username"] == "dbuser"
PY
  printed="$(baota_print_credentials example.com 19127 dbname dbuser 'db-pass' "$digits" http /root/auth-pro-example.com.txt)"
  printf '%s\n' "$printed" | grep -F -q "管理后台: http://example.com/admin"
  printf '%s\n' "$printed" | grep -F -q "管理员密码: ${digits}"
  printf '%s\n' "$printed" | grep -F -q '登录后请在后台修改密码'
)
ok "管理员密码为 8 位数字，向导字段与登录后台地址一致"

RESET_SITE="$WORKDIR/reset-admin-site"
mkdir -p "$RESET_SITE/backend"
printf 'keep-reset-page\n' > "$RESET_SITE/index.html"
printf 'lock\n' > "$RESET_SITE/backend/install.lock"
printf '{"keep":true}\n' > "$RESET_SITE/backend/db.json"
RESET_PAGE="$(sha256sum "$RESET_SITE/index.html" | awk '{print $1}')"
RESET_DB="$(sha256sum "$RESET_SITE/backend/db.json" | awk '{print $1}')"
if [[ "$(id -u)" -ne 0 ]]; then
  if "$INSTALL" --reset-admin-password --yes --site-root "$RESET_SITE" >"$WORKDIR/reset-admin.out" 2>"$WORKDIR/reset-admin.err"; then
    fail "非 root 重设了管理员密码"
  fi
  grep -q '只有 root' "$WORKDIR/reset-admin.err" || fail "非 root 没有拒绝重设密码"
  [[ "$(sha256sum "$RESET_SITE/index.html" | awk '{print $1}')" == "$RESET_PAGE" ]] || fail "拒绝重设时改了网站首页"
  [[ "$(sha256sum "$RESET_SITE/backend/db.json" | awk '{print $1}')" == "$RESET_DB" ]] || fail "拒绝重设时改了 db.json"
  ok "非 root 不能重设管理员密码"
else
  RESET_BIN="$WORKDIR/reset-admin-bin"
  cat > "$RESET_BIN" <<'EOF'
#!/bin/sh
if [ "$1" != "reset-admin-password" ]; then
  echo "unexpected args" >&2
  exit 1
fi
if [ -z "$AUTO_PRO_DATA_DIR" ] || [ ! -f "$AUTO_PRO_DATA_DIR/db.json" ]; then
  echo "missing data dir" >&2
  exit 1
fi
printf '%s\n' '管理员账号: admin' '管理员密码: 13572468' '登录后请在后台修改密码'
EOF
  chmod 755 "$RESET_BIN"
  AUTH_PRO_PUBLIC_HOST=reset.example.com "$INSTALL" --reset-admin-password --yes --site-root "$RESET_SITE" --reset-binary "$RESET_BIN" >"$WORKDIR/reset-admin.out" 2>"$WORKDIR/reset-admin.err" || fail "root 重设管理员密码失败：$(cat "$WORKDIR/reset-admin.err")"
  grep -F -q '管理员密码: 13572468' "$WORKDIR/reset-admin.out" || fail "没有打印新的管理员密码"
  grep -F -q '登录后请在后台修改密码' "$WORKDIR/reset-admin.out" || fail "重设时没有提示登录后修改密码"
  [[ "$(sha256sum "$RESET_SITE/index.html" | awk '{print $1}')" == "$RESET_PAGE" ]] || fail "重设密码时改了网站首页"
  [[ "$(sha256sum "$RESET_SITE/backend/db.json" | awk '{print $1}')" == "$RESET_DB" ]] || fail "重设密码时改了 db.json"
  [[ -f /root/auth-pro-reset.example.com.txt ]] || fail "没有写入凭据文件"
  grep -F -q '管理员密码: 13572468' /root/auth-pro-reset.example.com.txt || fail "凭据文件没有新密码"
  [[ "$(stat -c '%a' /root/auth-pro-reset.example.com.txt)" == "600" ]] || fail "凭据文件不是仅所有者可读"
  rm -f /root/auth-pro-reset.example.com.txt
  ok "root 本机重设管理员密码"
fi

grep -F -q 'for packaged_script in baota-panel.py' "$ROOT/scripts/build-release.sh" || fail "build-release.sh 未只把面板辅助脚本打进发布包"
grep -F -q 'guardian-start.sh' "$ROOT/scripts/build-release.sh" || fail "build-release.sh 未复制进程守护模板"
if grep -E -q 'packaged_script in .*install\.sh|cp .*scripts/install\.sh" "\$PACKAGE_DIR' "$ROOT/scripts/build-release.sh"; then
  fail "build-release.sh 仍会把 install.sh 打进发布包"
fi
if grep -E -q 'install_public\.sh|scripts/install\.sh' "$ROOT/scripts/build-release.sh" "$ROOT/scripts/build-release.ps1"; then
  fail "构建脚本仍在复制第二份 install.sh"
fi
grep -F -q "@('baota-panel.py')" "$ROOT/scripts/build-release.ps1" || fail "build-release.ps1 未只复制面板辅助脚本"
if grep -F -q "@('install.sh'" "$ROOT/scripts/build-release.ps1" || grep -F -q "'install.sh'," "$ROOT/scripts/build-release.ps1"; then
  fail "build-release.ps1 仍会把 install.sh 打进发布包"
fi
if grep -E -q 'baota-install\.sh|baota-upgrade\.sh|baota-lib\.sh' "$ROOT/scripts/build-release.ps1"; then
  fail "build-release.ps1 仍会把旧入口打进发布包"
fi
ok "发布脚本会带上宝塔脚本"

if "$INSTALL" --yes --dry-run --site-root /tmp >/dev/null 2>"$WORKDIR/deny.err"; then
  fail "不应接受 /tmp 作为网站根"
fi
grep -q '拒绝' "$WORKDIR/deny.err" || fail "拒绝系统目录时没有说明原因"
ok "拒绝把 /tmp 当作网站根"

PORT="$(free_port)"
PKG_V1="$WORKDIR/payload-v1"
SITE="$WORKDIR/site"
mkdir -p "$SITE"
printf 'keep-user-ini\n' > "$SITE/.user.ini"
make_payload "$PKG_V1" "v1"
tar -czf "$WORKDIR/v1.tar.gz" -C "$PKG_V1" .

dry_site="$WORKDIR/dry-site"
mkdir -p "$dry_site"
"$INSTALL" --yes --dry-run --no-start \
  --site-root "$dry_site" \
  --package "$WORKDIR/v1.tar.gz" \
  --port "$PORT" >"$WORKDIR/dry.out"
grep -q '预演' "$WORKDIR/dry.out" || fail "dry-run 没有说明这是预演"
[[ ! -e "$dry_site/backend/auth_pro" ]] || fail "dry-run 写入了二进制"
[[ ! -e "$dry_site/backend/baota.env" ]] || fail "dry-run 写入了环境文件"
ok "安装 dry-run 不改文件"

"$INSTALL" --yes --no-start \
  --site-root "$SITE" \
  --package "$WORKDIR/v1.tar.gz" \
  --port "$PORT" >"$WORKDIR/install.out"
[[ "$(stat -c '%a' "$SITE/backend/auth_pro")" == "755" ]] || fail "auth_pro 权限不是 755"
grep -q "PORT=${PORT}" "$SITE/backend/baota.env" || fail "baota.env 端口不对"
grep -q 'HOST=127.0.0.1' "$SITE/backend/baota.env" || fail "baota.env 未绑定本机"
grep -q "AUTO_PRO_DATA_DIR=${SITE}/backend" "$SITE/backend/baota.env" || fail "数据目录未写入 baota.env"
[[ "$(stat -c '%a' "$SITE/backend/baota.env")" == "600" ]] || fail "baota.env 不是 600"
[[ -x "$SITE/backend/start.sh" ]] || fail "缺少 start.sh"
grep -q 'location \^~ /backend/' "$SITE/backend/baota-nginx.snippet.conf" || fail "Nginx 片段未拦截 /backend/"
grep -F -q 'db\.json' "$SITE/backend/baota-nginx.snippet.conf" || fail "Nginx 片段未拦截 db.json"
grep -F -q 'install\.lock' "$SITE/backend/baota-nginx.snippet.conf" || fail "Nginx 片段未拦截 install.lock"
grep -q "$SITE/backend/start.sh" "$SITE/backend/baota-guardian.txt" || fail "进程守护说明缺少启动命令"
[[ "$(cat "$SITE/.user.ini")" == "keep-user-ini" ]] || fail "安装破坏了 .user.ini"
grep -q '<html>v1</html>' "$SITE/index.html" || fail "首页未装入"
grep -q 'unavailable-v1' "$SITE/backend-unavailable.html" || fail "后端不可达页面未装入"
grep -q 'AUTO_PRO_PROCESS_MANAGER=supervisor' "$SITE/backend/baota.env" || fail "baota.env 未声明进程守护"
grep -q 'supervisor' "$SITE/backend/process-manager" || fail "缺少进程守护标记"
grep -q 'AUTO_PRO_PROCESS_MANAGER' "$SITE/backend/start.sh" || fail "start.sh 未导出进程守护标记"
grep -q 'pending-restart/handoff.sh' "$SITE/backend/start.sh" || fail "start.sh 没有待验证交接"
grep -q 'auth-pro-guardian-start' "$SITE/backend/start.sh" || fail "start.sh 不是进程守护交接版本"
grep -q 'error_page 502 503 504 /backend-unavailable.html' "$SITE/backend/baota-nginx.snippet.conf" || fail "Nginx 片段没有后端不可达页面"
ok "全新安装：权限、环境文件、Nginx 片段、守护说明"

NEW_SITE="$WORKDIR/auto-site"
[[ ! -e "$NEW_SITE" ]] || fail "自动创建用的目录不应预先存在"
"$INSTALL" --yes --no-start \
  --site-root "$NEW_SITE" \
  --package "$WORKDIR/v1.tar.gz" \
  --port "$PORT" >"$WORKDIR/autocreate.out"
[[ -d "$NEW_SITE/backend" ]] || fail "没有创建网站目录"
[[ -x "$NEW_SITE/backend/start.sh" ]] || fail "自动创建后没有 start.sh"
grep -q '已创建网站目录' "$WORKDIR/autocreate.out" || fail "没有说明已创建目录"
grep -q '请用浏览器打开站点域名' "$WORKDIR/autocreate.out" || fail "没有提示打开网址"
ok "网站目录不存在时自动创建"

DRY_NEW="$WORKDIR/dry-auto-site"
"$INSTALL" --yes --dry-run --no-start \
  --site-root "$DRY_NEW" \
  --package "$WORKDIR/v1.tar.gz" \
  --port "$PORT" >"$WORKDIR/dry-auto.out"
[[ ! -e "$DRY_NEW" ]] || fail "预演创建了网站目录"
grep -q '将创建网站目录' "$WORKDIR/dry-auto.out" || fail "预演没有说明将创建目录"
ok "预演不创建网站目录"

if run_upgrade --yes --no-start --skip-mysql \
  --site-root "$WORKDIR/no-such-upgrade-site" \
  --source "$PKG_V1" >"$WORKDIR/up-missing.out" 2>"$WORKDIR/up-missing.err"; then
  fail "升级不应创建缺失的网站目录"
fi
grep -q '不存在' "$WORKDIR/up-missing.err" || fail "升级缺少目录时没有说明"
[[ ! -e "$WORKDIR/no-such-upgrade-site" ]] || fail "升级创建了网站目录"
ok "升级不会创建缺失目录"

if "$INSTALL" --yes --no-start --site-root "$SITE" --source "$PKG_V1" >"$WORKDIR/reinstall.out" 2>"$WORKDIR/reinstall.err"; then
  :
fi
# 尚无 install.lock 时允许再次整理。写入锁后再拒绝。
printf 'installed\n' > "$SITE/backend/install.lock"
if "$INSTALL" --yes --no-start --site-root "$SITE" --source "$PKG_V1" >"$WORKDIR/locked.out" 2>"$WORKDIR/locked.err"; then
  fail "已有 install.lock 时安装脚本不应继续"
fi
grep -q 'bash -s -- upgrade' "$WORKDIR/locked.err" || fail "拒绝安装时没有指向升级命令"
ok "已安装站点拒绝再次全新安装"

printf '{"host":"127.0.0.1","port":"3306","database":"auth_pro","username":"auth_user","password":"secret-pass"}\n' > "$SITE/backend/db.json"
chmod 600 "$SITE/backend/db.json"
printf 'jwt-secret-value\n' > "$SITE/backend/jwt.secret"
chmod 600 "$SITE/backend/jwt.secret"
mkdir -p "$SITE/backend/plugins/demo" "$SITE/backend/home-templates" "$SITE/backend/logs" "$SITE/backend/advertisement-images" "$SITE/backend/source-packages" "$SITE/backend/app-releases" "$SITE/backend/software-source-cache"
printf 'plugin-keep\n' > "$SITE/backend/plugins/demo/payload.txt"
printf 'old-asset\n' > "$SITE/assets/old.js"
cp "$SITE/backend/db.json" "$WORKDIR/db.json.before"
cp "$SITE/backend/plugins/demo/payload.txt" "$WORKDIR/plugin.before"
cp "$SITE/backend/install.lock" "$WORKDIR/lock.before"

PKG_V2="$WORKDIR/payload-v2"
make_payload "$PKG_V2" "v2"
tar -czf "$WORKDIR/v2.tar.gz" -C "$PKG_V2" .

run_upgrade --help >/dev/null
run_upgrade --yes --dry-run --no-start --skip-mysql \
  --site-root "$SITE" \
  --package "$WORKDIR/v2.tar.gz" >"$WORKDIR/upgrade-dry.out"
grep -q '预演' "$WORKDIR/upgrade-dry.out" || fail "升级 dry-run 没有预演说明"
grep -q '<html>v1</html>' "$SITE/index.html" || fail "升级 dry-run 改写了首页"
ok "升级 dry-run 不改文件"

FAKE_BIN="$WORKDIR/fake-bin"
mkdir -p "$FAKE_BIN"
cat > "$FAKE_BIN/mysqldump" <<'EOF'
#!/bin/bash
{
  printf '%s\n' "$@"
  if [[ -n "${MYSQL_PWD:-}" ]]; then
    printf 'HAS_MYSQL_PWD\n'
  fi
} >> "${AUTH_PRO_DUMP_LOG:?}"
printf 'CREATE TABLE kept;\n'
exit 0
EOF
chmod 755 "$FAKE_BIN/mysqldump"

AUTH_PRO_DUMP_LOG="$WORKDIR/dump-args.txt"
export AUTH_PRO_DUMP_LOG
PATH="$FAKE_BIN:$PATH" run_upgrade --yes --no-start \
  --site-root "$SITE" \
  --package "$WORKDIR/v2.tar.gz" >"$WORKDIR/upgrade.out"
unset AUTH_PRO_DUMP_LOG

grep -q '<html>v2</html>' "$SITE/index.html" || fail "升级后首页仍是旧版"
grep -q 'unavailable-v2' "$SITE/backend-unavailable.html" || fail "升级后静态错误页仍是旧版"
grep -q 'binary-v2' "$SITE/backend/auth_pro" || fail "升级后二进制仍是旧版"
[[ ! -f "$SITE/assets/old.js" ]] || fail "旧的 assets 文件还留在网站根"
grep -q 'asset-v2' "$SITE/assets/app.js" || fail "新的 assets 没有就位"
cmp -s "$SITE/backend/db.json" "$WORKDIR/db.json.before" || fail "db.json 内容变化"
cmp -s "$SITE/backend/plugins/demo/payload.txt" "$WORKDIR/plugin.before" || fail "插件文件内容变化"
cmp -s "$SITE/backend/install.lock" "$WORKDIR/lock.before" || fail "install.lock 内容变化"
grep -q 'jwt-secret-value' "$SITE/backend/jwt.secret" || fail "jwt.secret 丢失"
[[ "$(cat "$SITE/.user.ini")" == "keep-user-ini" ]] || fail "升级破坏了 .user.ini"
[[ "$(stat -c '%a' "$SITE/backend/auth_pro")" == "755" ]] || fail "升级后二进制权限不是 755"
[[ "$(stat -c '%a' "$SITE/backend/db.json")" == "600" ]] || fail "升级后 db.json 不是 600"
backup_dir="$(find "$SITE/backend/updates/backups" -mindepth 1 -maxdepth 1 -type d -name 'baota-upgrade-*' | head -n 1)"
[[ -n "$backup_dir" ]] || fail "没有升级备份目录"
[[ -f "$backup_dir/data/db.json" ]] || fail "备份里没有 db.json"
[[ -f "$backup_dir/data/install.lock" ]] || fail "备份里没有 install.lock"
[[ -f "$backup_dir/data/plugins/demo/payload.txt" ]] || fail "备份里没有插件文件"
[[ -f "$backup_dir/db.sql" ]] || fail "没有 mysqldump 结果"
grep -q 'CREATE TABLE kept' "$backup_dir/db.sql" || fail "mysqldump 结果内容不对"
grep -q 'secret-pass' "$WORKDIR/dump-args.txt" && fail "mysqldump 参数里出现了数据库口令"
grep -q 'HAS_MYSQL_PWD' "$WORKDIR/dump-args.txt" || fail "mysqldump 没有通过环境变量收到口令"
[[ -f "$backup_dir/auth_pro.prev" ]] || fail "备份里没有旧二进制"
grep -q 'binary-v1' "$backup_dir/auth_pro.prev" || fail "旧二进制备份内容不对"
grep -q "PORT=${PORT}" "$SITE/backend/baota.env" || fail "升级改写了已有端口"
ok "升级保留运行数据、备份路径和数据库导出"

# 无关进程占端口时，--stop-port 必须拒绝，且不能替换文件。
OTHER_PORT="$(free_port)"
python3 - "$OTHER_PORT" <<'PY' &
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
    def log_message(self, *args):
        pass
ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
other_pid=$!
PIDS+=("$other_pid")
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if python3 - "$OTHER_PORT" <<'PY'
import socket, sys
s = socket.socket()
try:
    s.connect(("127.0.0.1", int(sys.argv[1])))
except OSError:
    sys.exit(1)
PY
  then
    break
  fi
  sleep 0.2
done
printf 'do-not-replace\n' > "$SITE/backend/auth_pro"
chmod 755 "$SITE/backend/auth_pro"
cp "$SITE/backend/auth_pro" "$WORKDIR/auth-before-refuse"
# 让升级脚本看到这个端口被别人占用：临时改 baota.env，跑完再改回。
sed -i "s/^PORT=.*/PORT=${OTHER_PORT}/" "$SITE/backend/baota.env"
set +e
run_upgrade --yes --no-start --stop-port --skip-mysql \
  --site-root "$SITE" \
  --source "$PKG_V2" >"$WORKDIR/refuse.out" 2>"$WORKDIR/refuse.err"
refuse_status=$?
set -e
[[ "$refuse_status" -ne 0 ]] || fail "无关进程占端口时升级不应该成功"
grep -q '拒绝' "$WORKDIR/refuse.err" || fail "拒绝结束无关进程时没有说明"
cmp -s "$SITE/backend/auth_pro" "$WORKDIR/auth-before-refuse" || fail "拒绝升级后二进制被替换"
kill "$other_pid" 2>/dev/null || true
wait "$other_pid" 2>/dev/null || true
sed -i "s/^PORT=.*/PORT=${PORT}/" "$SITE/backend/baota.env"
ok "不会结束占用端口的无关进程"

# 本站脚本占用端口时，--stop-port 可以结束它并完成升级。
LISTEN_SITE="$WORKDIR/listen-site"
mkdir -p "$LISTEN_SITE"
make_payload "$WORKDIR/payload-listen" "listen-v1"
# 用会监听端口的脚本代替二进制，确认进程树识别。
cat > "$WORKDIR/payload-listen/backend/auth_pro" <<'EOF'
#!/bin/bash
python3 - <<'PY'
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')
    def log_message(self, *args):
        pass
ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
PY
EOF
chmod 755 "$WORKDIR/payload-listen/backend/auth_pro"
LISTEN_PORT="$(free_port)"
"$INSTALL" --yes --no-start \
  --site-root "$LISTEN_SITE" \
  --source "$WORKDIR/payload-listen" \
  --port "$LISTEN_PORT" >/dev/null
printf 'installed\n' > "$LISTEN_SITE/backend/install.lock"
printf 'keep-db\n' > "$LISTEN_SITE/backend/db.json"
mkdir -p "$LISTEN_SITE/backend/plugins"
printf 'keep-plugin\n' > "$LISTEN_SITE/backend/plugins/keep.txt"
PORT="$LISTEN_PORT" "$LISTEN_SITE/backend/auth_pro" &
listen_pid=$!
PIDS+=("$listen_pid")
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fsS "http://127.0.0.1:${LISTEN_PORT}/" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done
curl -fsS "http://127.0.0.1:${LISTEN_PORT}/" >/dev/null || fail "自检监听进程没有起来"
make_payload "$WORKDIR/payload-listen-v2" "listen-v2"
run_upgrade --yes --no-start --skip-mysql \
  --site-root "$LISTEN_SITE" \
  --source "$WORKDIR/payload-listen-v2" >"$WORKDIR/stopped.out"
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if ! kill -0 "$listen_pid" 2>/dev/null; then
    break
  fi
  sleep 0.2
done
kill -0 "$listen_pid" 2>/dev/null && fail "本站占用端口的进程还在"
grep -q 'binary-listen-v2' "$LISTEN_SITE/backend/auth_pro" || fail "结束本站进程后没有装上新程序"
grep -q 'keep-db' "$LISTEN_SITE/backend/db.json" || fail "结束进程升级后 db.json 丢失"
grep -q 'keep-plugin' "$LISTEN_SITE/backend/plugins/keep.txt" || fail "结束进程升级后插件丢失"
ok "会先停本站进程再升级，不必额外加 --stop-port"

# 父进程为 1、忽略 SIGTERM 且不响应 HTTP 的本站孤儿，升级要在超时后 SIGKILL，并且不误伤其它端口。
HUNG_PORT="$(free_port)"
HUNG_SITE="$WORKDIR/hung-site"
OTHER_SITE="$WORKDIR/other-site"
mkdir -p "$HUNG_SITE/backend" "$OTHER_SITE/backend"
make_payload "$WORKDIR/payload-hung" "hung-old"
cat > "$WORKDIR/payload-hung/backend/auth_pro" <<'EOF'
#!/usr/bin/env python3
import os, signal
from socket import socket, AF_INET, SOCK_STREAM, SOL_SOCKET, SO_REUSEADDR
signal.signal(signal.SIGTERM, signal.SIG_IGN)
s = socket(AF_INET, SOCK_STREAM)
s.setsockopt(SOL_SOCKET, SO_REUSEADDR, 1)
s.bind(("127.0.0.1", int(os.environ["PORT"])))
s.listen(8)
while True:
    conn, _ = s.accept()
EOF
chmod 755 "$WORKDIR/payload-hung/backend/auth_pro"
"$INSTALL" --yes --no-start \
  --site-root "$HUNG_SITE" \
  --source "$WORKDIR/payload-hung" \
  --port "$HUNG_PORT" >/dev/null
printf 'installed\n' > "$HUNG_SITE/backend/install.lock"
printf 'keep-db\n' > "$HUNG_SITE/backend/db.json"
python3 - "$HUNG_SITE/backend/auth_pro" "$HUNG_PORT" <<'PY' &
import os, sys
binary, port = sys.argv[1:]
if os.fork() > 0:
    raise SystemExit(0)
os.setsid()
if os.fork() > 0:
    raise SystemExit(0)
os.environ["PORT"] = port
os.execv(binary, [binary])
PY
sleep 0.3
hung_pid=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  hung_pid="$(ss -lptn "sport = :${HUNG_PORT}" 2>/dev/null | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | head -n 1)"
  [[ -n "$hung_pid" ]] && break
  sleep 0.2
done
[[ -n "$hung_pid" ]] || fail "不响应的孤儿进程没有占上端口"
hung_ppid="$(ps -o ppid= -p "$hung_pid" | tr -d ' ')"
[[ "$hung_ppid" == "1" ]] || fail "孤儿进程 PPID 不是 1（当前 ${hung_ppid}）"
curl -fsS --max-time 1 "http://127.0.0.1:${HUNG_PORT}/" >/dev/null 2>&1 && fail "孤儿进程不应该响应 HTTP"
OTHER_PORT="$(free_port)"
cat > "$OTHER_SITE/backend/auth_pro" <<'EOF'
#!/bin/bash
exec python3 - <<'PY'
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"other")
    def log_message(self, *args):
        pass
ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
PY
EOF
chmod 755 "$OTHER_SITE/backend/auth_pro"
PORT="$OTHER_PORT" "$OTHER_SITE/backend/auth_pro" &
other_site_pid=$!
PIDS+=("$other_site_pid")
for _ in 1 2 3 4 5 6 7 8 9 10; do
  curl -fsS "http://127.0.0.1:${OTHER_PORT}/" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsS "http://127.0.0.1:${OTHER_PORT}/" >/dev/null || fail "其它站点的 auth_pro 没有起来"
make_payload "$WORKDIR/payload-hung-v2" "hung-new"
AUTH_PRO_TERM_WAIT=2 run_upgrade --yes --no-start --skip-mysql \
  --site-root "$HUNG_SITE" \
  --source "$WORKDIR/payload-hung-v2" >"$WORKDIR/hung.out"
kill -0 "$hung_pid" 2>/dev/null && fail "不响应的本站孤儿还在"
grep -q 'binary-hung-new' "$HUNG_SITE/backend/auth_pro" || fail "清掉孤儿后没有装上新程序"
kill -0 "$other_site_pid" 2>/dev/null || fail "其它站点的 auth_pro 被误杀"
curl -fsS "http://127.0.0.1:${OTHER_PORT}/" >/dev/null || fail "其它站点的端口不再响应"
ok "会 SIGKILL 本站不响应的孤儿，且不误伤其它站点"

# 覆盖安装同样先停本站监听，再放文件。
REINSTALL_PORT="$(free_port)"
REINSTALL_SITE="$WORKDIR/reinstall-site"
mkdir -p "$REINSTALL_SITE"
make_payload "$WORKDIR/payload-reinstall" "reinstall-old"
cat > "$WORKDIR/payload-reinstall/backend/auth_pro" <<'EOF'
#!/usr/bin/env python3
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
    def log_message(self, *args):
        return
ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
EOF
chmod 755 "$WORKDIR/payload-reinstall/backend/auth_pro"
"$INSTALL" --yes --no-start \
  --site-root "$REINSTALL_SITE" \
  --source "$WORKDIR/payload-reinstall" \
  --port "$REINSTALL_PORT" >/dev/null
PORT="$REINSTALL_PORT" "$REINSTALL_SITE/backend/auth_pro" &
reinstall_pid=$!
PIDS+=("$reinstall_pid")
for _ in 1 2 3 4 5 6 7 8 9 10; do
  curl -fsS "http://127.0.0.1:${REINSTALL_PORT}/" >/dev/null 2>&1 && break
  sleep 0.2
done
make_payload "$WORKDIR/payload-reinstall-v2" "reinstall-new"
"$INSTALL" --yes --no-start \
  --site-root "$REINSTALL_SITE" \
  --source "$WORKDIR/payload-reinstall-v2" \
  --port "$REINSTALL_PORT" >"$WORKDIR/reinstall.out"
kill -0 "$reinstall_pid" 2>/dev/null && fail "覆盖安装没有结束本站旧进程"
grep -q 'binary-reinstall-new' "$REINSTALL_SITE/backend/auth_pro" || fail "覆盖安装没有换上新程序"
ok "覆盖安装会先停本站旧进程再写入新文件"

# --start 能拉起一个返回安装状态的桩程序，并在超时配置下失败得足够快。
START_PORT="$(free_port)"
START_SITE="$WORKDIR/start-site"
mkdir -p "$START_SITE"
make_payload "$WORKDIR/payload-start" "start"
cat > "$WORKDIR/payload-start/backend/auth_pro" <<'EOF'
#!/bin/bash
exec python3 - <<'PY'
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path.startswith("/api/install/status"):
            body = b'{"installed":false}'
            self.send_response(200)
        else:
            body = b'no'
            self.send_response(404)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        pass
ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
PY
EOF
chmod 755 "$WORKDIR/payload-start/backend/auth_pro"
"$INSTALL" --yes --start \
  --site-root "$START_SITE" \
  --source "$WORKDIR/payload-start" \
  --port "$START_PORT" >"$WORKDIR/start.out"
grep -q '后端已启动' "$WORKDIR/start.out" || fail "启动成功时没有提示"
curl -fsS "http://127.0.0.1:${START_PORT}/api/install/status" | grep -q 'installed' || fail "健康检查地址没有响应"
start_pid="$(cat "$START_SITE/backend/auto_pro.pid")"
PIDS+=("$start_pid")
kill "$start_pid" 2>/dev/null || true
wait "$start_pid" 2>/dev/null || true
ok "安装后可以按 baota.env 启动并做健康检查"

BAD_SITE="$WORKDIR/bad-site"
mkdir -p "$BAD_SITE"
python3 - "$WORKDIR/slip.tar.gz" <<'PY'
import io, tarfile, sys
path = sys.argv[1]
with tarfile.open(path, "w:gz") as tar:
    payload = b"slip"
    info = tarfile.TarInfo("../auth-pro-slip-test-unique-name.txt")
    info.size = len(payload)
    tar.addfile(info, io.BytesIO(payload))
PY
rm -f /tmp/auth-pro-slip-test-unique-name.txt
set +e
"$INSTALL" --yes --no-start --site-root "$BAD_SITE" --package "$WORKDIR/slip.tar.gz" >"$WORKDIR/slip.out" 2>"$WORKDIR/slip.err"
slip_status=$?
set -e
[[ "$slip_status" -ne 0 ]] || fail "不安全压缩包不应安装成功"
grep -q '不安全路径' "$WORKDIR/slip.err" || fail "没有报告不安全路径"
[[ ! -e /tmp/auth-pro-slip-test-unique-name.txt ]] || fail "路径穿越文件被写到了 /tmp"
ok "拒绝压缩包路径穿越"

NGINX_ROOT="$WORKDIR/nginx-vhost"
NGINX_LOG="$WORKDIR/nginx-calls.txt"
mkdir -p "$NGINX_ROOT"
cat > "$NGINX_ROOT/site.conf" <<EOF
server {
    listen 80;
    server_name example.test;
    root $SITE;
    location / {
        proxy_pass http://127.0.0.1:${PORT};
    }
}
EOF
cp "$NGINX_ROOT/site.conf" "$WORKDIR/nginx-before-ok.conf"
cat > "$WORKDIR/nginx-ok" <<EOF
#!/bin/bash
printf '%s\n' "\$*" >> "$NGINX_LOG"
exit 0
EOF
chmod 755 "$WORKDIR/nginx-ok"
: > "$NGINX_LOG"
AUTH_PRO_NGINX_BIN="$WORKDIR/nginx-ok" AUTH_PRO_NGINX_VHOST_DIR="$NGINX_ROOT" \
  run_upgrade --yes --no-start --skip-mysql \
  --site-root "$SITE" \
  --source "$PKG_V2" >"$WORKDIR/nginx-upgrade.out"
grep -q 'BEGIN AUTH_PRO_BACKEND_UNAVAILABLE' "$NGINX_ROOT/site.conf" || fail "没有写入 Nginx error_page"
grep -q "root $SITE;" "$NGINX_ROOT/site.conf" || fail "Nginx 静态页 root 不是网站根"
grep -c 'location = /backend-unavailable.html' "$NGINX_ROOT/site.conf" | grep -qx 1 || fail "error_page location 不是恰好一处"
grep -q -- '-t' "$NGINX_LOG" || fail "写入前没有 nginx -t"
grep -q -- '-s reload' "$NGINX_LOG" || fail "nginx -t 通过后没有 reload"
AUTH_PRO_NGINX_BIN="$WORKDIR/nginx-ok" AUTH_PRO_NGINX_VHOST_DIR="$NGINX_ROOT" \
  run_upgrade --yes --no-start --skip-mysql \
  --site-root "$SITE" \
  --source "$PKG_V2" >"$WORKDIR/nginx-upgrade-again.out"
grep -c 'location = /backend-unavailable.html' "$NGINX_ROOT/site.conf" | grep -qx 1 || fail "重复升级写了多段 error_page"
ok "Nginx error_page 自动写入且不重复"

cp "$WORKDIR/nginx-before-ok.conf" "$NGINX_ROOT/site.conf"
cat > "$WORKDIR/nginx-bad" <<EOF
#!/bin/bash
printf '%s\n' "\$*" >> "$NGINX_LOG"
if [[ "\$1" == "-t" ]]; then
  exit 1
fi
exit 0
EOF
chmod 755 "$WORKDIR/nginx-bad"
: > "$NGINX_LOG"
AUTH_PRO_NGINX_BIN="$WORKDIR/nginx-bad" AUTH_PRO_NGINX_VHOST_DIR="$NGINX_ROOT" \
  run_upgrade --yes --no-start --skip-mysql \
  --site-root "$SITE" \
  --source "$PKG_V2" >"$WORKDIR/nginx-bad.out"
cmp -s "$NGINX_ROOT/site.conf" "$WORKDIR/nginx-before-ok.conf" || fail "nginx -t 失败后没有还原配置"
grep -q -- '-s reload' "$NGINX_LOG" && fail "nginx -t 失败后仍然 reload"
ok "nginx -t 失败时还原配置并且不 reload"

REMOTE="$ROOT/backend/handler/install.sh"
bash -n "$REMOTE"
"$REMOTE" --help | grep -q 'auth.maizll.com/install.sh' || fail "安装入口 --help 没有官网地址"
if grep -E -q 'github\.com|githubusercontent' "$REMOTE"; then
  fail "一条命令安装脚本露出了仓库地址"
fi
if grep -q 'AUTH_PRO_UPDATE_BASE' "$REMOTE"; then
  fail "成品脚本仍读取 AUTH_PRO_UPDATE_BASE"
fi
grep -q 'https://auth.maizll.com/api/v1/update/latest.json' "$REMOTE" || fail "成品脚本没有写死官网清单地址"
grep -q 'https://auth.maizll.com/api/v1/update/package/' "$REMOTE" || fail "成品脚本没有写死官网下载地址"
grep -F -q -- "--proto '=https'" "$REMOTE" || fail "成品脚本没有锁定 https"
if grep -E -q '\$\{[A-Za-z_][A-Za-z0-9_]*:-https?://' "$REMOTE"; then
  fail "成品脚本用环境变量默认值拼官网地址"
fi
ok "一条命令安装脚本语法与帮助"

REMOTE_SRC="$WORKDIR/remote-src"
make_payload "$REMOTE_SRC" "remote"
# 发布包不带 install.sh。面板辅助脚本和启动模板在包里，安装脚本由测试副本自己执行。
cp "$ROOT/backend/handler/guardian_start.sh" "$REMOTE_SRC/guardian-start.sh"
chmod 755 "$REMOTE_SRC/guardian-start.sh"
tar -czf "$WORKDIR/remote.tar.gz" -C "$REMOTE_SRC" .
REMOTE_PORT="$(free_port)"
REMOTE_SITE="$WORKDIR/remote-site"
cat > "$WORKDIR/fake-update-server.py" <<'PY'
import hashlib, json, os, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

pkg_path, install_path, port, mode = sys.argv[1:5]
pkg = open(pkg_path, "rb").read()
digest = hashlib.sha256(pkg).hexdigest()
install = open(install_path, "rb").read()
version = "1.7.4"
base = "http://127.0.0.1:%s" % port
sha = digest
signature = "sha256:" + digest
if mode == "bad-hash":
    sha = "ab" * 32
    signature = "sha256:" + sha
manifest = {
    "version": version,
    "sha256": sha,
    "size": len(pkg),
    "url": "%s/api/v1/update/package/%s" % (base, version),
    "package": {
        "fileName": "auth_pro-full-v%s.tar.gz" % version,
        "url": "%s/api/v1/update/package/%s" % (base, version),
        "sha256": sha,
        "size": len(pkg),
        "signature": signature,
    },
}
body = json.dumps(manifest).encode()

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/install.sh":
            data, ctype = install, "text/x-shellscript; charset=utf-8"
        elif path == "/api/v1/update/latest.json":
            data, ctype = body, "application/json"
        elif path == "/api/v1/update/package/%s" % version:
            data, ctype = pkg, "application/gzip"
        else:
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        return

ThreadingHTTPServer(("127.0.0.1", int(port)), Handler).serve_forever()
PY
GOOD_COPY="$WORKDIR/install-ok.sh"
install_for_test "$GOOD_COPY" "http://127.0.0.1:${REMOTE_PORT}"
python3 "$WORKDIR/fake-update-server.py" "$WORKDIR/remote.tar.gz" "$GOOD_COPY" "$REMOTE_PORT" ok >"$WORKDIR/fake-update.log" 2>&1 &
PIDS+=("$!")
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fsS "http://127.0.0.1:${REMOTE_PORT}/install.sh" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
if bash "$GOOD_COPY" demo.example --site-root "$REMOTE_SITE" --port "$PORT" >"$WORKDIR/remote-install.out" 2>"$WORKDIR/remote-install.err"; then
  fail "没有宝塔面板时一条命令安装不应继续建站"
fi
grep -q '正在核对 SHA256' "$WORKDIR/remote-install.out" || fail "没有在拒绝前核对安装包"
grep -q '软件商店' "$WORKDIR/remote-install.err" || fail "没有面板时没有提示先在软件商店安装 Nginx 和 MySQL"
[[ ! -e "$REMOTE_SITE/backend/auth_pro" ]] || fail "没有面板时仍写入了站点"
ok "一条命令安装：核对后因没有面板而停止"

mkdir -p "$REMOTE_SITE/backend"
printf 'keep-page\n' > "$REMOTE_SITE/index.html"
SUM_BEFORE="$(sha256sum "$REMOTE_SITE/index.html" | awk '{print $1}')"
printf 'installed\n' > "$REMOTE_SITE/backend/install.lock"
if bash "$GOOD_COPY" demo.example --site-root "$REMOTE_SITE" --port "$PORT" >"$WORKDIR/remote-locked.out" 2>"$WORKDIR/remote-locked.err"; then
  fail "已安装站点仍继续一条命令安装"
fi
grep -q '在线更新' "$WORKDIR/remote-locked.err" || fail "已安装时没有提示改用升级"
SUM_AFTER="$(sha256sum "$REMOTE_SITE/index.html" | awk '{print $1}')"
[[ "$SUM_BEFORE" == "$SUM_AFTER" ]] || fail "已安装时覆盖了站点文件"
grep -q 'repair-guardian' "$WORKDIR/remote-locked.err" || fail "已安装时没有提示改用 --repair-guardian"
ok "已安装站点拒绝一条命令安装"

printf '#!/bin/sh\nexec sleep 3600\n' > "$REMOTE_SITE/backend/start.sh"
chmod 755 "$REMOTE_SITE/backend/start.sh"
cp "$REMOTE_SITE/backend/start.sh" "$REMOTE_SITE/backend/auth_pro"
printf 'PORT=19127\nHOST=127.0.0.1\n' > "$REMOTE_SITE/backend/baota.env"
printf '{"keep":true}\n' > "$REMOTE_SITE/backend/db.json"
DB_BEFORE="$(sha256sum "$REMOTE_SITE/backend/db.json" | awk '{print $1}')"
if bash "$GOOD_COPY" --repair-guardian demo.example --site-root "$REMOTE_SITE" >"$WORKDIR/remote-repair.out" 2>"$WORKDIR/remote-repair.err"; then
  fail "没有宝塔面板时修复命令不应成功"
fi
grep -q '未检测到宝塔面板' "$WORKDIR/remote-repair.err" || fail "没有面板时修复命令没有说明原因"
grep -q '安装完成' "$WORKDIR/remote-repair.out" && fail "修复失败时仍打印了安装完成"
SUM_REPAIR="$(sha256sum "$REMOTE_SITE/index.html" | awk '{print $1}')"
DB_AFTER="$(sha256sum "$REMOTE_SITE/backend/db.json" | awk '{print $1}')"
[[ "$SUM_BEFORE" == "$SUM_REPAIR" ]] || fail "修复失败时改了网站首页"
[[ "$DB_BEFORE" == "$DB_AFTER" ]] || fail "修复失败时改了 db.json"
ok "没有面板时修复命令失败且不改网站文件"

printf 'old-install-sh\n' > "$REMOTE_SITE/install.sh"
printf 'old-baota-install\n' > "$REMOTE_SITE/baota-install.sh"
printf 'old-panel\n' > "$REMOTE_SITE/baota-panel.py"
printf 'old-guardian\n' > "$REMOTE_SITE/guardian-start.sh"
UPGRADE_DB="$(sha256sum "$REMOTE_SITE/backend/db.json" | awk '{print $1}')"
UPGRADE_LOCK="$(sha256sum "$REMOTE_SITE/backend/install.lock" | awk '{print $1}')"
if ! bash "$GOOD_COPY" upgrade demo.example --no-start --skip-mysql --site-root "$REMOTE_SITE" >"$WORKDIR/remote-upgrade.out" 2>"$WORKDIR/remote-upgrade.err"; then
  fail "官网 upgrade 失败：$(cat "$WORKDIR/remote-upgrade.err")"
fi
grep -q '正在核对 SHA256' "$WORKDIR/remote-upgrade.out" || fail "upgrade 没有核对安装包"
grep -q '升级完成' "$WORKDIR/remote-upgrade.out" || fail "upgrade 没有完成"
[[ ! -e "$REMOTE_SITE/install.sh" && ! -e "$REMOTE_SITE/baota-install.sh" && ! -e "$REMOTE_SITE/baota-panel.py" && ! -e "$REMOTE_SITE/guardian-start.sh" ]] || fail "升级后网站根还留着旧脚本"
[[ "$(sha256sum "$REMOTE_SITE/backend/db.json" | awk '{print $1}')" == "$UPGRADE_DB" ]] || fail "upgrade 改了 db.json"
[[ "$(sha256sum "$REMOTE_SITE/backend/install.lock" | awk '{print $1}')" == "$UPGRADE_LOCK" ]] || fail "upgrade 改了 install.lock"
grep -q 'remote' "$REMOTE_SITE/index.html" || fail "upgrade 没有换上发布包里的页面"
ok "官网 upgrade 核对后升级，保留运行数据并删掉网站根旧脚本"

BAD_PORT="$(free_port)"
BAD_COPY="$WORKDIR/install-bad.sh"
install_for_test "$BAD_COPY" "http://127.0.0.1:${BAD_PORT}"
python3 "$WORKDIR/fake-update-server.py" "$WORKDIR/remote.tar.gz" "$BAD_COPY" "$BAD_PORT" bad-hash >"$WORKDIR/fake-bad.log" 2>&1 &
PIDS+=("$!")
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fsS "http://127.0.0.1:${BAD_PORT}/api/v1/update/latest.json" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
BAD_SITE="$WORKDIR/bad-hash-site"
if bash "$BAD_COPY" bad.example --site-root "$BAD_SITE" --port "$PORT" >"$WORKDIR/bad-hash.out" 2>"$WORKDIR/bad-hash.err"; then
  fail "SHA256 不一致时不应安装"
fi
grep -q 'SHA256 不一致' "$WORKDIR/bad-hash.err" || fail "SHA256 不一致时没有拒绝"
[[ ! -e "$BAD_SITE/backend/auth_pro" ]] || fail "校验失败后仍写入了站点"
ok "安装包 SHA256 不一致时停止"

# 备份迁移只认本站点的严格文件名。移动失败不能靠删除交差，这里用不可写目标验证源还在。
bash -c '
set -euo pipefail
SCRIPT_DIR="$1"
source "$SCRIPT_DIR/install.sh"
tmp=$(mktemp -d)
site="$tmp/wwwroot/demo.example"
mkdir -p "$site/backend/updates/backups/baota-upgrade-old"
mkdir -p "$site/backend/updates/backups/baota-install-keep"
mkdir -p "$tmp/wwwroot/demo.example.backup.20260101120000"
mkdir -p "$tmp/wwwroot/demo.example.overlay-backup.20260101120001"
mkdir -p "$tmp/wwwroot/demo.example.backup.notes"
mkdir -p "$tmp/wwwroot/other.example.backup.20260101120002"
printf kept > "$tmp/wwwroot/demo.example.backup.20260101120000/marker"
dest="$tmp/central"
baota_migrate_matching_backups "$site" "$dest"
[[ -d "$dest/rename/demo.example.backup.20260101120000" ]]
[[ -f "$dest/rename/demo.example.backup.20260101120000/marker" ]]
[[ -d "$dest/overlay/demo.example.overlay-backup.20260101120001" ]]
[[ -d "$dest/upgrade/baota-upgrade-old" ]]
[[ -d "$tmp/wwwroot/demo.example.backup.notes" ]]
[[ -d "$tmp/wwwroot/other.example.backup.20260101120002" ]]
[[ -d "$site/backend/updates/backups/baota-install-keep" ]]
[[ ! -d "$tmp/wwwroot/demo.example.backup.20260101120000" ]]
mkdir -p "$dest/upgrade/a" "$dest/upgrade/b" "$dest/upgrade/c" "$dest/upgrade/d"
touch -d "2026-01-01 00:00:01" "$dest/upgrade/a"
touch -d "2026-01-01 00:00:02" "$dest/upgrade/b"
touch -d "2026-01-01 00:00:03" "$dest/upgrade/c"
touch -d "2026-01-01 00:00:04" "$dest/upgrade/d"
baota_prune_backup_dir "$dest/upgrade" 3
count=$(find "$dest/upgrade" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d " ")
[[ "$count" == "3" ]]
[[ ! -d "$dest/upgrade/a" ]]
[[ -d "$dest/upgrade/d" ]]
printf x > "$tmp/notdir"
if baota_move_backup "$tmp/wwwroot/other.example.backup.20260101120002" "$tmp/notdir/nope"; then
  echo "目标父路径不是目录时移动不应该成功" >&2
  exit 1
fi
[[ -d "$tmp/wwwroot/other.example.backup.20260101120002" ]]
rm -rf "$tmp"
' bash "$ROOT/backend/handler"
ok "旧备份只迁移本站点，失败时不删除，每类可只留 3 份"

# 面板返回值只进变量。标准输出里不能出现 AUTH_PRO_*。
FAKE_BIN="$WORKDIR/fake-btpython"
mkdir -p "$FAKE_BIN"
cat > "$FAKE_BIN/btpython" <<'EOF'
#!/bin/sh
echo 'AUTH_PRO_RESULT=ok'
echo 'AUTH_PRO_PROGRAM=auth_pro_demo'
echo '[信息] 给操作者的说明' >&2
EOF
chmod 755 "$FAKE_BIN/btpython"
PANEL_OUT="$WORKDIR/panel-run.out"
PANEL_ERR="$WORKDIR/panel-run.err"
PATH="$FAKE_BIN:$PATH" bash -c '
set -euo pipefail
SCRIPT_DIR="$1"
source "$SCRIPT_DIR/install.sh"
baota_panel_run preflight
printf "RESULT=%s\n" "$BAOTA_PANEL_RESULT"
printf "PROGRAM=%s\n" "$BAOTA_PANEL_PROGRAM"
' bash "$ROOT/backend/handler" >"$PANEL_OUT" 2>"$PANEL_ERR"
grep -q '^RESULT=ok$' "$PANEL_OUT" || fail "没有读到面板返回值"
grep -q '^PROGRAM=auth_pro_demo$' "$PANEL_OUT" || fail "没有读到进程守护名称"
if grep -q 'AUTH_PRO_' "$PANEL_OUT"; then
  fail "面板键值出现在了给操作者的输出里"
fi
grep -q '给操作者的说明' "$PANEL_ERR" || fail "中文说明没有留在标准错误"
ok "面板返回值不出现在终端"

printf '\n自检完成。宝塔进程守护拉起和真实 Nginx reload 仍需要在面板或本机 nginx 上再确认。\n'
