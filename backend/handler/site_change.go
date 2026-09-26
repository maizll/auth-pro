package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

const (
	siteChangeUnlimited   = -1
	siteChangeOrderPrefix = "SC"
	msgSiteChangeDenied   = "免费更换次数已用完，本套餐不支持付费更换，请联系管理员"
	msgSiteChangeNeedPay  = "免费更换次数已用完，请支付后完成更换"
)

var (
	errSiteChangeDenied = errors.New(msgSiteChangeDenied)
	siteChangeSchemaMu  sync.Mutex
	siteChangeSchemaDB  = map[string]bool{}
)

type siteChangeState struct {
	LicenseType string
	AppID       int64
	Left        int
	Price       sql.NullFloat64
	OldTarget   string
	SiteID      int64
	TargetType  string
}

func EnsureSiteChangeSchema(db *sql.DB) error {
	return ensureSiteChangeSchema(db)
}

func ensureSiteChangeSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	var name string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		return err
	}
	siteChangeSchemaMu.Lock()
	defer siteChangeSchemaMu.Unlock()
	if siteChangeSchemaDB[name] {
		return nil
	}
	if err := ensureTableColumn(db, "license_plans", "free_site_changes", `ALTER TABLE license_plans
		ADD COLUMN free_site_changes INT NOT NULL DEFAULT -1
		COMMENT '免费更换次数，-1不限，0不允许免费更换'`); err != nil {
		return err
	}
	if err := ensureTableColumn(db, "license_plans", "site_change_price", `ALTER TABLE license_plans
		ADD COLUMN site_change_price DECIMAL(12,2) NULL
		COMMENT '超出后每次更换价格，空表示不可付费更换'`); err != nil {
		return err
	}
	if err := ensureTableColumn(db, "licenses", "free_site_changes", `ALTER TABLE licenses
		ADD COLUMN free_site_changes INT NOT NULL DEFAULT -1
		COMMENT '剩余免费更换次数，-1不限'`); err != nil {
		return err
	}
	if err := ensureTableColumn(db, "licenses", "site_change_price", `ALTER TABLE licenses
		ADD COLUMN site_change_price DECIMAL(12,2) NULL
		COMMENT '超出后每次更换价格快照，空表示不可付费更换'`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS license_site_change_orders (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		order_no VARCHAR(64) NOT NULL,
		owner_type VARCHAR(20) NOT NULL,
		owner_id BIGINT UNSIGNED NOT NULL,
		license_id BIGINT UNSIGNED NOT NULL,
		action VARCHAR(20) NOT NULL,
		site_id BIGINT UNSIGNED NULL,
		old_target VARCHAR(255) NOT NULL DEFAULT '',
		new_target VARCHAR(255) NOT NULL DEFAULT '',
		amount DECIMAL(12,2) NOT NULL,
		paid_amount DECIMAL(12,2) NULL,
		pay_channel VARCHAR(30) NOT NULL DEFAULT '',
		pay_method VARCHAR(30) NOT NULL DEFAULT '',
		gateway_trade_no VARCHAR(100) NOT NULL DEFAULT '',
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		return_url VARCHAR(500) NOT NULL DEFAULT '',
		notify_payload TEXT NULL,
		paid_at DATETIME NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_site_change_order_no (order_no),
		KEY idx_site_change_license (license_id, status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='更换授权站点订单'`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS operation_logs (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		operator_type VARCHAR(20) NOT NULL,
		operator_id BIGINT UNSIGNED NULL,
		action VARCHAR(100) NOT NULL,
		target_type VARCHAR(50) NOT NULL DEFAULT '',
		target_id BIGINT UNSIGNED NULL,
		detail JSON NULL,
		ip VARCHAR(45) NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_operation_target (target_type, target_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS license_site_change_logs (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		license_id BIGINT UNSIGNED NOT NULL,
		action VARCHAR(40) NOT NULL,
		actor_type VARCHAR(20) NOT NULL,
		actor_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
		order_no VARCHAR(64) NOT NULL DEFAULT '',
		detail JSON NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_site_change_log_license (license_id, id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='授权站点更换记录'`); err != nil {
		return err
	}
	siteChangeSchemaDB[name] = true
	return nil
}

func ensureNullableIntColumn(db *sql.DB, table, column, ddl string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.Exec(ddl)
	return err
}

func ensureTableColumn(db *sql.DB, table, column, ddl string) error {
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&tables); err != nil {
		return err
	}
	if tables == 0 {
		return nil
	}
	return ensureNullableIntColumn(db, table, column, ddl)
}

