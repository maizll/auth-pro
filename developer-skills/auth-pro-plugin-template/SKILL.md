---
name: auth-pro-plugin-template
description: Use when creating, packaging, validating, or submitting AuthPro source-station plugins or home templates; when authoring plugin.json or template.json; when a ZIP is rejected for manifest, kind, path traversal, sha256, downloadUrl, templateUrl, category, or app-scoped catalog rules.
---

# AuthPro 插件 / 整站模板

按仓库 `docs/developer/charter.md` 生成包。本章是操作清单。不要写介绍。不要发明章程之外的字段。

产出：清单 JSON、ZIP 布局、sha256 命令、登记表单要填的中文栏。登记来源三选一：上传 ZIP（收费时本站托管）、公开 HTTPS（仅免费，提交审核时核对可达、ZIP、sha256，且必须是公网地址）、私有 GitHub Release（收费推荐，开发者自己的只读令牌核对一次，不留 ZIP）。

## 先选一种

- 插件：ZIP 里只有 `plugin.json`。`kind` 省略或 `"plugin"`。
- 模板：登记 ZIP 里要有 `template.json`。`kind` 必须是 `template`。硬校验还要求数字 `schemaVersion: 1` 和 `hero.title`，并禁止 `scripts`。`hero.primaryAction.type = "login"` 由宿主打开登录框，不是上传拒绝条件。登记包不要依赖 `index.html`（本站另行安装静态页时才会优先 `index.html`，见 `docs/home-template.md`）。

`scripts` 会被拒绝。不要在模板里调用登录接口或保存 token。不要使用内置插件 id（`epay`、`epay-v2`、`alipay-f2f`、`alipay-realname`、`kuaitong-realname`、`tencent-realname`、`xiaomu-realname`）。

## 宿主会读的字段

硬校验只强制 `hero.title`（以及 `kind`、`schemaVersion`、作者和版本等清单字段）。下面这些写了才会出现在页面上，缺了不会导致 ZIP 被拒。不要在模板里实现注册、忘记密码或自己的登录接口。

| 面 | 写进 JSON |
| --- | --- |
| 主视觉 | `hero.title` 必填。可选 `badge`、`highlight`、`description` |
| 登录框 | `hero.primaryAction.type` 为 `"login"` 时打开宿主弹窗，文案用 `label`。外观跟随 `stylePreset` 与 `theme`（`cartoon-blue`、`fintech-gold`，其它值按普通预设）。不要写 `login.html` |
| 能力 | `features` 数组。普通预设最多显示 12 条，`fintech-gold` 最多 3 条 |
| 页脚 | `footer.text` |
| 版式 | `stylePreset` 为 `cartoon-blue` 或 `fintech-gold` |

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

面板：**登记插件** / **登记模板**。包来源三选一：「上传压缩包」「公开地址（仅免费）」「私有 GitHub 仓库（收费推荐）」。同一页有「收费安装包只读令牌」，按当前开发者账号加密保存，只用于这个账号的条目。按钮灰掉时，表单里有中文原因。

默认栏：

| 标签 | 键 | 怎么填 |
| --- | --- | --- |
| 应用 | `appId` | 向作者要。必选 |
| 分类 | `category` | 插件 `other`（界面「其他」）。模板 `home-template`（界面「首页模板」） |
| 名称 | `name` | 与清单相同 |
| 标识 | `id` | 手写，等于清单 `id`。不要用中文名自动生成的标识。模板同时写 `templateKey` |
| 版本 | `version` | `1.0.0` |
| 包来源 | `packageSource` | `upload`、`public` 或 `github`。公开地址不能收费 |
| 下载地址 | `downloadUrl` | 仅插件。上传后自动填；免费外链为 `https://` ZIP；收费私有仓库粘贴 `https://github.com/所有者/仓库/releases/download/标签/文件名.zip` |
| 模板地址 | `templateUrl` | 仅模板。规则同下载地址 |
| 校验码 (SHA256) | `sha256` | 公开地址和本站托管 ZIP 用上一步输出。私有仓库由本站自动计算 |
| 简介 | `description` | 与清单相同 |

高级选项：作者只读（登录名）。插件图标缺省 `ri:puzzle-line`。更新说明可空。

先 **保存草稿**。地址和校验码都有了再 **提交审核**。缺了界面提示 `提交审核前请先填写下载地址和校验码`（模板是「模板地址」）。

已上架后改包：列表 **版本** → **新增版本**（版本、地址、校验码、更新说明）→ 保存草稿 → 提交审核。不要改已发布地址。

## 交卷清单

1. 目录树
2. 完整 JSON
3. `zip` / `unzip -l` / `sha256sum` 命令
4. 上表每一栏的值
5. 一句话：免费用公开地址，提交时校验；收费用私有 GitHub 仓库（开发者自己的只读令牌，本站不留 ZIP）或上传 ZIP 由本站托管；登录框由宿主按 `stylePreset` 打开（与首页同一套颜色和圆角）；模板包内不要 `index.html` 或 `login.html`

## 被拒绝时

| 现象 | 改法 |
| --- | --- |
| `template.json 缺少 kind` | `"kind": "template"` |
| `缺少 schemaVersion` 或必须为 1 | 数字 `1` |
| `缺少 hero.title` | 写 `hero.title`。登录按钮是可选项 |
| `scripts` / `forbidden` | 删除该键 |
| `require_manifest` | 清单放到 ZIP 根目录 |
| `zip_layout` | 按打包命令重打，去掉 `..` 和反斜杠 |
| `sha256 必须是 64 位十六进制` | 对 ZIP 重算 |
| `拒绝访问非公网地址` | 外链改成公网 HTTPS，或改上传 ZIP |
| `公开地址只能用于免费条目` | 收费改私有 GitHub 仓库，或上传 ZIP |
| `请先在开发者面板配置 GitHub 只读令牌` | 在登记页保存 fine-grained、Contents 只读的令牌 |
| `需要改成私有仓库来源或上传 zip 才能继续收费出售` | 旧的收费公开地址。改来源后再提交，不要删条目 |
| `拒绝访问链路本地或云元数据地址` | 不要指向链路本地或云元数据 |
| `下载地址须为 https 开头的外链` | 换 https |
| `标识不合法` / 太短 | 手写 id |
| `不能覆盖内置插件标识` | 换 id |
| `该分类属于首页模板，不能用于插件包` | 插件改 `other` |
| `该分类属于插件，不能用于首页模板包` | 模板改 `home-template` |
| `软件源模板目录与 ZIP 入口类型不一致` | 删除 `index.html` |
| `已发布版本不可改包地址` | 新增版本 |
