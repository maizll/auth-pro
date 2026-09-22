# 插件清单

合同全文见 [章程](./charter.md)。本节只保留能打出可上传包的步骤。

## 目录

```text
demo-widget.zip
└── plugin.json
```

`plugin.json` 在 ZIP 根目录，或仅一层子目录（`demo-widget/plugin.json`）。不要用 `template.json`。

## 清单

```json
{
  "kind": "plugin",
  "id": "demo-widget",
  "name": "演示插件",
  "version": "1.0.0",
  "description": "AuthPro 源站开发者入门示例插件，仅用于演示清单字段。",
  "author": { "name": "示例作者" },
  "category": "other",
  "icon": "ri:puzzle-line"
}
```

| 字段 | 规则 |
| --- | --- |
| `kind` | 可省略。填写只能是 `plugin`。`template` 拒绝 |
| `id` | `^[a-z0-9][a-z0-9-]{1,58}$`。禁止内置 id：`epay` `epay-v2` `alipay-f2f` `alipay-realname` `kuaitong-realname` `tencent-realname` `xiaomu-realname` |
| `name` | 必填，≤100 |
| `version` | `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` |
| `description` | 必填，≤500 |
| `author` | 字符串或 `{ "name": "..." }`，`name` 必填 |
| `category` | 缺省 `other`。允许 `payment` / `realname` / `other` 及插件类自定义分类。禁止 `home-template` |
| `icon` | 可选，≤80。登记缺省 `ri:puzzle-line` |

可复制文件：[`starter/plugin-example/plugin.json`](./starter/plugin-example/plugin.json)。

## 打包

```bash
cd docs/developer/starter/plugin-example
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

`unzip -l` 必须看到 `plugin.json` 在根或一层子目录。

## 登记

开发者面板 → 我的插件 → **登记插件**。标识手改成清单里的 `id`。分类选「其他」（`other`）、「支付」（`payment`）或「实名认证」（`realname`）。下载地址填该 ZIP 的 `https://`。校验码填上面的 sha256。先 **保存草稿**，两项齐了再 **提交审核**。

字段对照见 [打包与登记](./packaging.md)。
