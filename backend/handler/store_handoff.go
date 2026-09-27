package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// 一次性登录链接的有效期。落在要求的 2～5 分钟里。
	storeHandoffTTL = 3 * time.Minute
	// 换到的登录状态短于普通密码登录的 7 天，只够这次去「我的授权」办事。
	storeHandoffSessionTTL = 2 * time.Hour
	storeHandoffIssueIP    = 10
	storeHandoffIssueBind  = 5
	storeHandoffConsumeIP  = 20
	storeHandoffWindow     = 10 * time.Minute
)

var (
	storeHandoffRate       storeRateWindow
	storeHandoffPublicBase = func() string { return "https://auth.maizll.com" }
)

// storeHandoffPaths 只允许进入用户中心或代理端的「我的授权」。不接受调用方传入的跳转地址。
func storeHandoffPaths(ownerType string) (entry, dest string, ok bool) {
	switch ownerType {
	case "user":
		return "/user/handoff", "/user/licenses", true
	case "agent":
		return "/agent-panel/handoff", "/agent-panel/licenses", true
	default:
		return "", "", false
	}
}

func hashStoreHandoffToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func handoffTokenShape(token string) bool {
	if len(token) != 64 {
		return false
	}
	for _, ch := range token {
		switch {
		case ch >= '0' && ch <= '9', ch >= 'a' && ch <= 'f':
		default:
			return false
		}
	}
	return true
}

func handoffClaimable(used bool, expires, now time.Time) bool {
	return !used && now.Before(expires)
}

// buildStoreHandoffURL 把随机票据放在网址片段里。片段不会发给源站访问日志。
func buildStoreHandoffURL(base, ownerType, token string) (string, error) {
	entry, _, ok := storeHandoffPaths(ownerType)
	if !ok || !handoffTokenShape(token) {
		return "", errStoreHandoffURL
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errStoreHandoffURL
	}
	parsed.Path = entry
	parsed.RawQuery = ""
	parsed.Fragment = token
	return parsed.String(), nil
}

var errStoreHandoffURL = errString("登录链接无效")

type errString string

func (e errString) Error() string { return string(e) }

// buyerManageLinkAllowed 只接受源站自己的交接页，并且票据在片段里，查询参数必须为空。
func buyerManageLinkAllowed(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errStoreHandoffURL
	}
	base, err := buyerSourceBase()
	if err != nil {
		return "", err
	}
	baseURL, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, baseURL.Host) {
		return "", errStoreHandoffURL
	}
	switch parsed.EscapedPath() {
	case "/user/handoff", "/agent-panel/handoff":
	default:
		return "", errStoreHandoffURL
	}
	if parsed.RawQuery != "" || !handoffTokenShape(parsed.Fragment) {
		return "", errStoreHandoffURL
	}
	return parsed.String(), nil
}

func newStoreHandoffToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	return raw, hashStoreHandoffToken(raw), nil
}

