# 重要提示：首页模板登记包

首页模板使用 ZIP 压缩包登记。开发者登记包是**一份** `template.json`（`schemaVersion` 为数字 `1`，`kind` 为 `template`），不是一组 HTML 页面。宿主用它画整站公开页，并在启用后用同一份 `stylePreset` 与 `theme` 画登录弹窗。

目录结构如下：

```text
首页模板.zip
└── template.json
```

一层子目录同样合法，例如 `demo-home/template.json`。不要放入 `index.html`、`login.html`、`register.html` 或 `forgot-password.html`。

## 文件要求

- `template.json`：模板清单，必须位于压缩包根目录或一层子目录，UTF-8 JSON。
- `kind` 必须是 `"template"`。省略、写成 `plugin` 或其他值都会被拒绝。
- `schemaVersion` 必须是数字 `1`。
- `hero.title` 必填。这是硬校验要求的标题，不是整份模板的全部内容。整站还要写登录入口、`stylePreset`、`theme`、能力卡片和页脚。
- 不要把 `index.html` 和 `template.json` 打在同一个登记包里。安装端先找 `index.html`，找到后按静态页安装，并把 `schemaVersion` 记成 0。登记接口固定提交 `schemaVersion: 1`，安装会报「软件源模板目录与 ZIP 入口类型不一致」。
- 不要写 `login.html`。换登录弹窗外观只改 `stylePreset` 和 `theme` 的三个颜色字段。
- 不要写 `scripts` 字段（空数组也不行）。不要写 `pages`、`register`、`forgotPassword`。
- 不要写服务器绝对路径、本地磁盘路径、用户密码、Token。
- 包体不超过 20 MiB，条目不超过 2048 个，解压后不超过 100 MiB。清单文件不超过 2 MiB。

## 1. 清单字段

硬校验读的是 `template.json`。缺省分类自动写成 `home-template`。

机器可读对照：`GET /software-source/package-schema.json`。

## 1.1 字段表

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `kind` | string | 是 | 必须为 `template` |
| `id` | string | 与 `templateKey` 至少一个 | `^[a-z0-9][a-z0-9-]{1,58}$`，2–59 位 |
| `templateKey` | string | 与 `id` 至少一个 | 规则同 `id`。两者都有时使用 `id` |
| `name` | string | 是 | 展示名称，去掉首尾空白后非空，入库截断到 100 字 |
| `version` | string | 是 | `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` |
| `description` | string | 是 | 非空，入库截断到 500 字 |
| `schemaVersion` | number | 是 | 必须为数字 `1` |
| `author` | string 或 object | 是 | 字符串，或 `{name,url,email}`；`name` 必填 |
| `author.name` | string | 是 | 作者名，截断到 100 字 |
| `author.url` | string | 否 | 截断到 300 字 |
| `author.email` | string | 否 | 截断到 200 字 |
| `category` | string | 否 | 模板类分类；省略则自动 `home-template` |
| `hero.title` | string | 是 | 首页主标题 |
| `scripts` | any | 禁止 | 字段出现即拒绝（`null` 除外） |

`id` 入库前会转成小写。`category` 不能填 `payment`、`realname`、`other`，也不能填插件类自定义分类（即使标识恰好叫 `template`）。

## 1.2 视觉与登录入口

下面这些字段**不参与** ZIP 硬校验。缺了仍能上传。宿主要画出整站和登录弹窗，章程要求写上它们。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `stylePreset` | string | `cartoon-blue`、`fintech-gold`，或省略。其他字符串整份模板失效，回退默认首页 |
| `theme.primaryColor` | string | `#` 加 3–8 位十六进制。非法则忽略该字段 |
| `theme.backgroundColor` | string | 同上 |
| `theme.textColor` | string | 同上 |
| `hero.badge` | string | 标题上方短标签 |
| `hero.highlight` | string | 标题中的强调片段 |
| `hero.description` | string | 副文案。空则用公开配置里的 `siteSubtitle` |
| `hero.imageUrl` | string | 声明式登记包只用 `https://`，或省略 |
| `hero.primaryAction.label` | string | 主按钮文案。空则显示「进入用户中心」 |
| `hero.primaryAction.type` | string | 登录入口。只能是 `login` |
| `hero.secondaryAction.label` | string | 次按钮文案 |
| `hero.secondaryAction.type` | string | 若写次按钮，`type` 也只能是 `login` |
| `features` | array | 写 3 条 `{icon,title,description}` |
| `features[].icon` | string | `ri:` 加小写，例如 `ri:shield-check-line`。不符合时标准版式换成 `ri:sparkling-line` |
| `footer.text` | string | 页脚。空则用站点名和年份 |

