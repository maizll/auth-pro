# 首页模板开发指南

## 1. 概述

| 项 | 说明 |
| --- | --- |
| 适用对象 | 编写授权服务首页外观的模板作者 |
| 前置条件 | 已入驻开发者；明确目标应用；模板为声明式 schema v1 |
| 与源站关系 | 清单文件必须是 `template.json`。上架后出现在公开目录的 `homeTemplates`，商店页签为「首页模板」 |

**Breaking（2026-09-20）：** `template.json` **必须**包含 `"kind": "template"`。仅有文件名、没有该字段的旧包会被硬校验拒绝。缺省分类自动填充 `home-template`，无需运营手工选分类。

## 2. 目录结构

```text
demo-home.zip
├── template.json
├── assets/
│   └── cover.png
└── …
```

`template.json` 必须在 ZIP 根目录或一层子目录。禁止 `scripts` 字段（包括空数组）。

## 3. 清单字段表（Manifest）

| 字段名 | 类型 | 必填 | 默认 / 自动填充 | 中文说明 | 校验规则 | 示例值 |
| --- | --- | --- | --- | --- | --- | --- |
| kind | string | **是** | 无（不再省略） | 包类型标识，用于自动绑定模板分类 | 必须为 `template`；`plugin` 拒绝 | `"template"` |
| id | string | 与 templateKey 至少一个 | 无 | 模板标识 | 同插件 id | `"demo-home"` |
| templateKey | string | 与 id 至少一个 | 回退到 id | 兼容旧字段 | 同 id | `"demo-home"` |
| name | string | 是 | 无 | 展示名称 | 非空，≤100 字 | `"演示首页"` |
| version | string | 是 | 无 | 版本 | 同插件 version | `"1.0.0"` |
| description | string | 是 | 无 | 简介 | 非空，≤500 字 | `"入门示例首页模板"` |
| schemaVersion | number | 是 | 无 | 声明式模板版本 | **必须为 1** | `1` |
| author | string 或 object | 是 | 无 | 作者 | name 必填 | `{"name":"示例作者"}` |
| category | string | 否 | **自动 `home-template`** | 模板分类 | 必须是 kind=template 的分类；禁止 payment / realname / other | `"home-template"` |
| hero.title | string | 是 | 无 | 首页主标题 | 声明式 schema v1 必填 | `"专业授权服务"` |
| scripts | any | 禁止 | — | 可执行脚本 | 出现即拒绝 | — |

### 合法 / 非法对照

| 字段 | 合法 | 非法 |
| --- | --- | --- |
| kind | `"template"` | 省略、`"plugin"`、`"home-template"` |
| schemaVersion | `1` | `2`、省略、`"1"` 以外的值 |
| category | 省略（自动 home-template）、自定义模板类 extras | `payment`、`realname`、`other`、插件 extras（如标识为 `template` 但 kind=plugin 的分类） |
| scripts | 字段不存在 | `[]`、`[{}]` |

说明：管理端可以存在**插件类**自定义分类，标识恰好叫 `template`、名称「模板」。那是 plugin.json 的 extras，**不能**写进 template.json 的 category。首页模板始终使用模板类分类（默认 `home-template`）。

## 4. kind / 分类绑定

1. 解析到 `template.json` **或** `kind === "template"` → 强制归入模板类分类。
2. category 省略 → 自动填充 `home-template`。
3. plugin.json 不能声明 `kind: "template"`；template.json 不能声明 `kind: "plugin"`。
4. 插件 extras（即使 key=`template`）出现在商店自己的页签；`template.json` 条目只出现在「首页模板」与 `homeTemplates`。

## 5. 完整示例

```json
{
  "kind": "template",
  "id": "demo-home",
  "templateKey": "demo-home",
  "name": "演示首页",
  "version": "1.0.0",
  "description": "AuthPro 源站开发者入门示例首页模板。",
  "schemaVersion": 1,
  "author": {
    "name": "示例作者"
  },
  "category": "home-template",
  "hero": {
    "title": "专业授权服务"
  }
}
```

可复制副本：[`starter/template-example/template.json`](./starter/template-example/template.json)。

提交元数据：

| 字段 | 规则 |
| --- | --- |
| appId | 必填 |
| templateUrl | HTTPS，或相对该应用 index.json 的路径 |
| sha256 | 64 位十六进制 |
| schemaVersion | 1 |

## 6. 开发者提交流程

1. 编写含 `kind: "template"` 的 `template.json`，打 ZIP 并托管。
2. 开发者面板「我的模板」登记草稿。
3. 提交审核 → 管理员通过 / 驳回。
4. 上架后自动进入该应用软件源目录的 `homeTemplates`。
5. 改包请新增版本，见 [更新与多版本](./versions.md)。

## 7. 常见错误与排查

| 字段 / 规则 | 原因 | 改法 |
| --- | --- | --- |
| kind / required | 未写 `kind` | 增加 `"kind": "template"` |
| kind / mismatch | 写成了 plugin | 改为 template |
| schemaVersion / format | 不是 1 | 固定为数字 `1` |
| scripts / forbidden | 含 scripts | 删除该字段 |
| category / kind | 使用了支付/实名/其他或插件 extras | 省略 category 或使用 `home-template` |

## 8. 变更记录

| 日期 | 变更 | 兼容性 |
| --- | --- | --- |
| 2026-09-20 | 上架后自动进入该应用软件源目录（`homeTemplates`） | 旧消费者兼容 |
| 2026-09-20 | **要求** `kind: "template"` | **Breaking**：旧 template.json 缺 kind 会被拒绝 |
| 2026-09-20 | category 省略自动绑定 `home-template` | 兼容；无需手工选分类 |
