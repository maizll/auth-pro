# 插件示例

本目录只有 `plugin.json`。打成 ZIP 后清单在根目录，可通过源站硬校验。

```bash
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

登记插件时：标识填 `demo-widget`，分类选「其他」。免费时包来源选公开地址，填写这个 ZIP 的 https 地址并粘贴 sha256。收费时上传这个 ZIP 由本站托管，或把它发到你的私有 GitHub 仓库 Release，在登记页粘贴资产链接；只读令牌保存在你的开发者账号下。简介与清单 `description` 相同。售价默认 0。付费上架尚未开放。价格不写进 `plugin.json`。

规范：[章程](../../charter.md)。
