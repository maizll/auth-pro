package handler

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterSourceStationRoutes(engine *gin.Engine, api *gin.RouterGroup) {
	engine.GET("/software-source/index.json", SourceStationIndex)
	engine.GET("/auth-pro/index.json", SourceStationIndex)
	engine.GET("/software-source/package-schema.json", SourcePackageSchema)

	api.POST("/v1/source/developer/apply", SourceDeveloperApply)
	api.POST("/v1/source/developer/apply/status", SourceDeveloperApplyStatus)
	api.POST("/v1/source/developer/login", SourceDeveloperLogin)

	developer := api.Group("/v1/source/developer")
	developer.Use(middleware.JWTAuth(), middleware.RequireDeveloper())
	{
		developer.GET("/me", SourceDeveloperMe)
		developer.GET("/items", SourceDeveloperItems)
		developer.POST("/plugins", SourceDeveloperUpsertPlugin)
		developer.PUT("/plugins/:id", SourceDeveloperUpsertPlugin)
		developer.POST("/plugins/:id/submit", SourceDeveloperSubmitPlugin)
		developer.GET("/plugins/:id/versions", SourceDeveloperPluginVersions)
		developer.POST("/plugins/:id/versions", SourceDeveloperUpsertPluginVersion)
		developer.POST("/plugins/:id/versions/:version/submit", SourceDeveloperSubmitPluginVersion)
		developer.POST("/templates", SourceDeveloperUpsertTemplate)
		developer.PUT("/templates/:id", SourceDeveloperUpsertTemplate)
		developer.POST("/templates/:id/submit", SourceDeveloperSubmitTemplate)
		developer.GET("/templates/:id/versions", SourceDeveloperTemplateVersions)
		developer.POST("/templates/:id/versions", SourceDeveloperUpsertTemplateVersion)
		developer.POST("/templates/:id/versions/:version/submit", SourceDeveloperSubmitTemplateVersion)
	}

	admin := api.Group("/v1/source/admin")
	admin.Use(middleware.JWTAuth(), middleware.RequireAdmin())
	{
		admin.GET("/applications", AdminSourceDeveloperApplications)
		admin.POST("/applications/:id/approve", AdminSourceDeveloperApprove)
		admin.POST("/applications/:id/reject", AdminSourceDeveloperReject)
		admin.POST("/applications/:id/freeze", AdminSourceDeveloperFreeze)
		admin.GET("/developers", AdminSourceDevelopers)
		admin.POST("/developers/:id/freeze", AdminSourceFreezeDeveloper)

		admin.GET("/plugins", AdminSourcePlugins)
		admin.PUT("/plugins", AdminSourceRegisterPlugin)
		admin.POST("/plugins/:id/approve", AdminSourcePluginApprove)
		admin.POST("/plugins/:id/reject", AdminSourcePluginReject)
		admin.POST("/plugins/:id/shelf", AdminSourcePluginShelf)
		admin.POST("/plugins/:id/unshelf", AdminSourcePluginUnshelf)
		admin.POST("/plugins/:id/deprecate", AdminSourcePluginDeprecate)
		admin.GET("/plugins/:id/versions", AdminSourcePluginVersions)
		admin.POST("/plugins/:id/versions", AdminSourceRegisterPluginVersion)
		admin.POST("/plugins/:id/versions/:version/approve", AdminSourcePluginVersionApprove)
		admin.POST("/plugins/:id/versions/:version/reject", AdminSourcePluginVersionReject)
		admin.POST("/plugins/:id/versions/:version/deprecate", AdminSourcePluginVersionDeprecate)
		admin.POST("/plugins/:id/versions/:version/latest", AdminSourcePluginVersionLatest)

		admin.GET("/templates", AdminSourceTemplates)
		admin.PUT("/templates", AdminSourceRegisterTemplate)
		admin.POST("/templates/:id/approve", AdminSourceTemplateApprove)
		admin.POST("/templates/:id/reject", AdminSourceTemplateReject)
		admin.POST("/templates/:id/shelf", AdminSourceTemplateShelf)
		admin.POST("/templates/:id/unshelf", AdminSourceTemplateUnshelf)
		admin.POST("/templates/:id/deprecate", AdminSourceTemplateDeprecate)
		admin.GET("/templates/:id/versions", AdminSourceTemplateVersions)
		admin.POST("/templates/:id/versions", AdminSourceRegisterTemplateVersion)
		admin.POST("/templates/:id/versions/:version/approve", AdminSourceTemplateVersionApprove)
		admin.POST("/templates/:id/versions/:version/reject", AdminSourceTemplateVersionReject)
		admin.POST("/templates/:id/versions/:version/deprecate", AdminSourceTemplateVersionDeprecate)
		admin.POST("/templates/:id/versions/:version/latest", AdminSourceTemplateVersionLatest)

		admin.GET("/index", AdminSourceIndexSnapshot)
		admin.POST("/index/regenerate", AdminSourceIndexRegenerate)
		admin.GET("/audit", AdminSourceAudit)

		admin.GET("/settings/release", AdminSourceReleaseSettings)
		admin.PUT("/settings/release", AdminSourceReleaseSettingsSave)
		admin.POST("/settings/release/test", AdminSourceReleaseSettingsTest)
		admin.GET("/packages/schema", SourcePackageSchema)
		admin.POST("/packages/parse", AdminSourcePackageParse)
		admin.POST("/packages/publish", AdminSourcePackagePublish)

		admin.GET("/advertisements", AdminSourceAdvertisements)
		admin.PUT("/advertisements", AdminSourceAdvertisementUpsert)
		admin.DELETE("/advertisements/:id", AdminSourceAdvertisementDelete)
	}

	adminAlias := engine.Group("/api/admin/source")
	adminAlias.Use(middleware.JWTAuth(), middleware.RequireAdmin())
	{
		adminAlias.GET("/plugins/:id/versions", AdminSourcePluginVersions)
		adminAlias.GET("/templates/:id/versions", AdminSourceTemplateVersions)
	}
}

