# 整站模板

登记模板时，ZIP 里要有 `template.json`，并且 `kind` 为 `template`。这和本站另行接受的静态 `index.html` 不是同一条闸门，静态页见 [首页模板](../home-template.md)。

## 硬校验

上传解析会拒绝下面缺项或写错的包：

| 字段 | 规则 |
| --- | --- |
| `kind` | 必填，必须是 `template` |
| `id` 或 `templateKey` | 至少一个，规则与插件 `id` 相同 |
| `name` | 必填，最长 100 字 |
| `version` | 必填，规则与插件相同 |
| `description` | 必填，最长 500 字 |
| `schemaVersion` | 必填，数字 `1`，不能写成字符串 |
| `author` | 必填，规则与插件相同 |
| `hero.title` | 必填 |
| `scripts` | 禁止出现 |
| `category` | 可空。空则自动记为 `home-template`。不能填插件分类 |

## 宿主会读、但上传不强制的字段

| 字段 | 作用 |
| --- | --- |
| `stylePreset` | `cartoon-blue` 或 `fintech-gold`。其它或省略时界面按 `standard` |
| `theme.primaryColor` / `backgroundColor` / `textColor` | 首页和登录框颜色 |
| `hero.primaryAction.type` | 值为 `login` 时，按钮打开宿主登录框。不是上传拒绝条件 |
| `hero.primaryAction.label` | 按钮文字，省略时为「进入用户中心」 |
| `features` | 列表。普通预设最多显示 12 条，`fintech-gold` 最多 3 条 |
| `footer.text` | 页脚 |

不要在模板里写登录接口、保存 Token，或再放一个 `login.html`。登录框属于宿主。

## 最小 template.json

```json
{
  "kind": "template",
  "id": "demo-home",
  "name": "演示首页",
  "version": "1.0.0",
  "description": "声明式首页，登录由站点打开。",
  "schemaVersion": 1,
  "author": { "name": "示例作者" },
  "hero": { "title": "专业授权服务" }
}
```

带登录按钮和配色、且同样能通过硬校验的示例：`docs/developer/starter/template-example/template.json`。

售价不在 `template.json` 里。登记时默认 0；大于 0 必须上传本站 ZIP，付费上架尚未开放。
