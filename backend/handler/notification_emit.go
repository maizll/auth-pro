package handler

import (
	"fmt"
	"strconv"
	"strings"
)

// Review notices share one writer. Audience is either every admin, or the
// applicant binding (agent id + developer id) so both the agent panel and the
// developer panel lists can see it. Add a key here instead of a new helper.
const (
	reviewEventDeveloperApplySubmitted       = "developer_apply_submitted"
	reviewEventDeveloperApplyApproved        = "developer_apply_approved"
	reviewEventDeveloperApplyRejected        = "developer_apply_rejected"
	reviewEventDeveloperQualificationRevoked = "developer_qualification_revoked"
	reviewEventCatalogSubmitted              = "catalog_item_submitted"
	reviewEventCatalogApproved               = "catalog_item_approved"
	reviewEventCatalogRejected               = "catalog_item_rejected"
	reviewEventCatalogDeprecated             = "catalog_item_deprecated"
)

type reviewNoticeAudience struct {
	AgentID     int64
	DeveloperID int64
}

func emitReviewNotice(admins bool, who reviewNoticeAudience, category, title, body, link, event, refType, refID string) {
	if admins {
		notifyAllAdmins(category, title, body, link, event, refType, refID)
		return
	}
	notifyDeveloperBinding(who.AgentID, who.DeveloperID, category, title, body, link, event, refType, refID)
}

func notifyDeveloperApplySubmitted(app sourceApplication) {
	name := sourceFirstNonEmpty(app.DisplayName, app.Username, "代理商")
	emitReviewNotice(true, reviewNoticeAudience{}, notificationTabNotice,
		"新的开发者入驻申请",
		name+" 提交了入驻申请，请审核",
		"/source-station/applications",
		reviewEventDeveloperApplySubmitted,
		"developer_application",
		itoaSourceID(app.ID),
	)
}

func notifyDeveloperApplyReviewed(app sourceApplication, developer sourceDeveloper, approved bool, note string) {
	agentID := app.AgentID
	developerID := developer.ID
	if agentID <= 0 {
		agentID = developer.AgentID
	}
	title := "开发者入驻已通过"
	body := "管理员已通过你的入驻申请，可进入开发者工作台"
	event := reviewEventDeveloperApplyApproved
	if !approved {
		title = "开发者入驻未通过"
		body = "入驻申请未通过"
		if strings.TrimSpace(note) != "" {
			body += "：" + truncateText(note, 80)
		}
		event = reviewEventDeveloperApplyRejected
	}
	emitReviewNotice(false, reviewNoticeAudience{AgentID: agentID, DeveloperID: developerID}, notificationTabMessage,
		title, body, "/agent-panel/become-developer", event, "developer_application", itoaSourceID(app.ID))
}

func notifyDeveloperQualificationRevoked(dev sourceDeveloper, note string) {
	if dev.AgentID <= 0 && dev.ID <= 0 {
		return
	}
	body := "管理员已取消你的开发者身份，入驻资格已撤销，可重新申请"
	if strings.TrimSpace(note) != "" {
		body += "：" + truncateText(note, 80)
	}
	emitReviewNotice(false, reviewNoticeAudience{AgentID: dev.AgentID, DeveloperID: dev.ID}, notificationTabMessage,
		"开发者身份已取消", body, "/agent-panel/become-developer",
		reviewEventDeveloperQualificationRevoked, "developer", itoaSourceID(dev.ID))
}

func notifyCatalogSubmitted(kind, itemID, name string) {
	label := "插件"
	if kind == sourceKindTemplate {
		label = "模板"
	}
	if strings.TrimSpace(name) == "" {
		name = itemID
	}
	emitReviewNotice(true, reviewNoticeAudience{}, notificationTabNotice,
		"有新的"+label+"待审核",
		name+" 已提交审核",
		"/source-station/catalog",
		reviewEventCatalogSubmitted,
		kind,
		itemID,
	)
}

func notifyCatalogReviewed(developerID int64, kind, itemID, name, status string) {
	if developerID <= 0 {
		return
	}
	label := "插件"
	link := "/developer-panel/plugins"
	if kind == sourceKindTemplate {
		label = "模板"
		link = "/developer-panel/templates"
	}
	if strings.TrimSpace(name) == "" {
		name = itemID
	}
	title, body, event := "", "", ""
	switch status {
	case sourceItemApproved:
		title, body, event = label+"审核已通过", name+" 已通过审核，可继续上架", reviewEventCatalogApproved
	case sourceItemRejected:
		title, body, event = label+"审核未通过", name+" 未通过审核，请修改后再次提交", reviewEventCatalogRejected
	case sourceItemDeprecated:
		title, body, event = label+"已被标记弃用", name+" 已从公开目录清除，请查看审核说明", reviewEventCatalogDeprecated
	default:
		return
	}
	agentID := int64(0)
	if dev, err := currentSourceStationStore().GetDeveloperByID(developerID); err == nil {
		agentID = dev.AgentID
	}
	emitReviewNotice(false, reviewNoticeAudience{AgentID: agentID, DeveloperID: developerID}, notificationTabMessage,
		title, body, link, event, kind, itemID)
}

