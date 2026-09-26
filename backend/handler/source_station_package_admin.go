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
	filename, payload, _, err := readSourcePackageSource(c)
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	manifest, err := parseSourcePackageBytes(filename, payload, c.PostForm("kind"), c.PostForm("category"))
	payload = nil
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已通过校验并解析清单（包未落盘、未入库）", "data": manifest.view()})
}

func AdminSourcePackagePublish(c *gin.Context) {
	defer discardSourceMultipart(c)
	filename, payload, remoteURL, err := readSourcePackageSource(c)
	if err != nil {
		writeSourcePackageReject(c, err)
		return
	}
	manifest, err := parseSourcePackageBytes(filename, payload, c.PostForm("kind"), c.PostForm("category"))
	if err != nil {
		payload = nil
		writeSourcePackageReject(c, err)
		return
	}
	location := strings.TrimSpace(sourceFirstNonEmpty(c.PostForm("downloadUrl"), c.PostForm("templateUrl")))
	if remoteURL != "" {
		location = remoteURL
	}
	settings, err := currentSourceStationStore().GetReleaseSettings()
	if err != nil {
		payload = nil
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取 Release 设置失败"})
		return
	}
	pushRequested := formFlag(c, "push") || formFlag(c, "pushRelease")
	if pushRequested && remoteURL != "" {
		payload = nil
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "推送 Release 时请上传压缩包"})
		return
	}
	// Strictly honor checkbox: never auto-push when unchecked.
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
	priceCentsEarly, priceEarlyErr := parseCatalogPriceCents(c.PostForm("priceCents"))
	if priceEarlyErr == nil && c.PostForm("packageSource") == "upload" && priceCentsEarly > 0 && len(payload) > 0 && strings.TrimSpace(location) == "" && !pushRequested {
		publicURL, fileSHA, storeErr := storeStationPackage(payload)
		if storeErr != nil {
			payload = nil
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": storeErr.Error()})
			return
		}
		location = publicURL
		manifest.SHA256 = fileSHA
	}
	payload = nil
	if location == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "未勾选推送 Release 时，请填写外部 https 下载地址"})
		return
	}

	priceCents, priceErr := parseCatalogPriceCents(c.PostForm("priceCents"))
	if priceErr != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": priceErr.Error()})
		return
	}
	billing := c.PostForm("billing")
	delivery := c.PostForm("delivery")
	actor := c.GetString("username")
	shelf := formFlag(c, "shelf")
	changelog := truncateText(c.PostForm("changelog"), 2000)
	minVersion := truncateText(c.PostForm("minVersion"), 40)
	forceUpdate := formFlag(c, "forceUpdate")
	appID, appErr := requestSourceCatalogAppID(c)
	if appErr != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": appErr.Error()})
		return
	}
	if appID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": errSourceAppRequired.Error()})
		return
	}

	var (
		pluginView    gin.H
		templateView  gin.H
		itemID        string
		kind          = manifest.Kind
		savedLocation = location
		savedSHA      = manifest.SHA256
	)
	if kind == sourceKindTemplate {
		item, convErr := adminTemplateFromRequest(sourceTemplateDraftRequest{
			ID: manifest.ID, AppID: appID, TemplateKey: manifest.ID, Name: manifest.Name, Description: manifest.Description,
			Version: manifest.Version, SchemaVersion: manifest.SchemaVersion, SHA256: manifest.SHA256,
			TemplateURL: location, Changelog: changelog, MinVersion: minVersion, ForceUpdate: forceUpdate,
			Author: manifest.Author, PriceCents: priceCents, Billing: billing, Delivery: delivery,
		})
		if convErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": convErr.Error()})
			return
		}
		item, convErr = guardAndFinalizeTemplate(item)
		if convErr != nil {
			writeCatalogPriceError(c, convErr)
			return
		}
		saved, upsertErr := currentSourceStationStore().UpsertTemplate(item, true)
		if upsertErr != nil {
			writeSourceDeveloperStoreError(c, upsertErr)
			return
		}
		if formFlag(c, "submit") && !shelf {
			saved, upsertErr = currentSourceStationStore().SetTemplateStatus(saved.ID, sourceItemReview, actor, "package submit")
			if upsertErr != nil {
				writeSourceDeveloperStoreError(c, upsertErr)
				return
			}
		}
		if shelf {
			if pubErr := publishSourcePackageItem(sourceKindTemplate, saved.ID, saved.Version, actor); pubErr != nil {
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
		savedLocation = saved.TemplateURL
		savedSHA = saved.SHA256
	} else {
		item, convErr := adminPluginFromRequest(sourcePluginDraftRequest{
			ID: manifest.ID, AppID: appID, Category: manifest.Category, Name: manifest.Name, Description: manifest.Description,
			Icon: manifest.Icon, Version: manifest.Version, SHA256: manifest.SHA256, DownloadURL: location,
			Changelog: changelog, MinVersion: minVersion, ForceUpdate: forceUpdate, Author: manifest.Author,
			PriceCents: priceCents, Billing: billing, Delivery: delivery,
		})
		if convErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": convErr.Error()})
			return
		}
		item, convErr = guardAndFinalizePlugin(item)
		if convErr != nil {
			writeCatalogPriceError(c, convErr)
			return
		}
		saved, upsertErr := currentSourceStationStore().UpsertPlugin(item, true)
		if upsertErr != nil {
			writeSourceDeveloperStoreError(c, upsertErr)
			return
		}
		if formFlag(c, "submit") && !shelf {
			saved, upsertErr = currentSourceStationStore().SetPluginStatus(saved.ID, sourceItemReview, actor, "package submit")
			if upsertErr != nil {
				writeSourceDeveloperStoreError(c, upsertErr)
				return
			}
		}
		if shelf {
			if pubErr := publishSourcePackageItem(sourceKindPlugin, saved.ID, saved.Version, actor); pubErr != nil {
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
		savedLocation = saved.DownloadURL
		savedSHA = saved.SHA256
	}

	detail := savedLocation
	if pushed {
		detail = provider + " " + savedLocation
	}
	_ = currentSourceStationStore().AppendAudit(sourceAuditEntry{
		ActorType: "admin", ActorName: actor, Action: "package_publish",
		TargetType: kind, TargetID: itemID + "@" + manifest.Version, Detail: truncateText(detail, 500),
	})
	hosted := isPrivatePackageRef(savedLocation)
	githubPaid := isGitHubPackageRef(savedLocation)
	data := gin.H{
		"kind": kind, "id": itemID, "version": manifest.Version, "sha256": savedSHA,
		"downloadUrl": savedLocation, "pushed": pushed, "storedPackage": hosted, "manifest": manifest.view(),
	}
	if kind == sourceKindTemplate {
		data["templateUrl"] = savedLocation
		delete(data, "downloadUrl")
		data["item"] = templateView
	} else {
		data["item"] = pluginView
	}
	msg := "校验通过，已保存为草稿（包已丢弃，源站不保存源码）。请走审核/上架"
	if githubPaid {
		msg = "校验通过，已核对私有仓库安装包并保存元数据，校验码已自动填写（压缩包未在本站保存）"
	} else if remoteURL != "" && hosted {
		msg = "校验通过，已拉取外链并私有托管，校验码已自动填写"
	} else if remoteURL != "" {
		msg = "校验通过，已保存元数据并自动填写校验码（安装包仍由外部地址提供）"
	}
	if pushed {
		msg = "校验通过，已推送到 " + provider + " Release 并保存为草稿（包已丢弃）"
	}
	if shelf && githubPaid {
		msg = "校验通过，已核对私有仓库安装包并上架，校验码已自动填写（压缩包未在本站保存）"
	} else if shelf && remoteURL != "" && hosted {
		msg = "校验通过，已拉取外链并私有托管后上架，校验码已自动填写"
	} else if shelf && remoteURL != "" && !hosted {
		msg = "校验通过，已保存并上架，校验码已按外部地址自动填写"
	} else if shelf {
		msg = "校验通过，已保存并上架（包已丢弃）"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg, "data": data})
}

