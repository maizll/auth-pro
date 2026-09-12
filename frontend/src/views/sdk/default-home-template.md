# 重要提示：模版压缩包格式

首页模版使用 ZIP 压缩包发布，压缩包根目录需要包含 `index.html`，页面使用的 CSS 和 JS 文件统一放在 `assets` 文件夹中。

目录结构如下：

```text
首页模版.zip
├── index.html
└── assets/
    ├── index.css
    └── index.js
```

## 文件要求

- `index.html`：模版入口文件，必须位于压缩包根目录。
- `assets/`：静态资源目录，用于存放 CSS、JS 以及页面需要的图片、字体等资源。
- CSS 和 JS 文件建议通过相对路径引用，例如：

```html
<link rel="stylesheet" href="./assets/index.css" />
<script src="./assets/index.js"></script>
```

- 资源文件必须实际存在于压缩包内。
- 文件名和路径大小写应保持引用关系一致。
- 不要在模版中写入服务器绝对路径或本地磁盘路径。
- 不要在模版中保存用户密码、Token 或其他敏感信息。

## 1. 公开系统配置

默认首页通过 `useSystemConfigStore` 获取公开配置：

```typescript
import { useSystemConfigStore } from '@/store/modules/system-config'

const systemConfigStore = useSystemConfigStore()
const {
  siteName,
  siteSubtitle,
  resolvedLogo,
  stationQQ,
  icpNumber,
  domainLicenseNotice,
  registrationEnabled
} = storeToRefs(systemConfigStore)
```

配置请求由 Store 内部完成，接口为：

```http
GET /api/system-config/public
```

前端建议统一通过 `useSystemConfigStore` 使用配置，不要在模板中重复请求该接口。

## 1.1 配置字段

| 字段 | 用途 |
| --- | --- |
| `siteName` | 站点名称、品牌名称 |
| `siteSubtitle` | 首页服务描述、副标题 |
| `siteLogo` | 站点 Logo 原始地址 |
| `resolvedLogo` | Store 处理后的 Logo 地址，Logo为空时使用默认 Logo |
| `stationQQ` | 页脚服务联系 QQ |
| `icpNumber` | 页脚 ICP备案号 |
| `domainLicenseNotice` | 网站公告内容 |
| `registrationEnabled` | 是否显示和允许用户注册 |
| `geetestEnabled` | 是否开启行为验证 |
| `geetestCaptchaId` | 行为验证客户端 ID |

## 1.2 配置返回示例

```json
{
  "code": 200,
  "msg": "",
  "data": {
    "siteName": "授权服务平台",
    "siteSubtitle": "专业的软件授权与服务平台",
    "siteLogo": "https://example.com/logo.png",
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

## 2. 登录接口

用于首页登录弹窗登录用户中心。

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

如果后台开启行为验证，需要在请求中附加行为验证参数。

登录支持：

- 邮箱
- 手机号
- 用户 ID

登录成功响应示例：

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

登录成功后保存：

```text
localStorage.user_panel_token
localStorage.user_panel_info
```

默认跳转地址：

```text
/user/dashboard
```

如果地址中存在合法的用户端 `redirect` 参数，则跳转到该地址。只允许跳转到 `/user` 开头的路径。

### 2.1 用户已经升级为代理商

如果用户账号已经升级为代理商，接口会返回：

```json
{
  "code": 409,
  "msg": "该账号已升级为代理，请前往代理端登录",
  "data": {
    "converted": true,
    "loginPath": "/agent-panel/login?upgraded=1"
  }
}
```

模板应跳转到 `data.loginPath`，没有该字段时使用：

```text
/agent-panel/login?upgraded=1
```

## 3. 发送注册邮箱验证码

用于注册弹窗发送邮箱验证码。

```http
POST /api/user-panel/register/email-code
Content-Type: application/json
```

请求参数：

```json
{
  "email": "user@example.com"
}
```

开启行为验证时，需要附加行为验证参数。

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

前端发送成功后建议开始 60 秒倒计时。

服务端限制：

- 验证码为 6 位数字
- 验证码有效期 10 分钟
- 验证码错误达到 5 次后失效
- 同一邮箱存在发送频率限制
- 已注册邮箱不能重复注册
- 邮件服务未配置时发送失败

## 4. 注册用户

用于注册弹窗创建用户账号。

```http
POST /api/user-panel/register
Content-Type: application/json
```

请求参数：

```json
{
  "email": "user@example.com",
  "emailCode": "123456",
  "phone": "13800138000",
  "nickname": "用户昵称",
  "password": "至少6位密码"
}
```

字段说明：

- `email`：必填，注册邮箱
- `emailCode`：必填，邮箱验证码
- `phone`：可选，手机号
- `nickname`：必填，用户昵称
- `password`：必填，至少 6 位

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

注册开关：

```text
registrationEnabled === true
```

当 `registrationEnabled` 为 `false` 时：

- 隐藏注册入口
- 不允许提交注册请求
- 后端返回 `403`

## 5. 忘记密码

忘记密码包含“申请重置链接”和“提交新密码”两个接口。

### 5.1 申请密码重置链接

```http
POST /api/user-panel/forgot-password
Content-Type: application/json
```

请求参数：

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

安全要求：

- 已注册邮箱和未注册邮箱返回统一提示
- 重置令牌有效期 30 分钟
- 重置令牌只能使用一次
- 新申请的令牌会使旧令牌失效

### 5.2 提交新密码

重置链接跳转到：

```text
/user/reset-password?token=重置令牌
```

提交新密码接口：

```http
POST /api/user-panel/reset-password
Content-Type: application/json
```

请求参数：

```json
{
  "token": "重置令牌",
  "password": "新的登录密码"
}
```

密码至少 6 位。

成功响应示例：

```json
{
  "code": 200,
  "msg": "密码重置成功，请使用新密码登录"
}
```

## 6. 授权查询

首页无需登录即可按用户账号或注册邮箱查询授权概况。

```http
GET /api/user-panel/license-query
```

请求参数：

```text
account   用户账号或注册邮箱，必填
page      页码，通常为 1
pageSize  每页数量，通常为 50
```

示例：

```text
/api/user-panel/license-query?account=user@example.com&page=1&pageSize=50
```

成功响应示例：

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
    "total": 1
  }
}
```

