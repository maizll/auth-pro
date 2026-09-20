package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func AdminSourceDevelopers(c *gin.Context) {
	items, err := currentSourceStationStore().ListDevelopers()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取开发者失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, gin.H{
			"id": item.ID, "agentId": item.AgentID, "username": item.Username, "email": item.Email, "displayName": item.DisplayName,
			"enabled": item.Enabled, "createdAt": item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func AdminSourceCancelDeveloper(c *gin.Context) {
	id, err := parseSourceApplicationID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "开发者标识不合法"})
		return
	}
	if err := currentSourceStationStore().FreezeDeveloper(id, c.GetString("username"), sourceNoteFromBody(c)); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已取消该开发者资格，对方无法再登录开发者端或发布内容"})
}

func AdminSourcePlugins(c *gin.Context) {
	appID, err := requestSourceCatalogAppID(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	items, err := currentSourceStationStore().ListPlugins(strings.TrimSpace(c.Query("status")))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取插件目录失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		if matchSourceCatalogAppID(item.AppID, appID) {
			list = append(list, sourcePluginView(item))
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func AdminSourceRegisterPlugin(c *gin.Context) {
	var req sourcePluginDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	plugin, err := adminPluginFromRequest(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	saved, err := currentSourceStationStore().UpsertPlugin(plugin, true)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if req.Shelf {
		if saved.Status != sourceItemApproved && saved.Status != sourceItemHidden && saved.Status != sourceItemPublished {
			saved, err = currentSourceStationStore().SetPluginStatus(saved.ID, sourceItemApproved, c.GetString("username"), "admin register")
			if err != nil {
				writeSourceDeveloperStoreError(c, err)
				return
			}
		}
		if saved.Status != sourceItemPublished {
			saved, err = currentSourceStationStore().SetPluginStatus(saved.ID, sourceItemPublished, c.GetString("username"), "admin register")
			if err != nil {
				writeSourceDeveloperStoreError(c, err)
				return
			}
		}
		persistIndexSnapshot(c.GetString("username"))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已登记外部插件地址（未上传源码）", "data": sourcePluginView(saved)})
}

func AdminSourcePluginApprove(c *gin.Context) {
	adminSetPluginStatus(c, sourceItemApproved, "已通过插件审核")
}

func AdminSourcePluginReject(c *gin.Context) {
	adminSetPluginStatus(c, sourceItemRejected, "已驳回插件")
}

func AdminSourcePluginShelf(c *gin.Context) {
	adminSetPluginStatus(c, sourceItemPublished, "已上架，写入公开 index.json")
}

func AdminSourcePluginUnshelf(c *gin.Context) {
	adminSetPluginStatus(c, sourceItemHidden, "已下架：仅从公开目录隐藏，不会远程卸载消费者已安装的插件")
}

func AdminSourcePluginDeprecate(c *gin.Context) {
	adminSetPluginStatus(c, sourceItemDeprecated, "已标记弃用")
}

func AdminSourceTemplates(c *gin.Context) {
	appID, err := requestSourceCatalogAppID(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	items, err := currentSourceStationStore().ListTemplates(strings.TrimSpace(c.Query("status")))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取模板目录失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		if matchSourceCatalogAppID(item.AppID, appID) {
			list = append(list, sourceTemplateView(item))
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func AdminSourceRegisterTemplate(c *gin.Context) {
	var req sourceTemplateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	item, err := adminTemplateFromRequest(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	saved, err := currentSourceStationStore().UpsertTemplate(item, true)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if req.Shelf {
		if saved.Status != sourceItemApproved && saved.Status != sourceItemHidden && saved.Status != sourceItemPublished {
			saved, err = currentSourceStationStore().SetTemplateStatus(saved.ID, sourceItemApproved, c.GetString("username"), "admin register")
			if err != nil {
				writeSourceDeveloperStoreError(c, err)
				return
			}
		}
		if saved.Status != sourceItemPublished {
			saved, err = currentSourceStationStore().SetTemplateStatus(saved.ID, sourceItemPublished, c.GetString("username"), "admin register")
			if err != nil {
				writeSourceDeveloperStoreError(c, err)
				return
			}
		}
		persistIndexSnapshot(c.GetString("username"))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已登记外部模板地址（未上传源码）", "data": sourceTemplateView(saved)})
}

func AdminSourceTemplateApprove(c *gin.Context) {
	adminSetTemplateStatus(c, sourceItemApproved, "已通过模板审核")
}

func AdminSourceTemplateReject(c *gin.Context) {
	adminSetTemplateStatus(c, sourceItemRejected, "已驳回模板")
}

func AdminSourceTemplateShelf(c *gin.Context) {
	adminSetTemplateStatus(c, sourceItemPublished, "已上架，写入公开 index.json")
}

func AdminSourceTemplateUnshelf(c *gin.Context) {
	adminSetTemplateStatus(c, sourceItemHidden, "已下架：仅从公开目录隐藏，不会远程卸载消费者已安装的模板")
}

func AdminSourceTemplateDeprecate(c *gin.Context) {
	adminSetTemplateStatus(c, sourceItemDeprecated, "已标记弃用")
}

func AdminSourceIndexSnapshot(c *gin.Context) {
	app, err := resolveAdminIndexApp(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	payload, catalog, err := sourceCatalogJSONForApp(app)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成目录失败"})
		return
	}
	snap, _ := currentSourceStationStore().LatestIndexSnapshot()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"live":          json.RawMessage(payload),
		"name":          catalog.Name,
		"appKey":        catalog.AppKey,
		"appId":         catalog.AppID,
		"indexUrl":      catalog.IndexURL,
		"pluginCount":   len(catalog.Plugins),
		"templateCount": len(catalog.HomeTemplates),
		"snapshot":      snap,
	}})
}

func AdminSourceIndexRegenerate(c *gin.Context) {
	app, err := resolveAdminIndexApp(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	persistIndexSnapshot(c.GetString("username"))
	payload, catalog, err := sourceCatalogJSONForApp(app)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已从数据库重新生成公开目录", "data": gin.H{
		"live":          json.RawMessage(payload),
		"name":          catalog.Name,
		"appKey":        catalog.AppKey,
		"appId":         catalog.AppID,
		"indexUrl":      catalog.IndexURL,
		"pluginCount":   len(catalog.Plugins),
		"templateCount": len(catalog.HomeTemplates),
	}})
}

func resolveAdminIndexApp(c *gin.Context) (sourceCatalogApp, error) {
	if id, err := parseSourceCatalogAppIDValue(c.Query("app_id")); err != nil {
		return sourceCatalogApp{}, err
	} else if id > 0 {
		return currentSourceStationStore().GetCatalogAppByID(id)
	}
	appKey := strings.TrimSpace(c.Query("app_key"))
	if appKey == "" {
		appKey = strings.TrimSpace(c.Query("appKey"))
	}
	if appKey != "" {
		return currentSourceStationStore().GetCatalogAppByKey(appKey)
	}
	apps, err := currentSourceStationStore().ListCatalogApps()
	if err != nil {
		return sourceCatalogApp{}, err
	}
	if len(apps) == 1 {
		return apps[0], nil
	}
	if len(apps) == 0 {
		return sourceCatalogApp{}, errSourceAppRequired
	}
	return apps[0], nil
}

func AdminSourceAudit(c *gin.Context) {
	limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	items, err := currentSourceStationStore().ListAudit(limit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取审计失败"})
		return
	}
	if items == nil {
		items = []sourceAuditEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": items, "total": len(items)}})
}

func adminSetPluginStatus(c *gin.Context, status, okMsg string) {
	id := strings.TrimSpace(c.Param("id"))
	saved, err := currentSourceStationStore().SetPluginStatus(id, status, c.GetString("username"), sourceNoteFromBody(c))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if status == sourceItemPublished || status == sourceItemHidden || status == sourceItemDeprecated {
		persistIndexSnapshot(c.GetString("username"))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": okMsg, "data": sourcePluginView(saved)})
}

func adminSetTemplateStatus(c *gin.Context, status, okMsg string) {
	id := strings.TrimSpace(c.Param("id"))
	saved, err := currentSourceStationStore().SetTemplateStatus(id, status, c.GetString("username"), sourceNoteFromBody(c))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if status == sourceItemPublished || status == sourceItemHidden || status == sourceItemDeprecated {
		persistIndexSnapshot(c.GetString("username"))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": okMsg, "data": sourceTemplateView(saved)})
}

func AdminSourcePluginVersions(c *gin.Context) {
	writeSourceVersionList(c, sourceKindPlugin, strings.TrimSpace(c.Param("id")))
}

func AdminSourceRegisterPluginVersion(c *gin.Context) {
	adminWriteVersion(c, sourceKindPlugin)
}

func AdminSourcePluginVersionApprove(c *gin.Context) {
	adminSetReleaseStatus(c, sourceKindPlugin, sourceVersionPublished, "版本已发布并标记为 latest")
}

func AdminSourcePluginVersionReject(c *gin.Context) {
	adminSetReleaseStatus(c, sourceKindPlugin, sourceVersionDraft, "版本已驳回为草稿")
}

func AdminSourcePluginVersionDeprecate(c *gin.Context) {
	adminSetReleaseStatus(c, sourceKindPlugin, sourceVersionDeprecated, "版本已弃用（元数据保留）")
}

func AdminSourcePluginVersionLatest(c *gin.Context) {
	adminSetLatest(c, sourceKindPlugin)
}

func AdminSourceTemplateVersions(c *gin.Context) {
	writeSourceVersionList(c, sourceKindTemplate, strings.TrimSpace(c.Param("id")))
}

func AdminSourceRegisterTemplateVersion(c *gin.Context) {
	adminWriteVersion(c, sourceKindTemplate)
}

func AdminSourceTemplateVersionApprove(c *gin.Context) {
	adminSetReleaseStatus(c, sourceKindTemplate, sourceVersionPublished, "版本已发布并标记为 latest")
}

func AdminSourceTemplateVersionReject(c *gin.Context) {
	adminSetReleaseStatus(c, sourceKindTemplate, sourceVersionDraft, "版本已驳回为草稿")
}

func AdminSourceTemplateVersionDeprecate(c *gin.Context) {
	adminSetReleaseStatus(c, sourceKindTemplate, sourceVersionDeprecated, "版本已弃用（元数据保留）")
}

func AdminSourceTemplateVersionLatest(c *gin.Context) {
	adminSetLatest(c, sourceKindTemplate)
}

func adminWriteVersion(c *gin.Context, kind string) {
	rel, err := bindSourceReleaseDraft(c, kind)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	rel.ItemID = strings.TrimSpace(c.Param("id"))
	saved, err := currentSourceStationStore().UpsertVersion(rel, 0, true)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已登记外部版本地址（未上传源码）", "data": sourceReleaseView(saved)})
}

func adminSetReleaseStatus(c *gin.Context, kind, status, okMsg string) {
	saved, err := currentSourceStationStore().SetVersionStatus(kind, strings.TrimSpace(c.Param("id")), strings.TrimSpace(c.Param("version")), status, c.GetString("username"), sourceNoteFromBody(c))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if status == sourceVersionPublished || status == sourceVersionDeprecated {
		persistIndexSnapshot(c.GetString("username"))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": okMsg, "data": sourceReleaseView(saved)})
}

func adminSetLatest(c *gin.Context, kind string) {
	id := strings.TrimSpace(c.Param("id"))
	version := strings.TrimSpace(c.Param("version"))
	if err := currentSourceStationStore().SetLatestVersion(kind, id, version, c.GetString("username"), sourceNoteFromBody(c)); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	persistIndexSnapshot(c.GetString("username"))
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已将 latest 回滚/指向该已发布版本", "data": gin.H{"id": id, "latestVersion": version}})
}

func adminPluginFromRequest(req sourcePluginDraftRequest) (sourcePlugin, error) {
	pluginID := strings.TrimSpace(req.ID)
	if !pluginIDPattern.MatchString(pluginID) {
		return sourcePlugin{}, errors.New("插件标识不合法")
	}
	if req.DownloadURL != "" {
		if err := validatePluginDownloadURL(req.DownloadURL); err != nil {
			return sourcePlugin{}, err
		}
	}
	if req.SHA256 != "" {
		if err := validateSHA256(req.SHA256); err != nil {
			return sourcePlugin{}, err
		}
	}
	name := truncateText(req.Name, 100)
	if name == "" {
		name = pluginID
	}
	version := truncateText(req.Version, 40)
	if version == "" {
		version = "1.0.0"
	}
	icon := truncateText(req.Icon, 80)
	if icon == "" {
		icon = "ri:puzzle-line"
	}
	category, err := normalizeAssignedCatalogCategory(sourceKindPlugin, req.Category)
	if err != nil {
		return sourcePlugin{}, err
	}
	return sourcePlugin{
		ID:          pluginID,
		AppID:       req.AppID,
		Category:    category,
		Name:        name,
		Description: truncateText(req.Description, 500),
		Icon:        icon,
		Version:     version,
		SHA256:      strings.ToLower(strings.TrimSpace(req.SHA256)),
		DownloadURL: strings.TrimSpace(req.DownloadURL),
		Changelog:   truncateText(req.Changelog, 2000),
		MinVersion:  truncateText(req.MinVersion, 40),
		ForceUpdate: req.ForceUpdate,
		Author: sourceAuthor{
			Name:  truncateText(req.Author.Name, 100),
			URL:   truncateText(req.Author.URL, 300),
			Email: truncateText(req.Author.Email, 200),
		},
		Status: sourceItemDraft,
	}, nil
}

func adminTemplateFromRequest(req sourceTemplateDraftRequest) (sourceTemplate, error) {
	templateKey := strings.TrimSpace(req.TemplateKey)
	if templateKey == "" {
		templateKey = strings.TrimSpace(req.ID)
	}
	if !pluginIDPattern.MatchString(templateKey) {
		return sourceTemplate{}, errors.New("模板标识不合法")
	}
	if req.TemplateURL != "" {
		if err := validateTemplateLocation(req.TemplateURL); err != nil {
			return sourceTemplate{}, err
		}
	}
	if req.SHA256 != "" {
		if err := validateSHA256(req.SHA256); err != nil {
			return sourceTemplate{}, err
		}
	}
	name := truncateText(req.Name, 100)
	if name == "" {
		name = templateKey
	}
	version := truncateText(req.Version, 40)
	if version == "" {
		version = "1.0.0"
	}
	schemaVersion := req.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = homeTemplateSchemaVersion
	}
	if schemaVersion != homeTemplateSchemaVersion {
		return sourceTemplate{}, errors.New("schemaVersion 必须为 1")
	}
	category, err := normalizeAssignedCatalogCategory(sourceKindTemplate, req.Category)
	if err != nil {
		return sourceTemplate{}, err
	}
	return sourceTemplate{
		ID:            templateKey,
		AppID:         req.AppID,
		Category:      category,
		TemplateKey:   templateKey,
		Name:          name,
		Description:   truncateText(req.Description, 500),
		Version:       version,
		SchemaVersion: schemaVersion,
		SHA256:        strings.ToLower(strings.TrimSpace(req.SHA256)),
		TemplateURL:   strings.TrimSpace(req.TemplateURL),
		Changelog:     truncateText(req.Changelog, 2000),
		MinVersion:    truncateText(req.MinVersion, 40),
		ForceUpdate:   req.ForceUpdate,
		Author: sourceAuthor{
			Name:  truncateText(req.Author.Name, 100),
			URL:   truncateText(req.Author.URL, 300),
			Email: truncateText(req.Author.Email, 200),
		},
		Status: sourceItemDraft,
	}, nil
}
