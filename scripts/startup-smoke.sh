#!/usr/bin/env bash
# 用真实二进制对官网和客户站各做一次启动、探活和重启。
# 官网预置签名私钥、GitHub 存储位置和收费仓库设置。GitHub 走不可达代理，失败也不能把进程打崩。
# 只在 CI 或本机有 MySQL 时跑，不打进发布包。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND="${ROOT}/backend"
DB_HOST="${AUTO_PRO_DB_HOST:-127.0.0.1}"
DB_PORT="${AUTO_PRO_DB_PORT:-3306}"
DB_USER="${AUTO_PRO_DB_USER:-root}"
DB_PASSWORD="${AUTO_PRO_DB_PASSWORD:-root}"
READY_SECONDS="${SMOKE_READY_SECONDS:-90}"
WORK="$(mktemp -d)"
BIN="${WORK}/auth-pro"
FRONTEND="${WORK}/frontend"
PUB=""
PIDS=()

cleanup() {
  local pid
  for pid in "${PIDS[@]:-}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill -TERM "${pid}" 2>/dev/null || true
      wait "${pid}" 2>/dev/null || true
    fi
  done
  rm -rf "${WORK}"
}
trap cleanup EXIT

mysql_exec() {
  mysql --protocol=TCP -h "${DB_HOST}" -P "${DB_PORT}" -u "${DB_USER}" -p"${DB_PASSWORD}" "$@"
}

echo "== 等待 MySQL =="
ready=0
for _ in $(seq 1 30); do
  if mysql_exec -e "SELECT 1" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "${ready}" != "1" ]]; then
  echo "连不上 MySQL ${DB_HOST}:${DB_PORT}" >&2
  exit 1
fi

echo "== 编译真实二进制 =="
mkdir -p "${FRONTEND}"
printf '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>auth-pro</title></head><body>auth-pro</body></html>\n' >"${FRONTEND}/index.html"

key_dir="${WORK}/keys"
mkdir -p "${key_dir}"
cat >"${WORK}/seal.go" <<'GO'
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir := os.Args[1]
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot-ed25519.key"), []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		panic(err)
	}
	paid := make([]byte, 32)
	storage := make([]byte, 32)
	if _, err := rand.Read(paid); err != nil {
		panic(err)
	}
	if _, err := rand.Read(storage); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "github-paid.key"), paid, 0o600); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "storage-locations.key"), storage, 0o600); err != nil {
		panic(err)
	}
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	fmt.Println(seal(paid, []byte("ghp_smoke_official")))
	fmt.Println(seal(storage, []byte("ghp_smoke_official")))
}

func seal(key, plain []byte) string {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	blob := append(nonce, gcm.Seal(nil, nonce, plain, nil)...)
	return base64.StdEncoding.EncodeToString(blob)
}
GO
PUB="$(cd "${WORK}" && go mod init smoke >/dev/null && go run ./seal.go "${key_dir}")"
mapfile -t KEY_LINES <<<"${PUB}"
PUB_B64="${KEY_LINES[0]}"
PAID_SEALED="${KEY_LINES[1]}"
STORAGE_SEALED="${KEY_LINES[2]}"
if [[ -z "${PUB_B64}" || -z "${PAID_SEALED}" || -z "${STORAGE_SEALED}" ]]; then
  echo "没有生成官网测试密钥" >&2
  exit 1
fi

(
  cd "${BACKEND}"
  go build -o "${BIN}" -ldflags "-X auto_pro/handler.embeddedStoreSnapshotPublicKey=${PUB_B64}" .
)

prepare_database() {
  local name="$1"
  mysql_exec -e "DROP DATABASE IF EXISTS \`${name}\`; CREATE DATABASE \`${name}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
  mysql_exec "${name}" <"${BACKEND}/handler/schema.sql"
}

