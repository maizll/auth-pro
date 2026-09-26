package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type storeRateWindow struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (w *storeRateWindow) allow(key string, limit int, window time.Duration, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hits == nil {
		w.hits = map[string][]time.Time{}
	}
	kept := w.hits[key][:0]
	for _, at := range w.hits[key] {
		if now.Sub(at) < window {
			kept = append(kept, at)
		}
	}
	if len(kept) >= limit {
		w.hits[key] = kept
		return false
	}
	w.hits[key] = append(kept, now)
	return true
}

var (
	storeLoginRate                storeRateWindow
	storeOrderRate                storeRateWindow
	storeStatusRate               storeRateWindow
	storeTicketRate               storeRateWindow
	storeNonceMu                  sync.Mutex
	storeNonces                   = map[string]time.Time{}
	stationChallengePermitAltPort bool
)

func rememberStoreNonce(nonce string, now time.Time) bool {
	nonce = strings.TrimSpace(nonce)
	if len(nonce) < 16 {
		return false
	}
	storeNonceMu.Lock()
	defer storeNonceMu.Unlock()
	for key, exp := range storeNonces {
		if !now.Before(exp) {
			delete(storeNonces, key)
		}
	}
	if _, exists := storeNonces[nonce]; exists {
		return false
	}
	storeNonces[nonce] = now.Add(10 * time.Minute)
	return true
}

func storeFail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
}

func storeData(c *gin.Context, data gin.H) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "", "data": data})
}

func writeEditionRequired(c *gin.Context, feature string) {
	c.JSON(http.StatusOK, gin.H{
		"code": storeEditionRequiredCode,
		"msg":  storeEditionRequiredMsg,
		"data": gin.H{"feature": feature},
	})
}

func rejectStoreDomain(ctx context.Context, domain string) error {
	domain = normalizeLicenseDomain(domain)
	if storeDomainShapeRejected(domain) {
		return errStorePrivateDomain
	}
	ips, err := activeResolve(ctx, domain)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("无法解析域名 %s，请确认公网 DNS", domain)
	}
	for _, ip := range ips {
		if err := fetchIPAllowed(ip, false); err != nil {
			return errStorePrivateDomain
		}
	}
	return nil
}

func stationChallengeURL(domain, nonce string) (string, error) {
	domain = normalizeLicenseDomain(domain)
	if err := rejectStoreDomain(context.Background(), domain); err != nil {
		return "", err
	}
	nonce = strings.TrimSpace(nonce)
	if nonce == "" || strings.Contains(nonce, "/") {
		return "", errors.New("挑战无效")
	}
	return "https://" + domain + "/api/v1/public/station-verify/" + url.PathEscape(nonce), nil
}

func fetchStationChallengeAt(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("回调地址无效")
	}
	if port := parsed.Port(); port != "" && port != "443" && !stationChallengePermitAltPort {
		return "", errors.New("回调只接受 HTTPS 443")
	}
	if err := assertSafeFetchHost(ctx, parsed.Hostname(), false); err != nil {
		return "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	client := newSafeHTTPClient(safeFetchOptions{
		AllowPrivate: false,
		RequireHTTPS: true,
		Timeout:      5 * time.Second,
		MaxRedirects: 1,
	})
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errStoreNoRedirect
	}
	client.Timeout = 5 * time.Second
	response, err := client.Do(request)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		if errors.Is(err, errStoreNoRedirect) {
			return "", errStoreNoRedirect
		}
		return "", fmt.Errorf("源站无法通过 HTTPS 访问 %s", parsed.Host)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("域名校验失败，状态码 %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	if err != nil {
		return "", err
	}
	if len(payload) > 1024 {
		return "", errors.New("域名校验响应过大")
	}
	return strings.TrimSpace(string(payload)), nil
}

type storeLoginBody struct {
	Account    string `json:"account"`
	Password   string `json:"password"`
	Role       string `json:"role"`
	Domain     string `json:"domain"`
	InstallID  string `json:"installId"`
	AppVersion string `json:"appVersion"`
	geetestValidateParams
}

func StoreAuthCaptcha(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	cfg := loadGeetestConfig(db)
	storeData(c, gin.H{"enabled": cfg.Enabled, "captchaId": cfg.CaptchaID})
}

