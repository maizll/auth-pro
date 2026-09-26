package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const softwareSourceAppGoneMessage = "该软件源对应的应用已删除或归档"

var softwareSourceAppKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

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
	payload, ok, err := publicCatalogPayloadForKey(appKey)
	c.Header("Cache-Control", "no-store")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "index_failed", "message": "软件源目录暂时无法生成", "appKey": appKey,
		})
		return true
	}
	if !ok {
		c.JSON(http.StatusGone, gin.H{
			"error": "app_gone", "message": softwareSourceAppGoneMessage, "appKey": appKey,
		})
		return true
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	return true
}

func publicCatalogPayloadForKey(appKey string) ([]byte, bool, error) {
	store := currentSourceStationStore()
	app, err := store.GetCatalogAppByKey(appKey)
	if err == nil && !app.Archived {
		payload, _, catalogErr := sourceCatalogJSONForApp(app)
		if catalogErr != nil {
			return nil, false, catalogErr
		}
		return payload, true, nil
	}
	if err != nil && !errors.Is(err, errSourceAppNotFound) && !errors.Is(err, errSourceAppRequired) {
		return nil, false, err
	}
	targetID, found, aliasErr := store.GetSoftwareSourceAlias(appKey)
	if aliasErr != nil {
		return nil, false, aliasErr
	}
	if found {
		target, targetErr := store.GetCatalogAppByID(targetID)
		if targetErr == nil && !target.Archived {
			payload, _, catalogErr := sourceCatalogJSONForApp(target)
			if catalogErr != nil {
				return nil, false, catalogErr
			}
			return payload, true, nil
		}
		if targetErr != nil && !errors.Is(targetErr, errSourceAppNotFound) {
			return nil, false, targetErr
		}
	}
	return nil, false, nil
}

func validateSoftwareSourceRedirect(oldAppKey string, targetAppID int64, rejectIfLive bool) error {
	oldAppKey = strings.TrimSpace(oldAppKey)
	if !softwareSourceAppKeyPattern.MatchString(oldAppKey) {
		return errors.New("应用标识不正确")
	}
	store := currentSourceStationStore()
	target, err := store.GetCatalogAppByID(targetAppID)
	if err != nil {
		if errors.Is(err, errSourceAppNotFound) {
			return errors.New("目标应用不存在")
		}
		return err
	}
	if target.Archived {
		return errors.New("不能转到已归档的应用")
	}
	if target.AppKey == oldAppKey || target.ID == 0 {
		return errors.New("不能转到同一个应用")
	}
	if rejectIfLive {
		current, currentErr := store.GetCatalogAppByKey(oldAppKey)
		if currentErr == nil && !current.Archived {
			return errors.New("该应用还在使用，请先归档再转走软件源地址")
		}
		if currentErr != nil && !errors.Is(currentErr, errSourceAppNotFound) && !errors.Is(currentErr, errSourceAppRequired) {
			return currentErr
		}
	}
	return nil
}

func saveSoftwareSourceAlias(oldAppKey string, targetAppID int64, rejectIfLive bool) error {
	if err := validateSoftwareSourceRedirect(oldAppKey, targetAppID, rejectIfLive); err != nil {
		return err
	}
	target, err := currentSourceStationStore().GetCatalogAppByID(targetAppID)
	if err != nil {
		return err
	}
	return currentSourceStationStore().UpsertSoftwareSourceAlias(strings.TrimSpace(oldAppKey), target.ID)
}

func rewriteSoftwareSourceURL(rawURL, newAppKey string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return "", errors.New("软件源地址不正确")
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	lowerPath := strings.ToLower(path)
	switch lowerPath {
	case "/software-source/index.json", "/auth-pro/index.json":
		query := parsed.Query()
		if strings.TrimSpace(query.Get("appKey")) != "" {
			query.Set("appKey", newAppKey)
		} else {
			query.Set("app_key", newAppKey)
		}
		parsed.RawQuery = query.Encode()
	default:
		prefix := ""
		switch {
		case strings.HasPrefix(lowerPath, "/software-source/") && strings.HasSuffix(lowerPath, "/index.json"):
			prefix = "/software-source/"
		case strings.HasPrefix(lowerPath, "/auth-pro/") && strings.HasSuffix(lowerPath, "/index.json"):
			prefix = "/auth-pro/"
		default:
			return "", errors.New("这条软件源地址里没有应用标识")
		}
		parsed.Path = prefix + url.PathEscape(newAppKey) + "/index.json"
		parsed.RawPath = ""
	}
	return parsed.String(), nil
}

func softwareSourceGoneMessage(payload []byte) string {
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	if body.Error == "app_gone" || body.Message == softwareSourceAppGoneMessage {
		return softwareSourceAppGoneMessage
	}
	return ""
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
	if plugin.PriceCents <= 0 && !isPrivatePackageRef(plugin.DownloadURL) && !isGitHubPackageRef(plugin.DownloadURL) {
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
	if template.PriceCents <= 0 && !isPrivatePackageRef(template.TemplateURL) && !isGitHubPackageRef(template.TemplateURL) {
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
		"archived": app.Archived,
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

func softwareSourceAliasView(item softwareSourceAlias) gin.H {
	view := gin.H{"oldAppKey": item.OldAppKey, "targetAppId": item.TargetAppID}
	target, err := currentSourceStationStore().GetCatalogAppByID(item.TargetAppID)
	if err == nil {
		view["targetAppKey"] = target.AppKey
		view["targetName"] = target.Name
		view["indexUrl"] = sourceStationPublicIndexPath(item.OldAppKey)
	}
	return view
}

func AdminSoftwareSourceAliases(c *gin.Context) {
	items, err := currentSourceStationStore().ListSoftwareSourceAliases()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取软件源地址映射失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, softwareSourceAliasView(item))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list}})
}

func AdminSoftwareSourceAliasSave(c *gin.Context) {
	var request struct {
		OldAppKey   string `json:"oldAppKey"`
		TargetAppID int64  `json:"targetAppId"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := saveSoftwareSourceAlias(request.OldAppKey, request.TargetAppID, true); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存软件源地址映射", "data": softwareSourceAliasView(softwareSourceAlias{
		OldAppKey: strings.TrimSpace(request.OldAppKey), TargetAppID: request.TargetAppID,
	})})
}
