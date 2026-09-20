---
name: auth-pro-plugin-template
description: Use when creating, packaging, validating, or submitting AuthPro source-station plugins or home templates; when authoring plugin.json or template.json; when a ZIP is rejected for manifest, path traversal, sha256, downloadUrl, templateUrl, category, or app-scoped catalog rules.
---

# AuthPro 源站插件 / 模板

## Overview

AuthPro 源站只登记**元数据 + 外部地址**，不保存源码或 ZIP。生成插件或首页模板时，先写合规清单，再让作者自行托管文件并提交 URL/sha256。

## When to Use

- 用户要做 AuthPro / 源站 / software-source 的插件或首页模板
- 正在写 `plugin.json` 或 `template.json`
- 上传 ZIP 被拒绝（缺清单、字段不合法、scripts、schemaVersion）
- 需要按应用隔离的公开目录 URL

不要用本 Skill 生成管理端「按应用下载的 AuthPro 客户端 SDK ZIP」（那是授权接入包，不是插件包）。

## Hard rules

1. **不要**把源码或 ZIP 设计成上传到源站存储。输出：清单 + 建议的 `downloadUrl`/`templateUrl` + sha256 计算方式。
2. 插件清单文件名必须是 `plugin.json`；首页模板必须是 `template.json`。放在 ZIP 根目录或一层子目录。
3. `id`：`^[a-z0-9][a-z0-9-]{1,58}$`。`version`：`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`。
4. 插件必填：id, name, version, description, author（字符串或对象，name 必填）。
5. 模板必填：id 或 templateKey、name、version、description、author、`schemaVersion: 1`、`hero.title`。**禁止 `scripts`。**
6. 分类：插件用 payment / realname / other（或管理端插件分类）；模板用 home-template。种类与清单必须一致。
7. 每个包绑定一个 `appId`。公开索引：`/software-source/{app_key}/index.json`。
8. ZIP 硬校验失败即拒绝：非 ZIP、>20MiB、路径穿越、符号链接、缺清单。
9. 提交流：draft → review → approved → published。上架需要 64 位 sha256 + 外部地址。

## Quick reference

| 产物 | 清单 | 地址字段 |
| --- | --- | --- |
| 插件 | plugin.json | downloadUrl（必须 https://） |
| 首页模板 | template.json | templateUrl（https:// 或相对路径） |

机器可读 schema：`GET /software-source/package-schema.json`。

## Output recipe

生成插件或模板时，按这个顺序给出：

1. 目录树（含且仅用对的清单文件名）
2. 完整清单 JSON（可直接保存）
3. 打包与 sha256 命令示例
4. 开发者面板要填的字段：appId、分类、downloadUrl/templateUrl、sha256、changelog
5. 提醒：源站不存源码；公开目录按 app_key 隔离

## Common mistakes

| 错误 | 正确 |
| --- | --- |
| 把 ZIP POST 给源站当文件存储 | 只提交 HTTPS URL + sha256 |
| 模板写 plugin.json | 模板必须 template.json |
| schemaVersion 2 或省略 | 必须为 1 |
| 模板带 scripts | 删除 scripts |
| 用未带应用的 /software-source/index.json | /software-source/{app_key}/index.json |
| 一个包打进多个应用 | 每个目录项只属于一个 appId |