func snapshotLicenseSiteChange(tx *sql.Tx, licenseID, planID int64) error {
	if tx == nil || licenseID <= 0 || planID <= 0 {
		return nil
	}
	_, err := tx.Exec(`UPDATE licenses l
		JOIN license_plans p ON p.id = ?
		SET l.free_site_changes = p.free_site_changes,
		    l.site_change_price = p.site_change_price
		WHERE l.id = ?`, planID, licenseID)
	if isMissingSiteChangeColumn(err) {
		return nil
	}
	return err
}

func isMissingSiteChangeColumn(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1054
}

func normalizePlanFreeChanges(value *int) (int, string) {
	if value == nil {
		return siteChangeUnlimited, ""
	}
	if *value < siteChangeUnlimited {
		return 0, "免费更换次数不能小于 -1"
	}
	return *value, ""
}

func normalizePlanChangePrice(value *float64) (sql.NullFloat64, string) {
	if value == nil {
		return sql.NullFloat64{}, ""
	}
	if *value <= 0 {
		return sql.NullFloat64{}, "更换价格要大于 0，不填表示用完后不能付费更换"
	}
	return sql.NullFloat64{Float64: math.Round(*value*100) / 100, Valid: true}, ""
}

func siteChangePriceArg(price sql.NullFloat64) any {
	if !price.Valid {
		return nil
	}
	return price.Float64
}

func payableSiteChangePrice(price sql.NullFloat64) (float64, bool) {
	if !price.Valid || price.Float64 <= 0 {
		return 0, false
	}
	return price.Float64, true
}

type siteChangeDecision struct {
	NeedPay  bool
	Price    float64
	Deducted bool
	Left     int
}

func decideSiteChange(actor string, left int, price sql.NullFloat64) (siteChangeDecision, error) {
	decision := siteChangeDecision{Left: left}
	if actor == "admin" || left < 0 {
		return decision, nil
	}
	if left > 0 {
		decision.Deducted = true
		decision.Left = left - 1
		return decision, nil
	}
	if amount, ok := payableSiteChangePrice(price); ok {
		decision.NeedPay = true
		decision.Price = amount
		return decision, nil
	}
	return decision, errSiteChangeDenied
}

func lockLicenseForSiteChange(tx *sql.Tx, licenseID int64, ownerType string, ownerID int64) (siteChangeState, error) {
	var state siteChangeState
	query := `SELECT l.type, l.app_id, COALESCE(l.free_site_changes, -1), l.site_change_price,
		COALESCE((SELECT domain FROM license_domains WHERE license_id = l.id ORDER BY id LIMIT 1), '')
		FROM licenses l WHERE l.id = ?`
	args := []any{licenseID}
	if ownerType != "" {
		query += ` AND l.owner_type = ? AND l.owner_id = ?`
		args = append(args, ownerType, ownerID)
	}
	query += ` FOR UPDATE`
	err := tx.QueryRow(query, args...).Scan(&state.LicenseType, &state.AppID, &state.Left, &state.Price, &state.OldTarget)
	return state, err
}

func consumeFreeSiteChange(tx *sql.Tx, licenseID int64) error {
	result, err := tx.Exec(`UPDATE licenses SET free_site_changes = free_site_changes - 1
		WHERE id = ? AND free_site_changes > 0`, licenseID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errSiteChangeDenied
	}
	return nil
}

func insertLicenseSiteChangeLog(tx *sql.Tx, licenseID int64, action, actorType string, actorID int64, orderNo string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO license_site_change_logs
		(license_id, action, actor_type, actor_id, order_no, detail)
		VALUES (?, ?, ?, ?, ?, ?)`, licenseID, action, actorType, actorID, orderNo, string(raw))
	return err
}

func writeSiteChangeRecord(tx *sql.Tx, licenseID int64, action, actorType string, actorID int64, orderNo, ip string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	if err := insertLicenseSiteChangeLog(tx, licenseID, action, actorType, actorID, orderNo, detail); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO operation_logs
		(operator_type, operator_id, action, target_type, target_id, detail, ip)
		VALUES (?, ?, ?, 'license', ?, ?, ?)`, actorType, actorID, action, licenseID, string(raw), ip); err != nil {
		return err
	}
	return nil
}