func StoreAuthLogin(c *gin.Context) {
	var req storeLoginBody
	if err := c.ShouldBindJSON(&req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	req.Account = strings.TrimSpace(req.Account)
	req.Role = strings.TrimSpace(req.Role)
	req.Domain = normalizeLicenseDomain(req.Domain)
	req.InstallID = strings.TrimSpace(req.InstallID)
	if req.Account == "" || req.Password == "" || (req.Role != "user" && req.Role != "agent") {
		storeFail(c, 400, "请填写账号、密码和身份")
		return
	}
	if !storeLoginRate.allow(c.ClientIP(), 20, time.Minute, time.Now()) {
		storeFail(c, 429, "登录尝试过多，请稍后再试")
		return
	}
	if err := rejectStoreDomain(c.Request.Context(), req.Domain); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	if remaining := middleware.LoginLockRemaining(c.ClientIP(), req.Account); remaining > 0 {
		storeFail(c, 429, fmt.Sprintf("登录尝试次数过多，请 %d 秒后重试", int(remaining.Seconds())+1))
		return
	}
	if pass, msg := verifyGeetestLogin(db, req.geetestValidateParams); !pass {
		storeFail(c, 403, msg)
		return
	}
	ownerID, display, authErr := storeAuthenticateAccount(db, req.Role, req.Account, req.Password, c.ClientIP())
	req.Password = ""
	if authErr != nil {
		storeFail(c, 401, authErr.Error())
		return
	}
	if !storeLoginRate.allow("challenge:"+req.Role+":"+strconv.FormatInt(ownerID, 10), 30, time.Hour, time.Now()) {
		storeFail(c, 429, "绑定尝试过多，请一小时后再试")
		return
	}
	nonceRaw := make([]byte, 32)
	if _, err := rand.Read(nonceRaw); err != nil {
		storeFail(c, 500, "创建挑战失败")
		return
	}
	nonce := hex.EncodeToString(nonceRaw)
	sum := sha256.Sum256([]byte(nonce))
	challengeID := "ch_" + randomHex(12)
	if _, err := db.Exec(`INSERT INTO store_bind_challenges
		(challenge_id, nonce, nonce_hash, owner_type, owner_id, domain, install_id, app_version, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		challengeID, nonce, hex.EncodeToString(sum[:]), req.Role, ownerID, req.Domain, req.InstallID, strings.TrimSpace(req.AppVersion),
		time.Now().Add(storeBindChallengeTTL)); err != nil {
		storeFail(c, 500, "创建挑战失败")
		return
	}
	storeData(c, gin.H{
		"challengeId": challengeID,
		"nonce":       nonce,
		"expiresIn":   int(storeBindChallengeTTL.Seconds()),
		"account":     gin.H{"name": display, "role": req.Role},
		"domain":      req.Domain,
	})
}

func StoreAuthConfirm(c *gin.Context) {
	var req struct {
		ChallengeID string `json:"challengeId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ChallengeID) == "" {
		storeFail(c, 400, "参数错误")
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	var ownerType, nonce, domain, installID, appVersion string
	var ownerID int64
	var expires time.Time
	var used sql.NullTime
	err = db.QueryRow(`SELECT owner_type, owner_id, nonce, domain, install_id, app_version, expires_at, used_at
		FROM store_bind_challenges WHERE challenge_id = ?`, strings.TrimSpace(req.ChallengeID)).
		Scan(&ownerType, &ownerID, &nonce, &domain, &installID, &appVersion, &expires, &used)
	if err != nil {
		storeFail(c, 400, "挑战不存在")
		return
	}
	if used.Valid || time.Now().After(expires) {
		storeFail(c, 400, "挑战已失效")
		return
	}
	rawURL, err := stationChallengeURL(domain, nonce)
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	receipt, err := fetchStationChallengeAt(c.Request.Context(), rawURL)
	if err != nil {
		_, _ = db.Exec(`UPDATE store_bind_challenges SET result = ? WHERE challenge_id = ?`, trimStoreText(err.Error(), 40), req.ChallengeID)
		storeFail(c, 400, err.Error())
		return
	}
	if !hmac.Equal([]byte(receipt), []byte(stationChallengeReceipt(installID, nonce))) {
		storeFail(c, 400, "域名校验未通过")
		return
	}
	settings, err := loadEffectiveStoreSettings(db)
	if err != nil {
		storeFail(c, 500, "源站未配置产品应用")
		return
	}
	appID, err := lookupEnabledStoreProductAppID(db, settings.ProductAppKey)
	if err != nil {
		storeFail(c, 500, err.Error())
		return
	}
	matches, err := listDomainLicenseMatches(db, appID, domain)
	if err != nil {
		storeFail(c, 500, "查询主授权失败")
		return
	}
	action, licenseID, decideErr := decideMainLicense(ownerType, ownerID, matches)
	if decideErr != nil {
		if errors.Is(decideErr, errStoreDomainOccupied) {
			storeFail(c, 409, fmt.Sprintf("域名 %s 已绑定在其他账号下。如需转移，请联系站长", domain))
			return
		}
		storeFail(c, 400, decideErr.Error()+"。请联系管理员改为单域名授权或新建")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		storeFail(c, 500, "绑定失败")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE store_bind_challenges SET used_at = NOW(), result = 'ok' WHERE challenge_id = ? AND used_at IS NULL`, req.ChallengeID)
	if err != nil {
		storeFail(c, 500, "绑定失败")
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		storeFail(c, 400, "挑战已失效")
		return
	}
	var licenseNo string
	if action == "create" {
		licenseNo, licenseID, err = insertFreeMainLicense(tx, appID, "", ownerType, ownerID, domain)
		if err != nil {
			storeFail(c, 500, "创建主授权失败")
			return
		}
	} else {
		if err := tx.QueryRow(`SELECT license_no FROM licenses WHERE id = ?`, licenseID).Scan(&licenseNo); err != nil {
			storeFail(c, 500, "读取主授权失败")
			return
		}
	}
	_, _ = tx.Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = 'relogin'
		WHERE license_id = ? AND install_id = ? AND status = 'active'`, licenseID, installID)
	bindingID := "sb_" + randomHex(12)
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		storeFail(c, 500, "绑定失败")
		return
	}
	if _, err := tx.Exec(`INSERT INTO store_bindings
		(binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, install_id, app_version, last_ip, status, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', NOW())`,
		bindingID, salt, ownerType, ownerID, licenseID, domain, installID, appVersion, c.ClientIP()); err != nil {
		storeFail(c, 500, "绑定失败")
		return
	}
	if err := tx.Commit(); err != nil {
		storeFail(c, 500, "绑定失败")
		return
	}
	master, err := loadOrCreateStoreFileKey("binding-master.key")
	if err != nil {
		storeFail(c, 500, "绑定失败")
		return
	}
	secret := deriveBindingSecret(master, bindingID, salt)
	display := storeAccountLabel(db, ownerType, ownerID)
	snapshot, err := buildStoreSnapshot(db, bindingID, licenseID, licenseNo, domain, settings)
	if err != nil {
		storeFail(c, 500, err.Error())
		return
	}
	storeData(c, gin.H{
		"bindingId":     bindingID,
		"bindingSecret": hex.EncodeToString(secret),
		"account":       gin.H{"name": display, "role": ownerType},
		"mainLicense":   gin.H{"licenseNo": licenseNo, "domain": domain, "edition": snapshot.Edition, "editionExpireAt": snapshot.EditionExpireAt, "licenseExpireAt": snapshot.LicenseExpireAt},
		"snapshot":      snapshot,
	})
}

func storeAuthenticateAccount(db *sql.DB, role, account, password, ip string) (int64, string, error) {
	if role == "agent" {
		var id int64
		var hash, name string
		err := db.QueryRow(`SELECT id, password_hash, name FROM agents WHERE (email = ? OR contact = ?) AND enabled = 1`, account, account).Scan(&id, &hash, &name)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			middleware.RecordLoginFailure(ip, account)
			return 0, "", errors.New("账号或密码错误")
		}
		middleware.RecordLoginSuccess(ip, account)
		if strings.TrimSpace(name) == "" {
			name = account
		}
		return id, name, nil
	}
	var id int64
	var hash, nickname, status string
	var enabled bool
	var converted sql.NullInt64
	var row *sql.Row
	if strings.Contains(account, "@") {
		row = db.QueryRow(`SELECT id, password_hash, nickname, enabled, account_status, converted_agent_id FROM users WHERE email = ?`, strings.ToLower(account))
	} else if uid, err := strconv.ParseUint(account, 10, 64); err == nil {
		row = db.QueryRow(`SELECT id, password_hash, nickname, enabled, account_status, converted_agent_id FROM users WHERE phone = ? OR id = ? ORDER BY (phone = ?) DESC LIMIT 1`, account, uid, account)
	} else {
		row = db.QueryRow(`SELECT id, password_hash, nickname, enabled, account_status, converted_agent_id FROM users WHERE phone = ?`, account)
	}
	if err := row.Scan(&id, &hash, &nickname, &enabled, &status, &converted); err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		middleware.RecordLoginFailure(ip, account)
		return 0, "", errors.New("账号或密码错误")
	}
	if !enabled || status == "converted" || converted.Valid {
		return 0, "", errors.New("该账号已升级为代理，请选择代理身份登录")
	}
	middleware.RecordLoginSuccess(ip, account)
	if strings.TrimSpace(nickname) == "" {
		nickname = account
	}
	return id, nickname, nil
}

