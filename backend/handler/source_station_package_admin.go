package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func AdminSourceReleaseSettings(c *gin.Context) {
	settings, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取 Release 设置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": settings.publicView()})
}

func AdminSourceReleaseSettingsSave(c *gin.Context) {
	var incoming sourceReleaseSettings
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	current, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取 Release 设置失败"})
		return
	}
	saved := mergeReleaseSettings(current, incoming)
	if err := validateReleaseSettings(saved); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if err := currentSourceStationStore().SaveReleaseSettings(saved); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存 Release 设置失败"})
		return
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: c.GetString("username"), Action: "settings",
		TargetType: "release", TargetID: saved.Provider,
		Detail: saved.Owner + "/" + saved.Repo + " tag=" + saved.TagStrategy,
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存 Release 推送设置（令牌仅保存在服务端，GET 只返回掩码）", "data": saved.publicView()})
}

func AdminSourceReleaseSettingsTest(c *gin.Context) {
	var incoming sourceReleaseSettings
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	current, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取 Release 设置失败"})
		return
	}
	merged := mergeReleaseSettings(current, incoming)
	if err := validateReleaseSettings(merged); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if !merged.releaseReady() {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写完整的 Provider / Owner / 仓库 / Token"})
		return
	}
	result, status, err := testSourceReleaseConnection(c.Request.Context(), merged)
	if err != nil {
		code := 502
		if status >= 400 && status < 500 {
			code = 400
		}
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "连接成功", "data": result})
}

func AdminSourcePackageParse(c *gin.Context) {
	defer discardSourceMultipart(c)
	filename, payload, err := readSourcePackageUpload(c)
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	manifest, err := parseSourcePackageBytes(filename, payload, c.PostForm("kind"))
	payload = nil
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已通过校验并解析清单（包未落盘、未入库）", "data": manifest.view()})
}