func respondSiteChangeBlocked(c *gin.Context, decision siteChangeDecision, err error) bool {
	if err != nil {
		msg := err.Error()
		if errors.Is(err, errSiteChangeDenied) {
			msg = msgSiteChangeDenied
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": msg})
		return true
	}
	if decision.NeedPay {
		c.JSON(http.StatusOK, gin.H{
			"code": 402,
			"msg":  msgSiteChangeNeedPay,
			"data": gin.H{"needPay": true, "price": decision.Price},
		})
		return true
	}
	return false
}

type siteChangeApply struct {
	Action string
	SiteID int64
	Target string
}

func applySiteChangeMutation(tx *sql.Tx, licenseID int64, state siteChangeState, apply siteChangeApply) (string, error) {
	switch apply.Action {
	case "unbind":
		if state.LicenseType == "key" {
			if apply.SiteID <= 0 {
				return "", errors.New("请选择要解绑的站点")
			}
			var old string
			err := tx.QueryRow(`SELECT domain FROM license_domains WHERE id = ? AND license_id = ? FOR UPDATE`, apply.SiteID, licenseID).Scan(&old)
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil
			}
			if err != nil {
				return "", err
			}
			if _, err := tx.Exec(`DELETE FROM license_domains WHERE id = ? AND license_id = ?`, apply.SiteID, licenseID); err != nil {
				return "", err
			}
			return old, nil
		}
		old := state.OldTarget
		if strings.TrimSpace(old) == "" {
			return "", nil
		}
		if _, err := tx.Exec(`DELETE FROM license_domains WHERE license_id = ?`, licenseID); err != nil {
			return "", err
		}
		return old, nil
	case "replace":
		if state.LicenseType == "key" {
			return replaceKeySite(tx, licenseID, apply)
		}
		return replaceLicenseTarget(tx, licenseID, state, apply.Target)
	default:
		return "", errors.New("更换动作不正确")
	}
}

func replaceLicenseTarget(tx *sql.Tx, licenseID int64, state siteChangeState, target string) (string, error) {
	validated, errMsg := validateLicenseTargetForSave(state.LicenseType, target)
	if errMsg != "" {
		return "", errors.New(errMsg)
	}
	if normalizeLicenseDomain(state.OldTarget) == normalizeLicenseDomain(validated) {
		return state.OldTarget, nil
	}
	if normalizeLicenseDomain(state.OldTarget) != "" {
		taken, err := licenseDomainTakenTx(tx, state.AppID, licenseID, validated)
		if err != nil {
			return "", err
		}
		if taken {
			return "", errors.New(licenseDomainOccupied)
		}
	}
	isWildcard := 0
	if state.LicenseType == "wildcard" {
		isWildcard = 1
	}
	if _, err := tx.Exec(`DELETE FROM license_domains WHERE license_id = ?`, licenseID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO license_domains (license_id, domain, is_wildcard) VALUES (?, ?, ?)`, licenseID, validated, isWildcard); err != nil {
		return "", err
	}
	return state.OldTarget, nil
}

func replaceKeySite(tx *sql.Tx, licenseID int64, apply siteChangeApply) (string, error) {
	if apply.SiteID <= 0 {
		return "", errors.New("请选择要更换的站点")
	}
	var old, targetType string
	err := tx.QueryRow(`SELECT domain, target_type FROM license_domains WHERE id = ? AND license_id = ? FOR UPDATE`, apply.SiteID, licenseID).Scan(&old, &targetType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("绑定站点不存在")
	}
	if err != nil {
		return "", err
	}
	nextType, nextValue, err := normalizeSiteChangeTarget(apply.Target)
	if err != nil {
		return "", err
	}
	if nextType == targetType && normalizeLicenseDomain(old) == normalizeLicenseDomain(nextValue) {
		return old, nil
	}
	if _, err := tx.Exec(`UPDATE license_domains SET target_type = ?, domain = ?, is_wildcard = 0 WHERE id = ? AND license_id = ?`, nextType, nextValue, apply.SiteID, licenseID); err != nil {
		return "", err
	}
	return old, nil
}

func normalizeSiteChangeTarget(raw string) (string, string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", "", errors.New("请填写新的域名或 IP")
	}
	if validated, errMsg := validateLicenseTargetForSave("ip", target); errMsg == "" {
		return "ip", validated, nil
	}
	validated, errMsg := validateLicenseTargetForSave("domain", target)
	if errMsg != "" {
		return "", "", errors.New("新站点格式不正确")
	}
	return "domain", validated, nil
}

func licenseDomainTakenTx(tx *sql.Tx, appID, licenseID int64, domain string) (bool, error) {
	if tx == nil || appID <= 0 || domain == "" {
		return false, nil
	}
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM licenses l
		JOIN license_domains ld ON ld.license_id = l.id
		WHERE l.app_id = ? AND l.id <> ? AND l.status = 'active' AND ld.domain = ?
	`, appID, licenseID, domain).Scan(&count)
	return count > 0, err
}