`hero.imageUrl` 的相对路径只在管理员本地上传静态包、并给出 `assetBaseUrl` 时重写。声明式登记包不要用相对路径。

## 1.3 完整示例

可复制副本：[`starter/template-example/template.json`](./starter/template-example/template.json)。

```json
{
  "kind": "template",
  "id": "demo-home",
  "templateKey": "demo-home",
  "name": "演示首页",
  "version": "1.0.0",
  "description": "AuthPro 源站声明式整站模板，含登录入口与能力卡片。",
  "schemaVersion": 1,
  "author": {
    "name": "示例作者"
  },
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
    "primaryAction": {
      "label": "进入用户中心",
      "type": "login"
    },
    "secondaryAction": {
      "label": "用户登录",
      "type": "login"
    }
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
  "footer": {
    "text": "安全、稳定的软件授权服务"
  }
}
```

换 `fintech-gold` 时只改 `stylePreset`，以及 `theme.primaryColor`、`theme.backgroundColor`、`theme.textColor`（建议 `#f0b90b`、`#0b0e11`、`#f5f5f5`）。不要为金黑版式再写一套页面，也不要写 `login.html`。

## 2. 登录弹窗主题

模板启用后，宿主用同一份字段画首页和登录弹窗。作者不另写 `login.html`。字段名与 `frontend/src/views/user-panel/login/home-template.ts` 一致。

| 字段 | 合法值 | 宿主怎么用 |
| --- | --- | --- |
| `stylePreset` | `cartoon-blue`、`fintech-gold`，或省略 | 选择首页和登录弹窗版式。其他字符串整份模板失效，回退默认首页 |
| `theme.primaryColor` | `#` 加 3–8 位十六进制 | 主色。`homeTemplateThemeStyle` 写入 `--remote-primary`，并同时写入 `--gold` |
| `theme.backgroundColor` | 同上 | 背景。写入 `--remote-background`，并同时写入 `--page-bg` |
| `theme.textColor` | 同上 | 文字。写入 `--remote-text`，并同时写入 `--text` |

颜色不匹配 `^#[0-9a-fA-F]{3,8}$` 时，该字段被忽略，改用宿主默认值。上传不会因此拒绝。

| `stylePreset` | `primaryColor` | `backgroundColor` | `textColor` |
| --- | --- | --- | --- |
| `fintech-gold` | `#f0b90b` | `#0b0e11` | `#f5f5f5` |
| `cartoon-blue` 或省略 | `#4d6bfe` | `#f7f8fc` | `#172033` |

| `stylePreset` | 登录弹窗 |
| --- | --- |
| `cartoon-blue` | 浅色圆角面板、胶囊按钮，类名 `remote-home--cartoon-blue` 与 `remote-login-dialog--cartoon-blue`。主色是 `theme.primaryColor` |
| `fintech-gold` | 金黑对话框，三色与首页相同。顶栏「注册」打开的是这个登录弹窗 |
| 省略 | 不换圆趣或金黑外形（类名 `remote-home--standard`）。登录按钮仍使用 `theme.primaryColor`（`--remote-primary`） |
| 未启用远程模板 | 默认首页的登录、注册、忘记密码不读上述字段，保持宿主默认弹窗 |

`hero.primaryAction.type` 必须是 `login`，并写 `label`。`hero.secondaryAction` 若出现，`type` 也必须是 `login`。两个按钮都打开宿主登录框，模板包不提交密码、不保存 token。

`cartoon-blue` 与省略预设最多展示 12 条 `features`。`fintech-gold` 只显示前 3 条；没有 `features` 时该版式使用组件内置的 3 条文案。`fintech-gold` 的查询区、三步说明和底部行动是宿主写死的，查询提交后打开登录框，没有对应 JSON。

启用本模板后，默认首页上的注册、忘记密码、授权查询、代理商查询、域名/IP 查询不会出现。下文第 5–10 节是这些宿主接口在**未启用远程模板**时的合同，方便对照站点能力。不要把它们写进 `template.json`。

