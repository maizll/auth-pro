package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type sourceDeveloperApplyRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Reason      string `json:"reason"`
}

type sourcePluginDraftRequest struct {
	ID          string       `json:"id"`
	Category    string       `json:"category"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Icon        string       `json:"icon"`
	Version     string       `json:"version"`
	SHA256      string       `json:"sha256"`
	DownloadURL string       `json:"downloadUrl"`
	Changelog   string       `json:"changelog"`
	MinVersion  string       `json:"minVersion"`
	ForceUpdate bool         `json:"forceUpdate"`
	Author      sourceAuthor `json:"author"`
	Shelf       bool         `json:"shelf"`
}

type sourceTemplateDraftRequest struct {
	ID            string       `json:"id"`
	TemplateKey   string       `json:"templateKey"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Version       string       `json:"version"`
	SchemaVersion int          `json:"schemaVersion"`
	SHA256        string       `json:"sha256"`
	TemplateURL   string       `json:"templateUrl"`
	Changelog     string       `json:"changelog"`
	MinVersion    string       `json:"minVersion"`
	ForceUpdate   bool         `json:"forceUpdate"`
	Author        sourceAuthor `json:"author"`
	Shelf         bool         `json:"shelf"`
}

type sourceReleaseDraftRequest struct {
	Version     string `json:"version"`
	Changelog   string `json:"changelog"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"downloadUrl"`
	TemplateURL string `json:"templateUrl"`
}

func SourceDeveloperApply(c *gin.Context) {
	var req sourceDeveloperApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !pluginIDPattern.MatchString(username) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "用户名需为 2-59 位小写字母、数字或连字符"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "密码至少 6 位"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "密码加密失败"})
		return
	}
	displayName := truncateText(strings.TrimSpace(req.DisplayName), 80)
	if displayName == "" {
		displayName = username
	}
	app, err := currentSourceStationStore().CreateApplication(sourceApplication{
		Username:     username,
		PasswordHash: string(hash),
		Email:        truncateText(strings.TrimSpace(req.Email), 100),
		DisplayName:  displayName,
		Reason:       truncateText(strings.TrimSpace(req.Reason), 500),
	})
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "入驻申请已提交，等待管理员审核", "data": gin.H{
		"id": app.ID, "username": app.Username, "status": app.Status,
	}})
}

func SourceDeveloperApplyStatus(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请提供用户名"})
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if developer, err := currentSourceStationStore().GetDeveloperByUsername(username); err == nil {
		status := sourceApplicationApproved
		if !developer.Enabled {
			status = sourceApplicationFrozen
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"username": developer.Username, "status": status, "enabled": developer.Enabled,
		}})
		return
	}
	apps, err := currentSourceStationStore().ListApplications("")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询申请失败"})
		return
	}
	for _, app := range apps {
		if strings.EqualFold(app.Username, username) {
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
				"id": app.ID, "username": app.Username, "status": app.Status, "reviewNote": app.ReviewNote,
			}})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "未找到入驻申请"})
}

func SourceDeveloperLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	developer, err := currentSourceStationStore().GetDeveloperByUsername(strings.ToLower(strings.TrimSpace(req.Username)))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "账号或密码错误"})
		return
	}
	if !developer.Enabled {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errDeveloperDisabled.Error()})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(developer.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "账号或密码错误"})
		return
	}
	now := time.Now()
	claims := &middleware.Claims{
		UserID:   uint(developer.ID),
		Username: developer.Username,
		Role:     "developer",
		RoleCode: sourceDeveloperRoleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成 token 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "登录成功", "data": gin.H{
		"token": token, "username": developer.Username, "displayName": developer.DisplayName,
	}})
}

func SourceDeveloperMe(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"id": developer.ID, "username": developer.Username, "displayName": developer.DisplayName,
		"email": developer.Email, "roleCode": sourceDeveloperRoleCode,
	}})
}

func SourceDeveloperItems(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	plugins, _ := currentSourceStationStore().ListPlugins("")
	templates, _ := currentSourceStationStore().ListTemplates("")
	ownedPlugins := make([]gin.H, 0)
	for _, plugin := range plugins {
		if plugin.DeveloperID == developer.ID {
			ownedPlugins = append(ownedPlugins, sourcePluginView(plugin))
		}
	}
	ownedTemplates := make([]gin.H, 0)
	for _, template := range templates {
		if template.DeveloperID == developer.ID {
			ownedTemplates = append(ownedTemplates, sourceTemplateView(template))
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"plugins": ownedPlugins, "homeTemplates": ownedTemplates}})
}

