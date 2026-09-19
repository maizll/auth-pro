package handler

import (
	"errors"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

const sourceStationPublicPrefix = "/auth-pro"

func RegisterSourceStationRoutes(engine *gin.Engine, api *gin.RouterGroup) {
	public := engine.Group(sourceStationPublicPrefix)
	{
		public.GET("/index.json", SourceStationIndex)
		public.GET("/templates/:file", SourceStationTemplateFile)
		public.GET("/plugins/:file", SourceStationPluginFile)
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
	}
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

func sourceStationFileID(raw string) (string, error) {
	if strings.Contains(raw, "..") || (strings.ContainsAny(raw, `/\`) && path.Base(raw) != raw) {
		return "", errSourceNotFound
	}
	name := path.Base(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "", errSourceNotFound
	}
	id := strings.TrimSuffix(name, path.Ext(name))
	if !pluginIDPattern.MatchString(id) {
		return "", errSourceNotFound
	}
	return id, nil
}

func sourceTemplatePublicPath(item sourceTemplate) string {
	ext := ".json"
	if strings.EqualFold(item.Format, "zip") {
		ext = ".zip"
	}
	return "templates/" + item.TemplateKey + ext
}

func SourceStationIndex(c *gin.Context) {
	base := sourcePublicBase(c) + sourceStationPublicPrefix
	plugins, _ := currentSourceStationStore().ListPlugins()
	templates, _ := currentSourceStationStore().ListTemplates()
	pluginItems := make([]gin.H, 0, len(plugins))
	sort.SliceStable(plugins, func(i, j int) bool { return plugins[i].ID < plugins[j].ID })
	for _, plugin := range plugins {
		pluginItems = append(pluginItems, gin.H{
			"id": plugin.ID, "category": plugin.Category, "name": plugin.Name,
			"description": plugin.Description, "icon": plugin.Icon, "version": plugin.Version,
			"author":      plugin.Author,
			"downloadUrl": base + "/plugins/" + plugin.ID + ".zip",
		})
	}
	homeTemplates := make([]gin.H, 0, len(templates))
	sort.SliceStable(templates, func(i, j int) bool { return templates[i].TemplateKey < templates[j].TemplateKey })
	for _, template := range templates {
		schemaVersion := template.SchemaVersion
		if schemaVersion == 0 {
			schemaVersion = homeTemplateSchemaVersion
		}
		homeTemplates = append(homeTemplates, gin.H{
			"id": template.TemplateKey, "name": template.Name, "description": template.Description,
			"version": template.Version, "schemaVersion": schemaVersion, "sha256": template.SHA256,
			"templateUrl": sourceTemplatePublicPath(template),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"name":          sourceStationSourceName,
		"plugins":       pluginItems,
		"homeTemplates": homeTemplates,
	})
}

func SourceStationTemplateFile(c *gin.Context) {
	id, err := sourceStationFileID(c.Param("file"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	item, content, _, err := currentSourceStationStore().GetTemplate(id)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, errSourceNotFound) {
			status = http.StatusInternalServerError
		}
		c.Status(status)
		return
	}
	contentType := "application/json"
	if item.Format == "zip" {
		contentType = "application/zip"
	}
	c.Header("X-Checksum-SHA256", item.SHA256)
	c.Data(http.StatusOK, contentType, content)
}

func SourceStationPluginFile(c *gin.Context) {
	id, err := sourceStationFileID(c.Param("file"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	item, payload, err := currentSourceStationStore().GetPlugin(id)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, errSourceNotFound) {
			status = http.StatusInternalServerError
		}
		c.Status(status)
		return
	}
	c.Header("X-Checksum-SHA256", item.SHA256)
	c.Data(http.StatusOK, "application/zip", payload)
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
