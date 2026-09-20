# 软件源客户端 URL（按应用隔离）

PHP SDK / 授权端「软件源」模块应固化**应用级**公开清单，不要写死未带应用的 `/software-source/index.json`。

## 推荐固化（路径）

```text
{origin}/software-source/{app_key}/index.json
```

- 无鉴权 `GET`
- `{app_key}` 为 `apps.app_key`（与授权校验里的 `appKey` 相同）
- 只返回该应用已上架的 `plugins` / `homeTemplates`

PHP 拼接示例（SDK ZIP 生成时写入配置即可）：

```php
$pluginSourceIndexUrl = rtrim($origin, '/') . '/software-source/' . rawurlencode($appKey) . '/index.json';
```

管理端 `GET /api/v1/source/admin/apps`（开发者 `GET /api/v1/source/developer/apps`）每条应用带 `indexUrl` 相对路径，可直接拼 origin，不必自己拼 path。

## 兼容写法

| 写法 | 说明 |
| --- | --- |
| `/software-source/index.json?app_key={app_key}` | 查询串 |
| `/software-source/index.json?appKey={app_key}` | 与授权 JSON 字段同名 |
| `/auth-pro/{app_key}/index.json` | 旧路径别名 |

未指定应用的 `/software-source/index.json` 返回空目录（`plugins: []`），**禁止**当默认源固化进 SDK。未知 `app_key` 为 HTTP 404。

## 清单字段

```json
{
  "name": "本站软件源",
  "appKey": "demo-app",
  "appId": 1,
  "indexUrl": "/software-source/demo-app/index.json",
  "plugins": [],
  "homeTemplates": []
}
```

消费者按配置 URL 原样 GET 即可；相对 `templateUrl` 相对该 `index.json` 解析。