func notifyCatalogVersionSubmitted(kind, itemID, version string) {
	label := "插件版本"
	if kind == sourceKindTemplate {
		label = "模板版本"
	}
	notifyAllAdmins(
		notificationTabNotice,
		"有新的"+label+"待审核",
		itemID+" "+version+" 已提交审核",
		"/source-station/catalog",
		"catalog_version_submitted",
		kind,
		itemID+":"+version,
	)
}

func notifyCatalogVersionReviewed(developerID int64, kind, itemID, version, status string) {
	if developerID <= 0 {
		return
	}
	label := "插件版本"
	link := "/developer-panel/plugins"
	if kind == sourceKindTemplate {
		label = "模板版本"
		link = "/developer-panel/templates"
	}
	title, body, event := "", "", ""
	switch status {
	case sourceVersionPublished:
		title, body, event = label+"已通过", itemID+" "+version+" 已发布", "catalog_version_approved"
	case sourceVersionDraft:
		title, body, event = label+"未通过", itemID+" "+version+" 已驳回为草稿", "catalog_version_rejected"
	case sourceVersionDeprecated:
		title, body, event = label+"已弃用", itemID+" "+version+" 已被标记弃用", "catalog_version_rejected"
	default:
		return
	}
	notifyDeveloperOwner(developerID, notificationTabMessage, title, body, link, event, kind, itemID+":"+version)
}

func notifyAdApplicationSubmitted(item sourceAdApplication) {
	notifyAllAdmins(
		notificationTabNotice,
		"有新的广告投放申请",
		sourceFirstNonEmpty(item.Title, "未命名广告")+" 待审核",
		"/source-station/ads",
		"ad_application_submitted",
		"ad_application",
		itoaSourceID(item.ID),
	)
}

func notifyAdApplicationReviewed(item sourceAdApplication, approved bool) {
	title := "广告申请已通过"
	body := item.Title + " 已通过并创建投放"
	event := "ad_application_approved"
	if !approved {
		title = "广告申请未通过"
		body = item.Title + " 未通过审核"
		event = "ad_application_rejected"
	}
	notifyDeveloperOwner(item.DeveloperID, notificationTabMessage, title, body, "/developer-panel/ads", event, "ad_application", itoaSourceID(item.ID))
}

func notifyPasswordChanged(role string, userID int64) {
	link := "/user/profile"
	switch role {
	case notificationRoleAdmin:
		link = "/"
	case notificationRoleAgent:
		link = "/agent-panel/profile"
	}
	notifyRole(role, userID, notificationTabNotice, "密码已修改", "若不是你本人操作，请立即联系管理员", link, "password_changed", role, itoaSourceID(userID))
}

func notifyTicketCreated(ticketID int64, ticketNo, title, creatorType, creatorName string) {
	notifyAllAdmins(
		notificationTabNotice,
		"新的工单待处理",
		sourceFirstNonEmpty(creatorName, creatorType)+" 提交了工单 "+sourceFirstNonEmpty(ticketNo, itoaSourceID(ticketID))+"："+title,
		"/tickets",
		"ticket_created",
		"ticket",
		itoaSourceID(ticketID),
	)
}

func notifyTicketUserReply(ticketID int64, ticketNo, title string) {
	notifyAllAdmins(
		notificationTabNotice,
		"工单有新回复",
		"工单 "+sourceFirstNonEmpty(ticketNo, itoaSourceID(ticketID))+" 等待处理："+title,
		"/tickets",
		"ticket_user_reply",
		"ticket",
		itoaSourceID(ticketID),
	)
}

func notifyTicketStaffReply(creatorType string, creatorID, ticketID int64, ticketNo, title string) {
	link := "/user/tickets"
	if creatorType == notificationRoleAgent {
		link = "/agent-panel/tickets"
	}
	notifyOwner(
		creatorType, creatorID, notificationTabMessage,
		"工单有新回复",
		"客服回复了工单 "+sourceFirstNonEmpty(ticketNo, itoaSourceID(ticketID))+"："+title,
		link, "ticket_reply", "ticket", itoaSourceID(ticketID),
	)
}

func notifyLicenseActivated(ownerType string, ownerID, agentID int64, licenseNo, appName string) {
	title := "授权已开通"
	body := sourceFirstNonEmpty(appName, "应用") + " 授权 " + licenseNo + " 已生效"
	if ownerType == notificationRoleUser {
		notifyRole(notificationRoleUser, ownerID, notificationTabMessage, title, body, "/user/licenses", "license_activated", "license", licenseNo)
	}
	if ownerType == notificationRoleAgent {
		notifyRole(notificationRoleAgent, ownerID, notificationTabMessage, title, body, "/agent-panel/licenses", "license_activated", "license", licenseNo)
	}
	if agentID > 0 && (ownerType != notificationRoleAgent || ownerID != agentID) {
		notifyRole(notificationRoleAgent, agentID, notificationTabMessage,
			"下级授权已开通",
			"用户授权 "+licenseNo+"（"+sourceFirstNonEmpty(appName, "应用")+"）已开通",
			"/agent-panel/licenses", "subordinate_license_issued", "license", licenseNo)
	}
}

