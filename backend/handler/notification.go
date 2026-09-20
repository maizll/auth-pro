package handler

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

const (
	notificationRoleAdmin     = "admin"
	notificationRoleAgent     = "agent"
	notificationRoleUser      = "user"
	notificationRoleDeveloper = "developer"

	notificationTabNotice  = "notice"
	notificationTabMessage = "message"
	notificationTabTodo    = "todo"

	notificationListLimit = 50
)

// Tab mapping (stable for all four roles):
//
//	notice  = 系统/运营/安全事件（入驻提审、目录提审、新工单、改密）
//	message = 对人结果（审核结论、工单回复、订单/充值、等级变更、实名结果）
//	todo    = 当前身份可处理的待办（管理员：待审入驻/目录/广告/工单；用户/代理：待回复工单）
var (
	notificationStoreOverride   notificationStore
	notificationStoreOverrideMu sync.RWMutex
)

type inAppNotification struct {
	ID          int64
	UserRole    string
	UserID      int64
	AgentID     int64
	DeveloperID int64
	Category    string
	Title       string
	Body        string
	Link        string
	ReadAt      *time.Time
	CreatedAt   time.Time
	EventType   string
	RefType     string
	RefID       string
}

type notificationRecipient struct {
	Role        string
	UserID      int64
	AgentID     int64
	DeveloperID int64
}

type notificationListFilter struct {
	notificationRecipient
	Category string
	Unread   bool
	Limit    int
}

type notificationStore interface {
	Ensure() error
	Insert(item inAppNotification) (inAppNotification, error)
	List(filter notificationListFilter) ([]inAppNotification, error)
	UnreadCount(recipient notificationRecipient) (int, error)
	MarkRead(id int64, recipient notificationRecipient) (inAppNotification, error)
	MarkAllRead(recipient notificationRecipient) (int, error)
	ListAdminIDs() ([]int64, error)
}

func SetNotificationStoreForTest(store notificationStore) func() {
	notificationStoreOverrideMu.Lock()
	previous := notificationStoreOverride
	notificationStoreOverride = store
	notificationStoreOverrideMu.Unlock()
	return func() {
		notificationStoreOverrideMu.Lock()
		notificationStoreOverride = previous
		notificationStoreOverrideMu.Unlock()
	}
}

func currentNotificationStore() notificationStore {
	notificationStoreOverrideMu.RLock()
	override := notificationStoreOverride
	notificationStoreOverrideMu.RUnlock()
	if override != nil {
		return override
	}
	return mysqlNotificationStore{}
}

func EnsureNotificationSchema() {
	_ = currentNotificationStore().Ensure()
}

func RegisterNotificationRoutes(api *gin.RouterGroup) {
	group := api.Group("/v1/notifications")
	group.Use(middleware.JWTAuth())
	{
		group.GET("", ListNotifications)
		group.GET("/unread-count", NotificationUnreadCount)
		group.POST("/read-all", MarkAllNotificationsRead)
		group.POST("/:id/read", MarkNotificationRead)
	}
}

func ListNotifications(c *gin.Context) {
	recipient, ok := currentNotificationRecipient(c)
	if !ok {
		return
	}
	tab := strings.TrimSpace(c.Query("tab"))
	if tab != "" && tab != notificationTabNotice && tab != notificationTabMessage && tab != notificationTabTodo {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "tab 仅支持 notice、message、todo"})
		return
	}
	if tab == notificationTabTodo {
		todos := derivedNotificationTodos(recipient)
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
			"list": notificationViews(todos), "total": len(todos),
		}})
		return
	}
	unread := strings.TrimSpace(c.Query("unread")) == "1" || strings.EqualFold(c.Query("unread"), "true")
	items, err := currentNotificationStore().List(notificationListFilter{
		notificationRecipient: recipient,
		Category:              tab,
		Unread:                unread,
		Limit:                 notificationListLimit,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取通知失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"list": notificationViews(items), "total": len(items),
	}})
}

func NotificationUnreadCount(c *gin.Context) {
	recipient, ok := currentNotificationRecipient(c)
	if !ok {
		return
	}
	count, err := currentNotificationStore().UnreadCount(recipient)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "读取未读数失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"count": count,
		"todo":  len(derivedNotificationTodos(recipient)),
	}})
}

