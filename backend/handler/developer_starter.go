package handler

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

const developerStarterZipName = "auth-pro-developer-starter.zip"

func SourceDeveloperStarterZIP(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		writeCurrentSourceDeveloperError(c, err)
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
		writeCurrentSourceDeveloperError(c, err)
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
	skill := developerSkillMarkdown()
	return map[string]string{
		"README.md":                                        developerStarterReadme(),
		"SKILL.md":                                         skill,
		"docs/README.md":                                   loadDeveloperDocOr("README.md", developerDocsIndexFallback()),
		"docs/plugin-package.md":                           loadDeveloperDocOr("plugin-package.md", developerPluginPackageFallback()),
		"docs/template-package.md":                         loadDeveloperDocOr("template-package.md", developerTemplatePackageFallback()),
		"docs/packaging.md":                                loadDeveloperDocOr("packaging.md", developerPackagingFallback()),
		"docs/validation.md":                               loadDeveloperDocOr("validation.md", developerValidationFallback()),
		"docs/versions.md":                                 loadDeveloperDocOr("versions.md", developerVersionsFallback()),
		"docs/review-and-catalog.md":                       loadDeveloperDocOr("review-and-catalog.md", developerReviewCatalogFallback()),
		"plugin-example/plugin.json":                       loadDeveloperDocOr("starter/plugin-example/plugin.json", developerExamplePluginJSON()),
		"plugin-example/README.md":                         loadDeveloperDocOr("starter/plugin-example/README.md", developerExamplePluginReadme()),
		"template-example/template.json":                   loadDeveloperDocOr("starter/template-example/template.json", developerExampleTemplateJSON()),
		"template-example/README.md":                       loadDeveloperDocOr("starter/template-example/README.md", developerExampleTemplateReadme()),
		".cursor/skills/auth-pro-plugin-template/SKILL.md": skill,
	}
}

func loadDeveloperDocOr(rel, fallback string) string {
	payload, err := readDeveloperDocFile(rel)
	if err != nil || len(bytes.TrimSpace(payload)) == 0 {
		return fallback
	}
	return string(payload)
}

func developerSkillMarkdown() string {
	if dir := findDeveloperDocsDir(); dir != "" {
		candidates := []string{
			filepath.Join(filepath.Dir(filepath.Dir(dir)), "developer-skills", "auth-pro-plugin-template", "SKILL.md"),
			filepath.Join(dir, "SKILL.md"),
		}
		for _, candidate := range candidates {
			if payload, err := os.ReadFile(candidate); err == nil && bytes.Contains(payload, []byte("kind")) {
				if bytes.Contains(payload, []byte("name: auth-pro-plugin-template")) {
					return string(payload)
				}
			}
		}
		if payload, err := os.ReadFile(candidates[0]); err == nil && len(payload) > 0 {
			return string(payload)
		}
	}
	return developerSkillFallback()
}

func developerStarterReadme() string {
	return `# AuthPro 源站开发者入门包

本包给**插件 / 首页模板作者**使用，不是管理端按应用下载的 AuthPro 客户端 SDK ZIP。

源站只保存元数据与外部下载地址（HTTPS URL + sha256），**不存储源码或 ZIP 内容**。

## 目录

| 路径 | 说明 |
| --- | --- |
| ` + "`plugin-example/`" + ` | 合规插件包示例（含 plugin.json，可选 kind=plugin） |
| ` + "`template-example/`" + ` | 合规首页模板示例（含 template.json，**必须** kind=template） |
| ` + "`docs/`" + ` | 企业级中文规范：插件、模板、打包、校验失败、多版本、审核与广告 |
| ` + "`SKILL.md`" + ` | 给 Cursor / 其他 AI 编码工具安装的 Skill |

## 建议流程

1. 按示例改清单，打成 ZIP（清单位于根目录或一层子目录）。
2. 把 ZIP 放到你自己的 HTTPS 空间，计算 64 位 sha256。
3. 登录开发者面板，绑定目标应用，填写元数据并提交审核。
4. 公开目录按应用隔离：` + "`/software-source/{app_key}/index.json`" + `。上架后自动进入该应用软件源目录。弃用后会从公开软件源目录清除，不再展示。自定义插件分类会出现在应用商店筛选页签。

完整规则见 ` + "`docs/`" + `。机器可读 schema：` + "`GET /software-source/package-schema.json`" + `。
`
}

func developerExamplePluginJSON() string {
	return `{
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
`
}

func developerExampleTemplateReadme() string {
	return `# 首页模板示例

首页模板清单必须是 template.json，且必须包含 "kind": "template"。

硬性规则：

- kind 必须为 template
- schemaVersion 必须为 1
- 必须有 hero.title
- 禁止 scripts 字段
- id 或 templateKey 至少一个，格式同插件 id

提交元数据时填写 templateUrl 和 sha256。
`
}

func developerDocsIndexFallback() string {
	return "# AuthPro 源站开发者文档\n\n见插件开发指南、首页模板开发指南、打包与上传规范、校验失败说明、更新与多版本、审核与广告。\n"
}

func developerPluginPackageFallback() string {
	return "# 插件开发指南\n\nplugin.json 可选 kind=plugin。自定义插件分类（如标识 template、名称「模板」）会出现在应用商店筛选页签。\n"
}

func developerTemplatePackageFallback() string {
	return "# 首页模板开发指南\n\ntemplate.json 必须包含 kind: template。category 省略自动绑定 home-template。\n"
}

func developerPackagingFallback() string {
	return "# 打包与上传规范\n\nZIP 硬校验失败即拒绝。源站不存储源码。\n"
}

func developerValidationFallback() string {
	return "# 校验失败说明\n\ntemplate.json 缺少 kind 返回 field=kind rule=required。\n"
}

func developerVersionsFallback() string {
	return "# 更新与多版本\n\n已发布版本不可改包地址，请新增版本。\n"
}

func developerReviewCatalogFallback() string {
	return "# 审核、目录与广告申请\n\ndraft → review → approved → published。广告申请独立状态机。\n"
}

func developerSkillFallback() string {
	return `---
name: auth-pro-plugin-template
description: Use when creating AuthPro source-station plugins or home templates.
---

# AuthPro 源站插件 / 模板

template.json 必须包含 kind: template。插件自定义分类会出现在应用商店筛选页签。
`
}