func notifyOrderPaid(ownerType string, ownerID int64, orderNo, summary string) {
	link := "/user/purchase"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/finance"
	}
	notifyOwner(ownerType, ownerID, notificationTabMessage, "订单已支付", sourceFirstNonEmpty(summary, "订单 "+orderNo+" 已到账"), link, "order_paid", "order", orderNo)
}

func notifyBalanceChanged(ownerType string, ownerID int64, summary string) {
	link := "/user/dashboard"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/finance"
	}
	notifyOwner(ownerType, ownerID, notificationTabMessage, "账户余额变动", summary, link, "balance_changed", ownerType, itoaSourceID(ownerID))
}

func notifyAgentLevelChanged(agentID int64, level string) {
	notifyRole(notificationRoleAgent, agentID, notificationTabMessage, "代理等级已变更", "当前等级："+sourceFirstNonEmpty(level, "未命名"), "/agent-panel/profile", "agent_level_changed", "agent", itoaSourceID(agentID))
}

func notifyRealnameResult(ownerType string, ownerID int64, passed bool, reason string) {
	title := "实名认证已通过"
	body := "实名信息已审核通过"
	event := "realname_approved"
	if !passed {
		title = "实名认证未通过"
		body = "实名认证失败"
		if strings.TrimSpace(reason) != "" {
			body += "：" + truncateText(reason, 80)
		}
		event = "realname_rejected"
	}
	link := "/user/profile"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/profile"
	}
	notifyOwner(ownerType, ownerID, notificationTabMessage, title, body, link, event, "realname", itoaSourceID(ownerID))
}

func notifyLicenseExpiring(ownerType string, ownerID int64, licenseNo, appName string, days int) {
	body := fmt.Sprintf("%s 授权 %s 将在 %d 天后到期", sourceFirstNonEmpty(appName, "应用"), licenseNo, days)
	link := "/user/licenses"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/licenses"
	}
	notifyOwner(ownerType, ownerID, notificationTabNotice, "授权即将到期", body, link, "license_expiring", "license", licenseNo)
}

// Remaining product events. These helpers are intentionally unused until the
// domain handlers grow a real write path; call them instead of inventing a
// second notification table.

// TODO(v1.4.x): hook when withdraw / settlement writes a ledger row.
func notifyWithdrawResult(ownerType string, ownerID int64, amount, status, refID string) {
	title := "提现/结算有更新"
	body := sourceFirstNonEmpty(amount, "一笔提现") + " 状态：" + sourceFirstNonEmpty(status, "已更新")
	link := "/user/dashboard"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/finance"
	}
	notifyOwner(ownerType, ownerID, notificationTabMessage, title, body, link, "withdraw_updated", "withdraw", refID)
}

// TODO(v1.4.x): hook when subordinate commission is credited to an agent.
func notifyCommissionCredited(agentID int64, amount, refID string) {
	notifyRole(notificationRoleAgent, agentID, notificationTabMessage,
		"下级分佣到账",
		sourceFirstNonEmpty(amount, "一笔分佣")+" 已计入账户",
		"/agent-panel/finance", "commission_credited", "commission", refID)
}

// TODO(v1.4.x): hook a persisted license-expired write event. Expiry is currently
// computed at read time (expired_at <= NOW), so there is no transition to emit.
func notifyLicenseExpired(ownerType string, ownerID int64, licenseNo, appName string) {
	body := sourceFirstNonEmpty(appName, "应用") + " 授权 " + licenseNo + " 已过期"
	link := "/user/licenses"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/licenses"
	}
	notifyOwner(ownerType, ownerID, notificationTabNotice, "授权已过期", body, link, "license_expired", "license", licenseNo)
}

// TODO(v1.4.x): call from LicenseToggle / LicenseDelete after loading owner_type,
// owner_id, license_no. Toggle currently only updates status and has no owner query.
func notifyLicenseStatusChanged(ownerType string, ownerID int64, licenseNo, appName, status string) {
	title := "授权状态已变更"
	body := sourceFirstNonEmpty(appName, "应用") + " 授权 " + licenseNo + " 现为 " + sourceFirstNonEmpty(status, "已更新")
	link := "/user/licenses"
	if ownerType == notificationRoleAgent {
		link = "/agent-panel/licenses"
	}
	notifyOwner(ownerType, ownerID, notificationTabNotice, title, body, link, "license_status_changed", "license", licenseNo)
}

func catalogOwnerDeveloperID(kind, itemID string) int64 {
	if kind == sourceKindTemplate {
		if item, err := currentSourceStationStore().GetTemplate(itemID); err == nil {
			return item.DeveloperID
		}
		return 0
	}
	if item, err := currentSourceStationStore().GetPlugin(itemID); err == nil {
		return item.DeveloperID
	}
	return 0
}

func parseNotificationInt64(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func notificationContextUserID(value any) int64 {
	switch typed := value.(type) {
	case uint:
		return int64(typed)
	case uint64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}