func AdminSourcePackagePublish(c *gin.Context) {
	defer discardSourceMultipart(c)
	filename, payload, err := readSourcePackageUpload(c)
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	manifest, err := parseSourcePackageBytes(filename, payload, c.PostForm("kind"))
	if err != nil {
		payload = nil
		writeSourcePackageReject(c, err)
		return
	}
	location := strings.TrimSpace(sourceFirstNonEmpty(c.PostForm("downloadUrl"), c.PostForm("templateUrl")))
	settings, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		payload = nil
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取 Release 设置失败"})
		return
	}
	pushRequested := formFlag(c, "push") || formFlag(c, "pushRelease")
	if !pushRequested && location == "" && settings.releaseReady() {
		pushRequested = true
	}
	pushed := false
	provider := ""
	if pushRequested {
		if !settings.releaseReady() {
			payload = nil
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "未配置 GitHub/Gitee 仓库与令牌：请先到「Release 设置」填写，或粘贴外部 https 下载地址"})
			return
		}
		assetURL, pushErr := pushSourcePackageRelease(c.Request.Context(), settings, manifest, payload)
		payload = nil
		if pushErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": pushErr.Error()})
			return
		}
		location = assetURL
		pushed = true
		provider = settings.Provider
	}
	payload = nil
	if location == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请粘贴外部 https 下载地址，或配置 GitHub/Gitee Release 推送"})
		return
	}

	actor := c.GetString("username")
	shelf := formFlag(c, "shelf")
	changelog := truncateText(c.PostForm("changelog"), 2000)
	minVersion := truncateText(c.PostForm("minVersion"), 40)
	forceUpdate := formFlag(c, "forceUpdate")

	var (
		pluginView   gin.H
		templateView gin.H
		itemID       string
		kind         = manifest.Kind
	)
	if kind == sourceKindTemplate {
		item, convErr := adminTemplateFromRequest(sourceTemplateDraftRequest{
			ID: manifest.ID, TemplateKey: manifest.ID, Name: manifest.Name, Description: manifest.Description,
			Version: manifest.Version, SchemaVersion: manifest.SchemaVersion, SHA256: manifest.SHA256,
			TemplateURL: location, Changelog: changelog, MinVersion: minVersion, ForceUpdate: forceUpdate,
			Author: manifest.Author,
		})
		if convErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": convErr.Error()})
			return
		}
		previous, _ := currentSourceStationStore().GetTemplate(item.ID)
		saved, upsertErr := currentSourceStationStore().ReplaceTemplateFromPackage(item, actor)
		if upsertErr != nil {
			writeSourceDeveloperStoreError(c, upsertErr)
			return
		}
		if previous.Status == sourceItemPublished {
			persistIndexSnapshot(actor)
		}
		if formFlag(c, "submit") && !shelf {
			saved, upsertErr = currentSourceStationStore().SetTemplateStatus(saved.ID, sourceItemReview, actor, "package submit")
			if upsertErr != nil {
				writeSourceDeveloperStoreError(c, upsertErr)
				return
			}
		}
		if shelf {
			if pubErr := publishSourcePackageItem(sourceKindTemplate, saved.ID, manifest.Version, actor); pubErr != nil {
				writeSourceDeveloperStoreError(c, pubErr)
				return
			}
			if saved, upsertErr = currentSourceStationStore().GetTemplate(saved.ID); upsertErr != nil {
				writeSourceDeveloperStoreError(c, upsertErr)
				return
			}
			persistIndexSnapshot(actor)
		}
		templateView = sourceTemplateView(saved)
		itemID = saved.ID
	} else {
		item, convErr := adminPluginFromRequest(sourcePluginDraftRequest{
			ID: manifest.ID, Category: manifest.Category, Name: manifest.Name, Description: manifest.Description,
			Icon: manifest.Icon, Version: manifest.Version, SHA256: manifest.SHA256, DownloadURL: location,
			Changelog: changelog, MinVersion: minVersion, ForceUpdate: forceUpdate, Author: manifest.Author,
		})
		if convErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": convErr.Error()})
			return
		}
		previous, _ := currentSourceStationStore().GetPlugin(item.ID)
		saved, upsertErr := currentSourceStationStore().ReplacePluginFromPackage(item, actor)
		if upsertErr != nil {
			writeSourceDeveloperStoreError(c, upsertErr)
			return
		}
		if previous.Status == sourceItemPublished {
			persistIndexSnapshot(actor)
		}
		if formFlag(c, "submit") && !shelf {
			saved, upsertErr = currentSourceStationStore().SetPluginStatus(saved.ID, sourceItemReview, actor, "package submit")
			if upsertErr != nil {
				writeSourceDeveloperStoreError(c, upsertErr)
				return
			}
		}
		if shelf {
			if pubErr := publishSourcePackageItem(sourceKindPlugin, saved.ID, manifest.Version, actor); pubErr != nil {
				writeSourceDeveloperStoreError(c, pubErr)
				return
			}
			if saved, upsertErr = currentSourceStationStore().GetPlugin(saved.ID); upsertErr != nil {
				writeSourceDeveloperStoreError(c, upsertErr)
				return
			}
			persistIndexSnapshot(actor)
		}
		pluginView = sourcePluginView(saved)
		itemID = saved.ID
	}

	detail := location
	if pushed {
		detail = provider + " " + location
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: actor, Action: "package_publish",
		TargetType: kind, TargetID: itemID + "@" + manifest.Version, Detail: truncateText(detail, 500),
	})
	data := gin.H{
		"kind": kind, "id": itemID, "version": manifest.Version, "sha256": manifest.SHA256,
		"downloadUrl": location, "pushed": pushed, "storedPackage": false, "manifest": manifest.view(),
	}
	if kind == sourceKindTemplate {
		data["templateUrl"] = location
		delete(data, "downloadUrl")
		data["item"] = templateView
	} else {
		data["item"] = pluginView
	}
	msg := "校验通过，已保存为草稿（包已丢弃，源站不保存源码）。请走审核/上架"
	if pushed {
		msg = "校验通过，已推送到 " + provider + " Release 并保存为草稿（包已丢弃）"
	}
	if shelf {
		msg = "校验通过，已保存并上架（包已丢弃）"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": data})
}

func publishSourcePackageItem(kind, id, version, actor string) error {
	if _, err := currentSourceStationStore().SetVersionStatus(kind, id, version, sourceVersionPublished, actor, "package publish"); err != nil {
		return err
	}
	if kind == sourceKindTemplate {
		item, err := currentSourceStationStore().GetTemplate(id)
		if err != nil {
			return err
		}
		if item.Status != sourceItemPublished {
			_, err = currentSourceStationStore().SetTemplateStatus(id, sourceItemPublished, actor, "package publish")
			return err
		}
		return nil
	}
	item, err := currentSourceStationStore().GetPlugin(id)
	if err != nil {
		return err
	}
	if item.Status != sourceItemPublished {
		_, err = currentSourceStationStore().SetPluginStatus(id, sourceItemPublished, actor, "package publish")
		return err
	}
	return nil
}

func discardSourceMultipart(c *gin.Context) {
	if c.Request != nil && c.Request.MultipartForm != nil {
		_ = c.Request.MultipartForm.RemoveAll()
	}
}
