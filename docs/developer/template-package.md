# 整站模板

合同全文见 [章程](./charter.md)。登记产物是**一份** `template.json`，不是一组 HTML 页面。宿主用它画出整站公开页和登录框。

## 目录

```text
demo-home.zip
└── template.json
```

不要放入 `index.html`。安装程序见到 `index.html` 会把它当静态入口（schemaVersion 0），与登记时固定提交的 `schemaVersion: 1` 冲突。

## 宿主画出的面

| 面 | 作者写入 |
| --- | --- |
| 顶栏站名 / Logo / 「登录」 | 站名来自系统配置。登录按钮宿主自带 |
| 主视觉 | `hero.title`（必填）以及 `badge`、`highlight`、`description` |
| 登录框 | `hero.primaryAction`: `{ "label": "进入用户中心", "type": "login" }`。宿主弹窗负责账号、密码、极验和 token。启用后外观跟随 `stylePreset` 与 `theme.primaryColor`、`theme.backgroundColor`、`theme.textColor`。不要写 `login.html` |
| 能力卡片 | `features` 写 3 条。`fintech-gold` 只显示前 3 条，标准预设最多 12 条 |
| 页脚 | `footer.text` |
| `stylePreset: "fintech-gold"` 时的查询区、三步说明、底部行动 | 宿主写死，没有 JSON。查询提交后打开登录框 |

`stylePreset` 只能是 `cartoon-blue`、`fintech-gold` 或省略。其他值会导致整份模板失效并回退默认首页。

启用后，默认首页上的注册、忘记密码、授权查询、代理商查询、域名查询**不会出现**。不要写 `pages`、`scripts`、`register`。

## 登录弹窗主题

模板启用后，宿主登录弹窗与首页共用下面这些字段。不要写 `login.html`。

| 字段 | 写什么 | 宿主变量 |
| --- | --- | --- |
| `stylePreset` | `cartoon-blue` 或 `fintech-gold`。省略则不换这两套外形 | 版式 |
| `theme.primaryColor` | `#` 加 3–8 位十六进制，例如 `#168fe5` | `--remote-primary`；`fintech-gold` 同时是 `--gold` |
| `theme.backgroundColor` | 同上，例如 `#f1faff` | `--remote-background`；`fintech-gold` 同时是 `--page-bg` |
| `theme.textColor` | 同上，例如 `#15334a` | `--remote-text`；`fintech-gold` 同时是 `--text` |

`cartoon-blue` 是浅色圆角、胶囊按钮。`fintech-gold` 是金黑对话框，顶栏「注册」打开的也是这个登录弹窗。省略 `stylePreset` 时外形保持宿主默认，登录按钮仍用 `theme.primaryColor`。颜色非法则忽略该字段：`fintech-gold` 默认 `#f0b90b` / `#0b0e11` / `#f5f5f5`，其余默认 `#4d6bfe` / `#f7f8fc` / `#172033`。未启用模板时，默认首页的登录、注册、忘记密码不读这些字段。

## 清单

`kind` 必须是 `"template"`。`schemaVersion` 必须是数字 `1`。`category` 写 `home-template`（省略也会自动填这个）。

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
    { "icon": "ri:shield-check-line", "title": "安全验证", "description": "授权状态经过校验，账户与服务信息清晰可查。" },
    { "icon": "ri:refresh-line", "title": "实时同步", "description": "授权期限和使用状态及时更新。" },
    { "icon": "ri:customer-service-2-line", "title": "用户中心", "description": "从首页打开登录框，进入用户中心。" }
  ],
  "footer": { "text": "安全、稳定的软件授权服务" }
}
```

可复制文件：[`starter/template-example/template.json`](./starter/template-example/template.json)。

图标用 `ri:` 前缀（`^ri:[a-z0-9-]+$`）。颜色用 `#` 加 3–8 位十六进制。`hero.imageUrl` 只用 `https://` 或省略。

## 打包与登记

```bash
cd docs/developer/starter/template-example
rm -f /tmp/demo-home.zip
zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip
```

开发者面板 → 我的模板 → **登记模板**。分类选「首页模板」（`home-template`）。标识手改成 `demo-home`。模板地址填该 ZIP 的 `https://`（或符合章程的相对路径）。校验码填 sha256。简介与清单 `description` 保持一致。
