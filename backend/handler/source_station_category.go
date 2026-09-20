package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	sourceCategoryHomeTemplate         = "home-template"
	sourceCatalogCategoriesSettingsKey = "catalog_categories"
)

type sourceCatalogCategory struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Builtin bool   `json:"builtin"`
}

func builtinSourceCatalogCategories() []sourceCatalogCategory {
	return []sourceCatalogCategory{
		{Key: "payment", Label: "支付", Kind: sourceKindPlugin, Builtin: true},
		{Key: "realname", Label: "实名认证", Kind: sourceKindPlugin, Builtin: true},
		{Key: "other", Label: "其他", Kind: sourceKindPlugin, Builtin: true},
		{Key: sourceCategoryHomeTemplate, Label: "首页模板", Kind: sourceKindTemplate, Builtin: true},
	}
}

func resolveSourceCatalogCategories() []sourceCatalogCategory {
	merged := append([]sourceCatalogCategory{}, builtinSourceCatalogCategories()...)
	seen := make(map[string]struct{}, len(merged))
	for _, item := range merged {
		seen[item.Key] = struct{}{}
	}
	extras, err := currentSourceStationStore().ListCatalogCategoryExtras()
	if err != nil {
		return merged
	}
	for _, extra := range extras {
		normalized, normErr := normalizeSourceCatalogCategoryInput(extra, false)
		if normErr != nil {
			continue
		}
		if _, exists := seen[normalized.Key]; exists {
			continue
		}
		seen[normalized.Key] = struct{}{}
		merged = append(merged, normalized)
	}
	return merged
}

func findSourceCatalogCategory(key string) (sourceCatalogCategory, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, item := range resolveSourceCatalogCategories() {
		if item.Key == key {
			return item, true
		}
	}
	return sourceCatalogCategory{}, false
}

func sourceCatalogCategoryKind(key string) string {
	item, ok := findSourceCatalogCategory(key)
	if !ok {
		return ""
	}
	return item.Kind
}

func defaultSourceCatalogCategory(kind string) string {
	if kind == sourceKindTemplate {
		return sourceCategoryHomeTemplate
	}
	return "other"
}

func normalizeAssignedCatalogCategory(kind, raw string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return defaultSourceCatalogCategory(kind), nil
	}
	item, ok := findSourceCatalogCategory(key)
	if !ok {
		return "", errors.New("未知分类，请先在目录分类中配置")
	}
	if item.Kind != kind {
		if kind == sourceKindPlugin {
			return "", errors.New("该分类属于首页模板，不能用于插件包")
		}
		return "", errors.New("该分类属于插件，不能用于首页模板包")
	}
	return item.Key, nil
}

func normalizeSourceCatalogCategoryInput(item sourceCatalogCategory, builtin bool) (sourceCatalogCategory, error) {
	key := strings.ToLower(strings.TrimSpace(item.Key))
	if !pluginIDPattern.MatchString(key) {
		return sourceCatalogCategory{}, errors.New("分类标识不合法：须为 2-59 位小写字母、数字或连字符")
	}
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	if kind != sourceKindPlugin && kind != sourceKindTemplate {
		return sourceCatalogCategory{}, errors.New("分类 kind 仅支持 plugin 或 template")
	}
	label := truncateText(strings.TrimSpace(item.Label), 40)
	if label == "" {
		label = key
	}
	return sourceCatalogCategory{Key: key, Label: label, Kind: kind, Builtin: builtin}, nil
}

func AdminSourceCatalogCategories(c *gin.Context) {
	list := resolveSourceCatalogCategories()
	extras, err := currentSourceStationStore().ListCatalogCategoryExtras()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取目录分类失败"})
		return
	}
	if extras == nil {
		extras = []sourceCatalogCategory{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "extras": extras}})
}