func countsAsSiteChange(action, licenseType, oldTarget, newTarget string) bool {
	if action == "unbind" {
		return strings.TrimSpace(oldTarget) != ""
	}
	if action != "replace" || licenseType == "key" {
		return action == "replace"
	}
	if strings.TrimSpace(oldTarget) == "" {
		return false
	}
	return normalizeLicenseDomain(oldTarget) != normalizeLicenseDomain(newTarget)
}

func configDBOrNil() *sql.DB {
	db, err := config.DB()
	if err != nil {
		return nil
	}
	return db
}

func userOrAgentSiteChange(c *gin.Context, actor string, actorID int64, apply siteChangeApply) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || licenseID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "授权ID不正确"})
		return
	}
	db, err := config.DB()
	if err != nil || ensureSiteChangeSchema(db) != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	ownerType := actor
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "系统错误"})
		return
	}
	defer tx.Rollback()
	owned, err := lockLicenseForSiteChange(tx, licenseID, ownerType, actorID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "授权不存在"})
		return
	}
	decision, err := runOwnedSiteChange(tx, licenseID, actor, actorID, owned, apply, c.ClientIP(), "")
	if respondSiteChangeBlocked(c, decision, err) {
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
		return
	}
	msg := "更新成功"
	if apply.Action == "unbind" && apply.SiteID > 0 {
		msg = "站点已解绑，名额已立即释放"
	} else if apply.Action == "unbind" {
		msg = "已解绑域名"
	} else if apply.Action == "replace" && apply.SiteID > 0 {
		msg = "已更换"
	} else if apply.Action == "replace" {
		msg = "更新成功"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg})
}

func runOwnedSiteChange(tx *sql.Tx, licenseID int64, actor string, actorID int64, state siteChangeState, apply siteChangeApply, ip, orderNo string) (siteChangeDecision, error) {
	oldForCount := state.OldTarget
	if state.LicenseType == "key" && apply.SiteID > 0 {
		_ = tx.QueryRow(`SELECT domain FROM license_domains WHERE id = ? AND license_id = ?`, apply.SiteID, licenseID).Scan(&oldForCount)
	}
	if !countsAsSiteChange(apply.Action, state.LicenseType, oldForCount, apply.Target) {
		if state.LicenseType == "domain" && apply.Action == "replace" && actor != "admin" && orderNo == "" {
			if err := guardProductDomainChange(configDBOrNil(), licenseID, apply.Target, false); err != nil {
				return siteChangeDecision{}, err
			}
		}
		_, err := applySiteChangeMutation(tx, licenseID, state, apply)
		if err == nil && state.LicenseType == "domain" && apply.Action == "replace" {
			finishProductDomainChange(configDBOrNil(), licenseID, oldForCount, apply.Target, actor)
		}
		return siteChangeDecision{Left: state.Left}, err
	}
	if state.LicenseType == "domain" && apply.Action == "replace" && actor != "admin" && orderNo == "" {
		if err := guardProductDomainChange(configDBOrNil(), licenseID, apply.Target, false); err != nil {
			return siteChangeDecision{}, err
		}
	}
	decision, err := decideSiteChange(actor, state.Left, state.Price)
	if err != nil || (decision.NeedPay && orderNo == "") {
		return decision, err
	}
	if decision.Deducted && orderNo == "" {
		if err := consumeFreeSiteChange(tx, licenseID); err != nil {
			return siteChangeDecision{}, err
		}
	}
	old, err := applySiteChangeMutation(tx, licenseID, state, apply)
	if err != nil {
		return decision, err
	}
	if apply.Action == "unbind" && strings.TrimSpace(old) == "" {
		return siteChangeDecision{Left: state.Left}, nil
	}
	actionName := "license_site_change"
	if apply.Action == "unbind" {
		actionName = "license_site_unbind"
	}
	if orderNo != "" {
		actionName = "license_site_change_paid"
		decision.Deducted = false
	}
	err = writeSiteChangeRecord(tx, licenseID, actionName, actor, actorID, orderNo, ip, map[string]any{
		"action": apply.Action, "siteId": apply.SiteID, "oldTarget": old, "newTarget": apply.Target,
		"deducted": decision.Deducted, "left": decision.Left, "orderNo": orderNo,
	})
	if err != nil {
		return decision, err
	}
	if state.LicenseType == "domain" && apply.Action == "replace" {
		finishProductDomainChange(configDBOrNil(), licenseID, old, apply.Target, actor)
	}
	return decision, nil
}

