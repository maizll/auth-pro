#!/usr/bin/env bash
# 合并和发版前的质量门槛。任一检查失败就退出。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="$(go env GOPATH)/bin:${PATH}"

if ! command -v staticcheck >/dev/null 2>&1; then
  go install honnef.co/go/tools/cmd/staticcheck@v0.5.1
fi

echo "== go vet =="
(cd "${ROOT}/backend" && go vet ./...)

echo "== staticcheck =="
(cd "${ROOT}/backend" && staticcheck ./...)

echo "== vue-tsc =="
(cd "${ROOT}/frontend" && pnpm exec vue-tsc --noEmit)

echo "== eslint =="
(cd "${ROOT}/frontend" && pnpm exec eslint . --max-warnings 0)

echo "== 未使用导出 =="
node "${ROOT}/scripts/check-unused-exports.mjs"

echo "质量检查通过"
