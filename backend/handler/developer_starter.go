package handler

import (
	"archive/zip"
	"bytes"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

const developerStarterZipName = "auth-pro-developer-starter.zip"

func SourceDeveloperStarterZIP(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	payload, err := buildDeveloperStarterZIP()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成开发者入门包失败"})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+developerStarterZipName+`"`)
	c.Data(http.StatusOK, "application/zip", payload)
}

func SourceDeveloperSkillMarkdown(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="SKILL.md"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(developerSkillMarkdown()))
}

func buildDeveloperStarterZIP() ([]byte, error) {
	files := developerStarterFiles()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	now := time.Now()
	for _, name := range names {
		header := &zip.FileHeader{Name: path.Join("auth-pro-developer-starter", name), Method: zip.Deflate}
		header.SetModTime(now)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := entry.Write([]byte(files[name])); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func developerStarterFiles() map[string]string {
	return map[string]string{
		"README.md":                                        developerStarterReadme(),
		"SKILL.md":                                         developerSkillMarkdown(),
		"docs/plugin-package.md":                           developerPluginPackageDoc(),
		"docs/template-package.md":                         developerTemplatePackageDoc(),
		"docs/review-and-catalog.md":                       developerReviewCatalogDoc(),
		"plugin-example/plugin.json":                       developerExamplePluginJSON(),
		"plugin-example/README.md":                         developerExamplePluginReadme(),
		"template-example/template.json":                   developerExampleTemplateJSON(),
		"template-example/README.md":                       developerExampleTemplateReadme(),
		".cursor/skills/auth-pro-plugin-template/SKILL.md": developerSkillMarkdown(),
	}
}

func developerStarterReadme() string {
	return `# AuthPro 源站开发者入门包

本包给**插件 / 首页模板作者**使用，不是管理端按应用下载的 AuthPro 客户端 SDK ZIP。

源站只保存元数据与外部下载地址（HTTPS URL + sha256），**不存储源码或 ZIP 内容**。

## 目录

| 路径 | 说明 |
| --- | --- |
| ` + "`plugin-example/`" + ` | 合规插件包示例（含 plugin.json） |
| ` + "`template-example/`" + ` | 合规首页模板示例（含 template.json） |
| ` + "`docs/`" + ` | 中文规范：清单字段、硬校验拒绝项、审核与应用隔离 |
| ` + "`SKILL.md`" + ` | 给 Cursor / 其他 AI 编码工具安装的 Skill |
| ` + "`.cursor/skills/auth-pro-plugin-template/SKILL.md`" + ` | 放到项目后即可被 Cursor 发现 |

## 建议流程

1. 按示例改清单，打成 ZIP（清单位于根目录或一层子目录）。
2. 把 ZIP 放到你自己的 HTTPS 空间，计算 64 位 sha256。
3. 登录开发者面板，绑定目标应用，填写元数据并提交审核。
4. 公开目录按应用隔离：` + "`/software-source/{app_key}/index.json`" + `。

完整规则见 ` + "`docs/`" + `。机器可读 schema：` + "`GET /software-source/package-schema.json`" + `。
`
}

func developerExamplePluginJSON() string {
	return `{
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
`
}

func developerExamplePluginReadme() string {
	return `# 插件示例

把本目录打成 ZIP 后，plugin.json 必须出现在压缩包根目录或一层子目录。

源站硬校验会拒绝：非 ZIP、超过 20 MiB、路径穿越、缺少必填字段、id/version 格式错误。失败不写库。

提交到源站时请另外提供：

- downloadUrl：HTTPS 外部地址（源站不托管这个 ZIP）
- sha256：整个 ZIP 的 64 位十六进制
- appId：目标应用（目录按应用隔离）
`
}

func developerExampleTemplateJSON() string {
	return `{
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
`
}

func developerExampleTemplateReadme() string {
	return `# 首页模板示例

首页模板与插件一样走源站目录，只是清单文件为 template.json（声明式 schema v1）。

硬性规则：

- schemaVersion 必须为 1
- 必须有 hero.title
- 禁止 scripts 字段
- id 或 templateKey 至少一个，格式同插件 id

提交元数据时填写 templateUrl（HTTPS 或相对路径如 templates/demo-home.json）和 ZIP/文件的 sha256。
`
}

func developerPluginPackageDoc() string {
	return `# 插件包规范

源站对上传 ZIP **失败即拒绝**：不写库、不推 Release、不留临时文件。开发者面板提交的是元数据；ZIP 由你自行托管。

## 包布局

` + "```text\nmy-plugin.zip\n├── plugin.json      # 根目录，或仅一层子目录如 my-plugin/plugin.json\n└── …                # 你的实现文件（源站不保存）\n```" + `

- 必须是 ZIP，≤ 20 MiB
- 禁止路径穿越、绝对路径、符号链接、重复/大小写冲突路径、空包、仅 ` + "`__MACOSX`" + `
- 清单必须是 UTF-8

## plugin.json 必填

| 字段 | 规则 |
| --- | --- |
| id | 2-59 位小写字母、数字或连字符 ` + "`^[a-z0-9][a-z0-9-]{1,58}$`" + ` |
| name | ≤100 字 |
| version | ` + "`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`" + ` |
| description | 必填，≤500 字 |
| author | 字符串，或 ` + "`{name,url,email}`" + `；name 必填 |

可选：` + "`category`" + `（内置 ` + "`payment` / `realname` / `other`" + `，也可为管理端配置的插件分类；缺省 ` + "`other`" + `；不能用首页模板分类）、` + "`icon`" + `。

## 提交到源站的元数据

开发者面板保存草稿时还需要：

| 字段 | 规则 |
| --- | --- |
| appId | 必填，包只属于一个应用 |
| downloadUrl | 若填写必须是 ` + "`https://`" + ` 外部地址 |
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
`
}

func developerTemplatePackageDoc() string {
	return `# 首页模板包规范

首页模板与插件同属源站目录，只是分类为模板、清单为 ` + "`template.json`" + `。公开 index 里出现在 ` + "`homeTemplates`" + `。

## 包布局

` + "```text\nmy-home.zip\n├── template.json\n└── …\n```" + `

ZIP 硬校验与插件相同。分类 ` + "`home-template`" + `（或其它 kind=template 的分类）必须使用 template.json，不能用 plugin.json。

## template.json 必填

| 字段 | 规则 |
| --- | --- |
| id 或 templateKey | 至少一个，格式同插件 id |
| name / version / description / author | 同插件 |
| schemaVersion | **必须为 1** |
| hero.title | 声明式模板必填 |

禁止：` + "`scripts`" + ` 字段（包括空数组）。

可选 category 必须是模板类分类，缺省 ` + "`home-template`" + `。

## 提交元数据

| 字段 | 规则 |
| --- | --- |
| appId | 必填 |
| templateUrl | HTTPS，或相对路径如 ` + "`templates/clean-home.json`" + ` |
| sha256 | 64 位十六进制 |
| schemaVersion | 1 |

相对 templateUrl 相对该应用的 ` + "`/software-source/{app_key}/index.json`" + ` 解析。
`
}

func developerReviewCatalogDoc() string {
	return `# 审核状态与应用隔离

## 目录项状态

` + "`draft` → `review` → `approved` → `published`" + `（上架）。也可 ` + "`rejected`" + ` / ` + "`hidden`" + `（下架） / ` + "`deprecated`" + `。

| 状态 | 含义 |
| --- | --- |
| draft | 草稿，可改元数据 |
| review | 已提交，等待管理员 |
| approved | 审核通过，尚未出现在公开目录 |
| published | 已上架，写入该应用的 index.json |
| hidden | 已下架，公开目录不再列出；不会远程卸载已安装实例 |
| rejected | 已驳回，可改后再提交 |
| deprecated | 已弃用 |

版本另有：` + "`draft` / `pending` / `published` / `deprecated`" + `。已发布版本不可改包地址，请新增版本。

## 应用隔离

- 每个插件/模板必须绑定 ` + "`appId`" + `
- 公开清单：` + "`GET /software-source/{app_key}/index.json`" + `
- 未带应用的 ` + "`/software-source/index.json`" + ` 是空目录，不要当默认源
- 开发者 ` + "`GET /api/v1/source/developer/apps`" + ` 返回每条应用的 ` + "`indexUrl`" + `

## 广告申请

广告对全站客户端投放，不按应用拆目录。开发者提交申请（pending），管理员通过后才会生成真实广告记录；拒绝不会投放。
`
}

func developerSkillMarkdown() string {
	return `---
name: auth-pro-plugin-template
description: Use when creating, packaging, validating, or submitting AuthPro source-station plugins or home templates; when authoring plugin.json or template.json; when a ZIP is rejected for manifest, path traversal, sha256, downloadUrl, templateUrl, category, or app-scoped catalog rules.
---

# AuthPro 源站插件 / 模板

## Overview

AuthPro 源站只登记**元数据 + 外部地址**，不保存源码或 ZIP。生成插件或首页模板时，先写合规清单，再让作者自行托管文件并提交 URL/sha256。

## When to Use

- 用户要做 AuthPro / 源站 / software-source 的插件或首页模板
- 正在写 ` + "`plugin.json`" + ` 或 ` + "`template.json`" + `
- 上传 ZIP 被拒绝（缺清单、字段不合法、scripts、schemaVersion）
- 需要按应用隔离的公开目录 URL

不要用本 Skill 生成管理端「按应用下载的 AuthPro 客户端 SDK ZIP」（那是授权接入包，不是插件包）。

## Hard rules

1. **不要**把源码或 ZIP 设计成上传到源站存储。输出：清单 + 建议的 ` + "`downloadUrl`/`templateUrl`" + ` + sha256 计算方式。
2. 插件清单文件名必须是 ` + "`plugin.json`" + `；首页模板必须是 ` + "`template.json`" + `。放在 ZIP 根目录或一层子目录。
3. ` + "`id`" + `：` + "`^[a-z0-9][a-z0-9-]{1,58}$`" + `。` + "`version`" + `：` + "`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`" + `。
4. 插件必填：id, name, version, description, author（字符串或对象，name 必填）。
5. 模板必填：id 或 templateKey、name、version、description、author、` + "`schemaVersion: 1`" + `、` + "`hero.title`" + `。**禁止 ` + "`scripts`" + `。**
6. 分类：插件用 payment / realname / other（或管理端插件分类）；模板用 home-template。种类与清单必须一致。
7. 每个包绑定一个 ` + "`appId`" + `。公开索引：` + "`/software-source/{app_key}/index.json`" + `。
8. ZIP 硬校验失败即拒绝：非 ZIP、>20MiB、路径穿越、符号链接、缺清单。
9. 提交流：draft → review → approved → published。上架需要 64 位 sha256 + 外部地址。

## Quick reference

| 产物 | 清单 | 地址字段 |
| --- | --- | --- |
| 插件 | plugin.json | downloadUrl（必须 https://） |
| 首页模板 | template.json | templateUrl（https:// 或相对路径） |

机器可读 schema：` + "`GET /software-source/package-schema.json`" + `。

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
`
}