func MarkNotificationRead(c *gin.Context) {
	recipient, ok := currentNotificationRecipient(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "通知标识不合法"})
		return
	}
	item, err := currentNotificationStore().MarkRead(id, recipient)
	if err != nil {
		if err == errNotificationNotFound {
			c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "通知不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "标记已读失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已读", "data": notificationView(item)})
}

func MarkAllNotificationsRead(c *gin.Context) {
	recipient, ok := currentNotificationRecipient(c)
	if !ok {
		return
	}
	n, err := currentNotificationStore().MarkAllRead(recipient)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "全部已读失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已全部标为已读", "data": gin.H{"updated": n}})
}

func currentNotificationRecipient(c *gin.Context) (notificationRecipient, bool) {
	role := c.GetString("role")
	userID := int64(c.GetUint("user_id"))
	if userID <= 0 || (role != notificationRoleAdmin && role != notificationRoleAgent && role != notificationRoleUser && role != notificationRoleDeveloper) {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问通知"})
		return notificationRecipient{}, false
	}
	recipient := notificationRecipient{Role: role, UserID: userID}
	if role == notificationRoleAgent {
		recipient.AgentID = userID
		if dev, err := currentSourceStationStore().GetDeveloperByAgentID(userID); err == nil {
			recipient.DeveloperID = dev.ID
		}
	}
	if role == notificationRoleDeveloper {
		recipient.DeveloperID = userID
		if dev, err := currentSourceStationStore().GetDeveloperByID(userID); err == nil {
			recipient.AgentID = dev.AgentID
			recipient.DeveloperID = dev.ID
		}
	}
	return recipient, true
}

func (r notificationRecipient) matches(item inAppNotification) bool {
	switch r.Role {
	case notificationRoleAdmin:
		return item.UserRole == notificationRoleAdmin && item.UserID == r.UserID
	case notificationRoleUser:
		return item.UserRole == notificationRoleUser && item.UserID == r.UserID
	case notificationRoleAgent:
		if item.UserRole == notificationRoleAgent && item.UserID == r.UserID {
			return true
		}
		return item.UserRole == notificationRoleDeveloper && (item.AgentID == r.UserID || item.UserID == r.UserID)
	case notificationRoleDeveloper:
		if item.UserRole != notificationRoleDeveloper {
			return false
		}
		if item.UserID == r.UserID || item.DeveloperID == r.UserID {
			return true
		}
		if r.DeveloperID > 0 && item.DeveloperID == r.DeveloperID {
			return true
		}
		return r.AgentID > 0 && item.AgentID == r.AgentID
	default:
		return false
	}
}

func notificationViews(items []inAppNotification) []gin.H {
	list := make([]gin.H, 0, len(items))
	for _, item := range items {
		list = append(list, notificationView(item))
	}
	return list
}

func notificationView(item inAppNotification) gin.H {
	read := item.ReadAt != nil
	return gin.H{
		"id":        item.ID,
		"category":  item.Category,
		"title":     item.Title,
		"body":      item.Body,
		"link":      item.Link,
		"read":      read,
		"createdAt": item.CreatedAt.UTC().Format(time.RFC3339),
		"eventType": item.EventType,
		"refType":   item.RefType,
		"refId":     item.RefID,
		"derived":   item.ID == 0,
	}
}

func derivedNotificationTodos(recipient notificationRecipient) []inAppNotification {
	switch recipient.Role {
	case notificationRoleAdmin:
		return derivedAdminTodos()
	case notificationRoleUser, notificationRoleAgent:
		return derivedPanelTicketTodos(recipient)
	default:
		return nil
	}
}

func derivedAdminTodos() []inAppNotification {
	now := time.Now().UTC()
	var items []inAppNotification
	if apps, err := currentSourceStationStore().ListApplications(sourceApplicationPending); err == nil && len(apps) > 0 {
		items = append(items, inAppNotification{
			Category: notificationTabTodo, Title: itoaSourceID(int64(len(apps))) + " 条开发者入驻待审核",
			Body: "请在源站「入驻审核」处理", Link: "/source-station/applications",
			EventType: "todo_developer_apply", RefType: "developer_application", CreatedAt: now,
		})
	}
	reviewCount := 0
	if plugins, err := currentSourceStationStore().ListPlugins(sourceItemReview); err == nil {
		reviewCount += len(plugins)
	}
	if templates, err := currentSourceStationStore().ListTemplates(sourceItemReview); err == nil {
		reviewCount += len(templates)
	}
	if reviewCount > 0 {
		items = append(items, inAppNotification{
			Category: notificationTabTodo, Title: itoaSourceID(int64(reviewCount)) + " 条插件/模板待审核",
			Body: "请在源站「软件目录」处理", Link: "/source-station/catalog",
			EventType: "todo_catalog_review", RefType: "catalog_item", CreatedAt: now,
		})
	}
	if ads, err := currentSourceStationStore().ListAdApplications(0, sourceApplicationPending); err == nil && len(ads) > 0 {
		items = append(items, inAppNotification{
			Category: notificationTabTodo, Title: itoaSourceID(int64(len(ads))) + " 条广告申请待审核",
			Body: "请在源站「广告投放」处理", Link: "/source-station/ads",
			EventType: "todo_ad_application", RefType: "ad_application", CreatedAt: now,
		})
	}
	return items
}

