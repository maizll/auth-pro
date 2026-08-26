#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=""
REPOSITORY="Zcy-sa/auth-pro"
REMOTE="origin"
SKIP_TESTS=0

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

warn() {
  printf 'warning: %s\n' "$*" >&2
}

step() {
  printf '[publish] %s\n' "$*"
}

usage() {
  cat <<'USAGE'
Usage: publish-gitee-release.sh <version> [options]

Options:
  -v, --version <X.Y.Z>       发行版本号
  -r, --repository <owner/repo>  Gitee 仓库，默认 Zcy-sa/auth-pro
      --remote <name>         git 远程名，默认 origin
      --skip-tests            跳过后端测试
  -h, --help                  显示帮助

Environment:
  GITEE_ACCESS_TOKEN          必填，具有仓库写入权限的 Gitee 私人令牌
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version) VERSION="${2:-}"; shift 2 ;;
    -r|--repository) REPOSITORY="${2:-}"; shift 2 ;;
    --remote) REMOTE="${2:-}"; shift 2 ;;
    --skip-tests) SKIP_TESTS=1; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) die "Unknown option: $1" ;;
    *)
      if [[ -z "$VERSION" ]]; then
        VERSION="$1"
        shift
      else
        die "Unexpected argument: $1"
      fi
      ;;
  esac
done

[[ -n "$VERSION" ]] || { usage >&2; die 'Version is required'; }
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "Version must match X.Y.Z: $VERSION"
[[ "$REPOSITORY" =~ ^([^/]+)/([^/]+)$ ]] || die "Repository must match owner/repo: $REPOSITORY"
OWNER="${BASH_REMATCH[1]}"
REPO="${BASH_REMATCH[2]}"
[[ -n "${GITEE_ACCESS_TOKEN:-}" ]] || die 'GITEE_ACCESS_TOKEN is required'

for binary in git curl node go pnpm tar; do
  command -v "$binary" >/dev/null 2>&1 || die "Required command not found: $binary"
done

TAG="v$VERSION"
PACKAGES_DIR="$ROOT_DIR/release/packages"
PACKAGE_PATH="$PACKAGES_DIR/auth_pro-full-v$VERSION.tar.gz"
LATEST_PATH="$PACKAGES_DIR/latest.json"
RELEASES_PATH="$PACKAGES_DIR/releases.json"

umask 077
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT
TOKEN_FILE="$WORK_DIR/token"
printf '%s' "$GITEE_ACCESS_TOKEN" >"$TOKEN_FILE"

url_encode() {
  node -e 'process.stdout.write(encodeURIComponent(process.argv[1]))' "$1"
}

API_BASE="https://gitee.com/api/v5/repos/$(url_encode "$OWNER")/$(url_encode "$REPO")"

# 返回 HTTP 状态码，响应体写入指定文件。
http_get() {
  local url="$1" output="$2" code
  code="$(curl -sS -m 60 -o "$output" -w '%{http_code}' "$url")" \
    || die "Request failed: $url"
  printf '%s' "$code"
}

json_release_id() {
  node -e '
    const data = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))
    if (data && data.id !== undefined) process.stdout.write(String(data.id))
  ' "$1"
}

json_attachment_url() {
  node -e '
    const list = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))
    const target = Array.isArray(list)
      ? list.find((item) => item && item.name === process.argv[2])
      : null
    if (target && target.browser_download_url) process.stdout.write(target.browser_download_url)
  ' "$1" "$2"
}

json_notes_body() {
  node -e '
    const manifest = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))
    const notes = Array.isArray(manifest.notes) ? manifest.notes : []
    const body = notes.map((item) => `- ${String(item).trim()}`).filter((line) => line !== "- ").join("\n")
    require("fs").writeFileSync(process.argv[2], body, "utf8")
  ' "$1" "$2"
}

delete_release() {
  curl -sS -m 60 -o /dev/null -X DELETE \
    --data-urlencode "access_token@$TOKEN_FILE" \
    "$API_BASE/releases/$1" \
    || warn "Failed to remove incomplete Gitee Release $TAG"
}

cd "$ROOT_DIR"

step 'Validating working tree and remote...'
git rev-parse --git-dir >/dev/null 2>&1 || die "Not a git repository: $ROOT_DIR"
[[ -z "$(git status --porcelain)" ]] || die 'Working tree must be clean before publishing a release'

REMOTE_URL="$(git remote get-url "$REMOTE")" || die "Remote $REMOTE is not configured"
shopt -s nocasematch
if [[ ! "$REMOTE_URL" =~ gitee\.com[/:]"$OWNER"/"$REPO"(\.git)?$ ]]; then
  shopt -u nocasematch
  die "Remote $REMOTE does not point to https://gitee.com/$REPOSITORY: $REMOTE_URL"
fi
shopt -u nocasematch

