package handler

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"auto_pro/middleware"
	"auto_pro/softwaresource"

	"github.com/gin-gonic/gin"
)

func RegisterSourceStationRoutes(api *gin.RouterGroup) {
	catalog := api.Group("/v1/catalog")
	catalog.Use(requireSourceCatalogKey)
	{
		catalog.GET("/sources", SourceCatalogSources)
		catalog.GET("/templates", SourceCatalogTemplates)
		catalog.GET("/templates/:id/content", SourceCatalogTemplateContent)
		catalog.GET("/templates/:id/preview", SourceCatalogTemplatePreview)
		catalog.GET("/plugins/:id/package", SourceCatalogPluginPackage)
		catalog.GET("/index.json", SourceCatalogPluginIndex)
		catalog.GET("", SourceCatalogPluginIndex)
	}

	api.POST("/v1/source/developer/apply", SourceDeveloperApply)
	api.POST("/v1/source/developer/login", SourceDeveloperLogin)

	developer := api.Group("/v1/source/developer")
	developer.Use(middleware.JWTAuth(), middleware.RequireDeveloper())
	{
		developer.GET("/me", SourceDeveloperMe)
		developer.GET("/items", SourceDeveloperItems)
		developer.POST("/plugins", SourceDeveloperPublishPlugin)
		developer.POST("/templates", SourceDeveloperPublishTemplate)
	}

	admin := api.Group("/v1/source/admin")
	admin.Use(middleware.JWTAuth(), middleware.RequireAdmin())
	{
		admin.GET("/applications", AdminSourceDeveloperApplications)
		admin.POST("/applications/:id/approve", AdminSourceDeveloperApprove)
		admin.POST("/applications/:id/reject", AdminSourceDeveloperReject)
		admin.GET("/advertisements", AdminSourceAdvertisements)
		admin.PUT("/advertisements", AdminSourceAdvertisementUpsert)
		admin.DELETE("/advertisements/:id", AdminSourceAdvertisementDelete)
		admin.GET("/catalog-key", AdminSourceCatalogKey)
		admin.PUT("/catalog-key", AdminSourceCatalogKeyUpdate)
	}
}

func requireSourceCatalogKey(c *gin.Context) {
	expected, err := currentSourceStationStore().CatalogKey()
	if err != nil || strings.TrimSpace(expected) == "" {
		c.Next()
		return
	}
	got := strings.TrimSpace(c.GetHeader("X-Software-Source-Key"))
	if !sourceKeyEqual(expected, got) {
		writeSourceCatalogError(c, http.StatusUnauthorized, "UNAUTHORIZED", "目录 API Key 无效")
		c.Abort()
		return
	}
	c.Next()
}

func sourceKeyEqual(left, right string) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func writeSourceCatalogError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"msg": message, "error": code, "requestId": c.GetHeader("X-Request-Id")})
}

func writeSourceCatalogData(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": data})
}

func localSourceStationSource() softwaresource.Source {
	return softwaresource.Source{
		ID:    sourceStationSourceID,
		Name:  sourceStationSourceName,
		Type:  sourceStationSourceType,
		State: "ok",
	}
}

func sourceCatalogRevision() int64 {
	revision, err := currentSourceStationStore().Revision()
	if err != nil || revision < 1 {
		return 1
	}
	return revision
}

func sourcePublicBase(c *gin.Context) string {
	scheme := "http"
	if proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if c.Request.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + host
}

func SourceCatalogSources(c *gin.Context) {
	writeSourceCatalogData(c, gin.H{
		"list":     []softwaresource.Source{localSourceStationSource()},
		"revision": sourceCatalogRevision(),
	})
}

func SourceCatalogTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	items, err := currentSourceStationStore().ListTemplates()
	if err != nil {
		writeSourceCatalogData(c, gin.H{"list": []softwaresource.Template{}, "total": 0, "revision": sourceCatalogRevision()})
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	templates := make([]softwaresource.Template, 0, len(items))
	for _, item := range items {
		templates = append(templates, catalogTemplateView(item))
	}
	total := len(templates)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeSourceCatalogData(c, gin.H{"list": templates[start:end], "total": total, "revision": sourceCatalogRevision()})
}

func catalogTemplateView(item sourceTemplate) softwaresource.Template {
	previewURL := ""
	if item.HasPreview {
		previewURL = "/api/v1/catalog/templates/" + item.ID + "/preview"
	}
	format := item.Format
	if format == "" {
		format = "json"
	}
	return softwaresource.Template{
		ID:            item.ID,
		TemplateKey:   item.TemplateKey,
		Name:          item.Name,
		Description:   item.Description,
		PreviewURL:    previewURL,
		Version:       item.Version,
		Author:        softwaresource.Author{Name: item.Author.Name, URL: item.Author.URL, Email: item.Author.Email},
		Format:        format,
		SchemaVersion: item.SchemaVersion,
		SHA256:        item.SHA256,
		ContentURL:    "/api/v1/catalog/templates/" + item.ID + "/content",
		Source:        localSourceStationSource(),
		Available:     true,
		Published:     true,
		UpdatedAt:     item.UpdatedAt.UTC(),
	}
}

func SourceCatalogTemplateContent(c *gin.Context) {
	item, content, _, err := currentSourceStationStore().GetTemplate(strings.TrimSpace(c.Param("id")))
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, errSourceNotFound) {
			status = http.StatusInternalServerError
		}
		writeSourceCatalogError(c, status, "NOT_FOUND", "模板不在软件源目录中")
		return
	}
	contentType := "application/json"
	if item.Format == "zip" {
		contentType = "application/zip"
	}
	c.Header("X-Checksum-SHA256", item.SHA256)
	c.Data(http.StatusOK, contentType, content)
}

