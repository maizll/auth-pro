#!/usr/bin/env bash
# 发版前核对当前提交是否有 box 三道关卡成功证明（Commit Status auth-pro/box-gate）。
# 由仓库变量 AUTH_PRO_REQUIRE_BOX_GATE 控制：未设置或为 1 时强制要求；设为 0 可临时关闭（不推荐）。
set -euo pipefail

REPO="${GITHUB_REPOSITORY:-maizll/auth-pro}"
SHA="${GITHUB_SHA:-}"
CONTEXT="auth-pro/box-gate"
REQUIRE="${AUTH_PRO_REQUIRE_BOX_GATE:-${REQUIRE_BOX_GATE:-1}}"

if [[ -z "$SHA" ]]; then
  echo "缺少 GITHUB_SHA" >&2
  exit 1
fi

if [[ "$REQUIRE" == "0" || "$REQUIRE" == "false" || "$REQUIRE" == "off" ]]; then
  echo "警告：AUTH_PRO_REQUIRE_BOX_GATE=${REQUIRE}，跳过 box 关卡证明（仅应急）" >&2
  exit 0
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "需要 gh" >&2
  exit 1
fi

echo "核对 ${CONTEXT} @ ${SHA:0:12} …"
# combined status 里找我们的 context；必须 success，且 description 非空。
json="$(gh api "repos/${REPO}/commits/${SHA}/status")"
state="$(printf '%s' "$json" | node -e '
  const fs = require("fs")
  const data = JSON.parse(fs.readFileSync(0, "utf8"))
  const ctx = process.argv[1]
  const hits = (data.statuses || []).filter((s) => s.context === ctx)
  if (!hits.length) {
    console.log("missing")
    process.exit(0)
  }
  // GitHub 按时间倒序；取最新一条。
  hits.sort((a, b) => Date.parse(b.updated_at || b.created_at || 0) - Date.parse(a.updated_at || a.created_at || 0))
  const latest = hits[0]
  console.log([latest.state || "", latest.description || ""].join("\t"))
' "$CONTEXT")"

if [[ "$state" == "missing" ]]; then
  echo "缺少 ${CONTEXT} 证明。请在 box 上三道关卡全过后运行：" >&2
  echo "  ./scripts/ci/record-box-gate.sh \"QC0 … CROSS n/n\"" >&2
  echo "详见 docs/ci-gates.md" >&2
  exit 1
fi

status="$(printf '%s' "$state" | cut -f1)"
desc="$(printf '%s' "$state" | cut -f2-)"
echo "最新状态: ${status} — ${desc}"

if [[ "$status" != "success" ]]; then
  echo "${CONTEXT} 不是 success（${status}），拒绝发版" >&2
  exit 1
fi

echo "box 关卡证明通过"
