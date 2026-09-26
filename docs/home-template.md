# 首页模板

本站安装首页模板时，ZIP 里要有入口文件。安装函数先找 `index.html`，没有再找 `template.json`（允许外面套一层目录）。入口文件不超过 2 MiB。整包仍受插件包上限约束：ZIP 不超过 20 MiB，解压后不超过 100 MiB，条目不超过 2048，并拒绝路径穿越、符号链接和特殊文件。

这和开发者在源站 **登记** 时的硬校验不是同一条。登记必须带完整的 `template.json` 清单（含 `kind`、作者、版本等），见 [整站模板](developer/template-package.md)。下面说的是本站「应用商店 / 首页模板」装到 `/user/login` 上的包。

管理端「接入开发 → 模板文档」渲染的就是本文。

## 声明式 template.json

入口是 `template.json` 时，服务端调用与登记相同的文档校验：

- `schemaVersion` 必须是数字 `1`
- `hero.title` 必填
- 不允许 `scripts` 字段

登记清单里的 `kind`、`id`、`author` 等，是源站上传闸门额外要求的。只在本站上传安装、且入口已是合法 `template.json` 时，不会再套一遍源站的 `kind` 规则。

宿主实际读取的字段（缺了不会在上传时拒绝，页面上只是没有对应区块）：

| 字段 | 作用 |
| --- | --- |
| `stylePreset` | `cartoon-blue` 或 `fintech-gold`。其它值或省略时按 `standard` |
| `theme.primaryColor` / `backgroundColor` / `textColor` | 首页和登录框的颜色。非法颜色回退到该预设的默认色 |
| `hero.primaryAction` | `type` 为 `login` 时，按钮打开宿主登录框。标签用 `label`，省略时按钮文案为「进入用户中心」 |
| `hero.secondaryAction` | 同样只认 `type: "login"` |
| `features` | 普通预设最多显示 12 条。`fintech-gold` 最多 3 条 |
| `footer.text` | 页脚文字 |

登录框是宿主的，模板里不要收集密码或保存 Token。不要写 `login.html`。

可直接通过源站硬校验的示例在 `docs/developer/starter/template-example/template.json`。

## 静态 index.html

入口是 `index.html` 时，文件必须非空且不能包含 NUL。安装后以静态页格式提供，资源地址在 `/api/home-template/assets/...`。

静态页放在 iframe 里。资源响应带 `Content-Security-Policy: sandbox allow-scripts ...`。登录按钮通知宿主即可：

```html
<button type="button" onclick="parent.postMessage({ type: 'auth-pro:login' }, '*')">登录</button>
```

宿主只接受 **这个 iframe** 发出的 `auth-pro:login`，不会把 Token 交给模板。静态页不应假设能读取主站 Cookie 或调用管理接口。

若 ZIP 里同时有 `index.html` 和 `template.json`，安装时优先 `index.html`。

软件源目录里的 `schemaVersion` 必须和包入口一致：静态页按 `0`，声明式 JSON 按 `1`。不一致时安装失败，错误为「软件源模板目录与 ZIP 入口类型不一致，请重新发布」。

## 和登记包的差别

| | 源站登记 | 本站安装 |
| --- | --- | --- |
| 插件 | `plugin.json` | 软件源 ZIP 解压到 `plugins/<id>/`，不执行包内脚本 |
| 模板 | 必须是带 `kind: "template"` 的 `template.json` | `template.json` 或 `index.html` |
| 地址 | 免费公开 HTTPS + SHA256；收费上传 ZIP 或公开地址，由本站保管 | 从软件源下载后再安装、启用 |

启用、停用、卸载在应用商店的首页模板操作里完成。同一时间只有一个活动模板。拉取或渲染失败时，`/user/login` 使用内置默认首页，浏览器地址不变。