// StoreLoginHandoffIssue 给已绑定的客户站签发一次性登录链接。
// 同一 IP、同一绑定都有次数限制。票据原文只出现在返回的网址片段里。
func StoreLoginHandoffIssue(c *gin.Context) {
	if !storeHandoffRate.allow("issue-ip:"+c.ClientIP(), storeHandoffIssueIP, storeHandoffWindow, time.Now()) {
		storeFail(c, 429, "打开太频繁，请稍后再试")
		return
	}
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	if !storeHandoffRate.allow("issue-bind:"+row.BindingID, storeHandoffIssueBind, storeHandoffWindow, time.Now()) {
		storeFail(c, 429, "打开太频繁，请稍后再试")
		return
	}
	if _, _, ok := storeHandoffPaths(row.OwnerType); !ok {
		storeFail(c, 400, "这个绑定不能打开我的授权")
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	if err := migrateStoreLoginHandoff(db); err != nil {
		storeFail(c, 500, "暂时打不开我的授权，请稍后再试")
		return
	}
	raw, hash, err := newStoreHandoffToken()
	if err != nil {
		storeFail(c, 500, "暂时打不开我的授权，请稍后再试")
		return
	}
	if _, err := db.Exec(`INSERT INTO store_login_handoffs
		(token_hash, binding_id, owner_type, owner_id, expires_at, created_ip)
		VALUES (?, ?, ?, ?, ?, ?)`,
		hash, row.BindingID, row.OwnerType, row.OwnerID, time.Now().Add(storeHandoffTTL), c.ClientIP()); err != nil {
		storeFail(c, 500, "暂时打不开我的授权，请稍后再试")
		return
	}
	_, _ = db.Exec(`DELETE FROM store_login_handoffs WHERE expires_at < DATE_SUB(NOW(), INTERVAL 1 DAY)`)
	link, err := buildStoreHandoffURL(storeHandoffPublicBase(), row.OwnerType, raw)
	raw = ""
	if err != nil {
		storeFail(c, 500, "暂时打不开我的授权，请稍后再试")
		return
	}
	storeData(c, gin.H{"url": link})
}

// StoreLoginHandoffConsume 用一次性票据换成短时登录状态。用过或过期都作废。
// 跳转地址只由绑定身份决定，请求体里的其它字段一律忽略。票据不写日志。
func StoreLoginHandoffConsume(c *gin.Context) {
	if !storeHandoffRate.allow("consume-ip:"+c.ClientIP(), storeHandoffConsumeIP, storeHandoffWindow, time.Now()) {
		storeFail(c, 429, "尝试太多，请稍后再试")
		return
	}
	var req struct {
		Ticket string `json:"ticket"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		storeFail(c, 400, "链接无效或已过期")
		return
	}
	ticket := strings.TrimSpace(req.Ticket)
	req.Ticket = ""
	if !handoffTokenShape(ticket) {
		storeFail(c, 400, "链接无效或已过期")
		return
	}
	db, err := config.DB()
	if err != nil {
		storeFail(c, 500, "暂时不能登录，请稍后再试")
		return
	}
	if err := migrateStoreLoginHandoff(db); err != nil {
		storeFail(c, 500, "暂时不能登录，请稍后再试")
		return
	}
	hash := hashStoreHandoffToken(ticket)
	ticket = ""
	tx, err := db.Begin()
	if err != nil {
		storeFail(c, 500, "暂时不能登录，请稍后再试")
		return
	}
	defer tx.Rollback()
	var id uint64
	var ownerType string
	var ownerID int64
	var expires time.Time
	var used sql.NullTime
	err = tx.QueryRow(`SELECT id, owner_type, owner_id, expires_at, used_at
		FROM store_login_handoffs WHERE token_hash = ? FOR UPDATE`, hash).
		Scan(&id, &ownerType, &ownerID, &expires, &used)
	if err != nil || !handoffClaimable(used.Valid, expires, time.Now()) {
		storeFail(c, 400, "链接无效或已过期")
		return
	}
	res, err := tx.Exec(`UPDATE store_login_handoffs SET used_at = NOW() WHERE id = ? AND used_at IS NULL`, id)
	if err != nil {
		storeFail(c, 500, "暂时不能登录，请稍后再试")
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		storeFail(c, 400, "链接无效或已过期")
		return
	}
	_, dest, ok := storeHandoffPaths(ownerType)
	if !ok {
		storeFail(c, 400, "链接无效或已过期")
		return
	}
	token, name, email, burn, err := signStoreHandoffSession(db, ownerType, ownerID)
	if err != nil {
		if burn {
			_ = tx.Commit()
		}
		storeFail(c, 400, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		storeFail(c, 500, "暂时不能登录，请稍后再试")
		return
	}
	storeData(c, gin.H{
		"accessToken": token,
		"role":        ownerType,
		"path":        dest,
		"nickname":    name,
		"email":       email,
	})
}

func storeHandoffClaims(id uint, email, role string, now time.Time) *middleware.Claims {
	return &middleware.Claims{
		UserID:   id,
		Username: email,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(storeHandoffSessionTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
}

// signStoreHandoffSession 签发用户或代理商自己的短时登录状态，不带管理员代登录标记。
// burn 为真表示票据应当作废（账号不存在或已停用）；查询本身失败时不作废，方便再试一次。
func signStoreHandoffSession(db *sql.DB, ownerType string, ownerID int64) (token, name, email string, burn bool, err error) {
	if ownerID <= 0 {
		return "", "", "", true, errString("链接无效或已过期")
	}
	var enabled bool
	switch ownerType {
	case "user":
		var accountStatus string
		var converted sql.NullInt64
		err = db.QueryRow(`SELECT email, nickname, enabled, account_status, converted_agent_id FROM users WHERE id = ?`, ownerID).
			Scan(&email, &name, &enabled, &accountStatus, &converted)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (!enabled || accountStatus == "converted" || converted.Valid)) {
			return "", "", "", true, errString("这个账号目前不能登录，请联系管理员")
		}
		if err != nil {
			return "", "", "", false, errString("暂时不能登录，请稍后再试")
		}
	case "agent":
		err = db.QueryRow(`SELECT email, name, enabled FROM agents WHERE id = ?`, ownerID).
			Scan(&email, &name, &enabled)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !enabled) {
			return "", "", "", true, errString("这个账号目前不能登录，请联系管理员")
		}
		if err != nil {
			return "", "", "", false, errString("暂时不能登录，请稍后再试")
		}
	default:
		return "", "", "", true, errString("链接无效或已过期")
	}
	now := time.Now()
	claims := storeHandoffClaims(uint(ownerID), email, ownerType, now)
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(middleware.JWTSecret())
	if err != nil {
		return "", "", "", false, errString("暂时不能登录，请稍后再试")
	}
	return token, strings.TrimSpace(name), email, false, nil
}