func derivedPanelTicketTodos(recipient notificationRecipient) []inAppNotification {
	db, err := config.DB()
	if err != nil {
		return nil
	}
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM tickets
		WHERE creator_type = ? AND creator_id = ? AND status = ?
	`, recipient.Role, recipient.UserID, ticketStatusReplied).Scan(&count)
	if err != nil || count == 0 {
		return nil
	}
	link := "/user/tickets"
	if recipient.Role == notificationRoleAgent {
		link = "/agent-panel/tickets"
	}
	return []inAppNotification{{
		Category: notificationTabTodo, Title: itoaSourceID(int64(count)) + " 条工单待查看回复",
		Body: "客服已回复，请到「我的工单」继续处理", Link: link,
		EventType: "todo_ticket_reply", RefType: "ticket", CreatedAt: time.Now().UTC(),
	}}
}

var errNotificationNotFound = errSourceNotFound

func emitNotification(item inAppNotification) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("in-app notification emit panic: %v", recovered)
		}
	}()
	if item.UserID <= 0 || strings.TrimSpace(item.UserRole) == "" || strings.TrimSpace(item.Title) == "" {
		return
	}
	if item.Category == "" {
		item.Category = notificationTabNotice
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	item.Title = truncateText(item.Title, 120)
	item.Body = truncateText(item.Body, 500)
	item.Link = truncateText(item.Link, 300)
	item.EventType = truncateText(item.EventType, 60)
	item.RefType = truncateText(item.RefType, 40)
	item.RefID = truncateText(item.RefID, 80)
	if _, err := currentNotificationStore().Insert(item); err != nil {
		if !notificationStoreUnavailable(err) {
			log.Printf("in-app notification emit failed: %v", err)
		}
	}
}

func notificationStoreUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "db.json") || strings.Contains(msg, "no such file")
}

func notifyRole(role string, userID int64, category, title, body, link, eventType, refType, refID string) {
	item := inAppNotification{
		UserRole: role, UserID: userID, Category: category,
		Title: title, Body: body, Link: link,
		EventType: eventType, RefType: refType, RefID: refID,
	}
	if role == notificationRoleAgent {
		item.AgentID = userID
	}
	if role == notificationRoleDeveloper {
		item.DeveloperID = userID
	}
	emitNotification(item)
}

func notifyAllAdmins(category, title, body, link, eventType, refType, refID string) {
	ids, err := currentNotificationStore().ListAdminIDs()
	if err != nil || len(ids) == 0 {
		ids = []int64{1}
	}
	for _, id := range ids {
		notifyRole(notificationRoleAdmin, id, category, title, body, link, eventType, refType, refID)
	}
}

func notifyDeveloperBinding(agentID, developerID int64, category, title, body, link, eventType, refType, refID string) {
	userID := agentID
	if userID <= 0 {
		userID = developerID
	}
	if userID <= 0 {
		return
	}
	emitNotification(inAppNotification{
		UserRole: notificationRoleDeveloper, UserID: userID, AgentID: agentID, DeveloperID: developerID,
		Category: category, Title: title, Body: body, Link: link,
		EventType: eventType, RefType: refType, RefID: refID,
	})
}

func notifyDeveloperOwner(developerID int64, category, title, body, link, eventType, refType, refID string) {
	if developerID <= 0 {
		return
	}
	agentID := int64(0)
	if dev, err := currentSourceStationStore().GetDeveloperByID(developerID); err == nil {
		agentID = dev.AgentID
		if developerID == 0 {
			developerID = dev.ID
		}
	}
	notifyDeveloperBinding(agentID, developerID, category, title, body, link, eventType, refType, refID)
}

func notifyOwner(ownerType string, ownerID int64, category, title, body, link, eventType, refType, refID string) {
	switch ownerType {
	case notificationRoleUser, notificationRoleAgent:
		notifyRole(ownerType, ownerID, category, title, body, link, eventType, refType, refID)
	}
}

type memoryNotificationStore struct {
	mu       sync.Mutex
	nextID   atomic.Int64
	items    []inAppNotification
	adminIDs []int64
}

func newMemoryNotificationStore() *memoryNotificationStore {
	store := &memoryNotificationStore{adminIDs: []int64{1}}
	store.nextID.Store(1)
	return store
}

func (store *memoryNotificationStore) Ensure() error { return nil }

func (store *memoryNotificationStore) Insert(item inAppNotification) (inAppNotification, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	item.ID = store.nextID.Add(1) - 1
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	store.items = append(store.items, item)
	return item, nil
}

func (store *memoryNotificationStore) List(filter notificationListFilter) ([]inAppNotification, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = notificationListLimit
	}
	out := make([]inAppNotification, 0)
	for i := len(store.items) - 1; i >= 0; i-- {
		item := store.items[i]
		if !filter.matches(item) {
			continue
		}
		if filter.Category != "" && item.Category != filter.Category {
			continue
		}
		if filter.Unread && item.ReadAt != nil {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (store *memoryNotificationStore) UnreadCount(recipient notificationRecipient) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	count := 0
	for _, item := range store.items {
		if recipient.matches(item) && item.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (store *memoryNotificationStore) MarkRead(id int64, recipient notificationRecipient) (inAppNotification, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for i := range store.items {
		if store.items[i].ID != id || !recipient.matches(store.items[i]) {
			continue
		}
		if store.items[i].ReadAt == nil {
			now := time.Now().UTC()
			store.items[i].ReadAt = &now
		}
		return store.items[i], nil
	}
	return inAppNotification{}, errNotificationNotFound
}

func (store *memoryNotificationStore) MarkAllRead(recipient notificationRecipient) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now().UTC()
	updated := 0
	for i := range store.items {
		if recipient.matches(store.items[i]) && store.items[i].ReadAt == nil {
			store.items[i].ReadAt = &now
			updated++
		}
	}
	return updated, nil
}

func (store *memoryNotificationStore) ListAdminIDs() ([]int64, error) {
	return append([]int64(nil), store.adminIDs...), nil
}

type mysqlNotificationStore struct{}

func (mysqlNotificationStore) Ensure() error {
	db, err := config.DB()
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			user_role VARCHAR(20) NOT NULL COMMENT 'admin/agent/user/developer',
			user_id BIGINT NOT NULL DEFAULT 0 COMMENT '当前身份主键',
			agent_id BIGINT NOT NULL DEFAULT 0 COMMENT '代理商绑定，开发者通知用',
			developer_id BIGINT NOT NULL DEFAULT 0 COMMENT '开发者绑定',
			category VARCHAR(20) NOT NULL DEFAULT 'notice' COMMENT 'notice/message/todo',
			title VARCHAR(120) NOT NULL DEFAULT '',
			body VARCHAR(500) NOT NULL DEFAULT '',
			link VARCHAR(300) NOT NULL DEFAULT '',
			read_at DATETIME DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			event_type VARCHAR(60) NOT NULL DEFAULT '',
			ref_type VARCHAR(40) NOT NULL DEFAULT '',
			ref_id VARCHAR(80) NOT NULL DEFAULT '',
			PRIMARY KEY (id),
			KEY idx_notifications_inbox (user_role, user_id, read_at, created_at),
			KEY idx_notifications_agent (agent_id, user_role, read_at),
			KEY idx_notifications_developer (developer_id, user_role, read_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='站内通知（按登录身份隔离）'
	`)
	return err
}