HEAD_COMMIT="$(git rev-parse HEAD)"
TAG_COMMIT="$(git rev-list -n 1 "$TAG" 2>/dev/null)" || die "Tag $TAG does not exist locally"
[[ "$HEAD_COMMIT" == "$TAG_COMMIT" ]] || die "Tag $TAG must point to HEAD before publishing"
git ls-remote --exit-code --tags "$REMOTE" "refs/tags/$TAG" >/dev/null \
  || die "Tag $TAG has not been pushed to $REMOTE"

step "Checking whether Gitee Release $TAG already exists..."
EXISTING_CODE="$(http_get "$API_BASE/releases/tags/$(url_encode "$TAG")" "$WORK_DIR/existing.json")"
case "$EXISTING_CODE" in
  200) die "Gitee Release $TAG already exists" ;;
  404) ;;
  *) die "Failed to query Gitee Release $TAG (HTTP $EXISTING_CODE)" ;;
esac

step 'Fetching release history from the latest Gitee Release...'
mkdir -p "$PACKAGES_DIR"
HISTORY_URL=""
LATEST_CODE="$(http_get "$API_BASE/releases/latest" "$WORK_DIR/latest-release.json")"
case "$LATEST_CODE" in
  200)
    LATEST_ID="$(json_release_id "$WORK_DIR/latest-release.json")"
    if [[ -n "$LATEST_ID" ]]; then
      ATTACH_CODE="$(http_get "$API_BASE/releases/$LATEST_ID/attach_files" "$WORK_DIR/attach-files.json")"
      [[ "$ATTACH_CODE" == "200" ]] || die "Failed to list release attachments (HTTP $ATTACH_CODE)"
      HISTORY_URL="$(json_attachment_url "$WORK_DIR/attach-files.json" 'releases.json')"
    fi
    ;;
  404) ;;
  *) die "Failed to query the latest Gitee Release (HTTP $LATEST_CODE)" ;;
esac

if [[ -n "$HISTORY_URL" ]]; then
  curl -fsSL -m 300 -o "$RELEASES_PATH" "$HISTORY_URL" || die 'Failed to download previous releases.json'
else
  rm -f "$RELEASES_PATH"
fi

step "Building release $VERSION..."
AUTO_PRO_RELEASE_REPOSITORY="$REPOSITORY" \
AUTO_PRO_UPDATE_PACKAGE_BASE_URL="https://gitee.com/$REPOSITORY/releases/download/$TAG" \
AUTO_PRO_UPDATE_RELEASES_URL="https://gitee.com/$REPOSITORY/releases/download/$TAG/releases.json" \
AUTO_PRO_REQUIRE_GIT_RELEASE_NOTES=1 \
AUTO_PRO_RELEASE_CURRENT_REF="$TAG" \
  bash "$SCRIPT_DIR/build-release.sh" "$VERSION" || die 'Release build failed'

for asset in "$PACKAGE_PATH" "$LATEST_PATH" "$RELEASES_PATH"; do
  [[ -s "$asset" ]] || die "Release asset was not generated: $asset"
done

if [[ "$SKIP_TESTS" -eq 0 ]]; then
  step 'Running backend tests...'
  go -C "$ROOT_DIR/backend" test ./... || die 'Backend tests failed'
fi

BODY_FILE="$WORK_DIR/release-body.txt"
json_notes_body "$LATEST_PATH" "$BODY_FILE"
[[ -s "$BODY_FILE" ]] || printf 'Version %s' "$VERSION" >"$BODY_FILE"

step "Creating Gitee Release $TAG..."
CREATE_CODE="$(curl -sS -m 120 -o "$WORK_DIR/created.json" -w '%{http_code}' \
  --data-urlencode "access_token@$TOKEN_FILE" \
  --data-urlencode "tag_name=$TAG" \
  --data-urlencode "name=$TAG" \
  --data-urlencode "body@$BODY_FILE" \
  --data-urlencode 'prerelease=false' \
  --data-urlencode "target_commitish=$HEAD_COMMIT" \
  "$API_BASE/releases")" || die 'Failed to create Gitee Release'

case "$CREATE_CODE" in
  200|201) ;;
  *) die "Failed to create Gitee Release $TAG (HTTP $CREATE_CODE): $(cat "$WORK_DIR/created.json")" ;;
esac

RELEASE_ID="$(json_release_id "$WORK_DIR/created.json")"
[[ -n "$RELEASE_ID" ]] || die 'Gitee Release response did not contain an id'

for asset in "$PACKAGE_PATH" "$LATEST_PATH" "$RELEASES_PATH"; do
  step "Uploading $(basename "$asset")..."
  UPLOAD_CODE="$(curl -sS -m 1800 -o "$WORK_DIR/upload.json" -w '%{http_code}' \
    -F "access_token=<$TOKEN_FILE" \
    -F "file=@$asset" \
    "$API_BASE/releases/$RELEASE_ID/attach_files")" || UPLOAD_CODE='000'

  case "$UPLOAD_CODE" in
    200|201) ;;
    *)
      delete_release "$RELEASE_ID"
      die "Failed to upload $(basename "$asset") (HTTP $UPLOAD_CODE)"
      ;;
  esac
done

step "Published: https://gitee.com/$REPOSITORY/releases/tag/$TAG"