package handler

import (
	"crypto/hmac"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
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
	// licenseVerifyRateAttempts 是同一 IP + app_key 在滑动窗口内允许的公开校验次数。
	// 业务常在每次请求里校验。1200 次/分钟挡住空转把 verify_logs 打满，同时给单台应用服务器留出余量。
	licenseVerifyRateAttempts = 1200
	licenseVerifyRateWindow   = time.Minute
	licenseVerifyRatePruneAt  = 1024
)

type licenseVerifyRateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func newLicenseVerifyRateLimiter(limit int, window time.Duration) *licenseVerifyRateLimiter {
	return &licenseVerifyRateLimiter{limit: limit, window: window, hits: make(map[string][]time.Time)}
}

// allow 按时间戳做滑动窗口，超限的请求不记入窗口，避免拒绝本身把窗口永远撑满。
func (limiter *licenseVerifyRateLimiter) allow(key string, now time.Time) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.hits) > licenseVerifyRatePruneAt {
		limiter.pruneExpired(now)
	}
	cutoff := now.Add(-limiter.window)
	kept := make([]time.Time, 0, limiter.limit)
	for _, hit := range limiter.hits[key] {
		if hit.After(cutoff) {
			kept = append(kept, hit)
		}
	}
	if len(kept) >= limiter.limit {
		if len(kept) == 0 {
			delete(limiter.hits, key)
		} else {
			limiter.hits[key] = kept
		}
		return false
	}
	limiter.hits[key] = append(kept, now)
	return true
}

func (limiter *licenseVerifyRateLimiter) pruneExpired(now time.Time) {
	cutoff := now.Add(-limiter.window)
	for key, hits := range limiter.hits {
		kept := hits[:0]
		for _, hit := range hits {
			if hit.After(cutoff) {
				kept = append(kept, hit)
			}
		}
		if len(kept) == 0 {
			delete(limiter.hits, key)
			continue
		}
		limiter.hits[key] = kept
	}
}

func licenseVerifyRateKey(clientIP, appKey string) string {
	return clientIP + "\x00" + appKey
}

var licenseVerifyLimiter = newLicenseVerifyRateLimiter(licenseVerifyRateAttempts, licenseVerifyRateWindow)

func licenseVerifyUnsignedFailureBody() gin.H {
	return gin.H{
		"code": 403,
		"msg":  "授权校验失败",
		"data": gin.H{"result": "fail", "reason": "verify_failed"},
	}
}

type licenseVerifyRequest struct {
	AppKey      string `json:"appKey" binding:"required"`
	Domain      string `json:"domain"`
	ServerIP    string `json:"serverIp"`
	LicenseKey  string `json:"licenseKey"`
	Timestamp   int64  `json:"timestamp" binding:"required"`
	SignVersion string `json:"signVersion"`
	Nonce       string `json:"nonce"`
	Sign        string `json:"sign" binding:"required"`
}

type matchedLicense struct {
	ID        int64
	PlanID    int64
	PlanName  string
	Type      string
	Status    string
	ExpiredAt sql.NullTime
}

var (
	appLicenseRequiredMu sync.Mutex
	appLicenseRequiredOK bool
)

