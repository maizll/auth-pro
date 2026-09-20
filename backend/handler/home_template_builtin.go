package handler

import (
	"embed"
	"errors"
	"net/http"
	"path"
	"strings"

	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

//go:embed builtin_templates/default-home.svg
var defaultHomePreviewFS embed.FS

const (
	builtinPreviewRoute       = "/api/software-source/previews/"
	builtinDefaultPreviewFile = "default-home.svg"
)

var builtinAuthor = templateAuthor{
	Name: "auth_pro 官方",
	URL:  "https://github.com/cy70923167/auth_pro",
}

func builtinDefaultPreviewURL() string {
	return builtinPreviewRoute + builtinDefaultPreviewFile
}

func PublicSoftwareSourcePlugins(c *gin.Context) {
	client, err := softwaresource.Default()
	if err != nil {
		// 未配置远程软件源时返回本站空目录，避免应用商店首页模板整页失败。
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"name": "本站软件源", "sourceType": "local", "schemaVersion": homeTemplateSchemaVersion,
			"homeTemplates": []gin.H{}, "plugins": []gin.H{},
		}})
		return
	}
	remoteCatalog, err := client.Catalog(c.Request.Context())
	if err != nil {
		writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"name": "本站软件源", "sourceType": "local", "schemaVersion": homeTemplateSchemaVersion,
			"homeTemplates": []gin.H{}, "plugins": []gin.H{},
		}})
		return
	}
	templates := make([]gin.H, 0, len(remoteCatalog.Templates))
	for _, template := range remoteCatalog.Templates {
		previewURL := ""
		if template.PreviewURL != "" {
			previewURL = "/api/software-source/templates/" + template.ID + "/preview"
		}
		templates = append(templates, gin.H{
			"id": template.TemplateKey, "catalogId": template.ID, "name": template.Name,
			"description": template.Description, "version": template.Version, "previewUrl": previewURL,
			"author":        templateAuthor{Name: template.Author.Name, URL: template.Author.URL, Email: template.Author.Email},
			"schemaVersion": template.SchemaVersion, "sha256": template.SHA256,
			"templateUrl": template.ContentURL, "templatePath": "",
		})
	}
	writeSystemConfig(c, http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"name": "独立软件源目录", "sourceType": "remote", "schemaVersion": homeTemplateSchemaVersion,
		"homeTemplates": templates, "plugins": []gin.H{},
	}})
}

func PublicSoftwareSourcePreview(c *gin.Context) {
	fileName := strings.TrimSpace(c.Param("file"))
	if fileName == builtinDefaultPreviewFile {
		payload, contentType, err := builtinPreviewPayload(fileName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "示例图片不存在"})
			return
		}
		writeSoftwareSourcePreview(c, payload, contentType)
		return
	}
	client, err := softwaresource.Default()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": err.Error()})
		return
	}
	remoteCatalog, err := client.Catalog(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "示例图片读取失败"})
		return
	}
	templateKey := strings.TrimSuffix(fileName, path.Ext(fileName))
	for _, template := range remoteCatalog.Templates {
		if template.TemplateKey != templateKey {
			continue
		}
		payload, contentType, previewErr := client.TemplatePreview(c.Request.Context(), template.ID)
		if previewErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "示例图片读取失败"})
			return
		}
		writeSoftwareSourcePreview(c, payload, contentType)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "示例图片不存在"})
}

func writeSoftwareSourcePreview(c *gin.Context, payload []byte, contentType string) {
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, contentType, payload)
}

func builtinPreviewPayload(fileName string) ([]byte, string, error) {
	if strings.TrimSpace(fileName) != builtinDefaultPreviewFile {
		return nil, "", errors.New("示例图片不存在")
	}
	payload, err := defaultHomePreviewFS.ReadFile(path.Join("builtin_templates", builtinDefaultPreviewFile))
	if err != nil {
		return nil, "", err
	}
	return payload, "image/svg+xml", nil
}
