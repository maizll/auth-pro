package handler

import "embed"

// clientSDKAssets 是仓库根目录 sdk/* 的嵌入快照，供「下载接入包」离线 vendor。
// 修改 sdk/ 后请同步（注意：不要把 sdk/go/go.mod 放进 sdk_assets/go/，否则嵌套 module 会导致 go:embed 跳过）：
//
//	rm -rf backend/handler/sdk_assets && mkdir -p backend/handler/sdk_assets/_meta backend/handler/sdk_assets/go
//	cp -a sdk/php sdk/node sdk/python sdk/browser backend/handler/sdk_assets/
//	cp -a sdk/go/authpro backend/handler/sdk_assets/go/
//	cp sdk/go/go.mod backend/handler/sdk_assets/_meta/go.mod.txt
//
//go:embed all:sdk_assets
var clientSDKAssets embed.FS
