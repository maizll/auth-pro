package handler

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
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
		"docs/charter.md":                                  loadDeveloperDocOr("charter.md", developerCharterFallback()),
		"docs/plugin-package.md":                           loadDeveloperDocOr("plugin-package.md", developerPluginPackageFallback()),
		"docs/template-package.md":                         loadDeveloperDocOr("template-package.md", developerTemplatePackageFallback()),
		"docs/packaging.md":                                loadDeveloperDocOr("packaging.md", developerPackagingFallback()),
		"docs/validation.md":                               loadDeveloperDocOr("validation.md", developerValidationFallback()),
		"docs/versions.md":                                 loadDeveloperDocOr("versions.md", developerVersionsFallback()),
		"docs/review-and-catalog.md":                       loadDeveloperDocOr("review-and-catalog.md", developerReviewCatalogFallback()),
		"plugin-example/plugin.json":                       loadDeveloperDocOr("starter/plugin-example/plugin.json", developerExamplePluginJSON()),
		"plugin-example/README.md":                         loadDeveloperDocOr("starter/plugin-example/README.md", developerExamplePluginReadme()),
		"template-example/template.json":                   loadDeveloperDocOr("starter/template-example/template.json", developerExampleTemplateJSON()),
		"template-example/template.fintech-gold.json":      loadDeveloperDocOr("starter/template-example/template.fintech-gold.json", developerExampleTemplateGoldJSON()),
		"template-example/README.md":                       loadDeveloperDocOr("starter/template-example/README.md", developerExampleTemplateReadme()),
		".cursor/skills/auth-pro-plugin-template/SKILL.md": skill,
	}
}

func loadDeveloperDocOr(rel, fallback string) string {
	if payload, err := readDeveloperDocFile(rel); err == nil && len(bytes.TrimSpace(payload)) > 0 {
		return string(payload)
	}
	if payload, err := readEmbeddedDeveloperDoc(rel); err == nil && len(bytes.TrimSpace(payload)) > 0 {
		return string(payload)
	}
	return fallback
}

func developerSkillMarkdown() string {
	if payload := readDiskDeveloperSkill(); developerSkillIsActionable(payload) {
		return string(payload)
	}
	if payload, err := developerEmbedFS.ReadFile(embeddedDeveloperSkillPath); err == nil && len(bytes.TrimSpace(payload)) > 0 {
		return string(payload)
	}
	return developerSkillFallback()
}

func readDiskDeveloperSkill() []byte {
	dir := findDeveloperDocsDir()
	if dir == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(filepath.Dir(filepath.Dir(dir)), "developer-skills", "auth-pro-plugin-template", "SKILL.md"),
		filepath.Join(dir, "SKILL.md"),
	}
	for _, candidate := range candidates {
		payload, err := os.ReadFile(candidate)
		if err == nil && developerSkillIsActionable(payload) {
			return payload
		}
	}
	return nil
}

