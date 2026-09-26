#!/usr/bin/env bash
# auth-pro-guardian-start
# 宝塔进程守护、supervisor 或 systemd 的启动命令。运行目录是本文件所在的 backend/。
# 没有待验证更新时直接执行 auth_pro，不额外留一个壳进程。
# 在线更新会先替换程序并写入 updates/pending-restart/handoff.sh，再让当前进程退出。
# 守护拉起本脚本后，由这里做有限次健康检查；失败则还原备份再执行旧程序。
# 不要 nohup 出脱离守护的进程。
set -euo pipefail
cd "$(dirname "$(readlink -f "$0" 2>/dev/null || realpath "$0")")"
if [[ -f ./baota.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source ./baota.env
  set +a
fi
export AUTO_PRO_PROCESS_MANAGER="${AUTO_PRO_PROCESS_MANAGER:-supervisor}"

HANDOFF="./updates/pending-restart/handoff.sh"
if [[ ! -f "$HANDOFF" ]]; then
  exec ./auth_pro
fi

# shellcheck disable=SC1090
source "$HANDOFF"
mkdir -p "$(dirname "$LOG_FILE")" ./logs ./updates/pending-restart

attempt=0
if [[ -f ./updates/pending-restart/attempts ]]; then
  attempt="$(tr -cd '0-9' < ./updates/pending-restart/attempts || true)"
fi
attempt="${attempt:-0}"
max_attempts="${MAX_ATTEMPTS:-2}"
child=0

stop_child() {
  local pid="$1" i
  [[ "$pid" =~ ^[0-9]+$ ]] || return 0
  [[ "$pid" -gt 1 ]] || return 0
  kill -0 "$pid" 2>/dev/null || return 0
  kill -TERM "$pid" 2>/dev/null || true
  i=0
  while [[ "$i" -lt 8 ]]; do
    kill -0 "$pid" 2>/dev/null || return 0
    i=$((i + 1))
    sleep 1
  done
  kill -KILL "$pid" 2>/dev/null || true
  sleep 1
}

trap 'if [[ "${child:-0}" -gt 1 ]]; then kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi; exit 143' TERM INT

while [[ "$attempt" -lt "$max_attempts" ]]; do
  attempt=$((attempt + 1))
  printf '%s\n' "$attempt" > ./updates/pending-restart/attempts
  if ! handoff_wait_port_free; then
    handoff_rollback "端口 ${PORT} 一直被占用，已回滚到 ${OLD_VERSION}。本次没有再启动第二个进程。请确认占用者是本站 backend/auth_pro 后，只由进程守护启动 start.sh。"
    exit 1
  fi
  ./auth_pro &
  child=$!
  if handoff_health_ok "$child"; then
    handoff_mark_success
    set +e
    wait "$child"
    status=$?
    set -e
    exit "$status"
  fi
  stop_child "$child"
  child=0
done

handoff_rollback "新版本连续 ${max_attempts} 次没有通过健康检查，已回滚到 ${OLD_VERSION}。网站会由进程守护继续运行旧版本。"
exec ./auth_pro
