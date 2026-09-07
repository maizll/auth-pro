package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
)

// ========== 工单系统 ==========
// 用户端 / 代理端 / 管理端三端共用一套表与状态机：
// pending(待处理) -> replied(已回复) -> closed(已关闭)；
// 创建人回复已回复工单会把状态拉回 pending，管理员可关闭/重开。

const (
	ticketStatusPending = "pending"
	ticketStatusReplied = "replied"
	ticketStatusClosed  = "closed"

	ticketMaxTitleLen   = 60
	ticketMaxContentLen = 2000
	ticketPageSizeMax   = 50
)

// 工单分类白名单，前端展示文案也以此为准
var ticketCategoryLabels = map[string]string{
	"authorization": "授权问题",
	"payment":       "支付问题",
	"deploy":        "部署咨询",
	"other":         "其他",
}

type ticketRow struct {
	ID           int64   `json:"id"`
	TicketNo     string  `json:"ticketNo"`
	CreatorType  string  `json:"creatorType"`
	CreatorID    int64   `json:"creatorId"`
	CreatorName  string  `json:"creatorName"`
	Category     string  `json:"category"`
	CategoryText string  `json:"categoryText"`
	Title        string  `json:"title"`
	Priority     string  `json:"priority"`
	Status       string  `json:"status"`
	StatusText   string  `json:"statusText"`
	LastReplyAt  string  `json:"lastReplyAt"`
	LastReplyBy  string  `json:"lastReplyBy"`
	ClosedAt     *string `json:"closedAt"`
	CreatedAt    string  `json:"createdAt"`
	Unread       bool    `json:"unread"`
}

type ticketMessageRow struct {
	ID         int64  `json:"id"`
	TicketID   int64  `json:"ticketId"`
	SenderType string `json:"senderType"`
	SenderID   int64  `json:"senderId"`
	SenderName string `json:"senderName"`
	Content    string `json:"content"`
	CreatedAt  string `json:"createdAt"`
}