## 3. 公开系统配置

站点名称、Logo、备案号由**宿主**读取，不要在 `template.json` 里写死，也不要在模板包里重复请求。

内置默认首页通过 `useSystemConfigStore` 使用这些字段。声明式模板的标题用 `hero.title`，副文案空着时宿主回退到 `siteSubtitle`。

```http
GET /api/system-config/public
```

## 3.1 配置字段

| 字段 | 用途 |
| --- | --- |
| `siteName` | 站点名称。宿主顶栏和登录框标题使用它 |
| `siteSubtitle` | 服务描述。`hero.description` 为空时使用 |
| `siteLogo` | Logo 原始地址 |
| `installedAt` | 安装时间 |
| `stationQQ` | 页脚联系 QQ |
| `icpNumber` | ICP 备案号 |
| `domainLicenseNotice` | 网站公告 |
| `registrationEnabled` | 为 `false` 时宿主关闭注册 |
| `selfPurchaseEnabled` | 是否允许自助购买 |
| `piracyDetectionEnabled` | 盗版检测开关 |
| `geetestEnabled` | 是否开启行为验证 |
| `geetestCaptchaId` | 行为验证客户端 ID |
| `geetestCaptchaKeySet` | 服务端是否已保存验证 Key。响应里没有 Key 本身 |

## 3.2 配置返回示例

```json
{
  "code": 200,
  "msg": "",
  "data": {
    "siteName": "授权管理系统",
    "siteSubtitle": "专业的软件授权与服务平台",
    "siteLogo": "",
    "installedAt": "2026-01-01 12:00:00",
    "stationQQ": "123456789",
    "icpNumber": "京ICP备00000000号",
    "domainLicenseNotice": "欢迎使用授权服务平台",
    "registrationEnabled": true,
    "selfPurchaseEnabled": true,
    "piracyDetectionEnabled": false,
    "geetestEnabled": false,
    "geetestCaptchaId": "",
    "geetestCaptchaKeySet": false
  }
}
```

## 4. 登录接口

登录框由宿主实现。声明式模板只声明入口：`hero.primaryAction.type` 为 `login`。

下面是宿主实际调用的接口，方便对照站点能力。不要在模板 ZIP 里自己保存 Token。

```http
POST /api/user-panel/login
Content-Type: application/json
```

请求参数：

```json
{
  "account": "手机号、邮箱或用户ID",
  "password": "登录密码"
}
```

`account` 为空时，宿主旧字段 `email` 仍可作为账号。开启行为验证时，宿主会附带：

```json
{
  "lot_number": "",
  "captcha_output": "",
  "pass_token": "",
  "gen_time": ""
}
```

登录支持邮箱、手机号、用户 ID。含 `@` 按邮箱；纯数字同时匹配手机号和用户 ID（手机号优先）。

成功响应示例：

```json
{
  "code": 200,
  "msg": "登录成功",
  "data": {
    "accessToken": "JWT_TOKEN",
    "userId": 1,
    "email": "user@example.com",
    "nickname": "用户"
  }
}
```

宿主登录成功后保存：

```text
localStorage.user_panel_token
localStorage.user_panel_info
```

默认跳转 `/user/dashboard`。地址栏里合法的 `redirect` 只有以 `/user` 开头的路径。

### 4.1 用户已经升级为代理商

```json
{
  "code": 409,
  "msg": "该账号已升级为代理，请前往代理端登录",
  "data": {
    "converted": true,
    "agentId": 1,
    "loginPath": "/agent-panel/login?upgraded=1"
  }
}
```

宿主跳转到 `data.loginPath`。没有该字段时使用 `/agent-panel/login?upgraded=1`。

## 5. 发送注册邮箱验证码

注册弹窗由宿主提供（内置默认首页的认证对话框）。声明式模板不包含这个表单。`registrationEnabled === false` 时接口返回业务码 `403`，文案为「普通用户注册已关闭，请联系管理员」。

```http
POST /api/user-panel/register/email-code
Content-Type: application/json
```

```json
{
  "email": "user@example.com"
}
```

开启行为验证时附带与登录相同的四个字段。

成功响应示例：

```json
{
  "code": 200,
  "msg": "验证码已发送，请查收邮件",
  "data": {
    "expiresIn": 600
  }
}
```