func listDomainLicenseMatches(db *sql.DB, appID int64, domain string) ([]mainLicenseMatch, error) {
	rows, err := db.Query(`SELECT l.id, l.owner_type, l.owner_id, l.type, l.status, COALESCE(ld.domain, ''), COALESCE(ld.is_wildcard, 0)
		FROM licenses l LEFT JOIN license_domains ld ON ld.license_id = l.id WHERE l.app_id = ?`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	req := licenseVerifyRequest{Domain: domain}
	var matches []mainLicenseMatch
	for rows.Next() {
		var item mainLicenseMatch
		var target string
		var wildcard int
		if err := rows.Scan(&item.ID, &item.OwnerType, &item.OwnerID, &item.Type, &item.Status, &target, &wildcard); err != nil {
			continue
		}
		if licenseRowMatchesRequest(item.Type, normalizeLicenseTarget(target), wildcard == 1, req) {
			matches = append(matches, item)
		}
	}
	return matches, rows.Err()
}

func insertFreeMainLicense(tx *sql.Tx, appID int64, planID, ownerType string, ownerID int64, domain string) (string, int64, error) {
	licenseNo := fmt.Sprintf("LIC%d%s", time.Now().Unix(), randomHex(3))
	var plan any
	if n, err := strconv.ParseInt(strings.TrimSpace(planID), 10, 64); err == nil && n > 0 {
		plan = n
	}
	res, err := tx.Exec(`INSERT INTO licenses
		(license_no, app_id, plan_id, type, status, source, owner_type, owner_id, duration_days, started_at, expired_at, license_key, max_domains, remark)
		VALUES (?, ?, ?, 'domain', 'active', 'store_bind', ?, ?, 0, NOW(), NULL, '', 0, '商店绑定自动建立')`,
		licenseNo, appID, plan, ownerType, ownerID)
	if err != nil {
		return "", 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return "", 0, err
	}
	if _, err := tx.Exec(`INSERT INTO license_domains (license_id, domain, is_wildcard) VALUES (?, ?, 0)`, id, domain); err != nil {
		return "", 0, err
	}
	return licenseNo, id, nil
}

func openStoreDB(c *gin.Context) (*sql.DB, error) {
	db, err := config.DB()
	if err != nil {
		storeFail(c, 500, "数据库连接失败")
		return nil, err
	}
	if err := ensurePaidStoreSchema(db); err != nil {
		storeFail(c, 500, "初始化商店数据失败")
		return nil, err
	}
	if err := ensureSystemConfigStorage(db); err != nil {
		storeFail(c, 500, "初始化配置失败")
		return nil, err
	}
	return db, nil
}

func loadEffectiveStoreSettings(db *sql.DB) (sourceStoreSettings, error) {
	settings, err := loadSourceStoreSettings(db)
	if err != nil {
		return sourceStoreSettings{}, err
	}
	settings, err = normalizeStoreSettings(settings)
	if err != nil {
		return sourceStoreSettings{}, err
	}
	if err := prepareCommercialProduct(db); err != nil {
		return sourceStoreSettings{}, err
	}
	_, appKey, _, err := lookupCommercialProduct(db)
	if err != nil {
		return sourceStoreSettings{}, err
	}
	settings.ProductAppKey = strings.TrimSpace(appKey)
	settings.FreePlanID = ""
	return settings, nil
}

func storeAccountLabel(db *sql.DB, ownerType string, ownerID int64) string {
	var name string
	if ownerType == "agent" {
		_ = db.QueryRow(`SELECT name FROM agents WHERE id = ?`, ownerID).Scan(&name)
	} else {
		_ = db.QueryRow(`SELECT nickname FROM users WHERE id = ?`, ownerID).Scan(&name)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ownerType
	}
	return name
}

type storeBindingRecord struct {
	BindingID string
	Salt      []byte
	OwnerType string
	OwnerID   int64
	LicenseID int64
	Domain    string
	InstallID string
	Status    string
	LastSeen  sql.NullTime
}

func loadStoreBinding(db *sql.DB, bindingID string) (storeBindingRecord, error) {
	var row storeBindingRecord
	err := db.QueryRow(`SELECT binding_id, secret_salt, owner_type, owner_id, license_id, domain_snapshot, install_id, status, last_seen_at
		FROM store_bindings WHERE binding_id = ?`, bindingID).
		Scan(&row.BindingID, &row.Salt, &row.OwnerType, &row.OwnerID, &row.LicenseID, &row.Domain, &row.InstallID, &row.Status, &row.LastSeen)
	return row, err
}

func readSignedStoreBody(c *gin.Context) ([]byte, storeBindingRecord, bool) {
	bindingID := strings.TrimSpace(c.GetHeader("X-Store-Binding"))
	tsRaw := strings.TrimSpace(c.GetHeader("X-Store-Timestamp"))
	nonce := strings.TrimSpace(c.GetHeader("X-Store-Nonce"))
	signature := strings.TrimSpace(c.GetHeader("X-Store-Signature"))
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if bindingID == "" || err != nil || nonce == "" || signature == "" {
		storeFail(c, 401, "缺少绑定签名")
		return nil, storeBindingRecord{}, false
	}
	if absInt64(time.Now().Unix()-ts) > int64(storeSignWindow.Seconds()) {
		storeFail(c, 401, "签名已过期")
		return nil, storeBindingRecord{}, false
	}
	db, err := openStoreDB(c)
	if err != nil {
		return nil, storeBindingRecord{}, false
	}
	row, err := loadStoreBinding(db, bindingID)
	if err != nil {
		storeFail(c, 401, "绑定不存在")
		return nil, storeBindingRecord{}, false
	}
	if row.Status != "active" {
		reason := "binding_revoked"
		if row.Status == "expired" {
			reason = "binding_expired"
		}
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "绑定已失效", "data": gin.H{"reason": reason}})
		return nil, storeBindingRecord{}, false
	}
	if row.LastSeen.Valid && time.Since(row.LastSeen.Time) > storeBindingIdleTTL {
		_, _ = db.Exec(`UPDATE store_bindings SET status = 'expired', revoked_at = NOW(), revoke_reason = 'idle' WHERE binding_id = ? AND status = 'active'`, bindingID)
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "绑定已过期", "data": gin.H{"reason": "binding_expired"}})
		return nil, storeBindingRecord{}, false
	}
	master, err := loadOrCreateStoreFileKey("binding-master.key")
	if err != nil {
		storeFail(c, 500, "绑定密钥不可用")
		return nil, storeBindingRecord{}, false
	}
	secret := deriveBindingSecret(master, row.BindingID, row.Salt)
	expect := storeRequestSignature(secret, c.Request.Method, c.Request.URL.Path, ts, nonce, body)
	if !hmac.Equal([]byte(expect), []byte(signature)) {
		storeFail(c, 401, "签名不正确")
		return nil, storeBindingRecord{}, false
	}
	if !rememberStoreNonce(bindingID+":"+nonce, time.Now()) {
		storeFail(c, 401, "请求重复")
		return nil, storeBindingRecord{}, false
	}
	_, _ = db.Exec(`UPDATE store_bindings SET last_seen_at = NOW(), last_ip = ? WHERE binding_id = ?`, c.ClientIP(), bindingID)
	c.Set("storeBinding", row)
	c.Set("storeDB", db)
	return body, row, true
}

