package handler

import (
	"net/http"
	"strconv"
	"strings"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

func RegisterAppRepoRoutes(admin *gin.RouterGroup) {
	admin.GET("/app-repos", AdminAppRepoList)
	// 只读列出令牌能访问的仓库。开发者分组没有这条路由。
	admin.GET("/app-repos/github", AdminListGitHubRepos)
	admin.GET("/app-repos/token", AdminAppRepoToken)
	admin.POST("/app-repos/suggest", AdminAppRepoSuggest)
	admin.POST("/app-repos/bind", AdminAppRepoBind)
	admin.POST("/app-repos/preview", AdminAppRepoPreview)
	admin.POST("/app-repos/rebind", AdminAppRepoRebind)
	admin.POST("/app-repos/impact", AdminAppRepoImpact)
	admin.POST("/app-repos/unbind", AdminAppRepoUnbind)
	admin.GET("/app-repos/audit", AdminAppRepoAudit)
}

func AdminAppRepoList(c *gin.Context) {
	db, err := config.DB()
	if err != nil || db == nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	_ = ensureAppRepoSchema(db)
	rows, err := db.Query(`SELECT id, app_name, app_key, deleted_at IS NOT NULL FROM apps ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取应用失败"})
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var name, appKey string
		var archived bool
		if err := rows.Scan(&id, &name, &appKey, &archived); err != nil {
			continue
		}
		item := gin.H{"appId": id, "name": name, "appKey": appKey, "archived": archived, "status": "unbound", "repo": "", "health": "尚未绑定"}
		if row, ok, _ := loadAppRepo(id); ok {
			item["status"] = row.Status
			if row.Status == "" {
				item["status"] = appRepoStatusReady
			}
			item["repo"] = row.Owner + "/" + row.Repo
			item["health"] = "私有，安装包都在"
			if row.Status == appRepoStatusDegraded {
				item["health"] = "令牌已失效"
			}
		}
		list = append(list, item)
	}
	if len(list) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "empty": appRepoNoAppsText}})
		return
	}
	_, _, tokenOK := primaryGitHubBindingLocation()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"list": list, "tokenReady": tokenOK,
		"note": "现有安装包还在存储管理的主存储里，不会自动拆给每个应用。本站程序的在线更新不在这里配置。",
	}})
}

func AdminAppRepoToken(c *gin.Context) {
	loc, _, ok := primaryGitHubBindingLocation()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"ready": false, "message": appRepoTokenMissingText,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"ready": true, "location": loc.Name, "message": "使用存储管理中的 GitHub 主存储，密钥已保存",
	}})
}

func AdminAppRepoSuggest(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		AppKey string `json:"appKey"`
	}
	_ = c.ShouldBindJSON(&req)
	loc, token, ok := primaryGitHubBindingLocation()
	owner := ""
	if ok {
		identity, err := fetchGitHubPaidIdentity(c.Request.Context(), token)
		if err == nil {
			owner = identity.Login
		}
	}
	_ = loc
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"repo": suggestAppRepoName(req.Name, req.AppKey, owner), "tokenReady": ok,
	}})
}

func AdminAppRepoBind(c *gin.Context) {
	var req struct {
		AppID  int64  `json:"appId"`
		Action string `json:"action"`
		Repo   string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AppID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择应用"})
		return
	}
	db, err := config.DB()
	if err != nil || db == nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	var name, appKey string
	var archived int
	if err := db.QueryRow(`SELECT app_name, app_key, deleted_at IS NOT NULL FROM apps WHERE id = ?`, req.AppID).Scan(&name, &appKey, &archived); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "应用不存在"})
		return
	}
	if archived == 1 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "已归档的应用不能绑定仓库"})
		return
	}
	create := req.Action != "bind"
	row, err := bindAppRepo(c.Request.Context(), req.AppID, appKey, name, req.Action, req.Repo, create)
	if err != nil {
		code := 400
		if err.Error() == appRepoBusyText {
			code = 409
		}
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已绑定 " + row.Owner + "/" + row.Repo, "data": gin.H{"repo": row.Owner + "/" + row.Repo}})
}

func AdminAppRepoPreview(c *gin.Context) {
	var req struct {
		AppID int64  `json:"appId"`
		Repo  string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AppID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择应用"})
		return
	}
	row, ok, err := loadAppRepo(req.AppID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": appRepoUnboundText})
		return
	}
	items, err := listReleaseImportReleases(c.Request.Context(), row.Owner, row.Repo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": appRepoListFailText})
		return
	}
	groups := make([]gin.H, 0, len(appRepoPrefixes()))
	total := 0
	for _, prefix := range appRepoPrefixes() {
		count := 0
		for _, item := range items {
			if strings.HasPrefix(item.Tag, prefix) {
				count++
			}
		}
		total += count
		label := "没有文件"
		if count > 0 {
			label = strconv.Itoa(count) + " 个"
		}
		groups = append(groups, gin.H{"prefix": prefix, "text": label})
	}
	empty := ""
	if total == 0 {
		empty = appRepoEmptyFilesText
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"from": row.Owner + "/" + row.Repo, "to": strings.TrimSpace(req.Repo), "groups": groups, "empty": empty,
	}})
}

func AdminAppRepoRebind(c *gin.Context) {
	var req struct {
		AppID int64  `json:"appId"`
		Repo  string `json:"repo"`
		Mode  string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AppID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择应用"})
		return
	}
	if appRepoBusyNow(req.AppID) {
		c.JSON(http.StatusOK, gin.H{"code": 409, "msg": appRepoImportBusyText})
		return
	}
	current, ok, err := loadAppRepo(req.AppID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": appRepoUnboundText})
		return
	}
	owner, repo, err := splitOwnerRepo(req.Repo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if req.Mode == "copy" {
		for _, prefix := range appRepoPrefixes() {
			if _, err := copyRepoPrefix(c.Request.Context(), current.Owner, current.Repo, owner, repo, prefix); err != nil {
				appRepoAudit("migrate_failed", current.AppKey, "迁走失败："+err.Error())
				c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
				return
			}
		}
	}
	current.Owner = owner
	current.Repo = repo
	current.Status = appRepoStatusReady
	if err := saveAppRepo(current); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "绑定没有写上"})
		return
	}
	msg := "已切换到 " + owner + "/" + repo + "。已发布的文件仍在原仓库。"
	action := "switch"
	if req.Mode == "copy" {
		msg = "已复制到 " + owner + "/" + repo
		action = "migrate"
	}
	appRepoAudit(action, current.AppKey, msg)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg})
}

func AdminAppRepoImpact(c *gin.Context) {
	var req struct {
		AppID int64 `json:"appId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AppID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择应用"})
		return
	}
	row, ok, err := loadAppRepo(req.AppID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "这个应用还没有绑定仓库"})
		return
	}
	published, drafts := appRepoImpact(req.AppID, row.Owner, row.Repo)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"published": published, "drafts": drafts, "blocked": published > 0, "busy": appRepoBusyNow(req.AppID),
	}})
}