func ensureAppLicenseRequiredColumn(db *sql.DB) error {
	appLicenseRequiredMu.Lock()
	defer appLicenseRequiredMu.Unlock()
	if appLicenseRequiredOK {
		return nil
	}

	var exists int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apps'
		  AND COLUMN_NAME = 'license_required'
	`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		_, err := db.Exec(`
			ALTER TABLE apps
			ADD COLUMN license_required TINYINT(1) NOT NULL DEFAULT 1
			COMMENT '是否要求授权验证: 1要求 0免授权' AFTER enabled
		`)
		if err != nil {
			var mysqlErr *mysql.MySQLError
			if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
				return err
			}
		}
	}
	appLicenseRequiredOK = true
	return nil
}

// LicenseVerify 公开授权校验接口，供业务系统 SDK 调用。
// LicenseVerify 处理 POST /api/license/verify。
// 请求要有 appKey、签名、时间戳和域名或机器标识。签名不对时和「应用不存在」用同一种响应，避免探测出应用是否存在。
// 时间戳超出 10 分钟返回 403。应用关闭授权开关时直接通过，并记一笔日志。
// 授权被拉黑、吊销、过期或域名不匹配时返回对应 reason。只有完全找不到授权才记盗版，过期客户不算盗版。
// v3 请求（signVersion=v3 + nonce）：10 分钟内同一 nonce 只认一次；每个响应都带 data.proof（Ed25519 签名），
// 覆盖 appKey、域名、服务器 IP、授权码哈希、nonce、服务器时间和结果。v1/v2 老 SDK 的响应保持原样。
func LicenseVerify(c *gin.Context) {
	var req licenseVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误", "data": gin.H{"result": "fail", "reason": "bad_request"}})
		return
	}

	rawDomain := strings.TrimSpace(req.Domain)
	rawServerIP := strings.TrimSpace(req.ServerIP)
	rawLicenseKey := strings.TrimSpace(req.LicenseKey)

	req.AppKey = strings.TrimSpace(req.AppKey)
	req.Domain = normalizeLicenseDomain(rawDomain)
	req.ServerIP = normalizeLicenseServerIP(rawServerIP)
	req.LicenseKey = rawLicenseKey
	req.Nonce = strings.TrimSpace(req.Nonce)
	req.Sign = strings.ToLower(strings.TrimSpace(req.Sign))
	respond := licenseVerifyResponder(c, req)
	if req.AppKey == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "参数错误", "data": gin.H{"result": "fail", "reason": "bad_request"}})
		return
	}
	if !licenseVerifyLimiter.allow(licenseVerifyRateKey(c.ClientIP(), req.AppKey), time.Now()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "msg": "请求过于频繁，请稍后再试", "data": gin.H{"result": "fail", "reason": "rate_limited"}})
		return
	}

	db, err := config.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "系统未配置", "data": gin.H{"result": "fail", "reason": "system_not_configured"}})
		return
	}
	if err := ensureAppLicenseRequiredColumn(db); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "初始化应用授权开关失败", "data": gin.H{"result": "fail", "reason": "schema_init_failed"}})
		return
	}

	var appID int64
	var appName, appSecret string
	var licenseRequired bool
	err = db.QueryRow("SELECT id, app_name, app_secret, license_required FROM apps WHERE app_key = ? AND enabled = 1", req.AppKey).Scan(&appID, &appName, &appSecret, &licenseRequired)
	if err != nil {
		c.JSON(http.StatusOK, licenseVerifyUnsignedFailureBody())
		return
	}

	signVersion, signValid := licenseVerifySignValid(req, appSecret, rawDomain, rawServerIP, rawLicenseKey)
	// 签名失败和未知应用返回同一类响应，不告诉调用方应用存不存在。
	if !signValid {
		writeVerifyLog(db, sql.NullInt64{}, appID, req.Domain, req.ServerIP, c.ClientIP(), "fail", "invalid_sign", c.GetHeader("User-Agent"))
		c.JSON(http.StatusOK, licenseVerifyUnsignedFailureBody())
		return
	}
	req.SignVersion = signVersion

	if req.Timestamp <= 0 || absInt64(time.Now().Unix()-req.Timestamp) > 600 {
		writeVerifyLog(db, sql.NullInt64{}, appID, req.Domain, req.ServerIP, c.ClientIP(), "fail", "invalid_timestamp", c.GetHeader("User-Agent"))
		respond(http.StatusOK, gin.H{"code": 403, "msg": "请求已过期", "data": gin.H{"result": "fail", "reason": "invalid_timestamp"}})
		return
	}
	if signVersion == licenseSignVersionV3 && !requestNonces.remember("license:"+req.AppKey+":"+req.Nonce, time.Now()) {
		writeVerifyLog(db, sql.NullInt64{}, appID, req.Domain, req.ServerIP, c.ClientIP(), "fail", "replayed_request", c.GetHeader("User-Agent"))
		respond(http.StatusOK, gin.H{"code": 403, "msg": "重复的校验请求", "data": gin.H{"result": "fail", "reason": "replayed_request"}})
		return
	}

	signTarget := licenseVerifySignTarget(req)
	if signTarget == "" {
		writeVerifyLog(db, sql.NullInt64{}, appID, req.Domain, req.ServerIP, c.ClientIP(), "fail", "empty_target", c.GetHeader("User-Agent"))
		respond(http.StatusOK, gin.H{"code": 403, "msg": "授权目标不能为空", "data": gin.H{"result": "fail", "reason": "empty_target"}})
		return
	}

	if isLicenseTargetBlacklisted(db, appID, req.Domain, req.ServerIP) {
		writeVerifyLog(db, sql.NullInt64{}, appID, req.Domain, req.ServerIP, c.ClientIP(), "blacklisted", "target_blacklisted", c.GetHeader("User-Agent"))
		respond(http.StatusOK, licenseVerifyFailureBody("target_blacklisted"))
		return
	}

	if !licenseRequired {
		writeVerifyLog(db, sql.NullInt64{}, appID, req.Domain, req.ServerIP, c.ClientIP(), "pass", "license_not_required", c.GetHeader("User-Agent"))
		respond(http.StatusOK, gin.H{
			"code": 200,
			"msg":  "应用无需授权验证",
			"data": gin.H{
				"result":          "pass",
				"appName":         appName,
				"licenseRequired": false,
			},
		})
		return
	}

	license, reason, ok := evaluateLicenseForTarget(db, appID, req.Domain, req.ServerIP, req.LicenseKey, req.SignVersion)
	if !ok {
		logResult := "fail"
		if reason == "target_blacklisted" {
			logResult = "blacklisted"
		}
		if reason == "license_expired" {
			logResult = "expired"
		}
		licenseID := sql.NullInt64{}
		if license.ID > 0 {
			licenseID = sql.NullInt64{Int64: license.ID, Valid: true}
		}
		writeVerifyLog(db, licenseID, appID, req.Domain, req.ServerIP, c.ClientIP(), logResult, reason, c.GetHeader("User-Agent"))
		// 过期、吊销、域名不符都不是盗版。只有库里没有这条授权才记一次命中。
		if reason == "license_not_found" && isPiracyDetectionEnabled() {
			recordPiracyHit(db, appID, req.Domain, req.ServerIP)
		}
		respond(http.StatusOK, licenseVerifyFailureBody(reason))
		return
	}

	writeVerifyLog(db, sql.NullInt64{Int64: license.ID, Valid: true}, appID, req.Domain, req.ServerIP, c.ClientIP(), "pass", "", c.GetHeader("User-Agent"))
	respond(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "授权有效",
		"data": licenseVerifySuccessData(appName, license),
	})
}

// licenseVerifyResponder 返回写响应的函数。只有请求签名核对通过之后的结果才经过这里签名：
// 参数错误、应用不存在、请求签名不对、限流和服务端故障都不签名，SDK 当成「暂时连不上」走离线宽限，
// 这样转发请求的假服务器也拿不到带签名的拒绝去顶掉客户的离线宽限。
// 请求是 v3 且 nonce 合法时给 data 加 proof；签不了名（私钥不可用）时
// 不发未签名的结果，统一改成 500，SDK 按「连不上授权站」进离线宽限，不会把伪造的通过当真。
func licenseVerifyResponder(c *gin.Context, req licenseVerifyRequest) func(int, gin.H) {
	wantProof := normalizeLicenseSignVersion(req.SignVersion) == licenseSignVersionV3 && validProofNonce(req.Nonce)
	return func(status int, body gin.H) {
		if !wantProof {
			c.JSON(status, body)
			return
		}
		data, _ := body["data"].(gin.H)
		if data == nil {
			data = gin.H{}
			body["data"] = data
		}
		proof, err := licenseVerifyProof(req, data, time.Now().Unix())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "授权站签名密钥不可用", "data": gin.H{"result": "fail", "reason": "response_sign_unavailable"}})
			return
		}
		data["proof"] = proof
		c.JSON(status, body)
	}
}

// licenseVerifyProof 生成 data.proof。域名和 IP 用服务端规范化后的值，授权码只放 SHA-256（空授权码为空串）。
func licenseVerifyProof(req licenseVerifyRequest, data gin.H, serverTime int64) (gin.H, error) {
	fields, keyHash := licenseVerifyProofFields(req, data, serverTime)
	signature, err := signResponseProof(responseProofLicense, fields)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"appKey": req.AppKey, "domain": req.Domain, "serverIp": req.ServerIP, "licenseKeyHash": keyHash,
		"nonce": req.Nonce, "serverTime": serverTime, "signature": signature,
	}, nil
}

// licenseVerifyProofFields 是 SDK 校验响应的签名字段，顺序和 docs/api-sdk.md、各语言 SDK 一致，不能改。
func licenseVerifyProofFields(req licenseVerifyRequest, data gin.H, serverTime int64) ([]proofField, string) {
	keyHash := ""
	if req.LicenseKey != "" {
		keyHash = sha256SumHex([]byte(req.LicenseKey))
	}
	text := func(key string) string {
		value, _ := data[key].(string)
		return value
	}
	expireTs := ""
	if v, ok := data["expireTs"].(int64); ok {
		expireTs = unixText(v)
	}
	return []proofField{
		{"appKey", req.AppKey},
		{"domain", req.Domain},
		{"serverIp", req.ServerIP},
		{"licenseKeyHash", keyHash},
		{"nonce", req.Nonce},
		{"serverTime", unixText(serverTime)},
		{"result", text("result")},
		{"reason", text("reason")},
		{"expireTs", expireTs},
	}, keyHash
}

// evaluateLicenseForTarget 复用公开授权校验里「黑名单 → 匹配 → 实名 → 吊销 → 过期 → 密钥站点」的判定。
// LicenseVerify 与商店状态快照都走这里，避免两套规则漂移。调用方负责写 verify_logs。
func evaluateLicenseForTarget(db *sql.DB, appID int64, domain, serverIP, licenseKey, signVersion string) (matchedLicense, string, bool) {
	req := licenseVerifyRequest{Domain: domain, ServerIP: serverIP, LicenseKey: licenseKey, SignVersion: signVersion}
	if isLicenseTargetBlacklisted(db, appID, domain, serverIP) {
		return matchedLicense{}, "target_blacklisted", false
	}
	license, ok, reason := findMatchedLicense(db, appID, req)
	if !ok {
		return matchedLicense{}, reason, false
	}
	if required, verified, err := userRealnameRequired(db, appID, license.ID); err == nil && required && !verified {
		return license, "realname_required", false
	}
	if license.Status == "revoked" {
		return license, "license_revoked", false
	}
	if license.Status == "expired" || (license.ExpiredAt.Valid && !license.ExpiredAt.Time.After(time.Now())) {
		return license, "license_expired", false
	}
	if license.Type == "key" {
		if err := requireKeyLicenseSite(db, license.ID, domain, serverIP, signVersion); err != nil {
			reason, _ := licenseSiteFailure(err)
			return license, reason, false
		}
	}
	return license, "", true
}

func licenseVerifyFailureBody(reason string) gin.H {
	switch reason {
	case "target_blacklisted":
		return gin.H{"code": 403, "msg": "授权目标已被拉黑", "data": gin.H{"result": "blacklisted", "reason": reason}}
	case "realname_required":
		return gin.H{"code": 403, "msg": "该应用要求实名认证，请先在用户中心完成实名后再安装", "data": gin.H{"result": "fail", "reason": reason}}
	case "license_revoked":
		return gin.H{"code": 403, "msg": "授权已禁用", "data": gin.H{"result": "fail", "reason": reason}}
	case "license_expired":
		return gin.H{"code": 403, "msg": "授权已过期", "data": gin.H{"result": "expired", "reason": reason}}
	default:
		if _, message := licenseSiteFailure(errors.New(reason)); message != "站点校验失败，请稍后重试" && isLicenseSiteReason(reason) {
			return gin.H{"code": 403, "msg": message, "data": gin.H{"result": "fail", "reason": reason}}
		}
		if message := licenseSiteMessage(reason); message != "" {
			return gin.H{"code": 403, "msg": message, "data": gin.H{"result": "fail", "reason": reason}}
		}
		return gin.H{"code": 403, "msg": "授权无效", "data": gin.H{"result": "fail", "reason": reason}}
	}
}

func licenseSiteMessage(reason string) string {
	switch reason {
	case "empty_target":
		return "授权站点不能为空"
	case "invalid_domain":
		return "授权域名格式不正确"
	case "invalid_server_ip":
		return "服务器 IP 格式不正确"
	case "signature_upgrade_required":
		return "新站点首次绑定需要升级 SDK 并使用 v2 签名"
	case "site_limit_exceeded":
		return "授权已达到最大站点数"
	case "site_not_bound":
		return "当前站点尚未绑定"
	case "site_check_failed":
		return "站点校验失败，请稍后重试"
	default:
		return ""
	}
}

func isLicenseSiteReason(reason string) bool {
	return licenseSiteMessage(reason) != ""
}

func licenseSiteFailure(err error) (string, string) {
	switch {
	case errors.Is(err, errLicenseSiteEmpty):
		return "empty_target", "授权站点不能为空"
	case errors.Is(err, errLicenseSiteInvalidDomain):
		return "invalid_domain", "授权域名格式不正确"
	case errors.Is(err, errLicenseSiteInvalidIP):
		return "invalid_server_ip", "服务器 IP 格式不正确"
	case errors.Is(err, errLicenseSignatureUpgrade):
		return "signature_upgrade_required", "新站点首次绑定需要升级 SDK 并使用 v2 签名"
	case errors.Is(err, errLicenseSiteLimitReached):
		return "site_limit_exceeded", "授权已达到最大站点数"
	case errors.Is(err, errLicenseSiteNotBound):
		return "site_not_bound", "当前站点尚未绑定"
	default:
		return "site_check_failed", "站点校验失败，请稍后重试"
	}
}

func findMatchedLicense(db *sql.DB, appID int64, req licenseVerifyRequest) (matchedLicense, bool, string) {
	rows, err := db.Query(`
		SELECT l.id, COALESCE(l.plan_id, 0), COALESCE(p.name, ''), l.type, l.status, l.expired_at,
		       COALESCE(ld.domain, ''), COALESCE(ld.is_wildcard, 0)
		FROM licenses l
		LEFT JOIN license_plans p ON p.id = l.plan_id AND p.app_id = l.app_id
		LEFT JOIN license_domains ld ON ld.license_id = l.id
		WHERE l.app_id = ?
		  AND (
		    (l.type = 'key' AND l.license_key = ?)
		    OR (l.type IN ('domain', 'wildcard', 'ip'))
		  )
		ORDER BY l.created_at DESC
	`, appID, req.LicenseKey)
	if err != nil {
		return matchedLicense{}, false, "query_failed"
	}
	defer rows.Close()

	for rows.Next() {
		var item matchedLicense
		var target string
		var isWildcard int
		if err := rows.Scan(
			&item.ID, &item.PlanID, &item.PlanName, &item.Type, &item.Status, &item.ExpiredAt, &target, &isWildcard,
		); err != nil {
			continue
		}
		target = normalizeLicenseTarget(target)
		if licenseRowMatchesRequest(item.Type, target, isWildcard == 1, req) {
			return item, true, ""
		}
	}

	return matchedLicense{}, false, "license_not_found"
}

func licenseRowMatchesRequest(licenseType, storedTarget string, isWildcard bool, req licenseVerifyRequest) bool {
	switch licenseType {
	case "key":
		return req.LicenseKey != ""
	case "domain":
		return req.Domain != "" && storedTarget == req.Domain
	case "wildcard":
		return req.Domain != "" && isWildcard && wildcardDomainMatch(storedTarget, req.Domain)
	case "ip":
		return storedTarget != "" && (storedTarget == req.ServerIP || storedTarget == req.Domain)
	default:
		return false
	}
}

func isLicenseTargetBlacklisted(db *sql.DB, appID int64, domain, serverIP string) bool {
	candidates := []struct {
		typeName string
		value    string
	}{
		{typeName: "domain", value: domain},
		{typeName: "ip", value: serverIP},
	}
	if net.ParseIP(domain) != nil {
		candidates = append(candidates, struct {
			typeName string
			value    string
		}{typeName: "ip", value: domain})
	}

	for _, item := range candidates {
		if item.value == "" {
			continue
		}
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM piracy_blacklist WHERE app_id = ? AND type = ? AND value = ?", appID, item.typeName, item.value).Scan(&count)
		if count > 0 {
			return true
		}
	}
	return false
}

func writeVerifyLog(db *sql.DB, licenseID sql.NullInt64, appID int64, domain, serverIP, clientIP, result, reason, userAgent string) {
	_, _ = db.Exec(`
		INSERT INTO verify_logs (license_id, app_id, domain, server_ip, client_ip, result, fail_reason, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, licenseID, appID, domain, serverIP, clientIP, result, reason, userAgent)
}