func UserLicenseSiteReplace(c *gin.Context) {
	userID, ok := getUserPanelID(c)
	if !ok {
		return
	}
	siteID, _ := strconv.ParseInt(c.Param("siteId"), 10, 64)
	var req struct {
		Target string `json:"target"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写新的域名或 IP"})
		return
	}
	userOrAgentSiteChange(c, "user", int64(userID), siteChangeApply{Action: "replace", SiteID: siteID, Target: req.Target})
}

func AgentLicenseSiteReplace(c *gin.Context) {
	agentID, ok := getAgentID(c)
	if !ok {
		return
	}
	siteID, _ := strconv.ParseInt(c.Param("siteId"), 10, 64)
	var req struct {
		Target string `json:"target"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写新的域名或 IP"})
		return
	}
	userOrAgentSiteChange(c, "agent", int64(agentID), siteChangeApply{Action: "replace", SiteID: siteID, Target: req.Target})
}

type siteChangePayRequest struct {
	Action    string `json:"action"`
	SiteID    int64  `json:"siteId"`
	Target    string `json:"target"`
	PayMethod string `json:"payMethod"`
}

func UserLicenseSiteChangePay(c *gin.Context) {
	userID, ok := getUserPanelID(c)
	if !ok {
		return
	}
	paySiteChange(c, "user", int64(userID), "/user/licenses")
}

func AgentLicenseSiteChangePay(c *gin.Context) {
	agentID, ok := getAgentID(c)
	if !ok {
		return
	}
	paySiteChange(c, "agent", int64(agentID), "/agent-panel/licenses")
}

func paySiteChange(c *gin.Context, ownerType string, ownerID int64, returnPath string) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || licenseID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "授权ID不正确"})
		return
	}
	var req siteChangePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写更换信息"})
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.PayMethod = strings.TrimSpace(req.PayMethod)
	if req.Action != "unbind" && req.Action != "replace" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "更换动作不正确"})
		return
	}
	db, err := config.DB()
	if err != nil || ensureSiteChangeSchema(db) != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	apply := siteChangeApply{Action: req.Action, SiteID: req.SiteID, Target: req.Target}
	if req.PayMethod == "" || req.PayMethod == "balance" {
		completeSiteChangeWithBalance(c, db, licenseID, ownerType, ownerID, apply)
		return
	}
	startSiteChangeOnlinePay(c, db, licenseID, ownerType, ownerID, apply, req.PayMethod, returnPath)
}

