package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const catalogUnassignedQuery = "unassigned"

type catalogRebindItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type catalogRebindRequest struct {
	AppID int64               `json:"appId"`
	Items []catalogRebindItem `json:"items"`
}

type appCatalogMigrateRequiredError struct {
	Count int
}

func (err appCatalogMigrateRequiredError) Error() string {
	return fmt.Sprintf("该应用下还有 %d 条软件目录条目，请选择要迁移到的其他应用后再归档，或直接归档并保留绑定", err.Count)
}

func requestAppArchiveInPlace(c *gin.Context) bool {
	switch strings.TrimSpace(c.Query("archive")) {
	case "1", "true":
		return true
	default:
		return false
	}
}

func requestSourceCatalogUnassigned(c *gin.Context) bool {
	switch strings.TrimSpace(c.Query(catalogUnassignedQuery)) {
	case "1", "true":
		return true
	default:
		return false
	}
}

func liveCatalogAppIDs() (map[int64]struct{}, error) {
	apps, err := currentSourceStationStore().ListCatalogApps()
	if err != nil {
		return nil, err
	}
	live := make(map[int64]struct{}, len(apps))
	for _, app := range apps {
		live[app.ID] = struct{}{}
	}
	return live, nil
}

func catalogItemMatchesApp(itemAppID, filterAppID int64, unassigned bool, live map[int64]struct{}) bool {
	if unassigned {
		if itemAppID <= 0 {
			return true
		}
		_, ok := live[itemAppID]
		return !ok
	}
	return filterAppID <= 0 || itemAppID == filterAppID
}

func catalogItemsOnApp(appID int64) (plugins []sourcePlugin, templates []sourceTemplate, err error) {
	allPlugins, err := currentSourceStationStore().ListPlugins("")
	if err != nil {
		return nil, nil, err
	}
	allTemplates, err := currentSourceStationStore().ListTemplates("")
	if err != nil {
		return nil, nil, err
	}
	for _, item := range allPlugins {
		if item.AppID == appID {
			plugins = append(plugins, item)
		}
	}
	for _, item := range allTemplates {
		if item.AppID == appID {
			templates = append(templates, item)
		}
	}
	return plugins, templates, nil
}

func AdminCatalogAppUsage(c *gin.Context) {
	appID, err := requestSourceCatalogAppID(c)
	if err != nil || appID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择应用"})
		return
	}
	plugins, templates, err := catalogItemsOnApp(appID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取软件目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"count":         len(plugins) + len(templates),
		"pluginCount":   len(plugins),
		"templateCount": len(templates),
	}})
}

func AdminRebindCatalogItems(c *gin.Context) {
	var req catalogRebindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	count, err := rebindCatalogItems(req, 0, "admin", c.GetString("username"))
	if err != nil {
		writeCatalogRebindError(c, err)
		return
	}
	persistIndexSnapshot(c.GetString("username"))
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已切换绑定应用", "data": gin.H{"count": count}})
}

func SourceDeveloperRebindCatalogItems(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	var req catalogRebindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := developerMayBindCatalogApp(req.AppID); err != nil {
		writeCatalogRebindError(c, err)
		return
	}
	count, err := rebindCatalogItems(req, developer.ID, "developer", developer.Username)
	if err != nil {
		writeCatalogRebindError(c, err)
		return
	}
	persistIndexSnapshot(developer.Username)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已切换绑定应用", "data": gin.H{"count": count}})
}

func developerMayBindCatalogApp(appID int64) error {
	if appID <= 0 {
		return errSourceAppRequired
	}
	if _, err := currentSourceStationStore().GetCatalogAppByID(appID); err != nil {
		if errors.Is(err, errSourceAppNotFound) {
			return errors.New("没有该应用的权限")
		}
		return err
	}
	return nil
}

func relocateCatalogBeforeAppDelete(appID, migrateAppID int64, archiveInPlace bool, actor string) error {
	plugins, templates, err := catalogItemsOnApp(appID)
	if err != nil {
		return err
	}
	if len(plugins)+len(templates) == 0 {
		return nil
	}
	if migrateAppID <= 0 {
		if archiveInPlace {
			return nil
		}
		return appCatalogMigrateRequiredError{Count: len(plugins) + len(templates)}
	}
	if migrateAppID == appID {
		return errors.New("不能迁移到正在归档的应用")
	}
	items := make([]catalogRebindItem, 0, len(plugins)+len(templates))
	for _, item := range plugins {
		items = append(items, catalogRebindItem{Kind: sourceKindPlugin, ID: item.ID})
	}
	for _, item := range templates {
		items = append(items, catalogRebindItem{Kind: sourceKindTemplate, ID: item.ID})
	}
	_, err = rebindCatalogItems(catalogRebindRequest{AppID: migrateAppID, Items: items}, 0, "admin", actor)
	return err
}

