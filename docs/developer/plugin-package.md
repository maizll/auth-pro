# 插件清单

登记插件时，ZIP 里要有 `plugin.json`。下面这份可以通过 `fillPluginManifest`。

## 最小 plugin.json

```json
{
  "kind": "plugin",
  "id": "demo-widget",
  "name": "演示插件",
  "version": "1.0.0",
  "description": "一句话说明这个插件做什么。",
  "author": { "name": "示例作者" },
  "category": "other"
}
```

`kind` 可以不写。写成 `template` 会被拒绝。

## 字段

| 字段 | 规则 |
| --- | --- |
| `id` | 必填。`^[a-z0-9][a-z0-9-]{1,58}$`（2–59 位小写字母、数字、连字符） |
| `name` | 必填，最长 100 字 |
| `version` | 必填。`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` |
| `description` | 必填，最长 500 字 |
| `author` | 必填。字符串，或 `{ "name", "url", "email" }`，其中 `name` 必填 |
| `category` | 可空，默认 `other`。内置插件分类还有 `payment`、`realname`，以及管理端配置的插件分类。不能填模板分类（如 `home-template`） |
| `icon` | 可空，最长 80 字 |

仓库示例：`docs/developer/starter/plugin-example/plugin.json`。

## 安装到其它站点时

消费者在应用商店安装的是这个 ZIP。服务端核对 SHA256 后解压到数据目录 `plugins/<id>/`。不会执行 `install.sh` 或包内可执行文件。未编进当前服务端的插件，装上之后只是已安装的文件，不会自动变成支付或实名通道。

支付通道要另实现服务端插件，见 [payment-channel-plugin.md](payment-channel-plugin.md)。