func completeSiteChangeWithBalance(c *gin.Context, db *sql.DB, licenseID int64, ownerType string, ownerID int64, apply siteChangeApply) {
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "系统错误"})
		return
	}
	defer tx.Rollback()
	state, err := lockLicenseForSiteChange(tx, licenseID, ownerType, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "授权不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询授权失败"})
		return
	}
	decision, err := decideForApply(tx, licenseID, state, apply)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if !decision.NeedPay {
		if _, err := runOwnedSiteChange(tx, licenseID, ownerType, ownerID, state, apply, c.ClientIP(), ""); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已完成更换"})
		return
	}
	table := "users"
	if ownerType == "agent" {
		table = "agents"
	}
	balanceAfter, err := deductPurchaseBalance(tx, table, ownerID, decision.Price)
	if errors.Is(err, errPurchaseBalanceInsufficient) {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "余额不足"})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "扣款失败"})
		return
	}
	orderNo := generateSiteChangeOrderNo()
	orderID, err := insertSiteChangeOrder(tx, orderNo, ownerType, ownerID, licenseID, apply, state.OldTarget, decision.Price, "balance", "balance", "paid")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建更换订单失败"})
		return
	}
	if _, err := runOwnedSiteChange(tx, licenseID, ownerType, ownerID, state, apply, c.ClientIP(), orderNo); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	txType := "purchase"
	if ownerType == "agent" {
		txType = "consume"
	}
	amount := -decision.Price
	if _, err := tx.Exec(`INSERT INTO transactions
		(tx_no, subject_type, subject_id, type, amount, balance_after, ref_type, ref_id, remark)
		VALUES (?, ?, ?, ?, ?, ?, 'site_change', ?, '更换授权站点')`,
		"TX"+orderNo, ownerType, ownerID, txType, amount, balanceAfter, orderID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "记录流水失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已支付并完成更换", "data": gin.H{"orderNo": orderNo, "status": "paid"}})
}

func decideForApply(tx *sql.Tx, licenseID int64, state siteChangeState, apply siteChangeApply) (siteChangeDecision, error) {
	old := state.OldTarget
	if state.LicenseType == "key" && apply.SiteID > 0 {
		_ = tx.QueryRow(`SELECT domain FROM license_domains WHERE id = ? AND license_id = ?`, apply.SiteID, licenseID).Scan(&old)
	}
	if !countsAsSiteChange(apply.Action, state.LicenseType, old, apply.Target) {
		return siteChangeDecision{Left: state.Left}, nil
	}
	if state.LicenseType == "domain" && apply.Action == "replace" {
		if err := guardProductDomainChange(configDBOrNil(), licenseID, apply.Target, false); err != nil {
			return siteChangeDecision{}, err
		}
	}
	return decideSiteChange("user", state.Left, state.Price)
}

