package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type sourceAdApplicationRequest struct {
	AppID     int64    `json:"appId"`
	Title     string   `json:"title"`
	ImageURL  string   `json:"imageUrl"`
	LinkURL   string   `json:"linkUrl"`
	Positions []string `json:"positions"`
	Note      string   `json:"note"`
}

func SourceDeveloperCatalogCategories(c *gin.Context) {
	if _, err := currentSourceDeveloper(c); err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	list := resolveSourceCatalogCategories()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func SourceDeveloperAdApplications(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	writeSourceAdApplicationList(c, developer.ID, strings.TrimSpace(c.Query("status")))
}

func SourceDeveloperCreateAdApplication(c *gin.Context) {
	developer, err := currentSourceDeveloper(c)
	if err != nil {
		writeCurrentSourceDeveloperError(c, err)
		return
	}
	item, err := bindSourceAdApplicationDraft(c, developer.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	saved, err := currentSourceStationStore().CreateAdApplication(item)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "广告申请已提交，等待管理员审核", "data": sourceAdApplicationView(saved, developer.Username)})
}

func AdminSourceAdApplications(c *gin.Context) {
	writeSourceAdApplicationList(c, 0, strings.TrimSpace(c.Query("status")))
}

func AdminSourceAdApplicationApprove(c *gin.Context) {
	id, err := parseSourceApplicationID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "申请标识不合法"})
		return
	}
	item, err := currentSourceStationStore().GetAdApplication(id)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	if item.Status != sourceApplicationPending {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": errAdApplicationReviewed.Error()})
		return
	}
	record := advertisementRecord{
		Title:          item.Title,
		ImageURL:       item.ImageURL,
		DestinationURL: item.LinkURL,
		Positions:      item.Positions,
		Description:    item.Note,
	}
	if len(prepareAdvertisementForStore(&record)) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告位不合法，无法从申请创建投放"})
		return
	}
	generated, err := generateAdvertisementID()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "生成广告标识失败"})
		return
	}
	record.ID = generated
	if record.ImageURL != "" {
		if err := validateAdvertisementImageURL(record.ImageURL); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告图片须为本站上传地址或 https:// 外部地址"})
			return
		}
	}
	if record.DestinationURL != "" {
		if err := validateExternalHTTPS(record.DestinationURL); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "广告跳转必须是 https:// 外部地址"})
			return
		}
	}
	if err := currentSourceStationStore().UpsertAdvertisement(record); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "保存广告失败"})
		return
	}
	saved, err := currentSourceStationStore().SetAdApplicationStatus(id, sourceApplicationApproved, c.GetString("username"), sourceNoteFromBody(c), record.ID)
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	resetLocalAdvertisementCache()
	hydrateAdvertisementRecord(&record)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已通过申请并创建广告投放", "data": gin.H{
		"application":     sourceAdApplicationView(saved, sourceAdApplicationDeveloperName(saved.DeveloperID)),
		"advertisement":   record,
		"advertisementId": record.ID,
	}})
}

func AdminSourceAdApplicationReject(c *gin.Context) {
	id, err := parseSourceApplicationID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "申请标识不合法"})
		return
	}
	saved, err := currentSourceStationStore().SetAdApplicationStatus(id, sourceApplicationRejected, c.GetString("username"), sourceNoteFromBody(c), "")
	if err != nil {
		writeSourceDeveloperStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已拒绝广告申请", "data": sourceAdApplicationView(saved, sourceAdApplicationDeveloperName(saved.DeveloperID))})
}

func bindSourceAdApplicationDraft(c *gin.Context, developerID int64) (sourceAdApplication, error) {
	var req sourceAdApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return sourceAdApplication{}, errors.New("参数错误")
	}
	title := truncateText(strings.TrimSpace(req.Title), 120)
	if title == "" {
		return sourceAdApplication{}, errors.New("请填写广告标题")
	}
	slots := normalizeAdvertisementSlotList(req.Positions)
	if len(slots) == 0 {
		return sourceAdApplication{}, errors.New("请选择合法广告位（首页横幅 / 侧栏 / 弹窗）")
	}
	imageURL := strings.TrimSpace(req.ImageURL)
	if imageURL != "" {
		if err := validateAdvertisementImageURL(imageURL); err != nil {
			return sourceAdApplication{}, err
		}
	}
	linkURL := strings.TrimSpace(req.LinkURL)
	if linkURL != "" {
		if err := validateExternalHTTPS(linkURL); err != nil {
			return sourceAdApplication{}, err
		}
	}
	appID := req.AppID
	if appID > 0 {
		if _, err := requireSourceCatalogApp(appID); err != nil {
			return sourceAdApplication{}, err
		}
	}
	return sourceAdApplication{
		DeveloperID: developerID,
		AppID:       appID,
		Title:       title,
		ImageURL:    truncateText(imageURL, 500),
		LinkURL:     truncateText(linkURL, 500),
		Positions:   slots,
		Note:        truncateText(strings.TrimSpace(req.Note), 500),
		Status:      sourceApplicationPending,
	}, nil
}

func writeSourceAdApplicationList(c *gin.Context, developerID int64, status string) {
	if status != "" && status != sourceApplicationPending && status != sourceApplicationApproved && status != sourceApplicationRejected {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "状态不合法"})
		return
	}
	items, err := currentSourceStationStore().ListAdApplications(developerID, status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取广告申请失败"})
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, sourceAdApplicationView(item, sourceAdApplicationDeveloperName(item.DeveloperID)))
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": len(list)}})
}

func sourceAdApplicationView(item sourceAdApplication, username string) gin.H {
	reviewedAt := ""
	if item.ReviewedAt != nil {
		reviewedAt = item.ReviewedAt.Format(time.RFC3339)
	}
	positions := item.Positions
	if positions == nil {
		positions = []string{}
	}
	return gin.H{
		"id": item.ID, "developerId": item.DeveloperID, "developerUsername": username, "appId": item.AppID,
		"title": item.Title, "imageUrl": item.ImageURL, "linkUrl": item.LinkURL, "positions": positions,
		"note": item.Note, "status": item.Status, "reviewNote": item.ReviewNote, "reviewedBy": item.ReviewedBy,
		"reviewedAt": reviewedAt, "advertisementId": item.AdvertisementID,
		"createdAt": item.CreatedAt.Format(time.RFC3339),
		"updatedAt": item.UpdatedAt.Format(time.RFC3339),
	}
}

func sourceAdApplicationDeveloperName(developerID int64) string {
	if developerID <= 0 {
		return ""
	}
	developer, err := currentSourceStationStore().GetDeveloperByID(developerID)
	if err != nil {
		return ""
	}
	return developer.Username
}