func StoreAuthLogout(c *gin.Context) {
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	db, _ := c.Get("storeDB")
	if _, err := db.(*sql.DB).Exec(`UPDATE store_bindings SET status = 'revoked', revoked_at = NOW(), revoke_reason = 'logout' WHERE binding_id = ?`, row.BindingID); err != nil {
		storeFail(c, 500, "解绑失败")
		return
	}
	storeData(c, gin.H{"revoked": true})
}

func StoreAuthRotate(c *gin.Context) {
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		storeFail(c, 500, "轮换失败")
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	if _, err := db.Exec(`UPDATE store_bindings SET secret_salt = ? WHERE binding_id = ? AND status = 'active'`, salt, row.BindingID); err != nil {
		storeFail(c, 500, "轮换失败")
		return
	}
	master, err := loadOrCreateStoreFileKey("binding-master.key")
	if err != nil {
		storeFail(c, 500, "轮换失败")
		return
	}
	secret := deriveBindingSecret(master, row.BindingID, salt)
	storeData(c, gin.H{"bindingId": row.BindingID, "bindingSecret": hex.EncodeToString(secret)})
}

func StoreStatus(c *gin.Context) {
	if !storeStatusRate.allow(c.GetHeader("X-Store-Binding"), 1, time.Minute, time.Now()) {
		storeFail(c, 429, "刷新过于频繁")
		return
	}
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	settings, err := loadEffectiveStoreSettings(db)
	if err != nil {
		storeFail(c, 500, "读取配置失败")
		return
	}
	var appID int64
	if err := db.QueryRow(`SELECT id FROM apps WHERE app_key = ?`, strings.TrimSpace(settings.ProductAppKey)).Scan(&appID); err != nil {
		storeFail(c, 500, "产品应用不存在")
		return
	}
	requestDomain := normalizeLicenseDomain(c.Query("domain"))
	if requestDomain == "" {
		requestDomain = row.Domain
	}
	license, reason, passed := evaluateLicenseForTarget(db, appID, requestDomain, "", "", "")
	if !passed {
		writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: license.ID > 0}, appID, requestDomain, "", c.ClientIP(), "fail", reason, "store-status")
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "主授权已失效", "data": gin.H{"reason": reason}})
		return
	}
	if license.ID != row.LicenseID {
		writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: true}, appID, requestDomain, "", c.ClientIP(), "fail", "license_not_found", "store-status")
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "主授权与绑定不一致", "data": gin.H{"reason": "license_not_found"}})
		return
	}
	var ownerType string
	var ownerID int64
	var licenseNo string
	if err := db.QueryRow(`SELECT owner_type, owner_id, license_no FROM licenses WHERE id = ?`, row.LicenseID).Scan(&ownerType, &ownerID, &licenseNo); err != nil || ownerType != row.OwnerType || ownerID != row.OwnerID {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "绑定已失效", "data": gin.H{"reason": "binding_revoked"}})
		return
	}
	writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: true}, appID, requestDomain, "", c.ClientIP(), "pass", "", "store-status")
	snapshot, err := buildStoreSnapshot(db, row.BindingID, row.LicenseID, licenseNo, row.Domain, settings)
	if err != nil {
		storeFail(c, 500, err.Error())
		return
	}
	storeData(c, gin.H{"snapshot": snapshot})
}

