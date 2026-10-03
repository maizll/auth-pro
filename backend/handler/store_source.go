// 源站侧的商店登录、域名挑战和绑定确认。买家的下单、查单和下载都要带签名进来。

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
	storeRegisterRate             storeRateWindow
	storeOrderRate                storeRateWindow
	storeStatusRate               storeRateWindow
	storeTicketRate               storeRateWindow
	stationChallengePermitAltPort bool
)

func rememberStoreNonce(nonce string, now time.Time) bool {
	nonce = strings.TrimSpace(nonce)
	if len(nonce) < 16 {
		return false
	}
	return requestNonces.remember("store:"+nonce, now)
}

func storeFail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
}

// storeTerminal 告诉买家这条授权或绑定已经不能再当商业版。revoked 必须是字段，不能靠中文判断。
func storeTerminal(c *gin.Context, code int, msg, reason string) {
	c.JSON(http.StatusOK, gin.H{
		"code": code,
		"msg":  msg,
		"data": gin.H{"reason": reason, "revoked": true},
	})
}

func storeLicenseGone(db *sql.DB, licenseID int64) bool {
	if db == nil || licenseID <= 0 {
		return false
	}
	var id int64
	err := db.QueryRow(`SELECT id FROM licenses WHERE id = ?`, licenseID).Scan(&id)
	return errors.Is(err, sql.ErrNoRows)
}

func storeStatusFailure(c *gin.Context, db *sql.DB, boundLicenseID int64, code int, msg, reason string) {
	if (reason == "license_not_found" || reason == "query_failed") && storeLicenseGone(db, boundLicenseID) {
		reason = "license_deleted"
	}
	if buyerSnapshotTerminal(reason, false) {
		storeTerminal(c, code, msg, reason)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg, "data": gin.H{"reason": reason}})
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

func paidItemRequiredMessage(kind string) string {
	if kind == "template" {
		return "该模板需要购买后才能启用"
	}
	return "该插件需要购买后才能启用"
}