func insertSiteChangeOrder(tx *sql.Tx, orderNo, ownerType string, ownerID, licenseID int64, apply siteChangeApply, old string, amount float64, channel, method, status string) (int64, error) {
	var site any
	if apply.SiteID > 0 {
		site = apply.SiteID
	}
	paid := any(nil)
	var paidAt any
	if status == "paid" {
		paid = amount
		paidAt = time.Now()
	}
	result, err := tx.Exec(`INSERT INTO license_site_change_orders
		(order_no, owner_type, owner_id, license_id, action, site_id, old_target, new_target, amount, paid_amount, pay_channel, pay_method, status, paid_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orderNo, ownerType, ownerID, licenseID, apply.Action, site, old, strings.TrimSpace(apply.Target), amount, paid, channel, method, status, paidAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func generateSiteChangeOrderNo() string {
	return fmt.Sprintf("%s%d%06d", siteChangeOrderPrefix, time.Now().Unix(), time.Now().Nanosecond()%1000000)
}

func startSiteChangeOnlinePay(c *gin.Context, db *sql.DB, licenseID int64, ownerType string, ownerID int64, apply siteChangeApply, payMethod, returnPath string) {
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "系统错误"})
		return
	}
	defer tx.Rollback()
	state, err := lockLicenseForSiteChange(tx, licenseID, ownerType, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "授权不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询授权失败"})
		return
	}
	decision, err := decideForApply(tx, licenseID, state, apply)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if !decision.NeedPay {
		if _, err := runOwnedSiteChange(tx, licenseID, ownerType, ownerID, state, apply, c.ClientIP(), ""); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": err.Error()})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已完成更换"})
		return
	}
	orderNo := generateSiteChangeOrderNo()
	if _, err := insertSiteChangeOrder(tx, orderNo, ownerType, ownerID, licenseID, apply, state.OldTarget, decision.Price, "", payMethod, "pending"); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "创建更换订单失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
		return
	}
	amountCents := int64(math.Round(decision.Price * 100))
	selection, ok := parseOnlinePaySelection(payMethod)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "不支持的支付方式"})
		return
	}
	subject := "更换授权站点"
	failOrder := func(reason string) {
		_, _ = db.Exec(`UPDATE license_site_change_orders SET status = 'failed' WHERE order_no = ? AND status = 'pending'`, orderNo)
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": reason})
	}
	if isRegisteredPayChannel(selection.Channel) {
		result, returnURL, err := createRegisteredChannelPayment(c, db, selection, orderNo, amountCents, subject, returnPath)
		if err != nil {
			failOrder(err.Error())
			return
		}
		_, _ = db.Exec(`UPDATE license_site_change_orders SET pay_channel = ?, pay_method = ?, return_url = ? WHERE order_no = ?`, selection.Channel, selection.PayType, returnURL, orderNo)
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "请完成支付，支付成功后自动更换", "data": checkoutData(orderNo, formatCents(amountCents), selection.PayType, result)})
		return
	}
	if selection.Channel == "" || selection.Channel == payChannelEpayV1 {
		payConfig, err := loadEpayConfig(db)
		if err == nil && payConfig.validateForPay() == nil && payConfig.isPayTypeEnabled(selection.PayType) {
			payURL, returnURL, err := buildEpaySubmitURL(c, payConfig, orderNo, amountCents, selection.PayType, subject, returnPath)
			if err != nil {
				failOrder(err.Error())
				return
			}
			_, _ = db.Exec(`UPDATE license_site_change_orders SET pay_channel = ?, pay_method = ?, return_url = ? WHERE order_no = ?`, payChannelEpayV1, selection.PayType, returnURL, orderNo)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "请完成支付，支付成功后自动更换", "data": gin.H{
				"orderNo": orderNo, "amount": formatCents(amountCents), "payType": selection.PayType, "payUrl": payURL,
			}})
			return
		}
	}
	if selection.Channel == "" || selection.Channel == payChannelEpayV2 {
		payConfig, err := loadEpayV2Config(db)
		if err == nil && payConfig.validateForPay() == nil && payConfig.isPayTypeEnabled(selection.PayType) {
			payURL, returnURL, err := buildEpayV2Payment(c, payConfig, orderNo, amountCents, selection.PayType, subject, returnPath)
			if err != nil {
				failOrder(err.Error())
				return
			}
			_, _ = db.Exec(`UPDATE license_site_change_orders SET pay_channel = ?, pay_method = ?, return_url = ? WHERE order_no = ?`, payChannelEpayV2, selection.PayType, returnURL, orderNo)
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "请完成支付，支付成功后自动更换", "data": gin.H{
				"orderNo": orderNo, "amount": formatCents(amountCents), "payType": selection.PayType, "payUrl": payURL,
			}})
			return
		}
	}
	failOrder("该支付方式未开启")
}

func settleSiteChangeOrder(db *sql.DB, orderNo string, paidCents int64, channel, payMethod, tradeNo, payload string) error {
	if err := ensureSiteChangeSchema(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, licenseID, ownerID sql.NullInt64
	var ownerType, action, oldTarget, newTarget, status string
	var amountText string
	var siteRaw sql.NullInt64
	err = tx.QueryRow(`SELECT id, owner_type, owner_id, license_id, action, site_id, old_target, new_target, CAST(amount AS CHAR), status
		FROM license_site_change_orders WHERE order_no = ? FOR UPDATE`, orderNo).
		Scan(&id, &ownerType, &ownerID, &licenseID, &action, &siteRaw, &oldTarget, &newTarget, &amountText, &status)
	if err != nil {
		return err
	}
	if status == "paid" {
		return tx.Commit()
	}
	if status != "pending" {
		return errors.New("更换订单已关闭")
	}
	amount, err := strconv.ParseFloat(amountText, 64)
	if err != nil {
		return err
	}
	if int64(math.Round(amount*100)) != paidCents {
		return errors.New("支付金额不一致")
	}
	apply := siteChangeApply{Action: action, Target: newTarget}
	if siteRaw.Valid {
		apply.SiteID = siteRaw.Int64
	}
	state, err := lockLicenseForSiteChange(tx, licenseID.Int64, ownerType, ownerID.Int64)
	if err != nil {
		return err
	}
	if _, err := runOwnedSiteChange(tx, licenseID.Int64, ownerType, ownerID.Int64, state, apply, "", orderNo); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE license_site_change_orders
		SET status = 'paid', paid_amount = ?, pay_channel = ?, pay_method = ?, gateway_trade_no = ?, notify_payload = ?, paid_at = NOW()
		WHERE id = ? AND status = 'pending'`, amount, channel, payMethod, tradeNo, payload, id.Int64); err != nil {
		return err
	}
	return tx.Commit()
}

