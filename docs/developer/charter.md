# AuthPro 源站开发者章程

本文件是插件与首页模板的**唯一规范**。交给 AI 时，只给这一份，外加 `starter/` 里的两份清单。AI 按本文生成的 ZIP 必须能通过源站硬校验，并且能填进开发者面板「登记插件」「登记模板」。

源站不保存 ZIP，也不执行包内代码。面板只登记元数据 + 外链 + SHA256。包放在作者自己的 HTTPS 空间。管理员「上传 ZIP」走同一套硬校验：失败返回 `code: 400` 与 `error.field` / `error.rule`，不写库、不留临时文件。

机器可读字段摘要：`GET /software-source/package-schema.json`。与本文冲突时，以校验器和宿主渲染代码为准。

## 0. AI 必须做 / 禁止做

必须：

1. 插件只产出 `plugin.json`。模板只产出 `template.json`。文件名不能互换。
2. 模板是**一份整站声明**，不是只写 `hero.title` 的空壳。登录入口写在 `hero.primaryAction`，`type` 只能是 `"login"`。
3. 清单放在 ZIP **根目录**（或仅一层子目录）。用下面第 7 节的命令打包。
4. `id` 与登记表单「标识」逐字相同。中文名称自动生成的标识经常对不上，必须手写。
5. `sha256` 是**整个 ZIP** 的 64 位小写十六进制，不是清单文件的哈希。

禁止：

- 发明校验器不读的字段：`pages`、`routes`、`loginPage`、`register`、`scripts`、自定义接口。
- 在模板里请求 `POST /api/user-panel/login`、保存 `user_panel_token`、写密码。登录框由**宿主**打开。
- 另写 `login.html`、`register.html`、`forgot-password.html` 或任何登录页。首页和宿主登录框共用 `stylePreset` 与 `theme`，不要再做一套页面。
- 登记包里同时放 `index.html` 和 `template.json`。安装时优先 `index.html`，目录记成 `schemaVersion: 0`，和登记接口固定提交的 `schemaVersion: 1` 冲突，安装报「软件源模板目录与 ZIP 入口类型不一致」。
- 使用内置插件标识：`epay`、`epay-v2`、`alipay-f2f`、`alipay-realname`、`kuaitong-realname`、`tencent-realname`、`xiaomu-realname`。服务端返回「不能覆盖内置插件标识」。
- 把 ZIP 当文件上传到开发者面板。面板没有文件框。

## 1. 两种产物，同一套登记

| | 插件 | 首页模板 |
| --- | --- | --- |
| 清单 | `plugin.json` | `template.json` |
| `kind` | 可省略；填写只能是 `plugin` | **必须** `"template"` |
| 登记按钮 | 我的插件 → **登记插件** | 我的模板 → **登记模板** |
| 地址标签 | 下载地址 → `downloadUrl` | 模板地址 → `templateUrl` |
| 公开目录 | `index.json` 的 `plugins` | `index.json` 的 `homeTemplates` |
| 安装后 | 解压到 `plugins/<id>/`。包内 `plugin.json` 的 `id` 必须等于目录标识 | 宿主读取 `template.json` 画整站。入口必须是这份 JSON |

公开目录：`GET /software-source/{app_key}/index.json`。不带 `app_key` 的 `/software-source/index.json` 是空目录。

## 2. 共用格式

| 项 | 规则 | 非法时服务端原文 |
| --- | --- | --- |
| `id` / `templateKey` | `^[a-z0-9][a-z0-9-]{1,58}$`（2–59 位，小写字母、数字、连字符） | ZIP：`字段 id 不合法`。登记：`插件标识不合法` / `模板标识不合法` |
| `version` | `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` | ZIP：`字段 version 不合法`。登记保存版本时：`版本号不合法` |
| `name` | 非空，截断到 100 字 | `缺少 name` |
| `description` | ZIP **必填**，截断到 500 字。登记表单「简介」可以空着存草稿，但 ZIP 不能空 | `缺少 description` |
| `author` | 字符串，或 `{ "name", "url", "email" }`，`name` 必填 | `缺少 author` / `author.name 必填` |
| 编码 | UTF-8 JSON。`schemaVersion`、数字不要写成字符串 | `必须是 UTF-8 文本` / `不是有效 JSON` |

登记表单里的「作者」是只读的，等于当前登录开发者的显示名，**不会**读取 ZIP 里的 `author`。ZIP 里的 `author` 仍必填，供管理员解析 ZIP 时使用。