func buildStoreSnapshot(db *sql.DB, bindingID string, licenseID int64, licenseNo, domain string, settings sourceStoreSettings) (storeSnapshot, error) {
	var status string
	var expired sql.NullTime
	if err := db.QueryRow(`SELECT status, expired_at FROM licenses WHERE id = ?`, licenseID).Scan(&status, &expired); err != nil {
		return storeSnapshot{}, err
	}
	edition, period, editionExp, active := loadCommercialEdition(db, licenseID)
	features := []string{}
	if active {
		features = append(features, settings.CommercialFeatures...)
	} else {
		edition = storeEditionFree
		period = ""
		editionExp = nil
	}
	items := loadActiveEntitlements(db, licenseID)
	var licenseExp *int64
	if expired.Valid {
		unix := expired.Time.Unix()
		licenseExp = &unix
	}
	snapshot := storeSnapshot{
		BindingID: bindingID, LicenseNo: licenseNo, Domain: domain, LicenseStatus: status,
		LicenseExpireAt: licenseExp, Edition: edition, EditionPeriod: period, EditionExpireAt: editionExp,
		Features: features, Items: items, AllPaidItems: active, ServerTime: time.Now().Unix(), GraceDays: settings.GraceDays,
	}
	if snapshot.Features == nil {
		snapshot.Features = []string{}
	}
	if snapshot.Items == nil {
		snapshot.Items = []storeSnapshotItem{}
	}
	return signStoreSnapshot(snapshot)
}

