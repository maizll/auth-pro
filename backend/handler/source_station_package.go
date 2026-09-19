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
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Icon        string          `json:"icon"`
	Author      json.RawMessage `json:"author"`
}

type templatePackageJSON struct {
	ID            string          `json:"id"`
	TemplateKey   string          `json:"templateKey"`
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Description   string          `json:"description"`
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

func readSourcePackageUpload(c *gin.Context) (string, []byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, pluginPackageMaxSize+(1<<20))
	header, err := c.FormFile("file")
	if err != nil || header == nil || header.Size <= 0 {
		return "", nil, errors.New("请上传插件 ZIP 或首页模板包（file 字段）")
	}
	if header.Size > pluginPackageMaxSize {
		return "", nil, errors.New("上传包不能超过 20 MiB")
	}
	file, err := header.Open()
	if err != nil {
		return "", nil, errors.New("读取上传文件失败")
	}
	defer file.Close()
	payload, err := readPluginReader(file, pluginPackageMaxSize)
	if err != nil {
		return "", nil, errors.New("读取上传文件失败：" + err.Error())
	}
	name := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if name == "." || name == "/" {
		name = "package.bin"
	}
	return name, payload, nil
}

func parseSourcePackageBytes(filename string, payload []byte, kindHint string) (sourcePackageManifest, error) {
	if len(payload) == 0 {
		return sourcePackageManifest{}, errors.New("上传包为空")
	}
	sum := sha256.Sum256(payload)
	manifest := sourcePackageManifest{
		Filename: filename,
		Size:     len(payload),
		SHA256:   hex.EncodeToString(sum[:]),
	}
	kindHint = strings.ToLower(strings.TrimSpace(kindHint))
	if isZipPayload(payload) {
		archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
		if err != nil {
			return sourcePackageManifest{}, errors.New("不是有效的 ZIP 压缩包")
		}
		pluginRaw, pluginPath, pluginErr := readZipManifest(archive, "plugin.json")
		templateRaw, templatePath, templateErr := readZipManifest(archive, "template.json")
		switch kindHint {
		case sourceKindPlugin:
			if pluginErr != nil {
				return sourcePackageManifest{}, errors.New("插件包缺少 plugin.json（根目录或一层子目录）")
			}
			return fillPluginManifest(manifest, pluginRaw, pluginPath)
		case sourceKindTemplate:
			if templateErr != nil {
				return sourcePackageManifest{}, errors.New("模板包缺少 template.json（根目录或一层子目录）")
			}
			return fillTemplateManifest(manifest, templateRaw, templatePath)
		default:
			if pluginErr == nil {
				return fillPluginManifest(manifest, pluginRaw, pluginPath)
			}
			if templateErr == nil {
				return fillTemplateManifest(manifest, templateRaw, templatePath)
			}
			return sourcePackageManifest{}, errors.New("压缩包中未找到 plugin.json 或 template.json")
		}
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return sourcePackageManifest{}, errors.New("请上传插件 ZIP、首页模板 ZIP 或模板 JSON")
	}
	switch kindHint {
	case sourceKindPlugin:
		return fillPluginManifest(manifest, trimmed, filename)
	case sourceKindTemplate:
		return fillTemplateManifest(manifest, trimmed, filename)
	default:
		if looksLikeTemplateManifest(trimmed) {
			return fillTemplateManifest(manifest, trimmed, filename)
		}
		if plugin, err := fillPluginManifest(manifest, trimmed, filename); err == nil {
			return plugin, nil
		}
		return fillTemplateManifest(manifest, trimmed, filename)
	}
}