## 3. ZIP 硬限制（插件与模板相同）

| 限制 | 值 | 失败原文（包在 `压缩包布局不合法：` 后面） |
| --- | --- | --- |
| 容器 | ZIP，魔数 `PK` | `必须上传 ZIP 压缩包` |
| 体积 | ≤ 20 MiB | `上传包不能超过 20 MiB` |
| 条目数 | ≤ 2048 | `条目数不能超过 2048` |
| 解压后 | ≤ 100 MiB | `ZIP 解压后总大小不能超过 100 MiB` |
| 清单位置 | 根目录，或**只**一层子目录。更深的同名文件被忽略 | `必须在根目录或一层子目录包含 plugin.json` / `template.json` |
| 清单大小 | ≤ 2 MiB | `plugin.json 过大` / `template.json 过大` |
| 路径 | 禁止 `..`、绝对路径、反斜杠、盘符、空段、符号链接、Windows 保留名 | `压缩包包含不安全的文件路径` 等 |
| 空包 / 只有 `__MACOSX` | 拒绝 | `ZIP 不包含可安装文件` |

`__MACOSX` 与 `.DS_Store` 会被跳过，但不能是包里唯一的内容。用第 7 节命令，不要用访达压缩。

## 4. 插件合同

### 4.1 目录

```text
demo-widget.zip
└── plugin.json
```

一层子目录也可以：`demo-widget/plugin.json`。不要两层。

### 4.2 必填与可选

| 字段 | 必填 | 规则 |
| --- | --- | --- |
| `kind` | 否 | 省略视为插件。`"template"` 拒绝：`plugin.json 的 kind 不能是 template` |
| `id` | 是 | 第 2 节 |
| `name` | 是 | ≤100 |
| `version` | 是 | 第 2 节。首次用 `1.0.0` |
| `description` | 是 | ≤500 |
| `author` | 是 | 第 2 节 |
| `category` | 否 | 缺省 `other`。只能用**插件类**分类 |
| `icon` | 否 | ≤80。登记表单缺省 `ri:puzzle-line` |

内置插件分类（下拉框看到的中文 → 写入的 key）：

| 界面 | key |
| --- | --- |
| 支付 | `payment` |
| 实名认证 | `realname` |
| 其他 | `other` |

`home-template` 以及任何 kind=template 的分类写入插件会拒绝：`该分类属于首页模板，不能用于插件包`。未在源站配置的 key：`未知分类，请先在目录分类中配置`。

支付渠道若要在运行中真正收款，还要有编进服务端的 Channel 实现。ZIP 不会被执行。包合同仍是本节。见 `payment-channel-plugin.md`。

### 4.3 可直接通过硬校验的清单

仓库副本：`starter/plugin-example/plugin.json`。

```json
{
  "kind": "plugin",
  "id": "demo-widget",
  "name": "演示插件",
  "version": "1.0.0",
  "description": "AuthPro 源站开发者入门示例插件，仅用于演示清单字段。",
  "author": { "name": "示例作者" },
  "category": "other",
  "icon": "ri:puzzle-line"
}
```

## 5. 模板合同：整站套件

上传校验只强制 `kind`、`schemaVersion: 1`、`hero.title` 以及第 2 节的公共字段。宿主渲染要的是下面这一整份。只交 `hero.title` 能过上传，画出来是空壳，**不符合本章程**。

### 5.1 宿主画出的面（作者能控制的全部）

启用远程模板后，默认首页被换掉。下面这些面由宿主组件绘制，数据只来自 `template.json` 与站点公开配置（站名、Logo、副标题）。作者**没有**第二份页面文件。

| 面 | 数据从哪来 | 作者要写什么 |
| --- | --- | --- |
| 顶栏：站名、Logo、「登录」 | 站名/Logo 来自系统配置。登录按钮宿主固定存在 | 不写 |
| 主视觉 | `hero` | `title` 必填。写上 `badge`、`highlight`、`description` |
| 登录框 | 宿主弹窗。账号、密码、极验、token、代理升级跳转都在宿主。启用本模板后，弹窗版式跟首页同一个 `stylePreset`，颜色跟 `theme.primaryColor`、`theme.backgroundColor`、`theme.textColor` | `hero.primaryAction.type` 必须是 `"login"`，并写 `label`。不要写 `login.html` |
| 能力卡片 | `features` | 写 **3** 条。标准预设最多显示 12 条；`fintech-gold` 只显示前 3 条 |
| 页脚 | `footer.text`；没有则用站名 | 写 `footer.text` |
| `fintech-gold` 额外的「快速查询 / 使用链路 / 底部行动 / 登录对话框」 | 宿主写死文案。查询框**不会**调用授权查询接口，提交后打开登录框 | 没有对应 JSON。不要发明 `pages` 去填它们 |

