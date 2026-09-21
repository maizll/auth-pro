#!/usr/bin/env bash
# 混合客户端 SDK 冒烟：语法检查 + 打包 ZIP 结构 + 五语言 verify（httptest stub）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== sync sdk_assets =="
rm -rf backend/handler/sdk_assets
mkdir -p backend/handler/sdk_assets/_meta backend/handler/sdk_assets/go
cp -a sdk/php sdk/node sdk/python sdk/browser backend/handler/sdk_assets/
cp -a sdk/go/authpro backend/handler/sdk_assets/go/
cp sdk/go/go.mod backend/handler/sdk_assets/_meta/go.mod.txt
rm -rf backend/handler/sdk_assets/python/authpro/__pycache__
rm -f backend/handler/sdk_assets/go/authpro/*_test.go

echo "== language syntax =="
php -l sdk/php/src/AuthPro.php
node --check sdk/node/src/index.js
node --check sdk/browser/src/auth-pro.js
python3 -m py_compile sdk/python/authpro/__init__.py
(cd sdk/go && go test ./...)

echo "== go pack builder =="
(cd backend && go test ./handler/ -run 'SDKPack|ClientSDK' -count=1)

echo "== zip self-test via go test helper =="
(cd backend && go test ./handler/ -run TestBuildSDKPackHybridLayoutAndAPIs -count=1 -v)

echo "ALL SMOKE PASS"