func AdminSourceCatalogCategoriesSave(c *gin.Context) {
	var req struct {
		Extras []sourceCatalogCategory `json:"extras"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	builtin := map[string]struct{}{}
	for _, item := range builtinSourceCatalogCategories() {
		builtin[item.Key] = struct{}{}
	}
	normalized := make([]sourceCatalogCategory, 0, len(req.Extras))
	seen := map[string]struct{}{}
	for _, extra := range req.Extras {
		item, err := normalizeSourceCatalogCategoryInput(extra, false)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		if _, exists := builtin[item.Key]; exists {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不能覆盖内置分类 " + item.Key})
			return
		}
		if _, exists := seen[item.Key]; exists {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "分类标识重复：" + item.Key})
			return
		}
		seen[item.Key] = struct{}{}
		normalized = append(normalized, item)
	}
	if err := currentSourceStationStore().SaveCatalogCategoryExtras(normalized); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存目录分类失败"})
		return
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: c.GetString("username"), Action: "categories",
		TargetType: "catalog", TargetID: "extras", Detail: truncateText(strings.Join(categoryKeys(normalized), ","), 500),
	})
	persistIndexSnapshot(c.GetString("username"))
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存目录分类", "data": gin.H{
		"list":   resolveSourceCatalogCategories(),
		"extras": normalized,
	}})
}

func AdminSourceCatalogItems(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	category := strings.ToLower(strings.TrimSpace(c.Query("category")))
	appID, err := requestSourceCatalogAppID(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	items, err := listSourceCatalogItems(status, category, appID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取软件目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": items, "total": len(items)}})
}

func listSourceCatalogItems(status, category string, appID int64) ([]gin.H, error) {
	plugins, err := currentSourceStationStore().ListPlugins(status)
	if err != nil {
		return nil, err
	}
	templates, err := currentSourceStationStore().ListTemplates(status)
	if err != nil {
		return nil, err
	}
	items := make([]gin.H, 0, len(plugins)+len(templates))
	for _, plugin := range plugins {
		if !matchSourceCatalogAppID(plugin.AppID, appID) {
			continue
		}
		if status == "" && plugin.Status == sourceItemDeprecated {
			continue
		}
		view := sourceCatalogItemFromPlugin(plugin)
		if category == "" || view["category"] == category {
			items = append(items, view)
		}
	}
	for _, template := range templates {
		if !matchSourceCatalogAppID(template.AppID, appID) {
			continue
		}
		if status == "" && template.Status == sourceItemDeprecated {
			continue
		}
		view := sourceCatalogItemFromTemplate(template)
		if category == "" || view["category"] == category {
			items = append(items, view)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, _ := items[i]["updatedAt"].(string)
		right, _ := items[j]["updatedAt"].(string)
		if left == right {
			leftID, _ := items[i]["id"].(string)
			rightID, _ := items[j]["id"].(string)
			return leftID < rightID
		}
		return left > right
	})
	return items, nil
}

func sourceCatalogItemFromPlugin(item sourcePlugin) gin.H {
	view := sourcePluginView(item)
	view["kind"] = sourceKindPlugin
	view["categoryLabel"] = sourceCatalogCategoryLabel(item.Category)
	view["location"] = item.DownloadURL
	return view
}

func sourceCatalogItemFromTemplate(item sourceTemplate) gin.H {
	view := sourceTemplateView(item)
	view["kind"] = sourceKindTemplate
	view["categoryLabel"] = sourceCatalogCategoryLabel(item.Category)
	view["location"] = item.TemplateURL
	return view
}

func sourceCatalogCategoryLabel(key string) string {
	if item, ok := findSourceCatalogCategory(key); ok {
		return item.Label
	}
	return key
}

func categoryKeys(items []sourceCatalogCategory) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	return keys
}

func marshalCatalogCategoryExtras(items []sourceCatalogCategory) (string, error) {
	if items == nil {
		items = []sourceCatalogCategory{}
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalCatalogCategoryExtras(raw string) []sourceCatalogCategory {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []sourceCatalogCategory{}
	}
	var items []sourceCatalogCategory
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []sourceCatalogCategory{}
	}
	normalized := make([]sourceCatalogCategory, 0, len(items))
	for _, item := range items {
		if parsed, err := normalizeSourceCatalogCategoryInput(item, false); err == nil {
			normalized = append(normalized, parsed)
		}
	}
	return normalized
}