func writePaidItemRequired(c *gin.Context, feature string, item paidCatalogItem) {
	kind := item.Kind
	if kind == "" {
		if feature == "paid_template" {
			kind = "template"
		} else {
			kind = "plugin"
		}
	}
	access := resolveCatalogAccess(currentBuyerAccess(c), kind, item.ID, item.PriceCents)
	c.JSON(http.StatusOK, gin.H{
		"code": storeEditionRequiredCode,
		"msg":  paidItemRequiredMessage(kind),
		"data": gin.H{
			"feature":      feature,
			"kind":         kind,
			"id":           item.ID,
			"name":         item.Name,
			"priceCents":   item.PriceCents,
			"period":       catalogSalePeriod(item.Billing),
			"purchaseOnly": access.Party == "third",
			"access":       access,
		},
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
	// ProductKey 是客户端打包时写入的应用标识。老客户端不带，落到接收老客户端的默认应用。
	ProductKey string `json:"productKey"`
	geetestValidateParams
}

// StoreAuthCaptcha 告诉买家登录前要不要做极验。
// 未启用时 enabled 为 false。读配置失败由 openStoreDB 写 500。
func StoreAuthCaptcha(c *gin.Context) {
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	cfg := loadGeetestConfig(db)
	storeData(c, gin.H{"enabled": cfg.Enabled, "captchaId": cfg.CaptchaID})
}

const (
	// 外部站点注册按 IP 限流。邮箱 1 分钟 1 次、验证码错 5 次作废仍在官网注册里，这里不另写一套。
	storeRegisterCodeLimit   = 5
	storeRegisterSubmitLimit = 8
	storeRegisterWindow      = 10 * time.Minute
)

// 商店注册直接调用官网注册，避免把校验和写库复制一份。测试可以换成空函数。
var (
	invokeRegisterEmailCode = UserSendRegisterEmailCode
	invokeRegister          = UserRegister
)

// StoreRegisterEmailCode 给买家站代发注册验证码。限流之后走官网的发信实现。
func StoreRegisterEmailCode(c *gin.Context) {
	if !storeRegisterRate.allow("code:"+c.ClientIP(), storeRegisterCodeLimit, storeRegisterWindow, time.Now()) {
		storeFail(c, 429, "验证码发得太频繁，请 10 分钟后再试")
		return
	}
	invokeRegisterEmailCode(c)
}

// StoreRegister 给买家站代注册。限流之后走官网的 UserRegister，字段和校验与官网相同。
func StoreRegister(c *gin.Context) {
	if !storeRegisterRate.allow("submit:"+c.ClientIP(), storeRegisterSubmitLimit, storeRegisterWindow, time.Now()) {
		storeFail(c, 429, "注册太频繁，请 10 分钟后再试")
		return
	}
	invokeRegister(c)
}

// StoreAuthLogin 校验账号密码和域名，成功后发一条短时挑战，还不创建绑定。
// 账号、密码或身份缺失，域名不合法，或登录锁定中，返回 400 或 429。
// 密码正确但域名已有其它有效绑定时返回 400，已删除或已吊销的旧记录不算占用。
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
	product, err := resolveStoreProductApp(db, req.ProductKey)
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	if !product.onSale() {
		storeFail(c, 400, errAppCommercialNotForSale.Error())
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
		(challenge_id, nonce, nonce_hash, owner_type, owner_id, app_id, domain, install_id, app_version, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		challengeID, nonce, hex.EncodeToString(sum[:]), req.Role, ownerID, product.AppID, req.Domain, req.InstallID, strings.TrimSpace(req.AppVersion),
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
		"product":     gin.H{"name": product.AppName},
	})
}

// StoreAuthConfirm 核对买家站回执后创建绑定，并返回签名过的快照和绑定密钥。
// challengeId 空、过期或回执对不上返回 400。签发快照失败返回 500，不留下半截绑定。
func StoreAuthConfirm(c *gin.Context) {
	var req struct {
		ChallengeID string `json:"challengeId"`
		// Nonce 和 ProductKey 是 1.8.7 起客户站带来的，只用来签 snapshotProof。应用仍以登录时记下的为准。
		Nonce      string `json:"nonce"`
		ProductKey string `json:"productKey"`
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
	var challengeApp sql.NullInt64
	var expires time.Time
	var used sql.NullTime
	err = db.QueryRow(`SELECT owner_type, owner_id, app_id, nonce, domain, install_id, app_version, expires_at, used_at
		FROM store_bind_challenges WHERE challenge_id = ?`, strings.TrimSpace(req.ChallengeID)).
		Scan(&ownerType, &ownerID, &challengeApp, &nonce, &domain, &installID, &appVersion, &expires, &used)
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
	// 确认只用登录时记下的应用。升级前发出的挑战没有应用，按接收老客户端的默认应用处理。
	var product appCommercial
	if challengeApp.Valid && challengeApp.Int64 > 0 {
		product, err = loadAppCommercial(db, challengeApp.Int64)
	} else {
		product, err = resolveStoreProductApp(db, "")
	}
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	if !product.onSale() {
		storeFail(c, 400, errAppCommercialNotForSale.Error())
		return
	}
	appID := product.AppID
	settings := product.storeSettings()
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
		(binding_id, secret_salt, owner_type, owner_id, license_id, app_id, domain_snapshot, install_id, app_version, last_ip, status, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', NOW())`,
		bindingID, salt, ownerType, ownerID, licenseID, appID, domain, installID, appVersion, c.ClientIP()); err != nil {
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
	// 确认响应带登录账号。没有邮箱或联系方式时，storeAccountLogin 会退回名称，买家再拿提交的账号兜底。
	display := storeAccountLogin(db, ownerType, ownerID)
	snapshot, err := buildStoreSnapshot(db, bindingID, licenseID, licenseNo, domain, settings)
	if err != nil {
		storeFail(c, 500, err.Error())
		return
	}
	data := gin.H{
		"bindingId":     bindingID,
		"bindingSecret": hex.EncodeToString(secret),
		"account":       gin.H{"name": display, "role": ownerType},
		"mainLicense":   gin.H{"licenseNo": licenseNo, "domain": domain, "edition": snapshot.Edition, "editionExpireAt": snapshot.EditionExpireAt, "licenseExpireAt": snapshot.LicenseExpireAt},
		"snapshot":      snapshot,
		"product":       gin.H{"name": product.AppName},
	}
	if !addStoreSnapshotProof(c, data, responseProofStoreBind, bindingID, req.ProductKey, req.Nonce, snapshot) {
		return
	}
	storeData(c, data)
}

// addStoreSnapshotProof 在快照响应里加 snapshotProof：用快照私钥把绑定号、应用标识、域名、请求方的随机数、服务器时间和快照签名一起签名。
// 客户站拿自己发出的随机数验签，假服务器伪造不了，重放旧响应也对不上。随机数不合规（老客户站确认时不带）就不加，老客户站本来也不看。
// 签不了名返回 false 并写 500，和快照签不了名一样不发响应。
func addStoreSnapshotProof(c *gin.Context, data gin.H, kind, binding, product, nonce string, snapshot storeSnapshot) bool {
	nonce = strings.TrimSpace(nonce)
	if !validProofNonce(nonce) {
		return true
	}
	key, err := loadStoreSnapshotPrivateKey()
	if err != nil {
		storeFail(c, 500, "签发快照失败")
		return false
	}
	data["snapshotProof"] = signResponseProofWith(key, kind, storeSnapshotProofFields(binding, strings.TrimSpace(product), nonce, snapshot))
	return true
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
	if planID, ok := plan.(int64); ok {
		if err := snapshotLicenseSiteChange(tx, id, planID); err != nil {
			return "", 0, err
		}
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
	AppID     int64
	Domain    string
	InstallID string
	Status    string
	LastSeen  sql.NullTime
}

func loadStoreBinding(db *sql.DB, bindingID string) (storeBindingRecord, error) {
	var row storeBindingRecord
	// 绑定上的 app_id 是刷新时判断应用的唯一依据。迁移前没回填到的行，按授权所属应用兜底。
	err := db.QueryRow(`SELECT b.binding_id, b.secret_salt, b.owner_type, b.owner_id, b.license_id,
		COALESCE(b.app_id, (SELECT l.app_id FROM licenses l WHERE l.id = b.license_id), 0),
		b.domain_snapshot, b.install_id, b.status, b.last_seen_at
		FROM store_bindings b WHERE b.binding_id = ?`, bindingID).
		Scan(&row.BindingID, &row.Salt, &row.OwnerType, &row.OwnerID, &row.LicenseID, &row.AppID, &row.Domain, &row.InstallID, &row.Status, &row.LastSeen)
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
		if errors.Is(err, sql.ErrNoRows) {
			storeTerminal(c, 401, "绑定不存在", "binding_deleted")
		} else {
			storeFail(c, 500, "查询绑定失败")
		}
		return nil, storeBindingRecord{}, false
	}
	if row.Status != "active" {
		reason := "binding_revoked"
		if row.Status == "expired" {
			reason = "binding_expired"
		}
		storeTerminal(c, 401, "绑定已失效", reason)
		return nil, storeBindingRecord{}, false
	}
	if row.LastSeen.Valid && time.Since(row.LastSeen.Time) > storeBindingIdleTTL {
		_, _ = db.Exec(`UPDATE store_bindings SET status = 'expired', revoked_at = NOW(), revoke_reason = 'idle' WHERE binding_id = ? AND status = 'active'`, bindingID)
		storeTerminal(c, 401, "绑定已过期", "binding_expired")
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
		rejectMismatchedStoreSignature(c)
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

// rejectMismatchedStoreSignature 表示本地拿来签名的令牌已经对不上源站。
// 这不是时钟偏差。买家应清掉旧令牌，让用户重新登录绑定。
func rejectMismatchedStoreSignature(c *gin.Context) {
	storeTerminal(c, 401, "绑定令牌已失效", "token_invalid")
}

// StoreBindingCheck 只核对签名和绑定是否还在。
// 不刷新商业版快照，也不占用状态接口每分钟一次的限制。
// 绑定已删除或令牌失效时由 readSignedStoreBody 返回终止原因。
func StoreBindingCheck(c *gin.Context) {
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	storeData(c, gin.H{"valid": true, "bindingId": row.BindingID})
}

// StoreAuthLogout 吊销当前签名对应的绑定。签名无效时 readSignedStoreBody 已写错误。
// 写库失败返回 500。买家收到 revoked 后应清本地快照。
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

// StoreAuthRotate 更换绑定密钥。旧密钥立即失效，调用方必须保存响应里的新密钥。
// 签名无效或绑定已吊销时拒绝。
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

// StoreStatus 按当前签名重新计算商业版快照。
// 权益按绑定账号名下、仍覆盖该域名的授权实时计算，不限于绑定时的那一条。
// 同一绑定一分钟最多 6 次，超出返回 429。账号授权走 idx_license_owner_app_status，权益走 idx_main_license_edition_lookup。
// 授权已删除或绑定已吊销时返回 400，并带 revoked，不进入离线宽限。
func StoreStatus(c *gin.Context) {
	if !storeStatusRate.allow(c.GetHeader("X-Store-Binding"), 6, time.Minute, time.Now()) {
		storeFail(c, 429, "刷新过于频繁")
		return
	}
	_, row, ok := readSignedStoreBody(c)
	if !ok {
		return
	}
	db := c.MustGet("storeDB").(*sql.DB)
	product, ok := storeBindingProduct(c, db, row)
	if !ok {
		return
	}
	appID := product.AppID
	settings := product.storeSettings()
	requestDomain := normalizeLicenseDomain(c.Query("domain"))
	if requestDomain == "" {
		requestDomain = row.Domain
	}
	license, reason, passed := evaluateLicenseForTarget(db, appID, requestDomain, "", "", "")
	// 绑定自己的授权仍覆盖这个域名时，就用它重新签发。后台后来另加的同域名授权不能把刷新打成重新绑定。
	matchedID := license.ID
	if !passed {
		matchedID = 0
	}
	_, terminal := storeStatusLicenseID(row.LicenseID, matchedID, boundLicenseCoversDomain(db, row.LicenseID, appID, requestDomain))
	if terminal != "" {
		if !passed {
			writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: license.ID > 0}, appID, requestDomain, "", c.ClientIP(), "fail", reason, "store-status")
			storeStatusFailure(c, db, row.LicenseID, 403, "主授权已失效", reason)
			return
		}
		mismatch := terminal
		if storeLicenseGone(db, row.LicenseID) {
			mismatch = "license_deleted"
		}
		writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: true}, appID, requestDomain, "", c.ClientIP(), "fail", mismatch, "store-status")
		storeTerminal(c, 403, "主授权与绑定不一致", mismatch)
		return
	}
	var ownerType string
	var ownerID int64
	var licenseNo string
	if err := db.QueryRow(`SELECT owner_type, owner_id, license_no FROM licenses WHERE id = ?`, row.LicenseID).Scan(&ownerType, &ownerID, &licenseNo); err != nil || ownerType != row.OwnerType || ownerID != row.OwnerID {
		if errors.Is(err, sql.ErrNoRows) {
			storeTerminal(c, 401, "绑定已失效", "license_deleted")
			return
		}
		storeTerminal(c, 401, "绑定已失效", "binding_revoked")
		return
	}
	writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: true}, appID, requestDomain, "", c.ClientIP(), "pass", "", "store-status")
	snapshot, err := buildStoreSnapshot(db, row.BindingID, row.LicenseID, licenseNo, row.Domain, settings)
	if err != nil {
		storeFail(c, 500, err.Error())
		return
	}
	// 刷新时带上当前绑定的登录账号，买家快照里没有名字的旧数据可以补上。
	data := gin.H{
		"snapshot": snapshot,
		"account":  gin.H{"name": storeAccountLogin(db, ownerType, ownerID), "role": ownerType},
		// 来源不进签名快照。1.7.1 客户站按结构体重算签名，多一个字段会验签失败。
		"editionSource": loadCommercialEditionSource(db, row.LicenseID),
	}
	if !addStoreSnapshotProof(c, data, responseProofStoreStat, row.BindingID, c.GetHeader(storeProductHeader), c.GetHeader("X-Store-Nonce"), snapshot) {
		return
	}
	storeData(c, data)
}