func SourceDeveloperUpsertPlugin(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	plugin, err := bindSourcePluginDraft(c, developer)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	saved, err := currentSourceStationStore().UpsertPlugin(plugin, false)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "插件元数据已保存（源站不存储源码）", "data": sourcePluginView(saved)})
}

func SourceDeveloperSubmitPlugin(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	item, err := currentSourceStationStore().GetPlugin(strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if item.DeveloperID != developer.ID {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
		return
	}
	saved, err := currentSourceStationStore().SetPluginStatus(item.ID, sourceItemReview, developer.Username, sourceNoteFromBody(c))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已提交审核", "data": sourcePluginView(saved)})
}

func SourceDeveloperPluginVersions(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	item, err := currentSourceStationStore().GetPlugin(strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if item.DeveloperID != developer.ID {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
		return
	}
	writeSourceVersionList(c, sourceKindPlugin, item.ID)
}

func SourceDeveloperUpsertPluginVersion(c *gin.Context) {
	sourceDeveloperWriteVersion(c, sourceKindPlugin)
}

func SourceDeveloperSubmitPluginVersion(c *gin.Context) {
	sourceDeveloperSubmitVersion(c, sourceKindPlugin)
}

func SourceDeveloperUpsertTemplate(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	item, err := bindSourceTemplateDraft(c, developer)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	saved, err := currentSourceStationStore().UpsertTemplate(item, false)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "模板元数据已保存（源站不存储源码）", "data": sourceTemplateView(saved)})
}

func SourceDeveloperSubmitTemplate(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	item, err := currentSourceStationStore().GetTemplate(strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if item.DeveloperID != developer.ID {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
		return
	}
	saved, err := currentSourceStationStore().SetTemplateStatus(item.ID, sourceItemReview, developer.Username, sourceNoteFromBody(c))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已提交审核", "data": sourceTemplateView(saved)})
}

func SourceDeveloperTemplateVersions(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	item, err := currentSourceStationStore().GetTemplate(strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if item.DeveloperID != developer.ID {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
		return
	}
	writeSourceVersionList(c, sourceKindTemplate, item.ID)
}

func SourceDeveloperUpsertTemplateVersion(c *gin.Context) {
	sourceDeveloperWriteVersion(c, sourceKindTemplate)
}

func SourceDeveloperSubmitTemplateVersion(c *gin.Context) {
	sourceDeveloperSubmitVersion(c, sourceKindTemplate)
}

func AdminSourceDeveloperApplications(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if status == "cancelled" {
		status = sourceApplicationFrozen
	}
	if status == "" {
		status = sourceApplicationPending
	}
	if status != sourceApplicationPending && status != sourceApplicationApproved &&
		status != sourceApplicationRejected && status != sourceApplicationFrozen {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "状态不合法"})
		return
	}
	items, err := currentSourceStationStore().ListApplications(status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取入驻申请失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, sourceApplicationView(item))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func AdminSourceDeveloperApprove(c *gin.Context) {
	id, err := parseSourceApplicationID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "申请标识不合法"})
		return
	}
	developer, err := currentSourceStationStore().ApproveApplication(id, c.GetString("username"))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已通过入驻并创建开发者账号", "data": gin.H{
		"developerId": developer.ID, "username": developer.Username, "roleCode": sourceDeveloperRoleCode,
	}})
}

func AdminSourceDeveloperReject(c *gin.Context) {
	id, err := parseSourceApplicationID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "申请标识不合法"})
		return
	}
	if err := currentSourceStationStore().RejectApplication(id, c.GetString("username"), sourceNoteFromBody(c)); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已拒绝入驻申请"})
}

func AdminSourceDeveloperCancel(c *gin.Context) {
	id, err := parseSourceApplicationID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "申请标识不合法"})
		return
	}
	if err := currentSourceStationStore().FreezeApplication(id, c.GetString("username"), sourceNoteFromBody(c)); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已取消开发者资格并删除账号，该用户名可重新申请入驻"})
}

