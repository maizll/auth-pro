---
name: auth-pro-plugin-template
description: Use when creating, packaging, validating, or submitting AuthPro source-station plugins or home templates; when authoring plugin.json or template.json; when a ZIP is rejected for manifest, kind, path traversal, sha256, downloadUrl, templateUrl, category, or app-scoped catalog rules.
---

# AuthPro 插件 / 整站模板

按仓库 `docs/developer/charter.md` 生成包。本章是操作清单。不要写介绍。不要发明章程之外的字段。

产出：清单 JSON、ZIP 布局、sha256 命令、登记表单要填的中文栏。登记可以上传 ZIP（本站托管，自动填地址和 sha256），也可以填外部 HTTPS（提交审核时核对可达、ZIP 与 sha256）。

## 先选一种

- 插件：ZIP 里只有 `plugin.json`。`kind` 省略或 `"plugin"`。
- 模板：ZIP 里只有 `template.json`。这是整站声明，不是首页标题块。必须含登录动作。禁止 `index.html`（会和登记时的 `schemaVersion: 1` 冲突）。

禁止：`pages`、`scripts`、自己调用登录接口、保存 token、内置插件 id（`epay`、`epay-v2`、`alipay-f2f`、`alipay-realname`、`kuaitong-realname`、`tencent-realname`、`xiaomu-realname`）。

## 模板必须覆盖的面

宿主用这一份 JSON 画整站。启用后默认首页的注册 / 忘记密码 / 三项查询消失，不要去实现它们。

| 面 | 写进 JSON |
| --- | --- |
| 主视觉 | `hero.title` 必填；加上 `badge`、`highlight`、`description` |
| 登录框 | `hero.primaryAction.type` = `"login"`，并写 `label`。登录 UI 是宿主弹窗，外观跟随 `stylePreset` 与 `theme`（`cartoon-blue` 浅色胶囊，`fintech-gold` 金黑）。不要写 `login.html` |
| 能力 | `features` 恰好 3 条，图标 `ri:` 前缀 |
| 页脚 | `footer.text` |
| 版式 | `stylePreset` 为 `cartoon-blue` 或 `fintech-gold` |

`fintech-gold` 的查询区、步骤、底部按钮是宿主写死的，没有额外字段。

## 文件

插件：

```text
demo-widget.zip
└── plugin.json
```

```json
{
  "kind": "plugin",
  "id": "demo-widget",
  "name": "演示插件",
  "version": "1.0.0",
  "description": "一句话说明。",
  "author": { "name": "示例作者" },
  "category": "other",
  "icon": "ri:puzzle-line"
}
```

模板（可直接过硬校验的整站示例，见 `docs/developer/starter/template-example/template.json`）：

```json
{
  "kind": "template",
  "id": "demo-home",
  "templateKey": "demo-home",
  "name": "演示首页",
  "version": "1.0.0",
  "description": "声明式整站模板，含登录入口与能力卡片。",
  "schemaVersion": 1,
  "author": { "name": "示例作者" },
  "category": "home-template",
  "stylePreset": "cartoon-blue",
  "theme": {
    "primaryColor": "#168fe5",
    "backgroundColor": "#f1faff",
    "textColor": "#15334a"
  },
  "hero": {
    "badge": "授权服务",
    "title": "专业授权服务",
    "highlight": "清晰可查",
    "description": "查看授权状态与有效期。登录由站点打开，模板不保存密码。",
    "primaryAction": { "label": "进入用户中心", "type": "login" },
    "secondaryAction": { "label": "用户登录", "type": "login" }
  },
  "features": [
    { "icon": "ri:shield-check-line", "title": "安全验证", "description": "授权状态经过校验。" },
    { "icon": "ri:refresh-line", "title": "实时同步", "description": "期限与状态及时更新。" },
    { "icon": "ri:customer-service-2-line", "title": "用户中心", "description": "从首页打开登录框。" }
  ],
  "footer": { "text": "安全、稳定的软件授权服务" }
}
```