宿主发送成功后开始 60 秒倒计时。服务端限制：

- 验证码为 6 位数字
- 有效期 10 分钟（`expiresIn` 为 600 秒）
- 错误达到 5 次后失效
- 同一邮箱 1 分钟内不可重复发送
- 已注册邮箱返回「该邮箱已注册」
- 邮件服务未配置时发送失败

## 6. 注册用户

```http
POST /api/user-panel/register
Content-Type: application/json
```

```json
{
  "email": "user@example.com",
  "emailCode": "123456",
  "phone": "13800138000",
  "nickname": "用户昵称",
  "password": "至少6位密码"
}
```

| 字段 | 说明 |
| --- | --- |
| `email` | 必填 |
| `emailCode` | 必填，6 位数字 |
| `phone` | 可选，填写时须为 `1` 开头的 11 位数字 |
| `nickname` | 必填 |
| `password` | 必填，至少 6 位 |

成功响应示例：

```json
{
  "code": 200,
  "msg": "注册成功",
  "data": {
    "userId": 1
  }
}
```

`registrationEnabled` 为 `false` 时，宿主隐藏注册入口，接口业务码为 `403`。

## 7. 忘记密码

忘记密码由宿主完成，包含申请重置链接和提交新密码。模板包不要做重置页。

### 7.1 申请密码重置链接

```http
POST /api/user-panel/forgot-password
Content-Type: application/json
```

```json
{
  "email": "user@example.com"
}
```

成功响应示例：

```json
{
  "code": 200,
  "msg": "如果该邮箱已注册，重置链接将发送到您的邮箱，请注意查收"
}
```

已注册和未注册邮箱返回同一句文案。令牌有效期 30 分钟，只能使用一次。新申请会使旧令牌失效。同一邮箱 1 分钟内不可重复发送。

### 7.2 提交新密码

重置链接跳转到：

```text
/user/reset-password?token=重置令牌
```

```http
POST /api/user-panel/reset-password
Content-Type: application/json
```

```json
{
  "token": "重置令牌",
  "password": "新的登录密码"
}
```

密码至少 6 位。

```json
{
  "code": 200,
  "msg": "密码重置成功，请使用新密码登录"
}
```

## 8. 授权查询

必须携带用户登录令牌，并且只返回该用户自己的授权。未登录返回 HTTP 401。`account`、`email` 会被忽略，不能按昵称或邮箱枚举他人购买记录。内置默认首页的「查看我的授权」在没有 `user_panel_token` 时打开登录框。声明式模板（如 `fintech-gold`）的查询入口仍由宿主登录框接管。不要在模板里展示授权密钥。

```http
GET /api/user-panel/license-query
Authorization: Bearer <用户 accessToken>
```

```text
page      页码，默认 1
pageSize  每页数量，默认 10，最大 50
```

```text
/api/user-panel/license-query?page=1&pageSize=10
```

```json
{
  "code": 200,
  "msg": "",
  "data": {
    "list": [
      {
        "appName": "示例应用",
        "planName": "年度套餐",
        "licenseType": "domain",
        "licenseTypeName": "单域名",
        "status": "active",
        "statusName": "正常",
        "openedAt": "2026-01-01 12:00:00",
        "expiredAt": "2027-01-01 12:00:00",
        "permanent": false
      }
    ],
    "total": 1,
    "page": 1,
    "pageSize": 10
  }
}
```

页面可以展示：应用名称、套餐名称、授权类型、授权状态、开通时间、到期时间。

接口不会返回：授权密钥、域名或 IP 绑定目标、用户余额、用户手机号。

`licenseType` 还有 `wildcard`（泛域名）、`ip`、`key`。`status` 还有 `expired`（已过期）、`revoked`（已吊销）。`permanent` 为 `true` 时没有 `expiredAt`。

## 9. 代理商查询

```http
GET /api/user-panel/agent-query
```

```text
account   代理商邮箱或联系方式，必填
```

```text
/api/user-panel/agent-query?account=agent@example.com
```

```json
{
  "code": 200,
  "msg": "",
  "data": {
    "found": true,
    "account": "agent@example.com",
    "agentName": "示例代理商",
    "levelName": "高级代理"
  }
}
```

