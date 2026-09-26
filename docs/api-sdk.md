# API 与 SDK

业务系统调用本站的公开接口完成授权校验和应用版本检查。管理端「接入开发 → SDK 示例」可以按应用、按语言下载接入包，不必从本页抄签名。

当前产品版本 **1.6.1**。下面的字段以 `backend/handler/license_verify.go`、`license_site.go`、`app_version.go`、`sdk_pack.go` 为准。

## 下载接入包

`POST /api/sdk/pack`，需要管理员 JWT。

```json
{
  "appId": 1,
  "language": "php",
  "modules": ["license"],
  "baseUrl": "https://license.example.com"
}
```

| 字段 | 说明 |
| --- | --- |
| `appId` | 必填，应用管理里的应用 |
| `language` | 必填。`php`、`node`、`python`、`go`、`browser` 之一。每次只打一种语言 |
| `modules` | `license`（授权校验）、`piracy`、`update`（版本检查）、`ads`、`plugin_source`（软件源清单地址） |
| `baseUrl` | 可选。不传时用当前站点的对外地址 |

解压后是单个目录：入口文件、预填的 `config.json`、中文 README，以及可选 example。浏览器包不含 `appSecret`。需要签名的模块（`license`、`piracy`、`update`）在其它语言的 `config.json` 里会写入 `appSecret`，只应放在服务端。

五种语言的源码仍在仓库 `sdk/`，设计记录在 `docs/superpowers/specs/2026-09-21-client-sdk-hybrid-design.md`。

## 授权校验

`POST /api/license/verify`

`Content-Type: application/json`。不需要登录。同一客户端 IP 与同一个 `appKey` 每分钟最多 1200 次，超出返回 HTTP 429，这次不写校验日志。

### 请求

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `appKey` | 是 | 应用标识 |
| `domain` | 否 | 域名授权、泛域名授权，或密钥授权要绑定的站点 |
| `serverIp` | 否 | IP 授权，或和域名一起上报 |
| `licenseKey` | 否 | 密钥授权 |
| `timestamp` | 是 | Unix 秒。与服务器时间相差不能超过 600 秒 |
| `signVersion` | 新接入请传 `v2` | 也接受 `2`。空或 `v1` / `1` 仍走旧版 MD5，仅兼容已绑定站点 |
| `sign` | 是 | 见下方。比较时不区分大小写 |

`domain`、`serverIp`、`licenseKey` 不能全空，否则签名通过后返回 `empty_target`。

v2 签名是 HMAC-SHA256，密钥为 `appSecret`，输出 64 位十六进制。规范串共 6 段，用换行连接，空字段保留空串：

```text
v2
{appKey}
{licenseKey}
{规范化域名}
{规范化 IP}
{timestamp}
```

规范化：域名会去掉首尾空白、转成小写、去掉 `http://` 或 `https://` 以及路径和端口；IP 必须能被解析，否则该段为空。请用规范化之后的值签名，不要把带协议的原始 URL 放进规范串。

v1（不推荐）：`MD5(appKey + 目标 + timestamp + appSecret)`。目标优先 `licenseKey`，否则域名，否则 IP。

### 成功

HTTP 200。

```json
{
  "code": 200,
  "msg": "授权有效",
  "data": {
    "result": "pass",
    "appName": "示例应用",
    "planId": 1,
    "planName": "年付",
    "type": "domain",
    "expireAt": "2027-12-31 23:59:59"
  }
}
```

没有到期时间时 `expireAt` 为「永久」。应用被设为不要求授权时，`msg` 为「应用无需授权验证」，`data.licenseRequired` 为 `false`，没有套餐字段。

`type` 为授权类型：`domain`、`wildcard`、`ip`、`key`。

### 失败

应用不存在、已禁用，或签名尚未通过时，对外都是同一句，避免用文案探测应用是否存在：

```json
{
  "code": 403,
  "msg": "授权校验失败",
  "data": { "result": "fail", "reason": "verify_failed" }
}
```

签名已经通过之后，才会返回更具体的原因，例如：

| `reason` | `msg` |
| --- | --- |
| `invalid_timestamp` | 请求已过期 |
| `empty_target` | 授权目标不能为空 |
| `target_blacklisted` | 授权目标已被拉黑（`result` 为 `blacklisted`） |
| `license_not_found` | 授权无效 |
| `license_revoked` | 授权已禁用 |
| `license_expired` | 授权已过期（`result` 为 `expired`） |
| `realname_required` | 该应用要求实名认证，请先在用户中心完成实名后再安装 |
| `signature_upgrade_required` | 新站点首次绑定需要升级 SDK 并使用 v2 签名 |
| `site_limit_exceeded` | 授权已达到最大站点数 |
| `site_not_bound` | 当前站点尚未绑定 |
| `invalid_domain` | 授权域名格式不正确 |
| `invalid_server_ip` | 服务器 IP 格式不正确 |

密钥授权在签名通过后会按域名或 IP 绑定站点。新站点必须使用 v2。已达到站点数上限时拒绝新站点，不取消已绑定的站点。

参数缺失时 `code` 为 400，`reason` 为 `bad_request`。限流时 HTTP 状态是 429，`reason` 为 `rate_limited`。

## 应用版本检查

`POST /api/app/version/check`

同样要求有效授权（应用设为免授权时除外）和 HMAC-SHA256。v2 规范串是 7 段：

```text
v2
{appKey}
{currentVersion}
{licenseKey}
{规范化域名}
{规范化 IP}
{timestamp}
```

请求字段在校验接口的基础上增加必填的 `currentVersion`。v1 使用 HMAC-SHA256，规范串为 `appKey`、`currentVersion`、目标、`timestamp`，以换行连接。

有更新时 `data.hasUpdate` 为 `true`，并包含 `latestVersion`、`title`、`changelog`、`forceUpdate`、`minVersion`、`fileSizeMb`、`fileMd5`、`updates`（按版本升序，每项含 `version`、`title`、`changelog`、`updateSql`）。本地更新包的 `downloadUrl` 形如 `/api/app/version/download?token=...`，是短期令牌，不要存下来反复使用。外部 URL 则直接返回该地址。

没有更新时 `hasUpdate` 为 `false`，`reason` 为 `up_to_date`。

客户端应先按 `updates` 执行 SQL，再下载最新包并核对大小与 MD5，失败时保留原版本。

## 软件源地址

接入包在勾选 `plugin_source` 时，会写入该应用的清单地址：

```text
{baseUrl}/software-source/{appKey}/index.json
```

清单字段见 [软件源清单地址](software-source-client-url.md)。付费行带 `priceCents`，不带下载地址。商业版站点看到「商业版免费」或「已包含」，免费版看到升级提示。授权校验 `POST /api/license/verify` 不看商业版，已有授权继续可用。
