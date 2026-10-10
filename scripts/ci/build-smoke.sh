#!/usr/bin/env bash
# PR/push 用的打包冒烟：不依赖正式发布私钥，打未签名包并核对目录结构。
# 正式发版仍走 release.yml + AUTH_PRO_REQUIRE_UPDATE_SIGNATURE=1。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "VERSION 无效: ${VERSION}" >&2
  exit 1
fi

NOTES_FILE="$ROOT/docs/release-notes-${VERSION}.txt"
if [[ -f "$NOTES_FILE" ]]; then
  export AUTO_PRO_RELEASE_NOTES
  AUTO_PRO_RELEASE_NOTES="$(cat "$NOTES_FILE")"
else
  export AUTO_PRO_RELEASE_NOTES="ci build smoke ${VERSION}"
fi

# 冒烟不强制签名；发布工作流另设 AUTH_PRO_REQUIRE_UPDATE_SIGNATURE=1。
unset AUTH_PRO_UPDATE_SIGNING_KEY || true
unset AUTH_PRO_REQUIRE_UPDATE_SIGNATURE || true
export AUTH_PRO_PACKAGE_EDITION="${AUTH_PRO_PACKAGE_EDITION:-official}"
export AUTO_PRO_UPDATE_PACKAGE_BASE_URL="${AUTO_PRO_UPDATE_PACKAGE_BASE_URL:-https://example.invalid/releases/download/v${VERSION}}"
export AUTO_PRO_UPDATE_RELEASES_URL="${AUTO_PRO_UPDATE_RELEASES_URL:-https://example.invalid/releases/download/v${VERSION}/releases.json}"

echo "== build-release.sh ${VERSION}（未签名冒烟） =="
(cd "$ROOT" && ./scripts/build-release.sh "$VERSION")

PACKAGE="$ROOT/release/packages/auth_pro-full-v${VERSION}.tar.gz"
test -s "$PACKAGE"
VERIFY_DIR="$(mktemp -d)"
trap 'rm -rf "$VERIFY_DIR"' EXIT

tar -xzf "$PACKAGE" -C "$VERIFY_DIR" \
  ./manifest.json \
  ./backend/auth_pro \
  ./index.html \
  ./baota-panel.py \
  ./guardian-start.sh \
  ./backend/release.json

file "$VERIFY_DIR/backend/auth_pro" | grep -Eq 'ELF 64-bit.*x86-64'
bash -n "$VERIFY_DIR/guardian-start.sh"
python3 -m py_compile "$VERIFY_DIR/baota-panel.py"
grep -q 'auth-pro-guardian-start' "$VERIFY_DIR/guardian-start.sh"
test ! -e "$VERIFY_DIR/install.sh"
if tar -tzf "$PACKAGE" | grep -Eq '(^|/)install\.sh$'; then
  echo "冒烟包里不能有 install.sh" >&2
  exit 1
fi
test ! -e "$VERIFY_DIR/baota-install.sh"
test ! -e "$VERIFY_DIR/baota-upgrade.sh"
test ! -e "$VERIFY_DIR/baota-lib.sh"

node -e '
  const fs = require("node:fs")
  const path = require("node:path")
  const version = process.argv[1]
  const verifyDir = process.argv[2]
  const edition = process.env.AUTH_PRO_PACKAGE_EDITION
  const inner = JSON.parse(fs.readFileSync(path.join(verifyDir, "manifest.json"), "utf8"))
  const info = JSON.parse(fs.readFileSync(path.join(verifyDir, "backend", "release.json"), "utf8"))
  if (inner.version !== version || inner.frontendDir !== "." || inner.backendFile !== "backend/auth_pro") {
    throw new Error("package manifest mismatch")
  }
  if (info.version !== version) throw new Error("package release info version mismatch")
  if (info.edition !== edition) throw new Error("package edition mismatch")
' "$VERSION" "$VERIFY_DIR"

echo "build smoke 通过（未签名包结构核对完成）"
