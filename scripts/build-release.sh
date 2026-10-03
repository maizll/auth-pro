#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# 发布签名私钥只交给第 4 步的签名工具。先收进不导出的变量，前端构建等子进程看不到它。
UPDATE_SIGNING_KEY="${AUTH_PRO_UPDATE_SIGNING_KEY:-}"
unset AUTH_PRO_UPDATE_SIGNING_KEY
VERSION_FILE="$ROOT_DIR/VERSION"
DEFAULT_VERSION="1.5.6"
if [[ -f "$VERSION_FILE" ]]; then
  DEFAULT_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"
fi
VERSION="${1:-$DEFAULT_VERSION}"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Version must match X.Y.Z: $VERSION" >&2
  exit 1
fi
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_DIR="$ROOT_DIR/backend"
DIST_NAME="auth_pro-full-v$VERSION"
RELEASE_ROOT="$ROOT_DIR/release"
PACKAGE_DIR="$RELEASE_ROOT/$DIST_NAME"
PACKAGES_DIR="$RELEASE_ROOT/packages"
PACKAGE_PATH="$PACKAGES_DIR/$DIST_NAME.tar.gz"
LATEST_PATH="$PACKAGES_DIR/latest.json"
RELEASES_PATH="$PACKAGES_DIR/releases.json"
RELEASE_REPOSITORY="${AUTO_PRO_RELEASE_REPOSITORY:-maizll/auth-pro-client}"
UPDATE_PACKAGE_BASE_URL="${AUTO_PRO_UPDATE_PACKAGE_BASE_URL:-https://github.com/$RELEASE_REPOSITORY/releases/download/v$VERSION}"
UPDATE_RELEASES_URL="${AUTO_PRO_UPDATE_RELEASES_URL:-https://github.com/$RELEASE_REPOSITORY/releases/download/v$VERSION/releases.json}"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
LDFLAGS="-s -w -X auto_pro/config.AppVersion=$VERSION -X auto_pro/config.BuildTime=$BUILD_TIME"
if [[ -n "${AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY:-}" ]]; then
  if [[ "${AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY}" == "PLACEHOLDER_NOT_CONFIGURED" ]]; then
    echo "AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY 仍是占位符，请改成源站 store-keygen 打印的公钥" >&2
    exit 1
  fi
  key_bytes="$(printf '%s' "$AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY" | openssl base64 -d -A 2>/dev/null | wc -c | tr -d '[:space:]')"
  if [[ "$key_bytes" != "32" ]]; then
    echo "AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY 必须是 32 字节 Ed25519 公钥的标准 base64" >&2
    exit 1
  fi
  LDFLAGS="$LDFLAGS -X auto_pro/handler.embeddedStoreSnapshotPublicKey=${AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY}"
  printf 'store snapshot public key: embed via ldflags\n'
fi
# 本站在源站上所属的商业版应用。留空时归入源站「接收老客户端」的那个应用。
if [[ -n "${AUTH_PRO_STORE_PRODUCT_APP_KEY:-}" ]]; then
  if [[ ! "${AUTH_PRO_STORE_PRODUCT_APP_KEY}" =~ ^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$ ]]; then
    echo "AUTH_PRO_STORE_PRODUCT_APP_KEY 只能包含字母、数字、下划线和短横线" >&2
    exit 1
  fi
  LDFLAGS="$LDFLAGS -X auto_pro/handler.embeddedStoreProductAppKey=${AUTH_PRO_STORE_PRODUCT_APP_KEY}"
  printf 'store product app key: %s\n' "$AUTH_PRO_STORE_PRODUCT_APP_KEY"
fi
# 适用端写进包内 backend/release.json 并随包签名：official 是官网包，client 是客户站包，站点拒绝装另一端的包。
PACKAGE_EDITION="${AUTH_PRO_PACKAGE_EDITION:-client}"
if [[ "$PACKAGE_EDITION" != "official" && "$PACKAGE_EDITION" != "client" ]]; then
  echo "AUTH_PRO_PACKAGE_EDITION 只能是 official 或 client：$PACKAGE_EDITION" >&2
  exit 1
fi
export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
mkdir -p "$GOCACHE"