// ensureTicketStorage 幂等建表与默认配置。
func ensureTicketStorage(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS tickets (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			ticket_no VARCHAR(24) NOT NULL COMMENT '工单编号',
			creator_type VARCHAR(10) NOT NULL COMMENT '创建人类型 user/agent',
			creator_id BIGINT UNSIGNED NOT NULL COMMENT '创建人ID',
			creator_name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '创建人显示名',
			category VARCHAR(20) NOT NULL DEFAULT 'other' COMMENT '分类',
			title VARCHAR(120) NOT NULL COMMENT '标题',
			priority VARCHAR(10) NOT NULL DEFAULT 'normal' COMMENT '优先级 low/normal/high',
			status VARCHAR(10) NOT NULL DEFAULT 'pending' COMMENT '状态 pending/replied/closed',
			last_reply_at DATETIME DEFAULT NULL COMMENT '最后回复时间',
			last_reply_by VARCHAR(10) NOT NULL DEFAULT '' COMMENT '最后回复角色',
			closed_at DATETIME DEFAULT NULL COMMENT '关闭时间',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_ticket_no (ticket_no),
			KEY idx_creator (creator_type, creator_id, status),
			KEY idx_status (status, last_reply_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工单'`,
		`CREATE TABLE IF NOT EXISTS ticket_messages (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			ticket_id BIGINT UNSIGNED NOT NULL,
			sender_type VARCHAR(10) NOT NULL COMMENT '发送方 user/agent/admin',
			sender_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			sender_name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '发送方显示名快照',
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_ticket (ticket_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工单消息'`,
		`CREATE TABLE IF NOT EXISTS ticket_reads (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			ticket_id BIGINT UNSIGNED NOT NULL,
			reader_type VARCHAR(10) NOT NULL COMMENT 'user/agent/admin',
			reader_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'admin 端共用 0',
			last_read_message_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			PRIMARY KEY (id),
			UNIQUE KEY uk_ticket_reader (ticket_id, reader_type, reader_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工单已读游标'`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	// 邮件通知开关，默认开启
	_, err := db.Exec(`
		INSERT INTO system_configs (` + "`group`" + `, ` + "`key`" + `, value, description)
		VALUES ('ticket', 'mail_notify', '1', '工单回复邮件通知')
		ON DUPLICATE KEY UPDATE ` + "`key`" + ` = VALUES(` + "`key`" + `)`)
	return err
}

func ticketStatusText(status string) string {
	switch status {
	case ticketStatusPending:
		return "待处理"
	case ticketStatusReplied:
		return "已回复"
	case ticketStatusClosed:
		return "已关闭"
	}
	return status
}

func ticketCategoryText(category string) string {
	if label, ok := ticketCategoryLabels[category]; ok {
		return label
	}
	return "其他"
}

func formatTicketTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func formatTicketTimePtr(t sql.NullTime) *string {
	if !t.Valid {
		return nil
	}
	value := t.Time.Local().Format("2006-01-02 15:04:05")
	return &value
}

// generateTicketNo 插入后按自增 ID 生成可读工单号。
func generateTicketNo(db *sql.DB, id int64, now time.Time) string {
	ticketNo := fmt.Sprintf("WO%s-%04d", now.Format("20060102"), id)
	_, _ = db.Exec("UPDATE tickets SET ticket_no = ? WHERE id = ?", ticketNo, id)
	return ticketNo
}

// creatorDisplayName 取用户/代理显示名，写入消息快照。
func creatorDisplayName(db *sql.DB, creatorType string, creatorID int64) string {
	var name string
	if creatorType == "agent" {
		if err := db.QueryRow("SELECT COALESCE(NULLIF(name,''), email) FROM agents WHERE id = ?", creatorID).Scan(&name); err == nil {
			return name
		}
		return ""
	}
	if err := db.QueryRow("SELECT COALESCE(NULLIF(nickname,''), email) FROM users WHERE id = ?", creatorID).Scan(&name); err == nil {
		return name
	}
	return ""
}

// ticketUnreadExists 判断某个角色在指定工单上是否有未读消息。
func ticketUnreadExists(db *sql.DB, ticketID int64, readerType string, readerID int64) bool {
	var exists int
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM ticket_messages m
			WHERE m.ticket_id = ? AND m.sender_type <> ?
			AND m.id > COALESCE((
				SELECT r.last_read_message_id FROM ticket_reads r
				WHERE r.ticket_id = ? AND r.reader_type = ? AND r.reader_id = ?
			), 0)
		)`, ticketID, readerType, ticketID, readerType, readerID).Scan(&exists)
	return err == nil && exists == 1
}

// markTicketRead 把读者的已读游标推到当前最大消息 ID。
func markTicketRead(db *sql.DB, ticketID int64, readerType string, readerID int64) {
	_, _ = db.Exec(`
		INSERT INTO ticket_reads (ticket_id, reader_type, reader_id, last_read_message_id)
		SELECT ?, ?, ?, COALESCE(MAX(id), 0) FROM ticket_messages WHERE ticket_id = ?
		ON DUPLICATE KEY UPDATE last_read_message_id = VALUES(last_read_message_id)`,
		ticketID, readerType, readerID, ticketID)
}

func scanTicketRow(row interface{ Scan(...any) error }) (ticketRow, error) {
	var item ticketRow
	var lastReplyAt, createdAt sql.NullTime
	var closedAt sql.NullTime
	err := row.Scan(&item.ID, &item.TicketNo, &item.CreatorType, &item.CreatorID, &item.CreatorName,
		&item.Category, &item.Title, &item.Priority, &item.Status,
		&lastReplyAt, &item.LastReplyBy, &closedAt, &createdAt)
	if err != nil {
		return item, err
	}
	if lastReplyAt.Valid {
		item.LastReplyAt = formatTicketTime(lastReplyAt.Time)
	}
	item.CreatedAt = formatTicketTime(createdAt.Time)
	item.ClosedAt = formatTicketTimePtr(closedAt)
	item.CategoryText = ticketCategoryText(item.Category)
	item.StatusText = ticketStatusText(item.Status)
	return item, nil
}

const ticketSelectFields = `id, ticket_no, creator_type, creator_id, creator_name, category, title, priority, status, last_reply_at, last_reply_by, closed_at, created_at`

func loadTicketMessages(db *sql.DB, ticketID int64) ([]ticketMessageRow, error) {
	rows, err := db.Query(`
		SELECT id, ticket_id, sender_type, sender_id, sender_name, content, created_at
		FROM ticket_messages WHERE ticket_id = ? ORDER BY id ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]ticketMessageRow, 0)
	for rows.Next() {
		var message ticketMessageRow
		var createdAt sql.NullTime
		if err := rows.Scan(&message.ID, &message.TicketID, &message.SenderType, &message.SenderID,
			&message.SenderName, &message.Content, &createdAt); err != nil {
			return nil, err
		}
		message.CreatedAt = formatTicketTime(createdAt.Time)
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// ========== 用户端 / 代理端工单接口 ==========

// panelTicketCreator 从 JWT 上下文解析工单创建人身份。
func panelTicketCreator(c *gin.Context) (string, int64, bool) {
	role := c.GetString("role")
	if role != "user" && role != "agent" {
		return "", 0, false
	}
	return role, int64(c.GetUint("user_id")), true
}

type panelTicketCreateRequest struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// PanelTicketCreate 创建工单，内容为第一条消息。
func PanelTicketCreate(c *gin.Context) {
	creatorType, creatorID, ok := panelTicketCreator(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
		return
	}
	var req panelTicketCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	category := strings.TrimSpace(req.Category)
	if _, valid := ticketCategoryLabels[category]; !valid {
		category = "other"
	}
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" || len([]rune(title)) > ticketMaxTitleLen {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": fmt.Sprintf("标题必填且不超过 %d 字", ticketMaxTitleLen)})
		return
	}
	if content == "" || len([]rune(content)) > ticketMaxContentLen {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": fmt.Sprintf("内容必填且不超过 %d 字", ticketMaxContentLen)})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}

	creatorName := creatorDisplayName(db, creatorType, creatorID)
	now := time.Now()
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建工单失败"})
		return
	}
	defer tx.Rollback()

	// 先占一个唯一临时编号，提交后按自增 ID 生成正式编号
	tempNo := fmt.Sprintf("TMP%d", now.UnixNano())
	result, err := tx.Exec(`
		INSERT INTO tickets (ticket_no, creator_type, creator_id, creator_name, category, title, priority, status, last_reply_at, last_reply_by)
		VALUES (?, ?, ?, ?, ?, ?, 'normal', ?, ?, ?)`,
		tempNo, creatorType, creatorID, creatorName, category, title, ticketStatusPending, now, creatorType)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建工单失败"})
		return
	}
	ticketID, _ := result.LastInsertId()
	if _, err := tx.Exec(`
		INSERT INTO ticket_messages (ticket_id, sender_type, sender_id, sender_name, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, ticketID, creatorType, creatorID, creatorName, content, now); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建工单失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建工单失败"})
		return
	}
	ticketNo := generateTicketNo(db, ticketID, now)
	queueTicketMail(ticketID, false)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "工单已提交", "data": gin.H{"id": ticketID, "ticketNo": ticketNo}})
}

// PanelTicketList 我的工单列表。
func PanelTicketList(c *gin.Context) {
	creatorType, creatorID, ok := panelTicketCreator(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > ticketPageSizeMax {
		pageSize = 10
	}

	where := "creator_type = ? AND creator_id = ?"
	args := []any{creatorType, creatorID}
	if status == ticketStatusPending || status == ticketStatusReplied || status == ticketStatusClosed {
		where += " AND status = ?"
		args = append(args, status)
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM tickets WHERE "+where, args...).Scan(&total); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单失败"})
		return
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := db.Query("SELECT "+ticketSelectFields+" FROM tickets WHERE "+where+
		" ORDER BY COALESCE(last_reply_at, created_at) DESC, id DESC LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单失败"})
		return
	}
	defer rows.Close()
	list := make([]ticketRow, 0)
	for rows.Next() {
		item, scanErr := scanTicketRow(rows)
		if scanErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单失败"})
			return
		}
		item.Unread = ticketUnreadExists(db, item.ID, creatorType, creatorID)
		list = append(list, item)
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list, "total": total}})
}

// loadOwnedTicket 校验工单归属并返回工单详情行。
func loadOwnedTicket(db *sql.DB, c *gin.Context, creatorType string, creatorID int64) (ticketRow, bool) {
	ticketID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if ticketID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "工单不存在"})
		return ticketRow{}, false
	}
	item, err := scanTicketRow(db.QueryRow(
		"SELECT "+ticketSelectFields+" FROM tickets WHERE id = ? AND creator_type = ? AND creator_id = ?",
		ticketID, creatorType, creatorID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "工单不存在"})
		return ticketRow{}, false
	}
	return item, true
}

// PanelTicketDetail 工单详情 + 消息流，读取后更新已读游标。
func PanelTicketDetail(c *gin.Context) {
	creatorType, creatorID, ok := panelTicketCreator(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	ticket, ok := loadOwnedTicket(db, c, creatorType, creatorID)
	if !ok {
		return
	}
	messages, err := loadTicketMessages(db, ticket.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单消息失败"})
		return
	}
	markTicketRead(db, ticket.ID, creatorType, creatorID)
	ticket.Unread = false
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"ticket": ticket, "messages": messages}})
}

type ticketReplyRequest struct {
	Content string `json:"content"`
}

// PanelTicketReply 创建人回复工单；回复已回复的工单会把状态拉回待处理。
func PanelTicketReply(c *gin.Context) {
	creatorType, creatorID, ok := panelTicketCreator(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
		return
	}
	var req ticketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || len([]rune(content)) > ticketMaxContentLen {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": fmt.Sprintf("回复内容必填且不超过 %d 字", ticketMaxContentLen)})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	ticket, ok := loadOwnedTicket(db, c, creatorType, creatorID)
	if !ok {
		return
	}
	if ticket.Status == ticketStatusClosed {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "工单已关闭，无法回复"})
		return
	}
	senderName := creatorDisplayName(db, creatorType, creatorID)
	now := time.Now()
	result, err := db.Exec(`
		INSERT INTO ticket_messages (ticket_id, sender_type, sender_id, sender_name, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, ticket.ID, creatorType, creatorID, senderName, content, now)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "回复失败"})
		return
	}
	messageID, _ := result.LastInsertId()
	_, _ = db.Exec(`
		UPDATE tickets SET status = ?, last_reply_at = ?, last_reply_by = ? WHERE id = ?`,
		ticketStatusPending, now, creatorType, ticket.ID)
	markTicketRead(db, ticket.ID, creatorType, creatorID)
	queueTicketMail(ticket.ID, false)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "回复成功", "data": gin.H{"messageId": messageID}})
}

