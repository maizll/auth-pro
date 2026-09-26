# 软件源客户端 URL（按应用隔离）

PHP / Node / Python / Go SDK 以及授权端「下载接入包」中的客户端库应固化**应用级**公开清单，不要写死未带应用的 `/software-source/index.json`。

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

未指定应用的 `/software-source/index.json` 返回空目录（`plugins: []`），**禁止**当默认源固化进 SDK。

应用不存在、或已归档且没有把地址转走时，上述地址返回 HTTP 410，正文是 JSON（不是 HTML，也不是空的 404 页）：

```json
{"error":"app_gone","message":"该软件源对应的应用已删除或归档","appKey":"app_example"}
```

响应里没有 `plugins`。刷新软件源的一方显示这句中文。归档时如果勾选把软件源地址转到另一个应用，或在源站设置里把旧标识映射到现有应用，旧地址仍返回 HTTP 200，正文是目标应用的目录。同一个旧标识再保存一次只更新目标。应用恢复后，映射去掉，旧地址重新返回它自己的目录。

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

管理端「软件源管理」粘贴上述地址时按 JSON 目录保存。以 `.json` 结尾或内容为 JSON 的地址不会被当成 Git 仓库克隆。若添加时选了 Git，保存当场改回 JSON 并提示。

首页模板与插件共用这一份按应用隔离的清单，不要再为模板单独配置第二套远程源。
