# 重要提示：插件登记包

插件使用 ZIP 压缩包登记。登记包是**声明式清单**：压缩包根目录放 `plugin.json`。源站只记录元数据和外部地址，不执行包里的代码，也不要求附带页面套件。

目录结构如下：

```text
插件.zip
├── plugin.json
└── README.md
```

`README.md` 可选，源站不解析。一层子目录同样合法：

```text
插件.zip
└── demo-widget/
    └── plugin.json
```

## 文件要求

- `plugin.json`：插件清单，必须位于压缩包根目录或一层子目录，UTF-8 JSON。
- `kind` 可以省略。省略时按插件处理。若填写，必须是 `plugin`。写成 `template` 会被拒绝。
- 必填字段：`id`、`name`、`version`、`description`、`author`。
- 不要把首页模板的 `template.json`、`schemaVersion`、`hero` 写进插件包。
- 不要把 `index.html` 多页站点当作插件登记包。
- 路径穿越、绝对路径、符号链接、空包、仅 `__MACOSX` 会被拒绝。
- 包体不超过 20 MiB，条目不超过 2048 个，解压后不超过 100 MiB。清单不超过 2 MiB。

## 1. 清单字段

`plugin.json` 决定商店里的名称、分类和图标。运行时逻辑（例如支付渠道）不在这个 ZIP 里执行。

机器可读对照：`GET /software-source/package-schema.json`。

## 1.1 字段表

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `kind` | string | 否 | 省略视为 `plugin`。填写时必须为 `plugin` |
| `id` | string | 是 | `^[a-z0-9][a-z0-9-]{1,58}$`，2–59 位。入库前转小写 |
| `name` | string | 是 | 展示名称，非空，截断到 100 字 |
| `version` | string | 是 | `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` |
| `description` | string | 是 | 非空，截断到 500 字 |
| `author` | string 或 object | 是 | 字符串，或 `{name,url,email}`；`name` 必填 |
| `author.name` | string | 是 | 截断到 100 字 |
| `author.url` | string | 否 | 截断到 300 字 |
| `author.email` | string | 否 | 截断到 200 字 |
| `category` | string | 否 | 插件类分类。省略则 `other` |
| `icon` | string | 否 | Iconify 名，截断到 80 字。登记表单缺省 `ri:puzzle-line` |

### 合法与非法

| 字段 | 合法 | 非法 |
| --- | --- | --- |
| `id` | `demo-widget`、`pay-v2` | `Demo`、`a`、`_x`、空 |
| `version` | `1.0.0`、`1.0.0-rc.1` | 空、含空格 |
| `kind` | 省略、`plugin` | `template`、`home-template` |
| `category` | `payment`、`realname`、`other`、已配置的插件 extras | `home-template`、未配置的标识 |
| `author` | `"示例作者"` 或 `{"name":"示例作者"}` | 缺字段、`{"url":"https://example.com"}` |

内置插件分类：`payment`（支付）、`realname`（实名认证）、`other`（其他）。管理端还可以添加插件类自定义分类。标识即使叫 `template`、名称叫「模板」，只要 kind 是 plugin，它仍然是插件分类，会出现在应用商店筛选页，不会进入 `homeTemplates`。

## 1.2 完整示例

可复制副本：[`starter/plugin-example/plugin.json`](./starter/plugin-example/plugin.json)。

```json
{
  "kind": "plugin",
  "id": "demo-widget",
  "name": "演示插件",
  "version": "1.0.0",
  "description": "AuthPro 源站开发者入门示例插件，仅用于演示清单字段。",
  "author": {
    "name": "示例作者",
    "url": "https://example.com",
    "email": "author@example.com"
  },
  "category": "other",
  "icon": "ri:puzzle-line"
}
```

`author` 也可以写成字符串 `"示例作者"`。支付类插件把 `category` 写成 `payment`，`id` 保持稳定。商店 ZIP 仍然只是这份清单，渠道代码不打进登记包。

## 2. 打包命令

在清单所在目录打包，保证 `plugin.json` 在 ZIP 根目录：