// PanelTicketClose 创建人关闭自己的工单。
func PanelTicketClose(c *gin.Context) {
	creatorType, creatorID, ok := panelTicketCreator(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	ticket, ok := loadOwnedTicket(db, c, creatorType, creatorID)
	if !ok {
		return
	}
	if ticket.Status == ticketStatusClosed {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "工单已关闭"})
		return
	}
	_, _ = db.Exec("UPDATE tickets SET status = ?, closed_at = NOW() WHERE id = ?", ticketStatusClosed, ticket.ID)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "工单已关闭"})
}

// PanelTicketUnreadCount 创建人维度的未读工单数。
func PanelTicketUnreadCount(c *gin.Context) {
	creatorType, creatorID, ok := panelTicketCreator(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限访问"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"count": countUnreadTickets(db, creatorType, creatorID)}})
}

// countUnreadTickets 统计指定读者有多少工单存在未读消息。
func countUnreadTickets(db *sql.DB, readerType string, readerID int64) int {
	var count int
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM tickets t
		WHERE EXISTS(
			SELECT 1 FROM ticket_messages m
			WHERE m.ticket_id = t.id AND m.sender_type <> ?
			AND m.id > COALESCE((
				SELECT r.last_read_message_id FROM ticket_reads r
				WHERE r.ticket_id = t.id AND r.reader_type = ? AND r.reader_id = ?
			), 0)
		)`+ticketCreatorScopeSQL(readerType),
		unreadQueryArgs(readerType, readerID)...).Scan(&count)
	return count
}

// ticketCreatorScopeSQL 创建人只能统计自己的工单；管理端统计全部。
func ticketCreatorScopeSQL(readerType string) string {
	if readerType == "admin" {
		return ""
	}
	return " AND t.creator_type = ? AND t.creator_id = ?"
}

func unreadQueryArgs(readerType string, readerID int64) []any {
	if readerType == "admin" {
		return []any{"admin", "admin", int64(0)}
	}
	return []any{readerType, readerType, readerID, readerType, readerID}
}

// ========== 管理端工单接口 ==========

// AdminTicketList 工单管理列表，含各工单对管理端的未读标记。
func AdminTicketList(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	category := strings.TrimSpace(c.Query("category"))
	creatorType := strings.TrimSpace(c.Query("creatorType"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > ticketPageSizeMax {
		pageSize = 10
	}

	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if status == ticketStatusPending || status == ticketStatusReplied || status == ticketStatusClosed {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if _, valid := ticketCategoryLabels[category]; valid {
		conditions = append(conditions, "category = ?")
		args = append(args, category)
	}
	if creatorType == "user" || creatorType == "agent" {
		conditions = append(conditions, "creator_type = ?")
		args = append(args, creatorType)
	}
	if keyword != "" {
		conditions = append(conditions, "(title LIKE ? OR ticket_no LIKE ? OR creator_name LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM tickets"+where, args...).Scan(&total); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单失败"})
		return
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := db.Query("SELECT "+ticketSelectFields+" FROM tickets"+where+
		" ORDER BY (status = 'pending') DESC, COALESCE(last_reply_at, created_at) DESC, id DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单失败"})
		return
	}
	defer rows.Close()
	list := make([]ticketRow, 0)
	for rows.Next() {
		item, scanErr := scanTicketRow(rows)
		if scanErr != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单失败"})
			return
		}
		item.Unread = ticketUnreadExists(db, item.ID, "admin", 0)
		list = append(list, item)
	}

	var pendingCount, todayCount, closedCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM tickets WHERE status = ?", ticketStatusPending).Scan(&pendingCount)
	_ = db.QueryRow("SELECT COUNT(*) FROM tickets WHERE DATE(created_at) = CURDATE()").Scan(&todayCount)
	_ = db.QueryRow("SELECT COUNT(*) FROM tickets WHERE status = ?", ticketStatusClosed).Scan(&closedCount)

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{
		"list": list, "total": total,
		"stats": gin.H{"pending": pendingCount, "today": todayCount, "closed": closedCount},
	}})
}

// AdminTicketDetail 管理端查看工单，读取后管理端游标推进（管理端共用 reader_id = 0）。
func AdminTicketDetail(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	ticketID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	item, err := scanTicketRow(db.QueryRow("SELECT "+ticketSelectFields+" FROM tickets WHERE id = ?", ticketID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "工单不存在"})
		return
	}
	messages, err := loadTicketMessages(db, item.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询工单消息失败"})
		return
	}
	markTicketRead(db, item.ID, "admin", 0)
	item.Unread = false
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"ticket": item, "messages": messages}})
}

// AdminTicketReply 管理员回复工单，状态置为已回复并通知创建人。
func AdminTicketReply(c *gin.Context) {
	var req ticketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || len([]rune(content)) > ticketMaxContentLen {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": fmt.Sprintf("回复内容必填且不超过 %d 字", ticketMaxContentLen)})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	ticketID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ticket, err := scanTicketRow(db.QueryRow("SELECT "+ticketSelectFields+" FROM tickets WHERE id = ?", ticketID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "工单不存在"})
		return
	}
	if ticket.Status == ticketStatusClosed {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "工单已关闭，请先重新打开再回复"})
		return
	}
	adminName := c.GetString("username")
	if adminName == "" {
		adminName = "客服"
	}
	now := time.Now()
	result, err := db.Exec(`
		INSERT INTO ticket_messages (ticket_id, sender_type, sender_id, sender_name, content, created_at)
		VALUES (?, 'admin', ?, ?, ?, ?)`, ticket.ID, c.GetUint("user_id"), adminName, content, now)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "回复失败"})
		return
	}
	messageID, _ := result.LastInsertId()
	_, _ = db.Exec(`
		UPDATE tickets SET status = ?, last_reply_at = ?, last_reply_by = 'admin' WHERE id = ?`,
		ticketStatusReplied, now, ticket.ID)
	markTicketRead(db, ticket.ID, "admin", 0)
	queueTicketMail(ticket.ID, true)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "回复成功", "data": gin.H{"messageId": messageID}})
}

type adminTicketStatusRequest struct {
	Action string `json:"action"` // close / reopen
}

// AdminTicketStatus 关闭或重开工单。
func AdminTicketStatus(c *gin.Context) {
	var req adminTicketStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	ticketID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var status string
	if err := db.QueryRow("SELECT status FROM tickets WHERE id = ?", ticketID).Scan(&status); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "工单不存在"})
		return
	}
	switch req.Action {
	case "close":
		_, _ = db.Exec("UPDATE tickets SET status = ?, closed_at = NOW() WHERE id = ?", ticketStatusClosed, ticketID)
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "工单已关闭"})
	case "reopen":
		_, _ = db.Exec("UPDATE tickets SET status = ?, closed_at = NULL WHERE id = ?", ticketStatusPending, ticketID)
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "工单已重新打开"})
	default:
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不支持的操作"})
	}
}

// AdminTicketUnreadCount 管理端未读工单数（全站视角，管理端共用已读游标）。
func AdminTicketUnreadCount(c *gin.Context) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	if err := ensureTicketStorage(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化工单存储失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"count": countUnreadTickets(db, "admin", 0)}})
}

// ========== 工单邮件通知 ==========

func ticketMailNotifyEnabled(db *sql.DB) bool {
	var value string
	if err := db.QueryRow(
		"SELECT value FROM system_configs WHERE `group` = 'ticket' AND `key` = 'mail_notify'").Scan(&value); err != nil {
		return true // 配置缺失时默认开启
	}
	return value != "0"
}

// queueTicketMail 异步发送工单通知邮件。
// notifyCreator=true 表示管理员回复 → 通知工单创建人；false 表示创建人动作 → 通知管理员。
func queueTicketMail(ticketID int64, notifyCreator bool) {
	go func() {
		db, err := openSystemConfigDB()
		if err != nil {
			return
		}
		if err := ensureTicketStorage(db); err != nil || !ticketMailNotifyEnabled(db) {
			return
		}
		if err := ensureMailStorage(db); err != nil {
			return
		}
		cfg, err := loadMailConfig(db, true)
		if err != nil || validateMailConfig(cfg, true) != nil {
			return // 邮件未配置时静默跳过
		}

		ticket, err := scanTicketRow(db.QueryRow("SELECT "+ticketSelectFields+" FROM tickets WHERE id = ?", ticketID))
		if err != nil {
			return
		}
		var lastContent string
		_ = db.QueryRow("SELECT content FROM ticket_messages WHERE ticket_id = ? ORDER BY id DESC LIMIT 1", ticketID).
			Scan(&lastContent)
		if len([]rune(lastContent)) > 120 {
			lastContent = string([]rune(lastContent)[:120]) + "…"
		}
		siteName := loadSiteNameForMail(db)
		subject := fmt.Sprintf("[%s] 工单 %s 有新回复", siteName, ticket.TicketNo)
		content := fmt.Sprintf("工单编号：%s\n标题：%s\n分类：%s\n\n最新回复：\n%s\n\n请登录系统查看完整对话。",
			ticket.TicketNo, ticket.Title, ticket.CategoryText, lastContent)

		recipients := make([]string, 0, 2)
		if notifyCreator {
			var email string
			if ticket.CreatorType == "agent" {
				_ = db.QueryRow("SELECT email FROM agents WHERE id = ?", ticket.CreatorID).Scan(&email)
			} else {
				_ = db.QueryRow("SELECT email FROM users WHERE id = ?", ticket.CreatorID).Scan(&email)
			}
			if email != "" {
				recipients = append(recipients, email)
			}
		} else {
			rows, err := db.Query("SELECT email FROM admins WHERE enabled = 1 AND email <> ''")
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var email string
					if rows.Scan(&email) == nil && email != "" {
						recipients = append(recipients, email)
					}
				}
			}
		}

		targetType, targetID := ticket.CreatorType, ticket.CreatorID
		if !notifyCreator {
			targetType, targetID = "admin", 0
		}
		for _, recipient := range recipients {
			logID, _ := createMailLog(db, "ticket_reply", targetType, targetID, 0, recipient, subject, content, 0, nil)
			if err := sendSMTPMail(cfg, mailMessage{To: recipient, Subject: subject, Content: content}); err != nil {
				markMailLogFailed(db, logID, err.Error())
				continue
			}
			markMailLogSent(db, logID)
		}
	}()
}

// ensureTicketMenu 清理旧版注册到菜单表的工单菜单。
// 「工单管理」已改为前端内置一级菜单（固定显示在在线更新下方），不再走菜单表驱动，
// 避免重复显示；此处幂等删除历史行，保持菜单表干净。
func ensureTicketMenu(db *sql.DB) {
	var menuID int64
	if err := db.QueryRow("SELECT id FROM menus WHERE name = 'TicketManage' LIMIT 1").Scan(&menuID); err != nil || menuID == 0 {
		return
	}
	_, _ = db.Exec("DELETE FROM role_menus WHERE menu_id = ?", menuID)
	_, _ = db.Exec("DELETE FROM menus WHERE id = ?", menuID)
}