可以展示账号、名称、等级。查不到时 `data.found` 为 `false`，`msg` 为「未查询到当前账号」。

## 10. 域名/IP 查询

```http
GET /api/user-panel/target-query
```

```text
target   域名或 IP，必填
```

```text
/api/user-panel/target-query?target=example.com
```

```json
{
  "code": 200,
  "msg": "",
  "data": {
    "found": true,
    "target": "example.com",
    "isIP": false,
    "list": [
      {
        "appName": "示例应用",
        "status": "active",
        "statusName": "正常",
        "matchType": "exact",
        "matchedBy": "example.com",
        "expiredAt": "2027-01-01 12:00:00",
        "permanent": false
      }
    ],
    "total": 1
  }
}
```

- `matchType`：`exact` 精确匹配，`wildcard` 泛域名匹配
- `matchedBy`：实际命中的授权目标
- `isIP`：查询目标是否为 IP
- 格式不合法时业务码 `400`，文案「域名或 IP 格式不正确」

## 11. 打包命令

在清单所在目录打包，保证 `template.json` 在 ZIP 根目录：

```bash
zip -r ../demo-home.zip template.json assets
sha256sum ../demo-home.zip
```

Windows PowerShell：

```powershell
Compress-Archive -Path template.json, assets -DestinationPath ..\demo-home.zip -Force
Get-FileHash ..\demo-home.zip -Algorithm SHA256
```

`sha256` 是**整个 ZIP** 的 64 位十六进制。可以在登记表单上传 ZIP，由源站托管并回填地址与校验码；也可以把 ZIP 放到自己的 HTTPS 空间。提交审核时外链会被下载核对，并且必须解析到公网地址。回环、私网、链路本地和云元数据会被拒绝。本站托管 ZIP 只核对本地文件。

最小包可以只有 `template.json`。有封面图时再加 `assets/`。

## 12. 登记表单对照

开发者面板「我的模板 → 登记模板」把表单标签写成目录元数据，不是再传一份 ZIP。

| 表单标签 | JSON 字段 | 规则 |
| --- | --- | --- |
| 应用 | `appId` | 必填。条目只属于这一个应用 |
| 分类 | `category` | 必填。模板类，默认选 `home-template` |
| 名称 | `name` | 必填，≤100 字 |
| 标识 | `id`，同时写入 `templateKey` | 2–59 位小写字母、数字、连字符。新建时按名称生成，提交后不可改 |
| 版本 | `version` | 首次可用 `1.0.0` |
| 模板地址 | `templateUrl` | 草稿可空。提交审核前必填。`https://` 或相对路径，如 `templates/demo-home.json` |
| 校验码 (SHA256) | `sha256` | 草稿可空。提交审核前必填，64 位十六进制 |
| 简介 | `description` | 表单可空。ZIP 硬校验要求清单内非空 |
| 作者 | `author.name` | 表单锁定为当前开发者名称 |
| 更新说明 | `changelog` | 可选，≤2000 字 |

保存草稿不要求地址和校验码。提交审核前两者都要有。面板不单独填写 `schemaVersion`，服务端缺省写成 `1`；请求里若带了其他数字会被拒绝。

公开目录：

```text
GET /software-source/{app_key}/index.json
```

上架后出现在该应用的 `homeTemplates`。

## 13. 拒绝信息

失败响应形如：

```json
{
  "code": 400,
  "msg": "template.json 缺少 kind（必须为 template）",
  "error": { "field": "kind", "rule": "required" }
}
```

| field | rule | msg |
| --- | --- | --- |
| `kind` | `required` | `template.json 缺少 kind（必须为 template）` |
| `kind` | `mismatch` | `template.json 的 kind 不能是 plugin` |
| `kind` | `invalid` | `template.json 字段 kind 必须为 template` |
| `schemaVersion` | `required` | `template.json 缺少 schemaVersion（必须为 1）` |
| `schemaVersion` | `format` | `template.json 字段 schemaVersion 必须为 1` |
| `hero.title` | `required` | `template.json 不合规：模板文件缺少 hero.title` |
| `scripts` | `forbidden` | `template.json 不合规：声明式模板不允许 scripts 字段` |
| `id` | `required` | `template.json 缺少 id / templateKey` |
| `id` | `format` | `template.json 字段 id/templateKey 不合法：须为 2-59 位小写字母、数字或连字符` |
| `name` | `required` | `template.json 缺少 name` |
| `version` | `required` | `template.json 缺少 version` |
| `version` | `format` | `template.json 字段 version 不合法` |
| `description` | `required` | `template.json 缺少 description` |
| `author` | `required` | `template.json 缺少 author` |
| `author.name` | `required` | `template.json 字段 author.name 必填（author 可为字符串或对象）` |
| `category` | `format` | `template.json 字段 category 不合法：` 加具体原因 |
| `template.json` | `require_manifest` | `首页模板分类的安装包必须在根目录或一层子目录包含 template.json` |
| `template.json` | `json` | `template.json 不是有效 JSON` |
| `template.json` | `encoding` | `template.json 必须是 UTF-8 文本` |

