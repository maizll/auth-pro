package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
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
	plugins, _ := currentSourceStationStore().ListPlugins()
	templates, _ := currentSourceStationStore().ListTemplates()
	ownedPlugins := make([]gin.H, 0)
	for _, plugin := range plugins {
		if plugin.DeveloperID == developer.ID {
			ownedPlugins = append(ownedPlugins, gin.H{
				"id": plugin.ID, "name": plugin.Name, "version": plugin.Version, "category": plugin.Category,
			})
		}
	}
	ownedTemplates := make([]gin.H, 0)
	for _, template := range templates {
		if template.DeveloperID == developer.ID {
			ownedTemplates = append(ownedTemplates, gin.H{
				"id": template.ID, "templateKey": template.TemplateKey, "name": template.Name,
				"version": template.Version, "format": template.Format,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"plugins": ownedPlugins, "homeTemplates": ownedTemplates}})
}

func SourceDeveloperPublishPlugin(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	payload, err := readSourceUpload(c, "file", pluginPackageMaxSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if _, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload))); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "插件包必须是有效 ZIP"})
		return
	}
	pluginID := strings.TrimSpace(c.PostForm("id"))
	if pluginID == "" {
		pluginID = strings.TrimSpace(c.PostForm("pluginId"))
	}
	if !pluginIDPattern.MatchString(pluginID) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "插件标识不合法"})
		return
	}
	if _, builtin := findCatalogPlugin(pluginID); builtin {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不能覆盖内置插件标识"})
		return
	}
	if err := validatePublishedPluginZIP(payload, pluginID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	name := truncateText(c.PostForm("name"), 100)
	if name == "" {
		name = pluginID
	}
	version := truncateText(c.PostForm("version"), 40)
	if version == "" {
		version = "1.0.0"
	}
	category := normalizePluginCategory(strings.TrimSpace(c.PostForm("category")))
	icon := truncateText(c.PostForm("icon"), 80)
	if icon == "" {
		icon = "ri:puzzle-line"
	}
	plugin := sourcePlugin{
		ID:          pluginID,
		DeveloperID: developer.ID,
		Category:    category,
		Name:        name,
		Description: truncateText(c.PostForm("description"), 500),
		Icon:        icon,
		Version:     version,
		Author: sourceAuthor{
			Name:  sourceFirstNonEmpty(truncateText(c.PostForm("authorName"), 100), developer.DisplayName, developer.Username),
			URL:   truncateText(c.PostForm("authorUrl"), 300),
			Email: sourceFirstNonEmpty(truncateText(c.PostForm("authorEmail"), 200), developer.Email),
		},
	}
	if err := currentSourceStationStore().PutPlugin(plugin, payload); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "插件已写入本站目录", "data": gin.H{"id": plugin.ID, "sha256": sourceContentSHA256(payload)}})
}

