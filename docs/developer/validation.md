# 拒绝与改法

ZIP 失败响应：

```json
{
  "code": 400,
  "msg": "template.json 缺少 kind（必须为 template）",
  "error": { "field": "kind", "rule": "required" }
}
```

失败不写库。先改包或表单，再解析或保存。

## ZIP

| field | rule | 改法 |
| --- | --- | --- |
| `file` | `required` | 上传非空 ZIP |
| `file` | `require_zip` | 用 `zip` 打包，确认文件以 `PK` 开头 |
| `file` | `max_size` | 压到 20 MiB 以内 |
| `file` | `zip_layout` | 去掉 `..`、反斜杠、符号链接、空包、重复路径。见 msg 冒号后的原文 |
| `plugin.json` | `require_manifest` | 插件分类的包要有 `plugin.json`，放根目录或一层子目录 |
| `template.json` | `require_manifest` | 模板分类的包要有 `template.json`，同样深度 |
| `plugin.json` | `json` / `encoding` | UTF-8 JSON |
| `template.json` | `json` / `encoding` | 同上。`schemaVersion` 不要写成字符串 `"1"` |
| `kind` | `required` | 模板加上 `"kind": "template"` |
| `kind` | `mismatch` | 插件清单不要写 `template`，模板清单不要写 `plugin` |
| `kind` | `invalid` | 只能是 `plugin` 或 `template` |
| `id` | `required` / `format` | 2–59 位小写字母、数字、连字符 |
| `name` / `version` / `description` | `required` | 补字段。版本匹配 `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` |
| `author` / `author.name` | `required` | `"author": { "name": "示例作者" }` 或 `"author": "示例作者"` |
| `schemaVersion` | `required` / `format` | 数字 `1` |
| `hero.title` | `required` | `"hero": { "title": "..." }` |
| `scripts` | `forbidden` | 删除 `scripts`，不要留空数组 |
| `category` | `kind` / `unknown` | 插件用 `payment`/`realname`/`other`。模板用 `home-template` 或省略 |

常见 msg：

| msg | 改法 |
| --- | --- |
| `template.json 缺少 kind（必须为 template）` | 写入 kind |
| `该分类属于首页模板，不能用于插件包` | 插件改分类 |
| `该分类属于插件，不能用于首页模板包` | 模板改 `home-template` |
| `未知分类，请先在目录分类中配置` | 改用内置分类 |
| `压缩包必须包含 plugin.json 或 template.json` | 补清单 |

## 登记接口

这些响应通常只有 `msg`：

| msg | 改法 |
| --- | --- |
| `插件标识不合法` / `模板标识不合法` | 改标识格式 |
| `不能覆盖内置插件标识` | 换 id |
| `必须是 https:// 外部地址，源站不保存插件或模板源码` | 插件改用 https |
| `templateUrl 须为 https:// 或相对路径（如 templates/clean-home.json）` | 去掉前导 `/` 和 `..` |
| `地址不合法` | https 路径里不要 `..` |
| `sha256 必须是 64 位十六进制` | 对整个 ZIP 重算 |
| `版本号不合法` | 改版本格式 |
| `schemaVersion 必须为 1` | 登记 JSON 里用数字 1 |
| `必须绑定应用` / `应用不存在` | 重选应用 |
| `上架需要 64 位 sha256 和外部下载/模板地址` | 管理员上架前补齐。开发者应在提交审核前就填好 |
| `已发布版本不可改包地址，请创建新版本` | 走「版本」 |
| `当前状态不允许该操作` | 只有草稿和已驳回能再提交 |
| `软件源模板目录与 ZIP 入口类型不一致，请重新发布` | 登记包去掉 `index.html`，只留 `template.json` |
| `软件源模板 ZIP 的 SHA256 校验失败` | 登记的校验码不是当前文件，重新 `sha256sum` |
| `plugin.json 的插件 ID 必须与软件源一致` | 清单 `id` 改成登记标识 |

界面校验（请求还没发出）：

| 原文 | 改法 |
| --- | --- |
| `标识只能用小写字母、数字和连字符，至少 2 位` | 手写标识 |
| `下载地址须为 https 开头的外链` | 插件外链改成 https，或改用上传 ZIP |
| `请先上传 ZIP` | 包来源是上传，但还没有文件 |
| `外链不可达` | 提交审核时下载失败或不是 2xx |
| `外链不是 ZIP` | 外链内容没有 ZIP 魔数 |
| `外链内容与 sha256 不一致` | 按实际文件重算校验码 |
| `拒绝访问非公网地址` | 外链解析到回环、私网或非全球单播地址。改成公网 HTTPS，或改用上传 ZIP |
| `拒绝访问链路本地或云元数据地址` | 不要指向 `169.254.0.0/16`、`fe80::/10`、`100.64.0.0/10` 或云元数据主机名 |
| `外链重定向离开 https` | 重定向目标必须仍是 https |
| `重定向过多` | 跳转不超过 3 次 |
| `sha256 与本站托管 ZIP 不一致` | 重新上传，不要手改本站地址或校验码 |
| `模板地址须为 https 开头，或相对路径如 templates/demo-home.json` | 模板地址 |
| `校验码须为 64 位十六进制` | 重算 |
| `提交审核前请先填写下载地址和校验码` | 两项都填再点提交审核 |
