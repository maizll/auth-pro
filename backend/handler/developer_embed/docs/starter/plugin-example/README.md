# 插件示例

本目录只有 `plugin.json`。打成 ZIP 后清单在根目录，可通过源站硬校验。不要放入 `index.html`、`login.html` 或 `scripts`。

```bash
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

`unzip -l` 里应看到 `plugin.json`。sha256 是整个 ZIP 的 64 位十六进制。

登记插件（可上传 ZIP，或填外部 HTTPS）：

| 标签 | 键 | 示例 |
| --- | --- | --- |
| 应用 | `appId` | 向作者要 |
| 分类 | `category` | 「其他」`other` |
| 名称 | `name` | 演示插件 |
| 标识 | `id` | `demo-widget`，手写，等于清单 `id` |
| 版本 | `version` | `1.0.0` |
| 包来源 | — | 上传这个 ZIP，或外部 HTTPS |
| 下载地址 | `downloadUrl` | 上传后自动填写；外链则是这个 ZIP 的 `https://` 地址 |
| 校验码 (SHA256) | `sha256` | 上一步输出 |
| 简介 | `description` | 与清单相同 |

先保存草稿，地址和校验码都有了再提交审核。

规范：[章程](../../charter.md)。
