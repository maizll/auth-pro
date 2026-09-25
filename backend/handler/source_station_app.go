package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func sourceStationPublicIndexPath(appKey string) string {
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return "/software-source/{app_key}/index.json"
	}
	return "/software-source/" + url.PathEscape(appKey) + "/index.json"
}

func resolvePublicCatalogAppKey(c *gin.Context) string {
	appKey := strings.TrimSpace(c.Param("appKey"))
	if appKey == "" {
		appKey = strings.TrimSpace(c.Query("app_key"))
	}
	if appKey == "" {
		appKey = strings.TrimSpace(c.Query("appKey"))
	}
	return appKey
}

func sortSourceCatalogApps(items []sourceCatalogApp) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ID == items[j].ID {
			return items[i].AppKey < items[j].AppKey
		}
		return items[i].ID < items[j].ID
	})
}

func parseSourceCatalogAppIDValue(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errSourceAppNotFound
	}
	return id, nil
}

func requestSourceCatalogAppID(c *gin.Context) (int64, error) {
	if id, err := parseSourceCatalogAppIDValue(c.Query("app_id")); err != nil || id > 0 {
		return id, err
	}
	if id, err := parseSourceCatalogAppIDValue(c.Query("appId")); err != nil || id > 0 {
		return id, err
	}
	if id, err := parseSourceCatalogAppIDValue(c.PostForm("appId")); err != nil || id > 0 {
		return id, err
	}
	if id, err := parseSourceCatalogAppIDValue(c.PostForm("app_id")); err != nil || id > 0 {
		return id, err
	}
	return 0, nil
}

func resolvePublicCatalogApp(c *gin.Context) bool {
	appKey := resolvePublicCatalogAppKey(c)
	if appKey == "" {
		return false
	}
	app, err := currentSourceStationStore().GetCatalogAppByKey(appKey)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusNotFound, gin.H{
			"name": sourceStationSourceName, "plugins": []any{}, "homeTemplates": []any{},
			"error": "unknown app_key",
		})
		return true
	}
	payload, _, err := sourceCatalogJSONForApp(app)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"name": sourceStationSourceName, "appKey": app.AppKey, "appId": app.ID,
			"indexUrl": sourceStationPublicIndexPath(app.AppKey),
			"plugins":  []any{}, "homeTemplates": []any{},
		})
		return true
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	return true
}

func writeUnscopedSourceIndex(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"name": sourceStationSourceName, "plugins": []any{}, "homeTemplates": []any{},
		"error": "app_key required; use /software-source/{app_key}/index.json or ?app_key=",
	})
}

func sourceCatalogJSONForApp(app sourceCatalogApp) ([]byte, ginHCatalog, error) {
	plugins, err := currentSourceStationStore().ListPlugins(sourceItemPublished)
	if err != nil {
		plugins = nil
	}
	templates, err := currentSourceStationStore().ListTemplates(sourceItemPublished)
	if err != nil {
		templates = nil
	}
	pluginItems := make([]map[string]any, 0)
	homeTemplates := make([]map[string]any, 0)
	for _, plugin := range plugins {
		if plugin.AppID != app.ID {
			continue
		}
		pluginItems = append(pluginItems, sourcePublicPluginEntry(plugin))
	}
	for _, template := range templates {
		if template.AppID != app.ID {
			continue
		}
		homeTemplates = append(homeTemplates, sourcePublicTemplateEntry(template))
	}
	catalog := ginHCatalog{
		Name: sourceStationSourceName, AppKey: app.AppKey, AppID: app.ID,
		IndexURL: sourceStationPublicIndexPath(app.AppKey),
		Plugins:  pluginItems, HomeTemplates: homeTemplates,
		Categories: resolveSourceCatalogCategories(),
	}
	payload, err := json.Marshal(catalog)
	return payload, catalog, err
}

func sourcePublicPluginEntry(plugin sourcePlugin) map[string]any {
	category := strings.TrimSpace(plugin.Category)
	if category == "" {
		category = "other"
	}
	entry := map[string]any{
		"id": plugin.ID, "category": category, "name": plugin.Name, "description": plugin.Description,
		"icon": plugin.Icon, "version": plugin.Version, "author": plugin.Author,
		"priceCents": plugin.PriceCents, "billing": catalogBillingLabel(plugin.Billing),
		"forceUpdate": plugin.ForceUpdate,
	}
	if plugin.PriceCents <= 0 && !isPrivatePackageRef(plugin.DownloadURL) {
		entry["downloadUrl"] = plugin.DownloadURL
		entry["sha256"] = plugin.SHA256
	}
	if plugin.Changelog != "" {
		entry["changelog"] = plugin.Changelog
	}
	if plugin.MinVersion != "" {
		entry["minVersion"] = plugin.MinVersion
	}
	return entry
}

func sourcePublicTemplateEntry(template sourceTemplate) map[string]any {
	schemaVersion := template.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = homeTemplateSchemaVersion
	}
	category := strings.TrimSpace(template.Category)
	if category == "" {
		category = sourceCategoryHomeTemplate
	}
	entry := map[string]any{
		"id": template.TemplateKey, "category": category, "name": template.Name, "description": template.Description,
		"version": template.Version, "schemaVersion": schemaVersion,
		"priceCents": template.PriceCents, "billing": catalogBillingLabel(template.Billing),
		"forceUpdate": template.ForceUpdate,
	}
	if template.PriceCents <= 0 && !isPrivatePackageRef(template.TemplateURL) {
		entry["sha256"] = template.SHA256
		entry["templateUrl"] = template.TemplateURL
	}
	if template.Changelog != "" {
		entry["changelog"] = template.Changelog
	}
	if template.MinVersion != "" {
		entry["minVersion"] = template.MinVersion
	}
	return entry
}

func sourceCatalogAppView(app sourceCatalogApp) gin.H {
	return gin.H{
		"id": app.ID, "appKey": app.AppKey, "name": app.Name, "enabled": app.Enabled,
		"indexUrl": sourceStationPublicIndexPath(app.AppKey),
	}
}

func AdminSourceCatalogApps(c *gin.Context) {
	items, err := currentSourceStationStore().ListCatalogApps()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取应用列表失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, sourceCatalogAppView(item))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func SourceDeveloperCatalogApps(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	AdminSourceCatalogApps(c)
}

func matchSourceCatalogAppID(itemAppID, filterAppID int64) bool {
	return filterAppID <= 0 || itemAppID == filterAppID
}
