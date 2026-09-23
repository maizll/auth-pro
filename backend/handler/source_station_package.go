package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type sourcePackageReject struct {
	Field string
	Rule  string
	Msg   string
}

func (e sourcePackageReject) Error() string { return e.Msg }

func rejectSourcePackage(field, rule, msg string) sourcePackageReject {
	return sourcePackageReject{Field: field, Rule: rule, Msg: msg}
}

type sourcePackageManifest struct {
	Kind          string       `json:"kind"`
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	Description   string       `json:"description"`
	Author        sourceAuthor `json:"author"`
	Category      string       `json:"category,omitempty"`
	Icon          string       `json:"icon,omitempty"`
	SchemaVersion int          `json:"schemaVersion,omitempty"`
	SHA256        string       `json:"sha256"`
	Filename      string       `json:"filename"`
	Size          int          `json:"size"`
	ManifestPath  string       `json:"manifestPath,omitempty"`
}

type pluginPackageJSON struct {
	Kind        string          `json:"kind"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Icon        string          `json:"icon"`
	Author      json.RawMessage `json:"author"`
}

type templatePackageJSON struct {
	Kind          string          `json:"kind"`
	ID            string          `json:"id"`
	TemplateKey   string          `json:"templateKey"`
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Description   string          `json:"description"`
	Category      string          `json:"category"`
	SchemaVersion int             `json:"schemaVersion"`
	Author        json.RawMessage `json:"author"`
	Hero          json.RawMessage `json:"hero"`
	Scripts       json.RawMessage `json:"scripts"`
}

func (m sourcePackageManifest) view() gin.H {
	data := gin.H{
		"kind": m.Kind, "id": m.ID, "name": m.Name, "version": m.Version,
		"description": m.Description, "author": m.Author, "sha256": m.SHA256,
		"filename": m.Filename, "size": m.Size, "stored": false,
	}
	if m.Kind == sourceKindTemplate {
		data["schemaVersion"] = m.SchemaVersion
		data["templateKey"] = m.ID
	}
	if m.Category != "" {
		data["category"] = m.Category
	}
	if m.Icon != "" {
		data["icon"] = m.Icon
	}
	if m.ManifestPath != "" {
		data["manifestPath"] = m.ManifestPath
	}
	return data
}

func SourcePackageSchema(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	doc := sourcePackageSchemaDocument()
	if strings.HasPrefix(c.Request.URL.Path, "/software-source/") {
		c.JSON(http.StatusOK, doc)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": doc})
}

func sourcePackageSchemaDocument() gin.H {
	return gin.H{
		"gate": "fail-closed：不合规 ZIP 一律拒绝，不写库、不推 Release、不保留临时文件",
		"upload": gin.H{
			"parse":    "POST /api/v1/source/admin/packages/parse",
			"publish":  "POST /api/v1/source/admin/packages/publish",
			"schema":   "GET /api/v1/source/admin/packages/schema 与 GET /software-source/package-schema.json",
			"file":     "multipart 字段 file，必须是 ZIP，≤ 20 MiB",
			"kind":     "可选 plugin | template；缺省时按包内清单文件名识别。template.json 必须自带 kind=template；plugin.json 若填写 kind 必须为 plugin",
			"category": "可选。与清单 kind 对应：插件分类不可用于模板，模板分类不可用于插件。模板缺省自动绑定 home-template。",
		},
		"zip": gin.H{
			"required":             true,
			"maxBytes":             pluginPackageMaxSize,
			"maxFiles":             packageMaxFiles,
			"maxUncompressedBytes": packageMaxExtractedBytes,
			"manifestLocation":     "根目录或一层子目录",
			"rejected":             []string{"路径穿越", "绝对路径", "符号链接", "重复/大小写冲突路径", "空包", "仅 __MACOSX 元数据"},
			"sourceCodeNotStored":  true,
			"afterValidation":      "自动填表 → 可选 GitHub/Gitee Release 推送 → 草稿/审核流",
		},
		"plugin": gin.H{
			"manifest": "plugin.json",
			"required": []string{"id", "name", "version", "description", "author"},
			"fields": gin.H{
				"kind":        "可选。若填写必须为 plugin；填写 template 会被拒绝",
				"id":          "必填，2-59 位小写字母、数字或连字符（^[a-z0-9][a-z0-9-]{1,58}$）",
				"name":        "必填，≤100 字",
				"version":     "必填，^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$",
				"description": "必填，≤500 字",
				"author":      "必填，字符串或 {name,url,email}；name 必填",
				"category":    "可选。内置 payment | realname | other，也可使用管理端配置的插件分类；缺省 other。禁止使用模板类分类",
				"icon":        "可选，≤80 字",
			},
			"example": map[string]any{
				"kind": "plugin", "id": "demo-plugin", "name": "演示插件", "version": "1.0.0",
				"description": "授权本地插件源测试", "author": map[string]string{"name": "源站"},
				"category": "other",
			},
		},
		"homeTemplate": gin.H{
			"manifest": "template.json",
			"required": []string{"kind", "id 或 templateKey", "name", "version", "description", "schemaVersion", "author", "hero.title"},
			"fields": gin.H{
				"kind":          "必填，必须为 template。据此自动绑定模板类分类，无需运营手工选择",
				"id":            "与 templateKey 至少填一个，规则同插件 id",
				"name":          "必填，≤100 字",
				"version":       "必填，规则同插件 version",
				"description":   "必填，≤500 字",
				"schemaVersion": "必填，必须为 1",
				"author":        "必填，字符串或 {name,url,email}；name 必填",
				"hero.title":    "必填（声明式模板 schema v1）",
				"scripts":       "禁止",
				"category":      "可选，须为模板类分类；缺省自动填充 home-template",
			},
			"example": map[string]any{
				"kind": "template", "id": "clean-home", "name": "清新首页", "version": "1.0.0",
				"description": "简洁的授权服务首页", "schemaVersion": 1,
				"author": map[string]string{"name": "设计组"},
				"hero":   map[string]string{"title": "专业授权服务"},
			},
		},
	}
}

func readSourcePackageUpload(c *gin.Context) (string, []byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, pluginPackageMaxSize+(1<<20))
	header, err := c.FormFile("file")
	if err != nil || header == nil || header.Size <= 0 {
		return "", nil, rejectSourcePackage("file", "required", "请上传插件或首页模板 ZIP（multipart 字段 file）")
	}
	if header.Size > pluginPackageMaxSize {
		return "", nil, rejectSourcePackage("file", "max_size", "上传包不能超过 20 MiB")
	}
	file, err := header.Open()
	if err != nil {
		return "", nil, rejectSourcePackage("file", "read", "读取上传文件失败")
	}
	defer file.Close()
	payload, err := readPluginReader(file, pluginPackageMaxSize)
	if err != nil {
		return "", nil, rejectSourcePackage("file", "read", "读取上传文件失败："+err.Error())
	}
	name := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if name == "." || name == "/" {
		name = "package.zip"
	}
	return name, payload, nil
}

func parseSourcePackageBytes(filename string, payload []byte, kindHint string, categoryHint string) (sourcePackageManifest, error) {
	if len(payload) == 0 {
		return sourcePackageManifest{}, rejectSourcePackage("file", "required", "上传包为空")
	}
	if !isZipPayload(payload) {
		return sourcePackageManifest{}, rejectSourcePackage("file", "require_zip", "必须上传 ZIP 压缩包，且包内须含 plugin.json 或 template.json")
	}
	archive, err := openValidatedPackageZIP(payload)
	if err != nil {
		return sourcePackageManifest{}, rejectSourcePackage("file", "zip_layout", "压缩包布局不合法："+err.Error())
	}
	sum := sha256.Sum256(payload)
	manifest := sourcePackageManifest{
		Filename: filename,
		Size:     len(payload),
		SHA256:   hex.EncodeToString(sum[:]),
	}
	kindHint = strings.ToLower(strings.TrimSpace(kindHint))
	categoryHint = strings.ToLower(strings.TrimSpace(categoryHint))
	if kindHint == "" && categoryHint != "" {
		if kind := sourceCatalogCategoryKind(categoryHint); kind != "" {
			kindHint = kind
		} else {
			return sourcePackageManifest{}, rejectSourcePackage("category", "unknown", "未知分类，请先在目录分类中配置")
		}
	}
	if kindHint != "" && categoryHint != "" {
		if kind := sourceCatalogCategoryKind(categoryHint); kind != "" && kind != kindHint {
			return sourcePackageManifest{}, rejectSourcePackage("category", "kind", "分类与包类型不匹配")
		}
	}
	pluginRaw, pluginPath, pluginErr := readZipManifest(archive, "plugin.json")
	templateRaw, templatePath, templateErr := readZipManifest(archive, "template.json")
	var parsed sourcePackageManifest
	switch kindHint {
	case sourceKindPlugin:
		if pluginErr != nil {
			return sourcePackageManifest{}, rejectSourcePackage("plugin.json", "require_manifest", "该分类的安装包必须在根目录或一层子目录包含 plugin.json")
		}
		parsed, err = fillPluginManifest(manifest, pluginRaw, pluginPath)
	case sourceKindTemplate:
		if templateErr != nil {
			return sourcePackageManifest{}, rejectSourcePackage("template.json", "require_manifest", "首页模板分类的安装包必须在根目录或一层子目录包含 template.json")
		}
		parsed, err = fillTemplateManifest(manifest, templateRaw, templatePath)
	case "":
		if pluginErr == nil {
			parsed, err = fillPluginManifest(manifest, pluginRaw, pluginPath)
		} else if templateErr == nil {
			parsed, err = fillTemplateManifest(manifest, templateRaw, templatePath)
		} else {
			return sourcePackageManifest{}, rejectSourcePackage("plugin.json", "require_manifest", "压缩包必须包含 plugin.json 或 template.json")
		}
	default:
		return sourcePackageManifest{}, rejectSourcePackage("kind", "invalid", "kind 仅支持 plugin 或 template")
	}
	if err != nil {
		return sourcePackageManifest{}, err
	}
	if categoryHint != "" {
		assigned, assignErr := normalizeAssignedCatalogCategory(parsed.Kind, categoryHint)
		if assignErr != nil {
			return sourcePackageManifest{}, rejectSourcePackage("category", "kind", assignErr.Error())
		}
		parsed.Category = assigned
	}
	return parsed, nil
}

func fillPluginManifest(base sourcePackageManifest, raw []byte, manifestPath string) (sourcePackageManifest, error) {
	var doc pluginPackageJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return sourcePackageManifest{}, rejectSourcePackage("plugin.json", "json", "plugin.json 不是有效 JSON")
	}
	id := strings.ToLower(strings.TrimSpace(doc.ID))
	if id == "" {
		return sourcePackageManifest{}, rejectSourcePackage("id", "required", "plugin.json 缺少 id")
	}
	if !pluginIDPattern.MatchString(id) {
		return sourcePackageManifest{}, rejectSourcePackage("id", "format", "plugin.json 字段 id 不合法：须为 2-59 位小写字母、数字或连字符")
	}
	name := strings.TrimSpace(doc.Name)
	if name == "" {
		return sourcePackageManifest{}, rejectSourcePackage("name", "required", "plugin.json 缺少 name")
	}
	name = truncateText(name, 100)
	version := strings.TrimSpace(doc.Version)
	if version == "" {
		return sourcePackageManifest{}, rejectSourcePackage("version", "required", "plugin.json 缺少 version")
	}
	if !sourceVersionPattern.MatchString(version) {
		return sourcePackageManifest{}, rejectSourcePackage("version", "format", "plugin.json 字段 version 不合法")
	}
	description := strings.TrimSpace(doc.Description)
	if description == "" {
		return sourcePackageManifest{}, rejectSourcePackage("description", "required", "plugin.json 缺少 description")
	}
	author, err := requireSourceAuthor(doc.Author, "plugin.json")
	if err != nil {
		return sourcePackageManifest{}, err
	}
	if _, err := normalizePackageManifestKind(doc.Kind, sourceKindPlugin, "plugin.json"); err != nil {
		return sourcePackageManifest{}, err
	}
	base.Kind = sourceKindPlugin
	base.ID = id
	base.Name = name
	base.Version = version
	base.Description = truncateText(description, 500)
	base.Author = author
	if category := strings.TrimSpace(doc.Category); category != "" {
		assigned, assignErr := normalizeAssignedCatalogCategory(sourceKindPlugin, category)
		if assignErr != nil {
			return sourcePackageManifest{}, rejectSourcePackage("category", "format", "plugin.json 字段 category 不合法："+assignErr.Error())
		}
		base.Category = assigned
	} else {
		base.Category = "other"
	}
	base.Icon = truncateText(doc.Icon, 80)
	base.ManifestPath = manifestPath
	return base, nil
}

func fillTemplateManifest(base sourcePackageManifest, raw []byte, manifestPath string) (sourcePackageManifest, error) {
	var doc templatePackageJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return sourcePackageManifest{}, rejectSourcePackage("template.json", "json", "template.json 不是有效 JSON")
	}
	if doc.SchemaVersion == 0 {
		return sourcePackageManifest{}, rejectSourcePackage("schemaVersion", "required", "template.json 缺少 schemaVersion（必须为 1）")
	}
	if doc.SchemaVersion != homeTemplateSchemaVersion {
		return sourcePackageManifest{}, rejectSourcePackage("schemaVersion", "format", "template.json 字段 schemaVersion 必须为 1")
	}
	if err := validateHomeTemplateDocument(raw); err != nil {
		field, rule := "hero.title", "required"
		msg := err.Error()
		switch {
		case strings.Contains(msg, "schemaVersion"):
			field, rule = "schemaVersion", "format"
		case strings.Contains(msg, "scripts"):
			field, rule = "scripts", "forbidden"
		case strings.Contains(msg, "JSON"):
			field, rule = "template.json", "json"
		}
		return sourcePackageManifest{}, rejectSourcePackage(field, rule, "template.json 不合规："+msg)
	}
	id := strings.ToLower(strings.TrimSpace(sourceFirstNonEmpty(doc.ID, doc.TemplateKey)))
	if id == "" {
		return sourcePackageManifest{}, rejectSourcePackage("id", "required", "template.json 缺少 id / templateKey")
	}
	if !pluginIDPattern.MatchString(id) {
		return sourcePackageManifest{}, rejectSourcePackage("id", "format", "template.json 字段 id/templateKey 不合法：须为 2-59 位小写字母、数字或连字符")
	}
	name := strings.TrimSpace(doc.Name)
	if name == "" {
		return sourcePackageManifest{}, rejectSourcePackage("name", "required", "template.json 缺少 name")
	}
	version := strings.TrimSpace(doc.Version)
	if version == "" {
		return sourcePackageManifest{}, rejectSourcePackage("version", "required", "template.json 缺少 version")
	}
	if !sourceVersionPattern.MatchString(version) {
		return sourcePackageManifest{}, rejectSourcePackage("version", "format", "template.json 字段 version 不合法")
	}
	description := strings.TrimSpace(doc.Description)
	if description == "" {
		return sourcePackageManifest{}, rejectSourcePackage("description", "required", "template.json 缺少 description")
	}
	author, err := requireSourceAuthor(doc.Author, "template.json")
	if err != nil {
		return sourcePackageManifest{}, err
	}
	if _, err := normalizePackageManifestKind(doc.Kind, sourceKindTemplate, "template.json"); err != nil {
		return sourcePackageManifest{}, err
	}
	base.Kind = sourceKindTemplate
	base.ID = id
	base.Name = truncateText(name, 100)
	base.Version = version
	base.Description = truncateText(description, 500)
	base.Author = author
	base.SchemaVersion = doc.SchemaVersion
	if category := strings.TrimSpace(doc.Category); category != "" {
		assigned, assignErr := normalizeAssignedCatalogCategory(sourceKindTemplate, category)
		if assignErr != nil {
			return sourcePackageManifest{}, rejectSourcePackage("category", "format", "template.json 字段 category 不合法："+assignErr.Error())
		}
		base.Category = assigned
	} else {
		base.Category = sourceCategoryHomeTemplate
	}
	base.ManifestPath = manifestPath
	return base, nil
}

func normalizePackageManifestKind(raw, expected, manifest string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(raw))
	if expected == sourceKindTemplate {
		if kind == "" {
			return "", rejectSourcePackage("kind", "required", manifest+" 缺少 kind（必须为 template）")
		}
		if kind == sourceKindPlugin {
			return "", rejectSourcePackage("kind", "mismatch", manifest+" 的 kind 不能是 plugin")
		}
		if kind != sourceKindTemplate {
			return "", rejectSourcePackage("kind", "invalid", manifest+" 字段 kind 必须为 template")
		}
		return sourceKindTemplate, nil
	}
	if kind == sourceKindTemplate {
		return "", rejectSourcePackage("kind", "mismatch", manifest+" 的 kind 不能是 template")
	}
	if kind != "" && kind != sourceKindPlugin {
		return "", rejectSourcePackage("kind", "invalid", manifest+" 字段 kind 若填写必须为 plugin")
	}
	return sourceKindPlugin, nil
}

func requireSourceAuthor(raw json.RawMessage, manifest string) (sourceAuthor, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return sourceAuthor{}, rejectSourcePackage("author", "required", manifest+" 缺少 author")
	}
	author := decodeSourceAuthor(raw)
	if strings.TrimSpace(author.Name) == "" {
		return sourceAuthor{}, rejectSourcePackage("author.name", "required", manifest+" 字段 author.name 必填（author 可为字符串或对象）")
	}
	return author, nil
}

func decodeSourceAuthor(raw json.RawMessage) sourceAuthor {
	if len(bytes.TrimSpace(raw)) == 0 {
		return sourceAuthor{}
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return sourceAuthor{Name: truncateText(name, 100)}
	}
	var author sourceAuthor
	if json.Unmarshal(raw, &author) != nil {
		return sourceAuthor{}
	}
	author.Name = truncateText(author.Name, 100)
	author.URL = truncateText(author.URL, 300)
	author.Email = truncateText(author.Email, 200)
	return author
}

func isZipPayload(payload []byte) bool {
	return len(payload) >= 4 && payload[0] == 'P' && payload[1] == 'K'
}

func readZipManifest(archive *zip.Reader, filename string) ([]byte, string, error) {
	want := strings.ToLower(filename)
	var match *zip.File
	matchDepth := 99
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if isPackageMetadataPath(name) || strings.HasSuffix(name, "/") {
			continue
		}
		if strings.ToLower(path.Base(name)) != want {
			continue
		}
		depth := strings.Count(strings.Trim(name, "/"), "/")
		if depth > 1 {
			continue
		}
		if depth < matchDepth {
			match = file
			matchDepth = depth
		}
	}
	if match == nil {
		return nil, "", errors.New("not found")
	}
	if match.UncompressedSize64 > uint64(pluginManifestMaxSize) {
		return nil, "", rejectSourcePackage(filename, "max_size", filename+" 过大")
	}
	reader, err := match.Open()
	if err != nil {
		return nil, "", rejectSourcePackage(filename, "read", "读取 "+filename+" 失败")
	}
	defer reader.Close()
	payload, err := io.ReadAll(io.LimitReader(reader, pluginManifestMaxSize+1))
	if err != nil {
		return nil, "", rejectSourcePackage(filename, "read", "读取 "+filename+" 失败")
	}
	if int64(len(payload)) > pluginManifestMaxSize {
		return nil, "", rejectSourcePackage(filename, "max_size", filename+" 过大")
	}
	if !utf8.Valid(payload) {
		return nil, "", rejectSourcePackage(filename, "encoding", filename+" 必须是 UTF-8 文本")
	}
	return payload, match.Name, nil
}

func formFlag(c *gin.Context, key string) bool {
	value := strings.ToLower(strings.TrimSpace(c.PostForm(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func sourceReleaseAssetName(manifest sourcePackageManifest) string {
	ext := ".zip"
	if !strings.EqualFold(path.Ext(manifest.Filename), ".zip") && path.Ext(manifest.Filename) != "" {
		ext = strings.ToLower(path.Ext(manifest.Filename))
	}
	name := manifest.ID + "-" + manifest.Version + ext
	name = strings.ReplaceAll(name, "/", "-")
	return name
}

func writeSourcePackageReject(c *gin.Context, err error) {
	var reject sourcePackageReject
	if errors.As(err, &reject) {
		c.JSON(http.StatusOK, gin.H{
			"code": 400, "msg": reject.Msg,
			"error": gin.H{"field": reject.Field, "rule": reject.Rule},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
}