func SourceStationIndex(c *gin.Context) {
	payload, catalog, err := sourceCatalogJSON()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"name": sourceStationSourceName, "plugins": []any{}, "homeTemplates": []any{}})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	_ = catalog
}

func persistIndexSnapshot(actor string) {
	payload, _, err := sourceCatalogJSON()
	if err != nil {
		return
	}
	_ = currentSourceStationStore().SaveIndexSnapshot(string(payload), actor)
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
	if record.ImageURL != "" {
		if err := validateExternalHTTPS(record.ImageURL); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告图片必须是 https:// 外部地址"})
			return
		}
	}
	if record.DestinationURL != "" {
		if err := validateExternalHTTPS(record.DestinationURL); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告跳转必须是 https:// 外部地址"})
			return
		}
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

func EnsureSourceStationSchema() {
	_ = currentSourceStationStore().Ensure()
}

var relativeTemplateURLPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

func validateExternalHTTPS(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("必须是 https:// 外部地址，源站不保存插件或模板源码")
	}
	if strings.Contains(parsed.Path, "..") {
		return errors.New("地址不合法")
	}
	return nil
}

func validatePluginDownloadURL(raw string) error {
	return validateExternalHTTPS(raw)
}

func validateTemplateLocation(raw string) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "https://") {
		return validateExternalHTTPS(raw)
	}
	if strings.Contains(raw, "..") || strings.HasPrefix(raw, "/") || !relativeTemplateURLPattern.MatchString(raw) {
		return errors.New("templateUrl 须为 https:// 或相对路径（如 templates/clean-home.json）")
	}
	return nil
}

func validateSHA256(raw string) error {
	if !sha256HexPattern.MatchString(strings.TrimSpace(raw)) {
		return errors.New("sha256 必须是 64 位十六进制")
	}
	return nil
}

func sourceNoteFromBody(c *gin.Context) string {
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	return strings.TrimSpace(req.Note)
}