func AdminAppRepoUnbind(c *gin.Context) {
	var req struct {
		AppID int64 `json:"appId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AppID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择应用"})
		return
	}
	if appRepoBusyNow(req.AppID) {
		c.JSON(http.StatusOK, gin.H{"code": 409, "msg": appRepoImportBusyText})
		return
	}
	row, ok, err := loadAppRepo(req.AppID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "这个应用还没有绑定仓库"})
		return
	}
	published, drafts := appRepoImpact(req.AppID, row.Owner, row.Repo)
	if published > 0 {
		appRepoAudit("unbind_blocked", row.AppKey, "已发布版本还依赖这个仓库，不能解除绑定")
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "已发布的版本还依赖这个仓库，请先迁到新仓库", "data": gin.H{"published": published, "drafts": drafts, "blocked": true}})
		return
	}
	if err := deleteAppRepo(req.AppID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "解除绑定失败"})
		return
	}
	appRepoAudit("unbind", row.AppKey, "已解除绑定。远程仓库仍保留。")
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已解除绑定。远程仓库仍保留。", "data": gin.H{"published": published, "drafts": drafts}})
}

func AdminAppRepoAudit(c *gin.Context) {
	items, err := currentSourceStationStore().ListAudit(50)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取变更记录失败"})
		return
	}
	list := make([]sourceAuditEntry, 0)
	for _, item := range items {
		if item.TargetType == appRepoTargetType {
			list = append(list, item)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list}})
}