func rebindCatalogItems(req catalogRebindRequest, ownerDeveloperID int64, actorType, actor string) (int, error) {
	if req.AppID <= 0 {
		return 0, errSourceAppRequired
	}
	target, err := currentSourceStationStore().GetCatalogAppByID(req.AppID)
	if err != nil {
		return 0, err
	}
	if target.Archived {
		return 0, errors.New("不能切换到已归档的应用")
	}
	if len(req.Items) == 0 {
		return 0, errors.New("请选择要切换的条目")
	}
	plugins, err := currentSourceStationStore().ListPlugins("")
	if err != nil {
		return 0, err
	}
	templates, err := currentSourceStationStore().ListTemplates("")
	if err != nil {
		return 0, err
	}
	pluginByID := map[string]sourcePlugin{}
	for _, item := range plugins {
		pluginByID[item.ID] = item
	}
	templateByID := map[string]sourceTemplate{}
	for _, item := range templates {
		templateByID[item.ID] = item
	}
	type planned struct {
		kind     string
		id       string
		publicID string
		fromApp  int64
	}
	plans := make([]planned, 0, len(req.Items))
	seen := map[string]struct{}{}
	for _, raw := range req.Items {
		kind := strings.TrimSpace(raw.Kind)
		id := strings.TrimSpace(raw.ID)
		if id == "" || (kind != sourceKindPlugin && kind != sourceKindTemplate) {
			return 0, errors.New("条目类型不正确")
		}
		key := kind + "\x00" + id
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		switch kind {
		case sourceKindPlugin:
			item, ok := pluginByID[id]
			if !ok {
				return 0, errSourceNotFound
			}
			if ownerDeveloperID > 0 && item.DeveloperID != ownerDeveloperID {
				return 0, errors.New("只能切换自己的条目")
			}
			plans = append(plans, planned{kind: kind, id: id, publicID: item.ID, fromApp: item.AppID})
		default:
			item, ok := templateByID[id]
			if !ok {
				return 0, errSourceNotFound
			}
			if ownerDeveloperID > 0 && item.DeveloperID != ownerDeveloperID {
				return 0, errors.New("只能切换自己的条目")
			}
			publicID := strings.TrimSpace(item.TemplateKey)
			if publicID == "" {
				publicID = item.ID
			}
			plans = append(plans, planned{kind: kind, id: id, publicID: publicID, fromApp: item.AppID})
		}
	}
	occupied := map[string]string{}
	for _, item := range plugins {
		if item.AppID == target.ID {
			occupied[sourceKindPlugin+"\x00"+item.ID] = item.ID
		}
	}
	for _, item := range templates {
		if item.AppID != target.ID {
			continue
		}
		publicID := strings.TrimSpace(item.TemplateKey)
		if publicID == "" {
			publicID = item.ID
		}
		occupied[sourceKindTemplate+"\x00"+publicID] = item.ID
	}
	moving := make([]planned, 0, len(plans))
	for _, plan := range plans {
		if plan.fromApp == target.ID {
			continue
		}
		slot := plan.kind + "\x00" + plan.publicID
		if owner, ok := occupied[slot]; ok && owner != plan.id {
			return 0, fmt.Errorf("目标应用里已经有标识「%s」", plan.publicID)
		}
		occupied[slot] = plan.id
		moving = append(moving, plan)
	}
	store := currentSourceStationStore()
	for _, plan := range moving {
		if err := store.SetCatalogAppID(plan.kind, plan.id, target.ID); err != nil {
			return 0, err
		}
		detail := fmt.Sprintf("从应用 %d 切换到 %s（%d）", plan.fromApp, target.Name, target.ID)
		_ = store.AppendAudit(sourceAuditEntry{
			ActorType: actorType, ActorName: actor, Action: "rebind_app",
			TargetType: plan.kind, TargetID: plan.id, Detail: detail,
		})
	}
	return len(plans), nil
}

func writeCatalogRebindError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errSourceNotFound):
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "条目不存在"})
	case errors.Is(err, errSourceAppNotFound):
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "应用不存在"})
	case errors.Is(err, errSourceAppRequired):
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
	}
}

func parseMigrateAppID(c *gin.Context) (int64, error) {
	raw := strings.TrimSpace(c.Query("migrateAppId"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("migrate_app_id"))
	}
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("请选择要迁移到的其他应用")
	}
	return id, nil
}
