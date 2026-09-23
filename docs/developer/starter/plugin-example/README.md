# 插件示例

本目录只有 `plugin.json`。打成 ZIP 后清单在根目录，可通过源站硬校验。

```bash
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

登记插件时：标识填 `demo-widget`，分类选「其他」，下载地址填这个 ZIP 的 https 地址，校验码填 sha256，简介与清单 `description` 相同。

规范：[章程](../../charter.md)。