func developerSkillIsActionable(payload []byte) bool {
	if len(payload) < 800 {
		return false
	}
	text := string(payload)
	for _, needle := range []string{
		"name: auth-pro-plugin-template",
		"sha256sum",
		"zip -X",
		"stylePreset",
		"cartoon-blue",
		"fintech-gold",
		"primaryColor",
		"backgroundColor",
		"textColor",
		"schemaVersion",
		"template.json",
		"plugin.json",
		"primaryAction",
		"downloadUrl",
		"templateUrl",
	} {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	if strings.Contains(text, "插件自定义分类会出现在应用商店筛选页签") && !strings.Contains(text, "登记（中文标签") {
		return false
	}
	return true
}

func developerStarterReadme() string {
	return `# AuthPro 源站开发者入门包

本包给插件 / 首页模板作者使用，不是管理端按应用下载的 AuthPro 客户端 SDK ZIP。

源站只保存元数据与外部下载地址（HTTPS URL + sha256），不存储 ZIP。

## 目录

| 路径 | 说明 |
| --- | --- |
| ` + "`docs/charter.md`" + ` | 开发者章程。交给 AI 的唯一规范 |
| ` + "`plugin-example/`" + ` | 可过硬校验的 plugin.json |
| ` + "`template-example/`" + ` | 可过硬校验的整站 template.json（含登录动作，不要加 index.html） |
| ` + "`SKILL.md`" + ` | AI 操作清单 |

## 步骤

1. 按章程改清单，在示例目录内用 zip 打包，使清单位于 ZIP 根目录。
2. sha256sum 整个 ZIP，放到自己的 HTTPS 空间。
3. 开发者面板「登记插件」或「登记模板」：手写标识，填地址和校验码，再提交审核。

公开目录：` + "`/software-source/{app_key}/index.json`" + `。
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
    "name": "示例作者",
    "url": "https://example.com",
    "email": "author@example.com"
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
  "description": "AuthPro 源站声明式整站模板，含登录入口与能力卡片。",
  "schemaVersion": 1,
  "author": { "name": "示例作者" },
  "category": "home-template",
  "stylePreset": "cartoon-blue",
  "theme": {
    "primaryColor": "#168fe5",
    "backgroundColor": "#f1faff",
    "textColor": "#15334a"
  },
  "hero": {
    "badge": "授权服务",
    "title": "专业授权服务",
    "highlight": "清晰可查",
    "description": "查看授权状态与有效期。登录由站点打开，模板不保存密码。",
    "primaryAction": { "label": "进入用户中心", "type": "login" },
    "secondaryAction": { "label": "用户登录", "type": "login" }
  },
  "features": [
    { "icon": "ri:shield-check-line", "title": "安全验证", "description": "授权状态经过校验，账户与服务信息清晰可查。" },
    { "icon": "ri:refresh-line", "title": "实时同步", "description": "授权期限和使用状态及时更新。" },
    { "icon": "ri:customer-service-2-line", "title": "用户中心", "description": "从首页打开登录框，进入用户中心。" }
  ],
  "footer": { "text": "安全、稳定的软件授权服务" }
}
`
}

func developerExampleTemplateGoldJSON() string {
	return `{
  "kind": "template",
  "id": "demo-home-gold",
  "templateKey": "demo-home-gold",
  "name": "演示首页 · 金黑",
  "version": "1.0.0",
  "description": "AuthPro 源站声明式整站模板的 fintech-gold 变体，含登录入口与能力卡片。",
  "schemaVersion": 1,
  "author": { "name": "示例作者" },
  "category": "home-template",
  "stylePreset": "fintech-gold",
  "theme": {
    "primaryColor": "#f0b90b",
    "backgroundColor": "#0b0e11",
    "textColor": "#f5f5f5"
  },
  "hero": {
    "badge": "授权服务",
    "title": "专业授权服务",
    "highlight": "清晰可查",
    "description": "查看授权状态与有效期。登录由站点打开，模板不保存密码。",
    "primaryAction": { "label": "进入用户中心", "type": "login" },
    "secondaryAction": { "label": "用户登录", "type": "login" }
  },
  "features": [
    { "icon": "ri:shield-check-line", "title": "安全验证", "description": "授权状态经过校验，账户与服务信息清晰可查。" },
    { "icon": "ri:refresh-line", "title": "实时同步", "description": "授权期限和使用状态及时更新。" },
    { "icon": "ri:customer-service-2-line", "title": "用户中心", "description": "从首页打开登录框，进入用户中心。" }
  ],
  "footer": { "text": "安全、稳定的软件授权服务" }
}
`
}

func developerExampleTemplateReadme() string {
	return `# 整站模板示例

template.json 是 cartoon-blue。template.fintech-gold.json 是金黑变体，登记前改名为 template.json 再打包。一个 ZIP 只放一份清单。

两份都必须包含 kind: template、数字 schemaVersion: 1、hero.title、hero.primaryAction.type = login、stylePreset，以及 theme.primaryColor、theme.backgroundColor、theme.textColor。禁止 scripts，不要放入 index.html 或 login.html。

打包：zip -X -r /tmp/demo-home.zip template.json，然后 sha256sum。
登记模板时分类选首页模板，模板地址填 https ZIP，校验码填 sha256。
`
}

func developerDocsIndexFallback() string {
	return `# AuthPro 源站开发者文档

唯一规范是 charter.md。插件只产出 plugin.json（kind 省略或 plugin）。模板只产出 template.json，必须含 kind: template、数字 schemaVersion: 1、hero.title、hero.primaryAction.type = login。stylePreset 为 cartoon-blue 或 fintech-gold。颜色写 theme.primaryColor、theme.backgroundColor、theme.textColor。禁止 scripts、index.html、login.html。

打包在清单目录内执行 zip -X，再 sha256sum 整个 ZIP。登记表单填写 appId、category、name、id、version、downloadUrl 或 templateUrl、sha256、description。源站不保存 ZIP。
`
}

func developerCharterFallback() string {
	return `# AuthPro 源站开发者章程

本文件是插件与首页模板的规范。源站只登记元数据、HTTPS 外链和 sha256，不保存 ZIP。

插件清单是 plugin.json，kind 省略或为 plugin。模板清单是 template.json，kind 必须为 template。schemaVersion 必须是数字 1。hero.title 必填。登录入口是 hero.primaryAction.type = login，并写 label。启用后宿主登录弹窗跟随 stylePreset（cartoon-blue 或 fintech-gold）与 theme.primaryColor、theme.backgroundColor、theme.textColor。features 写能力卡片，footer.text 写页脚。禁止 scripts、pages、index.html、login.html，也不要自己调用登录接口或保存 token。

id 匹配 ^[a-z0-9][a-z0-9-]{1,58}$。version 匹配 ^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$。清单放在 ZIP 根目录或一层子目录。打包命令：zip -X -r /tmp/package.zip plugin.json（模板换成 template.json），然后 unzip -l 与 sha256sum。

登记表单：应用 appId，分类 category（插件 other，模板 home-template），名称 name，标识 id，版本 version，下载地址 downloadUrl 或模板地址 templateUrl，校验码 sha256，简介 description。已发布版本不可改包地址，请新增版本。
`
}

func developerPluginPackageFallback() string {
	return `# 插件开发指南

plugin.json 的 kind 可以省略，填写时只能是 plugin。必填 id、name、version、description、author。category 用 payment、realname 或 other，不要用 home-template。图标缺省 ri:puzzle-line。

目录只有 plugin.json。打包：zip -X -r /tmp/demo-widget.zip plugin.json，然后 sha256sum。登记时下载地址填 https ZIP，校验码填 64 位十六进制。不要把 index.html 或多页站点当作插件包。
`
}

func developerTemplatePackageFallback() string {
	return `# 首页模板开发指南

template.json 必须包含 kind: template、数字 schemaVersion: 1、hero.title，以及 hero.primaryAction.type = login。stylePreset 为 cartoon-blue 或 fintech-gold。theme 写 primaryColor、backgroundColor、textColor。features 恰好 3 条，footer.text 写页脚。category 省略时自动绑定 home-template。

标准示例见 starter：template.json 是浅色，template.fintech-gold.json 是金黑变体。一个 ZIP 只放一份清单。禁止 scripts、index.html、login.html。打包：zip -X -r /tmp/demo-home.zip template.json，然后 sha256sum。登记时模板地址填 templateUrl。
`
}

func developerPackagingFallback() string {
	return `# 打包与登记

在含清单的目录内执行：

zip -X -r /tmp/package.zip plugin.json
模板改为 template.json。然后 unzip -l 与 sha256sum。

限制：ZIP、不超过 20 MiB、清单在根目录或一层子目录。禁止路径穿越。源站不存储 ZIP。

登记表单标签：应用 appId，分类 category，名称 name，标识 id，版本 version，下载地址 downloadUrl，模板地址 templateUrl，校验码 sha256，简介 description。先保存草稿，再提交审核。
`
}

func developerValidationFallback() string {
	return `# 拒绝与改法

template.json 缺少 kind 时 field=kind、rule=required，补上 "kind": "template"。schemaVersion 必须是数字 1。缺少 hero.title 要补标题和登录动作。scripts 为 forbidden，删除该键。require_manifest 表示把清单放到 ZIP 根目录。zip_layout 表示去掉 .. 和反斜杠后用 zip -X 重打。sha256 必须是整个 ZIP 的 64 位十六进制。下载地址须为 https。标识不合法时手写 id。已发布版本不可改包地址，请新增版本。
`
}

func developerVersionsFallback() string {
	return `# 更新与多版本

已发布版本不可改包地址，请新增版本。复制上一版，只改清单 version，重新 zip -X 并 sha256sum。列表打开版本，新增版本后保存草稿，再提交审核。模板仍须包含 kind: template、schemaVersion: 1、hero.title 和 hero.primaryAction.type = login。
`
}

func developerReviewCatalogFallback() string {
	return `# 审核、目录与上架

状态：draft 草稿，review 待审核，approved 已通过，published 已上架，rejected 已驳回，hidden 已下架。上架写入 /software-source/{app_key}/index.json。插件进 plugins，模板进 homeTemplates。换包走新增版本，不要改已发布地址。广告申请是独立状态，不在插件包里。
`
}

func developerSkillFallback() string {
	return `# AuthPro 插件 / 整站模板

嵌入的 SKILL.md 缺失时仍按章程操作，不要使用旧的三行 stub。

插件 ZIP 只放 plugin.json，kind 省略或为 plugin。模板 ZIP 只放 template.json，必须含 kind: template、数字 schemaVersion: 1、hero.title、hero.primaryAction.type = login。stylePreset 取 cartoon-blue 或 fintech-gold。theme 写 primaryColor、backgroundColor、textColor。features 写 3 条，footer.text 写页脚。禁止 scripts、index.html、login.html。

打包：

zip -X -r /tmp/demo-home.zip template.json
unzip -l /tmp/demo-home.zip
sha256sum /tmp/demo-home.zip

登记表单：应用 appId，分类 category，名称 name，标识 id，版本 version，下载地址 downloadUrl，模板地址 templateUrl，校验码 sha256，简介 description。
`
}