分类跨类型时的原因文案：

- `该分类属于插件，不能用于首页模板包`
- `未知分类，请先在目录分类中配置`

登记接口上的文案（不一定带 `error.field`）：

| 场景 | msg |
| --- | --- |
| 标识不合格 | `模板标识不合法` |
| `schemaVersion` 不是 1 | `schemaVersion 必须为 1` |
| `templateUrl` 不合格 | `templateUrl 须为 https:// 或相对路径（如 templates/clean-home.json）` |
| `sha256` 不合格 | `sha256 必须是 64 位十六进制` |

`stylePreset`、`theme`、`primaryAction` 缺失**不会**出现在上表。缺了能上传。`stylePreset` 写成 `cartoon-blue`、`fintech-gold` 以外的字符串时，宿主把整份模板判为非法并回退默认首页。登录弹窗主题见第 2 节。

## 14. 管理员静态 ZIP（旧路径）

这是管理端安装通道，不是开发者「登记模板」。

```http
POST /api/system/home-templates/upload
```

入口在管理端「上传首页模板 ZIP」。包体同样不超过 20 MiB。安装顺序是：根目录有 `index.html` 就用静态页，并把 `schemaVersion` 记为 0；没有才用 `template.json`。静态页在隔离 iframe 里打开，只能 `postMessage({ type: 'auth-pro:login' })` 唤起宿主登录框，不能自己持有密码或 token。

开发者登记不要走这条接口。登记包不要同时放 `index.html` 和 `template.json`，否则安装报「软件源模板目录与 ZIP 入口类型不一致」。

## 15. 功能归属

| 能力 | 谁负责 |
| --- | --- |
| 压缩包格式、`kind`、`hero.title` | 作者写在 `template.json`，源站硬校验 |
| `stylePreset`、`theme.primaryColor`、`theme.backgroundColor`、`theme.textColor` | 作者声明。启用后首页和登录弹窗共用 |
| 登录按钮 | 作者写 `hero.primaryAction.type = "login"` 和 `label`。宿主打开登录框，不写 `login.html` |
| 未启用模板时的注册、忘记密码 | 默认首页的宿主对话框。启用本模板后这些面不出现 |
| 站点名、Logo、备案、注册开关 | `GET /api/system-config/public` |
| 授权 / 代理商 / 域名查询 | 未启用远程模板时，默认首页调用公开接口 |
| 开发者登记 | 开发者面板元数据：`templateUrl` + `sha256` |
| 本机安装静态 HTML | 仅管理员上传接口 |

## 16. 开发注意事项

- 登记包根目录是 `template.json`，不要同时放入 `index.html` 或 `login.html`。
- `kind` 必须为 `template`，`schemaVersion` 必须为数字 `1`，必须有 `hero.title`。
- 登录入口字段名是 `hero.primaryAction.type`，值是 `login`。按钮文字是 `hero.primaryAction.label`。
- 启用后登录弹窗跟随 `stylePreset`（`cartoon-blue` / `fintech-gold`，或省略）以及 `theme.primaryColor`、`theme.backgroundColor`、`theme.textColor`。其他 `stylePreset` 会使整份模板失效。
- 不要在模板里保存密码或 `user_panel_token`。
- 公开查询结果不要展示授权密钥、绑定目标和余额。
- 页面提示优先用接口返回的 `msg`。
- 上传的 ZIP 由本站托管。外链用 HTTPS，或相对该应用目录的路径。本站托管地址提交时只核对本地文件。
- 改包请新增版本，见 [更新与多版本](./versions.md)。
