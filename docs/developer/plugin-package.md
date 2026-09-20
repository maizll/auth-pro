# 插件包规范

源站对上传 ZIP **失败即拒绝**：不写库、不推 Release、不留临时文件。开发者面板提交的是元数据；ZIP 由你自行托管。

## 包布局

```text
my-plugin.zip
├── plugin.json      # 根目录，或仅一层子目录如 my-plugin/plugin.json
└── …                # 你的实现文件（源站不保存）
```

- 必须是 ZIP，≤ 20 MiB
- 禁止路径穿越、绝对路径、符号链接、重复/大小写冲突路径、空包、仅 `__MACOSX`
- 清单必须是 UTF-8

## plugin.json 必填

| 字段 | 规则 |
| --- | --- |
| id | 2-59 位小写字母、数字或连字符 `^[a-z0-9][a-z0-9-]{1,58}$` |
| name | ≤100 字 |
| version | `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` |
| description | 必填，≤500 字 |
| author | 字符串，或 `{name,url,email}`；name 必填 |

可选：`category`（内置 `payment` / `realname` / `other`，也可为管理端配置的插件分类；缺省 `other`；不能用首页模板分类）、`icon`。

## 提交到源站的元数据

开发者面板保存草稿时还需要：

| 字段 | 规则 |
| --- | --- |
| appId | 必填，包只属于一个应用 |
| downloadUrl | 若填写必须是 `https://` 外部地址 |
| sha256 | 若填写必须是 64 位十六进制 |
| changelog | 可选 |

不能覆盖内置插件标识。上架还要求同时具备合法 sha256 与 downloadUrl。

## 常见拒绝原因

- 没有 plugin.json
- JSON 无效或非 UTF-8
- id / version 格式不对
- 缺少 description 或 author.name
- category 属于首页模板
- 把源码 POST 到源站（源站不收包内容，只收 URL）