func (mysqlNotificationStore) Insert(item inAppNotification) (inAppNotification, error) {
	db, err := config.DB()
	if err != nil {
		return inAppNotification{}, err
	}
	if err := (mysqlNotificationStore{}).Ensure(); err != nil {
		return inAppNotification{}, err
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	result, err := db.Exec(`
		INSERT INTO notifications
			(user_role, user_id, agent_id, developer_id, category, title, body, link, created_at, event_type, ref_type, ref_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.UserRole, item.UserID, item.AgentID, item.DeveloperID, item.Category, item.Title, item.Body, item.Link,
		item.CreatedAt, item.EventType, item.RefType, item.RefID)
	if err != nil {
		return inAppNotification{}, err
	}
	item.ID, _ = result.LastInsertId()
	return item, nil
}

func notificationSQLScope(recipient notificationRecipient) (string, []any) {
	switch recipient.Role {
	case notificationRoleAdmin, notificationRoleUser:
		return "(user_role = ? AND user_id = ?)", []any{recipient.Role, recipient.UserID}
	case notificationRoleAgent:
		return "((user_role = 'agent' AND user_id = ?) OR (user_role = 'developer' AND (agent_id = ? OR user_id = ?)))",
			[]any{recipient.UserID, recipient.UserID, recipient.UserID}
	case notificationRoleDeveloper:
		return "(user_role = 'developer' AND (user_id = ? OR developer_id = ? OR (? > 0 AND agent_id = ?) OR (? > 0 AND developer_id = ?)))",
			[]any{recipient.UserID, recipient.UserID, recipient.AgentID, recipient.AgentID, recipient.DeveloperID, recipient.DeveloperID}
	default:
		return "1=0", nil
	}
}

func (mysqlNotificationStore) List(filter notificationListFilter) ([]inAppNotification, error) {
	db, err := config.DB()
	if err != nil {
		return nil, err
	}
	if err := (mysqlNotificationStore{}).Ensure(); err != nil {
		return nil, err
	}
	scope, args := notificationSQLScope(filter.notificationRecipient)
	query := `SELECT id, user_role, user_id, agent_id, developer_id, category, title, body, link, read_at, created_at, event_type, ref_type, ref_id
		FROM notifications WHERE ` + scope
	if filter.Category != "" {
		query += " AND category = ?"
		args = append(args, filter.Category)
	}
	if filter.Unread {
		query += " AND read_at IS NULL"
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = notificationListLimit
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]inAppNotification, 0)
	for rows.Next() {
		item, scanErr := scanNotificationRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (mysqlNotificationStore) UnreadCount(recipient notificationRecipient) (int, error) {
	db, err := config.DB()
	if err != nil {
		return 0, err
	}
	if err := (mysqlNotificationStore{}).Ensure(); err != nil {
		return 0, err
	}
	scope, args := notificationSQLScope(recipient)
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE `+scope+` AND read_at IS NULL`, args...).Scan(&count)
	return count, err
}

func (mysqlNotificationStore) MarkRead(id int64, recipient notificationRecipient) (inAppNotification, error) {
	db, err := config.DB()
	if err != nil {
		return inAppNotification{}, err
	}
	if err := (mysqlNotificationStore{}).Ensure(); err != nil {
		return inAppNotification{}, err
	}
	scope, args := notificationSQLScope(recipient)
	args = append([]any{id}, args...)
	var item inAppNotification
	item, err = scanNotificationRow(db.QueryRow(`
		SELECT id, user_role, user_id, agent_id, developer_id, category, title, body, link, read_at, created_at, event_type, ref_type, ref_id
		FROM notifications WHERE id = ? AND `+scope, args...))
	if err != nil {
		if err == sql.ErrNoRows {
			return inAppNotification{}, errNotificationNotFound
		}
		return inAppNotification{}, err
	}
	if item.ReadAt == nil {
		now := time.Now().UTC()
		if _, err := db.Exec(`UPDATE notifications SET read_at = ? WHERE id = ?`, now, id); err != nil {
			return inAppNotification{}, err
		}
		item.ReadAt = &now
	}
	return item, nil
}

func (mysqlNotificationStore) MarkAllRead(recipient notificationRecipient) (int, error) {
	db, err := config.DB()
	if err != nil {
		return 0, err
	}
	if err := (mysqlNotificationStore{}).Ensure(); err != nil {
		return 0, err
	}
	scope, args := notificationSQLScope(recipient)
	args = append([]any{time.Now().UTC()}, args...)
	result, err := db.Exec(`UPDATE notifications SET read_at = ? WHERE read_at IS NULL AND `+scope, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (mysqlNotificationStore) ListAdminIDs() ([]int64, error) {
	db, err := config.DB()
	if err != nil {
		return []int64{1}, err
	}
	rows, err := db.Query(`SELECT id FROM admins WHERE enabled = 1`)
	if err != nil {
		return []int64{1}, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []int64{1}, nil
	}
	return ids, nil
}

type notificationRowScanner interface {
	Scan(dest ...any) error
}

func scanNotificationRow(row notificationRowScanner) (inAppNotification, error) {
	var item inAppNotification
	var readAt sql.NullTime
	err := row.Scan(
		&item.ID, &item.UserRole, &item.UserID, &item.AgentID, &item.DeveloperID,
		&item.Category, &item.Title, &item.Body, &item.Link, &readAt, &item.CreatedAt,
		&item.EventType, &item.RefType, &item.RefID,
	)
	if err != nil {
		return item, err
	}
	if readAt.Valid {
		value := readAt.Time.UTC()
		item.ReadAt = &value
	}
	item.CreatedAt = item.CreatedAt.UTC()
	return item, nil
}
