# 整站模板示例

本目录是可过硬校验的声明式整站，不是 HTML 页面。

| 文件 | 用途 |
| --- | --- |
| `template.json` | 标准示例，`stylePreset` 为 `cartoon-blue` |
| `template.fintech-gold.json` | 同一套字段的金黑变体。登记前改名为 `template.json` 再打包 |

两份都包含 `kind: "template"`、数字 `schemaVersion: 1`、`hero.title`、`hero.primaryAction.type: "login"`、`theme.primaryColor` / `backgroundColor` / `textColor`、恰好 3 条 `features` 和 `footer.text`。宿主用 `stylePreset` 画首页和登录框：`cartoon-blue` 为浅色胶囊，`fintech-gold` 为金黑。查询区、步骤和底部按钮是宿主写死的，没有额外字段。

不要放入 `index.html`、`login.html` 或 `scripts`。一个 ZIP 只放一份清单。

浅色包：

```bash
rm -f /tmp/demo-home.zip
zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip
```

金黑包先换成清单名再打：

```bash
cp template.fintech-gold.json /tmp/template.json
cd /tmp
rm -f /tmp/demo-home-gold.zip
zip -X -r /tmp/demo-home-gold.zip template.json
sha256sum /tmp/demo-home-gold.zip
```

也可以只改 `template.json` 里的 `stylePreset` 和 `theme` 三个颜色，不另存文件。

登记模板：应用向作者要，分类选「首页模板」（`home-template`），名称和简介与清单相同，标识手写（浅色 `demo-home`，金黑 `demo-home-gold`），版本 `1.0.0`，模板地址填 ZIP 的 `https://` 地址，校验码填 sha256。