// storeAccountLogin 取绑定账号用来展示。用户优先邮箱，代理优先邮箱或联系方式，都没有再用名称。
func storeAccountLogin(db *sql.DB, ownerType string, ownerID int64) string {
	var value sql.NullString
	var err error
	if ownerType == "agent" {
		err = db.QueryRow(`SELECT COALESCE(NULLIF(email, ''), NULLIF(contact, ''), name) FROM agents WHERE id = ?`, ownerID).Scan(&value)
	} else {
		err = db.QueryRow(`SELECT COALESCE(NULLIF(email, ''), nickname) FROM users WHERE id = ?`, ownerID).Scan(&value)
	}
	if err != nil || !value.Valid || strings.TrimSpace(value.String) == "" {
		return storeAccountLabel(db, ownerType, ownerID)
	}
	return strings.TrimSpace(value.String)
}

func buildStoreSnapshot(db *sql.DB, bindingID string, licenseID int64, licenseNo, domain string, settings sourceStoreSettings) (storeSnapshot, error) {
	var status string
	var expired sql.NullTime
	if err := db.QueryRow(`SELECT status, expired_at FROM licenses WHERE id = ?`, licenseID).Scan(&status, &expired); err != nil {
		return storeSnapshot{}, err
	}
	edition, period, editionExp, active := loadSnapshotCommercialEdition(db, licenseID, domain)
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

// loadSnapshotCommercialEdition 给刷新快照用。
// 先看绑定账号在同一产品应用下、仍覆盖该域名的全部有效授权，取未过期商业版里到期最晚的一条。
// 绑定时那条授权没有权益、后台后来另开的授权有权益时，刷新即为商业版。全部吊销或改回免费后即为免费版。
func loadSnapshotCommercialEdition(db *sql.DB, licenseID int64, domain string) (edition, period string, expire *int64, active bool) {
	var ownerType string
	var ownerID, appID int64
	err := db.QueryRow(`SELECT owner_type, owner_id, app_id FROM licenses WHERE id = ?`, licenseID).Scan(&ownerType, &ownerID, &appID)
	if err != nil || strings.TrimSpace(domain) == "" {
		return loadCommercialEdition(db, licenseID)
	}
	edition, period, expire, active = loadAccountCommercialEdition(db, ownerType, ownerID, appID, domain)
	if active {
		return edition, period, expire, active
	}
	return loadCommercialEdition(db, licenseID)
}

// loadAccountCommercialEdition 在账号的有效授权里挑选商业版。
// 永久权益优先；否则取得到期时间更晚的一条。没有覆盖该域名的商业版时返回免费版。
func loadAccountCommercialEdition(db *sql.DB, ownerType string, ownerID, appID int64, domain string) (edition, period string, expire *int64, active bool) {
	domain = normalizeLicenseDomain(domain)
	if db == nil || ownerType == "" || ownerID <= 0 || appID <= 0 || domain == "" {
		return storeEditionFree, "", nil, false
	}
	rows, err := db.Query(`SELECT id FROM licenses WHERE owner_type = ? AND owner_id = ? AND app_id = ? AND status = 'active'`, ownerType, ownerID, appID)
	if err != nil {
		return storeEditionFree, "", nil, false
	}
	defer rows.Close()
	found := false
	var bestPeriod string
	var bestExpire *int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if !boundLicenseCoversDomain(db, id, appID, domain) {
			continue
		}
		_, itemPeriod, itemExpire, itemActive := loadCommercialEdition(db, id)
		if !itemActive {
			continue
		}
		if !found || commercialEditionPreferred(itemExpire, bestExpire) {
			found = true
			bestPeriod = itemPeriod
			bestExpire = itemExpire
		}
	}
	if err := rows.Err(); err != nil || !found {
		return storeEditionFree, "", nil, false
	}
	return storeEditionCommercial, bestPeriod, bestExpire, true
}

// commercialEditionPreferred 判断候选权益是否比当前选择更长。空到期表示永久。
func commercialEditionPreferred(candidate, current *int64) bool {
	if candidate == nil {
		return true
	}
	if current == nil {
		return false
	}
	return *candidate > *current
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

// StorePayCompletePage 是支付完成后的静态页。
// 不根据参数跳转，避免回调里的地址把用户带到站外。
func StorePayCompletePage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, "<!doctype html><meta charset=utf-8><title>支付完成</title><p>支付完成，请回到后台。本页不会跳转到其他网站。</p>")
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
	Kind               string `json:"kind"`
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Version            string `json:"version"`
	PriceCents         int64  `json:"priceCents"`
	Billing            string `json:"billing"`
	Delivery           string `json:"delivery"`
	PurchaseOnly       bool   `json:"purchaseOnly"`
	Party              string `json:"party"`
	CommercialIncluded bool   `json:"commercialIncluded"`
}
