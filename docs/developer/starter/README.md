# AuthPro 源站开发者入门包

本包是插件 / 首页模板的登记材料，不是管理端按应用下载的客户端 SDK。源站只登记元数据和外链，不保存 ZIP，也不执行包内代码。

规范正文与开发者面板「开发文档」相同：`docs/charter.md`。操作清单是 `SKILL.md`。不要发明这两份文件之外的字段名。

## 先选一种

- 插件：登记 ZIP 里只有 `plugin.json`。`kind` 省略或 `"plugin"`。示例在 `plugin-example/`。
- 模板：登记 ZIP 里只有 `template.json`。必须是整站声明：`kind` 为 `"template"`，`schemaVersion` 为数字 `1`，`hero.title`，以及 `hero.primaryAction.type` = `"login"`。示例在 `template-example/`。
  - `template.json`：`stylePreset` = `cartoon-blue`
  - `template.fintech-gold.json`：`stylePreset` = `fintech-gold`。登记前改名为 `template.json` 再打包。一个 ZIP 只放一份清单。

颜色写 `theme.primaryColor`、`theme.backgroundColor`、`theme.textColor`。启用后宿主登录弹窗跟随 `stylePreset` 与这三个颜色。不要写 `login.html`，不要把 `index.html` 打进同一个 ZIP，不要写 `scripts`。

## 目录

| 路径 | 说明 |
| --- | --- |
| `SKILL.md` | AI 操作清单：打包命令、登记表、拒绝改法 |
| `docs/charter.md` | 章程，与开发者面板同一份 |
| `docs/plugin-package.md` | `plugin.json` |
| `docs/template-package.md` | `template.json` |
| `docs/packaging.md` | ZIP 与 sha256 |
| `docs/validation.md` | 拒绝原因 |
| `plugin-example/plugin.json` | 可过硬校验的插件清单 |
| `template-example/template.json` | 可过硬校验的浅色模板 |
| `template-example/template.fintech-gold.json` | 可过硬校验的金黑变体 |

## 打包

在含清单的目录内执行，保证清单在 ZIP 根目录：

```bash
cd plugin-example
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

浅色模板把目录换成 `template-example`，文件换成 `template.json`。金黑变体先复制成 `template.json` 再打，不要把两份清单打进同一个包。`unzip -l` 里不能出现 `index.html` 或 `login.html`。sha256 是整个 ZIP 的 64 位十六进制。

## 登记（中文标签 → 键）

面板：「登记插件」或「登记模板」。没有文件上传。

| 标签 | 键 | 怎么填 |
| --- | --- | --- |
| 应用 | `appId` | 向作者要 |
| 分类 | `category` | 插件「其他」`other`。模板「首页模板」`home-template` |
| 名称 | `name` | 与清单相同 |
| 标识 | `id` | 手写，等于清单 `id`。模板同时写 `templateKey` |
| 版本 | `version` | `1.0.0` |
| 下载地址 | `downloadUrl` | 仅插件。`https://` ZIP |
| 模板地址 | `templateUrl` | 仅模板。`https://` ZIP |
| 校验码 (SHA256) | `sha256` | 上一步输出 |
| 简介 | `description` | 与清单相同 |

先保存草稿。地址和校验码都有了再提交审核。已上架后改包走「版本 → 新增版本」，不要改已发布地址。

公开目录：`/software-source/{app_key}/index.json`。
