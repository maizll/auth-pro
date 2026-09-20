# AuthPro 源站开发者文档

本文档面向**插件（Plugin）与首页模板（Home Template）作者**，以及需要在开发者面板提交广告申请的合作方。它不是管理端「按应用下载的 AuthPro 客户端 SDK」说明。

源站（Software Source Station）只登记**元数据 + 外部地址**，不存储 ZIP 或源码。公开目录按应用隔离：

```text
GET /software-source/{app_key}/index.json
```

未带应用标识的 `/software-source/index.json` 返回空目录，不要当作默认软件源。管理员上架后条目自动进入该应用软件源目录。弃用后会从公开软件源目录清除，不再展示；下架同样从目录移除。

## 文档目录

| 章节 | 说明 |
| --- | --- |
| [插件开发指南](./plugin-package.md) | plugin.json 字段、分类、完整示例 |
| [首页模板开发指南](./template-package.md) | template.json、`kind: "template"`、自动绑定首页模板 |
| [打包与上传规范](./packaging.md) | ZIP 布局、硬校验、登记流程 |
| [校验失败说明](./validation.md) | 错误字段 / 规则 → 原因与改法 |
| [更新与多版本](./versions.md) | 版本状态、latest、已发布版本不可改地址 |
| [审核、目录与广告](./review-and-catalog.md) | 提交流程、应用隔离、广告申请、规范变更记录 |

机器可读 schema：`GET /software-source/package-schema.json`。

入门示例：[`starter/plugin-example/`](./starter/plugin-example/)、[`starter/template-example/`](./starter/template-example/)。按示例打的包必须能通过上传硬校验。

本文档只覆盖第三方开发者需要使用的清单、公开目录、上传校验与提交流程；源站内部实现与仅管理员使用的接口不在此展开。