`stylePreset` 只允许：

| 值 | 版式 |
| --- | --- |
| `cartoon-blue` | 标准版式（顶栏、主视觉、能力卡片、页脚、居中登录弹窗） |
| `fintech-gold` | 金黑版式（另含查询区、三步说明、底部行动、`<dialog>` 登录框） |
| 省略 | 按标准版式画 |
| 其他字符串 | 整份文档被宿主判为非法，**回退到默认首页**，等于模板没生效 |

### 5.1.1 登录弹窗主题合同

模板**启用**后，宿主用同一份字段画首页和登录弹窗。作者不另写页面。字段名如下（与宿主读取的 JSON 一致）：

| 字段 | 合法值 | 宿主怎么用 |
| --- | --- | --- |
| `stylePreset` | `cartoon-blue`、`fintech-gold`，或省略 | 选择首页和登录弹窗版式。其他字符串整份模板失效，回退默认首页 |
| `theme.primaryColor` | `#` 加 3–8 位十六进制 | 主色。写入 `--remote-primary`。`fintech-gold` 同时写入 `--gold` |
| `theme.backgroundColor` | 同上 | 背景。写入 `--remote-background`。`fintech-gold` 同时写入 `--page-bg` |
| `theme.textColor` | 同上 | 文字。写入 `--remote-text`。`fintech-gold` 同时写入 `--text` |

颜色不匹配 `^#[0-9a-fA-F]{3,8}$` 时，该字段被忽略，改用宿主默认值，上传不会因此拒绝：

| `stylePreset` | `primaryColor` | `backgroundColor` | `textColor` |
| --- | --- | --- | --- |
| `fintech-gold` | `#f0b90b` | `#0b0e11` | `#f5f5f5` |
| `cartoon-blue` 或省略 | `#4d6bfe` | `#f7f8fc` | `#172033` |

版式：

| `stylePreset` | 登录弹窗 |
| --- | --- |
| `cartoon-blue` | 浅色圆角面板、胶囊按钮。主色是 `theme.primaryColor` |
| `fintech-gold` | 金黑对话框，三色与首页相同。顶栏「注册」打开的是这个登录弹窗 |
| 省略 | 不换圆趣或金黑外形。登录按钮仍使用 `theme.primaryColor`（写入 `--remote-primary`） |
| 未启用远程模板 | 默认首页的登录、注册、忘记密码不读上述字段，保持宿主默认弹窗 |

不要在 ZIP 里放 `login.html`。换登录弹窗外观只改 `stylePreset` 与 `theme` 这三个颜色字段。

图标：写 Remix 名 `ri:` + 小写，例如 `ri:shield-check-line`。不符合 `^ri:[a-z0-9-]+$` 的图标，标准版式会换成 `ri:sparkling-line`。

`hero.imageUrl` 只用 `https://` 图片，或省略。相对路径只在本地上传静态包时重写，声明式登记包不要用。

`hero.secondaryAction` 若出现，`type` 也必须是 `"login"`。宿主不识别其他 type。

### 5.2 启用本模板后不存在的页面

未启用远程模板时，默认首页还有：注册、忘记密码、授权查询、代理商查询、域名/IP 查询。这些**不是** `template.json` 的字段，启用本章程的模板后不会出现。

不要生成这些 JSON 键（校验器不读，宿主不画）：`pages`、`login`、`register`、`forgotPassword`、`queries`、`scripts`。

`scripts` 出现即拒绝（包括空数组 `[]`）：`声明式模板不允许 scripts 字段`，`field=scripts`，`rule=forbidden`。省略该键。写成 `null` 不会拒绝，但不要写。

### 5.3 静态 `index.html` 为什么不能拿来登记

实例后台「上传首页模板」接受根目录 `index.html`（本地安装，`schemaVersion` 记 0）。那条路径**不是**开发者「登记模板」。