func recordPiracyHit(db *sql.DB, appID int64, domain, serverIP string) {
	target := domain
	if target == "" {
		target = serverIP
	}
	if target == "" {
		return
	}

	result, err := db.Exec(`
		UPDATE piracy_records
		SET hit_count = hit_count + 1, last_seen = NOW(), server_ip = ?
		WHERE app_id = ? AND domain = ?
	`, serverIP, appID, target)
	if err == nil {
		if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected > 0 {
			return
		}
	}

	_, _ = db.Exec(`
		INSERT INTO piracy_records (app_id, domain, server_ip, status, hit_count, first_seen, last_seen)
		VALUES (?, ?, ?, 'discovered', 1, NOW(), NOW())
	`, appID, target, serverIP)
}

func licenseVerifySignTarget(req licenseVerifyRequest) string {
	if req.LicenseKey != "" {
		return req.LicenseKey
	}
	if req.Domain != "" {
		return req.Domain
	}
	return req.ServerIP
}

func licenseVerifySignValid(req licenseVerifyRequest, appSecret, rawDomain, rawServerIP, rawLicenseKey string) (string, bool) {
	requestedVersion := strings.ToLower(strings.TrimSpace(req.SignVersion))
	switch requestedVersion {
	case "2", licenseSignVersionV2:
		return licenseSignVersionV2, hmac.Equal([]byte(req.Sign), []byte(licenseVerifyV2Sign(req, appSecret)))
	case "3", licenseSignVersionV3:
		if !validProofNonce(req.Nonce) {
			return "", false
		}
		return licenseSignVersionV3, hmac.Equal([]byte(req.Sign), []byte(licenseVerifyV3Sign(req, appSecret)))
	case "", "1", licenseSignVersionV1:
		targets := []string{licenseVerifySignTarget(req), licenseVerifyRawSignTarget(rawDomain, rawServerIP, rawLicenseKey)}
		for _, target := range targets {
			if target == "" {
				continue
			}
			if req.Sign == licenseVerifyMD5(req.AppKey+target+int64ToString(req.Timestamp)+appSecret) {
				return licenseSignVersionV1, true
			}
		}
	}
	return "", false
}