func currentSourceDeveloper(c *gin.Context) (sourceDeveloper, error) {
	developer, err := currentSourceStationStore().GetDeveloperByID(int64(c.GetUint("user_id")))
	if err != nil {
		return sourceDeveloper{}, errors.New("开发者账号不存在")
	}
	if !developer.Enabled {
		return sourceDeveloper{}, errDeveloperDisabled
	}
	return developer, nil
}

func sourceApplicationView(item sourceApplication) gin.H {
	reviewedAt := ""
	if item.ReviewedAt != nil {
		reviewedAt = item.ReviewedAt.Format(time.RFC3339)
	}
	return gin.H{
		"id": item.ID, "username": item.Username, "email": item.Email, "displayName": item.DisplayName,
		"reason": item.Reason, "status": item.Status, "reviewNote": item.ReviewNote, "reviewedBy": item.ReviewedBy,
		"reviewedAt": reviewedAt, "createdAt": item.CreatedAt.Format(time.RFC3339),
	}
}

func sourcePluginView(item sourcePlugin) gin.H {
	return gin.H{
		"id": item.ID, "developerId": item.DeveloperID, "category": item.Category, "name": item.Name,
		"description": item.Description, "icon": item.Icon, "version": item.Version, "author": item.Author,
		"sha256": item.SHA256, "downloadUrl": item.DownloadURL, "changelog": item.Changelog,
		"latestVersion": item.LatestVersion, "minVersion": item.MinVersion, "forceUpdate": item.ForceUpdate,
		"status": item.Status, "reviewNote": item.ReviewNote, "reviewedBy": item.ReviewedBy,
		"updatedAt": item.UpdatedAt.Format(time.RFC3339), "createdAt": item.CreatedAt.Format(time.RFC3339),
	}
}

func sourceTemplateView(item sourceTemplate) gin.H {
	return gin.H{
		"id": item.ID, "developerId": item.DeveloperID, "templateKey": item.TemplateKey, "name": item.Name,
		"description": item.Description, "version": item.Version, "schemaVersion": item.SchemaVersion,
		"sha256": item.SHA256, "templateUrl": item.TemplateURL, "changelog": item.Changelog,
		"latestVersion": item.LatestVersion, "minVersion": item.MinVersion, "forceUpdate": item.ForceUpdate,
		"status": item.Status, "author": item.Author, "reviewNote": item.ReviewNote, "reviewedBy": item.ReviewedBy,
		"updatedAt": item.UpdatedAt.Format(time.RFC3339), "createdAt": item.CreatedAt.Format(time.RFC3339),
	}
}

func sourceReleaseView(item sourceRelease) gin.H {
	view := gin.H{
		"kind": item.Kind, "itemId": item.ItemID, "version": item.Version, "changelog": item.Changelog,
		"sha256": item.SHA256, "status": item.Status, "reviewNote": item.ReviewNote, "reviewedBy": item.ReviewedBy,
		"createdAt": item.CreatedAt.Format(time.RFC3339), "updatedAt": item.UpdatedAt.Format(time.RFC3339),
	}
	if item.Kind == sourceKindTemplate {
		view["templateUrl"] = item.Location
	} else {
		view["downloadUrl"] = item.Location
	}
	return view
}

func bindSourcePluginDraft(c *gin.Context, developer sourceDeveloper) (sourcePlugin, error) {
	var req sourcePluginDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return sourcePlugin{}, errors.New("参数错误")
	}
	pluginID := strings.TrimSpace(req.ID)
	if pluginID == "" {
		pluginID = strings.TrimSpace(c.Param("id"))
	}
	if !pluginIDPattern.MatchString(pluginID) {
		return sourcePlugin{}, errors.New("插件标识不合法")
	}
	if _, builtin := findCatalogPlugin(pluginID); builtin {
		return sourcePlugin{}, errors.New("不能覆盖内置插件标识")
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
	authorName := sourceFirstNonEmpty(truncateText(req.Author.Name, 100), developer.DisplayName, developer.Username)
	return sourcePlugin{
		ID:          pluginID,
		DeveloperID: developer.ID,
		Category:    normalizePluginCategory(strings.TrimSpace(req.Category)),
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
			Name:  authorName,
			URL:   truncateText(req.Author.URL, 300),
			Email: sourceFirstNonEmpty(truncateText(req.Author.Email, 200), developer.Email),
		},
	}, nil
}