登记接口固定提交 `schemaVersion: 1`。目录安装时若入口是 `index.html`，会要求目录里的 schema 也是 0，于是失败。

iframe 里的静态页若要打开登录，只能 `postMessage({ type: 'auth-pro:login' })`，不能自己持有密码或 token。本章程的登记产物不使用这条路径。

### 5.4 校验器必填 vs 章程必填

| 字段 | 上传硬校验 | 本章程（要画成整站） |
| --- | --- | --- |
| `kind` | 必须 `"template"` | 同左 |
| `id` 或 `templateKey` | 至少一个 | 两个都写，且相同 |
| `name` `version` `description` `author` | 必填 | 同左 |
| `schemaVersion` | 数字 `1`。缺：`缺少 schemaVersion（必须为 1）`。非 1：`必须为 1` | 同左 |
| `hero.title` | 必填。缺：`模板文件缺少 hero.title` | 同左，且补齐 5.1 的其余 hero 字段 |
| `category` | 可省略，自动 `home-template` | 写 `home-template` |
| `stylePreset` `theme` `features` `footer` `primaryAction` | 不校验 | **要写**。缺了能上传，但不是整站 |

插件分类写入模板：`该分类属于插件，不能用于首页模板包`。

### 5.5 可直接通过硬校验的整站清单

仓库副本：`starter/template-example/template.json`。

```json
{
  "kind": "template",
  "id": "demo-home",
  "templateKey": "demo-home",
  "name": "演示首页",
  "version": "1.0.0",
  "description": "AuthPro 源站声明式整站模板，含登录入口与能力卡片。",
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
    {
      "icon": "ri:shield-check-line",
      "title": "安全验证",
      "description": "授权状态经过校验，账户与服务信息清晰可查。"
    },
    {
      "icon": "ri:refresh-line",
      "title": "实时同步",
      "description": "授权期限和使用状态及时更新。"
    },
    {
      "icon": "ri:customer-service-2-line",
      "title": "用户中心",
      "description": "从首页打开登录框，进入用户中心。"
    }
  ],
  "footer": { "text": "安全、稳定的软件授权服务" }
}
```

换 `fintech-gold` 时只改 `stylePreset`，以及 `theme.primaryColor`、`theme.backgroundColor`、`theme.textColor`（建议 `#f0b90b`、`#0b0e11`、`#f5f5f5`）。能力卡片仍用 `ri:` 图标。不要为金黑版式再写一套页面文件，也不要写 `login.html`。

## 6. 登记表单

路径：开发者面板 → 我的插件 / 我的模板 → **登记插件** / **登记模板**。

草稿可以不填地址和校验码。点 **提交审核** 之前，地址和校验码必填。界面原文：`提交审核前请先填写下载地址和校验码`（模板把「下载地址」换成「模板地址」）。

### 6.1 默认显示

| 界面标签 | JSON 键 | 草稿 | 提交审核 |
| --- | --- | --- | --- |
| 应用 | `appId` | 必选。只出现在该应用目录 | 同左。缺：`必须绑定应用`。不存在：`应用不存在`。保存后不能改应用 |
| 分类 | `category` | 必选。插件下拉只有插件类，模板只有模板类 | 同左。模板选「首页模板」即 `home-template` |
| 名称 | `name` | 必填，≤100。占位：`用户看到的名字，例如：微信支付` | 同左 |
| 标识 | `id`（模板同时写 `templateKey`，与 `id` 相同） | 会随名称自动生成。**改成与清单 `id` 完全一致**。保存后不能改 | 同左。界面：`标识只能用小写字母、数字和连字符，至少 2 位` |
| 版本 | `version` | 默认 `1.0.0` | 必填，且符合第 2 节 |
| 下载地址 / 模板地址 | `downloadUrl` / `templateUrl` | 可空 | 必填 |
| 校验码 (SHA256) | `sha256` | 可空。64 位十六进制 | 必填。界面：`校验码须为 64 位十六进制`。服务端：`sha256 必须是 64 位十六进制` |
| 简介 | `description` | 界面写「可不填」 | 仍可不填。ZIP 内简介仍必填 |

地址规则：

