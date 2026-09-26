#!/usr/bin/env bash
# 无宝塔面板时的安装/升级脚本自检。不启动真实 auth-pro 服务。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL="$ROOT/scripts/baota-install.sh"
UPGRADE="$ROOT/scripts/baota-upgrade.sh"
LIB="$ROOT/scripts/baota-lib.sh"
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
  cp "$INSTALL" "$UPGRADE" "$LIB" "$dir/"
}

pack_payload() {
  local dir="$1" archive="$2"
  tar -czf "$archive" -C "$dir" .
}

bash -n "$INSTALL" "$UPGRADE" "$LIB"
ok "bash -n"

help_out="$("$INSTALL" --help)"
printf '%s\n' "$help_out" | grep -q -- '--site-root' || fail "安装脚本 --help 缺少 --site-root"
printf '%s\n' "$help_out" | grep -q 'install.lock' || fail "安装脚本 --help 未说明 install.lock"
"$UPGRADE" --help | grep -q -- '--skip-mysql' || fail "升级脚本 --help 缺少 --skip-mysql"
"$LIB" >/tmp/baota-lib-direct.out 2>/tmp/baota-lib-direct.err && fail "直接执行 baota-lib.sh 应该失败" || true
grep -q 'baota-install.sh' /tmp/baota-lib-direct.err || fail "直接执行 baota-lib.sh 没有提示入口脚本"
ok "--help 与拒绝直接执行 lib"

grep -q 'baota-install.sh' "$ROOT/scripts/build-release.sh" || fail "build-release.sh 未复制安装脚本"
grep -q 'baota-upgrade.sh' "$ROOT/scripts/build-release.sh" || fail "build-release.sh 未复制升级脚本"
grep -q 'baota-lib.sh' "$ROOT/scripts/build-release.sh" || fail "build-release.sh 未复制共用脚本"
grep -q 'baota-install.sh' "$ROOT/scripts/build-release.ps1" || fail "build-release.ps1 未复制安装脚本"
grep -q 'baota-upgrade.sh' "$ROOT/scripts/build-release.ps1" || fail "build-release.ps1 未复制升级脚本"
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

if "$INSTALL" --yes --no-start --site-root "$SITE" --source "$PKG_V1" >"$WORKDIR/reinstall.out" 2>"$WORKDIR/reinstall.err"; then
  :
fi
# 尚无 install.lock 时允许再次整理。写入锁后再拒绝。
printf 'installed\n' > "$SITE/backend/install.lock"
if "$INSTALL" --yes --no-start --site-root "$SITE" --source "$PKG_V1" >"$WORKDIR/locked.out" 2>"$WORKDIR/locked.err"; then
  fail "已有 install.lock 时安装脚本不应继续"
fi
grep -q 'baota-upgrade.sh' "$WORKDIR/locked.err" || fail "拒绝安装时没有指向升级脚本"
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

"$UPGRADE" --help >/dev/null
"$UPGRADE" --yes --dry-run --no-start --skip-mysql \
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
PATH="$FAKE_BIN:$PATH" "$UPGRADE" --yes --no-start \
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
"$UPGRADE" --yes --no-start --stop-port --skip-mysql \
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
"$UPGRADE" --yes --no-start --skip-mysql \
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
AUTH_PRO_TERM_WAIT=2 "$UPGRADE" --yes --no-start --skip-mysql \
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
  "$UPGRADE" --yes --no-start --skip-mysql \
  --site-root "$SITE" \
  --source "$PKG_V2" >"$WORKDIR/nginx-upgrade.out"
grep -q 'BEGIN AUTH_PRO_BACKEND_UNAVAILABLE' "$NGINX_ROOT/site.conf" || fail "没有写入 Nginx error_page"
grep -q "root $SITE;" "$NGINX_ROOT/site.conf" || fail "Nginx 静态页 root 不是网站根"
grep -c 'location = /backend-unavailable.html' "$NGINX_ROOT/site.conf" | grep -qx 1 || fail "error_page location 不是恰好一处"
grep -q -- '-t' "$NGINX_LOG" || fail "写入前没有 nginx -t"
grep -q -- '-s reload' "$NGINX_LOG" || fail "nginx -t 通过后没有 reload"
AUTH_PRO_NGINX_BIN="$WORKDIR/nginx-ok" AUTH_PRO_NGINX_VHOST_DIR="$NGINX_ROOT" \
  "$UPGRADE" --yes --no-start --skip-mysql \
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
  "$UPGRADE" --yes --no-start --skip-mysql \
  --site-root "$SITE" \
  --source "$PKG_V2" >"$WORKDIR/nginx-bad.out"
cmp -s "$NGINX_ROOT/site.conf" "$WORKDIR/nginx-before-ok.conf" || fail "nginx -t 失败后没有还原配置"
grep -q -- '-s reload' "$NGINX_LOG" && fail "nginx -t 失败后仍然 reload"
ok "nginx -t 失败时还原配置并且不 reload"

printf '\n自检完成。宝塔进程守护拉起和真实 Nginx reload 仍需要在面板或本机 nginx 上再确认。\n'