seed_official() {
  local name="$1"
  local data="$2"
  mkdir -p "${data}/store"
  cp "${key_dir}/snapshot-ed25519.key" "${data}/store/snapshot-ed25519.key"
  cp "${key_dir}/github-paid.key" "${data}/store/github-paid.key"
  cp "${key_dir}/storage-locations.key" "${data}/store/storage-locations.key"
  chmod 600 "${data}/store/"*.key
  mysql_exec "${name}" <<SQL
CREATE TABLE IF NOT EXISTS source_station_settings (
  setting_key VARCHAR(50) NOT NULL PRIMARY KEY,
  setting_value TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS package_storage_state (
  id TINYINT NOT NULL PRIMARY KEY,
  payload MEDIUMTEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO apps (app_name, app_key, app_secret) VALUES ('授权系统', 'app_f93896d80066_5811', 'smoke-secret');
INSERT INTO source_station_settings (setting_key, setting_value) VALUES
  ('github_paid_owner', 'acme'),
  ('github_paid_repo', 'paid-packages'),
  ('github_paid_token', '${PAID_SEALED}');
INSERT INTO package_storage_state (id, payload) VALUES (1, '{"migrated":true,"locations":[{"id":"github-paid","name":"收费仓库","kind":"github","role":"primary","owner":"acme","repo":"paid-packages","secretSealed":"${STORAGE_SEALED}"}]}');
SQL
}

start_server() {
  local data="$1"
  local database="$2"
  local port="$3"
  local log="$4"
  AUTO_PRO_DATA_DIR="${data}" \
  AUTO_PRO_DB_HOST="${DB_HOST}" \
  AUTO_PRO_DB_PORT="${DB_PORT}" \
  AUTO_PRO_DB_NAME="${database}" \
  AUTO_PRO_DB_USER="${DB_USER}" \
  AUTO_PRO_DB_PASSWORD="${DB_PASSWORD}" \
  AUTO_PRO_FRONTEND_DIR="${FRONTEND}" \
  HOST=127.0.0.1 \
  PORT="${port}" \
  HTTP_PROXY="http://127.0.0.1:9" \
  HTTPS_PROXY="http://127.0.0.1:9" \
  NO_PROXY="127.0.0.1,localhost" \
  no_proxy="127.0.0.1,localhost" \
  GIN_MODE=release \
    "${BIN}" >"${log}" 2>&1 &
  LAST_PID=$!
  PIDS+=("${LAST_PID}")
}

stop_server() {
  local pid="$1"
  if kill -0 "${pid}" 2>/dev/null; then
    kill -TERM "${pid}" 2>/dev/null || true
    local i
    for i in $(seq 1 20); do
      if ! kill -0 "${pid}" 2>/dev/null; then
        wait "${pid}" 2>/dev/null || true
        return 0
      fi
      sleep 0.5
    done
    echo "进程 ${pid} 没有在限时内退出" >&2
    return 1
  fi
  wait "${pid}" 2>/dev/null || true
}

migration_marked() {
  local database="$1"
  local marked
  # 仓库绑定和按应用拆分商业版两步迁移都要写上标记。
  marked="$(mysql_exec -N -e "SELECT COUNT(*) FROM schema_migrations WHERE name IN ('app_repo_bindings_v1', 'per_app_commercial_v1')" "${database}" 2>/dev/null || true)"
  [[ "${marked}" == "2" ]]
}

assert_up() {
  local pid="$1"
  local port="$2"
  local log="$3"
  local label="$4"
  local database="$5"
  local i code version_ok home_ok marked=0
  for i in $(seq 1 "${READY_SECONDS}"); do
    if ! kill -0 "${pid}" 2>/dev/null; then
      echo "${label} 进程已退出" >&2
      cat "${log}" >&2 || true
      return 1
    fi
    code="$(curl -s -o /dev/null -w '%{http_code}' --noproxy '*' --max-time 2 "http://127.0.0.1:${port}/api/system/version" || true)"
    version_ok=0
    [[ "${code}" == "200" ]] && version_ok=1
    code="$(curl -s -o /dev/null -w '%{http_code}' --noproxy '*' --max-time 2 "http://127.0.0.1:${port}/" || true)"
    home_ok=0
    [[ "${code}" == "200" ]] && home_ok=1
    if migration_marked "${database}"; then
      marked=1
    fi
    if [[ "${version_ok}" == "1" && "${home_ok}" == "1" && "${marked}" == "1" ]]; then
      # 迁移标记写上之后，官网仓库迁移还在后台跑。再等一会儿，确认它没有把进程打崩。
      sleep 3
      if ! kill -0 "${pid}" 2>/dev/null; then
        echo "${label} 迁移结束后进程退出" >&2
        cat "${log}" >&2 || true
        return 1
      fi
      return 0
    fi
    sleep 1
  done
  echo "${label} 在 ${READY_SECONDS} 秒内没有同时满足端口、版本接口、首页和迁移完成" >&2
  cat "${log}" >&2 || true
  return 1
}

exercise() {
  local label="$1"
  local data="$2"
  local database="$3"
  local port="$4"
  local log="${WORK}/${label}.log"
  echo "== ${label} 第一次启动 =="
  start_server "${data}" "${database}" "${port}" "${log}"
  local pid="${LAST_PID}"
  assert_up "${pid}" "${port}" "${log}" "${label} 第一次启动" "${database}"
  echo "== ${label} 重启 =="
  stop_server "${pid}"
  : >"${log}"
  start_server "${data}" "${database}" "${port}" "${log}"
  pid="${LAST_PID}"
  assert_up "${pid}" "${port}" "${log}" "${label} 第二次启动" "${database}"
  stop_server "${pid}"
}

echo "== 客户站 =="
prepare_database auth_pro_smoke_customer
mkdir -p "${WORK}/customer"
exercise "客户站" "${WORK}/customer" auth_pro_smoke_customer 18181

echo "== 官网 =="
prepare_database auth_pro_smoke_official
seed_official auth_pro_smoke_official "${WORK}/official"
exercise "官网" "${WORK}/official" auth_pro_smoke_official 18182

echo "启动冒烟通过"