页面允许展示：

- 应用名称
- 套餐名称
- 授权类型
- 授权状态
- 开通时间
- 到期时间

接口不会返回或页面不应展示：

- 授权密钥
- 域名或 IP 绑定目标
- 用户余额
- 用户手机号

## 7. 代理商查询

首页无需登录即可查询代理商公开信息。

```http
GET /api/user-panel/agent-query
```

请求参数：

```text
account   代理商账号，必填
```

示例：

```text
/api/user-panel/agent-query?account=agent@example.com
```

成功响应示例：

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

页面可以展示：

- 代理商账号
- 代理商名称
- 代理等级

查询不到代理商时，`data.found` 为 `false`，页面显示未查询到结果。

## 8. 域名/IP 查询

首页无需登录即可根据域名或 IP 查询授权覆盖情况。

```http
GET /api/user-panel/target-query
```

请求参数：

```text
target   域名或 IP 地址，必填
```

示例：

```text
/api/user-panel/target-query?target=example.com
```

成功响应示例：

```json
{
  "code": 200,
  "msg": "",
  "data": {
    "target": "example.com",
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
    ]
  }
}
```

字段说明：

- `matchType`：`exact` 表示精确匹配，`wildcard` 表示泛域名匹配
- `matchedBy`：实际匹配到的授权目标
- `expiredAt`：到期时间
- `permanent`：是否永久有效

## 9. 首页功能与接口对应关系

| 首页功能 | 接口或配置 |
| --- | --- |
| 站点名称、Logo、副标题 | `useSystemConfigStore` → `GET /api/system-config/public` |
| 页脚联系 QQ、ICP备案号 | `useSystemConfigStore` → `GET /api/system-config/public` |
| 网站公告 | `useSystemConfigStore` → `GET /api/system-config/public` |
| 登录 | `POST /api/user-panel/login` |
| 发送注册邮箱验证码 | `POST /api/user-panel/register/email-code` |
| 注册用户 | `POST /api/user-panel/register` |
| 申请密码重置 | `POST /api/user-panel/forgot-password` |
| 提交新密码 | `POST /api/user-panel/reset-password` |
| 授权查询 | `GET /api/user-panel/license-query` |
| 代理商查询 | `GET /api/user-panel/agent-query` |
| 域名/IP 查询 | `GET /api/user-panel/target-query` |

## 10. 开发注意事项

- 首页公开查询接口不需要用户登录。
- 用户登录成功后才保存 `user_panel_token`。
- 不要在首页模板中保存用户密码。
- 不要在公开查询结果中展示授权密钥、授权目标和余额。
- 注册入口应根据 `registrationEnabled` 控制显示。
- 页面提示优先使用接口返回的 `msg`。
- 请求失败时显示网络错误，不影响其他首页功能。
- 所有接口地址都以 `/api` 开头，开发环境由 Vite 代理到后端。