- 插件「下载地址」必须是 `https://`。否则：`必须是 https:// 外部地址，源站不保存插件或模板源码`。界面：`下载地址须为 https 开头的外链`。
- 模板「模板地址」是 `https://`，或相对路径（不以 `/` 开头、不含 `..`、匹配 `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`）。界面：`模板地址须为 https 开头，或相对路径如 templates/demo-home.json`。服务端：`templateUrl 须为 https:// 或相对路径（如 templates/clean-home.json）`。
- 路径里带 `..` 的 https 地址：`地址不合法`。
- 「自动计算」只对 https 外链发起浏览器请求。跨域失败时用本地 `sha256sum`，提示：`浏览器无法直接读取该地址`。

### 6.2 高级选项（默认折叠）

| 界面标签 | 键 | 行为 |
| --- | --- | --- |
| 作者 | `author.name` | 只读。当前开发者显示名 |
| 图标 | `icon` | 仅插件。缺省 `ri:puzzle-line`。模板无此栏 |
| 更新说明 | `changelog` | 可空，≤2000 |

按钮：**保存草稿**、**提交审核**。

已有已发布版本后，主表单不能改地址。界面提示：`已有正式版本后，包地址请通过「版本」新增`。

### 6.3 保存时实际提交的 JSON

插件 `POST /api/v1/source/developer/plugins`：

```json
{
  "id": "demo-widget",
  "appId": 1,
  "category": "other",
  "name": "演示插件",
  "description": "与 plugin.json 的 description 相同",
  "icon": "ri:puzzle-line",
  "version": "1.0.0",
  "sha256": "64位小写十六进制",
  "downloadUrl": "https://cdn.example.com/demo-widget-1.0.0.zip",
  "changelog": "",
  "author": { "name": "当前开发者显示名" }
}
```

模板 `POST /api/v1/source/developer/templates` 把 `downloadUrl` 换成 `templateUrl`，并增加 `"templateKey"`（等于 `id`）与 `"schemaVersion": 1`。没有 `icon`。

提交审核：`POST /api/v1/source/developer/plugins/{id}/submit` 或 `.../templates/{id}/submit`。

成功文案：`插件元数据已保存（源站不存储源码）` / `模板元数据已保存（源站不存储源码）` / `已提交审核`。

服务端**不会**下载 URL 来核对 sha256。对不上的校验和会在应用侧安装时失败：`软件源模板 ZIP 的 SHA256 校验失败`。插件安装若包内 `id` 不一致：`plugin.json 的插件 ID 必须与软件源一致`。

## 7. 打包与自检

在示例目录内执行。不要把父目录打进去，不要用访达。

```bash
cd docs/developer/starter/plugin-example
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
unzip -l /tmp/demo-widget.zip
sha256sum /tmp/demo-widget.zip
```

`unzip -l` 里应看到 `plugin.json`，不能是 `../plugin.json`，也不能深于一层子目录。

模板把文件名换成 `template.json`，且 ZIP 内不要有 `index.html`。

```bash
cd docs/developer/starter/template-example
rm -f /tmp/demo-home.zip
zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip
```

自检（不代替服务端，但能在上传前挡住章程禁止项）：

```bash
python3 -c '
import json, sys
p = sys.argv[1]
doc = json.load(open(p, encoding="utf-8"))
assert "scripts" not in doc, "delete scripts"
if p.endswith("template.json"):
    assert doc.get("kind") == "template"
    assert doc.get("schemaVersion") == 1
    assert str(doc.get("hero", {}).get("title", "")).strip()
    action = (doc.get("hero") or {}).get("primaryAction") or {}
    assert action.get("type") == "login"
    assert doc.get("stylePreset") in (None, "cartoon-blue", "fintech-gold")
    assert isinstance(doc.get("features"), list) and len(doc["features"]) >= 3
else:
    assert doc.get("kind") in (None, "", "plugin")
    for key in ("id", "name", "version", "description"):
        assert str(doc.get(key, "")).strip(), key
print("ok", p)
' docs/developer/starter/template-example/template.json
```

插件把路径换成 `docs/developer/starter/plugin-example/plugin.json`。仓库测试 `TestDeveloperStarterExamplesPassHardValidation` 用服务端解析函数检查这两份 starter。

## 8. 审核与新版本

条目状态（列表「状态」列的中文）：

| 值 | 界面 | 开发者能做什么 |
| --- | --- | --- |
| `draft` | 草稿 | 编辑、提交审核 |
| `review` | 待审核 | 只能查看。等管理员 |
| `approved` | 已通过 | 还没进公开目录。等管理员上架 |
| `published` | 已上架 | 出现在该应用 `index.json`。改地址要走新版本 |
| `rejected` | 已驳回 | 按「审核说明」改完再提交 |
| `hidden` | 已下架 | 从公开目录去掉，不卸载已安装副本 |
| `deprecated` | 已弃用 | 从公开目录清除 |