case "$PACKAGE_DIR" in
  "$ROOT_DIR"/*) ;;
  *) echo "Invalid package directory: $PACKAGE_DIR" >&2; exit 1 ;;
esac

printf '[1/5] Building frontend...\n'
VITE_VERSION="$VERSION" pnpm -C "$FRONTEND_DIR" run build

test -f "$FRONTEND_DIR/dist/index.html"
test -f "$FRONTEND_DIR/dist/version.json"

printf '[2/5] Preparing package directories...\n'
rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR/backend" "$PACKAGES_DIR" "$BACKEND_DIR/static"
rm -rf "$BACKEND_DIR/static"/*
cp -R "$FRONTEND_DIR/dist"/. "$PACKAGE_DIR"/
cp -R "$FRONTEND_DIR/dist"/. "$BACKEND_DIR/static"/
# 客户包只带面板辅助脚本和进程守护模板。
# 安装脚本的唯一源是 backend/handler/install.sh，由 go:embed 直接下发，不复制、不打进包。
for packaged_script in baota-panel.py; do
  cp "$ROOT_DIR/scripts/$packaged_script" "$PACKAGE_DIR/$packaged_script"
  chmod 755 "$PACKAGE_DIR/$packaged_script"
done
cp "$ROOT_DIR/backend/handler/guardian_start.sh" "$PACKAGE_DIR/guardian-start.sh"
chmod 755 "$PACKAGE_DIR/guardian-start.sh"

printf '[3/5] Syncing client SDK assets and building Linux amd64 backend...\n'
rm -rf "$BACKEND_DIR/handler/sdk_assets"
mkdir -p "$BACKEND_DIR/handler/sdk_assets/_meta" "$BACKEND_DIR/handler/sdk_assets/go"
cp -a "$ROOT_DIR/sdk/php" "$ROOT_DIR/sdk/node" "$ROOT_DIR/sdk/python" "$ROOT_DIR/sdk/browser" \
  "$BACKEND_DIR/handler/sdk_assets/"
cp -a "$ROOT_DIR/sdk/go/authpro" "$BACKEND_DIR/handler/sdk_assets/go/"
cp "$ROOT_DIR/sdk/go/go.mod" "$BACKEND_DIR/handler/sdk_assets/_meta/go.mod.txt"
rm -rf "$BACKEND_DIR/handler/sdk_assets/python/authpro/__pycache__"
rm -f "$BACKEND_DIR/handler/sdk_assets/go/authpro/"*_test.go
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go -C "$BACKEND_DIR" build -trimpath -ldflags "$LDFLAGS" -o "$BACKEND_DIR/auto_pro_linux_amd64" .
cp "$BACKEND_DIR/auto_pro_linux_amd64" "$PACKAGE_DIR/backend/auth_pro"

printf '[4/5] Writing release info and signed manifest...\n'
# 版本、适用端和更新说明写进包内，上传更新包时只靠这一个文件就能展示和验签。latest.json 用同一份说明。
node "$ROOT_DIR/scripts/write-release-manifests.mjs" --release-info "$PACKAGE_DIR/backend/release.json" "$PACKAGE_EDITION" \
  "$LATEST_PATH" "$RELEASES_PATH" "$VERSION" "$BUILD_TIME"
AUTO_PRO_RELEASE_NOTES="$(node -e 'process.stdout.write(JSON.stringify(JSON.parse(require("fs").readFileSync(process.argv[1], "utf8")).notes))' "$PACKAGE_DIR/backend/release.json")"
export AUTO_PRO_RELEASE_NOTES
# manifest.json 记下每个文件的 SHA256 并用发布私钥签名，1.8.6 起的站点在线更新前会验签。
# 没有私钥时写未签名清单（本地试打包用）；发布工作流设了 AUTH_PRO_REQUIRE_UPDATE_SIGNATURE=1，缺私钥直接失败。
AUTH_PRO_UPDATE_SIGNING_KEY="$UPDATE_SIGNING_KEY" go -C "$BACKEND_DIR" run ./cmd/release-sign manifest "$PACKAGE_DIR" "$VERSION"

printf '[5/5] Creating tar.gz package and latest.json...\n'
rm -f "$PACKAGE_PATH"
tar -czf "$PACKAGE_PATH" -C "$PACKAGE_DIR" .

forbidden="$(tar -tzf "$PACKAGE_PATH" | grep -E '(^|/)([^/]*_test\.go|[^/]*\.test\.(ts|js|mjs)|[^/]*\.spec\.ts|commercial_mysql_e2e\.py|/tests/|/e2e/|__pycache__/|install\.sh|baota-install\.sh|baota-upgrade\.sh|baota-lib\.sh|quality-check\.sh|build-release\.sh|test-baota-scripts\.sh|simulate-supervised-update\.sh|restart-backend\.sh|smoke-client-sdk\.sh|startup-smoke\.sh|check-migration-cycles\.sh|publish-gitee-release\.(sh|ps1)|check-unused-exports\.mjs|write-release-manifests\.mjs)' || true)"
if [[ -n "$forbidden" ]]; then
  printf '发布包包含测试或调试文件:\n%s\n' "$forbidden" >&2
  exit 1
fi
if strings "$PACKAGE_DIR/backend/auth_pro" | grep -F 'AUTH_PRO_STORE_SOURCE_BASE' >/dev/null; then
  echo "正式二进制仍包含源站地址环境变量入口" >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  PACKAGE_SHA256="$(shasum -a 256 "$PACKAGE_PATH" | cut -d ' ' -f 1)"
else
  PACKAGE_SHA256="$(sha256sum "$PACKAGE_PATH" | cut -d ' ' -f 1)"
fi
if PACKAGE_SIZE="$(stat -f '%z' "$PACKAGE_PATH" 2>/dev/null)"; then
  :
else
  PACKAGE_SIZE="$(stat -c '%s' "$PACKAGE_PATH")"
fi
PACKAGE_URL="${UPDATE_PACKAGE_BASE_URL%/}/$DIST_NAME.tar.gz"
node "$ROOT_DIR/scripts/write-release-manifests.mjs" \
  "$LATEST_PATH" \
  "$RELEASES_PATH" \
  "$VERSION" \
  "$BUILD_TIME" \
  "$DIST_NAME.tar.gz" \
  "$PACKAGE_URL" \
  "$PACKAGE_SHA256" \
  "$PACKAGE_SIZE" \
  "$UPDATE_RELEASES_URL"

printf '\nRelease package: %s\n' "$PACKAGE_PATH"
printf 'Latest manifest: %s\n' "$LATEST_PATH"
printf 'Release history: %s\n' "$RELEASES_PATH"
printf 'SHA256: %s\n' "$PACKAGE_SHA256"
printf 'Size: %s bytes\n' "$PACKAGE_SIZE"