`fintech-gold` 变体与上面同一套字段，完整文件是 `docs/developer/starter/template-example/template.fintech-gold.json`。只改这些值：

- `id` / `templateKey`：`demo-home-gold`
- `stylePreset`：`fintech-gold`
- `theme.primaryColor`：`#f0b90b`
- `theme.backgroundColor`：`#0b0e11`
- `theme.textColor`：`#f5f5f5`

一个 ZIP 只放一份清单。金黑登记前把变体改名为 `template.json` 再打包。不要另写 `index.html` 或 `login.html`。

格式：`id` 为 `^[a-z0-9][a-z0-9-]{1,58}$`。`version` 为 `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`。`schemaVersion` 是数字 `1`，不是字符串。

## 打包

```bash
cd docs/developer/starter/template-example
rm -f /tmp/demo-home.zip
zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip
```

插件把 `template.json` 换成 `plugin.json`。`unzip -l` 里清单在根目录或一层子目录。sha256 是 ZIP 的 64 位十六进制。

## 登记（中文标签 → 键）

面板：**登记插件** / **登记模板**。包来源选「上传 ZIP（本站托管）」或「外部 HTTPS」。

默认栏：

| 标签 | 键 | 怎么填 |
| --- | --- | --- |
| 应用 | `appId` | 向作者要。必选 |
| 分类 | `category` | 插件 `other`（界面「其他」）。模板 `home-template`（界面「首页模板」） |
| 名称 | `name` | 与清单相同 |
| 标识 | `id` | 手写，等于清单 `id`。不要用中文名自动生成的标识。模板同时写 `templateKey` |
| 版本 | `version` | `1.0.0` |
| 包来源 | — | 上传 ZIP，或外部 HTTPS |
| 下载地址 | `downloadUrl` | 仅插件。上传后自动填；外链为 `https://` ZIP |
| 模板地址 | `templateUrl` | 仅模板。上传后自动填；外链为 `https://` ZIP |
| 校验码 (SHA256) | `sha256` | 上一步输出 |
| 简介 | `description` | 与清单相同 |

高级选项：作者只读（登录名）。插件图标缺省 `ri:puzzle-line`。更新说明可空。

先 **保存草稿**。地址和校验码都有了再 **提交审核**。缺了界面提示 `提交审核前请先填写下载地址和校验码`（模板是「模板地址」）。

已上架后改包：列表 **版本** → **新增版本**（版本、地址、校验码、更新说明）→ 保存草稿 → 提交审核。不要改已发布地址。

## 交卷清单

1. 目录树
2. 完整 JSON
3. `zip` / `unzip -l` / `sha256sum` 命令
4. 上表每一栏的值
5. 一句话：上传的 ZIP 由本站托管，外链在提交时校验；登录框由宿主按 `stylePreset` 打开（与首页同一套颜色和圆角）；模板包内不要 `index.html` 或 `login.html`

## 被拒绝时

| 现象 | 改法 |
| --- | --- |
| `template.json 缺少 kind` | `"kind": "template"` |
| `缺少 schemaVersion` 或必须为 1 | 数字 `1` |
| `缺少 hero.title` | 写标题，并补登录动作 |
| `scripts` / `forbidden` | 删除该键 |
| `require_manifest` | 清单放到 ZIP 根目录 |
| `zip_layout` | 按打包命令重打，去掉 `..` 和反斜杠 |
| `sha256 必须是 64 位十六进制` | 对 ZIP 重算 |
| `下载地址须为 https 开头的外链` | 换 https |
| `标识不合法` / 太短 | 手写 id |
| `不能覆盖内置插件标识` | 换 id |
| `该分类属于首页模板，不能用于插件包` | 插件改 `other` |
| `该分类属于插件，不能用于首页模板包` | 模板改 `home-template` |
| `软件源模板目录与 ZIP 入口类型不一致` | 删除 `index.html` |
| `已发布版本不可改包地址` | 新增版本 |
