# AuthPro 客户端 SDK

版本化多语言库（应用差异只写在 `config.json`）：

| 目录 | 语言 |
| --- | --- |
| `php/` | PHP |
| `node/` | Node.js |
| `python/` | Python |
| `go/` | Go |
| `browser/` | 浏览器（不含 appSecret） |

公共 API：`boot` / `verify` / `checkUpdate` / `ads` / `pluginSourceUrl`。

`verify` 走 v3：每次带随机数，只认授权站用 Ed25519 签过名的响应（`config.json` 的 `publicKey`），通过结果按 `cacheTtl` 缓存，连不上时按 `offlineGrace` 沿用。签名规则见 `docs/api-sdk.md`「响应签名」。

管理端「下载接入包」每次只打包**一种**语言：解压得到单文件夹（入口文件 + `config.json` + 短 README），可直接放进项目 require/import。仓库里五种语言库都保留，只是下载不再五语言打成一包。设计说明见 `docs/superpowers/specs/2026-09-21-client-sdk-hybrid-design.md`。