func loadCommercialEdition(db *sql.DB, licenseID int64) (edition, period string, expire *int64, active bool) {
	var status, periodValue string
	var expires sql.NullTime
	err := db.QueryRow(`SELECT period, expires_at, status FROM main_license_editions
		WHERE license_id = ? AND edition = 'commercial' ORDER BY id DESC LIMIT 1`, licenseID).Scan(&periodValue, &expires, &status)
	if err != nil || status != "active" {
		return storeEditionFree, "", nil, false
	}
	if expires.Valid && !expires.Time.After(time.Now()) {
		return storeEditionFree, periodValue, nil, false
	}
	if expires.Valid {
		unix := expires.Time.Unix()
		expire = &unix
	}
	return storeEditionCommercial, periodValue, expire, true
}

func loadActiveEntitlements(db *sql.DB, licenseID int64) []storeSnapshotItem {
	rows, err := db.Query(`SELECT item_kind, item_id, period, expires_at FROM plugin_entitlements
		WHERE license_id = ? AND status = 'active'`, licenseID)
	if err != nil {
		return []storeSnapshotItem{}
	}
	defer rows.Close()
	items := []storeSnapshotItem{}
	now := time.Now()
	for rows.Next() {
		var item storeSnapshotItem
		var expires sql.NullTime
		if err := rows.Scan(&item.Kind, &item.ID, &item.Period, &expires); err != nil {
			continue
		}
		if expires.Valid && !expires.Time.After(now) {
			continue
		}
		item.Source = "entitlement"
		if expires.Valid {
			unix := expires.Time.Unix()
			item.ExpireAt = &unix
		}
		items = append(items, item)
	}
	return items
}