```bash
zip -r ../demo-widget.zip plugin.json README.md
sha256sum ../demo-widget.zip
```

Windows PowerShell：

```powershell
Compress-Archive -Path plugin.json, README.md -DestinationPath ..\demo-widget.zip -Force
Get-FileHash ..\demo-widget.zip -Algorithm SHA256
```

只有清单时：

```bash
zip -r ../demo-widget.zip plugin.json
```

`sha256` 针对整个 ZIP，64 位十六进制。可以在登记表单上传 ZIP，由源站托管并回填 `downloadUrl` 与 `sha256`；也可以把文件放到自己的 HTTPS 空间。外链 `downloadUrl` 必须是 `https://`，不能写相对路径。提交审核时外链会被下载核对。

## 3. 登记表单对照

开发者面板「我的插件 → 登记插件」提交的是目录元数据。

| 表单标签 | JSON 字段 | 规则 |
| --- | --- | --- |
| 应用 | `appId` | 必填。条目只属于这一个应用 |
| 分类 | `category` | 必填。插件类，缺省 `other` |
| 名称 | `name` | 必填，≤100 字 |
| 标识 | `id` | 2–59 位小写字母、数字、连字符。新建时按名称生成，提交后不可改 |
| 版本 | `version` | 首次可用 `1.0.0` |
| 下载地址 | `downloadUrl` | 草稿可空。上传 ZIP 后自动填本站地址；外链提交审核前必填且必须 `https://` |
| 校验码 (SHA256) | `sha256` | 草稿可空。提交审核前必填，64 位十六进制 |
| 简介 | `description` | 表单可空。ZIP 硬校验要求清单内非空 |
| 作者 | `author.name` | 表单锁定为当前开发者名称 |
| 图标 | `icon` | 可选。空则 `ri:puzzle-line`，≤80 字 |
| 更新说明 | `changelog` | 可选，≤2000 字 |

保存草稿不要求下载地址和校验码。点「提交审核」之前两者都要有。

已有正式版本后，包地址从「版本」里新增，不要改当前草稿上的地址。

公开目录：

```text
GET /software-source/{app_key}/index.json
```

上架后出现在该应用的 `plugins` 数组，并按 `category` 进入商店筛选页。

## 4. 拒绝信息

失败响应形如：

```json
{
  "code": 400,
  "msg": "plugin.json 缺少 id",
  "error": { "field": "id", "rule": "required" }
}
```

| field | rule | msg |
| --- | --- | --- |
| `plugin.json` | `require_manifest` | `该分类的安装包必须在根目录或一层子目录包含 plugin.json` |
| `plugin.json` | `require_manifest` | `压缩包必须包含 plugin.json 或 template.json`（未指定类型且两份清单都没有） |
| `plugin.json` | `json` | `plugin.json 不是有效 JSON` |
| `plugin.json` | `encoding` | `plugin.json 必须是 UTF-8 文本` |
| `plugin.json` | `max_size` | `plugin.json 过大` |
| `id` | `required` | `plugin.json 缺少 id` |
| `id` | `format` | `plugin.json 字段 id 不合法：须为 2-59 位小写字母、数字或连字符` |
| `name` | `required` | `plugin.json 缺少 name` |
| `version` | `required` | `plugin.json 缺少 version` |
| `version` | `format` | `plugin.json 字段 version 不合法` |
| `description` | `required` | `plugin.json 缺少 description` |
| `author` | `required` | `plugin.json 缺少 author` |
| `author.name` | `required` | `plugin.json 字段 author.name 必填（author 可为字符串或对象）` |
| `kind` | `mismatch` | `plugin.json 的 kind 不能是 template` |
| `kind` | `invalid` | `plugin.json 字段 kind 若填写必须为 plugin` |
| `category` | `format` | `plugin.json 字段 category 不合法：` 加具体原因 |
| `file` | `require_zip` | `必须上传 ZIP 压缩包，且包内须含 plugin.json 或 template.json` |
| `file` | `max_size` | `上传包不能超过 20 MiB` |
| `file` | `zip_layout` | `压缩包布局不合法：` 加具体原因 |