func SourceDeveloperPublishTemplate(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": err.Error()})
		return
	}
	payload, err := readSourceUpload(c, "file", pluginPackageMaxSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	templateKey := strings.TrimSpace(c.PostForm("templateKey"))
	if templateKey == "" {
		templateKey = strings.TrimSpace(c.PostForm("id"))
	}
	if !pluginIDPattern.MatchString(templateKey) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "模板标识不合法"})
		return
	}
	filename := strings.ToLower(c.Request.FormValue("filename"))
	if header, headerErr := c.FormFile("file"); headerErr == nil {
		filename = strings.ToLower(header.Filename)
	}
	format := strings.ToLower(strings.TrimSpace(c.PostForm("format")))
	if format == "" {
		if strings.HasSuffix(filename, ".zip") || looksLikeZIP(payload) {
			format = "zip"
		} else {
			format = "json"
		}
	}
	schemaVersion := homeTemplateSchemaVersion
	switch format {
	case "json":
		if err := validateHomeTemplateDocument(payload); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
	case "zip":
		if _, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload))); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "模板包必须是有效 ZIP"})
			return
		}
	default:
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "模板格式只支持 json 或 zip"})
		return
	}
	name := truncateText(c.PostForm("name"), 100)
	if name == "" {
		name = templateKey
	}
	version := truncateText(c.PostForm("version"), 40)
	if version == "" {
		version = "1.0.0"
	}
	var preview []byte
	previewType := ""
	if previewPayload, previewErr := readOptionalSourceUpload(c, "preview", previewMaxBytesForSource()); previewErr != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": previewErr.Error()})
		return
	} else if len(previewPayload) > 0 {
		preview = previewPayload
		previewType = http.DetectContentType(preview)
	}
	template := sourceTemplate{
		ID:            templateKey,
		DeveloperID:   developer.ID,
		TemplateKey:   templateKey,
		Name:          name,
		Description:   truncateText(c.PostForm("description"), 500),
		Version:       version,
		Format:        format,
		SchemaVersion: schemaVersion,
		Author: sourceAuthor{
			Name:  sourceFirstNonEmpty(truncateText(c.PostForm("authorName"), 100), developer.DisplayName, developer.Username),
			URL:   truncateText(c.PostForm("authorUrl"), 300),
			Email: sourceFirstNonEmpty(truncateText(c.PostForm("authorEmail"), 200), developer.Email),
		},
		PreviewContentType: previewType,
		HasPreview:         len(preview) > 0,
	}
	if err := currentSourceStationStore().PutTemplate(template, payload, preview); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "首页模板已写入本站目录", "data": gin.H{
		"id": template.ID, "templateKey": template.TemplateKey, "format": format, "sha256": sourceContentSHA256(payload),
	}})
}

func AdminSourceDeveloperApplications(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && status != sourceApplicationPending && status != sourceApplicationApproved && status != sourceApplicationRejected {
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
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := currentSourceStationStore().RejectApplication(id, c.GetString("username"), req.Note); err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已拒绝入驻申请"})
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

func writeSourceDeveloperStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errSourceNotFound):
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": err.Error()})
	case errors.Is(err, errSourceConflict), errors.Is(err, errApplicationPending), errors.Is(err, errApplicationReviewed):
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
	case errors.Is(err, errSourceForbidden):
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

func readSourceUpload(c *gin.Context, field string, maxBytes int64) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+(1<<20))
	header, err := c.FormFile(field)
	if err != nil || header.Size <= 0 {
		return nil, errors.New("请上传文件")
	}
	if header.Size > maxBytes {
		return nil, errors.New("文件超过大小限制")
	}
	file, err := header.Open()
	if err != nil {
		return nil, errors.New("读取上传文件失败")
	}
	defer file.Close()
	return readPluginReader(file, maxBytes)
}

func readOptionalSourceUpload(c *gin.Context, field string, maxBytes int64) ([]byte, error) {
	header, err := c.FormFile(field)
	if err != nil {
		return nil, nil
	}
	if header.Size <= 0 {
		return nil, nil
	}
	if header.Size > maxBytes {
		return nil, errors.New("预览图超过大小限制")
	}
	file, err := header.Open()
	if err != nil {
		return nil, errors.New("读取预览图失败")
	}
	defer file.Close()
	return readPluginReader(file, maxBytes)
}

func previewMaxBytesForSource() int64 {
	return 5 << 20
}

func looksLikeZIP(payload []byte) bool {
	return len(payload) >= 4 && payload[0] == 'P' && payload[1] == 'K'
}

func validatePublishedPluginZIP(payload []byte, pluginID string) error {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return errors.New("插件包必须是有效 ZIP")
	}
	for _, entry := range reader.File {
		if filepath.Base(entry.Name) != "plugin.json" {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			return errors.New("读取 plugin.json 失败")
		}
		body, err := io.ReadAll(io.LimitReader(file, pluginManifestMaxSize))
		file.Close()
		if err != nil {
			return errors.New("读取 plugin.json 失败")
		}
		var metadata struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &metadata) != nil || metadata.ID != pluginID {
			return errors.New("plugin.json 的插件 ID 必须与发布标识一致")
		}
		return nil
	}
	return nil
}

func sourceFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