管理员在源站目录对条目执行：**通过** → **上架**。上架时若没有 64 位 sha256 或地址：`上架需要 64 位 sha256 和外部下载/模板地址`。驳回时填写审核说明，开发者在抽屉里看到 `审核说明：…`。

管理员不在审核时重新下载 ZIP。地址和校验和是开发者登记的原文。

### 新版本

主表单改不了已发布包的地址。打开行内 **版本** → **新增版本**：

| 界面标签 | 键 | 规则 |
| --- | --- | --- |
| 版本 | `version` | 新号，例如 `1.0.1`。与已有版本重复会改那一条草稿，而不是另开一条 |
| 下载地址 / 模板地址 | `downloadUrl` / `templateUrl` | 新 ZIP 的地址 |
| 校验码 (SHA256) | `sha256` | 新 ZIP 的哈希。此对话框必填 |
| 更新说明 | `changelog` | 可空 |

保存后该行状态为草稿，再点 **提交审核**。版本状态：草稿 `draft`、待审核 `pending`、已发布 `published`、已弃用 `deprecated`。

已发布版本再改地址：`已发布版本不可改包地址，请创建新版本`。

新 ZIP 的 `id` 不变，只改 `version`。管理员通过版本后把它设为当前对外版本。开发者不能自己设 latest。

接口：

- `POST /api/v1/source/developer/plugins/{id}/versions`
- `POST /api/v1/source/developer/plugins/{id}/versions/{version}/submit`
- 模板把 `plugins` 换成 `templates`，地址字段用 `templateUrl`。

## 9. 拒绝原因对照

ZIP 解析失败时响应形如：

```json
{
  "code": 400,
  "msg": "template.json 缺少 kind（必须为 template）",
  "error": { "field": "kind", "rule": "required" }
}
```

| field | rule | msg 要点 | 改法 |
| --- | --- | --- | --- |
| `file` | `required` | 未上传或空文件 | 选择 ZIP |
| `file` | `require_zip` | 不是 ZIP | 用 `zip` 重新打 |
| `file` | `max_size` | 超过 20 MiB | 缩小 |
| `file` | `zip_layout` | 路径、符号链接、空包、重复路径 | 按第 3、7 节重建 |
| `plugin.json` / `template.json` | `require_manifest` | 根或一层子目录没有对应清单 | 改文件名与位置 |
| `plugin.json` / `template.json` | `json` / `encoding` | 非法 JSON 或非 UTF-8 | 另存 UTF-8 |
| `kind` | `required` | 模板没写 kind | `"kind": "template"` |
| `kind` | `mismatch` | 清单种类写反 | 插件不要写 template，模板不要写 plugin |
| `kind` | `invalid` | 不是 plugin/template | 改回允许值 |
| `id` | `required` / `format` | 缺或格式错 | 第 2 节 |
| `schemaVersion` | `required` / `format` | 缺或不是数字 1 | `"schemaVersion": 1` |
| `hero.title` | `required` | 没有主标题 | 写入 `hero.title` |
| `scripts` | `forbidden` | 出现了 scripts | 删除该键 |
| `category` | `kind` / `format` / `unknown` | 跨种类或未配置 | 插件用插件分类，模板用 `home-template` |
| `author` / `author.name` | `required` | 缺作者 | 字符串或 `{ "name": "..." }` |

登记接口多数只返回 `msg`，没有 `error.field`。常见原文见第 2、6、8 节。

## 10. 生成顺序（给 AI）

1. 判定要插件还是模板。不要两种清单放进同一个 ZIP。
2. 按第 4 或第 5 节写出完整 JSON。模板必须含登录动作、3 条能力、页脚、`stylePreset`。登录框由宿主按 `stylePreset` 绘制，不要写 `login.html`。
3. 按第 7 节打包，记下 sha256 与 `unzip -l`。
4. 给出登记表：应用（向作者要）、分类、名称、**手写标识**、版本、地址、校验码、简介。作者用登录名，不要从 JSON 再填一遍。
5. 说明：先保存草稿，地址和校验码齐了再提交审核。更新走「版本」，不改已发布地址。
