# 插件开发指南

## 1. 概述

| 项 | 说明 |
| --- | --- |
| 适用对象 | 为 AuthPro 授权系统编写可安装插件的开发者 |
| 前置条件 | 已用代理商账号完成开发者入驻；明确目标应用的 `appId` / `app_key` |
| 与源站关系 | 源站只保存清单元数据与 `downloadUrl`（下载地址）、`sha256`（SHA-256 校验和）。ZIP 必须自行托管在 HTTPS 空间 |

插件在公开 `index.json` 的 `plugins` 数组中出现。分类（Category）决定它在**应用商店 / 插件商城**中的二级筛选页签。

## 2. 目录结构

最小可上传 ZIP：

```text
demo-widget.zip
├── plugin.json          # 必须位于根目录，或仅一层子目录
└── README.md            # 可选，源站不解析
```

一层子目录同样合法：

```text
demo-widget.zip
└── demo-widget/
    └── plugin.json
```

禁止：路径穿越、绝对路径、符号链接、仅 `__MACOSX` 元数据、空包。包体 ≤ 20 MiB。

## 3. 清单字段表（Manifest）

`plugin.json` 字段如下。

| 字段名 | 类型 | 必填 | 默认 / 自动填充 | 中文说明 | 校验规则 | 示例值 |
| --- | --- | --- | --- | --- | --- | --- |
| kind | string | 否 | 缺省视为 `plugin` | 包类型标识 | 若填写必须为 `plugin`；`template` 直接拒绝 | `"plugin"` |
| id | string | 是 | 无 | 插件标识 | `^[a-z0-9][a-z0-9-]{1,58}$` | `"demo-widget"` |
| name | string | 是 | 无 | 展示名称 | 非空，≤100 字 | `"演示插件"` |
| version | string | 是 | 无 | 语义化版本 | `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` | `"1.0.0"` |
| description | string | 是 | 无 | 简介 | 非空，≤500 字 | `"入门示例插件"` |
| author | string 或 object | 是 | 无 | 作者 | 字符串，或 `{name,url,email}` 且 `name` 必填 | `{"name":"示例作者"}` |
| category | string | 否 | `other` | 插件分类 | 必须是已配置的**插件类**分类；禁止 `home-template` 及其他模板类分类 | `"other"` |
| icon | string | 否 | `ri:puzzle-line`（登记时） | Iconify 图标名 | ≤80 字 | `"ri:puzzle-line"` |

### 合法 / 非法对照

| 字段 | 合法 | 非法 |
| --- | --- | --- |
| id | `demo-widget`、`pay-v2` | `Demo`、`a`、`_x` |
| version | `1.0.0`、`1.0.0-rc.1` | 空、含空格 |
| kind | 省略或 `plugin` | `template`、`home-template` |
| category | `payment` / `realname` / `other` / 管理端 extras（如 `template`） | `home-template`、未配置的标识 |

## 4. kind / 分类绑定

- 插件清单文件必须是 `plugin.json`。
- `kind` 若填写只能是 `plugin`。`kind: "template"` 属于首页模板，放在 plugin.json 中会被拒绝。
- **插件不能归入模板类分类**（内置 `home-template` 或管理端 kind=template 的 extras）。
- 管理端可添加**自定义插件分类**（extras）。示例复现：标识 `template`、名称「模板」、清单类型 plugin.json。该分类会写入公开 `index.json` 的 `categories`，应用商店据此生成**二级筛选页签**；条目仍在 `plugins` 数组，不会进入 `homeTemplates`。
- 内置页签：支付、实名认证、其他、首页模板。extras 是额外页签，不会被折叠进「其他」。

## 5. 完整示例

```json
{
  "kind": "plugin",
  "id": "demo-widget",
  "name": "演示插件",
  "version": "1.0.0",
  "description": "AuthPro 源站开发者入门示例插件，仅用于演示清单字段。",
  "author": {
    "name": "示例作者"
  },
  "category": "other",
  "icon": "ri:puzzle-line"
}
```

仓库可复制副本：[`starter/plugin-example/plugin.json`](./starter/plugin-example/plugin.json)。将该目录打成 ZIP 后，硬校验必须通过。

提交到源站时另需：

| 字段 | 规则 |
| --- | --- |
| appId | 必填，条目只属于一个应用 |
| downloadUrl | HTTPS 外部地址 |
| sha256 | 整个 ZIP 的 64 位十六进制 |

## 6. 开发者提交流程

1. 按本规范编写 `plugin.json` 并打包 ZIP，托管到 HTTPS。
2. 在开发者面板「我的插件」**登记**元数据，状态为草稿（draft）。
3. 补齐 downloadUrl 与 sha256 后**提交审核**（review）。
4. 管理员通过（approved）或驳回（rejected）；驳回后可改再提交。
5. 管理员**上架**后写入该应用 `index.json`（published）。更新请走[多版本](./versions.md)。

## 7. 常见错误与排查

| 现象 / 字段 | 原因 | 改法 |
| --- | --- | --- |
| `plugin.json` / require_manifest | 根目录或一层子目录没有 plugin.json | 调整 ZIP 布局 |
| kind / mismatch | plugin.json 写了 `kind: "template"` | 改为 `plugin` 或删除 kind |
| category / kind | 把插件放到 `home-template` | 改用 payment / realname / other 或插件 extras |
| id / format | 标识含大写或过短 | 使用 2–59 位小写字母、数字、连字符 |
| 商店看不到自定义分类 | 公开 index 未带 extras，或软件源未刷新 | 确认 `categories` 含该分类后刷新软件源 |

完整错误表见 [校验失败说明](./validation.md)。

## 8. 变更记录

| 日期 | 变更 | 兼容性 |
| --- | --- | --- |
| 2026-09-20 | 建议 plugin.json 声明 `kind: "plugin"`；填写 `template` 会被拒绝 | 省略 kind 仍视为插件，**非 breaking** |
| 2026-09-20 | 自定义插件分类（含标识恰好为 `template`）写入 index.categories，并作为应用商店筛选页签 | 公开 index 仍拆 plugins / homeTemplates |
