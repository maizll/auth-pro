# 打包与上传规范

## 1. 概述

| 项 | 说明 |
| --- | --- |
| 适用对象 | 准备上传或登记安装包的开发者、源站管理员 |
| 前置条件 | 已按插件或模板指南写好清单 |
| 与源站关系 | 管理端「上传 ZIP」只做硬校验与元数据入库；默认不落盘保存 ZIP。开发者面板只登记 URL |

失败即拒绝（fail-closed）：不写库、不推 Release、不留临时文件。

## 2. 目录结构

插件：

```text
my-plugin.zip
├── plugin.json
└── …
```

首页模板：

```text
my-home.zip
├── template.json
└── …
```

规则：必须是 ZIP；UTF-8 清单；位于根目录或一层子目录；≤ 20 MiB。

## 3. 清单字段表

打包阶段只校验包内清单，不校验 downloadUrl。提交/上架另需：

| 字段名 | 类型 | 必填 | 默认 / 自动填充 | 中文说明 | 校验规则 | 示例值 |
| --- | --- | --- | --- | --- | --- | --- |
| appId | number | 是 | 无 | 所属应用 | 必须存在 | `1` |
| downloadUrl | string | 插件上架必填 | 无 | 插件包地址 | `https://` | `https://cdn.example.com/a.zip` |
| templateUrl | string | 模板上架必填 | 无 | 模板地址 | HTTPS 或相对路径 | `templates/demo-home.json` |
| sha256 | string | 上架必填 | 上传时可按 ZIP 字节计算 | 完整性校验和 | 64 位十六进制 | `ab…`（64 位） |
| changelog | string | 否 | 空 | 版本说明 | ≤2000 字 | `"首发"` |

## 4. kind / 分类绑定

- 上传可省略 kind，按 `plugin.json` / `template.json` 识别。
- `template.json` 内部仍必须带 `kind: "template"`，并自动绑定 `home-template`。
- 自定义插件分类（extras）写入公开 `categories`，商店按此做二级筛选；index 主体仍拆 `plugins` / `homeTemplates` 以兼容旧消费者。

## 5. 完整示例

计算校验和：

```bash
sha256sum demo-widget.zip
```

最小插件 ZIP：仅含合规 `plugin.json`。最小模板 ZIP：仅含合规 `template.json`（必须有 kind、schemaVersion、hero.title）。

## 6. 开发者提交流程

1. 本地打包并计算 sha256。
2. 将 ZIP 放到自己的 HTTPS 空间（源站不代存）。
3. 开发者面板登记：应用、分类、名称、地址、校验码。
4. 提交审核；管理员通过后上架。
5. 管理员也可「上传 ZIP（硬校验）」解析清单并登记；已上架条目可用「编辑」改元数据且保持 published。

## 7. 常见错误与排查

| 字段 / 规则 | 原因 | 改法 |
| --- | --- | --- |
| file / require_zip | 不是 ZIP | 使用 zip 工具重新打包 |
| file / max_size | 超过 20 MiB | 减小资源 |
| file / zip_layout | 路径穿越、符号链接、空包 | 按第 2 节重建 |
| downloadUrl | 非 HTTPS | 换 HTTPS 托管 |
| sha256 | 不是 64 位十六进制 | 对**整个 ZIP** 重新计算 |

## 8. 变更记录

| 日期 | 变更 | 兼容性 |
| --- | --- | --- |
| 2026-09-20 | 管理员可编辑已上架元数据且不退回审核 | 行为增强 |
| 2026-09-20 | extras 进入公开 categories 并驱动商店页签 | 旧消费者仍读 plugins / homeTemplates |