func bindSourceTemplateDraft(c *gin.Context, developer sourceDeveloper) (sourceTemplate, error) {
	var req sourceTemplateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return sourceTemplate{}, errors.New("参数错误")
	}
	templateKey := strings.TrimSpace(req.TemplateKey)
	if templateKey == "" {
		templateKey = strings.TrimSpace(req.ID)
	}
	if templateKey == "" {
		templateKey = strings.TrimSpace(c.Param("id"))
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
	return sourceTemplate{
		ID:            templateKey,
		DeveloperID:   developer.ID,
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
			Name:  sourceFirstNonEmpty(truncateText(req.Author.Name, 100), developer.DisplayName, developer.Username),
			URL:   truncateText(req.Author.URL, 300),
			Email: sourceFirstNonEmpty(truncateText(req.Author.Email, 200), developer.Email),
		},
	}, nil
}

func writeSourceDeveloperStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errSourceNotFound):
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": err.Error()})
	case errors.Is(err, errSourceConflict), errors.Is(err, errApplicationPending), errors.Is(err, errApplicationReviewed),
		errors.Is(err, errApplicationNotCancellable), errors.Is(err, errDeveloperAlreadyCancelled),
		errors.Is(err, errSourcePublishIncomplete), errors.Is(err, errSourceInvalidStatus),
		errors.Is(err, errSourceVersionImmutable), errors.Is(err, errSourceVersionNotLatest):
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
	case errors.Is(err, errSourceForbidden), errors.Is(err, errDeveloperDisabled):
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "源站存储失败"})
	}
}

func parseSourceApplicationID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func sourceFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeSourceVersionList(c *gin.Context, kind, itemID string) {
	items, err := currentSourceStationStore().ListVersions(kind, itemID)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, sourceReleaseView(item))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func sourceDeveloperWriteVersion(c *gin.Context, kind string) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	rel, err := bindSourceReleaseDraft(c, kind)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	rel.ItemID = strings.TrimSpace(c.Param("id"))
	saved, err := currentSourceStationStore().UpsertVersion(rel, developer.ID, false)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "版本草稿已保存（源站不存储源码）", "data": sourceReleaseView(saved)})
}

func sourceDeveloperSubmitVersion(c *gin.Context, kind string) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	itemID := strings.TrimSpace(c.Param("id"))
	if kind == sourceKindTemplate {
		item, err := currentSourceStationStore().GetTemplate(itemID)
		if err != nil {
			writeSourceDeveloperStoreError(c, err)
			return
		}
		if item.DeveloperID != developer.ID {
			c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
			return
		}
	} else {
		item, err := currentSourceStationStore().GetPlugin(itemID)
		if err != nil {
			writeSourceDeveloperStoreError(c, err)
			return
		}
		if item.DeveloperID != developer.ID {
			c.JSON(http.StatusOK, gin.H{"code": 403, "msg": errSourceForbidden.Error()})
			return
		}
	}
	saved, err := currentSourceStationStore().SetVersionStatus(kind, itemID, strings.TrimSpace(c.Param("version")), sourceVersionPending, developer.Username, sourceNoteFromBody(c))
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "版本已提交审核", "data": sourceReleaseView(saved)})
}

func bindSourceReleaseDraft(c *gin.Context, kind string) (sourceRelease, error) {
	var req sourceReleaseDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return sourceRelease{}, errors.New("参数错误")
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = strings.TrimSpace(c.Param("version"))
	}
	location := strings.TrimSpace(req.DownloadURL)
	if kind == sourceKindTemplate {
		location = strings.TrimSpace(req.TemplateURL)
		if location != "" {
			if err := validateTemplateLocation(location); err != nil {
				return sourceRelease{}, err
			}
		}
	} else if location != "" {
		if err := validatePluginDownloadURL(location); err != nil {
			return sourceRelease{}, err
		}
	}
	if req.SHA256 != "" {
		if err := validateSHA256(req.SHA256); err != nil {
			return sourceRelease{}, err
		}
	}
	return sourceRelease{
		Kind:      kind,
		Version:   version,
		Changelog: truncateText(req.Changelog, 2000),
		Location:  location,
		SHA256:    strings.ToLower(strings.TrimSpace(req.SHA256)),
		Status:    sourceVersionDraft,
	}, nil
}