func UserLicenseSiteChangeOrderStatus(c *gin.Context) {
	userID, ok := getUserPanelID(c)
	if !ok {
		return
	}
	readSiteChangeOrderStatus(c, "user", int64(userID))
}

func AgentLicenseSiteChangeOrderStatus(c *gin.Context) {
	agentID, ok := getAgentID(c)
	if !ok {
		return
	}
	readSiteChangeOrderStatus(c, "agent", int64(agentID))
}

func readSiteChangeOrderStatus(c *gin.Context, ownerType string, ownerID int64) {
	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	var status string
	err = db.QueryRow(`SELECT status FROM license_site_change_orders WHERE order_no = ? AND owner_type = ? AND owner_id = ?`,
		strings.TrimSpace(c.Param("orderNo")), ownerType, ownerID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询订单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"status": status}})
}

func AdminLicenseSiteChangeAdjust(c *gin.Context) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || licenseID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "授权ID不正确"})
		return
	}
	var req struct {
		Unlimited bool `json:"unlimited"`
		Delta     *int `json:"delta"`
		Value     *int `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写调整次数"})
		return
	}
	db, err := config.DB()
	if err != nil || ensureSiteChangeSchema(db) != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "系统错误"})
		return
	}
	defer tx.Rollback()
	var left int
	if err := tx.QueryRow(`SELECT COALESCE(free_site_changes, -1) FROM licenses WHERE id = ? FOR UPDATE`, licenseID).Scan(&left); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "msg": "授权不存在"})
		return
	}
	next := left
	switch {
	case req.Unlimited:
		next = siteChangeUnlimited
	case req.Value != nil:
		if *req.Value < 0 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "剩余次数不能小于 0"})
			return
		}
		next = *req.Value
	case req.Delta != nil:
		if left < 0 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "当前不限次数，请先设为具体次数后再加减"})
			return
		}
		next = left + *req.Delta
		if next < 0 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "剩余次数不能小于 0"})
			return
		}
	default:
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请填写调整次数"})
		return
	}
	if _, err := tx.Exec(`UPDATE licenses SET free_site_changes = ? WHERE id = ?`, next, licenseID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "调整失败"})
		return
	}
	if err := writeSiteChangeRecord(tx, licenseID, "license_site_change_adjust", "admin", contextUserID(c), "", c.ClientIP(), map[string]any{
		"before": left, "after": next,
	}); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "记录调整失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "提交失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已调整剩余更换次数", "data": gin.H{"freeSiteChanges": next}})
}

func AdminLicenseSiteChangeLogs(c *gin.Context) {
	licenseID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || licenseID <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "授权ID不正确"})
		return
	}
	db, err := config.DB()
	if err != nil || ensureSiteChangeSchema(db) != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "数据库连接失败"})
		return
	}
	rows, err := db.Query(`SELECT action, actor_type, actor_id, order_no, COALESCE(CAST(detail AS CHAR), ''), created_at
		FROM license_site_change_logs WHERE license_id = ? ORDER BY id DESC LIMIT 20`, licenseID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "查询更换记录失败"})
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var action, actor, orderNo, detail string
		var actorID int64
		var created time.Time
		if err := rows.Scan(&action, &actor, &actorID, &orderNo, &detail, &created); err != nil {
			continue
		}
		list = append(list, gin.H{
			"action": action, "actorType": actor, "actorId": actorID, "orderNo": orderNo,
			"detail": detail, "createdAt": created.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": gin.H{"list": list}})
}

func recordAdminLicenseSiteEdit(c *gin.Context, db *sql.DB, licenseID int64, oldDomain, newDomain string) {
	if db == nil || licenseID <= 0 || strings.TrimSpace(oldDomain) == "" {
		return
	}
	if normalizeLicenseDomain(oldDomain) == normalizeLicenseDomain(newDomain) {
		return
	}
	if ensureSiteChangeSchema(db) != nil {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if err := writeSiteChangeRecord(tx, licenseID, "license_site_change", "admin", contextUserID(c), "", c.ClientIP(), map[string]any{
		"action": "replace", "oldTarget": oldDomain, "newTarget": newDomain, "deducted": false,
	}); err != nil {
		return
	}
	_ = tx.Commit()
}