func trimStoreText(value string, n int) string {
	value = strings.TrimSpace(value)
	if len(value) <= n {
		return value
	}
	return value[:n]
}

func StorePayCompletePage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, "<!doctype html><meta charset=utf-8><title>支付完成</title><p>支付完成，请回到后台。本页不会跳转到其他网站。</p>")
}

func configValue(db *sql.DB, group, key string) string {
	var value string
	_ = db.QueryRow("SELECT value FROM system_configs WHERE `group` = ? AND `key` = ?", group, key).Scan(&value)
	return strings.TrimSpace(value)
}

func upsertConfigValue(db *sql.DB, group, key, value, description string) error {
	_, err := db.Exec("INSERT INTO system_configs (`group`, `key`, value, description) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)", group, key, value, description)
	return err
}

func paidCatalogPath() string {
	return filepath.Join(config.GetDataDir(), "store", "paid-catalog.json")
}

func savePaidCatalog(items []paidCatalogItem) {
	dir := filepath.Dir(paidCatalogPath())
	_ = os.MkdirAll(dir, 0750)
	payload, err := json.Marshal(items)
	if err != nil {
		return
	}
	_ = os.WriteFile(paidCatalogPath(), payload, 0600)
}

func loadPaidCatalog() []paidCatalogItem {
	payload, err := os.ReadFile(paidCatalogPath())
	if err != nil {
		return nil
	}
	var items []paidCatalogItem
	if json.Unmarshal(payload, &items) != nil {
		return nil
	}
	return items
}

type paidCatalogItem struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	PriceCents int64  `json:"priceCents"`
	Billing    string `json:"billing"`
	Delivery   string `json:"delivery"`
}