func publishSourcePackageItem(kind, id, version, actor string) error {
	_ = version
	if kind == sourceKindTemplate {
		item, err := currentSourceStationStore().GetTemplate(id)
		if err != nil {
			return err
		}
		if item.Status == sourceItemPublished {
			return nil
		}
		if item.Status != sourceItemApproved && item.Status != sourceItemHidden {
			if _, err = currentSourceStationStore().SetTemplateStatus(id, sourceItemApproved, actor, "package auto-approve before shelf"); err != nil {
				return err
			}
		}
		_, err = currentSourceStationStore().SetTemplateStatus(id, sourceItemPublished, actor, "package publish")
		return err
	}
	item, err := currentSourceStationStore().GetPlugin(id)
	if err != nil {
		return err
	}
	if item.Status == sourceItemPublished {
		return nil
	}
	if item.Status != sourceItemApproved && item.Status != sourceItemHidden {
		if _, err = currentSourceStationStore().SetPluginStatus(id, sourceItemApproved, actor, "package auto-approve before shelf"); err != nil {
			return err
		}
	}
	_, err = currentSourceStationStore().SetPluginStatus(id, sourceItemPublished, actor, "package publish")
	return err
}

func discardSourceMultipart(c *gin.Context) {
	if c.Request != nil && c.Request.MultipartForm != nil {
		_ = c.Request.MultipartForm.RemoveAll()
	}
}
