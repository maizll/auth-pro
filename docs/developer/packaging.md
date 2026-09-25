# 打包与登记

## ZIP 限制

登记上传与管理员解析使用同一套限制：

- 必须是 ZIP（文件头为 `PK`），表单字段名 `file`
- 不超过 20 MiB
- 解压后不超过 100 MiB，条目不超过 2048
- 清单在根目录或一层子目录
- 拒绝路径穿越、绝对路径、符号链接、重复或大小写冲突的路径、空包、只有 `__MACOSX` 的包

`kind` 可随表单提交。不提交时按包里是 `plugin.json` 还是 `template.json` 识别。两者都在时，未指定 `kind` 会先按插件解析。

## 打包命令

在示例目录执行。`-X` 去掉额外属性，便于 SHA256 稳定。

```bash
cd docs/developer/starter/plugin-example
rm -f /tmp/demo-widget.zip
zip -X -r /tmp/demo-widget.zip plugin.json
sha256sum /tmp/demo-widget.zip
```

模板把目录换成 `docs/developer/starter/template-example`，把 `plugin.json` 换成 `template.json`。

Windows 可用资源管理器压缩该 json，再计算 SHA256。算的是 ZIP 文件，不是 json 文本。

## 登记时怎么填

| 表单标签 | 插件 | 模板 |
| --- | --- | --- |
| 标识 | `demo-widget` | `demo-home` |
| 分类 | 其他（`other`） | 首页模板（`home-template`） |
| 包来源 | 上传刚才的 ZIP，或填写其 HTTPS 地址 | 同左 |
| 校验码 | `sha256sum` 的 64 位十六进制 | 同左 |

上传成功后地址和校验码不可手改。外链在提交审核时才会下载核对。

## 管理员解析

管理员在「源站 → 软件目录」也可以上传。只解析、不入库：

`POST /api/v1/source/admin/packages/parse`

字段 `file`，可选 `kind=plugin|template`。失败时 HTTP 体里 `code` 为 400，并带 `error.field` 与 `error.rule`，不写库、不推 Release。