func licenseVerifyRawSignTarget(rawDomain, rawServerIP, rawLicenseKey string) string {
	if rawLicenseKey != "" {
		return rawLicenseKey
	}
	if rawDomain != "" {
		return rawDomain
	}
	return rawServerIP
}

func licenseVerifyMD5(text string) string {
	sum := md5.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

func wildcardDomainMatch(pattern, domain string) bool {
	pattern = normalizeLicenseTarget(pattern)
	domain = normalizeLicenseTarget(domain)
	if !strings.HasPrefix(pattern, "*.") {
		return pattern == domain
	}
	suffix := strings.TrimPrefix(pattern, "*")
	return strings.HasSuffix(domain, suffix) && domain != strings.TrimPrefix(pattern, "*.")
}

func normalizeLicenseTarget(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "https://")
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return value
}

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func licenseVerifySuccessData(appName string, license matchedLicense) gin.H {
	return gin.H{
		"result":   "pass",
		"appName":  appName,
		"planId":   license.PlanID,
		"planName": license.PlanName,
		"type":     license.Type,
		"expireAt": formatVerifyExpireAt(license.ExpiredAt),
		// expireTs 是到期的 Unix 秒，0 表示永久。签进 proof，SDK 的离线缓存不会用到授权过期之后。
		"expireTs": verifyExpireUnix(license.ExpiredAt),
	}
}

func verifyExpireUnix(expiredAt sql.NullTime) int64 {
	if !expiredAt.Valid {
		return 0
	}
	return expiredAt.Time.Unix()
}

func formatVerifyExpireAt(expiredAt sql.NullTime) string {
	if !expiredAt.Valid {
		return "永久"
	}
	return expiredAt.Time.Format("2006-01-02 15:04:05")
}
