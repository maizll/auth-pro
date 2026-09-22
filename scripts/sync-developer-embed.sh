#!/usr/bin/env bash
# Copy developer docs into the Go embed tree. Online update replaces the binary
# and does not install docs next to the process, so Skill download and the
# starter ZIP must not depend on the Baota working directory.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT_DIR/backend/handler/developer_embed"

rm -rf "$DEST"
mkdir -p "$DEST/skill" "$DEST/docs"
cp -a "$ROOT_DIR/docs/developer/." "$DEST/docs/"
cp "$ROOT_DIR/developer-skills/auth-pro-plugin-template/SKILL.md" "$DEST/skill/SKILL.md"