func looksLikeTemplateManifest(raw []byte) bool {
	var probe struct {
		Hero          json.RawMessage `json:"hero"`
		TemplateKey   string          `json:"templateKey"`
		SchemaVersion int             `json:"schemaVersion"`
		ID            string          `json:"id"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return false
	}
	if len(bytes.TrimSpace(probe.Hero)) > 0 || strings.TrimSpace(probe.TemplateKey) != "" {
		return true
	}
	return probe.SchemaVersion == homeTemplateSchemaVersion && strings.TrimSpace(probe.ID) != ""
}

func fillPluginManifest(base sourcePackageManifest, raw []byte, manifestPath string) (sourcePackageManifest, error) {
	var doc pluginPackageJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return sourcePackageManifest{}, errors.New("plugin.json 不是有效 JSON")
	}
	id := strings.ToLower(strings.TrimSpace(doc.ID))
	if !pluginIDPattern.MatchString(id) {
		return sourcePackageManifest{}, errors.New("plugin.json 缺少合法 id（2-59 位小写字母、数字或连字符）")
	}
	name := truncateText(doc.Name, 100)
	if name == "" {
		return sourcePackageManifest{}, errors.New("plugin.json 缺少 name")
	}
	version, err := normalizeSourceVersion(doc.Version)
	if err != nil {
		return sourcePackageManifest{}, errors.New("plugin.json 版本号不合法")
	}
	base.Kind = sourceKindPlugin
	base.ID = id
	base.Name = name
	base.Version = version
	base.Description = truncateText(doc.Description, 500)
	base.Author = decodeSourceAuthor(doc.Author)
	base.Category = normalizePluginCategory(strings.TrimSpace(doc.Category))
	base.Icon = truncateText(doc.Icon, 80)
	base.ManifestPath = manifestPath
	return base, nil
}

func fillTemplateManifest(base sourcePackageManifest, raw []byte, manifestPath string) (sourcePackageManifest, error) {
	var doc templatePackageJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return sourcePackageManifest{}, errors.New("template.json 不是有效 JSON")
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = homeTemplateSchemaVersion
	}
	if doc.SchemaVersion != homeTemplateSchemaVersion {
		return sourcePackageManifest{}, errors.New("schemaVersion 必须为 1")
	}
	if len(doc.Hero) > 0 {
		if err := validateHomeTemplateDocument(raw); err != nil {
			return sourcePackageManifest{}, err
		}
	} else if len(doc.Scripts) > 0 && string(doc.Scripts) != "null" {
		return sourcePackageManifest{}, errors.New("声明式模板不允许 scripts 字段")
	}
	id := strings.ToLower(strings.TrimSpace(sourceFirstNonEmpty(doc.ID, doc.TemplateKey)))
	if !pluginIDPattern.MatchString(id) {
		return sourcePackageManifest{}, errors.New("模板清单缺少合法 id / templateKey")
	}
	name := truncateText(doc.Name, 100)
	if name == "" {
		if title := templateHeroTitle(doc.Hero); title != "" {
			name = truncateText(title, 100)
		}
	}
	if name == "" {
		return sourcePackageManifest{}, errors.New("模板清单缺少 name")
	}
	version, err := normalizeSourceVersion(doc.Version)
	if err != nil {
		return sourcePackageManifest{}, errors.New("模板版本号不合法")
	}
	base.Kind = sourceKindTemplate
	base.ID = id
	base.Name = name
	base.Version = version
	base.Description = truncateText(doc.Description, 500)
	base.Author = decodeSourceAuthor(doc.Author)
	base.SchemaVersion = doc.SchemaVersion
	base.ManifestPath = manifestPath
	return base, nil
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

func templateHeroTitle(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var hero struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(raw, &hero) != nil {
		return ""
	}
	return strings.TrimSpace(hero.Title)
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
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "__macosx/") || strings.HasSuffix(lower, "/.ds_store") || strings.HasSuffix(name, "/") {
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
		return nil, "", errors.New(filename + " 过大")
	}
	reader, err := match.Open()
	if err != nil {
		return nil, "", errors.New("读取 " + filename + " 失败")
	}
	defer reader.Close()
	payload, err := io.ReadAll(io.LimitReader(reader, pluginManifestMaxSize+1))
	if err != nil {
		return nil, "", errors.New("读取 " + filename + " 失败")
	}
	if int64(len(payload)) > pluginManifestMaxSize {
		return nil, "", errors.New(filename + " 过大")
	}
	if !utf8.Valid(payload) {
		return nil, "", errors.New(filename + " 必须是 UTF-8 文本")
	}
	return payload, match.Name, nil
}

func applyPackageFormOverrides(c *gin.Context, manifest sourcePackageManifest) sourcePackageManifest {
	if id := strings.ToLower(strings.TrimSpace(c.PostForm("id"))); pluginIDPattern.MatchString(id) {
		manifest.ID = id
	}
	if key := strings.ToLower(strings.TrimSpace(c.PostForm("templateKey"))); pluginIDPattern.MatchString(key) {
		manifest.ID = key
	}
	if name := truncateText(c.PostForm("name"), 100); name != "" {
		manifest.Name = name
	}
	if version := strings.TrimSpace(c.PostForm("version")); version != "" {
		if normalized, err := normalizeSourceVersion(version); err == nil {
			manifest.Version = normalized
		}
	}
	if desc := strings.TrimSpace(c.PostForm("description")); desc != "" {
		manifest.Description = truncateText(desc, 500)
	}
	if author := strings.TrimSpace(c.PostForm("authorName")); author != "" {
		manifest.Author.Name = truncateText(author, 100)
	}
	if url := strings.TrimSpace(c.PostForm("authorUrl")); url != "" {
		manifest.Author.URL = truncateText(url, 300)
	}
	if email := strings.TrimSpace(c.PostForm("authorEmail")); email != "" {
		manifest.Author.Email = truncateText(email, 200)
	}
	if category := strings.TrimSpace(c.PostForm("category")); category != "" {
		manifest.Category = normalizePluginCategory(category)
	}
	if icon := truncateText(c.PostForm("icon"), 80); icon != "" {
		manifest.Icon = icon
	}
	return manifest
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
