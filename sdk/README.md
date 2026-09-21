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

管理端「下载接入包」会把本目录快照进 ZIP 的 `vendor/`，并预填本应用 `config.json`。设计说明见 `docs/superpowers/specs/2026-09-21-client-sdk-hybrid-design.md`。
