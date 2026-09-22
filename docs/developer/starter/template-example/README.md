# 整站模板示例

本目录只有 `template.json`。这是声明式整站：主视觉、登录动作（`type: "login"`）、三条能力、页脚。宿主负责画出登录框。不要再加 `index.html` 或 `scripts`。

`kind` 必须是 `template`，`schemaVersion` 必须是数字 `1`。

```bash
rm -f /tmp/demo-home.zip
zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip
```

登记模板时：标识填 `demo-home`，分类选「首页模板」，模板地址填这个 ZIP 的 https 地址，校验码填 sha256。

规范：[章程](../../charter.md)。