func SourceCatalogTemplatePreview(c *gin.Context) {
	item, _, preview, err := currentSourceStationStore().GetTemplate(strings.TrimSpace(c.Param("id")))
	if err != nil || len(preview) == 0 {
		writeSourceCatalogError(c, http.StatusNotFound, "NOT_FOUND", "模板预览不存在")
		return
	}
	contentType := item.PreviewContentType
	if contentType == "" {
		contentType = http.DetectContentType(preview)
	}
	c.Data(http.StatusOK, contentType, preview)
}

func SourceCatalogPluginPackage(c *gin.Context) {
	item, payload, err := currentSourceStationStore().GetPlugin(strings.TrimSpace(c.Param("id")))
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, errSourceNotFound) {
			status = http.StatusInternalServerError
		}
		writeSourceCatalogError(c, status, "NOT_FOUND", "插件不在软件源目录中")
		return
	}
	c.Header("X-Checksum-SHA256", item.SHA256)
	c.Data(http.StatusOK, "application/zip", payload)
}

func SourceCatalogPluginIndex(c *gin.Context) {
	base := sourcePublicBase(c)
	plugins, _ := currentSourceStationStore().ListPlugins()
	templates, _ := currentSourceStationStore().ListTemplates()
	pluginItems := make([]gin.H, 0, len(plugins))
	sort.SliceStable(plugins, func(i, j int) bool { return plugins[i].ID < plugins[j].ID })
	for _, plugin := range plugins {
		pluginItems = append(pluginItems, gin.H{
			"id": plugin.ID, "category": plugin.Category, "name": plugin.Name,
			"description": plugin.Description, "icon": plugin.Icon, "version": plugin.Version,
			"author":      plugin.Author,
			"downloadUrl": base + "/api/v1/catalog/plugins/" + plugin.ID + "/package",
			"sha256":      plugin.SHA256,
		})
	}
	homeTemplates := make([]gin.H, 0, len(templates))
	sort.SliceStable(templates, func(i, j int) bool { return templates[i].TemplateKey < templates[j].TemplateKey })
	for _, template := range templates {
		homeTemplates = append(homeTemplates, gin.H{
			"id": template.TemplateKey, "catalogId": template.ID, "name": template.Name,
			"description": template.Description, "version": template.Version,
			"schemaVersion": template.SchemaVersion, "sha256": template.SHA256,
			"format":      template.Format,
			"templateUrl": base + "/api/v1/catalog/templates/" + template.ID + "/content",
			"author":      template.Author,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"name":          sourceStationSourceName,
		"schemaVersion": homeTemplateSchemaVersion,
		"plugins":       pluginItems,
		"homeTemplates": homeTemplates,
	})
}

func AdminSourceAdvertisements(c *gin.Context) {
	records, err := currentSourceStationStore().ListAdvertisements("")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取广告失败"})
		return
	}
	if records == nil {
		records = []advertisementRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"records": records}})
}

func AdminSourceAdvertisementUpsert(c *gin.Context) {
	var record advertisementRecord
	if err := c.ShouldBindJSON(&record); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	record.ID = strings.TrimSpace(record.ID)
	record.Position = strings.TrimSpace(record.Position)
	if record.ID == "" || advertisementLocks[record.Position] == nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告标识或广告位不合法"})
		return
	}
	record.Title = truncateText(record.Title, 120)
	record.ImageURL = truncateText(record.ImageURL, 500)
	record.DestinationURL = truncateText(record.DestinationURL, 500)
	record.Description = truncateText(record.Description, 500)
	if err := currentSourceStationStore().UpsertAdvertisement(record); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存广告失败"})
		return
	}
	resetLocalAdvertisementCache()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "广告已保存", "data": record})
}

func AdminSourceAdvertisementDelete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := currentSourceStationStore().DeleteAdvertisement(id); err != nil {
		if errors.Is(err, errSourceNotFound) {
			c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "广告不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "删除广告失败"})
		return
	}
	resetLocalAdvertisementCache()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "广告已删除"})
}

func AdminSourceCatalogKey(c *gin.Context) {
	key, err := currentSourceStationStore().CatalogKey()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取目录 Key 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"catalogKey": key, "required": strings.TrimSpace(key) != ""}})
}

func AdminSourceCatalogKeyUpdate(c *gin.Context) {
	var req struct {
		CatalogKey string `json:"catalogKey"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := currentSourceStationStore().SetCatalogKey(strings.TrimSpace(req.CatalogKey)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存目录 Key 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "目录 Key 已更新"})
}

func resetLocalAdvertisementCache() {
	advertisementCacheL.Lock()
	advertisementCache = map[string]advertisementCacheEntry{}
	advertisementCacheL.Unlock()
}

func localSourceAdvertisements(position string) []advertisementRecord {
	records, err := currentSourceStationStore().ListAdvertisements(position)
	if err != nil || records == nil {
		return []advertisementRecord{}
	}
	return normalizeAdvertisements(records, position, time.Now())
}

// EnsureSourceStationSchema 在进程启动时补齐源站表；数据库未就绪时忽略。
func EnsureSourceStationSchema() {
	_ = currentSourceStationStore().Ensure()
}
