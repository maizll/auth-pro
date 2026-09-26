# 整站模板示例

本目录只有 `template.json`。硬校验要求 `kind` 为 `template`、`schemaVersion` 为数字 `1`、`hero.title`，并且没有 `scripts`。本示例另外写了登录动作（`type` 为 `login`）、三条能力、页脚、`stylePreset` 与 `theme`，宿主会用来画首页和登录框（`cartoon-blue` 为浅色，`fintech-gold` 为金黑）。这些展示字段不是上传拒绝条件。不要再加 `login.html` 或 `scripts`。

```bash
rm -f /tmp/demo-home.zip
zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip
```

登记模板时：标识填 `demo-home`，分类选「首页模板」。免费时模板地址填这个 ZIP 的 https 地址，校验码填 sha256。收费时上传这个 ZIP，或填写公开地址让本站拉一次。不用自己的仓库，也不填令牌。售价默认 0。付费上架尚未开放。价格不写进 `template.json`。

规范：[章程](../../charter.md)。
