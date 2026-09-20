---
name: auth-pro-plugin-template
description: Use when creating, packaging, validating, or submitting AuthPro source-station plugins or home templates; when authoring plugin.json or template.json; when a ZIP is rejected for manifest, kind, path traversal, sha256, downloadUrl, templateUrl, category, or app-scoped catalog rules.
---

# AuthPro 源站插件 / 模板

## Overview

AuthPro 源站只登记**元数据 + 外部地址**，不保存源码或 ZIP。生成插件或首页模板时，先写合规清单，再让作者自行托管文件并提交 URL/sha256。

规范原文：仓库 `docs/developer/`（插件、模板、打包、校验失败、多版本、审核与广告）。按 starter 打的包必须能通过硬校验。

## When to Use

- 用户要做 AuthPro / 源站 / software-source 的插件或首页模板
- 正在写 `plugin.json` 或 `template.json`
- 上传 ZIP 被拒绝（缺清单、缺 kind、字段不合法、scripts、schemaVersion）
- 需要按应用隔离的公开目录 URL，或自定义分类要出现在应用商店筛选

不要用本 Skill 生成管理端「按应用下载的 AuthPro 客户端 SDK ZIP」（那是授权接入包，不是插件包）。

## Hard rules

1. **不要**把源码或 ZIP 设计成上传到源站存储。输出：清单 + 建议的 `downloadUrl`/`templateUrl` + sha256 计算方式。
2. 插件清单文件名必须是 `plugin.json`；首页模板必须是 `template.json`。放在 ZIP 根目录或一层子目录。
3. `id`：`^[a-z0-9][a-z0-9-]{1,58}$`。`version`：`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`。
4. 插件必填：id, name, version, description, author（字符串或对象，name 必填）。`kind` 可选，若填必须为 `plugin`。
5. 模板必填：`kind: "template"`、id 或 templateKey、name、version、description、author、`schemaVersion: 1`、`hero.title`。**禁止 `scripts`。** category 省略则自动 `home-template`。
6. 分类：插件用 payment / realname / other（或管理端**插件** extras，例如标识 `template`、名称「模板」）；模板用 home-template。种类与清单必须一致。插件 extras 会出现在应用商店二级筛选，但仍在 `plugins` 数组。
7. 每个包绑定一个 `appId`。公开索引：`/software-source/{app_key}/index.json`。上架后自动进入该应用软件源目录。弃用后会从公开软件源目录清除，不再展示。
8. ZIP 硬校验失败即拒绝：非 ZIP、>20MiB、路径穿越、符号链接、缺清单、模板缺 kind。
9. 提交流：draft → review → approved → published。上架需要 64 位 sha256 + 外部地址。

## Breaking (2026-09-20)

- `template.json` **必须**包含 `"kind": "template"`。旧包省略 kind 会被拒绝。
- 插件 `kind` 仍可省略。

## Quick reference

| 产物 | 清单 | kind | 地址字段 | 商店位置 |
| --- | --- | --- | --- | --- |
| 插件 | plugin.json | 可选 `plugin` | downloadUrl（必须 https://） | categories 对应页签；不进 homeTemplates |
| 首页模板 | template.json | **必填 `template`** | templateUrl（https:// 或相对路径） | 首页模板 / homeTemplates |

机器可读 schema：`GET /software-source/package-schema.json`。

## Output recipe

生成插件或模板时，按这个顺序给出：

1. 目录树（含且仅用对的清单文件名）
2. 完整清单 JSON（可直接保存；模板必须含 kind）
3. 打包与 sha256 命令示例
4. 开发者面板要填的字段：appId、分类、downloadUrl/templateUrl、sha256、changelog
5. 提醒：源站不存源码；公开目录按 app_key 隔离；自定义分类会出现在商店筛选

## Common mistakes

| 错误 | 正确 |
| --- | --- |
| 把 ZIP POST 给源站当文件存储 | 只提交 HTTPS URL + sha256 |
| 模板写 plugin.json | 模板必须 template.json |
| 模板省略 kind | `"kind": "template"` |
| schemaVersion 2 或省略 | 必须为 1 |
| 模板带 scripts | 删除 scripts |
| 插件 category=home-template | 插件用插件分类 / extras |
| 用未带应用的 /software-source/index.json | /software-source/{app_key}/index.json |
| 一个包打进多个应用 | 每个目录项只属于一个 appId |
