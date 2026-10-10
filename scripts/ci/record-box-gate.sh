#!/usr/bin/env bash
# 在 box 上三道关卡全过后，把成功状态记到该提交的 GitHub Commit Status。
# release.yml 发版前会核对 context=auth-pro/box-gate 且 state=success。
#
# 用法：
#   ./scripts/ci/record-box-gate.sh "QC0 CYC0 GOTEST0 MYSQL 7/0/1 BAOTA0 BUILD0 BC0 REH 96/96 CROSS 278/278"
#   ./scripts/ci/record-box-gate.sh --sha <40位SHA> "……"
#   ./scripts/ci/record-box-gate.sh --fail "REH 失败摘要"
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="${GITHUB_REPOSITORY:-maizll/auth-pro}"
CONTEXT="auth-pro/box-gate"
STATE="success"
SHA=""
DESC=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha)
      SHA="${2:-}"
      shift 2
      ;;
    --fail)
      STATE="failure"
      shift
      ;;
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    -*)
      echo "未知参数: $1" >&2
      exit 2
      ;;
    *)
      DESC="$1"
      shift
      ;;
  esac
done

if [[ -z "$DESC" ]]; then
  echo "用法: $0 [--sha SHA] [--fail] \"关卡摘要\"" >&2
  exit 2
fi

# Commit Status 的 description 最长 140 字符。
DESC="$(printf '%s' "$DESC" | tr '\n' ' ' | cut -c1-140)"

if [[ -z "$SHA" ]]; then
  SHA="$(git -C "$ROOT" rev-parse HEAD)"
fi
if [[ ! "$SHA" =~ ^[0-9a-f]{40}$ ]]; then
  # 允许短 SHA，解析成完整。
  SHA="$(git -C "$ROOT" rev-parse "$SHA")"
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "需要已登录的 gh" >&2
  exit 1
fi

echo "记录 ${CONTEXT}=${STATE} @ ${SHA:0:12} (${REPO}): ${DESC}"
gh api --method POST "repos/${REPO}/statuses/${SHA}" \
  -f state="$STATE" \
  -f context="$CONTEXT" \
  -f description="$DESC" \
  -f target_url="${GITHUB_SERVER_URL:-https://github.com}/${REPO}/commit/${SHA}" \
  >/dev/null

echo "已写入 Commit Status"