分类跨类型时的原因文案：

- `该分类属于首页模板，不能用于插件包`
- `未知分类，请先在目录分类中配置`

登记接口上的文案：

| 场景 | msg |
| --- | --- |
| 下载地址不是 HTTPS | `必须是 https:// 外部地址，源站不保存插件或模板源码` |
| `sha256` 不合格 | `sha256 必须是 64 位十六进制` |
| 使用内置插件 id | `不能覆盖内置插件标识` |

不能占用的内置 id：`epay`、`epay-v2`、`alipay-f2f`、`alipay-realname`、`kuaitong-realname`、`tencent-realname`、`xiaomu-realname`。

`icon` 超长会被截断到 80 字，不会因此拒绝。

## 5. 分类与商店页签

| 分类标识 | 名称 | 清单 |
| --- | --- | --- |
| `payment` | 支付 | `plugin.json` |
| `realname` | 实名认证 | `plugin.json` |
| `other` | 其他 | `plugin.json` |
| 自定义 extras | 管理端配置的名称 | `plugin.json` |
| `home-template` | 首页模板 | 只能用于 `template.json` |

公开 `index.json` 的 `categories` 会带上 extras，应用商店用它做二级筛选。插件条目始终在 `plugins`，不会因为分类名称里有「模板」就进入 `homeTemplates`。

## 6. 提交流程

1. 按本页写 `plugin.json`，打 ZIP，托管到 HTTPS，计算 sha256。
2. 开发者面板「我的插件」登记，状态为草稿 `draft`。
3. 补齐 `downloadUrl` 与 `sha256` 后提交审核，状态为 `review`。
4. 管理员通过为 `approved`，驳回为 `rejected`。驳回后可修改再提交。
5. 管理员上架后为 `published`，写入该应用 `index.json` 的 `plugins`。
6. 改包请新增版本。已发布版本的地址不能改写。见 [更新与多版本](./versions.md)。

## 7. 与首页模板的边界

| 项 | 插件 | 首页模板 |
| --- | --- | --- |
| 清单文件 | `plugin.json` | `template.json` |
| `kind` | 可省略，或 `plugin` | 必须 `template` |
| 地址字段 | `downloadUrl`，仅 HTTPS | `templateUrl`，HTTPS 或相对路径 |
| 缺省分类 | `other` | `home-template` |
| 页面 | 登记包不提供站点页面 | 声明式 JSON，由宿主渲染 |
| 登录 / 注册 / 忘记密码 | 不在插件包里 | 宿主对话框。模板只声明 `hero.primaryAction.type = login` |
| 商店位置 | `plugins` | `homeTemplates` |

不要把 `schemaVersion`、`hero.title`、`stylePreset` 写进 `plugin.json` 来冒充首页。那些字段属于模板清单。

## 8. 开发注意事项

- 登记包根目录是 `plugin.json`，不是多页 HTML，也不是 `template.json`。
- `id` 提交后不能改。改版本时 `id` 保持不变，只提升 `version`。
- 源站不执行 ZIP。支付等能力要另有进程内实现，见附录。
- 一个包只绑定一个 `appId`。
- 校验失败不写库。先改清单或地址，再保存草稿或提交审核。
- 复制 [`starter/plugin-example`](./starter/plugin-example/plugin.json) 是最快的基线。

## 9. 附录：支付渠道 SPI

这一节不改变登记包契约。支付插件的商店 ZIP 仍然是上一节的 `plugin.json`，`category` 使用 `payment`。下载的 ZIP **不会**被当作代码执行。

渠道行为要在后端进程里实现 `payment.Channel` 并 `payment.Register`。通知地址由核心生成：`/api/payment/{ID()}/notify`。官方参考是仓库 `plugins/alipay-f2f/plugin.json` 与 `backend/payment/alipayf2f`。

清单侧只要求：

- `id` 稳定，符合插件 `id` 规则
- `category` 为 `payment`
- 其余字段与普通插件相同

接口、配置页和收银台去重规则见 [支付渠道插件](./payment-channel-plugin.md)。那篇文档不替换本页的 `plugin.json` 字段表。
