#!/usr/bin/env bash
# 在本机用 supervisord 演练在线更新：成功切换、坏包回滚、无守护自重启，以及 nginx 502 静态页。
# 不修改仓库 VERSION，不打 tag。演练二进制只用 -ldflags 注入版本号（当前仓库 1.5.4，演练从 1.5.4 升到 1.5.5）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ART="${AUTH_PRO_SIM_ARTIFACTS:-/opt/cursor/artifacts}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/auth-pro-sim.XXXXXX")"
mkdir -p "$ART"
LOG="$ART/supervised-update.log"
: > "$LOG"
exec > >(stdbuf -oL -eL tee -a "$LOG") 2>&1
trap 'status=$?; printf "脚本在第 %s 行失败，退出码 %s\n" "$LINENO" "$status" >&2' ERR

PIDS=()
cleanup() {
  local pid
  for pid in "${PIDS[@]+"${PIDS[@]}"}"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  if [[ -n "${SUP_CONF:-}" && -f "$SUP_CONF" ]] && command -v supervisorctl >/dev/null 2>&1; then
    supervisorctl -c "$SUP_CONF" shutdown >/dev/null 2>&1 || true
  fi
  if [[ -n "${NGINX_PID:-}" && -f "$NGINX_PID" ]]; then
    nginx -c "${NGINX_CONF:-/dev/null}" -s stop >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() {
  printf '[失败] %s\n' "$1" >&2
  exit 1
}
ok() { printf '[通过] %s\n' "$1"; }

[[ "$(uname -m)" == "x86_64" ]] || fail "演练需要 linux amd64，当前是 $(uname -m)"
command -v go >/dev/null || fail "缺少 go"
command -v python3 >/dev/null || fail "缺少 python3"
command -v supervisord >/dev/null || fail "缺少 supervisord"
command -v supervisorctl >/dev/null || fail "缺少 supervisorctl"
command -v nginx >/dev/null || fail "缺少 nginx"
command -v mysql >/dev/null || fail "缺少 mysql 客户端"
command -v openssl >/dev/null || fail "缺少 openssl"

printf '工作目录 %s\n日志 %s\n' "$WORK" "$LOG"

GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
mkdir -p "$GOCACHE"
export GOCACHE
build_bin() {
  local version="$1" out="$2"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go -C "$ROOT/backend" build -trimpath \
    -ldflags "-X auto_pro/config.AppVersion=${version}" \
    -o "$out" .
}

printf '编译旧版本 1.5.4 与新版本 1.5.5\n'
build_bin 1.5.4 "$WORK/auth_pro_1.5.4"
build_bin 1.5.5 "$WORK/auth_pro_1.5.5"
cp "$ROOT/frontend/public/backend-unavailable.html" "$WORK/backend-unavailable.html"

openssl req -x509 -newkey rsa:2048 -keyout "$WORK/update.key" -out "$WORK/update.crt" -days 2 -nodes \
  -subj "/CN=127.0.0.1" -addext "subjectAltName=IP:127.0.0.1" >/dev/null 2>&1
sudo cp "$WORK/update.crt" /usr/local/share/ca-certificates/auth-pro-update-sim.crt
sudo update-ca-certificates >/dev/null

if ! sudo mysql --protocol=socket -e 'SELECT 1' >/dev/null 2>&1; then
  sudo mkdir -p /run/mysqld
  sudo chown mysql:mysql /run/mysqld
  sudo /etc/init.d/mariadb start
fi
mysql_exec() {
  sudo mysql --protocol=socket -e "$1"
}
mysql_exec "CREATE DATABASE IF NOT EXISTS auth_pro_sim CHARACTER SET utf8mb4;"
mysql_exec "CREATE USER IF NOT EXISTS 'auth_pro_sim'@'127.0.0.1' IDENTIFIED BY 'sim-pass';"
mysql_exec "CREATE USER IF NOT EXISTS 'auth_pro_sim'@'localhost' IDENTIFIED BY 'sim-pass';"
mysql_exec "GRANT ALL PRIVILEGES ON auth_pro_sim.* TO 'auth_pro_sim'@'127.0.0.1';"
mysql_exec "GRANT ALL PRIVILEGES ON auth_pro_sim.* TO 'auth_pro_sim'@'localhost';"
mysql_exec "FLUSH PRIVILEGES;"
# 启动时会跑结构迁移，缺表会直接退出。演练库用安装脚本同一份 schema。
sudo mysql --protocol=socket auth_pro_sim < "$ROOT/backend/handler/schema.sql"
mysql --protocol=tcp -h 127.0.0.1 -u auth_pro_sim -psim-pass auth_pro_sim <<'SQL'
INSERT INTO roles (id, role_name, role_code, enabled)
VALUES (1, '超级管理员', 'R_SUPER', 1)
ON DUPLICATE KEY UPDATE role_code = 'R_SUPER', enabled = 1;
INSERT INTO admins (id, username, password_hash, role_id, enabled)
VALUES (1, 'sim-admin', '$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012', 1, 1)
ON DUPLICATE KEY UPDATE role_id = 1, enabled = 1;
SQL

sign_token() {
  python3 - "$1" <<'PY'
import base64, hashlib, hmac, json, sys, time
secret = open(sys.argv[1], "rb").read().strip()
def b64(raw: bytes) -> bytes:
    return base64.urlsafe_b64encode(raw).rstrip(b"=")
now = int(time.time())
header = b64(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
payload = b64(json.dumps({
    "user_id": 1,
    "username": "sim-admin",
    "role": "admin",
    "role_code": "R_SUPER",
    "iat": now,
    "exp": now + 3600,
}, separators=(",", ":")).encode())
signing = header + b"." + payload
sig = b64(hmac.new(secret, signing, hashlib.sha256).digest())
sys.stdout.write((signing + b"." + sig).decode())
PY
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

start_update_source() {
  local dir="$1" port="$2"
  python3 - "$dir" "$port" "$WORK/update.crt" "$WORK/update.key" <<'PY' &
import http.server, ssl, sys
root, port, cert, key = sys.argv[1:]
class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=root, **kwargs)
    def log_message(self, fmt, *args):
        sys.stderr.write("update-source " + (fmt % args) + "\n")
httpd = http.server.ThreadingHTTPServer(("127.0.0.1", int(port)), Handler)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(cert, key)
httpd.socket = ctx.wrap_socket(httpd.socket, server_side=True)
httpd.serve_forever()
PY
  local pid=$!
  PIDS+=("$pid")
  for _ in $(seq 1 30); do
    if curl -fsS --retry 0 "https://127.0.0.1:${port}/latest.json" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  fail "本地更新源没有起来: https://127.0.0.1:${port}/latest.json"
}

make_package() {
  local version="$1" binary="$2" index_text="$3" outdir="$4"
  local name="auth_pro-full-v${version}.tar.gz"
  local stage="$outdir/stage"
  rm -rf "$stage"
  mkdir -p "$stage/backend" "$stage/assets" "$outdir/www"
  printf '%s\n' "$index_text" > "$stage/index.html"
  printf '{"version":"%s"}\n' "$version" > "$stage/version.json"
  printf 'asset-%s\n' "$version" > "$stage/assets/app.js"
  cp "$WORK/backend-unavailable.html" "$stage/backend-unavailable.html"
  cp "$binary" "$stage/backend/auth_pro"
  chmod 755 "$stage/backend/auth_pro"
  python3 - "$stage/manifest.json" "$version" <<'PY'
import json, sys
json.dump({
    "version": sys.argv[2],
    "frontendDir": ".",
    "backendFile": "backend/auth_pro",
    "requiredFiles": [],
}, open(sys.argv[1], "w"), ensure_ascii=False)
PY
  tar -czf "$outdir/www/$name" -C "$stage" .
  python3 - "$outdir/www" "$name" "$version" <<'PY'
import hashlib, json, os, sys
www, name, version = sys.argv[1:]
path = os.path.join(www, name)
data = open(path, "rb").read()
digest = hashlib.sha256(data).hexdigest()
port = os.environ["UPDATE_PORT"]
base = f"https://127.0.0.1:{port}"
manifest = {
    "version": version,
    "channel": "stable",
    "minVersion": "",
    "force": False,
    "releasedAt": "2026-09-25T00:00:00Z",
    "releasesUrl": "",
    "package": {
        "os": "linux",
        "arch": "amd64",
        "fileName": name,
        "url": f"{base}/{name}",
        "sha256": digest,
        "size": len(data),
        "signature": "sha256:" + digest,
    },
    "actions": {
        "updateFrontend": True,
        "updateBackend": True,
        "restartBackend": True,
        "backupDatabase": False,
    },
    "notes": ["演练包"],
}
open(os.path.join(www, "latest.json"), "w").write(json.dumps(manifest, ensure_ascii=False))
print(digest, len(data))
PY
}

write_site() {
  local site="$1" port="$2" update_port="$3" manager="$4" binary="$5"
  mkdir -p "$site/backend/logs" "$site/assets"
  printf 'old-index\n' > "$site/index.html"
  printf '{"version":"1.5.4"}\n' > "$site/version.json"
  printf 'old-asset\n' > "$site/assets/app.js"
  cp "$WORK/backend-unavailable.html" "$site/backend-unavailable.html"
  cp "$binary" "$site/backend/auth_pro"
  chmod 755 "$site/backend/auth_pro"
  openssl rand -hex 32 > "$site/backend/jwt.secret"
  chmod 600 "$site/backend/jwt.secret"
  cat > "$site/backend/baota.env" <<EOF
PORT=${port}
HOST=127.0.0.1
AUTO_PRO_DATA_DIR=${site}/backend
AUTO_PRO_FRONTEND_DIR=${site}
AUTO_PRO_PROCESS_MANAGER=${manager}
AUTO_PRO_UPDATE_URL=https://127.0.0.1:${update_port}/latest.json
AUTO_PRO_UPDATE_LOCAL_DIGEST=1
AUTO_PRO_UPDATE_HEALTH_TRIES=20
AUTO_PRO_SUPERVISOR_PROGRAM=auth_pro
AUTO_PRO_SUPERVISOR_CONF=${SUP_CONF:-}
AUTO_PRO_DB_HOST=127.0.0.1
AUTO_PRO_DB_PORT=3306
AUTO_PRO_DB_NAME=auth_pro_sim
AUTO_PRO_DB_USER=auth_pro_sim
AUTO_PRO_DB_PASSWORD=sim-pass
GIN_MODE=release
EOF
  chmod 600 "$site/backend/baota.env"
  if [[ "$manager" != "none" ]]; then
    printf 'supervisor\n' > "$site/backend/process-manager"
  fi
  cat > "$site/backend/start.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$(readlink -f "$0" 2>/dev/null || realpath "$0")")"
if [[ -f ./baota.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source ./baota.env
  set +a
fi
export AUTO_PRO_PROCESS_MANAGER="${AUTO_PRO_PROCESS_MANAGER:-supervisor}"
exec ./auth_pro
EOF
  chmod 755 "$site/backend/start.sh"
}

wait_version() {
  local port="$1" expect="${2:-}" i body
  for i in $(seq 1 60); do
    body="$(curl -fsS --max-time 2 "http://127.0.0.1:${port}/api/system/version" 2>/dev/null || true)"
    if [[ -n "$expect" ]]; then
      if printf '%s' "$body" | grep -Fq "\"version\":\"${expect}\""; then
        printf '%s' "$body"
        return 0
      fi
    elif [[ -n "$body" ]]; then
      printf '%s' "$body"
      return 0
    fi
    sleep 1
  done
  return 1
}

listener_pids() {
  local port="$1"
  ss -lptn "sport = :${port}" 2>/dev/null | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | sort -u
}

apply_update() {
  local port="$1" token="$2"
  curl -sS --max-time 30 -H "Authorization: Bearer ${token}" -H 'Content-Type: application/json' \
    -X POST "http://127.0.0.1:${port}/api/system/update/apply"
}

poll_job() {
  local port="$1" token="$2" id="$3" i body status
  for i in $(seq 1 90); do
    body="$(curl -sS --max-time 3 -H "Authorization: Bearer ${token}" \
      "http://127.0.0.1:${port}/api/system/update/jobs/${id}" 2>/dev/null || true)"
    status="$(printf '%s' "$body" | python3 -c 'import json,sys
raw=sys.stdin.read().strip()
if not raw:
    print("")
    raise SystemExit
try:
    data=json.loads(raw)
except Exception:
    print("")
    raise SystemExit
job=data.get("data") or {}
print(job.get("status") or "")' 2>/dev/null || true)"
    if [[ "$status" == "success" || "$status" == "failed" ]]; then
      printf '%s' "$body"
      return 0
    fi
    sleep 1
  done
  printf '%s' "$body"
  return 1
}

SUP_CONF="$WORK/supervisord.conf"
SUP_SOCK="$WORK/supervisor.sock"
cat > "$SUP_CONF" <<EOF
[supervisord]
nodaemon=false
logfile=$WORK/supervisord.log
pidfile=$WORK/supervisord.pid
childlogdir=$WORK

[unix_http_server]
file=$SUP_SOCK

[supervisorctl]
serverurl=unix://$SUP_SOCK

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

[program:auth_pro]
command=%(here)s/unused
directory=/tmp
autostart=false
autorestart=false
EOF

# 占位，后面每个场景覆盖 program 段。supervisord 先用一份只含 rpc 的配置启动，再 reread。
cat > "$SUP_CONF" <<EOF
[supervisord]
nodaemon=false
logfile=$WORK/supervisord.log
pidfile=$WORK/supervisord.pid
childlogdir=$WORK

[unix_http_server]
file=$SUP_SOCK

[supervisorctl]
serverurl=unix://$SUP_SOCK

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

[program:auth_pro]
command=/bin/sleep 3600
directory=/tmp
autostart=false
autorestart=false
startsecs=0
stdout_logfile=$WORK/sleep.out
stderr_logfile=$WORK/sleep.err
EOF
# supervisorctl status：0 表示进程都在跑，3 表示守护已连通但有程序停止。
# 演练里 auth_pro 默认 autostart=false，停止状态是预期，不能当成守护没起来。
supervisor_ping() {
  local code
  supervisorctl -c "$SUP_CONF" status >/dev/null 2>&1
  code=$?
  [[ "$code" -eq 0 || "$code" -eq 3 ]]
}

supervisord -c "$SUP_CONF"
for _ in $(seq 1 30); do
  if supervisor_ping; then
    break
  fi
  sleep 0.2
done
if ! supervisor_ping; then
  echo "---- supervisord.log ----"
  cat "$WORK/supervisord.log" 2>/dev/null || true
  fail "supervisord 没有起来"
fi

run_supervised() {
  local name="$1" binary="$2" package_bin="$3" version="$4" index_text="$5" expect_status="$6" expect_version="$7"
  local port update_port site src
  port="$(free_port)"
  update_port="$(free_port)"
  site="$WORK/$name/site"
  src="$WORK/$name/src"
  mkdir -p "$src"
  UPDATE_PORT="$update_port" make_package "$version" "$package_bin" "$index_text" "$src"
  write_site "$site" "$port" "$update_port" supervisor "$binary"
  # 把本场景的启动命令写进 supervisord。先停掉上一个程序。
  supervisorctl -c "$SUP_CONF" stop auth_pro >/dev/null 2>&1 || true
  python3 - "$SUP_CONF" "$site" "$port" <<'PY'
import pathlib, sys
conf, site, port = sys.argv[1:]
text = pathlib.Path(conf).read_text()
start = text.index("[program:auth_pro]")
head = text[:start]
block = f"""[program:auth_pro]
command={site}/backend/start.sh
directory={site}/backend
autostart=false
autorestart=true
startsecs=0
startretries=40
stopsignal=TERM
stopwaitsecs=12
stdout_logfile={site}/backend/logs/supervisor.out
stderr_logfile={site}/backend/logs/supervisor.err
environment=PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
"""
pathlib.Path(conf).write_text(head + block)
PY
  supervisorctl -c "$SUP_CONF" reread >/dev/null
  supervisorctl -c "$SUP_CONF" update >/dev/null
  supervisorctl -c "$SUP_CONF" start auth_pro
  local old_body old_pid
  old_body="$(wait_version "$port" "1.5.4")" || {
    echo "---- server log ----"
    tail -n 80 "$site/backend/logs/supervisor.err" "$site/backend/logs/auto_pro.log" 2>/dev/null || true
    fail "$name 旧版本没有监听到 $port"
  }
  printf '旧版本响应 %s\n' "$old_body"
  old_pid="$(listener_pids "$port" | head -n 1)"
  [[ -n "$old_pid" ]] || fail "$name 没有监听进程"
  local parent_comm
  parent_comm="$(ps -o comm= -p "$(ps -o ppid= -p "$old_pid" | tr -d ' ')" | tr -d ' ')"
  [[ "$parent_comm" == "supervisord" ]] || fail "$name 启动后父进程是 $parent_comm，不是 supervisord"
  start_update_source "$src/www" "$update_port"
  local token apply_body job_id
  token="$(sign_token "$site/backend/jwt.secret")"
  apply_body="$(apply_update "$port" "$token")"
  printf '申请更新 %s\n' "$apply_body"
  printf '%s' "$apply_body" | grep -q '"code":200' || fail "$name 申请更新失败: $apply_body"
  job_id="$(printf '%s' "$apply_body" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"
  local job_body
  if ! job_body="$(poll_job "$port" "$token" "$job_id")"; then
    echo "---- update job ----"
    printf '%s\n' "$job_body"
    echo "---- script log ----"
    tail -n 80 "$site/backend/logs/"*.log 2>/dev/null || true
    fail "$name 更新任务没有在时限内结束"
  fi
  printf '任务结果 %s\n' "$job_body"
  printf '%s' "$job_body" | grep -q "\"status\":\"${expect_status}\"" || fail "$name 任务状态不是 ${expect_status}: $job_body"
  local new_body listeners ppid comm
  new_body="$(wait_version "$port" "$expect_version")" || fail "$name 重启后版本不是 ${expect_version}"
  printf '新版本响应 %s\n' "$new_body"
  mapfile -t listeners < <(listener_pids "$port")
  [[ "${#listeners[@]}" -eq 1 ]] || fail "$name 端口上有 ${#listeners[@]} 个监听进程: ${listeners[*]-}"
  [[ "${listeners[0]}" != "$old_pid" ]] || fail "$name 仍是更新前的进程 $old_pid"
  ppid="$(ps -o ppid= -p "${listeners[0]}" | tr -d ' ')"
  comm="$(ps -o comm= -p "$ppid" | tr -d ' ')"
  [[ "$comm" == "supervisord" ]] || fail "$name 新进程父进程是 $comm"
  kill -0 "$old_pid" 2>/dev/null && fail "$name 旧进程 $old_pid 还在"
  if [[ "$expect_status" == "success" ]]; then
    grep -q "$index_text" "$site/index.html" || fail "$name 前端没有切到新版本"
    printf '%s\n' "$job_body" > "$ART/${name}-job.json"
  else
    grep -q 'old-index' "$site/index.html" || fail "$name 回滚后前端不是旧页面"
    file "$site/backend/auth_pro" | grep -q 'ELF' || fail "$name 回滚后二进制不是 ELF"
    printf '%s' "$job_body" | grep -q '回滚' || fail "$name 失败原因没有写明回滚"
    printf '%s\n' "$job_body" > "$ART/${name}-job.json"
  fi
  cp "$site/backend/logs/"*.log "$ART/" 2>/dev/null || true
  supervisorctl -c "$SUP_CONF" stop auth_pro >/dev/null 2>&1 || true
  ok "$name"
}

run_supervised success "$WORK/auth_pro_1.5.4" "$WORK/auth_pro_1.5.5" 1.5.5 "new-index-1.5.5" success 1.5.5

printf '#!/bin/sh\nprintf "broken\\n" >&2\nexit 1\n' > "$WORK/broken-auth-pro"
chmod 755 "$WORK/broken-auth-pro"
run_supervised rollback "$WORK/auth_pro_1.5.4" "$WORK/broken-auth-pro" 1.5.5 "broken-index" failed 1.5.4

# 无守护：自行拉起，端口上只留新进程。
STAND_PORT="$(free_port)"
STAND_UPDATE="$(free_port)"
STAND_SITE="$WORK/standalone/site"
STAND_SRC="$WORK/standalone/src"
mkdir -p "$STAND_SRC"
UPDATE_PORT="$STAND_UPDATE" make_package 1.5.5 "$WORK/auth_pro_1.5.5" "standalone-new-index" "$STAND_SRC"
write_site "$STAND_SITE" "$STAND_PORT" "$STAND_UPDATE" none "$WORK/auth_pro_1.5.4"
rm -f "$STAND_SITE/backend/process-manager"
(
  cd "$STAND_SITE/backend"
  set -a
  # shellcheck disable=SC1091
  source ./baota.env
  set +a
  export AUTO_PRO_PROCESS_MANAGER=none
  ./auth_pro >> "$STAND_SITE/backend/logs/auto_pro.log" 2>&1 &
  echo $! > "$STAND_SITE/backend/auto_pro.pid"
)
STAND_OLD="$(cat "$STAND_SITE/backend/auto_pro.pid")"
PIDS+=("$STAND_OLD")
wait_version "$STAND_PORT" "1.5.4" >/dev/null || {
  tail -n 80 "$STAND_SITE/backend/logs/auto_pro.log" || true
  fail "无守护旧版本没有起来"
}
start_update_source "$STAND_SRC/www" "$STAND_UPDATE"
STAND_TOKEN="$(sign_token "$STAND_SITE/backend/jwt.secret")"
STAND_APPLY="$(apply_update "$STAND_PORT" "$STAND_TOKEN")"
printf '无守护申请 %s\n' "$STAND_APPLY"
printf '%s' "$STAND_APPLY" | grep -q '"code":200' || fail "无守护申请更新失败: $STAND_APPLY"
STAND_JOB="$(printf '%s' "$STAND_APPLY" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"
STAND_RESULT="$(poll_job "$STAND_PORT" "$STAND_TOKEN" "$STAND_JOB")" || fail "无守护更新没有结束: $STAND_RESULT"
printf '无守护任务 %s\n' "$STAND_RESULT"
printf '%s' "$STAND_RESULT" | grep -q '"status":"success"' || fail "无守护更新没有成功: $STAND_RESULT"
wait_version "$STAND_PORT" "1.5.5" >/dev/null || fail "无守护重启后版本不是 1.5.5"
mapfile -t STAND_LISTENERS < <(listener_pids "$STAND_PORT")
[[ "${#STAND_LISTENERS[@]}" -eq 1 ]] || fail "无守护端口监听数不是 1: ${STAND_LISTENERS[*]-}"
PIDS+=("${STAND_LISTENERS[0]}")
kill -0 "$STAND_OLD" 2>/dev/null && fail "无守护旧进程还在"
grep -q 'standalone-new-index' "$STAND_SITE/index.html" || fail "无守护前端没有切换"
printf '%s\n' "$STAND_RESULT" > "$ART/standalone-job.json"
ok "无守护自重启"

spawn_orphan() {
  python3 - "$1" <<'PY'
import os, sys
path = sys.argv[1]
if os.fork() > 0:
    os._exit(0)
os.setsid()
if os.fork() > 0:
    os._exit(0)
os.execv(path, [path])
PY
}

point_supervisor() {
  local site="$1"
  supervisorctl -c "$SUP_CONF" stop auth_pro >/dev/null 2>&1 || true
  python3 - "$SUP_CONF" "$site" <<'PY'
import pathlib, sys
conf, site = sys.argv[1:]
text = pathlib.Path(conf).read_text()
start = text.index("[program:auth_pro]")
block = f"""[program:auth_pro]
command={site}/backend/start.sh
directory={site}/backend
autostart=false
autorestart=true
startsecs=0
startretries=40
stopsignal=TERM
stopwaitsecs=12
stdout_logfile={site}/backend/logs/supervisor.out
stderr_logfile={site}/backend/logs/supervisor.err
environment=PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
"""
pathlib.Path(conf).write_text(text[:start] + block)
PY
  supervisorctl -c "$SUP_CONF" reread >/dev/null
  supervisorctl -c "$SUP_CONF" update >/dev/null
}

# 复现事故：父进程为 1 的本站 auth_pro 占着端口。在线更新必须先停掉它，再由 supervisord 拉起新版。
ORPHAN_PORT="$(free_port)"
ORPHAN_UPDATE="$(free_port)"
ORPHAN_SITE="$WORK/orphan-online/site"
ORPHAN_SRC="$WORK/orphan-online/src"
mkdir -p "$ORPHAN_SRC"
UPDATE_PORT="$ORPHAN_UPDATE" make_package 1.5.5 "$WORK/auth_pro_1.5.5" "orphan-new-index" "$ORPHAN_SRC"
write_site "$ORPHAN_SITE" "$ORPHAN_PORT" "$ORPHAN_UPDATE" supervisor "$WORK/auth_pro_1.5.4"
point_supervisor "$ORPHAN_SITE"
spawn_orphan "$ORPHAN_SITE/backend/start.sh"
ORPHAN_BODY="$(wait_version "$ORPHAN_PORT" "1.5.4")" || {
  tail -n 40 "$ORPHAN_SITE/backend/logs/auto_pro.log" 2>/dev/null || true
  fail "孤儿旧版本没有起来"
}
printf '孤儿在线更新前 %s\n' "$ORPHAN_BODY"
ORPHAN_OLD="$(listener_pids "$ORPHAN_PORT" | head -n 1)"
ORPHAN_PPID="$(ps -o ppid= -p "$ORPHAN_OLD" | tr -d ' ')"
[[ "$ORPHAN_PPID" == "1" ]] || fail "在线更新前的旧进程 PPID 不是 1（当前 ${ORPHAN_PPID}）"
start_update_source "$ORPHAN_SRC/www" "$ORPHAN_UPDATE"
ORPHAN_TOKEN="$(sign_token "$ORPHAN_SITE/backend/jwt.secret")"
ORPHAN_APPLY="$(apply_update "$ORPHAN_PORT" "$ORPHAN_TOKEN")"
printf '孤儿申请 %s\n' "$ORPHAN_APPLY"
printf '%s' "$ORPHAN_APPLY" | grep -q '"code":200' || fail "孤儿在线更新申请失败: $ORPHAN_APPLY"
ORPHAN_JOB="$(printf '%s' "$ORPHAN_APPLY" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"
ORPHAN_RESULT="$(poll_job "$ORPHAN_PORT" "$ORPHAN_TOKEN" "$ORPHAN_JOB")" || {
  tail -n 80 "$ORPHAN_SITE/backend/logs/"*.log 2>/dev/null || true
  fail "孤儿在线更新没有结束: $ORPHAN_RESULT"
}
printf '孤儿任务 %s\n' "$ORPHAN_RESULT"
printf '%s' "$ORPHAN_RESULT" | grep -q '"status":"success"' || fail "孤儿在线更新没有成功: $ORPHAN_RESULT"
wait_version "$ORPHAN_PORT" "1.5.5" >/dev/null || fail "孤儿更新后版本不是 1.5.5"
kill -0 "$ORPHAN_OLD" 2>/dev/null && fail "PPID=1 的旧进程还在"
mapfile -t ORPHAN_LISTENERS < <(listener_pids "$ORPHAN_PORT")
[[ "${#ORPHAN_LISTENERS[@]}" -eq 1 ]] || fail "孤儿更新后监听数不是 1: ${ORPHAN_LISTENERS[*]-}"
ORPHAN_NEW_PPID="$(ps -o ppid= -p "${ORPHAN_LISTENERS[0]}" | tr -d ' ')"
ORPHAN_NEW_COMM="$(ps -o comm= -p "$ORPHAN_NEW_PPID" | tr -d ' ')"
[[ "$ORPHAN_NEW_COMM" == "supervisord" ]] || fail "孤儿更新后父进程是 $ORPHAN_NEW_COMM"
grep -q 'orphan-new-index' "$ORPHAN_SITE/index.html" || fail "孤儿更新后前端没有切换"
printf '%s\n' "$ORPHAN_RESULT" > "$ART/orphan-online-job.json"
ok "在线更新清掉 PPID=1 的旧进程并由守护启动新版"

# 不响应的孤儿：占着端口、忽略 SIGTERM。baota-upgrade.sh 要 SIGKILL 它，再由守护启动新版。
HUNG_PORT="$(free_port)"
HUNG_SITE="$WORK/orphan-upgrade/site"
write_site "$HUNG_SITE" "$HUNG_PORT" "9" supervisor "$WORK/auth_pro_1.5.4"
cat > "$HUNG_SITE/backend/auth_pro" <<'EOF'
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
chmod 755 "$HUNG_SITE/backend/auth_pro"
printf 'installed\n' > "$HUNG_SITE/backend/install.lock"
point_supervisor "$HUNG_SITE"
PORT="$HUNG_PORT" spawn_orphan "$HUNG_SITE/backend/auth_pro"
HUNG_OLD=""
for _ in $(seq 1 20); do
  HUNG_OLD="$(listener_pids "$HUNG_PORT" | head -n 1)"
  [[ -n "$HUNG_OLD" ]] && break
  sleep 0.2
done
[[ -n "$HUNG_OLD" ]] || fail "不响应的孤儿没有占上端口"
PIDS+=("$HUNG_OLD")
HUNG_PPID="$(ps -o ppid= -p "$HUNG_OLD" | tr -d ' ')"
[[ "$HUNG_PPID" == "1" ]] || fail "不响应孤儿的 PPID 不是 1（当前 ${HUNG_PPID}）"
curl -fsS --max-time 1 "http://127.0.0.1:${HUNG_PORT}/api/system/version" >/dev/null 2>&1 && fail "这个孤儿不应该响应健康检查"
KEEP_PORT="$(free_port)"
KEEP_SITE="$WORK/other-site"
mkdir -p "$KEEP_SITE/backend"
cat > "$KEEP_SITE/backend/auth_pro" <<'EOF'
#!/usr/bin/env python3
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"other-site")
    def log_message(self, fmt, *args):
        return
ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
EOF
chmod 755 "$KEEP_SITE/backend/auth_pro"
PORT="$KEEP_PORT" "$KEEP_SITE/backend/auth_pro" &
KEEP_PID=$!
PIDS+=("$KEEP_PID")
for _ in $(seq 1 20); do
  curl -fsS --max-time 1 "http://127.0.0.1:${KEEP_PORT}/" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsS --max-time 1 "http://127.0.0.1:${KEEP_PORT}/" >/dev/null || fail "其它站点没有起来"
UP_PAYLOAD="$WORK/upgrade-payload"
mkdir -p "$UP_PAYLOAD/assets" "$UP_PAYLOAD/backend"
cp "$WORK/auth_pro_1.5.5" "$UP_PAYLOAD/backend/auth_pro"
chmod 755 "$UP_PAYLOAD/backend/auth_pro"
printf '<html>upgrade-new</html>\n' > "$UP_PAYLOAD/index.html"
printf '{"version":"1.5.5"}\n' > "$UP_PAYLOAD/version.json"
printf '{"version":"1.5.5","frontendDir":".","backendFile":"backend/auth_pro","requiredFiles":[]}\n' > "$UP_PAYLOAD/manifest.json"
printf 'asset-new\n' > "$UP_PAYLOAD/assets/app.js"
cp "$WORK/backend-unavailable.html" "$UP_PAYLOAD/backend-unavailable.html"
AUTH_PRO_TERM_WAIT=3 AUTH_PRO_YES=1 AUTH_PRO_START=1 AUTH_PRO_SKIP_MYSQL=1 \
  bash "$ROOT/scripts/baota-upgrade.sh" \
    --site-root "$HUNG_SITE" \
    --source "$UP_PAYLOAD" >"$ART/orphan-upgrade.out" 2>"$ART/orphan-upgrade.err"
grep -q 'SIGKILL' "$ART/orphan-upgrade.out" "$ART/orphan-upgrade.err" || fail "升级没有在超时后 SIGKILL 不响应的孤儿"
kill -0 "$HUNG_OLD" 2>/dev/null && fail "不响应的孤儿还在"
wait_version "$HUNG_PORT" "1.5.5" >/dev/null || {
  echo "---- upgrade out ----"
  cat "$ART/orphan-upgrade.out" "$ART/orphan-upgrade.err" || true
  tail -n 40 "$HUNG_SITE/backend/logs/supervisor.err" "$HUNG_SITE/backend/logs/auto_pro.log" 2>/dev/null || true
  fail "升级后新版本没有起来"
}
HUNG_NEW="$(listener_pids "$HUNG_PORT" | head -n 1)"
HUNG_NEW_PPID="$(ps -o ppid= -p "$HUNG_NEW" | tr -d ' ')"
HUNG_NEW_COMM="$(ps -o comm= -p "$HUNG_NEW_PPID" | tr -d ' ')"
[[ "$HUNG_NEW_COMM" == "supervisord" ]] || fail "升级后的父进程是 $HUNG_NEW_COMM"
kill -0 "$KEEP_PID" 2>/dev/null || fail "升级误杀了其它站点"
curl -fsS --max-time 1 "http://127.0.0.1:${KEEP_PORT}/" >/dev/null || fail "其它站点不再响应"
grep -q 'upgrade-new' "$HUNG_SITE/index.html" || fail "升级没有换上新页面"
supervisorctl -c "$SUP_CONF" stop auth_pro >/dev/null 2>&1 || true
ok "baota-upgrade 清掉不响应的孤儿并由守护启动新版"

# nginx：后端端口没有进程时，502 返回静态说明页。
NGINX_PORT="$(free_port)"
NGINX_CONF="$WORK/nginx.conf"
NGINX_PID="$WORK/nginx.pid"
NGINX_SITE="$WORK/nginx-site"
mkdir -p "$NGINX_SITE" "$WORK/nginx-logs" "$WORK/nginx-temp"/{body,proxy,fastcgi,uwsgi,scgi}
cp "$WORK/backend-unavailable.html" "$NGINX_SITE/backend-unavailable.html"
cat > "$NGINX_CONF" <<EOF
worker_processes 1;
error_log $WORK/nginx-logs/error.log;
pid $NGINX_PID;
events { worker_connections 64; }
http {
  access_log $WORK/nginx-logs/access.log;
  client_body_temp_path $WORK/nginx-temp/body;
  proxy_temp_path $WORK/nginx-temp/proxy;
  fastcgi_temp_path $WORK/nginx-temp/fastcgi;
  uwsgi_temp_path $WORK/nginx-temp/uwsgi;
  scgi_temp_path $WORK/nginx-temp/scgi;
  server {
    listen 127.0.0.1:${NGINX_PORT};
    server_name localhost;
    error_page 502 503 504 /backend-unavailable.html;
    location = /backend-unavailable.html {
      root ${NGINX_SITE};
      default_type text/html;
    }
    location / {
      proxy_pass http://127.0.0.1:1;
    }
  }
}
EOF
nginx -t -c "$NGINX_CONF"
nginx -c "$NGINX_CONF"
NGINX_BODY="$(curl -sS -D "$ART/nginx-headers.txt" "http://127.0.0.1:${NGINX_PORT}/" || true)"
printf '%s\n' "$NGINX_BODY" > "$ART/nginx-body.html"
printf '%s' "$NGINX_BODY" | grep -q '后台服务暂时无法连接' || fail "nginx 502 没有返回静态说明页"
grep -q 'HTTP/1.1 502' "$ART/nginx-headers.txt" || fail "nginx 没有返回 502"
ok "nginx 502 返回静态错误页"

printf '\n演练完成。日志：%s\n' "$LOG"
