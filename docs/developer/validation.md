# 拒绝与改法

上传解析失败时返回 `error.field` 与 `error.rule`。下面是代码里会出现的情况。

## 清单

| 现象 | 改法 |
| --- | --- |
| `plugin.json` / `template.json`，`require_manifest` | 把对应清单放到 ZIP 根目录或一层子目录 |
| `file` / `require_zip` | 上传 ZIP，不要上传裸 json |
| `file` / `max_size` | 压缩包超过 20 MiB |
| `file` / `zip_layout` | 去掉 `..`、绝对路径、符号链接、重复路径 |
| `kind` / `required` | 模板补上 `"kind": "template"` |
| `kind` / `mismatch` | 插件不要写 `template`，模板不要写 `plugin` |
| `id` / `format` | 2–59 位小写字母、数字、连字符，且不能以连字符开头 |
| `version` / `format` | 用 `1.0.0` 这种版本串，不要用空格 |
| `schemaVersion` / `format` | 数字 `1`，不要加引号 |
| `hero.title` / `required` | 补上标题 |
| `scripts` / `forbidden` | 删除 `scripts` |
| `category` / `kind` | 插件不要用首页模板分类，模板不要用支付、实名或「其他」 |
| `author.name` / `required` | `author` 写成字符串，或对象里带 `name` |

## 地址与校验码

| 现象 | 改法 |
| --- | --- |
| 提交审核前提示填写地址和校验码 | 先上传 ZIP，或把 HTTPS 与 64 位 SHA256 填全 |
| `sha256 必须是 64 位十六进制` | 对 ZIP 重新 `sha256sum`，不要对 json 计算 |
| 校验码与文件不一致 | 外链文件已更换，或抄错了摘要 |
| `下载地址须为 https 开头的外链，或上传 ZIP 由本站托管` | 公网地址改用 HTTPS，或改为上传 ZIP。不要手写相对路径 |
| `拒绝访问非公网地址` | 外链不要指向私网。请上传 ZIP，或改用公网 HTTPS |
| `拒绝访问链路本地或云元数据地址` | 不要指向链路本地或云元数据 |
| `不能覆盖内置插件标识` | 换一个 `id`，不要用 `epay`、`epay-v2`、`alipay-f2f` 以及实名类内置 id |
| `付费条目必须上传 ZIP 由本站托管，不能使用外链` | 售价大于 0 时改用上传 ZIP |
| `年付尚未开放，当前只支持买断` | 去掉 `billing: "yearly"` |
| `付费条目暂不能上架，购买与交付将在后续版本开放` | 先保持免费，或只保存草稿 |
| `已公开的免费条目不能直接改为付费` | 已上架或已有正式版本的免费条目不要改售价 |
| `售价须为非负金额，最多两位小数` | 界面按元填写，不要写负数或超过两位小数 |

## 装到首页时才会出现

这些不是登记解析的错误。消费者安装软件源里的模板时：

| 现象 | 改法 |
| --- | --- |
| 软件源模板 ZIP 的 SHA256 校验失败 | 目录里的 `sha256` 必须是 ZIP 字节的摘要 |
| 软件源模板目录与 ZIP 入口类型不一致 | 静态 `index.html` 的目录 `schemaVersion` 为 0；声明式 `template.json` 为 1 |
| ZIP 根目录需包含 index.html 或 template.json | 本站安装包缺入口，或入口超过 2 MiB |
