# 插件示例

本目录只有 `plugin.json`。打成 ZIP 后清单在根目录，可通过源站硬校验。

```bash
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

登记插件时：标识填 `demo-widget`，分类选「其他」。包来源可以上传这个 ZIP（本站回填地址和校验码），或填写外部 HTTPS 并粘贴 sha256。简介与清单 `description` 相同。售价默认 0。大于 0 时可以上传这个 ZIP，也可以填它的 HTTPS 地址，由本站立即拉取并私有托管、自动计算校验码；付费上架尚未开放。价格不写进 `plugin.json`。

规范：[章程](../../charter.md)。
